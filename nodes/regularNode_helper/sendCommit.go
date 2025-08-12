package regularNode_helper

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

type CommitData struct {
	Round           string            `json:"round"`
	SecretValue     [32]byte          `json:"secret_value"`
	Cos             [32]byte          `json:"cos"`
	Cvs             [32]byte          `json:"cvs"`
	SendToLeader    bool              `json:"send_to_leader"`
	SendCosToLeader bool              `json:"send_cos_to_leader"`
	Sign            map[string]string `json:"sign"`
}

var Execution bool
var ActivatedOperator []string
var Halted int32 // 0 = false, 1 = true

func MonitorCommitRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	receiveCommitRequest(fallbackEthClient)
}

var StartTime *big.Int

type RoundData struct {
	MerkleRoot   bool
	RandomNumber bool
}

var RoundsData map[string]RoundData
var Req RandomRequest

type RandomRequest struct {
	Round     *big.Int
	StartTime *big.Int
	State     *big.Int
}

var Round *big.Int
var CurrentRound string

// Add new variables for monitoring
var (
	leaderMonitoringActive bool
	monitoringTimer        *time.Timer
	// Event tracking variables
	cvRequestedEventEmitted         bool
	merkleRootSubmittedEventEmitted bool
	CurrentTrialNum                 string
	// New variables for merkle root monitoring
	merkleRootMonitoringTimer         *time.Timer
	requestedToSubmitCvTime           *big.Int
	MerkleRootOrRequestToSubmitCoTime *big.Int
	// Variables for request to submit S or generate random number monitoring
	requestToSubmitSOrGenerateRandomNumberMonitoringActive bool
	requestToSubmitSOrGenerateRandomNumberMonitoringTimer  *time.Timer
	merkleRootSubmittedTOrRequestedCvTime                  *big.Int
)

var cvRequestIndices []*big.Int
var submittedCvIndices map[string]map[string]bool // Track which indices have submitted CV values
var submittedCvIndicesMutex sync.RWMutex          // Protect access to submittedCvIndices

func receiveCommitRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	contractAddress := os.Getenv("CONTRACT_ADDRESS")
	contractAddr := common.HexToAddress(contractAddress)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Fatalf("Failed to parse contract ABI: %v", err)
	}

	query := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddr},
	}

	for { // Outer loop for reconnection
		logs := make(chan types.Log)
		sub, err := fallbackEthClient.SubscribeFilterLogs(context.Background(), query, logs)
		if err != nil {
			log.Printf("Failed to subscribe to logs: %v. Retrying in 5 seconds...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		SubmitCVS := parsedABI.Events["RequestedToSubmitCv"].ID
		CvsEventSig := parsedABI.Events["CvSubmitted"].ID
		StatusSig := parsedABI.Events["Status"].ID
		MerkleRootSubmittedSig := parsedABI.Events["MerkleRootSubmitted"].ID
		RequestedToSubmitCoSig := parsedABI.Events["RequestedToSubmitCo"].ID
		RequestedToSubmitSFromIndexKSig := parsedABI.Events["RequestedToSubmitSFromIndexK"].ID
		SSubmittedSig := parsedABI.Events["SSubmitted"].ID

		reconnect := false
		for {
			select {
			case err := <-sub.Err():
				log.Printf("Error in event subscription: %v", err)

				if err != nil && (strings.Contains(err.Error(), "websocket: close 1006") || strings.Contains(err.Error(), "unexpected EOF")) {
					log.Printf("Websocket closed abnormally. Attempting to reconnect in 1 seconds...")
					reconnect = true
					time.Sleep(1 * time.Second)
				} else {
					log.Printf("Fatal error in event subscription: %v", err)
					reconnect = true
					time.Sleep(1 * time.Second)
				}

			case vLog := <-logs:
				{
					isReorg := vLog.Removed
					if isReorg {
						log.Printf("\033[34mReorg detected. Skipping event: %v\033[0m", vLog.TxHash)
						continue
					}
					switch vLog.Topics[0] {
					case SubmitCVS:
						eventData := struct {
							Round                         *big.Int
							TrialNum                      *big.Int
							PackedIndicesAscendingFromLSB *big.Int
						}{}
						err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitCv", vLog.Data)
						if err != nil {
							log.Printf("Failed to decode event log: %v", err)
							continue
						}

						fmt.Printf("\033[34mCommitRequest Event: Round %v, TrialNum %v, indices %v\033[0m\n", eventData.Round, eventData.TrialNum, eventData.PackedIndicesAscendingFromLSB)

						// Mark CV as requested and stop leader monitoring
						cvRequestedEventEmitted = true
						StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(eventData.Round.String(), eventData.TrialNum.String())

						// Get the block timestamp for the RequestedToSubmitCv event
						blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}
						requestedToSubmitCvTime = big.NewInt(int64(blockTimestamp))

						// Start monitoring for merkle root submission
						StartMerkleRootMonitoring(fallbackEthClient, eventData.Round.String(), eventData.TrialNum.String())

						processCommitRequest(fallbackEthClient, eventData.Round, eventData.TrialNum, eventData.PackedIndicesAscendingFromLSB)

					case StatusSig:
						eventData := struct {
							CurRound    *big.Int
							CurTrialNum *big.Int
							CurState    *big.Int
						}{}

						err := parsedABI.UnpackIntoInterface(&eventData, "Status", vLog.Data)
						if err != nil {
							log.Printf("Failed to decode Status event log: %v", err)
							continue
						}

						// Get the actual block timestamp
						blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}

						processRandomRequestNumber(fallbackEthClient, big.NewInt(int64(blockTimestamp)), eventData.CurRound, eventData.CurTrialNum, eventData.CurState)

					case MerkleRootSubmittedSig:
						eventData := struct {
							Round      *big.Int
							TrialNum   *big.Int
							MerkleRoot [32]byte
						}{}

						err := parsedABI.UnpackIntoInterface(&eventData, "MerkleRootSubmitted", vLog.Data)
						if err != nil {
							log.Printf("Failed to decode MerkleRootSubmitted event log: %v", err)
							continue
						}
						fmt.Printf("\033[34mMerkleRootSubmitted Event:\n Round %v, TrialNum %v, MerkleRoot: %v\033[0m\n",
							eventData.Round, eventData.TrialNum, eventData.MerkleRoot)

						// Mark Merkle root as submitted and stop leader monitoring
						merkleRootSubmittedEventEmitted = true
						blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
						merkleRootSubmittedTOrRequestedCvTime = big.NewInt(int64(blockTimestamp))

						StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(eventData.Round.String(), eventData.TrialNum.String())
						StopFailToSubmitMerkleRootAfterDisputeMonitoring(eventData.Round.String(), eventData.TrialNum.String())
						StartRequestToSubmitSOrGenerateRandomNumberMonitoring(fallbackEthClient, eventData.Round.String(), eventData.TrialNum.String())

						processMerkleRoot(eventData.Round, eventData.TrialNum)

						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}

					case RequestedToSubmitCoSig:
						eventData := struct {
							Round         *big.Int
							TrialNum      *big.Int
							IndicesLength *big.Int
							PackedIndices *big.Int
						}{}
						err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitCo", vLog.Data)
						if err != nil {
							log.Printf("Failed to decode RequestedToSubmitCo event log: %v", err)
							continue
						}
						fmt.Printf("\033[34mRequestedToSubmitCo Event: Round %v, TrialNum %v, indicesLength %v\n, indices %v\033[0m\n", eventData.Round, eventData.TrialNum, eventData.IndicesLength, eventData.PackedIndices)

						processCosRequest(fallbackEthClient, eventData.Round, eventData.TrialNum, eventData.PackedIndices, eventData.IndicesLength)

					case RequestedToSubmitSFromIndexKSig:
						eventData := struct {
							Round    *big.Int
							TrialNum *big.Int
							IndexK   *big.Int
						}{}

						err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitSFromIndexK", vLog.Data)

						if err != nil {
							log.Printf("Failed to decode RequestedToSubmitSFromIndexK event log: %v", err)
							continue
						}
						fmt.Printf("\033[34mRequestedToSubmitSFromIndexK Event:\n Round %v, TrialNum %v, indexK %v\033[0m\n", eventData.Round, eventData.TrialNum, eventData.IndexK)

						// Stop request to submit S or generate random number monitoring when RequestedToSubmitSFromIndexK event is received
						StopRequestToSubmitSOrGenerateRandomNumberMonitoring(eventData.Round.String(), eventData.TrialNum.String())
						blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
						merkleRootSubmittedTOrRequestedCvTime = big.NewInt(int64(blockTimestamp))
						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}
						processSecretRequest(fallbackEthClient, eventData.Round.String(), eventData.TrialNum.String(), eventData.IndexK)

					case SSubmittedSig:
						eventData := struct {
							Round    *big.Int
							TrialNum *big.Int
							S        [32]byte
							Index    *big.Int
						}{}

						err := parsedABI.UnpackIntoInterface(&eventData, "SSubmitted", vLog.Data)

						if err != nil {
							log.Printf("Failed to decode SSubmitted event log: %v", err)
							continue
						}
						fmt.Printf("\033[34mSSubmitted Event:\n Round %v, TrialNum %v, Secret %v\n, indexK %v\033[0m\n", eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)
						processSubmittedSecretRequest(fallbackEthClient, eventData.Round, eventData.TrialNum, eventData.Index)
					case CvsEventSig:
						eventData := struct {
							Round    *big.Int
							TrialNum *big.Int
							Cv       [32]byte
							Index    *big.Int
						}{}
						err := parsedABI.UnpackIntoInterface(&eventData, "CvSubmitted", vLog.Data)
						if err != nil {
							log.Printf("Failed to decode CvSubmitted event log: %v", err)
							continue
						}
						fmt.Printf("\033[34mCvSubmitted Event: Fetched successfully\033[0m\n")
						processCvSubmitted(eventData.Round, eventData.TrialNum, eventData.Index)
					}
				}
			}
			if reconnect {
				break // break inner for loop to reconnect
			}
		}
	}
}

func processCvSubmitted(round *big.Int, trialNum *big.Int, index *big.Int) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processCVS.")
		return
	}
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)

	// Use mutex to protect access to submittedCvIndices
	submittedCvIndicesMutex.Lock()
	defer submittedCvIndicesMutex.Unlock()

	// Initialize the tracking map if it doesn't exist
	if submittedCvIndices == nil {
		submittedCvIndices = make(map[string]map[string]bool)
	}

	// Initialize the inner map for this uniqueKey if it doesn't exist
	if submittedCvIndices[uniqueKey] == nil {
		submittedCvIndices[uniqueKey] = make(map[string]bool)
	}

	// Mark this index as submitted
	indexStr := index.String()
	submittedCvIndices[uniqueKey][indexStr] = true

	log.Printf("CV submitted for index %s in round %s with trial %s", indexStr, round.String(), trialNum.String())

}

func checkAllCVsSubmittedOnChain(round string, trialNum string) bool {
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Use read lock to protect access to submittedCvIndices
	submittedCvIndicesMutex.RLock()
	defer submittedCvIndicesMutex.RUnlock()

	// Check if the outer map or inner map doesn't exist
	if submittedCvIndices == nil || submittedCvIndices[uniqueKey] == nil {
		return false
	}

	// Check if all requested indices have submitted their CV values
	allSubmitted := true
	for _, requestedIndex := range cvRequestIndices {
		if !submittedCvIndices[uniqueKey][requestedIndex.String()] {
			allSubmitted = false
			break
		}
	}

	return allSubmitted
}

func processSubmittedSecretRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round, trialNum, index *big.Int) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}

	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)
	data, err := database.GetRevealOrder(round.String(), trialNum.String())
	if err != nil {
		log.Printf("Failed to get reveal order for round %s with trial %s: %v", round.String(), trialNum.String(), err)
	}
	orderedNodes := data.OrderedNodes
	revealOrder := data.RevealOrder
	if index.Int64() >= int64(len(orderedNodes)) {
		log.Printf("Index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(orderedNodes))
		return
	}
	var temp int
	intValue := int(index.Int64())
	for i, order := range revealOrder {
		if order == intValue {
			temp = i
		}
	}

	if temp+1 < len(revealOrder) {
		if temp+1 < len(orderedNodes) {
			regularEoaAddress := orderedNodes[temp+1]
			if regularNodeEOA == regularEoaAddress {
				fmt.Printf("\033[34mProcessing RequestedToSubmitSFromIndexK event for Round: %v, TrialNum: %v, EOA: %v\033[0m\n", round.String(), trialNum.String(), regularEoaAddress)
				submitS(fallbackEthClient, round.String(), trialNum.String())
			}
		}
	}
}

func processSecretRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, index *big.Int) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)
	revealOrder, err := database.GetRevealOrder(round, trialNum)
	if err != nil {
		log.Printf("Failed to get reveal order for round %s with trial %s: %v", round, trialNum, err)
	}

	if index.Int64() >= int64(len(revealOrder.OrderedNodes)) {
		log.Printf("Index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(revealOrder.OrderedNodes))
		return
	}
	// regularEoaAddress := revealOrder.OrderedNodes[2]
	// if regularNodeEOA == regularEoaAddress {
	// 	return
	// }
	regularEoaAddress := revealOrder.OrderedNodes[index.Int64()]
	if regularNodeEOA == regularEoaAddress {
		fmt.Printf("\033[34mProcessing RequestedToSubmitSFromIndexK event for Round: %v, EOA: %v\033[0m\n", round, regularEoaAddress)
		submitS(fallbackEthClient, round, trialNum)
	}

}

func submitS(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	roundData, err := database.GetCommitByRound(round, trialNum)
	if err != nil {
		log.Printf("Failed to get regular commit for round %s with : %v", round, err)
	}

	secretValueBytes := roundData.SecretValue

	fmt.Printf("Extracted secret_value as bytes32: %x\n", secretValueBytes)

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Fatalf("Failed to load contract ABI: %v", err)
	}

	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode leader private key: %v", err)
	}
	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"submitS",
		big.NewInt(0),
		secretValueBytes,
	)
	if err != nil {
		log.Printf("Failed to submit secret_value: %v", err)
		return
	}

	log.Printf("Successfully submitted secret_value: %x", secretValueBytes)
}

func processMerkleRoot(Round *big.Int, TrialNum *big.Int) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}
	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}
	uniqueKey := utils.GetUniqueKey(Round.String(), TrialNum.String())
	roundData := RoundsData[uniqueKey]
	roundData.MerkleRoot = true
	RoundsData[uniqueKey] = roundData
}

func processRandomRequestNumber(fallbackEthClient *fallback_ethclient.FallbackRPCClient, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int, state *big.Int) {

	fmt.Printf("\033[34mStatus Event:\n Round: %v\n Trial: %v\n State: %v\033[0m\n",
	round, trialNum, state)
	// Reset monitoring state for new round
	roundStr := round.String()
	ResetMonitoringState(round.String(), trialNum.String())
	MerkleRootOrRequestToSubmitCoTime = nil

	// Store the current round and trialNum from Status event
	CurrentTrialNum = trialNum.String()

	eth.UpdateActivatedOperators(fallbackEthClient)
	CurrentRound = round.String()
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())

	req := RandomRequest{
		Round:     round,
		StartTime: blockTimestamp,
		State:     state,
	}

	if state.Cmp(big.NewInt(1)) == 0 {
		// Set Halted to 0 to resume the round
		atomic.StoreInt32(&Halted, 0)
		ActivatedOperator, _ = FetchActivatedOperators(fallbackEthClient, roundStr)
		Req = req
		Execution = true

		// Start leader monitoring for the new round using block timestamp
		StartLeaderMonitoring(fallbackEthClient, blockTimestamp, round.String(), trialNum.String())
	}

	if state.Cmp(big.NewInt(2)) == 0 {
		roundData := RoundsData[uniqueKey]
		roundData.RandomNumber = true
		RoundsData[uniqueKey] = roundData
		Execution = false
		log.Printf("Execution stopped for round %s", roundStr)
	}

	if state.Cmp(big.NewInt(3)) == 0 {
		// Delete round and trial data from database
		database.DeleteRoundTrialDataForLeaderNode(CurrentRound, trialNum.String())
		// resume the round
		atomic.StoreInt32(&Halted, 1)
		// resuming(fallbackEthClient)
		Execution = false
	}

	// Update rounds data
	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}

	RoundsData[uniqueKey] = RoundData{
		MerkleRoot:   false,
		RandomNumber: false,
	}

	go AllCosReceivedUnlocked(ActivatedOperator, round.String(), trialNum.String())
}

func AllCosReceivedUnlocked(activatedOperator []string, round string, trialNum string) {
	for {
		ops := eth.ActivatedOperators
		if atomic.LoadInt32(&Halted) == 1 {
			log.Println("System is halted. Skipping AllCosReceivedUnlocked.")
			return
		}
		if allCosReceivedUnlockedRegular(round, trialNum, ops) {
			flag, _ := commitreveal2.DetermineRegularRevealOrder(round, trialNum, ops)
			if flag {
				break
			}
			time.Sleep(5 * time.Second)

		}
		time.Sleep(2 * time.Second)
	}
}

func allCosReceivedUnlockedRegular(round string, trialNum string, ops []common.Address) bool {
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	for _, op := range ops {
		v, ok := GetCosReceived(uniqueKey, op.Hex())
		if !ok || !v {
			return false
		}
	}
	return true
}

func processCommitRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round *big.Int, trialNum *big.Int, packedIndices *big.Int) error {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processCommitRequest.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v, packedIndices %v\n", round, trialNum, packedIndices)
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode Ethereum private key: %v", err)
	}
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	acitvatedOps := ActivatedOperator
	indices := unpackIndices(packedIndices)
	// Copy indices by value (deep copy)
	cvRequestIndices = make([]*big.Int, len(indices))
	for i, index := range indices {
		cvRequestIndices[i] = new(big.Int).Set(index)
	}
	flag, err := findEOAAddress(indices, acitvatedOps, eoaAddress)

	if err != nil {
		fmt.Println(err)
	}
	if !flag {
		fmt.Println("Cv Request does not contain our EOA")
		return nil
	}

	fmt.Printf("\033[34mProcessing RequestedToSubmitCv event for Round: %v\033[0m\n", round.String())

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to load contract ABI: %v", err)
	}

	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	commitData, err := database.GetCommitByRound(round.String(), trialNum.String())
	if err != nil {
		return err
	}

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"submitCv",
		big.NewInt(0),
		commitData.Cvs,
	)
	if err != nil {
		fmt.Println("It contains error")
	}

	return nil
}

func processCosRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, Round *big.Int, TrialNum *big.Int, packedIndices *big.Int, indicesLength *big.Int) error {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processCosRequest.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v\n", Round, TrialNum)
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode Ethereum private key: %v", err)
	}
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	acitvatedOps := ActivatedOperator
	indices := unpackIndicesWithLength(packedIndices, indicesLength)
	flag, err := findEOAAddress(indices, acitvatedOps, eoaAddress)

	if err != nil {
		fmt.Println(err)
	}
	if !flag {
		fmt.Println("Cos Request does not contain our EOA")
		return nil
	}

	fmt.Printf("\033[34mProcessing RequestedToSubmitCo event for Round: %v\033[0m\n", Round.String())

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to load contract ABI: %v", err)
	}

	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	commitData, err := database.GetCommitByRound(Round.String(), TrialNum.String())
	if err != nil {
		fmt.Println("Error loading commits:", err)
		return err
	}

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"submitCo",
		big.NewInt(0),
		commitData.Cos,
	)
	if err != nil {
		fmt.Println("It contains error")
	}
	return nil
}

func unpackIndices(packedIndices *big.Int) []*big.Int {
	order := []*big.Int{}
	mask := big.NewInt(0xFF)
	i := 0
	for {
		shift := uint(8 * i)
		shifted := new(big.Int).Rsh(packedIndices, shift)
		value := new(big.Int).And(shifted, mask)

		if i != 0 && value.Cmp(big.NewInt(0)) == 0 {
			break
		}
		order = append(order, value)
		i++
	}
	return order
}

func unpackIndicesWithLength(unpackIndices *big.Int, indicesLength *big.Int) []*big.Int {
	order := []*big.Int{}
	mask := big.NewInt(0xFF)
	i := 0
	for i < int(indicesLength.Int64()) {
		shift := uint(8 * i)
		shifted := new(big.Int).Rsh(unpackIndices, shift)
		value := new(big.Int).And(shifted, mask)
		order = append(order, value)
		i++
	}
	return order
}

func findEOAAddress(indices []*big.Int, activatedOps []string, eoaAddress string) (bool, error) {
	if len(indices) > len(activatedOps) {
		return false, fmt.Errorf("indices length is greater than activated operators")
	}
	for _, index := range indices {
		if eoaAddress == activatedOps[index.Int64()] {
			return true, nil
		}
	}
	return false, nil
}

func FetchActivatedOperators(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string) ([]string, error) {
	var result []string
	activatedOperators, err := eth.GetActivatedOperators(fallbackEthClient)
	if err != nil {
		log.Printf("Error fetching the activated operators %v", err)
		return result, err
	}
	strAddresses := make([]string, len(activatedOperators))
	for i, addr := range activatedOperators {
		strAddresses[i] = addr.Hex()
	}
	return strAddresses, nil
}

// StartLeaderMonitoring starts monitoring the leader for the current round
func StartLeaderMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, startTime *big.Int, round string, trialNum string) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping StartLeaderMonitoring.")
		return
	}
	if leaderMonitoringActive {
		log.Printf("Leader monitoring already active for round %s", round)
		return
	}

	if startTime == nil {
		// log.Printf("StartTime is nil, cannot start monitoring")
		return
	}

	leaderMonitoringActive = true

	// Get timing parameters from contract
	offChainSubmissionPeriod := big.NewInt(40)
	requestOrSubmitOrFailDecisionPeriod := big.NewInt(30)

	// Calculate deadline: startTime + offChainSubmissionPeriod + requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(startTime, offChainSubmissionPeriod)
	deadline.Add(deadline, requestOrSubmitOrFailDecisionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	if duration <= 0 {
		log.Printf("Deadline has already passed for round %s with trial %s, calling failToRequestSubmitCVOrSubmitMerkleRoot immediately", round, trialNum)
		callFailToRequestSubmitCVOrSubmitMerkleRoot(fallbackEthClient, round, trialNum)
		return
	}

	// Set timer to call the function when deadline is reached
	monitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToRequestSubmitCVOrSubmitMerkleRoot", round)
		callFailToRequestSubmitCVOrSubmitMerkleRoot(fallbackEthClient, round, trialNum)
		leaderMonitoringActive = false
	})
}

// StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring stops the current leader monitoring
func StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(round string, trialNum string) {
	if !leaderMonitoringActive {
		return
	}

	if monitoringTimer != nil {
		monitoringTimer.Stop()
		monitoringTimer = nil
	}

	leaderMonitoringActive = false
	// log.Printf("Stopped leader monitoring for round %s", round)
}

// ResetMonitoringState resets all monitoring variables
func ResetMonitoringState(round string, trialNum string) {
	StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(round, trialNum)
	StopFailToSubmitMerkleRootAfterDisputeMonitoring(round, trialNum)
	StopRequestToSubmitSOrGenerateRandomNumberMonitoring(round, trialNum)
	cvRequestedEventEmitted = false
	merkleRootSubmittedEventEmitted = false
	requestedToSubmitCvTime = nil
	merkleRootSubmittedTOrRequestedCvTime = nil

	// Use mutex to protect access to submittedCvIndices
	submittedCvIndicesMutex.Lock()
	cvRequestIndices = nil
	submittedCvIndices = nil
	submittedCvIndicesMutex.Unlock()

	log.Printf("Reset monitoring state")
}

// callFailToRequestSubmitCVOrSubmitMerkleRoot calls the contract function to fail the leader
func callFailToRequestSubmitCVOrSubmitMerkleRoot(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode Ethereum private key: %v", err)
		return
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to load contract ABI: %v", err)
		return
	}

	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"failToRequestSubmitCvOrSubmitMerkleRoot",
		big.NewInt(0),
	)
	if err != nil {
		log.Printf("Failed to call failToRequestSubmitCVOrSubmitMerkleRoot: %v", err)
		return
	}

	log.Printf("Successfully called failToRequestSubmitCVOrSubmitMerkleRoot for round %swith trial %s", round, trialNum)
}

// StartMerkleRootMonitoring starts monitoring for merkle root submission after CV request
func StartMerkleRootMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {

	if requestedToSubmitCvTime == nil {
		log.Printf("RequestedToSubmitCvTime is nil, cannot start merkle root monitoring")
		return
	}

	// Get timing parameters from contract
	onChainSubmissionPeriod := big.NewInt(60)            // onChainSubmissionPeriod = 120
	requestOrSubmitOrFailDecisionPeriod := big.NewInt(30) // requestOrSubmitOrFailDecisionPeriod = 60

	// Calculate deadline: requestedToSubmitCvTime + onChainSubmissionPeriod + requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(requestedToSubmitCvTime, onChainSubmissionPeriod)
	deadline.Add(deadline, requestOrSubmitOrFailDecisionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	log.Printf("Starting merkle root monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	merkleRootMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, checking conditions before calling failToSubmitMerkleRootAfterDispute", round)

		// Check if all CVs have been submitted on-chain and merkle root hasn't been submitted
		if checkAllCVsSubmittedOnChain(round, trialNum) && !merkleRootSubmittedEventEmitted {
			log.Printf("All CVs submitted on-chain but merkle root not submitted, calling failToSubmitMerkleRootAfterDispute")
			callFailToSubmitMerkleRootAfterDispute(fallbackEthClient, round, trialNum)
		} else {
			log.Printf("Conditions not met for failToSubmitMerkleRootAfterDispute - CVs not all submitted or merkle root already submitted")
		}
	})
}

// StopFailToSubmitMerkleRootAfterDisputeMonitoring stops the current merkle root monitoring
func StopFailToSubmitMerkleRootAfterDisputeMonitoring(round string, trialNum string) {

	if merkleRootMonitoringTimer != nil {
		merkleRootMonitoringTimer.Stop()
		merkleRootMonitoringTimer = nil
	}

	// log.Printf("Stopped merkle root monitoring for round %s", round)
}

// callFailToSubmitMerkleRootAfterDispute calls the contract function to fail the leader for not submitting merkle root
func callFailToSubmitMerkleRootAfterDispute(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode Ethereum private key: %v", err)
		return
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to load contract ABI: %v", err)
		return
	}

	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"failToSubmitMerkleRootAfterDispute",
		big.NewInt(0),
	)
	if err != nil {
		log.Printf("Failed to call failToSubmitMerkleRootAfterDispute: %v", err)
		return
	}

	log.Printf("Successfully called failToSubmitMerkleRootAfterDispute for round %s with trial %s", round, trialNum)
}

// CheckAndStartMonitoring checks if monitoring should be started and starts it if needed
func CheckAndStartMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	// Check if monitoring is already active
	if leaderMonitoringActive {
		log.Printf("Leader monitoring already active for round %s", round)
		return
	}

	// Check if CV has been requested
	if cvRequestedEventEmitted {
		log.Printf("CV requested event already emitted for round %s", round)
		return
	}

	// Check if Merkle root has been submitted
	if merkleRootSubmittedEventEmitted {
		// log.Printf("Merkle root submitted event already emitted for round %s", round)
		return
	}

	// Check if we have a valid start time
	if StartTime == nil {
		// log.Printf("StartTime is nil, cannot start monitoring")
		return
	}

	StartLeaderMonitoring(fallbackEthClient, StartTime, round, trialNum)
}

// StartRequestToSubmitSOrGenerateRandomNumberMonitoring starts monitoring for failToRequestSOrGenerateRandomNumber condition
func StartRequestToSubmitSOrGenerateRandomNumberMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping StartRequestToSubmitSOrGenerateRandomNumberMonitoring.")
		return
	}

	if requestToSubmitSOrGenerateRandomNumberMonitoringActive {
		log.Printf("Request to submit S or generate random number monitoring already active for round %s", round)
		return
	}

	offChainSubmissionPeriod := big.NewInt(40)
	offChainSubmissionPeriodPerOperator := big.NewInt(20)
	activatedOperatorsLength := new(big.Int).SetInt64(int64(len(eth.ActivatedOperators)))
	requestOrSubmitOrFailDecisionPeriod := big.NewInt(30)

	// Calculate deadline: s_merkleRootSubmittedTime + s_offChainSubmissionPeriod + (s_offChainSubmissionPeriodPerOperator * activatedOperatorsLength) + s_requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(merkleRootSubmittedTOrRequestedCvTime, offChainSubmissionPeriod)
	operatorDelay := new(big.Int).Mul(offChainSubmissionPeriodPerOperator, activatedOperatorsLength)
	deadline.Add(deadline, operatorDelay)
	deadline.Add(deadline, requestOrSubmitOrFailDecisionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	requestToSubmitSOrGenerateRandomNumberMonitoringActive = true

	log.Printf("\033[31mStarting request to submit S or generate random number monitoring for round %s, deadline: %v (in %v)\033[0m", round, deadlineTime, duration)
	log.Printf("Parameters - merkleRootSubmittedTime: %v, offChainSubmissionPeriod: %v, offChainSubmissionPeriodPerOperator: %v, activatedOperatorsLength: %v, requestOrSubmitOrFailDecisionPeriod: %v",
		merkleRootSubmittedTOrRequestedCvTime, offChainSubmissionPeriod, offChainSubmissionPeriodPerOperator, activatedOperatorsLength, requestOrSubmitOrFailDecisionPeriod)

	// Set timer to call the function when deadline is reached
	requestToSubmitSOrGenerateRandomNumberMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToRequestSOrGenerateRandomNumber", round)
		callFailToRequestSOrGenerateRandomNumber(fallbackEthClient, round, trialNum)
		requestToSubmitSOrGenerateRandomNumberMonitoringActive = false
	})
}

// StopRequestToSubmitSOrGenerateRandomNumberMonitoring stops the monitoring
func StopRequestToSubmitSOrGenerateRandomNumberMonitoring(round string, trialNum string) {
	if !requestToSubmitSOrGenerateRandomNumberMonitoringActive {
		return
	}

	if requestToSubmitSOrGenerateRandomNumberMonitoringTimer != nil {
		requestToSubmitSOrGenerateRandomNumberMonitoringTimer.Stop()
		requestToSubmitSOrGenerateRandomNumberMonitoringTimer = nil
	}

	requestToSubmitSOrGenerateRandomNumberMonitoringActive = false
	log.Printf("Stopped request to submit S or generate random number monitoring for round %s", round)
}

// callFailToRequestSOrGenerateRandomNumber calls the contract function to fail
func callFailToRequestSOrGenerateRandomNumber(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	log.Printf("Calling failToRequestSOrGenerateRandomNumber for round %s with trial %s", round, trialNum)

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to load contract ABI: %v", err)
		return
	}

	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in environment variables.")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode private key: %v", err)
		return
	}

	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	// Execute the transaction
	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"failToRequestSorGenerateRandomNumber",
		big.NewInt(0),
	)
	if err != nil {
		log.Printf("Failed to execute failToRequestSOrGenerateRandomNumber transaction: %v", err)
		return
	}

	log.Printf("Successfully called failToRequestSOrGenerateRandomNumber for round %s with trial %s", round, trialNum)
}

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
	"unsafe"

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

// Atomic variables for thread safety
var Execution int32 // 0 = false, 1 = true
var Halted int32    // 0 = false, 1 = true

// ActivatedOperator slice with mutex protection
var ActivatedOperator []string
var ActivatedOperatorMu sync.RWMutex

// String variables - using atomic with unsafe.Pointer
var CurrentRound unsafe.Pointer    // *string
var CurrentTrialNum unsafe.Pointer // *string

// Map variables with mutex protection
var RoundsData map[string]RoundData
var RoundsDataMu sync.RWMutex

var StartTime *big.Int
var StartTimeMu sync.RWMutex

// StartTime getter/setter functions
func SetStartTime(timestamp *big.Int) {
	StartTimeMu.Lock()
	defer StartTimeMu.Unlock()
	StartTime = timestamp
}

func GetStartTime() *big.Int {
	StartTimeMu.RLock()
	defer StartTimeMu.RUnlock()
	return StartTime
}

type RoundData struct {
	MerkleRoot   bool
	RandomNumber bool
}

// Add new variables for monitoring with atomic protection
var (
	leaderMonitoringActive                                int32 // 0 = false, 1 = true
	monitoringTimer                                       *time.Timer
	merkleRootSubmittedEventEmitted                       int32 // 0 = false, 1 = true
	merkleRootMonitoringTimer                             *time.Timer
	requestToSubmitSOrGenerateRandomNumberMonitoringTimer *time.Timer
	merkleRootSubmittedTOrRequestedCvTime                 *big.Int
	merkleRootTimeMu                                      sync.RWMutex // Protect big.Int pointer
)

var cvRequestIndices []*big.Int
var cvRequestIndicesMu sync.RWMutex

var submittedCvIndices map[string]map[string]bool // Track which indices have submitted CV values
var submittedCvIndicesMutex sync.RWMutex          // Protect access to submittedCvIndices

func MonitorCommitRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	receiveCommitRequest(fallbackEthClient)
}

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

		RequestedToSubmitCvS := parsedABI.Events["RequestedToSubmitCv"].ID
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
				} else {
					log.Printf("Fatal error in event subscription: %v", err)
				}
				time.Sleep(1 * time.Second)
				reconnect = true

			case vLog := <-logs:
				{
					isReorg := vLog.Removed
					if isReorg {
						log.Printf("Reorg detected. Skipping event: %v", vLog.TxHash)
						continue
					}
					switch vLog.Topics[0] {
					case RequestedToSubmitCvS:
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

						fmt.Printf("CommitRequest Event: Round %v, TrialNum %v, indices %v\n", eventData.Round, eventData.TrialNum, eventData.PackedIndicesAscendingFromLSB)

						// Mark CV as requested and stop leader monitoring
						StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(eventData.Round.String(), eventData.TrialNum.String())

						// Get the block timestamp for the RequestedToSubmitCv event
						blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}
						// requestedToSubmitCvTime = big.NewInt(int64(blockTimestamp))

						// Start monitoring for merkle root submission
						StartMerkleRootMonitoring(fallbackEthClient, eventData.Round.String(), eventData.TrialNum.String(), big.NewInt(int64(blockTimestamp)))

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
						fmt.Printf("MerkleRootSubmitted Event:\n Round %v, TrialNum %v, MerkleRoot: %v\n Round: %v\n",
							eventData.Round, eventData.TrialNum, eventData.MerkleRoot, eventData.Round.String())

						// Mark Merkle root as submitted and stop leader monitoring
						SetMerkleRootSubmittedEventEmitted(true)
						blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
						MerkleRootSubmittedTOrRequestedCvTime(big.NewInt(int64(blockTimestamp)))

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
						fmt.Printf("RequestedToSubmitCo Event: Round %v, TrialNum %v, indicesLength %v\n, indices %v\n", eventData.Round, eventData.TrialNum, eventData.IndicesLength, eventData.PackedIndices)

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
						fmt.Printf("RequestedToSubmitSFromIndexK Event:\n Round %v, TrialNum %v, indexK %v\n", eventData.Round, eventData.TrialNum, eventData.IndexK)

						// Stop request to submit S or generate random number monitoring when RequestedToSubmitSFromIndexK event is received
						StopRequestToSubmitSOrGenerateRandomNumberMonitoring(eventData.Round.String(), eventData.TrialNum.String())
						blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
						MerkleRootSubmittedTOrRequestedCvTime(big.NewInt(int64(blockTimestamp)))
						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}
						processSecretRequest(fallbackEthClient, eventData.Round, eventData.TrialNum, eventData.IndexK)

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
						fmt.Printf("SSubmitted Event:\n Round %v, TrialNum %v, Secret %v\n, indexK %v\n ", eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)
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
						fmt.Printf("CvSubmitted Event: Fetched successfully")
						processCvSubmitted(eventData.Round, eventData.TrialNum, eventData.Index)
					}
				}
			}
			if reconnect {
				log.Printf("Reconnection triggered, breaking out of event loop to restart subscription...")
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

	// Use the new atomic setter function
	indexStr := index.String()
	SetSubmittedCvIndicesValue(uniqueKey, indexStr, true)

	log.Printf("CV submitted for index %s in round %s with trail %s", indexStr, round.String(), trialNum.String())

}

func checkAllCVsSubmittedOnChain(round string, trialNum string) bool {
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Check if the map doesn't exist
	submittedCvIndicesMap, exists := GetSubmittedCvIndicesMap(uniqueKey)
	if !exists {
		return false
	}

	// Check if all requested indices have submitted their CV values
	indices := GetCvRequestIndices()
	allSubmitted := true
	for _, requestedIndex := range indices {
		submitted, exists := submittedCvIndicesMap[requestedIndex.String()]
		if !exists || !submitted {
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
		log.Printf("Failed to get reveal order for round %s with trail %s: %v", round.String(), trialNum.String(), err)
	}
	orderedNodes := data.OrderedNodes
	revealOrder := data.RevealOrder
	if index.Int64() >= int64(len(orderedNodes)) {
		log.Printf("Index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(orderedNodes))
		return
	}
	var indexInRevealOrder int
	intValue := int(index.Int64())
	for i, order := range revealOrder {
		if order == intValue {
			indexInRevealOrder = i
		}
	}
	eoaAddress := GetRegularNodeEOA()
	if indexInRevealOrder+1 < len(revealOrder) {
		if indexInRevealOrder+1 < len(orderedNodes) {
			regularEoaAddress := orderedNodes[indexInRevealOrder+1]
			if eoaAddress == regularEoaAddress {
				fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, TrialNum: %v, EOA: %v\n", round.String(), trialNum.String(), regularEoaAddress)
				submitS(fallbackEthClient, round.String(), trialNum.String())
			}
		}
	}
}

func processSecretRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round, trialNum, index *big.Int) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)

	// Try to get reveal order, if it doesn't exist, try to create it
	revealOrder, err := database.GetRevealOrder(round.String(), trialNum.String())
	if err != nil {
		log.Printf("Failed to get reveal order for round %s with trail %s: %v", round.String(), trialNum.String(), err)
		log.Printf("Attempting to determine reveal order for regular node...")

		// Try to determine reveal order for regular node
		ops := eth.ActivatedOperators

		success, err := commitreveal2.DetermineRegularRevealOrder(round.String(), trialNum.String(), ops)
		if err != nil || !success {
			log.Printf("Failed to determine reveal order: %v", err)
			return
		}

		// Try to get reveal order again after creation
		revealOrder, err = database.GetRevealOrder(round.String(), trialNum.String())
		if err != nil {
			log.Printf("Still failed to get reveal order after creation: %v", err)
			return
		}
		log.Printf("Successfully created and retrieved reveal order")
	}

	// Check bounds safely
	if revealOrder == nil {
		log.Printf("Reveal order is nil for round %s with trail %s", round.String(), trialNum.String())
		return
	}

	if revealOrder.OrderedNodes == nil {
		log.Printf("Reveal order ordered nodes is nil for round %s with trail %s", round.String(), trialNum.String())
		return
	}

	if index.Int64() >= int64(len(revealOrder.OrderedNodes)) {
		log.Printf("Index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(revealOrder.OrderedNodes))
		return
	}

	regularEoaAddress := revealOrder.OrderedNodes[index.Int64()]
	if GetRegularNodeEOA() == regularEoaAddress {
		fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, EOA: %v\n", round, regularEoaAddress)
		submitS(fallbackEthClient, round.String(), trialNum.String())
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
	fmt.Printf("Round %v, TrialNum %v\n", Round, TrialNum)
	uniqueKey := utils.GetUniqueKey(Round.String(), TrialNum.String())
	roundData, exists := GetRoundData(uniqueKey)
	if !exists {
		roundData = RoundData{}
	}
	roundData.MerkleRoot = true
	SetRoundData(uniqueKey, roundData)
}

func processRandomRequestNumber(fallbackEthClient *fallback_ethclient.FallbackRPCClient, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int, state *big.Int) {

	fmt.Printf("Round %v, TrialNum %v, state %v\n", round, trialNum, state)

	// fetch the last round and trial, and cleanup the data (current round and trial has not been updated yet)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	EnqueueUniqueKeyForCleanup(uniqueKey)

	roundStr := round.String()
	// Reset monitoring state for new round
	ResetMonitoringState(round.String(), trialNum.String())

	// Store the current round and trialNum from Status event
	SetCurrentTrialNum(trialNum.String())

	eth.UpdateActivatedOperators(fallbackEthClient)
	// Convert []common.Address to []string for SetActivatedOperator
	stringAddrs := make([]string, len(eth.ActivatedOperators))
	for i, addr := range eth.ActivatedOperators {
		stringAddrs[i] = addr.Hex()
	}
	SetActivatedOperator(stringAddrs)
	SetCurrentRound(round.String())

	if state.Cmp(big.NewInt(1)) == 0 {
		// Set Halted to 0 to resume the round
		atomic.StoreInt32(&Halted, 0)
		// Delete old round data except current round from database
		err := database.DeleteOldRoundDataForRegularNode(round.String())
		if err != nil {
			log.Printf("Failed to delete old round data except round %v for regular node\n", round)
		}

		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			blockTimestamp, state, round)
		operators := FetchActivatedOperators(fallbackEthClient, roundStr)
		SetActivatedOperator(operators)
		SetExecution(true)
		log.Printf("Execution started for round %s", roundStr)

		// Start leader monitoring for the new round using block timestamp
		StartLeaderMonitoring(fallbackEthClient, blockTimestamp, round.String(), trialNum.String())
		go AllCosReceivedUnlocked(GetActivatedOperator(), round.String(), trialNum.String())
	}

	if state.Cmp(big.NewInt(2)) == 0 {
		// Delete old round data except current round from database
		err := database.DeleteOldRoundDataForRegularNode(round.String())
		if err != nil {
			log.Printf("Failed to delete old round data except round %v for regular node\n", round)
		}
		// Update in-memory round data
		roundData, exists := GetRoundData(uniqueKey)
		if !exists {
			roundData = RoundData{}
		}
		roundData.RandomNumber = true
		SetRoundData(uniqueKey, roundData)
		SetExecution(false)
		log.Printf("Execution stopped for round %s", roundStr)
	}

	if state.Cmp(big.NewInt(3)) == 0 {
		// Delete round and trial data from database
		database.DeleteRoundTrialDataForRegularNode(GetCurrentRound(), trialNum.String())
		// resume the round
		atomic.StoreInt32(&Halted, 1)
		// resuming(fallbackEthClient)
		SetExecution(false)
	}

	// Update rounds data
	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}

	SetRoundData(uniqueKey, RoundData{
		MerkleRoot:   false,
		RandomNumber: false,
	})
}

func CleanupRoundDataByUniqueKey(uniqueKey string) {
	DeleteRoundsData(uniqueKey)
	DeleteSubmittedCvIndices(uniqueKey)
	DeleteStrictOrder(uniqueKey)
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

	acitvatedOps := GetActivatedOperator()
	indices := unpackIndices(packedIndices)
	// Copy indices by value (deep copy)
	SetCvRequestIndices(indices)
	flag, err := findEOAAddress(indices, acitvatedOps, eoaAddress)

	if err != nil {
		fmt.Println(err)
	}
	if !flag {
		fmt.Println("Cv Request does not contain our EOA")
		return nil
	}

	fmt.Printf("Processing RequestedToSubmitCv event for Round: %v\n", round.String())

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

	acitvatedOps := GetActivatedOperator()
	indices := unpackIndicesWithLength(packedIndices, indicesLength)
	flag, err := findEOAAddress(indices, acitvatedOps, eoaAddress)

	if err != nil {
		fmt.Println(err)
	}
	if !flag {
		fmt.Println("Cos Request does not contain our EOA")
		return nil
	}

	fmt.Printf("Processing RequestedToSubmitCo event for Round: %v\n", Round.String())

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

func FetchActivatedOperators(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string) []string {
	var result []string
	activatedOperators, err := eth.GetActivatedOperators(fallbackEthClient)
	if err != nil {
		log.Printf("Error fetching the activated operators %v", err)
		return result
	}
	strAddresses := make([]string, len(activatedOperators))
	for i, addr := range activatedOperators {
		strAddresses[i] = addr.Hex()
	}
	return strAddresses
}

// StartLeaderMonitoring starts monitoring the leader for the current round
func StartLeaderMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, startTime *big.Int, round string, trialNum string) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping StartLeaderMonitoring.")
		return
	}
	if GetLeaderMonitoringActive() {
		log.Printf("Leader monitoring already active for round %s", round)
		return
	}

	if startTime == nil {
		log.Printf("StartTime is nil, cannot start monitoring")
		return
	}

	SetLeaderMonitoringActive(true)

	// Get timing parameters from contract
	offChainSubmissionPeriod := big.NewInt(80)
	requestOrSubmitOrFailDecisionPeriod := big.NewInt(60)

	// Calculate deadline: startTime + offChainSubmissionPeriod + requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(startTime, offChainSubmissionPeriod)
	deadline.Add(deadline, requestOrSubmitOrFailDecisionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	if duration <= 0 {
		log.Printf("Deadline has already passed for round %s with trail %s, calling failToRequestSubmitCVOrSubmitMerkleRoot immediately", round, trialNum)
		callFailToRequestSubmitCVOrSubmitMerkleRoot(fallbackEthClient, round, trialNum)
		return
	}

	log.Printf("Starting leader monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	monitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToRequestSubmitCVOrSubmitMerkleRoot", round)
		callFailToRequestSubmitCVOrSubmitMerkleRoot(fallbackEthClient, round, trialNum)
		SetLeaderMonitoringActive(false)
	})
}

// StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring stops the current leader monitoring
func StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(round string, trialNum string) {
	if !GetLeaderMonitoringActive() {
		return
	}

	if monitoringTimer != nil {
		monitoringTimer.Stop()
		monitoringTimer = nil
	}

	SetLeaderMonitoringActive(false)
	log.Printf("Stopped leader monitoring for round %s", round)
}

// ResetMonitoringState resets all monitoring variables
func ResetMonitoringState(round string, trialNum string) {
	StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(round, trialNum)
	StopFailToSubmitMerkleRootAfterDisputeMonitoring(round, trialNum)
	StopRequestToSubmitSOrGenerateRandomNumberMonitoring(round, trialNum)
	SetMerkleRootSubmittedEventEmitted(false)
	// requestedToSubmitCvTime = nil
	MerkleRootSubmittedTOrRequestedCvTime(nil)

	// Reset monitoring state variables
	ClearCvRequestIndices()
	// Note: submittedCvIndices will be cleaned up by the cleanup queue system

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

	log.Printf("Successfully called failToRequestSubmitCVOrSubmitMerkleRoot for round %swith trail %s", round, trialNum)
}

// StartMerkleRootMonitoring starts monitoring for merkle root submission after CV request
func StartMerkleRootMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, requestedToSubmitCvTime *big.Int) {

	if requestedToSubmitCvTime == nil {
		log.Printf("RequestedToSubmitCvTime is nil, cannot start merkle root monitoring")
		return
	}

	// Get timing parameters from contract
	onChainSubmissionPeriod := big.NewInt(120)            // onChainSubmissionPeriod = 120
	requestOrSubmitOrFailDecisionPeriod := big.NewInt(60) // requestOrSubmitOrFailDecisionPeriod = 60

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
		if checkAllCVsSubmittedOnChain(round, trialNum) && !GetMerkleRootSubmittedEventEmitted() {
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

	log.Printf("Stopped merkle root monitoring for round %s", round)
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

	log.Printf("Successfully called failToSubmitMerkleRootAfterDispute for round %s with trail %s", round, trialNum)
}

// CheckAndStartMonitoring checks if monitoring should be started and starts it if needed
func CheckAndStartMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	// Check if monitoring is already active
	if GetLeaderMonitoringActive() {
		log.Printf("Leader monitoring already active for round %s", round)
		return
	}

	// Check if Merkle root has been submitted
	if GetMerkleRootSubmittedEventEmitted() {
		log.Printf("Merkle root submitted event already emitted for round %s", round)
		return
	}

	// Check if we have a valid start time
	startTime := GetStartTime()
	if startTime == nil {
		log.Printf("StartTime is nil, cannot start monitoring")
		return
	}

	StartLeaderMonitoring(fallbackEthClient, startTime, round, trialNum)
}

// StartRequestToSubmitSOrGenerateRandomNumberMonitoring starts monitoring for failToRequestSOrGenerateRandomNumber condition
func StartRequestToSubmitSOrGenerateRandomNumberMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping StartRequestToSubmitSOrGenerateRandomNumberMonitoring.")
		return
	}

	offChainSubmissionPeriod := big.NewInt(80)
	offChainSubmissionPeriodPerOperator := big.NewInt(20)
	activatedOperatorsLength := new(big.Int).SetInt64(int64(len(eth.ActivatedOperators)))
	requestOrSubmitOrFailDecisionPeriod := big.NewInt(60)

	// Calculate deadline: s_merkleRootSubmittedTime + s_offChainSubmissionPeriod + (s_offChainSubmissionPeriodPerOperator * activatedOperatorsLength) + s_requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(GetMerkleRootSubmittedTOrRequestedCvTime(), offChainSubmissionPeriod)
	operatorDelay := new(big.Int).Mul(offChainSubmissionPeriodPerOperator, activatedOperatorsLength)
	deadline.Add(deadline, operatorDelay)
	deadline.Add(deadline, requestOrSubmitOrFailDecisionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	log.Printf("Starting request to submit S or generate random number monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)
	log.Printf("Parameters - merkleRootSubmittedTime: %v, offChainSubmissionPeriod: %v, offChainSubmissionPeriodPerOperator: %v, activatedOperatorsLength: %v, requestOrSubmitOrFailDecisionPeriod: %v",
		GetMerkleRootSubmittedTOrRequestedCvTime(), offChainSubmissionPeriod, offChainSubmissionPeriodPerOperator, activatedOperatorsLength, requestOrSubmitOrFailDecisionPeriod)

	// Set timer to call the function when deadline is reached
	requestToSubmitSOrGenerateRandomNumberMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToRequestSOrGenerateRandomNumber", round)
		callFailToRequestSOrGenerateRandomNumber(fallbackEthClient, round, trialNum)
	})
}

// StopRequestToSubmitSOrGenerateRandomNumberMonitoring stops the monitoring
func StopRequestToSubmitSOrGenerateRandomNumberMonitoring(round string, trialNum string) {

	if requestToSubmitSOrGenerateRandomNumberMonitoringTimer != nil {
		requestToSubmitSOrGenerateRandomNumberMonitoringTimer.Stop()
		requestToSubmitSOrGenerateRandomNumberMonitoringTimer = nil
	}

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

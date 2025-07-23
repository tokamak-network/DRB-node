package regularNode_helper

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
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
)

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
	logs := make(chan types.Log)
	sub, err := fallbackEthClient.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		log.Fatalf("Failed to subscribe to logs: %v", err)
	}

	SubmitCVS := parsedABI.Events["RequestedToSubmitCv"].ID
	StatusSig := parsedABI.Events["Status"].ID
	MerkleRootSubmittedSig := parsedABI.Events["MerkleRootSubmitted"].ID
	RequestedToSubmitCoSig := parsedABI.Events["RequestedToSubmitCo"].ID
	RequestedToSubmitSFromIndexKSig := parsedABI.Events["RequestedToSubmitSFromIndexK"].ID
	SSubmittedSig := parsedABI.Events["SSubmitted"].ID

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("Error in event subscription: %v", err)

		case vLog := <-logs:
			{
				isReorg := vLog.Removed
				if isReorg {
					log.Printf("Reorg detected. Skipping event: %v", vLog.TxHash)
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

					fmt.Printf("CommitRequest Event: Round %v, TrialNum %v, indices %v\n", eventData.Round, eventData.TrialNum, eventData.PackedIndicesAscendingFromLSB)

					// Mark CV as requested and stop leader monitoring
					cvRequestedEventEmitted = true
					StopLeaderMonitoring(eventData.Round.String(), eventData.TrialNum.String())

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
					merkleRootSubmittedEventEmitted = true
					StopLeaderMonitoring(eventData.Round.String(), eventData.TrialNum.String())

					processMerkleRoot(eventData.Round, eventData.TrialNum)

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
					fmt.Printf("SSubmitted Event:\n Round %v, TrialNum %v, Secret %v\n, indexK %v\n ", eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)
					// index := new(big.Int).Add(eventData.Index, big.NewInt(1))
					processSubmittedSecretRequest(fallbackEthClient, eventData.Round, eventData.TrialNum, eventData.Index)
				}
			}
		}
	}
}

func processSubmittedSecretRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round *big.Int, trialNum *big.Int, index *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	data, err := database.GetRevealOrder(round.String(), trialNum.String(), uniqueKey)
	if err != nil {
		log.Printf("Failed to get reveal order for round %s with trail %s: %v", round.String(), trialNum.String(), err)
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
			if EoaAddress == regularEoaAddress {
				fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, TrialNum: %v, EOA: %v\n", round.String(), trialNum.String(), regularEoaAddress)
				submitS(fallbackEthClient, round.String(), trialNum.String())
			}
		}
	}
}

func processSecretRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, index *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	revealOrder, err := database.GetRevealOrder(round, trialNum, uniqueKey)
	if err != nil {
		log.Printf("Failed to get reveal order for round %s with trail %s: %v", round, trialNum, err)
	}

	if index.Int64() >= int64(len(revealOrder.OrderedNodes)) {
		log.Printf("Index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(revealOrder.OrderedNodes))
		return
	}

	regularEoaAddress := revealOrder.OrderedNodes[index.Int64()]
	if EoaAddress == regularEoaAddress {
		fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, EOA: %v\n", round, regularEoaAddress)
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
	fmt.Printf("Round %v, TrialNum %v\n", Round, TrialNum)
	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}
	uniqueKey := utils.GetUniqueKey(Round.String(), TrialNum.String())
	roundData := RoundsData[uniqueKey]
	roundData.MerkleRoot = true
	RoundsData[uniqueKey] = roundData
}

func processRandomRequestNumber(fallbackEthClient *fallback_ethclient.FallbackRPCClient, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int, state *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, state %v\n", round, trialNum, state)
	// Reset monitoring state for new round
	roundStr := round.String()
	ResetMonitoringState(round.String(), trialNum.String())

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
		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			blockTimestamp, state, round)
		ActivatedOperator, _ = FetchActivatedOperators(fallbackEthClient, roundStr)
		Req = req
		Execution = true
		log.Printf("Execution started for round %s", roundStr)

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
		if !CosRecevied[uniqueKey][op.Hex()] {
			return false
		}
	}
	return true
}

func processCommitRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round *big.Int, trialNum *big.Int, packedIndices *big.Int) error {
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
	if leaderMonitoringActive {
		log.Printf("Leader monitoring already active for round %s", round)
		return
	}

	if startTime == nil {
		log.Printf("StartTime is nil, cannot start monitoring")
		return
	}

	leaderMonitoringActive = true

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
		leaderMonitoringActive = false
	})
}

// StopLeaderMonitoring stops the current leader monitoring
func StopLeaderMonitoring(round string, trialNum string) {
	if !leaderMonitoringActive {
		return
	}

	if monitoringTimer != nil {
		monitoringTimer.Stop()
		monitoringTimer = nil
	}

	leaderMonitoringActive = false
	log.Printf("Stopped leader monitoring for round %s", round)
}

// ResetMonitoringState resets all monitoring variables
func ResetMonitoringState(round string, trialNum string) {
	StopLeaderMonitoring(round, trialNum)
	cvRequestedEventEmitted = false
	merkleRootSubmittedEventEmitted = false

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
		log.Printf("Merkle root submitted event already emitted for round %s", round)
		return
	}

	// Check if we have a valid start time
	if StartTime == nil {
		log.Printf("StartTime is nil, cannot start monitoring")
		return
	}

	StartLeaderMonitoring(fallbackEthClient, StartTime, round, trialNum)
}

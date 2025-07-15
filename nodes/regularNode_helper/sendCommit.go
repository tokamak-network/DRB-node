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

var RequestQueue []RandomRequest
var Round *big.Int
var CurrentRound string

// Add new variables for monitoring
var (
	leaderMonitoringActive bool
	monitoringTimer        *time.Timer
	// Event tracking variables
	cvRequestedEventEmitted         bool
	merkleRootSubmittedEventEmitted bool
	// Current round and trialNum from Status event
	CurrentRoundNum *big.Int
	CurrentTrialNum *big.Int
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
					StopLeaderMonitoring()

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
						eventData.Round, eventData.TrialNum, eventData.MerkleRoot, CurrentRound)

					// Mark Merkle root as submitted and stop leader monitoring
					merkleRootSubmittedEventEmitted = true
					StopLeaderMonitoring()

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
					// index := new(big.Int).Add(eventData.Index, big.NewInt(1))
					processSubmittedSecretRequest(fallbackEthClient, eventData.Round, eventData.TrialNum, eventData.Index)
				}
			}
		}
	}
}

func processSubmittedSecretRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, Round *big.Int, TrialNum *big.Int, index *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, index %v\n", Round, TrialNum, index)
	data, _ := commitreveal2.LoadRevealOrder("regular_reveal_order.json", CurrentRound)
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
				fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, EOA: %v\n", CurrentRound, regularEoaAddress)
				submitS(fallbackEthClient)
			}
		}
	}
}

func processSecretRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, Round *big.Int, TrialNum *big.Int, index *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, index %v\n", Round, TrialNum, index)
	data, _ := commitreveal2.LoadRevealOrders("regular_reveal_order.json")
	revealOrder, exists := data[CurrentRound]
	if !exists {
		log.Printf("No reveal order found for round %s", CurrentRound)
		return
	}

	if index.Int64() >= int64(len(revealOrder.OrderedNodes)) {
		log.Printf("Index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(revealOrder.OrderedNodes))
		return
	}

	regularEoaAddress := revealOrder.OrderedNodes[index.Int64()]
	if EoaAddress == regularEoaAddress {
		fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, EOA: %v\n", CurrentRound, regularEoaAddress)
		submitS(fallbackEthClient)
	}

}

func submitS(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	commits, err := utils.LoadRegularCommits()
	if err != nil {
		log.Fatalf("Failed to load commits: %v", err)
	}

	roundData, exists := commits[CurrentRound]
	if !exists {
		log.Fatalf("Round %s not found in commits.json", CurrentRound)
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
	roundData := RoundsData[Round.String()]
	roundData.MerkleRoot = true
	RoundsData[Round.String()] = roundData
}

func processRandomRequestNumber(fallbackEthClient *fallback_ethclient.FallbackRPCClient, blockTimestamp *big.Int, Round *big.Int, TrialNum *big.Int, state *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, state %v\n", Round, TrialNum, state)
	// Reset monitoring state for new round
	ResetMonitoringState()

	// Store the current round and trialNum from Status event
	CurrentRoundNum = Round
	CurrentTrialNum = TrialNum

	eth.UpdateActivatedOperators(fallbackEthClient)
	round, _ := fetchCurrentRound(fallbackEthClient)
	CurrentRound = round.String()

	req := RandomRequest{
		Round:     round,
		StartTime: blockTimestamp,
		State:     state,
	}

	if state.Cmp(big.NewInt(1)) == 0 {
		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			blockTimestamp, state, round)
		ActivatedOperator, _ = FetchActivatedOperators(fallbackEthClient, CurrentRound)
		Req = req
		Execution = true
		log.Printf("Execution started for round %s", CurrentRound)

		// Start leader monitoring for the new round using block timestamp
		StartLeaderMonitoring(fallbackEthClient, blockTimestamp)
	}
	if state.Cmp(big.NewInt(2)) == 0 {
		roundData := RoundsData[round.String()]
		roundData.RandomNumber = true
		RoundsData[round.String()] = roundData
		Execution = false
		log.Printf("Execution stopped for round %s", CurrentRound)
	}

	// Update rounds data
	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}

	RoundsData[CurrentRound] = RoundData{
		MerkleRoot:   false,
		RandomNumber: false,
	}

	go AllCosReceivedUnlocked(ActivatedOperator)
}

func AllCosReceivedUnlocked(ActivatedOperator []string) {
	for {
		round := CurrentRound
		ops := eth.ActivatedOperators
		if allCosReceivedUnlockedRegular(round, ops) {
			flag, _ := commitreveal2.DetermineRevealOrderForRegular(CurrentRound, ops, "regular_reveal_order.json")
			if flag {
				break
			}
			time.Sleep(5 * time.Second)

		}
		time.Sleep(2 * time.Second)
	}
}

func allCosReceivedUnlockedRegular(roundNum string, ops []common.Address) bool {
	for _, op := range ops {
		if !CosRecevied[roundNum][op.Hex()] {
			return false
		}
	}
	return true
}

func processCommitRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, Round *big.Int, TrialNum *big.Int, packedIndices *big.Int) error {
	fmt.Printf("Round %v, TrialNum %v, packedIndices %v\n", Round, TrialNum, packedIndices)
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

	fmt.Printf("Processing RequestedToSubmitCv event for Round: %v\n", CurrentRound)

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

	commits, err := utils.LoadRegularCommits()
	if err != nil {
		fmt.Println("Error loading commits:", err)
		return err
	}

	var cvs []uint8
	for key, data := range commits {
		if CurrentRound == key {
			cvs = data.Cvs[:]
			break
		}
	}
	cvsSlice := cvs[:]
	var cv [32]byte
	copy(cv[:], []byte(cvsSlice))

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"submitCv",
		big.NewInt(0),
		cv,
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

	fmt.Printf("Processing RequestedToSubmitCo event for Round: %v\n", CurrentRound)

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

	commits, err := utils.LoadRegularCommits()
	if err != nil {
		fmt.Println("Error loading commits:", err)
		return err
	}

	var cos []uint8
	for key, data := range commits {
		if CurrentRound == key {
			cos = data.Cos[:]
			break
		}
	}
	cvsSlice := cos[:]
	var cv [32]byte
	copy(cv[:], []byte(cvsSlice))

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"submitCo",
		big.NewInt(0),
		cv,
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

func fetchCurrentRound(fallbackEthClient *fallback_ethclient.FallbackRPCClient) (*big.Int, error) {
	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load contract ABI: %v", err)
	}

	result, err := eth.CallSmartContract(fallbackEthClient, parsedABI, "s_currentRound", contractAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to call s_currentRound: %v", err)
	}

	currentRound := result.(*big.Int)
	return currentRound, nil
}

// StartLeaderMonitoring starts monitoring the leader for the current round
func StartLeaderMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, startTime *big.Int) {
	if leaderMonitoringActive {
		log.Printf("Leader monitoring already active for round %s", CurrentRound)
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
		log.Printf("Deadline has already passed for round %s, calling failToRequestSubmitCVOrSubmitMerkleRoot immediately", CurrentRound)
		callFailToRequestSubmitCVOrSubmitMerkleRoot(fallbackEthClient)
		return
	}

	log.Printf("Starting leader monitoring for round %s, deadline: %v (in %v)", CurrentRound, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	monitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToRequestSubmitCVOrSubmitMerkleRoot", CurrentRound)
		callFailToRequestSubmitCVOrSubmitMerkleRoot(fallbackEthClient)
		leaderMonitoringActive = false
	})
}

// StopLeaderMonitoring stops the current leader monitoring
func StopLeaderMonitoring() {
	if !leaderMonitoringActive {
		return
	}

	if monitoringTimer != nil {
		monitoringTimer.Stop()
		monitoringTimer = nil
	}

	leaderMonitoringActive = false
	log.Printf("Stopped leader monitoring for round %s", CurrentRound)
}

// ResetMonitoringState resets all monitoring variables
func ResetMonitoringState() {
	StopLeaderMonitoring()
	cvRequestedEventEmitted = false
	merkleRootSubmittedEventEmitted = false
	// Clear current round and trialNum
	CurrentRoundNum = nil
	CurrentTrialNum = nil
	log.Printf("Reset monitoring state")
}

// callFailToRequestSubmitCVOrSubmitMerkleRoot calls the contract function to fail the leader
func callFailToRequestSubmitCVOrSubmitMerkleRoot(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
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

	log.Printf("Successfully called failToRequestSubmitCVOrSubmitMerkleRoot for round %s", CurrentRound)
}

// CheckAndStartMonitoring checks if monitoring should be started and starts it if needed
func CheckAndStartMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	// Check if monitoring is already active
	if leaderMonitoringActive {
		log.Printf("Leader monitoring already active for round %s", CurrentRound)
		return
	}

	// Check if CV has been requested
	if cvRequestedEventEmitted {
		log.Printf("CV requested event already emitted for round %s", CurrentRound)
		return
	}

	// Check if Merkle root has been submitted
	if merkleRootSubmittedEventEmitted {
		log.Printf("Merkle root submitted event already emitted for round %s", CurrentRound)
		return
	}

	// Check if we have a valid start time
	if StartTime == nil {
		log.Printf("StartTime is nil, cannot start monitoring")
		return
	}

	StartLeaderMonitoring(fallbackEthClient, StartTime)
}

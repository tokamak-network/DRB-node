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

var RequestQueue []RandomRequest
var Round *big.Int
var CurrentRound string

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
				switch vLog.Topics[0] {
				case SubmitCVS:
					eventData := struct {
						StartTime     *big.Int
						PackedIndices *big.Int
					}{}
					err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitCv", vLog.Data)
					if err != nil {
						log.Printf("Failed to decode event log: %v", err)
						continue
					}

					fmt.Printf("CommitRequest Event: startTime %v\n, indices %v\n", eventData.StartTime, eventData.PackedIndices)

					processCommitRequest(fallbackEthClient, eventData.PackedIndices)

				case StatusSig:
					eventData := struct {
						CurStartTime *big.Int
						CurState     *big.Int
					}{}

					err := parsedABI.UnpackIntoInterface(&eventData, "Status", vLog.Data)
					if err != nil {
						log.Printf("Failed to decode Status event log: %v", err)
						continue
					}

					processRandomRequestNumber(fallbackEthClient, eventData.CurStartTime, eventData.CurState)

				case MerkleRootSubmittedSig:
					eventData := struct {
						StartTime  *big.Int
						MerkleRoot [32]byte
					}{}

					err := parsedABI.UnpackIntoInterface(&eventData, "MerkleRootSubmitted", vLog.Data)
					if err != nil {
						log.Printf("Failed to decode MerkleRootSubmitted event log: %v", err)
						continue
					}
					fmt.Printf("MerkleRootSubmitted Event:\n StartTime: %v\n MerkleRoot: %v\n Round: %v\n",
						eventData.StartTime, eventData.MerkleRoot, CurrentRound)

					processMerkleRoot(CurrentRound)

				case RequestedToSubmitCoSig:
					eventData := struct {
						StartTime     *big.Int
						IndicesLength *big.Int
						PackedIndices *big.Int
					}{}
					err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitCo", vLog.Data)
					if err != nil {
						log.Printf("Failed to decode RequestedToSubmitCo event log: %v", err)
						continue
					}
					fmt.Printf("RequestedToSubmitCo Event: startTime %v\n, indicesLength %v\n, indices %v\n", eventData.StartTime, eventData.IndicesLength, eventData.PackedIndices)

					processCosRequest(fallbackEthClient, eventData.PackedIndices, eventData.IndicesLength)

				case RequestedToSubmitSFromIndexKSig:
					eventData := struct {
						StartTime *big.Int
						IndexK    *big.Int
					}{}

					err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitSFromIndexK", vLog.Data)

					if err != nil {
						log.Printf("Failed to decode RequestedToSubmitSFromIndexK event log: %v", err)
						continue
					}
					fmt.Printf("RequestedToSubmitSFromIndexK Event:\n startTime %v\n indexK %v\n", eventData.StartTime, eventData.IndexK)

					processSecretRequest(fallbackEthClient, eventData.IndexK)

				case SSubmittedSig:
					eventData := struct {
						StartTime *big.Int
						S         [32]byte
						Index     *big.Int
					}{}

					err := parsedABI.UnpackIntoInterface(&eventData, "SSubmitted", vLog.Data)

					if err != nil {
						log.Printf("Failed to decode SSubmitted event log: %v", err)
						continue
					}
					fmt.Printf("SSubmitted Event:\n startTime %v\n Secret %v\n, indexK %v\n ", eventData.StartTime, eventData.S, eventData.Index)
					// index := new(big.Int).Add(eventData.Index, big.NewInt(1))
					processSubmittedSecretRequest(fallbackEthClient, eventData.Index)
				}
			}
		}
	}
}

func processSubmittedSecretRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, index *big.Int) {
	data, err := database.GetRevealOrder(CurrentRound)
	if err != nil {
		log.Printf("Failed to get reveal order for round %s: %v", CurrentRound, err)
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
				fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, EOA: %v\n", CurrentRound, regularEoaAddress)
				submitS(fallbackEthClient)
			}
		}
	}
}

func processSecretRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, index *big.Int) {
	revealOrder, err := database.GetRevealOrder(CurrentRound)
	if err != nil {
		log.Printf("Failed to get reveal order for round %s: %v", CurrentRound, err)
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
	roundData, err := database.GetCommitByRound(CurrentRound)
	if err != nil {
		log.Printf("Failed to get regular commit for round %s: %v", CurrentRound, err)
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

func processMerkleRoot(round string) {
	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}
	roundData := RoundsData[round]
	roundData.MerkleRoot = true
	RoundsData[round] = roundData
}

func processRandomRequestNumber(fallbackEthClient *fallback_ethclient.FallbackRPCClient, startTime *big.Int, state *big.Int) {
	eth.UpdateActivatedOperators(fallbackEthClient)
	round, _ := fetchCurrentRound(fallbackEthClient)
	CurrentRound = round.String()
	req := RandomRequest{
		Round:     round,
		StartTime: startTime,
		State:     state,
	}
	if state.Cmp(big.NewInt(1)) == 0 {
		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			startTime, state, round)
		ActivatedOperator, _ = FetchActivatedOperators(fallbackEthClient, CurrentRound)
		Req = req

		Execution = true
	}
	if state.Cmp(big.NewInt(2)) == 0 {
		roundData := RoundsData[round.String()]
		roundData.RandomNumber = true
		RoundsData[round.String()] = roundData

		Execution = false
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

func processCommitRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, packedIndices *big.Int) error {
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

	commitData, err := database.GetCommitByRound(CurrentRound)
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

func processCosRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, packedIndices *big.Int, indicesLength *big.Int) error {
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

	commitData, err := database.GetCommitByRound(CurrentRound)
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

func fetchCurrentRound(fallbackEthClient *fallback_ethclient.FallbackRPCClient) (*big.Int, error) {
	abiFilePath := "contract/abi/Commit2RevealDRB.json"
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		log.Fatalf("Failed to load contract ABI: %v", err)
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}

	contractAddress := common.HexToAddress(contractAddressStr)

	result, err := eth.CallSmartContract(fallbackEthClient, parsedABI, "s_currentRound", contractAddress)
	if err != nil {
		log.Printf("Failed to fetch activated operators: %v", err)
		return nil, err
	}
	currentRound, ok := result.(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unexpected type: expected *big.Int, got %v", result)
	}

	return currentRound, nil
}

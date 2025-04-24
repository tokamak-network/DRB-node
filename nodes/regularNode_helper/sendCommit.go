package regularNode_helper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tokamak-network/DRB-node/eth"
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
var commits map[string]CommitData

func MonitorCommitRequest() {
	receiveCommitRequest()
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

func receiveCommitRequest() {
	rpcURL := os.Getenv("ETH_RPC_URL")
	contractAddress := os.Getenv("CONTRACT_ADDRESS")
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum node: %v", err)
	}
	contractAddr := common.HexToAddress(contractAddress)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Fatalf("Failed to parse contract ABI: %v", err)
	}
	query := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddr},
	}
	logs := make(chan types.Log)
	sub, err := client.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		log.Fatalf("Failed to subscribe to logs: %v", err)
	}
	SubmitCVS := parsedABI.Events["RequestedToSubmitCv"].ID
	StatusSig := parsedABI.Events["Status"].ID
	MerkleRootSubmittedSig := parsedABI.Events["MerkleRootSubmitted"].ID
	RandomNumberGeneratedSig := parsedABI.Events["RandomNumberGenerated"].ID

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("Error in event subscription: %v", err)

		case vLog := <-logs:
			{
				switch vLog.Topics[0] {
				case SubmitCVS:
					eventData := struct {
						StartTime *big.Int
						Indices   []*big.Int
					}{}
					err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitCv", vLog.Data)
					if err != nil {
						log.Printf("Failed to decode event log: %v", err)
						continue
					}

					fmt.Printf("CommitRequest Event: startTime %v\n, indices %v\n", eventData.StartTime, eventData.Indices)

					processCommitRequest(eventData.Indices)
				
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

					fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
						eventData.CurStartTime, eventData.CurState, CurrentRound)
	
					processRandomRequestNumber(eventData.CurStartTime, eventData.CurState)

				case MerkleRootSubmittedSig: 
					eventData := struct {
						StartTime *big.Int
						MerkleRoot     [32]byte
					}{}
					
					err := parsedABI.UnpackIntoInterface(&eventData, "MerkleRootSubmitted", vLog.Data)
					if err != nil {
						log.Printf("Failed to decode MerkleRootSubmitted event log: %v", err)
						continue
					}
					fmt.Printf("MerkleRootSubmitted Event:\n StartTime: %v\n MerkleRoot: %v\n Round: %v\n",
						eventData.StartTime, eventData.MerkleRoot, CurrentRound)

					processMerkleRoot(CurrentRound)
				
				case RandomNumberGeneratedSig: 
					eventData := struct {
						Round *big.Int
						RandomNumber *big.Int
						CallbackSuccess     bool
					}{}
					
					err := parsedABI.UnpackIntoInterface(&eventData, "RandomNumberGenerated", vLog.Data)
					if err != nil {
						log.Printf("Failed to decode MerkleRootSubmitted event log: %v", err)
						continue
					}
					fmt.Printf("MerkleRootSubmitted Event:\n StartTime: %v\n MerkleRoot: %v\n Round: %v\n",
						eventData.Round, eventData.RandomNumber, eventData.CallbackSuccess)

					// processGeneratedRandomNumber(eventData.Round, eventData.RandomNumber)
				}
			}
		}
	}
}

// func processGeneratedRandomNumber(round *big.Int, randomNumber *big.Int) {
// 	data := RoundsData[round.String()]
// 	data.RandomNumber = true
// 	RoundsData[round.String()] = data

// 	StartNextRound = true
// 	RequestQueue = RequestQueue[1:]

// 	fmt.Printf("Generated random number %v, for round: %v\n",randomNumber, round )
// }

func processMerkleRoot(round string) {
	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}
	roundData := RoundsData[round]
	roundData.MerkleRoot = true
	RoundsData[round] = roundData
}

func processRandomRequestNumber(startTime *big.Int, state *big.Int) {
	round, _ := fetchCurrentRound();
	CurrentRound = round.String()
	req := RandomRequest{
		Round:     round,
		StartTime: startTime,
		State:     state,
	}
	if state.Cmp(big.NewInt(1)) == 0 {
		fmt.Printf("Round Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			startTime, state, round)
		
		Req = req
		Execution = true
	}
	if state.Cmp(big.NewInt(2)) == 0 {
		roundData := RoundsData[round.String()]
		roundData.RandomNumber = true
		RoundsData[round.String()] = roundData

		Execution = false
	}		
}

func processCommitRequest(indices []*big.Int) error {
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode Ethereum private key: %v", err)
	}
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	// temporary variable round for now. In the future there will be round global variable which would be accessible by every file to keep track of current round.
	// currently there is no mechanism to do it.
	round := "0"
	acitvatedOps, _ := FetchActivatedOperators(round)

	flag, err := findEOAAddress(indices, acitvatedOps, eoaAddress)

	if err != nil {
		fmt.Println(err)
	}
	if !flag {
		fmt.Println("Commit Request does not contain our EOA")
		return nil
	}

	fmt.Printf("Processing commit for Round: %v\n", round)

	ethRPCURL := os.Getenv("ETH_RPC_URL")
	if ethRPCURL == "" {
		log.Fatal("ETH_RPC_URL is not set in the environment variables")
	}
	client, err := ethclient.Dial(ethRPCURL)
	if err != nil {
		return fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}
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
		Client:          client,
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}
	
	file, err := os.ReadFile("commits.json")
	if err != nil {
		fmt.Println("Error reading file:", err)
		return err
	}

	err = json.Unmarshal(file, &commits)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return err
	}
	var cvs []uint8
	for key, data := range commits {
		if round == key {
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
		"submitCv",
		big.NewInt(0),
		cv,
	)
	if err != nil {
		fmt.Println("It contains error")
	}
	return nil
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

func FetchActivatedOperators(round string) ([]string, error) {
	var result []string
	activatedOperators, err := eth.GetActivatedOperators()
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

func fetchCurrentRound() (*big.Int, error){
	ethRPCURL := os.Getenv("ETH_RPC_URL")
	client, err := ethclient.Dial(ethRPCURL)
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum RPC: %v", err)
	}

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

	result, err := eth.CallSmartContract(client, parsedABI, "s_currentRound", contractAddress)
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

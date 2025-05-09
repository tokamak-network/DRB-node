package leaderNode_helper

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

var CommitMu sync.Mutex
var StartTime *big.Int
var Execution bool
var ActivatedOperator []string

type RandomRequest struct {
	Round     *big.Int
	StartTime *big.Int
	State     *big.Int
}
type RoundData struct {
	MerkleRoot   bool
	RandomNumber bool
}

var RoundsData map[string]RoundData

var Round *big.Int
var CurrentRound string
var Req RandomRequest

type LeaderCommitData struct {
	Round                 string            `json:"round"`
	EOAAddress            string            `json:"eoa_address"`
	Cvs                   [32]byte          `json:"cvs"`
	CvsHex                string            `json:"cvs_hex,omitempty"`
	Cos                   [32]byte          `json:"cos"`
	CosHex                string            `json:"cos_hex"`
	SecretValue           [32]byte          `json:"secret_value"`
	SecretValueHex        string            `json:"secret_value_hex"`
	Sign                  map[string]string `json:"sign"`
	SubmitMerkleRootDone  bool              `json:"submit_merkle_root_done"`
	RandomNumberGenerated bool              `json:"random_number_generated"`
}

func ReceiveCommit() {
	receiveCommit()
}

func receiveCommit() {
	rpcURL := os.Getenv("ETH_RPC_URL")
	client, err := ethclient.Dial(rpcURL)
	contractAddress := os.Getenv("CONTRACT_ADDRESS")
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum node 2323: %v", err)
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

	cvsEventSig := parsedABI.Events["CvSubmitted"].ID
	StatusSig := parsedABI.Events["Status"].ID

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("Error in event subscription: %v", err)

		case vLog := <-logs:
			switch vLog.Topics[0] {
			case cvsEventSig:
				eventData := struct {
					StartTime *big.Int
					Cv        [32]byte
					Index     *big.Int
				}{}
				err := parsedABI.UnpackIntoInterface(&eventData, "CvSubmitted", vLog.Data)
				if err != nil {
					log.Printf("Failed to decode CvSubmitted event log: %v", err)
					continue
				}
				fmt.Printf("CvSubmitted Event: Fetched successfully")

				processCVS(eventData.Cv, eventData.Index)

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
				processRandomRequestNumber(eventData.CurStartTime, eventData.CurState)
			}
		}
	}
}

func processRandomRequestNumber(startTime *big.Int, state *big.Int) {
	round, _ := fetchCurrentRound()
	CurrentRound = round.String()
	req := RandomRequest{
		Round:     round,
		StartTime: startTime,
		State:     state,
	}
	if state.Cmp(big.NewInt(1)) == 0 {
		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			startTime, state, round)
		Req = req
		ActivatedOperator, _ = FetchActivatedOperators(CurrentRound)
		eth.UpdateActivatedOperators()
		Execution = true
	}
	if state.Cmp(big.NewInt(2)) == 0 {
		if RoundsData == nil {
			RoundsData = make(map[string]RoundData)
		}
		data := RoundsData[Round.String()]
		data.RandomNumber = true
		RoundsData[Round.String()] = data
		Execution = false
	}
}

func processCVS(cvs [32]byte, activatedOperatorIndex *big.Int) error {
	filePath := "leader_commits.json"
	CommitMu.Lock()
	defer CommitMu.Unlock()

	var commitData map[string]LeaderCommitData
	file, err := os.ReadFile(filePath)
	if err == nil {
		err = json.Unmarshal(file, &commitData)
		if err != nil {
			fmt.Println("Error parsing JSON:", err)
			return err
		}
	} else {
		commitData = make(map[string]LeaderCommitData)
	}

	round := CurrentRound
	eoaAddress := ActivatedOperator[activatedOperatorIndex.Int64()]
	eoa := common.HexToAddress(eoaAddress)
	key := fmt.Sprintf("%s+%s", round, eoa.Hex())
	cvsHex := hex.EncodeToString(cvs[:])
	cvsBytes, _ := hex.DecodeString(cvsHex)

	copy(cvs[:], cvsBytes)
	if _, exists := commitData[key]; !exists {
		commitData[key] = LeaderCommitData{
			Round:                 round,
			EOAAddress:            eoa.Hex(),
			Cvs:                   cvs,
			CvsHex:                cvsHex,
			Cos:                   [32]byte{},
			CosHex:                "",
			SecretValue:           [32]byte{},
			SecretValueHex:        "",
			Sign:                  make(map[string]string),
			SubmitMerkleRootDone:  false,
			RandomNumberGenerated: false,
		}
	} else {
		data := commitData[key]
		data.Cvs = cvs
		data.CvsHex = cvsHex
		commitData[key] = data
	}

	updatedJSON, err := json.MarshalIndent(commitData, "", "  ")
	if err != nil {
		fmt.Println("Error serializing updated JSON:", err)
		return err
	}

	err = os.WriteFile(filePath, updatedJSON, 0644)
	if err != nil {
		fmt.Println("Error writing updated JSON file:", err)
		return err
	}
	updateCVS(round, eoa, cvs)
	fmt.Printf("Successfully stored CVS for Round %s, EOA %s\n", round, eoa.Hex())
	return nil
}

func updateCVS(round string, eoa common.Address, cvs [32]byte) {
	if _, exists := utils.CommittedNodes[round]; !exists {
		utils.CommittedNodes[round] = make(map[common.Address]utils.LeaderCommitData)
	}

	commitData, exists := utils.CommittedNodes[round][eoa]
	if !exists {
		commitData = utils.LeaderCommitData{}
	}

	commitData.EOAAddress = eoa.Hex()
	commitData.Round = round
	commitData.Cvs = cvs
	cvsHex := hex.EncodeToString(cvs[:])
	commitData.CvsHex = cvsHex
	utils.CommittedNodes[round][eoa] = commitData
}

func fetchCurrentRound() (*big.Int, error) {
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

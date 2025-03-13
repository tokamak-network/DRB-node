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
	"github.com/tokamak-network/DRB-node/utils"
)


var CommitMu sync.Mutex

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

	cvsEventSig := parsedABI.Events["COV"].ID

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("Error in event subscription: %v", err)

		case vLog := <-logs:
			switch vLog.Topics[0] {
			case cvsEventSig:
				eventData := struct {
					Round *big.Int
					Op    common.Address
					Cov   [32]byte
				}{}
				err := parsedABI.UnpackIntoInterface(&eventData, "COV", vLog.Data)
				if err != nil {
					log.Printf("Failed to decode COV event log: %v", err)
					continue
				}
				fmt.Printf("COV Event:\n Round: %d\n Address: %s\n COV: %v\n",
					eventData.Round, eventData.Op.Hex(), eventData.Cov)

				processCVS(eventData.Round, eventData.Op, eventData.Cov)
			}
		}
	}
}


func processCVS(round *big.Int, eoa common.Address, cvs [32]byte) error {
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
	key := fmt.Sprintf("%s+%s", round.String(), eoa.Hex())
	cvsHex := hex.EncodeToString(cvs[:])
	cvsBytes, _ := hex.DecodeString(cvsHex)
	
	copy(cvs[:], cvsBytes)
	if _, exists := commitData[key]; !exists {
		commitData[key] = LeaderCommitData{
			Round:                 round.String(),
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
	fmt.Printf("Successfully stored CVS for Round %s, EOA %s\n", round.String(), eoa.Hex())
	return nil
}


func updateCVS(round *big.Int, eoa common.Address, cvs [32]byte) {

	roundKey := round.String()
	if _, exists := utils.CommittedNodes[roundKey]; !exists {
		utils.CommittedNodes[roundKey] = make(map[common.Address]utils.LeaderCommitData)
	}

	commitData, exists := utils.CommittedNodes[roundKey][eoa]
	if !exists {
		commitData = utils.LeaderCommitData{}
	}

	commitData.EOAAddress = eoa.Hex()
	commitData.Round = roundKey
	commitData.Cvs = cvs
	cvsHex := hex.EncodeToString(cvs[:])
	commitData.CvsHex = cvsHex
	utils.CommittedNodes[roundKey][eoa] = commitData
}

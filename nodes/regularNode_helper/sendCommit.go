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

var commits map[string]CommitData

func MonitorCommitRequest() {
	receiveCommitRequest()
}

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
	SubmitCVS := parsedABI.Events["SubmitCVS"].ID

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("Error in event subscription: %v", err)

		case vLog := <-logs:
			{
				switch vLog.Topics[0] {
				case SubmitCVS:
					eventData := struct {
						Round *big.Int
						Op    common.Address
					}{}
					err := parsedABI.UnpackIntoInterface(&eventData, "SubmitCVS", vLog.Data)
					if err != nil {
						log.Printf("Failed to decode event log: %v", err)
						continue
					}

					fmt.Printf("CommitRequest Event:\n Round: %d\n Address: %s\n", eventData.Round, eventData.Op.Hex())

					processCommitRequest(eventData.Round, eventData.Op)
				}
			}
		}
	}
}

func processCommitRequest(round *big.Int, eoa common.Address) error {
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode Ethereum private key: %v", err)
	}
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	if eoaAddress != eoa.String() {
		fmt.Println("Commit Request does not contain our EOA")
		return nil
	}

	fmt.Printf("Processing commit for Round: %d and Address: %s\n", round, eoa.Hex())

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
		roundInt := new(big.Int)
		roundInt, ok := roundInt.SetString(key, 10)
		if !ok {
			fmt.Errorf("Failed to convert the the string to big.Int")
		}
		if round.Cmp(roundInt) == 0 {
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
		"submitCommit",
		big.NewInt(0),
		common.HexToAddress(eoaAddress),
		round,
		cv,
	)
	if err != nil {
		fmt.Println("It contains error")
	}
	return nil
}

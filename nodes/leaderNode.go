package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/transactions"
	"github.com/tokamak-network/DRB-node/utils"
)

const abiFilePath = "contract/abi/Commit2RevealDRB.json"

// Leader Node
func RunLeaderNode() {
	port := os.Getenv("LEADER_PORT")
	if port == "" {
		log.Fatal("LEADER_PORT not set in environment variables.")
	}

	h, err := libp2p.New(
		libp2p.DefaultTransports,
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)),
	)
	if err != nil {
		log.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer h.Close()

	h.SetStreamHandler("/register", func(s network.Stream) {
		handleRegistrationRequest(s)
	})

	log.Printf("Leader node is running on addresses: %s\n", h.Addrs())
	log.Printf("Leader node PeerID: %s\n", h.ID().String())

	select {}
}

func handleRegistrationRequest(s network.Stream) {
	defer s.Close()

	var req utils.RegistrationRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode registration request: %v", err)
		return
	}

	if !utils.VerifySignature(req) {
		log.Printf("Failed to verify signature for PeerID: %s", req.PeerID)
		return
	}

	log.Printf("Verified registration for PeerID: %s", req.PeerID)

	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum client: %v", err)
	}

	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		log.Fatalf("Failed to load contract ABI: %v", err)
	}

	operatorAddress := common.HexToAddress(req.EOAAddress)

	activatedOperatorsResult, err := transactions.CallSmartContract(client, parsedABI, "getActivatedOperators", contractAddress)
	if err != nil {
		log.Printf("Failed to call getActivatedOperators: %v", err)
		return
	}

	activatedOperators := activatedOperatorsResult.([]common.Address)
	isActivated := false
	for _, operator := range activatedOperators {
		if operator == operatorAddress {
			isActivated = true
			break
		}
	}

	if isActivated {
		log.Println("Operator is already activated.")
		return
	}

	depositAmountResult, err := transactions.CallSmartContract(client, parsedABI, "s_depositAmount", contractAddress, operatorAddress)
	if err != nil {
		log.Printf("Failed to call s_depositAmount: %v", err)
		return
	}
	depositAmount := depositAmountResult.(*big.Int)

	activationThresholdResult, err := transactions.CallSmartContract(client, parsedABI, "s_activationThreshold", contractAddress)
	if err != nil {
		log.Printf("Failed to call s_activationThreshold: %v", err)
		return
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	if depositAmount.Cmp(activationThreshold) < 0 {
		log.Printf("Deposit amount is insufficient. Deposit: %s, Threshold: %s", depositAmount, activationThreshold)
		return
	}

	// Prepare for transaction execution using ExecuteTransaction from transactions package
	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode leader private key: %v", err)
	}

	clientUtils := &utils.Client{
		Client:       client,
		ContractAddress: contractAddress,
		PrivateKey:   privateKey,
		ContractABI:  parsedABI,
	}

	_, _, err = transactions.ExecuteTransaction(
		context.Background(),
		clientUtils,
		"activate",
		big.NewInt(0),
		operatorAddress,
	)
	if err != nil {
		log.Printf("Failed to activate operator: %v", err)
		return
	}

	log.Println("Operator activated successfully.")
}
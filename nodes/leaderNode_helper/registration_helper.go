package leaderNode_helper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/transactions"
	"github.com/tokamak-network/DRB-node/utils"
)

// NodeInfo stores the information for a registered node
type NodeInfo struct {
	IP      string `json:"ip"`
	Port    string `json:"port"`
	PeerID  string `json:"peer_id"`
}

// LoadRegisteredNodes loads the registered nodes from a JSON file.
func LoadRegisteredNodes(filePath string) (map[string]NodeInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return an empty map if the file doesn't exist
			return make(map[string]NodeInfo), nil
		}
		return nil, fmt.Errorf("failed to open registered nodes file: %v", err)
	}
	defer file.Close()

	var data map[string]NodeInfo
	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode registered nodes file: %v", err)
	}

	return data, nil
}

// SaveRegisteredNodes saves the registered nodes to a JSON file.
func SaveRegisteredNodes(filePath string, data map[string]NodeInfo) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create registered nodes file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(data)
	if err != nil {
		return fmt.Errorf("failed to write registered nodes to file: %v", err)
	}

	return nil
}

// RegisterNode handles both saving node information and activating the node on-chain.
func RegisterNode(s network.Stream, filePath, abiFilePath string) error {
	var req utils.RegistrationRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		return fmt.Errorf("failed to decode registration request: %v", err)
	}

	if !utils.VerifySignature(req) {
		return fmt.Errorf("failed to verify signature for PeerID: %s", req.PeerID)
	}

	log.Printf("Verified registration for PeerID: %s", req.PeerID)

	// Get the remote IP and port
	remoteAddr := s.Conn().RemoteMultiaddr().String()
	parts := strings.Split(remoteAddr, "/")
	if len(parts) < 5 {
		return fmt.Errorf("invalid remote address format: %s", remoteAddr)
	}

	ip := parts[2]  // Extract IP
	port := parts[4] // Extract port

	// Load existing nodes
	nodes, err := LoadRegisteredNodes(filePath)
	if err != nil {
		return fmt.Errorf("failed to load registered nodes: %v", err)
	}

	// Update or add the node information
	nodes[req.EOAAddress] = NodeInfo{
		IP:     ip,
		Port:   port,
		PeerID: req.PeerID,
	}

	// Save updated nodes
	err = SaveRegisteredNodes(filePath, nodes)
	if err != nil {
		return fmt.Errorf("failed to save registered nodes: %v", err)
	}

	log.Printf("Successfully registered or updated EOA %s with NodeInfo: IP=%s, Port=%s, PeerID=%s.", req.EOAAddress, ip, port, req.PeerID)

	// Perform on-chain activation
	err = ActivateOnChain(req.EOAAddress, abiFilePath)
	if err != nil {
		return fmt.Errorf("failed to activate EOA %s on-chain: %v", req.EOAAddress, err)
	}

	log.Printf("Successfully activated EOA %s on-chain.", req.EOAAddress)
	return nil
}

// ActivateOnChain handles the on-chain activation of the node.
func ActivateOnChain(eoaAddress, abiFilePath string) error {
	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		return fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		return fmt.Errorf("failed to load contract ABI: %v", err)
	}

	operatorAddress := common.HexToAddress(eoaAddress)

	// Verify if the operator is activated
	activatedOperatorsResult, err := transactions.CallSmartContract(client, parsedABI, "getActivatedOperators", contractAddress)
	if err != nil {
		return fmt.Errorf("failed to call getActivatedOperators: %v", err)
	}

	activatedOperators := activatedOperatorsResult.([]common.Address)
	for _, operator := range activatedOperators {
		if operator == operatorAddress {
			log.Printf("Operator %s is already activated.", eoaAddress)
			return nil
		}
	}

	// Check deposit amount and activation threshold
	depositAmountResult, err := transactions.CallSmartContract(client, parsedABI, "s_depositAmount", contractAddress, operatorAddress)
	if err != nil {
		return fmt.Errorf("failed to call s_depositAmount: %v", err)
	}
	depositAmount := depositAmountResult.(*big.Int)

	activationThresholdResult, err := transactions.CallSmartContract(client, parsedABI, "s_activationThreshold", contractAddress)
	if err != nil {
		return fmt.Errorf("failed to call s_activationThreshold: %v", err)
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	if depositAmount.Cmp(activationThreshold) < 0 {
		return fmt.Errorf("deposit amount is insufficient. Deposit: %s, Threshold: %s", depositAmount, activationThreshold)
	}

	// Activate the operator
	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return fmt.Errorf("failed to decode leader private key: %v", err)
	}

	clientUtils := &utils.Client{
		Client:          client,
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	_, _, err = transactions.ExecuteTransaction(
		context.Background(),
		clientUtils,
		"activate",
		big.NewInt(0),
		operatorAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to activate operator: %v", err)
	}

	return nil
}

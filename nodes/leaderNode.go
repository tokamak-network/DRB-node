package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto" // Ethereum crypto
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto" // Libp2p crypto (aliased)
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/tokamak-network/DRB-node/transactions"
	"github.com/tokamak-network/DRB-node/utils"
)

// RunLeaderNode starts the leader node and listens for registration requests
func RunLeaderNode() {
	port := os.Getenv("LEADER_PORT")
	if port == "" {
		log.Fatal("LEADER_PORT not set in environment variables.")
	}

	// Check if PeerID already exists, if not create and save it
	privKey, peerID, err := utils.LoadPeerID() // Correctly handle the three return values (privKey, peerID, error)
	if err != nil {
		log.Println("PeerID not found, generating new one.")
		// Generate a new deterministic libp2p private key (for example, from a fixed seed)
		privKey, _, err = libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 0)
		if err != nil {
			log.Fatalf("Failed to generate libp2p private key: %v", err)
		}

		// Save the generated PeerID for future restarts
		err = utils.SavePeerID(privKey)
		if err != nil {
			log.Fatalf("Failed to save PeerID: %v", err)
		}

		// After saving the new private key, get the PeerID
		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			log.Fatalf("Failed to get PeerID from private key: %v", err)
		}
	}

	log.Printf("Loaded or generated PeerID: %s", peerID.String())

	// Create the libp2p host with the loaded or generated PeerID
	h, err := libp2p.New(libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)), libp2p.Identity(privKey))
	if err != nil {
		log.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer h.Close()

	h.SetStreamHandler("/register", func(s network.Stream) {
		handleRegistrationRequest(s)
	})

	log.Printf("Leader node is running on addresses: %s\n", h.Addrs())
	log.Printf("Leader node PeerID: %s\n", peerID.String())

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

	// Verify if the operator is activated
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

	// Continue with contract interaction for activation
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
		log.Printf("Failed to activate operator: %v", err)
		return
	}

	log.Println("Operator activated successfully.")
}

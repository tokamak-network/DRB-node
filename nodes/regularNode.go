package nodes

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/libp2p/go-libp2p"
	core "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
	"github.com/tokamak-network/DRB-node/transactions"
	"github.com/tokamak-network/DRB-node/utils"
)

func RunRegularNode() {
	ctx := context.Background()

	h, err := libp2p.New(libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", os.Getenv("PORT"))))
	if err != nil {
		log.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer h.Close()

	leaderIP := os.Getenv("LEADER_IP")
	leaderPort := os.Getenv("LEADER_PORT")
	leaderPeerID := os.Getenv("LEADER_PEER_ID")

	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode Ethereum private key: %v", err)
	}

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	log.Printf("EOA Address: %s", eoaAddress)

	leaderAddrString := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", leaderIP, leaderPort, leaderPeerID)
	log.Printf("Leader multiaddress: %s", leaderAddrString)

	leaderAddr, err := multiaddr.NewMultiaddr(leaderAddrString)
	if err != nil {
		log.Fatalf("Failed to parse leader multiaddress: %v", err)
	}

	leaderInfo, err := peer.AddrInfoFromP2pAddr(leaderAddr)
	if err != nil {
		log.Fatalf("Failed to create peer info from leader multiaddress: %v", err)
	}

	h.Peerstore().AddAddrs(leaderInfo.ID, leaderInfo.Addrs, peerstore.PermanentAddrTTL)

	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum client: %v", err)
	}

	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		log.Fatalf("Failed to load contract ABI: %v", err)
	}

	clientUtils := &utils.Client{
		Client:       client,
		ContractAddress: contractAddress,
		PrivateKey:   privateKey,
		ContractABI:  parsedABI,
	}

	for {
		// Check activation status
		isActivated := checkActivationStatus(clientUtils, eoaAddress)
		if isActivated {
			log.Println("Node is activated. No further action required.")
			time.Sleep(30 * time.Second)
			continue
		}

		log.Println("Node is not activated. Checking deposit amount...")

		// Check and ensure deposit is sufficient
		depositSufficient, err := checkDepositAmount(clientUtils, eoaAddress)
		if err != nil {
			log.Printf("Error checking deposit amount: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		if !depositSufficient {
			log.Println("Deposit insufficient. Initiating deposit transaction...")
			txSent, err := depositAndCheckActivation(ctx, eoaAddress, privateKey)
			if err != nil {
				log.Printf("Error during deposit transaction: %v", err)
				time.Sleep(30 * time.Second)
				continue
			}
			if txSent {
				log.Println("Deposit transaction sent. Waiting for confirmation...")
				time.Sleep(30 * time.Second)
				continue
			}
		}

		// Send registration request to leader
		log.Println("Deposit sufficient. Sending registration request to leader...")
		sendRegistrationRequestToLeader(ctx, h, leaderInfo.ID, eoaAddress, privateKey)

		// Wait before rechecking activation status
		time.Sleep(30 * time.Second)
	}
}

func checkActivationStatus(client *utils.Client, eoaAddress string) bool {
	activatedOperatorsResult, err := transactions.CallSmartContract(client.Client, client.ContractABI, "getActivatedOperators", client.ContractAddress)
	if err != nil {
		log.Printf("Failed to call getActivatedOperators: %v", err)
		return false
	}

	activatedOperators := activatedOperatorsResult.([]common.Address)
	for _, operator := range activatedOperators {
		if operator.Hex() == eoaAddress {
			return true
		}
	}

	return false
}

func sendRegistrationRequestToLeader(ctx context.Context, h core.Host, leaderID peer.ID, eoaAddress string, privateKey *ecdsa.PrivateKey) {
	req := utils.RegistrationRequest{
		EOAAddress: eoaAddress,
		Signature:  utils.SignData(eoaAddress, privateKey),
		PeerID:     h.ID().String(),
	}

	s, err := h.NewStream(ctx, leaderID, "/register")
	if err != nil {
		log.Printf("Failed to create stream to leader: %v", err)
		h.Peerstore().AddAddrs(leaderID, h.Peerstore().Addrs(leaderID), peerstore.PermanentAddrTTL)
		return
	}
	defer s.Close()

	if err := json.NewEncoder(s).Encode(req); err != nil {
		log.Printf("Failed to send registration request: %v", err)
	} else {
		log.Println("Registration request sent to leader.")
	}
}

func depositAndCheckActivation(ctx context.Context, eoaAddress string, privateKey *ecdsa.PrivateKey) (bool, error) {
	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		return false, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to load contract ABI: %v", err)
	}

	// Fetch deposit amount
	depositAmountResult, err := transactions.CallSmartContract(client, parsedABI, "s_depositAmount", contractAddress, common.HexToAddress(eoaAddress))
	if err != nil {
		return false, fmt.Errorf("failed to call s_depositAmount: %v", err)
	}
	depositAmount := depositAmountResult.(*big.Int)

	// Fetch activation threshold
	activationThresholdResult, err := transactions.CallSmartContract(client, parsedABI, "s_activationThreshold", contractAddress)
	if err != nil {
		return false, fmt.Errorf("failed to call s_activationThreshold: %v", err)
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	// If deposit is insufficient, we calculate the remaining amount and proceed with the deposit
	if depositAmount.Cmp(activationThreshold) < 0 {
		remaining := new(big.Int).Sub(activationThreshold, depositAmount)
		log.Printf("Deposit insufficient. Adding remaining: %s", remaining.String())

		// Check account balance
		balance, err := client.BalanceAt(ctx, common.HexToAddress(eoaAddress), nil)
		if err != nil {
			return false, fmt.Errorf("failed to fetch account balance: %v", err)
		}
		log.Printf("Account balance: %s", balance.String())

		if balance.Cmp(remaining) < 0 {
			return false, fmt.Errorf("insufficient balance: required %s, available %s", remaining.String(), balance.String())
		}

		// Create and send deposit transaction
		_, _, err = transactions.ExecuteTransaction(
			ctx,
			&utils.Client{
				Client:       client,
				ContractAddress: contractAddress,
				PrivateKey:   privateKey,
				ContractABI:  parsedABI,
			},
			"deposit",
			remaining,
		)
		if err != nil {
			return false, fmt.Errorf("failed to send deposit transaction: %v", err)
		}

		log.Println("Deposit transaction sent.")
		return true, nil
	}

	log.Println("Deposit amount is sufficient. No additional deposit required.")
	return false, nil
}

func checkDepositAmount(client *utils.Client, eoaAddress string) (bool, error) {
	// Fetch deposit amount
	depositAmountResult, err := transactions.CallSmartContract(client.Client, client.ContractABI, "s_depositAmount", client.ContractAddress, common.HexToAddress(eoaAddress))
	if err != nil {
		return false, fmt.Errorf("failed to call s_depositAmount: %v", err)
	}
	depositAmount := depositAmountResult.(*big.Int)

	// Fetch activation threshold
	activationThresholdResult, err := transactions.CallSmartContract(client.Client, client.ContractABI, "s_activationThreshold", client.ContractAddress)
	if err != nil {
		return false, fmt.Errorf("failed to call s_activationThreshold: %v", err)
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	log.Printf("Deposit amount: %s, Activation threshold: %s", depositAmount.String(), activationThreshold.String())

	// Check if deposit is sufficient
	if depositAmount.Cmp(activationThreshold) >= 0 {
		return true, nil
	}

	return false, nil
}
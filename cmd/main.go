package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tokamak-network/DRB-node/nodes/leader"
	"github.com/tokamak-network/DRB-node/nodes/regular"

	"github.com/joho/godotenv"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found")
	}

	logger.InitLogger()
	defer logger.CloseLogger()

	nodeType := os.Getenv("NODE_TYPE") // Expecting 'leader' or 'regular'

	// Initialize the fallback ethclient
	rpcUrls := strings.Split(os.Getenv("ETH_RPC_URLS"), ",")
	fallbackEthClient, err := fallback_ethclient.NewFallbackRPCClient(rpcUrls)
	if err != nil {
		logger.Fatal("Failed to init the fallback ethclient", "err", err)
	}

	switch nodeType {
	case "leader":
		startLeaderNode(context.Background(), fallbackEthClient)
	case "regular":
		startRegularNode(context.Background(), fallbackEthClient)
	default:
		logger.Fatal("NODE_TYPE must be set to either 'leader' or 'regular'")
	}
}

func startLeaderNode(ctx context.Context, fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	port, err := strconv.ParseUint(os.Getenv("LEADER_PORT"), 10, 64)
	if err != nil {
		logger.Fatalf("Failed to parse LEADER_PORT: %v", err)
	}

	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
	if privateKeyHex == "" {
		logger.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		logger.Fatalf("Failed to decode leader private key: %v", err)
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		logger.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	leaderNode, err := leader.NewNode(&leader.Config{
		Port:             port,
		ContractAddress:  contractAddress,
		LeaderPrivateKey: privateKey,
	}, fallbackEthClient)
	if err != nil {
		// Close the leader node before exiting
		leaderNode.Close()
		logger.Fatalf("Failed to init the leader node: %v", err)
	}
	leaderNode.Start(ctx)
}

func startRegularNode(ctx context.Context, fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	port := os.Getenv("PORT")
	if port == "" {
		logger.Fatal("PORT not set in environment variables.")
	}
	leaderIP := os.Getenv("LEADER_IP")
	if leaderIP == "" {
		logger.Fatal("LEADER_IP is not set in environment variables.")
	}

	leaderPort := os.Getenv("LEADER_PORT")
	if leaderPort == "" {
		logger.Fatal("LEADER_PORT is not set in environment variables.")
	}

	leaderPeerID := os.Getenv("LEADER_PEER_ID")
	if leaderPeerID == "" {
		logger.Fatal("LEADER_PEER_ID is not set in environment variables.")
	}

	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		logger.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}

	// The Ethereum private key is used separately for Ethereum transactions
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		logger.Fatalf("Failed to decode Ethereum private key: %v", err)
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		logger.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	regularNode, err := regular.NewNode(&regular.Config{
		Port:            port,
		LeaderIP:        leaderIP,
		LeaderPort:      leaderPort,
		LeaderPeerID:    leaderPeerID,
		EOAPrivateKey:   privateKey,
		ContractAddress: contractAddress,
	}, fallbackEthClient)
	if err != nil {
		// Close the regular node before exiting
		regularNode.Close()
		logger.Fatalf("Failed to init the regular node: %v", err)
	}
	regularNode.Start(ctx)
}

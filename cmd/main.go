package main

import (
	"github.com/tokamak-network/DRB-node/nodes"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	logger.InitLogger()
	defer logger.CloseLogger()

	nodeType := os.Getenv("NODE_TYPE") // Expecting 'leader' or 'regular'

	// Initialize the fallback ethclient
	rpcUrls := strings.Split(os.Getenv("ETH_RPC_URLS"), ",")
	fallbackEthClient, err := fallback_ethclient.NewFallbackRPCClient(rpcUrls)
	if err != nil {
		log.Fatal("Failed to init the fallback ethclient", "err", err)
	}

	switch nodeType {
	case "leader":
		nodes.RunLeaderNode(fallbackEthClient)
	case "regular":
		nodes.RunRegularNode(fallbackEthClient)
	default:
		log.Fatal("NODE_TYPE must be set to either 'leader' or 'regular'")
	}
}

package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/logger"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
	regular_node "github.com/tokamak-network/DRB-node/nodes/regular"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	ctx := context.Background()

	logger.InitLogger()
	defer logger.CloseLogger()

	// Initialise DB
	// Load configuration
	cfg := database.LoadConfig()

	err := database.InitSQLDB(ctx, cfg.PostgresPort, cfg.PostgresHost, cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresName)
	if err != nil {
		log.Fatalf("Error initializing sql db: %v", err)
	}

	nodeType := os.Getenv("NODE_TYPE") // Expecting 'leader' or 'regular'

	// Initialize the fallback ethclient
	rpcUrls := strings.Split(os.Getenv("ETH_RPC_URLS"), ",")
	fallbackEthClient, err := fallback_ethclient.NewFallbackRPCClient(rpcUrls)
	if err != nil {
		log.Fatal("Failed to init the fallback ethclient", "err", err)
	}

	switch nodeType {
	case "leader":
		db := database.GetDB()
		leaderNodeHandler := leader_node.NewLeaderNodeHandler(fallbackEthClient, db)
		leaderNodeHandler.Run(ctx)
	case "regular":
		db := database.GetDB()
		regularNodeHandler := regular_node.NewRegularNodeHandler(fallbackEthClient, db)
		regularNodeHandler.Run(ctx)
	default:
		log.Fatal("NODE_TYPE must be set to either 'leader' or 'regular'")
	}
}

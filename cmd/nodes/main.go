package main

import (
	"log"
	"sync"
	"time"

	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/logger"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
	regular_node "github.com/tokamak-network/DRB-node/nodes/regular"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/pkg/shutdown"
)

func main() {
	// Initialize shutdown manager with 30 second timeout
	shutdownManager := shutdown.NewManager(30 * time.Second)
	ctx := shutdownManager.Context()

	logger.InitLogger()
	defer logger.CloseLogger()

	envCfg := appconfig.Get()

	// Initialise DB
	// Load configuration
	cfg := database.LoadConfig()

	err := database.InitSQLDB(ctx, cfg.PostgresPort, cfg.PostgresHost, cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresName)
	if err != nil {
		log.Fatalf("Error initializing sql db: %v", err)
	}

	// Initialize the fallback ethclient
	fallbackEthClient, err := fallback_ethclient.NewFallbackRPCClient(envCfg.EthRPCURLs)
	if err != nil {
		log.Fatal("Failed to init the fallback ethclient", "err", err)
	}

	// WaitGroup to track running services
	var wg sync.WaitGroup

	switch envCfg.NodeType {
	case "leader":
		db := database.GetDB()
		leaderNodeHandler := leader_node.NewLeaderNodeHandler(fallbackEthClient, db)

		wg.Add(1)
		go func() {
			defer wg.Done()
			leaderNodeHandler.Run(ctx)
		}()

	case "regular":
		db := database.GetDB()
		regularNodeHandler := regular_node.NewRegularNodeHandler(fallbackEthClient, db)

		wg.Add(1)
		go func() {
			defer wg.Done()
			regularNodeHandler.Run(ctx)
		}()

	default:
		log.Fatal("NODE_TYPE must be set to either 'leader' or 'regular'")
	}

	// Wait for shutdown signal
	shutdownManager.Wait()
	log.Println("Shutting down services...")

	// Close fallback eth client
	if fallbackEthClient != nil {
		fallbackEthClient.Close()
		log.Println("Fallback ETH client closed")
	}

	// Close database connection
	if err := database.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}

	// Wait for all services to complete (with timeout)
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		log.Println("All services stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Println("Warning: Some services did not stop within timeout")
	}

	log.Println("Graceful shutdown complete")
}

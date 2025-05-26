package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/nodes"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	logger.InitLogger()
	defer logger.CloseLogger()

	// Initialise DB
	// Load configuration
	cfg := database.LoadConfig()

	err := database.InitSQLDB(cfg.DBPort, cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		log.Fatalf("Error initializing sql db: %v", err)
	}

	nodeType := os.Getenv("NODE_TYPE") // Expecting 'leader' or 'regular'

	switch nodeType {
	case "leader":
		nodes.RunLeaderNode()
	case "regular":
		nodes.RunRegularNode()
	default:
		log.Fatal("NODE_TYPE must be set to either 'leader' or 'regular'")
	}
}

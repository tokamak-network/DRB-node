package main

import (
	"context"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/tokamak-network/DRB-node/client"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/service"
	"github.com/tokamak-network/DRB-node/utils"
)

func main() {
	logger.InitLogger()
	defer logger.CloseLogger()

	if err := godotenv.Load("./.env"); err != nil {
		logger.Log.Fatalf("Error loading .env file: %v", err)
	}

	logger.Log.Info("Service starting...")

	utils.PrintLogo()
	color.New(color.FgHiGreen, color.Bold).Println("Configuration loaded successfully. Ready to operate.")

	cfg := utils.LoadConfig()
	logger.Log.Infof("Loaded configuration: %+v", cfg)

	client, err := client.NewClient(cfg)
	if err != nil {
		logger.Log.Fatalf("Failed to create Client: %v", err)
	}
	logger.Log.Info("Client created successfully")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				// Process round results and check if there's a pending transaction
				if err := service.ProcessRoundResults(context.Background(), client); err != nil {
					logger.Log.Errorf("Processing round results failed: %v", err)
				} else {
					logger.Log.Info("Round results processed successfully")
				}
			}
		}
	}()

	logger.Log.Info("Service is now running")
	// Keep the service running
	_, _ = os.Stdout.Read(make([]byte, 1))
}

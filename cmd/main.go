package main

import (
	"context"
	"time"

	"github.com/fatih/color"
	"github.com/tokamak-network/DRB-Node/client"
	"github.com/tokamak-network/DRB-Node/logger"
	"github.com/tokamak-network/DRB-Node/service"
	"github.com/tokamak-network/DRB-Node/utils"
)

func main() {
	logger.InitLogger()
	defer logger.CloseLogger() // Ensure the log file is properly closed

	logger.Log.Info("Service starting...")

	utils.PrintLogo()
	color.New(color.FgHiGreen, color.Bold).Println("Configuration loaded successfully. Ready to operate.")

	cfg := utils.LoadConfig()
	logger.Log.Infof("Loaded configuration: %+v", cfg)

	pofClient, err := client.NewPoFClient(cfg)
	if err != nil {
		logger.Log.Fatalf("Failed to create PoFClient: %v", err)
	}
	logger.Log.Info("PoFClient created successfully")

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				if err := service.ProcessRoundResults(context.Background(), pofClient); err != nil {
					logger.Log.Errorf("Processing round results failed: %v", err)
				} else {
					logger.Log.Info("Round results processed successfully")
				}
			}
		}
	}()

	logger.Log.Info("Service is now running")
	select {}
}

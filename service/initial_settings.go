package service

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tokamak-network/DRB-node/logger" // Import your logger package
	"github.com/tokamak-network/DRB-node/service/transactions"
	"github.com/tokamak-network/DRB-node/utils"
	"math/big"
	"os"
)

func InitialSettings(ctx context.Context, client *utils.Client) error {
	walletAddress := os.Getenv("WALLET_ADDRESS")
	if walletAddress == "" {
		logger.Log.Error("WALLET_ADDRESS environment variable is not set")
		return fmt.Errorf("WALLET_ADDRESS environment variable is not set")
	}

	privateKeyHex := os.Getenv("PRIVATE_KEY")
	if privateKeyHex == "" {
		logger.Log.Error("PRIVATE_KEY environment variable is not set")
		return fmt.Errorf("PRIVATE_KEY environment variable is not set")
	}

	// Remove the '0x' prefix if present
	if privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		logger.Log.Errorf("Failed to parse private key: %v", err)
		return fmt.Errorf("Failed to parse private key: %v", err)
	}

	// Create auth transact opts with the loaded private key
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, big.NewInt(1)) // Specify the correct chain ID
	if err != nil {
		logger.Log.Errorf("Failed to create authorized transactor: %v", err)
		return fmt.Errorf("Failed to create authorized transactor: %v", err)
	}
	auth.From = crypto.PubkeyToAddress(privateKey.PublicKey)

	// Call the DepositAndActivate function from the transactions package
	_, tx, err := transactions.DepositAndActivate(ctx, client.Client, client.ContractAddress, auth)
	if err != nil {
		logger.Log.Errorf("Failed to deposit and activate: %v", err)
		return err
	}

	logger.Log.Infof("Deposit and Activate transaction submitted. TX Hash: %s", tx.Hash().Hex())
	return nil
}

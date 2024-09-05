package transactions

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

func FulfillRandomness(ctx context.Context, round *big.Int, pofClient *utils.PoFClient) (*types.Transaction, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"round": round,
	})

	log.Info("Starting FulfillRandomness process")

	// Use the generic ExecuteTransaction function to handle the transaction
	signedTx, _, err := ExecuteTransaction(ctx, pofClient, "fulfillRandomness", round)
	if err != nil {
		return nil, err
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, pofClient.Client, signedTx)
	if err != nil {
		log.Errorf("Failed to wait for transaction to be mined: %v", err)
		return nil, fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("Transaction %s reverted", signedTx.Hash().Hex())
		log.Errorf("❌ %s", errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	roundStatus.Store(round.String(), "Fulfilled")

	color.New(color.FgHiGreen, color.Bold).Printf("✅ FulfillRandomness successful!!\n🔗 Tx Hash: %s\n", signedTx.Hash().Hex())
	log.Infof("FulfillRandomness successful! Tx Hash: %s", signedTx.Hash().Hex())

	return signedTx, nil
}

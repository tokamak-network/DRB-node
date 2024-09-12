package transactions

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

// ReRequestRandomWordAtRound re-requests a random word for a specified round.
func ReRequestRandomWordAtRound(ctx context.Context, round *big.Int, pofClient *utils.Client) error {
	log := logger.Log.WithFields(logrus.Fields{
		"function": "ReRequestRandomWordAtRound",
		"round":    round.String(),
	})

	log.Info("Preparing to re-request random word at round...")

	// Execute the transaction using the generic function
	tx, _, err := ExecuteTransaction(ctx, pofClient, "reRequestRandomWordAtRound", nil, round)
	if err != nil {
		return err
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, pofClient.Client, tx)
	if err != nil {
		log.Errorf("Failed to wait for transaction to be mined: %v", err)
		return fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("Transaction %s reverted", tx.Hash().Hex())
		log.Errorf("❌ %s", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	roundStatus.Store(round.String(), "ReRequested")

	log.WithFields(logrus.Fields{
		"tx_hash": tx.Hash().Hex(),
	}).Info("✅ Re-request successful!")

	return nil
}

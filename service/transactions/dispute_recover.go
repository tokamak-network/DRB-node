package transactions

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-Node/logger"
	"github.com/tokamak-network/DRB-Node/utils"
)

// DisputeRecover handles the dispute recovery process.
func DisputeRecover(ctx context.Context, round *big.Int, v []utils.BigNumber, x utils.BigNumber, y utils.BigNumber, pofClient *utils.PoFClient) (*types.Transaction, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"round": round,
	})

	log.Info("Starting DisputeRecover process")

	// Execute the transaction using the generic function
	tx, _, err := ExecuteTransaction(ctx, pofClient, "disputeRecover", nil, round, v, x, y)
	if err != nil {
		return nil, err
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, pofClient.Client, tx)
	if err != nil {
		log.Errorf("Failed to wait for transaction to be mined: %v", err)
		return nil, fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("Transaction %s reverted", tx.Hash().Hex())
		log.Errorf("❌ %s", errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	roundStatus.Store(round.String(), "DisputeRecovered")

	log.WithFields(logrus.Fields{
		"tx_hash": tx.Hash().Hex(),
	}).Info("✅ Dispute recover successful!")

	return tx, nil
}

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

// DisputeLeadershipAtRound handles the dispute leadership process.
func DisputeLeadershipAtRound(ctx context.Context, round *big.Int, pofClient *utils.PoFClient) error {
	log := logger.Log.WithFields(logrus.Fields{
		"round": round,
	})

	log.Info("Starting DisputeLeadershipAtRound process")

	// Execute the transaction using the generic function
	tx, _, err := ExecuteTransaction(ctx, pofClient, "disputeLeadershipAtRound", nil, round)
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

	roundStatus.Store(round.String(), "DisputeLeadershiped")

	log.WithFields(logrus.Fields{
		"tx_hash": tx.Hash().Hex(),
	}).Info("✅ Dispute leadership successful!")

	return nil
}

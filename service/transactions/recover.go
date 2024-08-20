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

func Recover(ctx context.Context, round *big.Int, y utils.BigNumber, pofClient *utils.PoFClient) error {
	log := logger.Log.WithFields(logrus.Fields{
		"function": "Recover",
		"round":    round.String(),
	})

	log.Info("Starting recovery process...")

	// Use the generic ExecuteTransaction function to handle the transaction
	signedTx, _, err := ExecuteTransaction(ctx, pofClient, "recover", round, y)
	if err != nil {
		return err
	}

	log.Infof("Recovery transaction sent! Tx Hash: %s", signedTx.Hash().Hex())

	receipt, err := bind.WaitMined(ctx, pofClient.Client, signedTx)
	if err != nil {
		log.Errorf("Failed to wait for transaction to be mined: %v", err)
		return fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("Transaction %s reverted", signedTx.Hash().Hex())
		log.Errorf("❌ %s", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	roundStatus.Store(round.String(), "Recovered")

	log.Infof("✅ Recovery successful!!\n🔗 Tx Hash: %s", signedTx.Hash().Hex())

	return nil
}

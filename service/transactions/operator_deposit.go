package transactions

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
	"math/big"
)

func OperatorDepositAndActivate(ctx context.Context, client *utils.Client) (common.Address, *types.Transaction, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"function": "OperatorDepositAndActivate",
	})

	log.Info("Starting OperatorDeposit process")

	// Configuration and amount setup
	config := utils.GetConfig()
	amount := new(big.Int)
	amount.SetString(config.OperatorDepositFee, 10)

	// Execute the transaction using the generic function
	tx, auth, err := ExecuteTransaction(ctx, client, "depositAndActivate", amount)
	if err != nil {
		return common.Address{}, nil, err
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, client.Client, tx)
	if err != nil {
		log.Errorf("Failed to wait for transaction to be mined: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("Transaction %s reverted", tx.Hash().Hex())
		log.Errorf("❌ %s", errMsg)
		return common.Address{}, nil, fmt.Errorf("%s", errMsg)
	}

	log.Infof("✅ Deposit successful!! 🔗 Tx Hash: %s", tx.Hash().Hex())
	return auth.From, tx, nil
}

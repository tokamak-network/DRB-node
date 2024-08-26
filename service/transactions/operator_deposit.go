package transactions

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-Node/logger"
	"github.com/tokamak-network/DRB-Node/utils"
)

func OperatorDeposit(ctx context.Context, pofClient *utils.PoFClient) (common.Address, *types.Transaction, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"function": "OperatorDeposit",
	})

	log.Info("Starting OperatorDeposit process")

	config := utils.GetConfig()
		
	// Define the amount of Ether you want to send in the transaction
	amount := new(big.Int)
	amount.SetString(config.OperatorDespoitFee, 10)

	// Execute the transaction using the generic function
	tx, auth, err := ExecuteTransaction(ctx, pofClient, "operatorDeposit", amount)
	if err != nil {
		return common.Address{}, nil, err
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, pofClient.Client, tx)
	if err != nil {
		log.Errorf("Failed to wait for transaction to be mined: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("Transaction %s reverted", tx.Hash().Hex())
		log.Errorf("❌ %s", errMsg)
		return common.Address{}, nil, fmt.Errorf("%s", errMsg)
	}

	log.Infof("✅ Deposit successful!!\n🔗 Tx Hash: %s", tx.Hash().Hex())
	return auth.From, tx, nil
}

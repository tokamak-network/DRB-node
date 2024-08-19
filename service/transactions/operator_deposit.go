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
	// Use the logger from the logger package
	log := logger.Log.WithFields(logrus.Fields{
		"function": "OperatorDeposit",
	})

	log.Info("Starting OperatorDeposit process")

	chainID, err := pofClient.Client.NetworkID(ctx)
	if err != nil {
		log.Errorf("Failed to fetch network ID: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to fetch network ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(pofClient.PrivateKey, chainID)
	if err != nil {
		log.Errorf("Failed to create authorized transactor: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to create authorized transactor: %v", err)
	}

	nonce, err := pofClient.Client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		log.Errorf("Failed to fetch nonce: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to fetch nonce: %v", err)
	}
	auth.Nonce = big.NewInt(int64(nonce))

	gasPrice, err := pofClient.Client.SuggestGasPrice(ctx)
	if err != nil {
		log.Errorf("Failed to suggest gas price: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to suggest gas price: %v", err)
	}
	auth.GasPrice = gasPrice

	// Set the amount of Ether you want to send in the transaction
	amount := new(big.Int)
	amount.SetString("5000000000000000", 10) // 0.005 ether in wei
	auth.Value = amount                      // Setting the value of the transaction to 0.005 ether

	packedData, err := pofClient.ContractABI.Pack("operatorDeposit")
	if err != nil {
		log.Errorf("Failed to pack data for deposit: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to pack data for deposit: %v", err)
	}

	tx := types.NewTransaction(auth.Nonce.Uint64(), pofClient.ContractAddress, amount, 3000000, auth.GasPrice, packedData)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), pofClient.PrivateKey)
	if err != nil {
		log.Errorf("Failed to sign the transaction: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to sign the transaction: %v", err)
	}

	if err := pofClient.Client.SendTransaction(ctx, signedTx); err != nil {
		log.Errorf("Failed to send the signed transaction: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to send the signed transaction: %v", err)
	}

	receipt, err := bind.WaitMined(ctx, pofClient.Client, signedTx)
	if err != nil {
		log.Errorf("Failed to wait for transaction to be mined: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("Transaction %s reverted", signedTx.Hash().Hex())
		log.Errorf("❌ %s", errMsg)
		return common.Address{}, nil, fmt.Errorf("%s", errMsg)
	}

	log.Infof("✅ Deposit successful!!\n🔗 Tx Hash: %s", signedTx.Hash().Hex())
	return auth.From, signedTx, nil
}

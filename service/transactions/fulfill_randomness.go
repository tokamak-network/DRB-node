package transactions

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-Node/utils"
)

func FulfillRandomness(ctx context.Context, round *big.Int, pofClient *utils.PoFClient) (*types.Transaction, error) {
	logrus.Info("Starting FulfillRandomness process")

	chainID, err := pofClient.Client.NetworkID(ctx)
	if err != nil {
		logrus.Errorf("Failed to fetch network ID: %v", err)
		return nil, fmt.Errorf("failed to fetch network ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(pofClient.PrivateKey, chainID)
	if err != nil {
		logrus.Errorf("Failed to create authorized transactor: %v", err)
		return nil, fmt.Errorf("failed to create authorized transactor: %v", err)
	}

	nonce, err := pofClient.Client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		logrus.Errorf("Failed to fetch nonce: %v", err)
		return nil, fmt.Errorf("failed to fetch nonce: %v", err)
	}
	auth.Nonce = big.NewInt(int64(nonce))

	gasPrice, err := pofClient.Client.SuggestGasPrice(ctx)
	if err != nil {
		logrus.Errorf("Failed to suggest gas price: %v", err)
		return nil, fmt.Errorf("failed to suggest gas price: %v", err)
	}
	auth.GasPrice = gasPrice

	packedData, err := pofClient.ContractABI.Pack("fulfillRandomness", round)
	if err != nil {
		logrus.Errorf("Failed to pack data for fulfillRandomness: %v", err)
		return nil, fmt.Errorf("failed to pack data for fulfillRandomness: %v", err)
	}

	tx := types.NewTransaction(auth.Nonce.Uint64(), pofClient.ContractAddress, nil, 6000000, auth.GasPrice, packedData)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), pofClient.PrivateKey)
	if err != nil {
		logrus.Errorf("Failed to sign the transaction: %v", err)
		return nil, fmt.Errorf("failed to sign the transaction: %v", err)
	}

	if err := pofClient.Client.SendTransaction(ctx, signedTx); err != nil {
		logrus.Errorf("Failed to send the signed transaction: %v", err)
		return nil, fmt.Errorf("failed to send the signed transaction: %v", err)
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, pofClient.Client, signedTx)
	if err != nil {
		logrus.Errorf("Failed to wait for transaction to be mined: %v", err)
		return nil, fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("Transaction %s reverted", signedTx.Hash().Hex())
		logrus.Errorf("❌ %s", errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	roundStatus.Store(round.String(), "Fulfilled")

	color.New(color.FgHiGreen, color.Bold).Printf("✅ FulfillRandomness successful!!\n🔗 Tx Hash: %s\n", signedTx.Hash().Hex())
	logrus.Infof("FulfillRandomness successful! Tx Hash: %s", signedTx.Hash().Hex())

	return signedTx, nil
}

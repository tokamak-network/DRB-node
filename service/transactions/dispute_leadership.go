package transactions

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-Node/utils"
)

func DisputeLeadershipAtRound(ctx context.Context, round *big.Int, pofClient *utils.PoFClient) error {
	logrus.Info("Starting DisputeLeadershipAtRound process")

	chainID, err := pofClient.Client.NetworkID(ctx)
	if err != nil {
		logrus.Errorf("Failed to fetch network ID: %v", err)
		return fmt.Errorf("failed to fetch network ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(pofClient.PrivateKey, chainID)
	if err != nil {
		logrus.Errorf("Failed to create authorized transactor: %v", err)
		return fmt.Errorf("failed to create authorized transactor: %v", err)
	}

	nonce, err := pofClient.Client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		logrus.Errorf("Failed to fetch nonce: %v", err)
		return fmt.Errorf("failed to fetch nonce: %v", err)
	}
	auth.Nonce = big.NewInt(int64(nonce))

	gasPrice, err := pofClient.Client.SuggestGasPrice(ctx)
	if err != nil {
		logrus.Errorf("Failed to suggest gas price: %v", err)
		return fmt.Errorf("failed to suggest gas price: %v", err)
	}
	auth.GasPrice = gasPrice

	packedData, err := pofClient.ContractABI.Pack("disputeLeadershipAtRound", round)
	if err != nil {
		logrus.Errorf("Failed to pack data for disputeLeadershipAtRound: %v", err)
		return fmt.Errorf("failed to pack data for disputeLeadershipAtRound: %v", err)
	}

	tx := types.NewTransaction(auth.Nonce.Uint64(), pofClient.ContractAddress, nil, 6000000, auth.GasPrice, packedData)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), pofClient.PrivateKey)
	if err != nil {
		logrus.Errorf("Failed to sign the transaction: %v", err)
		return fmt.Errorf("failed to sign the transaction: %v", err)
	}

	if err := pofClient.Client.SendTransaction(ctx, signedTx); err != nil {
		logrus.Errorf("Failed to send the signed transaction: %v", err)
		return fmt.Errorf("failed to send the signed transaction: %v", err)
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, pofClient.Client, signedTx)
	if err != nil {
		logrus.Errorf("Failed to wait for transaction to be mined: %v", err)
		return fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("Transaction %s reverted", signedTx.Hash().Hex())
		logrus.Errorf("❌ %s", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	roundStatus.Store(round.String(), "DisputeLeadershiped")

	logrus.WithFields(logrus.Fields{
		"round":   round.String(),
		"tx_hash": signedTx.Hash().Hex(),
	}).Info("✅ Dispute leadership successful!")

	return nil
}

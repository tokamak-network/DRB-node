package transactions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-Node/logger"
	"github.com/tokamak-network/DRB-Node/utils"
)

var roundStatus sync.Map

func Commit(ctx context.Context, round *big.Int, pofClient *utils.PoFClient) (common.Address, []byte, error) {
	// Use the logger from the logger package
	log := logger.Log.WithFields(logrus.Fields{
		"round": round,
	})

	log.Info("Preparing to commit...")

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

	randomData := make([]byte, 32)
	if _, err := rand.Read(randomData); err != nil {
		log.Errorf("Failed to generate random data: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to generate random data: %v", err)
	}

	hexData := hex.EncodeToString(randomData)
	byteData, err := hex.DecodeString(hexData)
	if err != nil {
		log.Errorf("Failed to decode hex data: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to decode hex data: %v", err)
	}

	commitData := struct {
		Val    []byte
		Bitlen *big.Int
	}{
		Val:    byteData,
		Bitlen: big.NewInt(int64(len(byteData) * 8)), // Assuming byteData is directly the value committed
	}

	packedData, err := pofClient.ContractABI.Pack("commit", round, commitData)
	if err != nil {
		log.Errorf("Failed to pack data for commit: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to pack data for commit: %v", err)
	}

	tx := types.NewTransaction(auth.Nonce.Uint64(), pofClient.ContractAddress, nil, 3000000, auth.GasPrice, packedData)
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

	roundStatus.Store(round.String(), "Committed")

	log.Infof("✅  Commit successful!!\n🔗 Tx Hash: %s\n", signedTx.Hash().Hex())

	return auth.From, byteData, nil
}

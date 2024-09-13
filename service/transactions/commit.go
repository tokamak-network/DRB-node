package transactions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

func Commit(ctx context.Context, round *big.Int, client *utils.Client) (common.Address, []byte, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"round": round,
	})

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
		Round *big.Int
		Val   [32]byte
	}{
		Round: round,
		Val:   [32]byte{},
	}

	copy(commitData.Val[:], byteData)

	signedTx, auth, err := ExecuteTransaction(ctx, client, "commit", big.NewInt(0), round, commitData.Val)
	if err != nil {
		log.Errorf("Failed to execute transaction: %v", err)
		return common.Address{}, nil, err
	}

	receipt, err := bind.WaitMined(ctx, client.Client, signedTx)
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

	log.Infof("✅ Commit successful!!\n🔗 Tx Hash: %s\n", signedTx.Hash().Hex())

	return auth.From, byteData, nil
}

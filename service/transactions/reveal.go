package transactions

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

func Reveal(ctx context.Context, round *big.Int, client *utils.Client) (common.Address, []byte, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"round": round,
	})

	// Retrieve the randomData using the round as the key
	mu.Lock()
	randomData, exists := randomDataStore[round.String()]
	mu.Unlock()

	if !exists {
		errMsg := fmt.Sprintf("No random data found for round %s", round.String())
		log.Errorf(errMsg)
		return common.Address{}, nil, fmt.Errorf(errMsg)
	}

	// Log the revealed random data
	log.Printf("randomData (bytes32): %s", common.BytesToHash(randomData[:]).Hex())

	// Execute the transaction
	signedTx, auth, err := ExecuteTransaction(ctx, client, "reveal", big.NewInt(0), round, randomData)
	if err != nil {
		log.Errorf("Failed to execute transaction: %v", err)
		return common.Address{}, nil, err
	}

	// Wait for the transaction to be mined
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

	roundStatus.Store(round.String(), "Revealed")

	log.Infof("✅ Reveal successful!!\n🔗 Tx Hash: %s\n", signedTx.Hash().Hex())

	return auth.From, randomData[:], nil
}

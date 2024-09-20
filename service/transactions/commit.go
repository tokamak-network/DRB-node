package transactions

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
	"golang.org/x/crypto/sha3"
)

// Global map to store random data
var (
	randomDataStore = make(map[string][32]byte)
	mu              sync.Mutex
)

func Commit(ctx context.Context, round *big.Int, client *utils.Client) (common.Address, []byte, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"round": round,
	})

	// Generate 32 bytes of random data
	randomData := make([]byte, 32)
	if _, err := rand.Read(randomData); err != nil {
		log.Errorf("Failed to generate random data: %v", err)
		return common.Address{}, nil, fmt.Errorf("failed to generate random data: %v", err)
	}

	// Keccak256 hash of the random data
	hash := sha3.NewLegacyKeccak256()
	hash.Write(randomData)
	keccakHash := hash.Sum(nil)

	// Log random data and its hash
	log.Printf("randomData (bytes32): %s", common.BytesToHash(randomData).Hex())
	log.Printf("Keccak256 hash of randomData: %s", common.BytesToHash(keccakHash).Hex())

	// Store the randomData using round as the key
	mu.Lock()
	randomDataStore[round.String()] = *(*[32]byte)(randomData)
	mu.Unlock()

	// Prepare commitData with the hashed value and round value
	commitData := struct {
		Round *big.Int
		Val   [32]byte
	}{
		Round: round,
		Val:   [32]byte{},
	}

	// Copy the hashed value into commitData.Val
	copy(commitData.Val[:], keccakHash)

	// Execute the transaction
	signedTx, auth, err := ExecuteTransaction(ctx, client, "commit", big.NewInt(0), round, commitData.Val)
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

	roundStatus.Store(round.String(), "Committed")

	log.Infof("✅ Commit successful!!🔗 Tx Hash: %s", signedTx.Hash().Hex())

	return auth.From, randomData, nil
}

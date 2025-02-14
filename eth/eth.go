package eth

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

var roundStatus sync.Map

// Smart contract call helper function
func CallSmartContract(client *ethclient.Client, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	data, err := parsedABI.Pack(method, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack data for %s: %v", method, err)
	}

	result, err := client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &contractAddress,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract method %s: %v", method, err)
	}

	var unpackedResult interface{}
	err = parsedABI.UnpackIntoInterface(&unpackedResult, method, result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result for %s: %v", method, err)
	}

	return unpackedResult, nil
}

func ExecuteTransaction(
	ctx context.Context,
	client *utils.Client,
	functionName string,
	amount *big.Int,
	params ...interface{},
) (*types.Transaction, *bind.TransactOpts, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"function": functionName,
	})

	log.Infof("Preparing to execute %s...", functionName)

	chainID, err := client.Client.NetworkID(ctx)
	if err != nil {
		log.Errorf("Failed to fetch network ID: %v", err)
		return nil, nil, fmt.Errorf("failed to fetch network ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(client.PrivateKey, chainID)
	if err != nil {
		log.Errorf("Failed to create authorized transactor: %v", err)
		return nil, nil, fmt.Errorf("failed to create authorized transactor: %v", err)
	}

	nonce, err := client.Client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		log.Errorf("Failed to fetch nonce: %v", err)
		return nil, nil, fmt.Errorf("failed to fetch nonce: %v", err)
	}

	auth.Nonce = big.NewInt(int64(nonce))

	gasPrice, err := client.Client.SuggestGasPrice(ctx)
	if err != nil {
		log.Errorf("Failed to suggest gas price: %v", err)
		return nil, nil, fmt.Errorf("failed to suggest gas price: %v", err)
	}
	auth.GasPrice = gasPrice

	packedData, err := client.ContractABI.Pack(functionName, params...)
	if err != nil {
		log.Errorf("Failed to pack data for %s: %v", functionName, err)
		return nil, nil, fmt.Errorf("failed to pack data for %s: %v", functionName, err)
	}

	tx := types.NewTransaction(auth.Nonce.Uint64(), client.ContractAddress, amount, 3000000, auth.GasPrice, packedData)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), client.PrivateKey)
	if err != nil {
		log.Errorf("Failed to sign the transaction: %v", err)
		return nil, nil, fmt.Errorf("failed to sign the transaction: %v", err)
	}

	// Send the transaction
	if err := client.Client.SendTransaction(ctx, signedTx); err != nil {
		log.Errorf("Failed to send the signed transaction: %v", err)
		return nil, nil, fmt.Errorf("failed to send the signed transaction: %v", err)
	}

	// Wait for the transaction to be mined
	receipt, err := waitForTransactionSuccess(ctx, client, signedTx)
	if err != nil {
		log.Errorf("Transaction failed: %v", err)
		return nil, nil, err
	}

	log.Infof("Transaction %s confirmed in block %v", signedTx.Hash().Hex(), receipt.BlockNumber)
	return signedTx, auth, nil
}

// waitForTransactionSuccess waits for the transaction to be mined and returns the receipt
func waitForTransactionSuccess(ctx context.Context, client *utils.Client, tx *types.Transaction) (*types.Receipt, error) {
	for {
		receipt, err := client.Client.TransactionReceipt(ctx, tx.Hash())
		if err != nil {
			// Check if it's just waiting for confirmation (receipt not yet available)
			if err.Error() == "not found" {
				time.Sleep(3 * time.Second) // Wait and try again
				continue
			}
			return nil, fmt.Errorf("failed to get transaction receipt: %v", err)
		}
		if receipt.Status == types.ReceiptStatusSuccessful {
			return receipt, nil
		}
		return nil, fmt.Errorf("transaction failed with status: %v", receipt.Status)
	}
}

package eth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/pkg/constants"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

var (
	ErrTransactionFailed = errors.New("transaction failed")
)

var ActivatedOperators = make([]common.Address, 0)
var ActivatedOperatorsMu sync.RWMutex

// ActivatedOperators thread-safe access functions
func GetActivatedOperatorsCached() []common.Address {
	ActivatedOperatorsMu.RLock()
	defer ActivatedOperatorsMu.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]common.Address, len(ActivatedOperators))
	copy(result, ActivatedOperators)
	return result
}

func SetActivatedOperatorsCached(operators []common.Address) {
	ActivatedOperatorsMu.Lock()
	defer ActivatedOperatorsMu.Unlock()
	ActivatedOperators = make([]common.Address, len(operators))
	copy(ActivatedOperators, operators)
}

func GetActivatedOperatorsUnsafe() []common.Address {
	// For internal use where mutex is already held
	return ActivatedOperators
}

// Smart contract call helper function
func CallSmartContract(fallbackEthClient *fallback_ethclient.FallbackRPCClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	data, err := parsedABI.Pack(method, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack data for %s: %v", method, err)
	}

	result, err := fallbackEthClient.CallContract(context.Background(), ethereum.CallMsg{
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
	fallbackEthClient *fallback_ethclient.FallbackRPCClient,
	functionName string,
	amount *big.Int,
	params ...interface{},
) (*types.Transaction, *bind.TransactOpts, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"function": functionName,
	})

	log.Infof("Preparing to execute %s...", functionName)

	chainID, err := fallbackEthClient.NetworkID(ctx)
	if err != nil {
		log.Errorf("Failed to fetch network ID: %v", err)
		return nil, nil, fmt.Errorf("failed to fetch network ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(client.PrivateKey, chainID)
	if err != nil {
		log.Errorf("Failed to create authorized transactor: %v", err)
		return nil, nil, fmt.Errorf("failed to create authorized transactor: %v", err)
	}

	nonce, err := fallbackEthClient.PendingNonceAt(ctx, auth.From)
	if err != nil {
		log.Errorf("Failed to fetch nonce: %v", err)
		return nil, nil, fmt.Errorf("failed to fetch nonce: %v", err)
	}

	auth.Nonce = big.NewInt(int64(nonce))

	packedData, err := client.ContractABI.Pack(functionName, params...)
	if err != nil {
		log.Errorf("Failed to pack data for %s: %v", functionName, err)
		return nil, nil, fmt.Errorf("failed to pack data for %s: %v", functionName, err)
	}

	callMsg := ethereum.CallMsg{
		From: auth.From,
		To:   &client.ContractAddress,
		Data: packedData,
	}

	var estimateGas uint64
	maxAttempt := 3
	attempt := 0

	for attempt < maxAttempt {
		estimateGas, err = fallbackEthClient.EstimateGas(ctx, callMsg)
		if err != nil {
			attempt++
			log.Errorf("Gas estimation failed for %s, attempt %d: %v", functionName, attempt, err)
			if attempt == maxAttempt {
				return nil, nil, fmt.Errorf("gas estimation failed after %d attempts, %v transaction will revert", maxAttempt, functionName)
			}
			time.Sleep(10 * time.Second)
			continue
		}
		break
	}
	log.Infof("Transaction simulation successful, estimated gas: %d", estimateGas)

	receipt, signedTx, err := sendWithRetry(ctx, fallbackEthClient, chainID, auth, client.ContractAddress, amount, packedData)
	if err != nil {
		log.Errorf("Failed to send the signed transaction: %v", err)
		return nil, nil, fmt.Errorf("failed to send the signed transaction: %v", err)
	}

	log.Infof("Transaction %s confirmed in block %v", signedTx.Hash().Hex(), receipt.BlockNumber)
	return signedTx, auth, nil
}

func sendWithRetry(
	ctx context.Context,
	client *fallback_ethclient.FallbackRPCClient,
	chainID *big.Int,
	auth *bind.TransactOpts,
	toAddr common.Address,
	amount *big.Int,
	data []byte,
) (*types.Receipt, *types.Transaction, error) {

	// Estimate gas limit
	callMsg := ethereum.CallMsg{
		From: auth.From,
		To:   &toAddr,
		Data: data,
	}

	bumpFactor := 1.2 // Increase gas price by 20% each retry time

	var signedTx *types.Transaction
	maxRetries := 5 // Limit retries
	retryCount := 0

	priorityFee, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to suggest tip cap: %v", err)
	}

	baseFee, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to suggest gas price: %v", err)
	}

	maxFeePerGas := new(big.Int).Add(baseFee, priorityFee)

	for retryCount < maxRetries {
		nonce, err := client.PendingNonceAt(ctx, auth.From)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get nonce: %v", err)
		}

		gasLimit, err := client.EstimateGas(ctx, callMsg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to estimate gas: %v", err)
		}

		log.Printf("Estimated gas: %d", gasLimit)
		gasLimit = gasLimit * constants.Chains[chainID.Uint64()].EstimatedGasFactorPercent / 100
		log.Printf("Estimated gas after multiplying by factor: %d", gasLimit)

		// Start with base fee + tip
		log.Printf("Sending tx with maxFee %s wei", maxFeePerGas.String())
		log.Printf("Sending tx with priorityFee %s wei", priorityFee.String())

		// Build EIP-1559 transaction
		txData := &types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &toAddr,
			Value:     amount,
			Gas:       gasLimit,
			GasFeeCap: maxFeePerGas,
			GasTipCap: priorityFee,
			Data:      data,
		}
		tx := types.NewTx(txData)

		// Sign transaction
		signedTx, err = auth.Signer(auth.From, tx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to sign tx: %v", err)
		}

		// Send
		err = client.SendTransaction(ctx, signedTx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to send tx: %v", err)
		}

		log.Printf("Sent tx %s with maxFee %s wei", signedTx.Hash().Hex(), maxFeePerGas.String())

		blockTime := constants.Chains[chainID.Uint64()].BlockTime

		timeout := blockTime * 5 // wait for 5 blocks

		// Wait for receipt
		receipt, err := waitForTransactionSuccess(ctx, client, signedTx, timeout)
		if err != nil {
			if errors.Is(err, ErrTransactionFailed) {
				// If transaction failed, return the error
				return nil, nil, err
			}

			retryCount++
			if retryCount >= maxRetries {
				return nil, nil, fmt.Errorf("transaction failed after %d retries: %v", maxRetries, err)
			}
			// If failed or timeout -> bump gas and retry
			log.Printf("Bumping gas and retrying... (attempt %d/%d)", retryCount, maxRetries)
			newFee := new(big.Float).Mul(new(big.Float).SetInt(maxFeePerGas), big.NewFloat(bumpFactor))
			maxFeePerGas, _ = newFee.Int(nil)
			newPriorityFee := new(big.Float).Mul(new(big.Float).SetInt(priorityFee), big.NewFloat(bumpFactor))
			priorityFee, _ = newPriorityFee.Int(nil)
			continue
		}

		return receipt, signedTx, nil
	}

	// This should never be reached, but added for completeness
	return nil, nil, fmt.Errorf("unexpected end of retry loop")
}

func waitForTransactionSuccess(ctx context.Context, client *fallback_ethclient.FallbackRPCClient, tx *types.Transaction, timeout time.Duration) (*types.Receipt, error) {
	log.Printf("Waiting for transaction %s to be mined...", tx.Hash().Hex())
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeoutChan := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while waiting for transaction %s: %v", tx.Hash().Hex(), ctx.Err())
		case <-timeoutChan:
			return nil, fmt.Errorf("transaction %s stuck in mempool after %s", tx.Hash().Hex(), timeout)
		case <-ticker.C:
			log.Printf("Checking transaction %s status...", tx.Hash().Hex())
			receipt, err := client.TransactionReceipt(ctx, tx)
			if err != nil {
				if err == ethereum.NotFound {
					continue // still pending
				}
				return nil, fmt.Errorf("failed to get transaction receipt: %v", err)
			}

			// Check transaction status
			if receipt.Status == types.ReceiptStatusSuccessful {
				return receipt, nil
			} else if receipt.Status == types.ReceiptStatusFailed {
				return receipt, ErrTransactionFailed
			}

			// This should not happen, but handle it gracefully
			return receipt, fmt.Errorf("transaction %s has unknown status: %v", tx.Hash().Hex(), receipt.Status)
		}
	}
}

func GetActivatedOperators(fallbackEthClient *fallback_ethclient.FallbackRPCClient) ([]common.Address, error) {
	var activatedOperators []common.Address
	abiFilePath := "contract/abi/Commit2RevealDRB.json"
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		log.Fatalf("Failed to load contract ABI: %v", err)
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}

	contractAddress := common.HexToAddress(contractAddressStr)

	result, err := CallSmartContract(fallbackEthClient, parsedABI, "getActivatedOperators", contractAddress)
	if err != nil {
		log.Printf("Failed to fetch activated operators: %v", err)
		return activatedOperators, err
	}
	activatedOperators, _ = result.([]common.Address)
	return activatedOperators, nil
}

func UpdateActivatedOperators(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	operators, err := GetActivatedOperators(fallbackEthClient)
	if err != nil {
		log.Printf("Error updating ActivatedOperators: %v", err)
		return
	}
	SetActivatedOperatorsCached(operators)
}

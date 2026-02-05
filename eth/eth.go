package eth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/pkg/constants"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// Service exposes a default implementation of IEthService so callers can depend on
// an interface for easier testing and mocking: eth.Service.(method)
var Service IEthService = NewDefaultEthService()

var (
	ErrTransactionFailed = errors.New("transaction failed")
	txMu                 sync.Mutex // Mutex to serialize transaction sending
)

var ActivatedOperators = make([]common.Address, 0)
var ActivatedOperatorsMu sync.RWMutex

// Cached contract ABI - loaded once and reused
var (
	cachedABI     abi.ABI
	cachedABIOnce sync.Once
)

// getCachedABI returns the cached contract ABI, loading it once on first call
func getCachedABI() abi.ABI {
	cachedABIOnce.Do(func() {
		var err error
		cachedABI, err = utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
		if err != nil {
			log.Fatalf("Failed to load contract ABI: %v", err)
		}
	})
	return cachedABI
}

// ActivatedOperators thread-safe access functions
func GetActivatedOperatorsCached() []common.Address {
	ActivatedOperatorsMu.RLock()
	defer ActivatedOperatorsMu.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]common.Address, len(ActivatedOperators))
	copy(result, ActivatedOperators)
	return result
}

func GetActivatedOperatorsLength() int64 {
	ActivatedOperatorsMu.RLock()
	defer ActivatedOperatorsMu.RUnlock()
	return int64(len(ActivatedOperators))
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
func CallSmartContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	data, err := parsedABI.Pack(method, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack data for %s: %v", method, err)
	}

	result, err := fallbackEthClient.CallContract(ctx, ethereum.CallMsg{
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
	fallbackEthClient fallback_ethclient.IFallbackEthClient,
	functionName string,
	amount *big.Int,
	params ...interface{},
) (*types.Transaction, *bind.TransactOpts, error) {
	// Serialize transaction sending to simplify nonce management
	txMu.Lock()
	defer txMu.Unlock()

	log := logger.Log.WithFields(logrus.Fields{
		"function": functionName,
	})

	log.Infof("Preparing to execute %s...", functionName)

	chainID := appconfig.GetChainIDAsBigInt()
	if chainID == nil {
		return nil, nil, fmt.Errorf("CHAIN_ID environment variable is not set or invalid")
	}

	auth, err := bind.NewKeyedTransactorWithChainID(client.PrivateKey, chainID)
	if err != nil {
		log.Errorf("Failed to create authorized transactor: %v", err)
		return nil, nil, fmt.Errorf("failed to create authorized transactor: %v", err)
	}

	packedData, err := client.ContractABI.Pack(functionName, params...)
	if err != nil {
		log.Errorf("Failed to pack data for %s: %v", functionName, err)
		return nil, nil, fmt.Errorf("failed to pack data for %s: %v", functionName, err)
	}

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
	client fallback_ethclient.IFallbackEthClient,
	chainID *big.Int,
	auth *bind.TransactOpts,
	toAddr common.Address,
	amount *big.Int,
	data []byte,
) (*types.Receipt, *types.Transaction, error) {

	// Estimate gas limit
	callMsg := ethereum.CallMsg{
		From:  auth.From,
		To:    &toAddr,
		Data:  data,
		Value: amount,
	}

	bumpFactor := 1.2 // Increase gas price by 20% each retry time

	var signedTx *types.Transaction
	maxRetries := 5 // Limit retries
	retryCount := 0

	var priorityFee *big.Int
	var err error
	maxGasRetries := 3
	gasRetryDelay := 2 * time.Second
	for attempt := 1; attempt <= maxGasRetries; attempt++ {
		priorityFee, err = client.SuggestGasTipCap(ctx)
		if err == nil {
			break
		}
		log.Printf("Failed to suggest tip cap, attempt %d/%d: %v", attempt, maxGasRetries, err)
		if attempt < maxGasRetries {
			time.Sleep(gasRetryDelay)
			continue
		}
		return nil, nil, fmt.Errorf("failed to suggest tip cap after %d attempts: %v", maxGasRetries, err)
	}

	var baseFee *big.Int
	for attempt := 1; attempt <= maxGasRetries; attempt++ {
		baseFee, err = client.SuggestGasPrice(ctx)
		if err == nil {
			break
		}
		log.Printf("Failed to suggest gas price, attempt %d/%d: %v", attempt, maxGasRetries, err)
		if attempt < maxGasRetries {
			time.Sleep(gasRetryDelay)
			continue
		}
		return nil, nil, fmt.Errorf("failed to suggest gas price after %d attempts: %v", maxGasRetries, err)
	}

	minBaseFee := big.NewInt(1000000000)     // 1 gwei
	minPriorityFee := big.NewInt(1000000000) // 1 gwei

	if baseFee.Cmp(minBaseFee) < 0 {
		baseFee = minBaseFee
	}
	if priorityFee.Cmp(minPriorityFee) < 0 {
		priorityFee = minPriorityFee
	}

	maxFeePerGas := new(big.Int).Add(baseFee, priorityFee)

	maxAllowedGasPrice := big.NewInt(100000000000) // 100 gwei
	if maxFeePerGas.Cmp(maxAllowedGasPrice) > 0 {
		log.Printf("Warning: maxFeePerGas %s exceeds max allowed %s, capping to max allowed", maxFeePerGas.String(), maxAllowedGasPrice.String())
		maxFeePerGas = maxAllowedGasPrice
		if priorityFee.Cmp(maxAllowedGasPrice) > 0 {
			priorityFee = big.NewInt(1000000000) // 1 gwei
			maxFeePerGas = new(big.Int).Add(baseFee, priorityFee)
		}
	}

	// Fetch nonce once before the retry loop to prevent race conditions
	// If we retry with a new nonce, we might execute the same transaction twice
	nonce, err := client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get nonce: %v", err)
	}
	log.Printf("Using nonce %d for transaction", nonce)

	for retryCount < maxRetries {
		gasLimit, err := client.EstimateGas(ctx, callMsg)
		if err != nil {
			log.Printf("Failed to estimate gas: %v, continuing to next retry", err)
			retryCount++
			if retryCount >= maxRetries {
				return nil, nil, fmt.Errorf("failed to estimate gas after %d retries: %v", maxRetries, err)
			}
			continue
		}

		log.Printf("Estimated gas: %d", gasLimit)

		// Get gas factor for chain, default to 150% if chain not found
		chainInfo, exists := constants.Chains[chainID.Uint64()]
		gasFactor := uint64(150) // Default 150% if chain not found
		if exists {
			gasFactor = chainInfo.EstimatedGasFactorPercent
		} else {
			log.Printf("Chain ID %d not found in constants, using default gas factor 150%%", chainID.Uint64())
		}

		gasLimit = gasLimit * gasFactor / 100
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
			log.Printf("Failed to sign tx %v, continuing to next retry", err)
			retryCount++
			if retryCount >= maxRetries {
				return nil, nil, fmt.Errorf("failed to sign tx after %d retries: %v", maxRetries, err)
			}
			continue
		}

		// Send
		err = client.SendTransaction(ctx, signedTx)
		if err != nil {
			log.Printf("Failed to send tx %v, continuing to next retry", err)

			if utils.IsReplacementError(err) {
				log.Printf("Transaction with nonce %d was already processed (nonce too low). Checking for receipt...", nonce)
				// Try to find the receipt of the original transaction
				receipt, tx, err, found := checkTransactionInclusion(ctx, client, signedTx)
				if found {
					return receipt, tx, err
				}
				// If we can't find the receipt, the transaction might still be pending
				// Continue with retry logic but preserve the nonce
			}

			retryCount++
			if retryCount >= maxRetries {
				return nil, nil, fmt.Errorf("failed to send tx after %d retries: %v", maxRetries, err)
			}
			continue
		}

		log.Printf("Sent tx %s with maxFee %s wei, value: %s wei", signedTx.Hash().Hex(), maxFeePerGas.String(), amount.String())

		blockTime := 1 * time.Second // Default 1 second for Geth dev mode
		if chainInfo, exists := constants.Chains[chainID.Uint64()]; exists {
			blockTime = chainInfo.BlockTime
		}

		timeout := blockTime * 5 // wait for 5 blocks

		// Wait for receipt
		receipt, err := waitForTransactionSuccess(ctx, client, signedTx, timeout)
		if err != nil {
			if errors.Is(err, ErrTransactionFailed) {
				// If transaction failed, return the error
				return nil, nil, err
			}

			// Before retrying, check if the transaction was included after the timeout
			// This handles the race condition where the transaction is processed right after timeout
			log.Printf("Transaction %s timed out, checking if it was included after timeout...", signedTx.Hash().Hex())
			receipt, tx, err, found := checkTransactionInclusion(ctx, client, signedTx)
			if found {
				return receipt, tx, err
			}

			retryCount++
			if retryCount >= maxRetries {
				return nil, nil, fmt.Errorf("transaction failed after %d retries: %v", maxRetries, err)
			}
			// If failed or timeout -> bump gas and retry with same nonce
			log.Printf("Bumping gas and retrying with same nonce %d... (attempt %d/%d)", nonce, retryCount, maxRetries)
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

func waitForTransactionSuccess(ctx context.Context, client fallback_ethclient.IFallbackEthClient, tx *types.Transaction, timeout time.Duration) (*types.Receipt, error) {
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
				log.Printf("Transaction %s failed (reverted). Gas used: %d", tx.Hash().Hex(), receipt.GasUsed)
				return receipt, fmt.Errorf("%w: transaction %s reverted (gas used: %d)", ErrTransactionFailed, tx.Hash().Hex(), receipt.GasUsed)
			}

			// This should not happen, but handle it gracefully
			return receipt, fmt.Errorf("transaction %s has unknown status: %v", tx.Hash().Hex(), receipt.Status)
		}
	}
}

func GetActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
	var activatedOperators []common.Address
	parsedABI := getCachedABI()

	contractAddressStr := appconfig.Get().ContractAddress
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}

	contractAddress := common.HexToAddress(contractAddressStr)

	result, err := CallSmartContract(ctx, fallbackEthClient, parsedABI, "getActivatedOperators", contractAddress)
	if err != nil {
		log.Printf("Failed to fetch activated operators: %v", err)
		return activatedOperators, err
	}
	activatedOperators, _ = result.([]common.Address)
	return activatedOperators, nil
}

func UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) error {
	operators, err := GetActivatedOperators(ctx, fallbackEthClient)
	if err != nil {
		log.Printf("Error updating ActivatedOperators: %v", err)
		return err
	}
	SetActivatedOperatorsCached(operators)
	return nil
}

// UpdateCurrentRoundFromContract fetches the current round from the contract
func UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	parsedABI := getCachedABI()

	contractAddressStr := appconfig.Get().ContractAddress
	if contractAddressStr == "" {
		return nil, fmt.Errorf("CONTRACT_ADDRESS is not set in environment variables")
	}

	contractAddress := common.HexToAddress(contractAddressStr)

	// Call the s_currentRound() function (public getter for s_currentRound state variable)
	result, err := CallSmartContract(ctx, fallbackEthClient, parsedABI, "s_currentRound", contractAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to call s_currentRound: %v", err)
	}

	// The result is in *big.Int
	currentRound, ok := result.(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unexpected result type for s_currentRound: %T", result)
	}

	log.Printf("Fetched current round from contract: %s", currentRound.String())
	return currentRound, nil
}

// GetTrialNumFromContract fetches the trial number for a given round from the smart contract
func GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	parsedABI := getCachedABI()

	contractAddressStr := appconfig.Get().ContractAddress
	if contractAddressStr == "" {
		return nil, fmt.Errorf("CONTRACT_ADDRESS is not set in environment variables")
	}

	contractAddress := common.HexToAddress(contractAddressStr)

	result, err := CallSmartContract(ctx, fallbackEthClient, parsedABI, "s_trialNum", contractAddress, round)
	if err != nil {
		return nil, fmt.Errorf("failed to call s_trialNum: %v", err)
	}

	trialNum, ok := result.(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unexpected result type for s_trialNum: %T", result)
	}

	log.Printf("Fetched trialNum for round %s from contract: %s", round.String(), trialNum.String())
	return trialNum, nil
}

// checkTransactionInclusion attempts to find a receipt for the given transaction.
// It returns the receipt, the transaction, an error if the transaction failed, and a boolean indicating if a receipt was found.
func checkTransactionInclusion(ctx context.Context, client fallback_ethclient.IFallbackEthClient, tx *types.Transaction) (*types.Receipt, *types.Transaction, error, bool) {
	receipt, err := client.TransactionReceipt(ctx, tx)
	if err == nil && receipt != nil {
		if receipt.Status == types.ReceiptStatusSuccessful {
			return receipt, tx, nil, true
		}
		// transaction reverted
		revertErr := fmt.Errorf("%w: transaction %s reverted (gas used: %d)", ErrTransactionFailed, tx.Hash().Hex(), receipt.GasUsed)
		return receipt, tx, revertErr, true
	}
	// NotFound or other errors are treated as "not found" for simplicity here.
	return nil, nil, nil, false
}

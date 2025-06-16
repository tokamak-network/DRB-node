package eth

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

var ActivatedOperators = make([]common.Address, 0)

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
	logger.Infof("Preparing to execute %s...", functionName)

	chainID, err := fallbackEthClient.NetworkID(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch network ID: %v", err)
		return nil, nil, fmt.Errorf("failed to fetch network ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(client.PrivateKey, chainID)
	if err != nil {
		logger.Errorf("Failed to create authorized transactor: %v", err)
		return nil, nil, fmt.Errorf("failed to create authorized transactor: %v", err)
	}

	nonce, err := fallbackEthClient.PendingNonceAt(ctx, auth.From)
	if err != nil {
		logger.Errorf("Failed to fetch nonce: %v", err)
		return nil, nil, fmt.Errorf("failed to fetch nonce: %v", err)
	}

	auth.Nonce = big.NewInt(int64(nonce))

	gasPrice, err := fallbackEthClient.SuggestGasPrice(ctx)
	if err != nil {
		logger.Errorf("Failed to suggest gas price: %v", err)
		return nil, nil, fmt.Errorf("failed to suggest gas price: %v", err)
	}
	auth.GasPrice = new(big.Int).Mul(gasPrice, big.NewInt(3))

	packedData, err := client.ContractABI.Pack(functionName, params...)
	if err != nil {
		logger.Errorf("Failed to pack data for %s: %v", functionName, err)
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
			logger.Errorf("Gas estimation failed for %s, attempt %d: %v", functionName, attempt, err)
			if attempt == maxAttempt {
				return nil, nil, fmt.Errorf("gas estimation failed after %d attempts, %v transaction will revert", maxAttempt, functionName)
			}
			time.Sleep(10 * time.Second)
			continue
		}
		break
	}
	logger.Infof("Transaction simulation successful, estimated gas: %d", estimateGas)

	tx := types.NewTransaction(auth.Nonce.Uint64(), client.ContractAddress, amount, 3000000, auth.GasPrice, packedData)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), client.PrivateKey)
	if err != nil {
		logger.Errorf("Failed to sign the transaction: %v", err)
		return nil, nil, fmt.Errorf("failed to sign the transaction: %v", err)
	}

	// Send the transaction
	if err := fallbackEthClient.SendTransaction(ctx, signedTx); err != nil {
		logger.Errorf("Failed to send the signed transaction: %v", err)
		return nil, nil, fmt.Errorf("failed to send the signed transaction: %v", err)
	}

	// Wait for the transaction to be mined
	receipt, err := waitForTransactionSuccess(ctx, fallbackEthClient, signedTx)
	if err != nil {
		logger.Errorf("Transaction failed: %v", err)
		return nil, nil, err
	}

	logger.Infof("Transaction %s confirmed in block %v", signedTx.Hash().Hex(), receipt.BlockNumber)
	return signedTx, auth, nil
}

// waitForTransactionSuccess waits for the transaction to be mined and returns the receipt
func waitForTransactionSuccess(ctx context.Context, fallbackEthClient *fallback_ethclient.FallbackRPCClient, tx *types.Transaction) (*types.Receipt, error) {
	for {
		receipt, err := fallbackEthClient.TransactionReceipt(ctx, tx)
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

func GetActivatedOperators(fallbackEthClient *fallback_ethclient.FallbackRPCClient) ([]common.Address, error) {
	var activatedOperators []common.Address
	abiFilePath := "contract/abi/Commit2RevealDRB.json"
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		logger.Fatalf("Failed to load contract ABI: %v", err)
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		logger.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}

	contractAddress := common.HexToAddress(contractAddressStr)

	result, err := CallSmartContract(fallbackEthClient, parsedABI, "getActivatedOperators", contractAddress)
	if err != nil {
		logger.Infof("Failed to fetch activated operators: %v", err)
		return activatedOperators, err
	}
	activatedOperators, _ = result.([]common.Address)
	return activatedOperators, nil
}

func UpdateActivatedOperators(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	var err error
	ActivatedOperators, err = GetActivatedOperators(fallbackEthClient)
	if err != nil {
		logger.Infof("Error updating ActivatedOperators: %v", err)
	}
}

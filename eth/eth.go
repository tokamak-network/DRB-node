package eth

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
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

var ActivatedOperators = make([]common.Address, 0)

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
	auth.GasPrice = new(big.Int).Mul(gasPrice, big.NewInt(3))

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
		estimateGas, err = client.Client.EstimateGas(ctx, callMsg)
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

	// log.Infof("\033[32m✅ Transaction %s confirmed in block %v\033[0m", signedTx.Hash().Hex(), receipt.BlockNumber)
	fmt.Printf("✅ \033[32mTransaction %s confirmed in block %v for function %v.\033[0m\n", signedTx.Hash().Hex(), receipt.BlockNumber, functionName)
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

func GetActivatedOperators() ([]common.Address, error) {

	var activatedOperators []common.Address
	ethRPCURL := os.Getenv("ETH_RPC_URL")
	client, err := ethclient.Dial(ethRPCURL)
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum RPC: %v", err)
	}

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

	result, err := CallSmartContract(client, parsedABI, "getActivatedOperators", contractAddress)
	if err != nil {
		log.Printf("Failed to fetch activated operators: %v", err)
		return activatedOperators, err
	}
	activatedOperators, _ = result.([]common.Address)
	return activatedOperators, nil
}

func UpdateActivatedOperators() {
	var err error
	ActivatedOperators, err = GetActivatedOperators()
	if err != nil {
		log.Printf("Error updating ActivatedOperators: %v", err)
	}
}

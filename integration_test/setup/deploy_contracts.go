package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ContractJSON represents the structure of the contract JSON file
type ContractJSON struct {
	ABI      interface{} `json:"abi"`
	Bytecode struct {
		Object string `json:"object"`
	} `json:"bytecode"`
}

func LoadContractFromJSON(abiPath string) (*abi.ABI, []byte, error) {
	// Read the JSON file
	jsonData, err := os.ReadFile(abiPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read contract JSON: %w", err)
	}

	// Parse the JSON
	var contractJSON ContractJSON
	if err := json.Unmarshal(jsonData, &contractJSON); err != nil {
		return nil, nil, fmt.Errorf("failed to parse contract JSON: %w", err)
	}

	// Parse the ABI
	abiBytes, err := json.Marshal(contractJSON.ABI)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ABI: %w", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(string(abiBytes)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse ABI: %w", err)
	}

	// Parse the bytecode
	bytecodeStr := contractJSON.Bytecode.Object
	if strings.HasPrefix(bytecodeStr, "0x") {
		bytecodeStr = bytecodeStr[2:]
	}

	bytecode := common.Hex2Bytes(bytecodeStr)
	if len(bytecode) == 0 {
		return nil, nil, fmt.Errorf("empty bytecode")
	}

	return &parsedABI, bytecode, nil
}

func DeployCommitReveal2L2(
	ctx context.Context,
	client *ethclient.Client,
	auth *bind.TransactOpts,
	abiPath string,
) (common.Address, *common.Hash, *abi.ABI, error) {
	// Load contract ABI and bytecode
	parsedABI, bytecode, err := LoadContractFromJSON(abiPath)
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	// Constructor parameters (optimized for testing)
	activationThreshold := big.NewInt(1e16) // 0.01 ETH
	flatFee := big.NewInt(1e15)             // 0.001 ETH
	name := "Commit Reveal2"
	version := "1"
	offChainSubmissionPeriod := big.NewInt(40)            // 40 seconds
	requestOrSubmitOrFailDecisionPeriod := big.NewInt(30) // 30 seconds
	onChainSubmissionPeriod := big.NewInt(60)             // 60 seconds
	offChainSubmissionPeriodPerOperator := big.NewInt(20) // 20 seconds
	onChainSubmissionPeriodPerOperator := big.NewInt(30)  // 30 seconds
	maxGasPrice := big.NewInt(2e9)                        // 2 gwei
	governanceMultisig := auth.From

	constructorArgs, err := parsedABI.Constructor.Inputs.Pack(
		activationThreshold,
		flatFee,
		name,
		version,
		offChainSubmissionPeriod,
		requestOrSubmitOrFailDecisionPeriod,
		onChainSubmissionPeriod,
		offChainSubmissionPeriodPerOperator,
		onChainSubmissionPeriodPerOperator,
		maxGasPrice,
		governanceMultisig,
	)
	if err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to pack constructor args: %w", err)
	}

	// Combine bytecode and constructor args
	deployData := append(bytecode, constructorArgs...)

	// Get nonce
	nonce, err := client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	// Create deployment transaction
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to suggest gas price: %w", err)
	}

	deployValue := big.NewInt(2e18) // 2 ETH

	fmt.Printf("Sending %s ETH with deployment (required by constructor)\n",
		new(big.Int).Div(deployValue, big.NewInt(1e18)).String())

	// Ensure we have a reasonable gas limit
	gasLimit := auth.GasLimit
	if gasLimit == 0 {
		gasLimit = 10_000_000
	}

	fmt.Printf("Gas limit: %d, Gas price: %s wei\n", gasLimit, gasPrice.String())

	tx := types.NewContractCreation(
		nonce,
		deployValue,
		gasLimit,
		gasPrice,
		deployData,
	)

	// Sign transaction
	signedTx, err := auth.Signer(auth.From, tx)
	if err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction
	if err := client.SendTransaction(ctx, signedTx); err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	txHash := signedTx.Hash()

	// Calculate contract address
	contractAddress := crypto.CreateAddress(auth.From, nonce)

	fmt.Printf("Transaction hash: %s\n", txHash.Hex())
	fmt.Printf("Contract will be deployed at: %s\n", contractAddress.Hex())

	return contractAddress, &txHash, parsedABI, nil
}

func DeployConsumerExample(
	ctx context.Context,
	client *ethclient.Client,
	auth *bind.TransactOpts,
	abiPath string,
	coordinatorAddress common.Address,
) (common.Address, *common.Hash, *abi.ABI, error) {
	// Load contract ABI and bytecode
	parsedABI, bytecode, err := LoadContractFromJSON(abiPath)
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	// Constructor parameter
	constructorArgs, err := parsedABI.Constructor.Inputs.Pack(coordinatorAddress)
	if err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to pack constructor args: %w", err)
	}

	fmt.Printf("Consumer constructor args packed: %d bytes\n", len(constructorArgs))

	// Combine bytecode and constructor args
	deployData := append(bytecode, constructorArgs...)

	fmt.Printf("Total deployment data: %d bytes (bytecode: %d + args: %d)\n",
		len(deployData), len(bytecode), len(constructorArgs))

	// Get nonce
	nonce, err := client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	// Create deployment transaction
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to suggest gas price: %w", err)
	}

	// Ensure we have a reasonable gas limit
	gasLimit := auth.GasLimit
	if gasLimit == 0 {
		gasLimit = 10_000_000
	}

	fmt.Printf("Gas limit: %d, Gas price: %s wei\n", gasLimit, gasPrice.String())

	tx := types.NewContractCreation(
		nonce,
		big.NewInt(0),
		gasLimit,
		gasPrice,
		deployData,
	)

	// Sign transaction
	signedTx, err := auth.Signer(auth.From, tx)
	if err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction
	if err := client.SendTransaction(ctx, signedTx); err != nil {
		return common.Address{}, nil, nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	txHash := signedTx.Hash()

	// Calculate contract address
	contractAddress := crypto.CreateAddress(auth.From, nonce)

	fmt.Printf("Transaction hash: %s\n", txHash.Hex())
	fmt.Printf("Contract will be deployed at: %s\n", contractAddress.Hex())

	return contractAddress, &txHash, parsedABI, nil
}

// ActivateOperator activates an operator in the Commit2RevealDRB contract
func (env *GethTestEnv) ActivateOperator(ctx context.Context, operatorAuth *bind.TransactOpts) error {
	depositAmount := big.NewInt(2e18)

	operatorAuth.Value = depositAmount
	operatorAuth.GasLimit = 500000

	// Call depositAndActivate function
	input, err := env.ContractABI.Pack("depositAndActivate")
	if err != nil {
		return fmt.Errorf("failed to pack depositAndActivate: %w", err)
	}

	nonce, err := env.Client.PendingNonceAt(ctx, operatorAuth.From)
	if err != nil {
		return fmt.Errorf("failed to get nonce: %w", err)
	}

	gasPrice, err := env.Client.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("failed to suggest gas price: %w", err)
	}

	tx := types.NewTransaction(
		nonce,
		env.ContractAddress,
		depositAmount,
		operatorAuth.GasLimit,
		gasPrice,
		input,
	)

	signedTx, err := operatorAuth.Signer(operatorAuth.From, tx)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	if err := env.Client.SendTransaction(ctx, signedTx); err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	txHash := signedTx.Hash()
	fmt.Printf(" ActivateOperator tx: %s\n", txHash.Hex())

	if err := env.WaitForTransaction(ctx, &txHash); err != nil {
		return fmt.Errorf("failed to wait for activation: %w", err)
	}
	operatorAuth.Value = big.NewInt(0)

	fmt.Printf("Operator %s activated\n", operatorAuth.From.Hex())
	return nil
}

// GetActivatedOperators returns the list of activated operators
func (env *GethTestEnv) GetActivatedOperators(ctx context.Context) ([]common.Address, error) {
	input, err := env.ContractABI.Pack("getActivatedOperators")
	if err != nil {
		return nil, fmt.Errorf("failed to pack getActivatedOperators: %w", err)
	}

	// Call the contract
	result, err := env.Client.CallContract(ctx, ethereum.CallMsg{
		To:   &env.ContractAddress,
		Data: input,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %w", err)
	}

	// Unpack the result
	var operators []common.Address
	if err := env.ContractABI.UnpackIntoInterface(&operators, "getActivatedOperators", result); err != nil {
		return nil, fmt.Errorf("failed to unpack result: %w", err)
	}

	return operators, nil
}

// GetCurrentRound returns the current round and trial number
func (env *GethTestEnv) GetCurrentRound(ctx context.Context) (*big.Int, *big.Int, error) {
	input, err := env.ContractABI.Pack("getCurRoundAndTrialNum")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to pack getCurRoundAndTrialNum: %w", err)
	}

	result, err := env.Client.CallContract(ctx, ethereum.CallMsg{
		To:   &env.ContractAddress,
		Data: input,
	}, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to call contract: %w", err)
	}

	// The function returns two uint256 values
	round := new(big.Int).SetBytes(result[:32])
	trialNum := new(big.Int).SetBytes(result[32:64])

	return round, trialNum, nil
}

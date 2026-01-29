package eth

import (
	"context"
	"errors"
	"math/big"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

// MockFallbackEthClient for testing
type MockFallbackEthClient struct {
	mock.Mock
}

func (m *MockFallbackEthClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	args := m.Called(ctx, account, blockNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	args := m.Called(ctx, msg, blockNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockFallbackEthClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	args := m.Called(ctx, tx)
	return args.Error(0)
}

func (m *MockFallbackEthClient) BlockTimestamp(ctx context.Context, blockNumber *big.Int) (uint64, error) {
	args := m.Called(ctx, blockNumber)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockFallbackEthClient) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	args := m.Called(ctx, q, ch)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ethereum.Subscription), args.Error(1)
}

func (m *MockFallbackEthClient) ChainID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	args := m.Called(ctx, msg)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockFallbackEthClient) TransactionReceipt(ctx context.Context, tx *types.Transaction) (*types.Receipt, error) {
	args := m.Called(ctx, tx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Receipt), args.Error(1)
}

func (m *MockFallbackEthClient) NetworkID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	args := m.Called(ctx, account)
	return args.Get(0).(uint64), args.Error(1)
}

func TestMain(m *testing.M) {
	logger.InitLogger()

	// Setup: Create ABI file before tests
	// Run tests
	code := m.Run()

	// Cleanup
	os.Exit(code)
}

// Test activated operators cache functions
func TestGetActivatedOperatorsCached(t *testing.T) {
	// Reset cache
	SetActivatedOperatorsCached([]common.Address{})

	operators := GetActivatedOperatorsCached()
	assert.NotNil(t, operators)
	assert.Equal(t, 0, len(operators))
}

func TestGetActivatedOperatorsLength(t *testing.T) {
	// Reset cache
	SetActivatedOperatorsCached([]common.Address{})

	length := GetActivatedOperatorsLength()
	assert.Equal(t, int64(0), length)

	// Set some operators
	testOperators := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0x0987654321098765432109876543210987654321"),
	}
	SetActivatedOperatorsCached(testOperators)

	length = GetActivatedOperatorsLength()
	assert.Equal(t, int64(2), length)
}

func TestSetActivatedOperatorsCached(t *testing.T) {
	testOperators := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0x0987654321098765432109876543210987654321"),
	}

	SetActivatedOperatorsCached(testOperators)

	operators := GetActivatedOperatorsCached()
	assert.Equal(t, 2, len(operators))
	assert.Equal(t, testOperators[0], operators[0])
	assert.Equal(t, testOperators[1], operators[1])

	// Verify it's a copy - modifying original shouldn't affect cache
	testOperators[0] = common.HexToAddress("0x0000000000000000000000000000000000000000")
	operators = GetActivatedOperatorsCached()
	assert.NotEqual(t, testOperators[0], operators[0])
}

func TestGetActivatedOperatorsUnsafe(t *testing.T) {
	testOperators := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	SetActivatedOperatorsCached(testOperators)

	operators := GetActivatedOperatorsUnsafe()
	assert.Equal(t, 1, len(operators))
}

func TestActivatedOperatorsThreadSafety(t *testing.T) {
	// Reset cache
	SetActivatedOperatorsCached([]common.Address{})

	var wg sync.WaitGroup
	numGoroutines := 10
	iterations := 100

	// Concurrent writes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				addr := common.BigToAddress(big.NewInt(int64(id*iterations + j)))
				SetActivatedOperatorsCached([]common.Address{addr})
			}
		}(i)
	}

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = GetActivatedOperatorsCached()
				_ = GetActivatedOperatorsLength()
			}
		}()
	}

	wg.Wait()
	// If we get here without race condition, test passes
}

// Test CallSmartContract
func TestCallSmartContract_Success(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	// Create a simple ABI
	abiJSON := `[{"constant":true,"inputs":[],"name":"testMethod","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	require.NoError(t, err)

	contractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Pack the method call
	expectedData, err := parsedABI.Pack("testMethod")
	require.NoError(t, err)

	// Create ABI-encoded return value for uint256 (42)
	// uint256 is 32 bytes, so we need to encode 42 as a 32-byte big-endian value
	resultValue := big.NewInt(42)
	// Use the ABI's method to encode the output
	method, exists := parsedABI.Methods["testMethod"]
	require.True(t, exists, "Method should exist in ABI")

	// Encode the return value using the output types
	resultData, err := method.Outputs.Pack(resultValue)
	require.NoError(t, err)

	mockClient.On("CallContract", ctx, ethereum.CallMsg{
		To:   &contractAddress,
		Data: expectedData,
	}, (*big.Int)(nil)).Return(resultData, nil)

	result, err := CallSmartContract(ctx, mockClient, parsedABI, "testMethod", contractAddress)
	require.NoError(t, err)

	// Verify the result is correct
	unpackedResult, ok := result.(*big.Int)
	require.True(t, ok, "Result should be *big.Int")
	assert.Equal(t, resultValue, unpackedResult)

	mockClient.AssertExpectations(t)
}

func TestCallSmartContract_PackError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	// Create ABI with method that requires parameters
	abiJSON := `[{"constant":true,"inputs":[{"name":"x","type":"uint256"}],"name":"testMethod","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	require.NoError(t, err)

	contractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Call without required parameter - should fail on pack
	result, err := CallSmartContract(ctx, mockClient, parsedABI, "testMethod", contractAddress)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to pack data")
}

func TestCallSmartContract_CallContractError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	abiJSON := `[{"constant":true,"inputs":[],"name":"testMethod","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	require.NoError(t, err)

	contractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	expectedData, err := parsedABI.Pack("testMethod")
	require.NoError(t, err)

	mockClient.On("CallContract", ctx, ethereum.CallMsg{
		To:   &contractAddress,
		Data: expectedData,
	}, (*big.Int)(nil)).Return(nil, errors.New("RPC error"))

	result, err := CallSmartContract(ctx, mockClient, parsedABI, "testMethod", contractAddress)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to call contract method")

	mockClient.AssertExpectations(t)
}

func TestCallSmartContract_UnpackError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	abiJSON := `[{"constant":true,"inputs":[],"name":"testMethod","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	require.NoError(t, err)

	contractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	expectedData, err := parsedABI.Pack("testMethod")
	require.NoError(t, err)

	// Return invalid data that can't be unpacked
	invalidData := []byte{0x01, 0x02, 0x03}

	mockClient.On("CallContract", ctx, ethereum.CallMsg{
		To:   &contractAddress,
		Data: expectedData,
	}, (*big.Int)(nil)).Return(invalidData, nil)

	result, err := CallSmartContract(ctx, mockClient, parsedABI, "testMethod", contractAddress)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to unpack result")

	mockClient.AssertExpectations(t)
}

// Test ExecuteTransaction
func TestExecuteTransaction_ChainIDError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	abiJSON := `[{"constant":false,"inputs":[],"name":"testMethod","outputs":[],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	require.NoError(t, err)

	client := &utils.Client{
		ContractABI:     parsedABI,
		ContractAddress: common.HexToAddress("0x1234567890123456789012345678901234567890"),
		PrivateKey:      privateKey,
	}

	// ChainID should be called 3 times (maxRetries) before giving up
	mockClient.On("ChainID", ctx).Return(nil, errors.New("chain ID error")).Times(3)

	tx, auth, err := ExecuteTransaction(ctx, client, mockClient, "testMethod", big.NewInt(0))
	assert.Error(t, err)
	assert.Nil(t, tx)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "failed to fetch chain ID")

	mockClient.AssertExpectations(t)
}

func TestExecuteTransaction_NonceError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	// ChainID succeeds on first attempt
	mockClient.On("ChainID", ctx).Return(chainID, nil).Once()

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	abiJSON := `[{"constant":false,"inputs":[],"name":"testMethod","outputs":[],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	require.NoError(t, err)

	client := &utils.Client{
		ContractABI:     parsedABI,
		ContractAddress: common.HexToAddress("0x1234567890123456789012345678901234567890"),
		PrivateKey:      privateKey,
	}
	mockClient.On("PendingNonceAt", ctx, auth.From).Return(uint64(0), errors.New("nonce error")).Times(3)

	tx, auth, err := ExecuteTransaction(ctx, client, mockClient, "testMethod", big.NewInt(0))
	assert.Error(t, err)
	assert.Nil(t, tx)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "failed to fetch nonce")

	mockClient.AssertExpectations(t)
}

func TestExecuteTransaction_PackError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	mockClient.On("ChainID", ctx).Return(chainID, nil).Once()

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	abiJSON := `[{"constant":false,"inputs":[{"name":"x","type":"uint256"}],"name":"testMethod","outputs":[],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	require.NoError(t, err)

	client := &utils.Client{
		ContractABI:     parsedABI,
		ContractAddress: common.HexToAddress("0x1234567890123456789012345678901234567890"),
		PrivateKey:      privateKey,
	}
	mockClient.On("PendingNonceAt", ctx, auth.From).Return(uint64(0), nil).Once()

	tx, auth, err := ExecuteTransaction(ctx, client, mockClient, "testMethod", big.NewInt(0))
	assert.Error(t, err)
	assert.Nil(t, tx)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "failed to pack data")

	mockClient.AssertExpectations(t)
}

func TestExecuteTransaction_GasEstimationError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	// ChainID succeeds on first attempt
	mockClient.On("ChainID", ctx).Return(chainID, nil).Once()

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	abiJSON := `[{"constant":false,"inputs":[],"name":"testMethod","outputs":[],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	require.NoError(t, err)

	client := &utils.Client{
		ContractABI:     parsedABI,
		ContractAddress: common.HexToAddress("0x1234567890123456789012345678901234567890"),
		PrivateKey:      privateKey,
	}
	mockClient.On("PendingNonceAt", ctx, auth.From).Return(uint64(0), nil).Once()

	packedData, err := client.ContractABI.Pack("testMethod")
	require.NoError(t, err)

	callMsg := ethereum.CallMsg{
		From:  auth.From,
		To:    &client.ContractAddress,
		Data:  packedData,
		Value: big.NewInt(0),
	}

	// Gas estimation fails 3 times
	mockClient.On("EstimateGas", ctx, callMsg).Return(uint64(0), errors.New("gas estimation failed")).Times(3)

	tx, auth, err := ExecuteTransaction(ctx, client, mockClient, "testMethod", big.NewInt(0))
	assert.Error(t, err)
	assert.Nil(t, tx)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "gas estimation failed after")

	mockClient.AssertExpectations(t)
}

// Test waitForTransactionSuccess
func TestWaitForTransactionSuccess_Success(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	toAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	txData := &types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     0,
		To:        &toAddr,
		Value:     big.NewInt(0),
		Gas:       21000,
		GasFeeCap: big.NewInt(1000000000),
		GasTipCap: big.NewInt(1000000000),
		Data:      []byte{},
	}
	tx := types.NewTx(txData)
	signedTx, err := auth.Signer(auth.From, tx)
	require.NoError(t, err)

	receipt := &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		BlockNumber: big.NewInt(1),
	}

	mockClient.On("TransactionReceipt", ctx, signedTx).Return(receipt, nil).Once()

	result, err := waitForTransactionSuccess(ctx, mockClient, signedTx, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, receipt, result)

	mockClient.AssertExpectations(t)
}

func TestWaitForTransactionSuccess_Failed(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	toAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	txData := &types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     0,
		To:        &toAddr,
		Value:     big.NewInt(0),
		Gas:       21000,
		GasFeeCap: big.NewInt(1000000000),
		GasTipCap: big.NewInt(1000000000),
		Data:      []byte{},
	}
	tx := types.NewTx(txData)
	signedTx, err := auth.Signer(auth.From, tx)
	require.NoError(t, err)

	receipt := &types.Receipt{
		Status:      types.ReceiptStatusFailed,
		BlockNumber: big.NewInt(1),
		GasUsed:     21000,
	}

	mockClient.On("TransactionReceipt", ctx, signedTx).Return(receipt, nil).Once()

	result, err := waitForTransactionSuccess(ctx, mockClient, signedTx, 5*time.Second)
	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.True(t, errors.Is(err, ErrTransactionFailed))
	assert.Contains(t, err.Error(), "reverted")

	mockClient.AssertExpectations(t)
}

func TestWaitForTransactionSuccess_NotFound(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	toAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	txData := &types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     0,
		To:        &toAddr,
		Value:     big.NewInt(0),
		Gas:       21000,
		GasFeeCap: big.NewInt(1000000000),
		GasTipCap: big.NewInt(1000000000),
		Data:      []byte{},
	}
	tx := types.NewTx(txData)
	signedTx, err := auth.Signer(auth.From, tx)
	require.NoError(t, err)

	// Return NotFound error multiple times, then timeout
	mockClient.On("TransactionReceipt", ctx, signedTx).Return(nil, ethereum.NotFound).Maybe()

	result, err := waitForTransactionSuccess(ctx, mockClient, signedTx, 1*time.Second)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "stuck in mempool")
}

func TestWaitForTransactionSuccess_ContextCancelled(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx, cancel := context.WithCancel(context.Background())

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	toAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	txData := &types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     0,
		To:        &toAddr,
		Value:     big.NewInt(0),
		Gas:       21000,
		GasFeeCap: big.NewInt(1000000000),
		GasTipCap: big.NewInt(1000000000),
		Data:      []byte{},
	}
	tx := types.NewTx(txData)
	signedTx, err := auth.Signer(auth.From, tx)
	require.NoError(t, err)

	// Cancel context immediately
	cancel()

	mockClient.On("TransactionReceipt", ctx, signedTx).Return(nil, ethereum.NotFound).Maybe()

	result, err := waitForTransactionSuccess(ctx, mockClient, signedTx, 5*time.Second)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "context cancelled")
}

// Test sendWithRetry - this is complex, so we'll test key scenarios
func TestSendWithRetry_SuggestGasTipCapError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)
	mockClient.On("SuggestGasTipCap", ctx).Return(nil, errors.New("tip cap error")).Times(3)

	receipt, tx, err := sendWithRetry(ctx, mockClient, chainID, auth, auth.From, big.NewInt(0), []byte{})
	assert.Error(t, err)
	assert.Nil(t, receipt)
	assert.Nil(t, tx)
	assert.Contains(t, err.Error(), "failed to suggest tip cap after 3 attempts")

	mockClient.AssertExpectations(t)
}

func TestSendWithRetry_SuggestGasPriceError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	mockClient.On("SuggestGasTipCap", ctx).Return(big.NewInt(1000000000), nil)
	mockClient.On("SuggestGasPrice", ctx).Return(nil, errors.New("gas price error")).Times(3)

	receipt, tx, err := sendWithRetry(ctx, mockClient, chainID, auth, auth.From, big.NewInt(0), []byte{})
	assert.Error(t, err)
	assert.Nil(t, receipt)
	assert.Nil(t, tx)
	assert.Contains(t, err.Error(), "failed to suggest gas price after 3 attempts")

	mockClient.AssertExpectations(t)
}

func TestSendWithRetry_PendingNonceAtError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	mockClient.On("SuggestGasTipCap", ctx).Return(big.NewInt(1000000000), nil)
	mockClient.On("SuggestGasPrice", ctx).Return(big.NewInt(1000000000), nil)
	mockClient.On("PendingNonceAt", ctx, auth.From).Return(uint64(0), errors.New("nonce error")).Times(5)

	receipt, tx, err := sendWithRetry(ctx, mockClient, chainID, auth, auth.From, big.NewInt(0), []byte{})
	assert.Error(t, err)
	assert.Nil(t, receipt)
	assert.Nil(t, tx)
	// Should return error after maxRetries
	assert.Contains(t, err.Error(), "failed to get nonce after 5 retries")
	assert.Contains(t, err.Error(), "nonce error")

	mockClient.AssertExpectations(t)
}

func TestSendWithRetry_EstimateGasError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(1337)
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	mockClient.On("SuggestGasTipCap", ctx).Return(big.NewInt(1000000000), nil)
	mockClient.On("SuggestGasPrice", ctx).Return(big.NewInt(1000000000), nil)
	mockClient.On("PendingNonceAt", ctx, auth.From).Return(uint64(0), nil).Times(5)

	callMsg := ethereum.CallMsg{
		From:  auth.From,
		To:    &auth.From,
		Data:  []byte{},
		Value: big.NewInt(0),
	}
	mockClient.On("EstimateGas", ctx, callMsg).Return(uint64(0), errors.New("gas estimate error")).Times(5)

	receipt, tx, err := sendWithRetry(ctx, mockClient, chainID, auth, auth.From, big.NewInt(0), []byte{})
	assert.Error(t, err)
	assert.Nil(t, receipt)
	assert.Nil(t, tx)
	assert.Contains(t, err.Error(), "failed to estimate gas after 5 retries")
	assert.Contains(t, err.Error(), "gas estimate error")

	mockClient.AssertExpectations(t)
}


func TestGetActivatedOperators_CallContractError(t *testing.T) {
	// Test error when CallContract fails
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	// Set contract address
	originalContractAddr := os.Getenv("CONTRACT_ADDRESS")
	defer func() {
		if originalContractAddr != "" {
			os.Setenv("CONTRACT_ADDRESS", originalContractAddr)
		} else {
			os.Unsetenv("CONTRACT_ADDRESS")
		}
	}()
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")

	contractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Load ABI to get the method signature
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	expectedData, err := parsedABI.Pack("getActivatedOperators")
	require.NoError(t, err)

	// Mock CallContract to return error
	mockClient.On("CallContract", ctx, ethereum.CallMsg{
		To:   &contractAddress,
		Data: expectedData,
	}, (*big.Int)(nil)).Return(nil, errors.New("RPC error"))

	operators, err := GetActivatedOperators(ctx, mockClient)
	assert.Error(t, err)
	assert.Nil(t, operators)
	assert.Contains(t, err.Error(), "failed to call contract method getActivatedOperators")

	mockClient.AssertExpectations(t)
}

func TestUpdateActivatedOperators_Error(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	// Reset cache first
	SetActivatedOperatorsCached([]common.Address{})

	// Set contract address (required to avoid log.Fatal)
	originalContractAddr := os.Getenv("CONTRACT_ADDRESS")
	defer func() {
		if originalContractAddr != "" {
			os.Setenv("CONTRACT_ADDRESS", originalContractAddr)
		} else {
			os.Unsetenv("CONTRACT_ADDRESS")
		}
	}()
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")

	contractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Load ABI - use relative path like production code does
	// The setup ensures the file exists relative to project root
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	expectedData, err := parsedABI.Pack("getActivatedOperators")
	require.NoError(t, err)

	// Mock CallContract to return error - this tests the error handling path
	mockClient.On("CallContract", ctx, ethereum.CallMsg{
		To:   &contractAddress,
		Data: expectedData,
	}, (*big.Int)(nil)).Return(nil, errors.New("RPC error"))

	// UpdateActivatedOperators should handle the error gracefully
	UpdateActivatedOperators(ctx, mockClient)

	// Cache should remain empty
	operators := GetActivatedOperatorsCached()
	assert.Equal(t, 0, len(operators))

	mockClient.AssertExpectations(t)
}

func TestUpdateActivatedOperators_Success(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	// Reset cache first
	SetActivatedOperatorsCached([]common.Address{})

	// Set contract address
	originalContractAddr := os.Getenv("CONTRACT_ADDRESS")
	defer func() {
		if originalContractAddr != "" {
			os.Setenv("CONTRACT_ADDRESS", originalContractAddr)
		} else {
			os.Unsetenv("CONTRACT_ADDRESS")
		}
	}()
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")

	contractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Load ABI
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	expectedData, err := parsedABI.Pack("getActivatedOperators")
	require.NoError(t, err)

	// Create test operators
	testOperators := []common.Address{
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
	}

	// Encode the return value
	method, exists := parsedABI.Methods["getActivatedOperators"]
	require.True(t, exists)
	resultData, err := method.Outputs.Pack(testOperators)
	require.NoError(t, err)

	mockClient.On("CallContract", ctx, ethereum.CallMsg{
		To:   &contractAddress,
		Data: expectedData,
	}, (*big.Int)(nil)).Return(resultData, nil)

	UpdateActivatedOperators(ctx, mockClient)

	// Cache should be updated
	operators := GetActivatedOperatorsCached()
	assert.Equal(t, 2, len(operators))
	assert.Equal(t, testOperators[0], operators[0])
	assert.Equal(t, testOperators[1], operators[1])

	mockClient.AssertExpectations(t)
}

func TestUpdateCurrentRoundFromContract_ConfigError(t *testing.T) {
	// Save original contract address
	originalContractAddr := os.Getenv("CONTRACT_ADDRESS")
	defer func() {
		if originalContractAddr != "" {
			os.Setenv("CONTRACT_ADDRESS", originalContractAddr)
		} else {
			os.Unsetenv("CONTRACT_ADDRESS")
		}
	}()

	// Unset contract address
	os.Unsetenv("CONTRACT_ADDRESS")

	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	round, err := UpdateCurrentRoundFromContract(ctx, mockClient)
	assert.Error(t, err)
	assert.Nil(t, round)
	assert.Contains(t, err.Error(), "CONTRACT_ADDRESS is not set")
}

func TestUpdateCurrentRoundFromContract_CallContractError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	// Set contract address
	originalContractAddr := os.Getenv("CONTRACT_ADDRESS")
	defer func() {
		if originalContractAddr != "" {
			os.Setenv("CONTRACT_ADDRESS", originalContractAddr)
		} else {
			os.Unsetenv("CONTRACT_ADDRESS")
		}
	}()
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")

	contractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Load ABI
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	expectedData, err := parsedABI.Pack("s_currentRound")
	require.NoError(t, err)

	// Mock CallContract to return error
	mockClient.On("CallContract", ctx, ethereum.CallMsg{
		To:   &contractAddress,
		Data: expectedData,
	}, (*big.Int)(nil)).Return(nil, errors.New("RPC error"))

	round, err := UpdateCurrentRoundFromContract(ctx, mockClient)
	assert.Error(t, err)
	assert.Nil(t, round)
	assert.Contains(t, err.Error(), "failed to call s_currentRound")

	mockClient.AssertExpectations(t)
}

func TestGetTrialNumFromContract_ConfigError(t *testing.T) {
	// Save original contract address
	originalContractAddr := os.Getenv("CONTRACT_ADDRESS")
	defer func() {
		if originalContractAddr != "" {
			os.Setenv("CONTRACT_ADDRESS", originalContractAddr)
		} else {
			os.Unsetenv("CONTRACT_ADDRESS")
		}
	}()

	// Unset contract address
	os.Unsetenv("CONTRACT_ADDRESS")

	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	trialNum, err := GetTrialNumFromContract(ctx, mockClient, big.NewInt(1))
	assert.Error(t, err)
	assert.Nil(t, trialNum)
	assert.Contains(t, err.Error(), "CONTRACT_ADDRESS is not set")
}

func TestGetTrialNumFromContract_CallContractError(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	// Set contract address
	originalContractAddr := os.Getenv("CONTRACT_ADDRESS")
	defer func() {
		if originalContractAddr != "" {
			os.Setenv("CONTRACT_ADDRESS", originalContractAddr)
		} else {
			os.Unsetenv("CONTRACT_ADDRESS")
		}
	}()
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")

	contractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")
	round := big.NewInt(1)

	// Load ABI
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	expectedData, err := parsedABI.Pack("s_trialNum", round)
	require.NoError(t, err)

	// Mock CallContract to return error
	mockClient.On("CallContract", ctx, ethereum.CallMsg{
		To:   &contractAddress,
		Data: expectedData,
	}, (*big.Int)(nil)).Return(nil, errors.New("RPC error"))

	trialNum, err := GetTrialNumFromContract(ctx, mockClient, round)
	assert.Error(t, err)
	assert.Nil(t, trialNum)
	assert.Contains(t, err.Error(), "failed to call s_trialNum")

	mockClient.AssertExpectations(t)
}

// Test DefaultEthService implementation
func TestDefaultEthService(t *testing.T) {
	service := NewDefaultEthService()
	assert.NotNil(t, service)

	// Test GetActivatedOperatorsCached
	SetActivatedOperatorsCached([]common.Address{})
	operators := service.GetActivatedOperatorsCached()
	assert.NotNil(t, operators)

	// Test GetActivatedOperatorsLength
	length := service.GetActivatedOperatorsLength()
	assert.Equal(t, int64(0), length)

	// Test SetActivatedOperatorsCached
	testOperators := []common.Address{common.HexToAddress("0x1234567890123456789012345678901234567890")}
	service.SetActivatedOperatorsCached(testOperators)
	operators = service.GetActivatedOperatorsCached()
	assert.Equal(t, 1, len(operators))

	// Test GetActivatedOperatorsUnsafe
	operators = service.GetActivatedOperatorsUnsafe()
	assert.Equal(t, 1, len(operators))
}

// TestExecuteTransaction_SequentialExecution verifies that concurrent ExecuteTransaction
// calls are serialized by the txMu mutex (no concurrent execution)
func TestExecuteTransaction_SequentialExecution(t *testing.T) {
	numTransactions := 3
	txDelay := 100 * time.Millisecond // Simulated transaction processing time

	// Track concurrent executions using atomic counter
	var activeCount int32
	var maxConcurrent int32
	var maxConcurrentMu sync.Mutex

	var wg sync.WaitGroup

	// Launch multiple goroutines that call ExecuteTransaction concurrently
	wg.Add(numTransactions)
	for i := 0; i < numTransactions; i++ {
		go func(txID int) {
			defer wg.Done()

			mockClient := new(MockFallbackEthClient)
			ctx := context.Background()

			privateKey, err := crypto.GenerateKey()
			require.NoError(t, err)

			// Setup mock - ChainID will track concurrent execution
			// ExecuteTransaction retries ChainID up to 3 times
			mockClient.On("ChainID", ctx).Run(func(args mock.Arguments) {
				// Increment counter when entering critical section
				current := atomic.AddInt32(&activeCount, 1)

				// Track maximum concurrent count
				maxConcurrentMu.Lock()
				if current > maxConcurrent {
					maxConcurrent = current
				}
				maxConcurrentMu.Unlock()

				// Simulate some work
				time.Sleep(txDelay)

				// Decrement counter when leaving
				atomic.AddInt32(&activeCount, -1)
			}).Return(nil, errors.New("simulated error")).Times(3)

			abiJSON := `[{"constant":false,"inputs":[],"name":"testMethod","outputs":[],"type":"function"}]`
			parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
			require.NoError(t, err)

			client := &utils.Client{
				ContractABI:     parsedABI,
				ContractAddress: common.HexToAddress("0x1234567890123456789012345678901234567890"),
				PrivateKey:      privateKey,
			}

			// Execute transaction (will fail on first ChainID call)
			_, _, _ = ExecuteTransaction(ctx, client, mockClient, "testMethod", big.NewInt(0))
		}(i)
	}

	wg.Wait()

	// With mutex protection, maxConcurrent should be exactly 1
	// If mutex wasn't working, multiple goroutines would be inside simultaneously
	assert.Equal(t, int32(1), maxConcurrent,
		"Expected max concurrent count of 1 (sequential execution), but got %d", maxConcurrent)

	t.Logf("Max concurrent executions: %d (expected 1 for sequential execution)", maxConcurrent)
}

// TestExecuteTransaction_MutexNotHeldOnReturn verifies that the mutex is released
// even when ExecuteTransaction returns an error
func TestExecuteTransaction_MutexNotHeldOnReturn(t *testing.T) {
	mockClient := new(MockFallbackEthClient)
	ctx := context.Background()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	abiJSON := `[{"constant":false,"inputs":[],"name":"testMethod","outputs":[],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	require.NoError(t, err)

	client := &utils.Client{
		ContractABI:     parsedABI,
		ContractAddress: common.HexToAddress("0x1234567890123456789012345678901234567890"),
		PrivateKey:      privateKey,
	}

	// First call - should fail
	mockClient.On("ChainID", ctx).Return(nil, errors.New("error")).Times(3)
	_, _, err = ExecuteTransaction(ctx, client, mockClient, "testMethod", big.NewInt(0))
	assert.Error(t, err)

	// Second call should not deadlock - mutex should have been released
	done := make(chan bool, 1)
	go func() {
		mockClient2 := new(MockFallbackEthClient)
		mockClient2.On("ChainID", ctx).Return(nil, errors.New("error")).Times(3)
		_, _, _ = ExecuteTransaction(ctx, client, mockClient2, "testMethod", big.NewInt(0))
		done <- true
	}()

	select {
	case <-done:
		// Success - mutex was released
	case <-time.After(5 * time.Second):
		t.Fatal("Deadlock detected - mutex was not released after error")
	}
}

package fallback_ethclient

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tokamak-network/DRB-node/logger"
)

// EthClientInterface defines the interface for Ethereum client operations
type EthClientInterface interface {
	CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	NetworkID(ctx context.Context) (*big.Int, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error)
	Close()
}

// MockEthClient is a mock implementation of the EthClientInterface
type MockEthClient struct {
	mock.Mock
}

func (m *MockEthClient) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	args := m.Called(ctx, msg, blockNumber)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockEthClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	args := m.Called(ctx, tx)
	return args.Error(0)
}

func (m *MockEthClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	args := m.Called(ctx, txHash)
	return args.Get(0).(*types.Receipt), args.Error(1)
}

func (m *MockEthClient) NetworkID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockEthClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	args := m.Called(ctx, account)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockEthClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockEthClient) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	args := m.Called(ctx, msg)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockEthClient) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	args := m.Called(ctx, q, ch)
	return args.Get(0).(ethereum.Subscription), args.Error(1)
}

func (m *MockEthClient) Close() {
	m.Called()
}

// TestFallbackRPCClient is a test-specific implementation of FallbackRPCClient
type TestFallbackRPCClient struct {
	clients    []EthClientInterface
	urls       []string
	currentIdx int
	mu         sync.RWMutex
	maxRetries int
	retryDelay time.Duration
	logger     *logrus.Logger
}

func (f *TestFallbackRPCClient) getCurrentClient() EthClientInterface {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.clients[f.currentIdx]
}

func (f *TestFallbackRPCClient) switchToNextClient() {
	f.mu.Lock()
	defer f.mu.Unlock()

	oldIdx := f.currentIdx
	f.currentIdx = (f.currentIdx + 1) % len(f.clients)

	f.logger.WithFields(logrus.Fields{
		"old_url": f.urls[oldIdx],
		"new_url": f.urls[f.currentIdx],
	}).Info("Switching to fallback RPC")
}

func (f *TestFallbackRPCClient) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		result, err := client.CallContract(ctx, msg, blockNumber)
		if err == nil {
			return result, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC call failed, switching to fallback")
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

func (f *TestFallbackRPCClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		err := client.SendTransaction(ctx, tx)
		if err == nil {
			return nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC send transaction failed, switching to fallback")
		f.switchToNextClient()
	}
	return fmt.Errorf("all RPCs failed: %v", lastErr)
}

func (f *TestFallbackRPCClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil {
			return receipt, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC get receipt failed, switching to fallback")
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

func (f *TestFallbackRPCClient) NetworkID(ctx context.Context) (*big.Int, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		id, err := client.NetworkID(ctx)
		if err == nil {
			return id, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC get network ID failed, switching to fallback")
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

func (f *TestFallbackRPCClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		nonce, err := client.PendingNonceAt(ctx, account)
		if err == nil {
			return nonce, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC get nonce failed, switching to fallback")
		f.switchToNextClient()
	}
	return 0, fmt.Errorf("all RPCs failed: %v", lastErr)
}

func (f *TestFallbackRPCClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		price, err := client.SuggestGasPrice(ctx)
		if err == nil {
			return price, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC get gas price failed, switching to fallback")
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

func (f *TestFallbackRPCClient) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		gas, err := client.EstimateGas(ctx, msg)
		if err == nil {
			return gas, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC estimate gas failed, switching to fallback")
		f.switchToNextClient()
	}
	return 0, fmt.Errorf("all RPCs failed: %v", lastErr)
}

func (f *TestFallbackRPCClient) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		sub, err := client.SubscribeFilterLogs(ctx, q, ch)
		if err == nil {
			return sub, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC subscribe filter logs failed, switching to fallback")
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

func (f *TestFallbackRPCClient) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, client := range f.clients {
		client.Close()
	}
}

func TestNewFallbackRPCClient(t *testing.T) {
	// Initialize logger for testing
	logger.Log = logrus.New()

	tests := []struct {
		name    string
		urls    []string
		wantErr bool
	}{
		{
			name:    "empty urls",
			urls:    []string{},
			wantErr: true,
		},
		{
			name:    "valid urls",
			urls:    []string{"http://localhost:8545"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewFallbackRPCClient(tt.urls)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestFallbackRPCClient_CallContract(t *testing.T) {
	ctx := context.Background()
	msg := ethereum.CallMsg{}
	blockNumber := big.NewInt(1)
	expectedResult := []byte("test")

	mockClient1 := new(MockEthClient)
	mockClient2 := new(MockEthClient)

	// First client fails, second succeeds
	mockClient1.On("CallContract", ctx, msg, blockNumber).Return([]byte{}, errors.New("failed"))
	mockClient2.On("CallContract", ctx, msg, blockNumber).Return(expectedResult, nil)

	client := &TestFallbackRPCClient{
		clients:    []EthClientInterface{mockClient1, mockClient2},
		urls:       []string{"url1", "url2"},
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second,
		logger:     logrus.New(),
	}

	result, err := client.CallContract(ctx, msg, blockNumber)
	assert.NoError(t, err)
	assert.Equal(t, expectedResult, result)
	mockClient1.AssertExpectations(t)
	mockClient2.AssertExpectations(t)
}

func TestFallbackRPCClient_SendTransaction(t *testing.T) {
	ctx := context.Background()
	tx := &types.Transaction{}

	mockClient1 := new(MockEthClient)
	mockClient2 := new(MockEthClient)

	// First client fails, second succeeds
	mockClient1.On("SendTransaction", ctx, tx).Return(errors.New("failed"))
	mockClient2.On("SendTransaction", ctx, tx).Return(nil)

	client := &TestFallbackRPCClient{
		clients:    []EthClientInterface{mockClient1, mockClient2},
		urls:       []string{"url1", "url2"},
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second,
		logger:     logrus.New(),
	}

	err := client.SendTransaction(ctx, tx)
	assert.NoError(t, err)
	mockClient1.AssertExpectations(t)
	mockClient2.AssertExpectations(t)
}

func TestFallbackRPCClient_TransactionReceipt(t *testing.T) {
	ctx := context.Background()
	txHash := common.HexToHash("0x123")
	expectedReceipt := &types.Receipt{}

	mockClient1 := new(MockEthClient)
	mockClient2 := new(MockEthClient)

	// First client fails, second succeeds
	mockClient1.On("TransactionReceipt", ctx, txHash).Return(&types.Receipt{}, errors.New("failed"))
	mockClient2.On("TransactionReceipt", ctx, txHash).Return(expectedReceipt, nil)

	client := &TestFallbackRPCClient{
		clients:    []EthClientInterface{mockClient1, mockClient2},
		urls:       []string{"url1", "url2"},
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second,
		logger:     logrus.New(),
	}

	receipt, err := client.TransactionReceipt(ctx, txHash)
	assert.NoError(t, err)
	assert.Equal(t, expectedReceipt, receipt)
	mockClient1.AssertExpectations(t)
	mockClient2.AssertExpectations(t)
}

func TestFallbackRPCClient_NetworkID(t *testing.T) {
	ctx := context.Background()
	expectedID := big.NewInt(1)

	mockClient1 := new(MockEthClient)
	mockClient2 := new(MockEthClient)

	// First client fails, second succeeds
	mockClient1.On("NetworkID", ctx).Return(big.NewInt(0), errors.New("failed"))
	mockClient2.On("NetworkID", ctx).Return(expectedID, nil)

	client := &TestFallbackRPCClient{
		clients:    []EthClientInterface{mockClient1, mockClient2},
		urls:       []string{"url1", "url2"},
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second,
		logger:     logrus.New(),
	}

	id, err := client.NetworkID(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expectedID, id)
	mockClient1.AssertExpectations(t)
	mockClient2.AssertExpectations(t)
}

func TestFallbackRPCClient_PendingNonceAt(t *testing.T) {
	ctx := context.Background()
	account := common.HexToAddress("0x123")
	expectedNonce := uint64(5)

	mockClient1 := new(MockEthClient)
	mockClient2 := new(MockEthClient)

	// First client fails, second succeeds
	mockClient1.On("PendingNonceAt", ctx, account).Return(uint64(0), errors.New("failed"))
	mockClient2.On("PendingNonceAt", ctx, account).Return(expectedNonce, nil)

	client := &TestFallbackRPCClient{
		clients:    []EthClientInterface{mockClient1, mockClient2},
		urls:       []string{"url1", "url2"},
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second,
		logger:     logrus.New(),
	}

	nonce, err := client.PendingNonceAt(ctx, account)
	assert.NoError(t, err)
	assert.Equal(t, expectedNonce, nonce)
	mockClient1.AssertExpectations(t)
	mockClient2.AssertExpectations(t)
}

func TestFallbackRPCClient_SuggestGasPrice(t *testing.T) {
	ctx := context.Background()
	expectedPrice := big.NewInt(1000000000)

	mockClient1 := new(MockEthClient)
	mockClient2 := new(MockEthClient)

	// First client fails, second succeeds
	mockClient1.On("SuggestGasPrice", ctx).Return(big.NewInt(0), errors.New("failed"))
	mockClient2.On("SuggestGasPrice", ctx).Return(expectedPrice, nil)

	client := &TestFallbackRPCClient{
		clients:    []EthClientInterface{mockClient1, mockClient2},
		urls:       []string{"url1", "url2"},
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second,
		logger:     logrus.New(),
	}

	price, err := client.SuggestGasPrice(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expectedPrice, price)
	mockClient1.AssertExpectations(t)
	mockClient2.AssertExpectations(t)
}

func TestFallbackRPCClient_EstimateGas(t *testing.T) {
	ctx := context.Background()
	msg := ethereum.CallMsg{}
	expectedGas := uint64(21000)

	mockClient1 := new(MockEthClient)
	mockClient2 := new(MockEthClient)

	// First client fails, second succeeds
	mockClient1.On("EstimateGas", ctx, msg).Return(uint64(0), errors.New("failed"))
	mockClient2.On("EstimateGas", ctx, msg).Return(expectedGas, nil)

	client := &TestFallbackRPCClient{
		clients:    []EthClientInterface{mockClient1, mockClient2},
		urls:       []string{"url1", "url2"},
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second,
		logger:     logrus.New(),
	}

	gas, err := client.EstimateGas(ctx, msg)
	assert.NoError(t, err)
	assert.Equal(t, expectedGas, gas)
	mockClient1.AssertExpectations(t)
	mockClient2.AssertExpectations(t)
}

func TestFallbackRPCClient_Close(t *testing.T) {
	mockClient1 := new(MockEthClient)
	mockClient2 := new(MockEthClient)

	mockClient1.On("Close").Return()
	mockClient2.On("Close").Return()

	client := &TestFallbackRPCClient{
		clients:    []EthClientInterface{mockClient1, mockClient2},
		urls:       []string{"url1", "url2"},
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second,
		logger:     logrus.New(),
	}

	client.Close()
	mockClient1.AssertExpectations(t)
	mockClient2.AssertExpectations(t)
}

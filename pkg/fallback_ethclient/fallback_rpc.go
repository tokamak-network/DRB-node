package fallback_ethclient

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

// FallbackRPCClient implements a fallback mechanism for Ethereum RPC calls
type FallbackRPCClient struct {
	clients    []*ethclient.Client
	urls       []string
	currentIdx int
	mu         sync.RWMutex
	maxRetries int
	retryDelay time.Duration
	logger     *logrus.Logger
}

var rpcURLPrinted int32 // 0 = false, 1 = true (atomic)

// NewFallbackRPCClient creates a new FallbackRPCClient with the given RPC URLs
func NewFallbackRPCClient(urls []string) (*FallbackRPCClient, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("at least one RPC URL is required")
	}

	l := logger.Log

	if l == nil {
		return nil, errors.New("logger not found")
	}

	validURLs := make([]string, 0, len(urls))
	validClients := make([]*ethclient.Client, 0, len(urls))
	for _, url := range urls {
		client, err := ethclient.Dial(url)
		if err != nil {
			l.WithError(err).Error("failed to connect to RPC", "url", url)
			continue
		}

		// Test if the RPC is reachable
		_, err = client.BlockNumber(context.Background())
		if err != nil {
			l.WithError(err).Error("failed to connect to RPC", "url", url)
			continue
		}

		validClients = append(validClients, client)
		validURLs = append(validURLs, url)
	}

	if len(validClients) == 0 {
		return nil, errors.New("failed to connect to any RPC")
	}

	return &FallbackRPCClient{
		clients:    validClients,
		urls:       validURLs,
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second * 2,
		logger:     l,
	}, nil
}

// switchToNextClient switches to the next available RPC client
func (f *FallbackRPCClient) switchToNextClient() {
	f.mu.Lock()
	defer f.mu.Unlock()

	oldIdx := f.currentIdx
	f.currentIdx = (f.currentIdx + 1) % len(f.clients)

	f.logger.WithFields(logrus.Fields{
		"old_url": f.urls[oldIdx],
		"new_url": f.urls[f.currentIdx],
	}).Info("Switching to fallback RPC")
	atomic.StoreInt32(&rpcURLPrinted, 0)
}

// getCurrentClient returns the current active client
func (f *FallbackRPCClient) getCurrentClient() *ethclient.Client {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if atomic.LoadInt32(&rpcURLPrinted) == 0 {
		atomic.StoreInt32(&rpcURLPrinted, 1)
	}

	return f.clients[f.currentIdx]
}

// CallContract implements the ethereum.ContractCaller interface
func (f *FallbackRPCClient) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
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

// SendTransaction implements the ethereum.ContractTransactor interface
func (f *FallbackRPCClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		err := client.SendTransaction(ctx, tx)
		if err == nil {
			return nil
		}
		lastErr = err

		// Don't switch RPC for replacement transaction errors as these are client-side issues
		if utils.IsReplacementError(err) {
			f.logger.WithError(err).Warn("Replacement transaction error - not switching RPC")
			return err
		}

		f.logger.WithError(err).Warn("RPC send transaction failed, switching to fallback")
		f.switchToNextClient()
	}
	return fmt.Errorf("all RPCs failed: %v", lastErr)
}

// TransactionReceipt implements the ethereum.ContractTransactor interface
func (f *FallbackRPCClient) TransactionReceipt(ctx context.Context, signedTx *types.Transaction) (*types.Receipt, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		receipt, err := client.TransactionReceipt(ctx, signedTx.Hash())
		if err == nil {
			return receipt, nil
		}
		if err.Error() == "not found" {
			return nil, ethereum.NotFound
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC get receipt failed, switching to fallback")
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

// NetworkID implements the ethereum.ContractTransactor interface
func (f *FallbackRPCClient) NetworkID(ctx context.Context) (*big.Int, error) {
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

// PendingNonceAt implements the ethereum.ContractTransactor interface
func (f *FallbackRPCClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
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

// SuggestGasPrice implements the ethereum.ContractTransactor interface
func (f *FallbackRPCClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
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

// SuggestGasTipCap implements the ethereum.ContractTransactor interface
func (f *FallbackRPCClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		price, err := client.SuggestGasTipCap(ctx)
		if err == nil {
			return price, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC get gas tip cap failed, switching to fallback")
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

// EstimateGas implements the ethereum.ContractTransactor interface
func (f *FallbackRPCClient) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
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
	return 0, lastErr
}

func (f *FallbackRPCClient) ChainID(ctx context.Context) (*big.Int, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		id, err := client.ChainID(ctx)
		if err == nil {
			return id, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC get chain ID failed, switching to fallback")
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

// SubscribeFilterLogs implements the ethereum.ContractTransactor interface
func (f *FallbackRPCClient) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
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

// BalanceAt implements the ethereum.ContractCaller interface
func (f *FallbackRPCClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		balance, err := client.BalanceAt(ctx, account, blockNumber)
		if err == nil {
			return balance, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC get balance failed, switching to fallback")
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

// BlockTimestamp gets the timestamp of a specific block
func (f *FallbackRPCClient) BlockTimestamp(ctx context.Context, blockNumber *big.Int) (uint64, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		header, err := client.HeaderByNumber(ctx, blockNumber)
		if err == nil {
			return header.Time, nil
		}
		lastErr = err
		f.logger.WithError(err).Warn("RPC get block header failed, switching to fallback")
		f.switchToNextClient()
	}
	return 0, fmt.Errorf("all RPCs failed: %v", lastErr)
}

// Close closes all RPC client connections
func (f *FallbackRPCClient) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, client := range f.clients {
		client.Close()
	}
}

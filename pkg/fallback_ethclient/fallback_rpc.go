package fallback_ethclient

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tokamak-network/DRB-node/logger"
)

// FallbackRPCClient implements a fallback mechanism for Ethereum RPC calls
type FallbackRPCClient struct {
	clients    []*ethclient.Client
	urls       []string
	currentIdx int
	mu         sync.RWMutex
	maxRetries int
	retryDelay time.Duration
}

var rpcURLPrinted = false

// NewFallbackRPCClient creates a new FallbackRPCClient with the given RPC URLs
func NewFallbackRPCClient(urls []string) (*FallbackRPCClient, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("at least one RPC URL is required")
	}

	clients := make([]*ethclient.Client, len(urls))
	for i, url := range urls {
		client, err := ethclient.Dial(url)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to RPC %s: %v", url, err)
		}
		clients[i] = client
	}

	return &FallbackRPCClient{
		clients:    clients,
		urls:       urls,
		currentIdx: 0,
		maxRetries: 3,
		retryDelay: time.Second * 2,
	}, nil
}

// switchToNextClient switches to the next available RPC client
func (f *FallbackRPCClient) switchToNextClient() {
	f.mu.Lock()
	defer f.mu.Unlock()

	oldIdx := f.currentIdx
	f.currentIdx = (f.currentIdx + 1) % len(f.clients)

	logger.Infof("Switching to fallback RPC, old_url: %s, new_url: %s", f.urls[oldIdx], f.urls[f.currentIdx])
	rpcURLPrinted = false
}

// getCurrentClient returns the current active client
func (f *FallbackRPCClient) getCurrentClient() *ethclient.Client {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if !rpcURLPrinted {
		rpcURLPrinted = true
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
		logger.Errorf("RPC call failed, switching to fallback, err: %v", err)
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
		logger.Errorf("RPC send transaction failed, switching to fallback, err: %v", err)
		f.switchToNextClient()
	}
	return fmt.Errorf("all RPCs failed: %v", lastErr)
}

// TransactionReceipt implements the ethereum.ContractTransactor interface
func (f *FallbackRPCClient) TransactionReceipt(ctx context.Context, signedTx *types.Transaction) (*types.Receipt, error) {
	var lastErr error
	for i := 0; i < len(f.clients); i++ {
		client := f.getCurrentClient()
		receipt, err := bind.WaitMined(ctx, client, signedTx)
		if err == nil {
			return receipt, nil
		}
		lastErr = err
		logger.Errorf("RPC get receipt failed, switching to fallback, err: %v", err)
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
		logger.Errorf("RPC get network ID failed, switching to fallback, err: %v", err)
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
		logger.Errorf("RPC get nonce failed, switching to fallback, err: %v", err)
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
		logger.Errorf("RPC get gas price failed, switching to fallback, err: %v", err)
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
		logger.Errorf("RPC estimate gas failed, switching to fallback, err: %v", err)
		f.switchToNextClient()
	}
	return 0, fmt.Errorf("all RPCs failed: %v", lastErr)
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
		logger.Errorf("RPC subscribe filter logs failed, switching to fallback, err: %v", err)
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
		logger.Errorf("RPC get balance failed, switching to fallback, err: %v", err)
		f.switchToNextClient()
	}
	return nil, fmt.Errorf("all RPCs failed: %v", lastErr)
}

// Close closes all RPC client connections
func (f *FallbackRPCClient) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, client := range f.clients {
		client.Close()
	}
}

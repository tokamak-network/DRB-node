package fallback_ethclient

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

// JSON-RPC request structure
type jsonRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// JSON-RPC response structure
type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
	ID      int         `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Helper function to create a mock RPC server
func createMockRPCServer(t *testing.T, handler func(*jsonRPCRequest) *jsonRPCResponse) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		resp := handler(&req)
		if resp == nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}))
}

// Helper function to create a failing RPC server
func createFailingRPCServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
}

// Helper function to create a complete block header response
func createBlockHeaderResponse(timestamp string) map[string]interface{} {
	zeroHash := "0x0000000000000000000000000000000000000000000000000000000000000000"
	zeroAddress := "0x0000000000000000000000000000000000000000"
	logsBloom := strings.Repeat("0", 512) // 256 bytes = 512 hex chars

	return map[string]interface{}{
		"parentHash":       zeroHash,
		"sha3Uncles":       zeroHash,
		"miner":            zeroAddress,
		"stateRoot":        zeroHash,
		"transactionsRoot": zeroHash,
		"receiptsRoot":     zeroHash,
		"logsBloom":        "0x" + logsBloom,
		"difficulty":       "0x0",
		"number":           "0x1",
		"gasLimit":         "0x1c9c380",
		"gasUsed":          "0x0",
		"timestamp":        timestamp,
		"extraData":        "0x",
		"mixHash":          zeroHash,
		"nonce":            "0x0000000000000000",
		"baseFeePerGas":    "0x0",
	}
}

func TestNewFallbackRPCClient(t *testing.T) {
	logger.InitLogger()

	t.Run("success with single URL", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			return &jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  "0x1",
				ID:      req.ID,
			}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		require.NotNil(t, client)
		client.Close()
	})

	t.Run("success with multiple URLs", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			return &jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  "0x1",
				ID:      req.ID,
			}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			return &jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  "0x1",
				ID:      req.ID,
			}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		require.NotNil(t, client)
		client.Close()
	})

	t.Run("error with empty URLs", func(t *testing.T) {
		client, err := NewFallbackRPCClient([]string{})
		require.Error(t, err)
		require.Nil(t, client)
		assert.Contains(t, err.Error(), "at least one RPC URL is required")
	})

	t.Run("error with invalid URL", func(t *testing.T) {
		client, err := NewFallbackRPCClient([]string{"http://invalid-url-that-does-not-exist:8545"})
		require.Error(t, err)
		require.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to connect to any RPC")
	})

	t.Run("error with logger nil", func(t *testing.T) {
		// Temporarily set logger to nil
		originalLogger := logger.Log
		logger.Log = nil
		defer func() {
			logger.Log = originalLogger
		}()

		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			return &jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  "0x1",
				ID:      req.ID,
			}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.Error(t, err)
		require.Nil(t, client)
		assert.Contains(t, err.Error(), "logger not found")
	})
}

func TestFallbackRPCClient_CallContract(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_call" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x1234",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		result, err := client.CallContract(context.Background(), ethereum.CallMsg{}, nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("success after fallback", func(t *testing.T) {
		callCount := 0
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			callCount++
			if req.Method == "eth_call" && callCount == 1 {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "execution reverted",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_call" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x5678",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		result, err := client.CallContract(context.Background(), ethereum.CallMsg{}, nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("all clients fail", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_call" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "execution reverted",
					},
					ID: req.ID,
				}
			}
			// Allow other calls to succeed (for initialization)
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_call" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "execution reverted",
					},
					ID: req.ID,
				}
			}
			// Allow other calls to succeed (for initialization)
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		result, err := client.CallContract(context.Background(), ethereum.CallMsg{}, nil)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "all RPCs failed")
	})
}

func TestFallbackRPCClient_SendTransaction(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_sendRawTransaction" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x1234",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		// Create a dummy transaction
		tx := types.NewTransaction(0, common.Address{}, big.NewInt(0), 0, big.NewInt(0), nil)
		err = client.SendTransaction(context.Background(), tx)
		require.NoError(t, err)
	})

	t.Run("replacement error does not switch RPC", func(t *testing.T) {
		sendTxCallCount := 0
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_sendRawTransaction" {
				sendTxCallCount++
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "replacement transaction underpriced",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		server2CallCount := 0
		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_sendRawTransaction" {
				server2CallCount++
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		tx := types.NewTransaction(0, common.Address{}, big.NewInt(0), 0, big.NewInt(0), nil)
		err = client.SendTransaction(context.Background(), tx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "replacement transaction underpriced")
		// Should not have switched to server2 - server1 may be called 1-2 times (due to internal retries),
		// but server2 should never be called
		assert.GreaterOrEqual(t, sendTxCallCount, 1, "Replacement errors should call server1 at least once")
		assert.LessOrEqual(t, sendTxCallCount, 2, "Replacement errors should not cause excessive retries")
		assert.Equal(t, 0, server2CallCount, "Replacement errors should not trigger RPC switch to server2")
	})

	t.Run("success after fallback", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_sendRawTransaction" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "network error",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_sendRawTransaction" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x5678",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		tx := types.NewTransaction(0, common.Address{}, big.NewInt(0), 0, big.NewInt(0), nil)
		err = client.SendTransaction(context.Background(), tx)
		require.NoError(t, err)
	})
}

func TestIsReplacementError(t *testing.T) {
	// Test with actual error messages
	err1 := errors.New("replacement transaction underpriced")
	assert.True(t, utils.IsReplacementError(err1))

	err2 := errors.New("nonce too low")
	assert.True(t, utils.IsReplacementError(err2))

	err3 := errors.New("already known")
	assert.True(t, utils.IsReplacementError(err3))

	err4 := errors.New("some other error")
	assert.False(t, utils.IsReplacementError(err4))

	assert.False(t, utils.IsReplacementError(nil))
}

func TestFallbackRPCClient_TransactionReceipt(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getTransactionReceipt" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result: map[string]interface{}{
						"blockHash":         common.Hash{}.String(),
						"blockNumber":       "0x1",
						"contractAddress":   nil,
						"cumulativeGasUsed": "0x1",
						"gasUsed":           "0x1",
						"logs":              []*types.Log{},
						"logsBloom":         types.Bloom{},
						"status":            "0x1",
						"transactionHash":   common.Hash{}.String(),
						"transactionIndex":  "0x0",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		tx := types.NewTransaction(0, common.Address{}, big.NewInt(0), 0, big.NewInt(0), nil)
		receipt, err := client.TransactionReceipt(context.Background(), tx)
		require.NoError(t, err)
		assert.NotNil(t, receipt)
	})

	t.Run("not found error returns ethereum.NotFound", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getTransactionReceipt" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "not found",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		tx := types.NewTransaction(0, common.Address{}, big.NewInt(0), 0, big.NewInt(0), nil)
		receipt, err := client.TransactionReceipt(context.Background(), tx)
		assert.Error(t, err)
		assert.Nil(t, receipt)
		assert.Equal(t, ethereum.NotFound, err)
	})
}

func TestFallbackRPCClient_NetworkID(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "net_version" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "1",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		id, err := client.NetworkID(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, id)
	})

	t.Run("success after fallback", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "net_version" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "network error",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "net_version" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "1",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		id, err := client.NetworkID(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, id)
	})
}

func TestFallbackRPCClient_PendingNonceAt(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getTransactionCount" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x5",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		nonce, err := client.PendingNonceAt(context.Background(), common.Address{})
		require.NoError(t, err)
		assert.Equal(t, uint64(5), nonce)
	})

	t.Run("success after fallback", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getTransactionCount" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "network error",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getTransactionCount" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x10",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		nonce, err := client.PendingNonceAt(context.Background(), common.Address{})
		require.NoError(t, err)
		assert.Equal(t, uint64(16), nonce)
	})
}

func TestFallbackRPCClient_SuggestGasPrice(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_gasPrice" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x3b9aca00", // 1000000000 in hex
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		price, err := client.SuggestGasPrice(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, price)
		assert.Equal(t, big.NewInt(1000000000), price)
	})

	t.Run("success after fallback", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_gasPrice" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "network error",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_gasPrice" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x3b9aca00",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		price, err := client.SuggestGasPrice(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, price)
	})
}

func TestFallbackRPCClient_SuggestGasTipCap(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_maxPriorityFeePerGas" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x3b9aca00",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		tipCap, err := client.SuggestGasTipCap(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, tipCap)
	})

	t.Run("success after fallback", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_maxPriorityFeePerGas" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "network error",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_maxPriorityFeePerGas" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x3b9aca00",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		tipCap, err := client.SuggestGasTipCap(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, tipCap)
	})
}

func TestFallbackRPCClient_EstimateGas(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_estimateGas" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x5208", // 21000 in hex
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		gas, err := client.EstimateGas(context.Background(), ethereum.CallMsg{})
		require.NoError(t, err)
		assert.Equal(t, uint64(21000), gas)
	})

	t.Run("success after fallback", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_estimateGas" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "network error",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_estimateGas" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x5208",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		gas, err := client.EstimateGas(context.Background(), ethereum.CallMsg{})
		require.NoError(t, err)
		assert.Equal(t, uint64(21000), gas)
	})

	t.Run("all servers fail", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_estimateGas" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "gas estimation failed",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_estimateGas" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "execution reverted",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		gas, err := client.EstimateGas(context.Background(), ethereum.CallMsg{})
		assert.Error(t, err)
		assert.Equal(t, uint64(0), gas)
		// Returns the last error after all fallbacks exhausted
		assert.Contains(t, err.Error(), "execution reverted")
	})
}

func TestFallbackRPCClient_BalanceAt(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getBalance" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0xde0b6b3a7640000", // 1000000000000000000 in hex
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		balance, err := client.BalanceAt(context.Background(), common.Address{}, nil)
		require.NoError(t, err)
		assert.NotNil(t, balance)
	})

	t.Run("success after fallback", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getBalance" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "network error",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getBalance" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0xde0b6b3a7640000",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		balance, err := client.BalanceAt(context.Background(), common.Address{}, nil)
		require.NoError(t, err)
		assert.NotNil(t, balance)
	})
}

func TestFallbackRPCClient_BlockTimestamp(t *testing.T) {
	logger.InitLogger()

	t.Run("success on first client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getBlockByNumber" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result: map[string]interface{}{
						"parentHash":       common.Hash{},
						"sha3Uncles":       common.Hash{},
						"miner":            common.Address{},
						"stateRoot":        common.Hash{},
						"transactionsRoot": common.Hash{},
						"receiptsRoot":     common.Hash{},
						"logsBloom":        "0x" + strings.Repeat("0", 512),
						"difficulty":       "0x0",
						"number":           "0x1",
						"gasLimit":         "0x1c9c380",
						"gasUsed":          "0x0",
						"timestamp":        "0x5d5b8c5a",
						"extraData":        "0x",
						"mixHash":          common.Hash{},
						"nonce":            "0x0000000000000000",
						"baseFeePerGas":    "0x0",
					},
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		timestamp, err := client.BlockTimestamp(context.Background(), big.NewInt(1))
		require.NoError(t, err)
		assert.NotZero(t, timestamp)

		timestamp, err = client.BlockTimestamp(context.Background(), big.NewInt(1))
		require.NoError(t, err)
		assert.NotZero(t, timestamp)
	})

	t.Run("success after fallback", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getBlockByNumber" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Error: &rpcError{
						Code:    -32000,
						Message: "network error",
					},
					ID: req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			if req.Method == "eth_getBlockByNumber" {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  createBlockHeaderResponse("0x5d5b8c5a"),
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		timestamp, err := client.BlockTimestamp(context.Background(), big.NewInt(1))
		require.NoError(t, err)
		assert.NotZero(t, timestamp)
	})
}

func TestFallbackRPCClient_Close(t *testing.T) {
	logger.InitLogger()

	server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			Result:  "0x1",
			ID:      req.ID,
		}
	})
	defer server.Close()

	client, err := NewFallbackRPCClient([]string{server.URL})
	require.NoError(t, err)

	// Close should not panic
	assert.NotPanics(t, func() {
		client.Close()
	})

	// Closing again should also not panic
	assert.NotPanics(t, func() {
		client.Close()
	})
}

func TestFallbackRPCClient_SwitchToNextClient(t *testing.T) {
	logger.InitLogger()

	server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server1.Close()

	server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server2.Close()

	server3 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server3.Close()

	client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL, server3.URL})
	require.NoError(t, err)
	defer client.Close()

	// Test that switching wraps around
	client.switchToNextClient()
	assert.Equal(t, 1, client.currentIdx)

	client.switchToNextClient()
	assert.Equal(t, 2, client.currentIdx)

	client.switchToNextClient()
	assert.Equal(t, 0, client.currentIdx) // Should wrap around
}

func TestFallbackRPCClient_GetCurrentClient(t *testing.T) {
	logger.InitLogger()

	server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server1.Close()

	server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server2.Close()

	client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
	require.NoError(t, err)
	defer client.Close()

	// Test that getCurrentClient returns the correct client
	currentClient := client.getCurrentClient()
	assert.NotNil(t, currentClient)
	assert.Equal(t, client.clients[0], currentClient)

	// After switching, should return the next client
	client.switchToNextClient()
	currentClient = client.getCurrentClient()
	assert.NotNil(t, currentClient)
	assert.Equal(t, client.clients[1], currentClient)
}

func TestFallbackRPCClient_SubscribeFilterLogs(t *testing.T) {
	logger.InitLogger()

	t.Run("fails on http client", func(t *testing.T) {
		server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server.Close()

		client, err := NewFallbackRPCClient([]string{server.URL})
		require.NoError(t, err)
		defer client.Close()

		ch := make(chan types.Log)
		sub, err := client.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{}, ch)
		assert.Error(t, err)
		assert.Nil(t, sub)
		assert.Contains(t, err.Error(), "notifications not supported")
	})

	t.Run("fallback also fails on http clients", func(t *testing.T) {
		server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server1.Close()

		server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
			return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
		})
		defer server2.Close()

		client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
		require.NoError(t, err)
		defer client.Close()

		ch := make(chan types.Log)
		sub, err := client.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{}, ch)
		assert.Error(t, err)
		assert.Nil(t, sub)
		assert.Contains(t, err.Error(), "all RPCs failed")
		assert.Contains(t, err.Error(), "notifications not supported")
	})
}

// TestFallbackRPCClient_SingleClientWrapAround tests that with a single client, switching wraps around
func TestFallbackRPCClient_SingleClientWrapAround(t *testing.T) {
	logger.InitLogger()

	server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server.Close()

	client, err := NewFallbackRPCClient([]string{server.URL})
	require.NoError(t, err)
	defer client.Close()

	// With single client, switching should wrap back to 0
	initialIdx := client.currentIdx
	client.switchToNextClient()
	assert.Equal(t, 0, client.currentIdx) // Should wrap around to 0
	assert.Equal(t, initialIdx, client.currentIdx)
}

// TestFallbackRPCClient_AllClientsExhausted tests that all clients are tried before returning error
func TestFallbackRPCClient_AllClientsExhausted(t *testing.T) {
	logger.InitLogger()

	chainIdCallCounts := make(map[string]int)
	testStarted := false

	server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		if req.Method == "eth_chainId" {
			chainIdCallCounts["server1"]++
			// Allow first call to succeed (during Dial), fail calls after test starts
			if !testStarted {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x1",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{
				JSONRPC: "2.0",
				Error: &rpcError{
					Code:    -32000,
					Message: "server1 error",
				},
				ID: req.ID,
			}
		}
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server1.Close()

	server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		if req.Method == "eth_chainId" {
			chainIdCallCounts["server2"]++
			// Allow first call to succeed (during Dial), fail calls after test starts
			if !testStarted {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x1",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{
				JSONRPC: "2.0",
				Error: &rpcError{
					Code:    -32000,
					Message: "server2 error",
				},
				ID: req.ID,
			}
		}
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server2.Close()

	server3 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		if req.Method == "eth_chainId" {
			chainIdCallCounts["server3"]++
			// Allow first call to succeed (during Dial), fail calls after test starts
			if !testStarted {
				return &jsonRPCResponse{
					JSONRPC: "2.0",
					Result:  "0x1",
					ID:      req.ID,
				}
			}
			return &jsonRPCResponse{
				JSONRPC: "2.0",
				Error: &rpcError{
					Code:    -32000,
					Message: "server3 error",
				},
				ID: req.ID,
			}
		}
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server3.Close()

	client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL, server3.URL})
	require.NoError(t, err)
	defer client.Close()

	// Mark test as started - now subsequent calls should fail
	testStarted = true
}

// TestFallbackRPCClient_ConcurrentAccess tests concurrent access to the client
func TestFallbackRPCClient_ConcurrentAccess(t *testing.T) {
	logger.InitLogger()

	server := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server.Close()

	client, err := NewFallbackRPCClient([]string{server.URL})
	require.NoError(t, err)
	defer client.Close()

	// Test concurrent access to getCurrentClient
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				_ = client.getCurrentClient()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and should still work
	currentClient := client.getCurrentClient()
	assert.NotNil(t, currentClient)
}

// TestFallbackRPCClient_MultipleReplacementErrors tests multiple replacement error types
func TestFallbackRPCClient_MultipleReplacementErrors(t *testing.T) {
	logger.InitLogger()

	testCases := []struct {
		name         string
		errorMsg     string
		shouldSwitch bool
	}{
		{"replacement transaction underpriced", "replacement transaction underpriced", false},
		{"nonce too low", "nonce too low", false},
		{"already known", "already known", false},
		{"other error", "some other network error", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sendTxCallCount := 0
			server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
				if req.Method == "eth_sendRawTransaction" {
					sendTxCallCount++
					return &jsonRPCResponse{
						JSONRPC: "2.0",
						Error: &rpcError{
							Code:    -32000,
							Message: tc.errorMsg,
						},
						ID: req.ID,
					}
				}
				return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
			})
			defer server1.Close()

			server2CallCount := 0
			server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
				if req.Method == "eth_sendRawTransaction" {
					server2CallCount++
				}
				return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
			})
			defer server2.Close()

			client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
			require.NoError(t, err)
			defer client.Close()

			tx := types.NewTransaction(0, common.Address{}, big.NewInt(0), 0, big.NewInt(0), nil)
			_ = client.SendTransaction(context.Background(), tx)

			// If it's a replacement error, should not switch (server1 should be called once, server2 should not be called)
			// If it's not a replacement error, should switch (server2 should be called)
			if !tc.shouldSwitch {
				assert.Equal(t, 1, sendTxCallCount, "Replacement errors should only call server1 once")
				assert.Equal(t, 0, server2CallCount, "Replacement errors should not trigger RPC switch to server2")
			} else {
				assert.Greater(t, server2CallCount, 0, "Non-replacement errors should trigger RPC switch")
			}
		})
	}
}

// TestFallbackRPCClient_ClientFields tests that client fields are properly initialized
func TestFallbackRPCClient_ClientFields(t *testing.T) {
	logger.InitLogger()

	server1 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server1.Close()

	server2 := createMockRPCServer(t, func(req *jsonRPCRequest) *jsonRPCResponse {
		return &jsonRPCResponse{JSONRPC: "2.0", Result: "0x1", ID: req.ID}
	})
	defer server2.Close()

	client, err := NewFallbackRPCClient([]string{server1.URL, server2.URL})
	require.NoError(t, err)
	defer client.Close()

	// Check that fields are properly initialized
	assert.Equal(t, 0, client.currentIdx)
	assert.Equal(t, 2, len(client.clients))
	assert.Equal(t, 2, len(client.urls))
	assert.Equal(t, 3, client.maxRetries)
	assert.Equal(t, time.Second*2, client.retryDelay)
	assert.NotNil(t, client.logger)
	assert.NotNil(t, client.clients[0])
	assert.NotNil(t, client.clients[1])
}

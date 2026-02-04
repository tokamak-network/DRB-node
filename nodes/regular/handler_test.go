package regular_node

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/eapache/queue"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-pg/pg/v10"
	_ "github.com/lib/pq"
	"github.com/libp2p/go-libp2p/core/connmgr"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

const testABIContent = `{
    "abi": [
        {
            "inputs": [],
            "name": "deposit",
            "outputs": [],
            "stateMutability": "payable",
            "type": "function"
        },
        {
            "inputs": [{"name": "operator", "type": "address"}],
            "name": "s_depositAmount",
            "outputs": [{"name": "", "type": "uint256"}],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "s_activationThreshold",
            "outputs": [{"name": "", "type": "uint256"}],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "activate",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "getActivatedOperators",
            "outputs": [{"name": "", "type": "address[]"}],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [{"name": "cv", "type": "bytes32"}],
            "name": "submitCv",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [{"name": "co", "type": "bytes32"}],
            "name": "submitCo",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [{"name": "s", "type": "bytes32"}],
            "name": "submitS",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "packedIndicesAscendingFromLSB", "type": "uint256"}
            ],
            "name": "RequestedToSubmitCv",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "cv", "type": "bytes32"},
                {"indexed": false, "name": "index", "type": "uint256"}
            ],
            "name": "CvSubmitted",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "curRound", "type": "uint256"},
                {"indexed": false, "name": "curTrialNum", "type": "uint256"},
                {"indexed": false, "name": "curState", "type": "uint256"}
            ],
            "name": "Status",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "merkleRoot", "type": "bytes32"}
            ],
            "name": "MerkleRootSubmitted",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "indicesLength", "type": "uint256"},
                {"indexed": false, "name": "packedIndices", "type": "uint256"}
            ],
            "name": "RequestedToSubmitCo",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "indexK", "type": "uint256"}
            ],
            "name": "RequestedToSubmitSFromIndexK",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "s", "type": "bytes32"},
                {"indexed": false, "name": "index", "type": "uint256"}
            ],
            "name": "SSubmitted",
            "type": "event"
        },
        {
            "inputs": [],
            "name": "failToRequestSubmitCvOrSubmitMerkleRoot",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "failToSubmitMerkleRootAfterDispute",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "failToRequestSorGenerateRandomNumber",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        }
    ]
}`

type MockHost struct {
	mock.Mock
	streamHandlers map[protocol.ID]network.StreamHandler
}

func (m *MockHost) ID() peer.ID {
	args := m.Called()
	if args.Get(0) == nil {
		return ""
	}
	return args.Get(0).(peer.ID)
}

func (m *MockHost) Peerstore() peerstore.Peerstore {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(peerstore.Peerstore)
}

func (m *MockHost) Addrs() []multiaddr.Multiaddr {
	args := m.Called()
	if args.Get(0) == nil {
		return []multiaddr.Multiaddr{}
	}
	return args.Get(0).([]multiaddr.Multiaddr)
}

func (m *MockHost) Network() network.Network {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(network.Network)
}

func (m *MockHost) Mux() protocol.Switch {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(protocol.Switch)
}

func (m *MockHost) Connect(ctx context.Context, pi peer.AddrInfo) error {
	args := m.Called(ctx, pi)
	return args.Error(0)
}

func (m *MockHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	m.Called(pid, handler)
	if m.streamHandlers == nil {
		m.streamHandlers = make(map[protocol.ID]network.StreamHandler)
	}
	m.streamHandlers[pid] = handler
}

func (m *MockHost) SetStreamHandlerMatch(pid protocol.ID, match func(protocol.ID) bool, handler network.StreamHandler) {
	m.Called(pid, match, handler)
}

func (m *MockHost) RemoveStreamHandler(pid protocol.ID) {
	m.Called(pid)
}

func (m *MockHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
	args := m.Called(ctx, p, pids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(network.Stream), args.Error(1)
}

func (m *MockHost) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockHost) ConnManager() connmgr.ConnManager {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(connmgr.ConnManager)
}

func (m *MockHost) EventBus() event.Bus {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(event.Bus)
}

// MockPeerstore for testing peerstore operations
type MockPeerstore struct {
	mock.Mock
}

func (m *MockPeerstore) AddAddr(p peer.ID, addr multiaddr.Multiaddr, ttl time.Duration) {
	m.Called(p, addr, ttl)
}

func (m *MockPeerstore) Addrs(p peer.ID) []multiaddr.Multiaddr {
	args := m.Called(p)
	if args.Get(0) == nil {
		return []multiaddr.Multiaddr{}
	}
	return args.Get(0).([]multiaddr.Multiaddr)
}

func (m *MockPeerstore) AddAddrs(p peer.ID, addrs []multiaddr.Multiaddr, ttl time.Duration) {
	m.Called(p, addrs, ttl)
}

// Implementing minimal peerstore.Peerstore interface methods
func (m *MockPeerstore) SetAddr(peer.ID, multiaddr.Multiaddr, time.Duration)            {}
func (m *MockPeerstore) SetAddrs(peer.ID, []multiaddr.Multiaddr, time.Duration)         {}
func (m *MockPeerstore) UpdateAddrs(peer.ID, time.Duration, time.Duration)              {}
func (m *MockPeerstore) AddrStream(context.Context, peer.ID) <-chan multiaddr.Multiaddr { return nil }
func (m *MockPeerstore) ClearAddrs(peer.ID)                                             {}
func (m *MockPeerstore) PeersWithAddrs() peer.IDSlice                                   { return nil }
func (m *MockPeerstore) PeerInfo(peer.ID) peer.AddrInfo                                 { return peer.AddrInfo{} }
func (m *MockPeerstore) Peers() peer.IDSlice                                            { return nil }
func (m *MockPeerstore) Get(peer.ID, string) (interface{}, error)                       { return nil, nil }
func (m *MockPeerstore) Put(peer.ID, string, interface{}) error                         { return nil }
func (m *MockPeerstore) GetProtocols(peer.ID) ([]protocol.ID, error)                    { return nil, nil }
func (m *MockPeerstore) AddProtocols(peer.ID, ...protocol.ID) error                     { return nil }
func (m *MockPeerstore) SetProtocols(peer.ID, ...protocol.ID) error                     { return nil }
func (m *MockPeerstore) RemoveProtocols(peer.ID, ...protocol.ID) error                  { return nil }
func (m *MockPeerstore) SupportsProtocols(peer.ID, ...protocol.ID) ([]protocol.ID, error) {
	return nil, nil
}
func (m *MockPeerstore) FirstSupportedProtocol(peer.ID, ...protocol.ID) (protocol.ID, error) {
	return "", nil
}
func (m *MockPeerstore) RemovePeer(peer.ID)                     {}
func (m *MockPeerstore) PeersWithKeys() peer.IDSlice            { return nil }
func (m *MockPeerstore) AddPrivKey(peer.ID, ic.PrivKey) error   { return nil }
func (m *MockPeerstore) PrivKey(peer.ID) ic.PrivKey             { return nil }
func (m *MockPeerstore) GetPrivKey(peer.ID) (ic.PrivKey, error) { return nil, nil }
func (m *MockPeerstore) AddPubKey(peer.ID, ic.PubKey) error     { return nil }
func (m *MockPeerstore) PubKey(peer.ID) ic.PubKey               { return nil }
func (m *MockPeerstore) GetPubKey(peer.ID) (ic.PubKey, error)   { return nil, nil }
func (m *MockPeerstore) RecordLatency(peer.ID, time.Duration)   {}
func (m *MockPeerstore) LatencyEWMA(peer.ID) time.Duration      { return 0 }
func (m *MockPeerstore) Close() error                           { return nil }

// MockFallbackEthClient for testing blockchain interactions
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

func (m *MockFallbackEthClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
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

func (m *MockFallbackEthClient) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	args := m.Called(ctx, q)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]types.Log), args.Error(1)
}

func (m *MockFallbackEthClient) HeaderByNumber(ctx context.Context, blockNumber *big.Int) (*types.Header, error) {
	args := m.Called(ctx, blockNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Header), args.Error(1)
}

// MockP2PClient for testing P2P operations
type MockP2PClient struct {
	mock.Mock
}

func (m *MockP2PClient) CreateHost(port string, nodeType string) (interface{}, peer.ID, error) {
	args := m.Called(port, nodeType)
	if args.Get(0) == nil {
		return nil, "", args.Error(2)
	}
	return args.Get(0), args.Get(1).(peer.ID), args.Error(2)
}

func (m *MockP2PClient) ConnectToLeader(ctx context.Context, leaderIP, leaderPort, leaderPeerID string) (*utils.NodeInfo, error) {
	args := m.Called(ctx, leaderIP, leaderPort, leaderPeerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.NodeInfo), args.Error(1)
}

func TestMain(m *testing.M) {
	// Setup: Create ABI file before tests
	setupTestEnvironment()

	// Run tests
	code := m.Run()

	// Cleanup
	cleanupTestEnvironment()

	os.Exit(code)
}

func setupTestEnvironment() {
	// Create contract/abi directory
	err := os.MkdirAll("contract/abi", 0755)
	if err != nil {
		log.Fatal("Failed to create test directory:", err)
	}

	// Write minimal ABI file
	err = os.WriteFile("contract/abi/Commit2RevealDRB.json", []byte(testABIContent), 0644)
	if err != nil {
		log.Fatal("Failed to create test ABI file:", err)
	}

	log.Println("✓ Test ABI file created")
}

func cleanupTestEnvironment() {
	// Remove test ABI file
	os.Remove("contract/abi/Commit2RevealDRB.json")
	log.Println("✓ Test environment cleaned up")
}

// MockStream for testing stream operations
type MockStream struct {
	readBuffer  *bytes.Buffer
	writeBuffer *bytes.Buffer
	closed      bool
}

func newMockStream() *MockStream {
	return &MockStream{
		readBuffer:  new(bytes.Buffer),
		writeBuffer: new(bytes.Buffer),
		closed:      false,
	}
}

func (m *MockStream) Read(p []byte) (n int, err error)   { return m.readBuffer.Read(p) }
func (m *MockStream) Write(p []byte) (n int, err error)  { return m.writeBuffer.Write(p) }
func (m *MockStream) Close() error                       { m.closed = true; return nil }
func (m *MockStream) Reset() error                       { return nil }
func (m *MockStream) SetDeadline(t time.Time) error      { return nil }
func (m *MockStream) SetReadDeadline(t time.Time) error  { return nil }
func (m *MockStream) SetWriteDeadline(t time.Time) error { return nil }
func (m *MockStream) ID() string                         { return "mock-stream-id" }
func (m *MockStream) Protocol() protocol.ID              { return protocol.ID("/test") }
func (m *MockStream) SetProtocol(protocol.ID) error      { return nil }
func (m *MockStream) Stat() network.Stats                { return network.Stats{} }
func (m *MockStream) Conn() network.Conn                 { return nil }
func (m *MockStream) CloseWrite() error                  { m.closed = true; return nil }
func (m *MockStream) CloseRead() error                   { m.closed = true; return nil }
func (m *MockStream) Scope() network.StreamScope         { return nil }

// MockRegularCommitRepository for testing
type MockRegularCommitRepository struct {
	mock.Mock
}

func (m *MockRegularCommitRepository) AddCommit(ctx context.Context, commitData *utils.CommitData) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

func (m *MockRegularCommitRepository) UpdateCommit(ctx context.Context, commitData *utils.CommitData) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

func (m *MockRegularCommitRepository) GetCommitByRound(ctx context.Context, round, trialNum string) (*utils.CommitData, error) {
	args := m.Called(ctx, round, trialNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.CommitData), args.Error(1)
}

// MockEthService for testing
type MockEthService struct {
	GetActivatedOperatorsCachedFunc func() []common.Address
	GetActivatedOperatorsFunc       func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error)
	CallSmartContractFunc           func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error)
	ExecuteTransactionFunc          func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error)
	GetActivatedOperatorsLengthFunc func() int64
}

func (m *MockEthService) GetActivatedOperatorsCached() []common.Address {
	if m.GetActivatedOperatorsCachedFunc != nil {
		return m.GetActivatedOperatorsCachedFunc()
	}
	return []common.Address{}
}

func (m *MockEthService) GetActivatedOperatorsLength() int64 {
	if m.GetActivatedOperatorsLengthFunc != nil {
		return m.GetActivatedOperatorsLengthFunc()
	}
	return int64(len(m.GetActivatedOperatorsCached()))
}

func (m *MockEthService) SetActivatedOperatorsCached(operators []common.Address) {
	// No-op for tests
}

func (m *MockEthService) GetActivatedOperatorsUnsafe() []common.Address {
	return m.GetActivatedOperatorsCached()
}

func (m *MockEthService) GetActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
	if m.GetActivatedOperatorsFunc != nil {
		return m.GetActivatedOperatorsFunc(ctx, fallbackEthClient)
	}
	return []common.Address{}, nil
}

func (m *MockEthService) UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) error {
	// No-op for tests
	return nil
}

func (m *MockEthService) CallSmartContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	if m.CallSmartContractFunc != nil {
		return m.CallSmartContractFunc(ctx, fallbackEthClient, parsedABI, method, contractAddress, params...)
	}
	return nil, nil
}

func (m *MockEthService) ExecuteTransaction(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
	if m.ExecuteTransactionFunc != nil {
		return m.ExecuteTransactionFunc(ctx, clientUtils, fallbackEthClient, method, value, args...)
	}
	return nil, nil, nil
}

func (m *MockEthService) UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	return nil, nil
}

func (m *MockEthService) GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	return nil, nil
}

// MockNodeInfoRepository for testing
type MockNodeInfoRepository struct {
	mock.Mock
}

func (m *MockNodeInfoRepository) AddAndUpdateNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	args := m.Called(ctx, nodeInfo)
	return args.Error(0)
}

func (m *MockNodeInfoRepository) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.NodeInfo), args.Error(1)
}

func (m *MockNodeInfoRepository) GetNodeInfoByEOA(ctx context.Context, eoaAddress string) (*utils.NodeInfo, error) {
	args := m.Called(ctx, eoaAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.NodeInfo), args.Error(1)
}

func (m *MockNodeInfoRepository) DeleteNodeInfoByEOA(ctx context.Context, eoaAddress string) error {
	args := m.Called(ctx, eoaAddress)
	return args.Error(0)
}

type MockRevealOrderRepository struct {
	mock.Mock
}

func (m *MockRevealOrderRepository) GetRevealOrder(ctx context.Context, round, trialNum string) (*utils.RevealOrderData, error) {
	args := m.Called(ctx, round, trialNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.RevealOrderData), args.Error(1)
}

func (m *MockRevealOrderRepository) AddRevealOrder(ctx context.Context, revealOrder *utils.RevealOrderData) error {
	args := m.Called(ctx, revealOrder)
	return args.Error(0)
}

type MockBatchRepository struct {
	mock.Mock
}

func (m *MockBatchRepository) DeleteOldRoundDataForRegularNode(ctx context.Context, currentRound string) error {
	args := m.Called(ctx, currentRound)
	return args.Error(0)
}

func (m *MockBatchRepository) DeleteRoundTrialDataForRegularNode(ctx context.Context, round, trialNum string) error {
	args := m.Called(ctx, round, trialNum)
	return args.Error(0)
}

func (m *MockBatchRepository) DeleteOldRoundDataForLeaderNode(ctx context.Context, currentRound string) error {
	args := m.Called(ctx, currentRound)
	return args.Error(0)
}

func (m *MockBatchRepository) DeleteRoundTrialDataForLeaderNode(ctx context.Context, round, trialNum string) error {
	args := m.Called(ctx, round, trialNum)
	return args.Error(0)
}

type MockLeaderCommitRepository struct {
	mock.Mock
}

func (m *MockLeaderCommitRepository) GetLeaderCommitByRoundAndEoaAddr(ctx context.Context, round, trialNum, eoaAddress string) (*utils.LeaderCommitData, error) {
	args := m.Called(ctx, round, trialNum, eoaAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.LeaderCommitData), args.Error(1)
}

func (m *MockLeaderCommitRepository) UpdateLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

func (m *MockLeaderCommitRepository) AddLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

func (m *MockLeaderCommitRepository) GetLeaderCommitsByRoundAndTrialNum(ctx context.Context, round, trialNum string) ([]*utils.LeaderCommitData, error) {
	args := m.Called(ctx, round, trialNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.LeaderCommitData), args.Error(1)
}

func (m *MockLeaderCommitRepository) UpdateLeaderCommitRandomNumberGenerated(ctx context.Context, round, trialNum string) error {
	args := m.Called(ctx, round, trialNum)
	return args.Error(0)
}

func createTestRegularNodeHandler() *RegularNodeHandler {
	mockCommitRepo := new(MockRegularCommitRepository)
	mockNodeInfoRepo := new(MockNodeInfoRepository)

	// Create a test client with generated private key
	testPrivateKey, _ := crypto.GenerateKey()
	testContractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	testClient := &utils.Client{
		ContractAddress: testContractAddress,
		PrivateKey:      testPrivateKey,
	}

	handler := &RegularNodeHandler{
		fallbackEthClient: nil,
		regularNode: &RegularNode{
			client:                  testClient,
			regularCommitRepository: mockCommitRepo,
			nodeInfoRepository:      mockNodeInfoRepo,
		},
	}

	return handler
}

func generateValidSignatureForRegular(eoaAddress string, privateKey *ecdsa.PrivateKey) []byte {
	hash := crypto.Keccak256Hash([]byte(eoaAddress))
	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return nil
	}
	return signature
}

func createTestKeyPairForRegular() (*ecdsa.PrivateKey, string) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, ""
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, ""
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	return privateKey, address
}

func TestRegularNodeHandler_NewRegularNodeHandler(t *testing.T) {
	handler := createTestRegularNodeHandler()

	assert.NotNil(t, handler, "Handler should not be nil")
	assert.NotNil(t, handler.regularNode, "Regular node should not be nil")
}

func TestRegularNodeHandler_isEOAActivated_Success(t *testing.T) {
	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	result := isEOAActivated(testOp.Hex())
	assert.True(t, result, "EOA should be activated")
}

func TestRegularNodeHandler_isEOAActivated_NotFound(t *testing.T) {
	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")
	otherOp := common.HexToAddress("0xABCDEF1234567890123456789012345678901234")

	// Set different operator
	eth.SetActivatedOperatorsCached([]common.Address{otherOp})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	result := isEOAActivated(testOp.Hex())
	assert.False(t, result, "EOA should not be activated")
}

func TestRegularNodeHandler_isEOAActivated_EmptyList(t *testing.T) {
	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set empty list
	eth.SetActivatedOperatorsCached([]common.Address{})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	result := isEOAActivated(testOp.Hex())
	assert.False(t, result, "EOA should not be activated when list is empty")
}

func TestRegularNodeHandler_isEOAActivated_MultipleOperators(t *testing.T) {
	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	testOp3 := common.HexToAddress("0x3333333333333333333333333333333333333333")

	// Set multiple operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2, testOp3})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Test each one
	assert.True(t, isEOAActivated(testOp1.Hex()), "First operator should be activated")
	assert.True(t, isEOAActivated(testOp2.Hex()), "Second operator should be activated")
	assert.True(t, isEOAActivated(testOp3.Hex()), "Third operator should be activated")

	// Test non-existent
	assert.False(t, isEOAActivated("0x4444444444444444444444444444444444444444"), "Non-existent operator should not be activated")
}

func TestRegularNodeHandler_checkActivationStatus_Activated(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			return []common.Address{testOp}, nil
		},
	}

	// Temporarily replace eth.Service
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isNetworkErr, isActivated := handler.checkActivationStatus(context.Background(), client, testOp.Hex())

	assert.False(t, isNetworkErr, "Should not be network error")
	assert.True(t, isActivated, "EOA should be activated")
}

func TestRegularNodeHandler_checkActivationStatus_NotActivated(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")
	otherOp := common.HexToAddress("0xABCDEF1234567890123456789012345678901234")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			return []common.Address{otherOp}, nil // Different operator
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isNetworkErr, isActivated := handler.checkActivationStatus(context.Background(), client, testOp.Hex())

	assert.False(t, isNetworkErr, "Should not be network error")
	assert.False(t, isActivated, "EOA should not be activated")
}

func TestRegularNodeHandler_checkActivationStatus_NetworkError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			return nil, errors.New("network connection failed")
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isNetworkErr, isActivated := handler.checkActivationStatus(context.Background(), client, testOp.Hex())

	assert.True(t, isNetworkErr, "Should be network error")
	assert.False(t, isActivated, "EOA should not be activated on network error")
}

func TestRegularNodeHandler_checkDepositAmount_Sufficient(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1000), nil // Deposit: 1000
			}
			if method == "s_activationThreshold" {
				return big.NewInt(500), nil // Threshold: 500
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isSufficient, err := handler.checkDepositAmount(context.Background(), client, testOp.Hex())

	assert.NoError(t, err, "Should not return error")
	assert.True(t, isSufficient, "Deposit should be sufficient")
}

func TestRegularNodeHandler_checkDepositAmount_Insufficient(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(300), nil // Deposit: 300
			}
			if method == "s_activationThreshold" {
				return big.NewInt(1000), nil // Threshold: 1000
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isSufficient, err := handler.checkDepositAmount(context.Background(), client, testOp.Hex())

	assert.NoError(t, err, "Should not return error")
	assert.False(t, isSufficient, "Deposit should be insufficient")
}

func TestRegularNodeHandler_checkDepositAmount_ErrorGettingDepositAmount(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return nil, errors.New("failed to get deposit amount")
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isSufficient, err := handler.checkDepositAmount(context.Background(), client, testOp.Hex())

	assert.Error(t, err, "Should return error")
	assert.False(t, isSufficient, "Should return false on error")
	assert.Contains(t, err.Error(), "failed to call s_depositAmount")
}

func TestRegularNodeHandler_checkDepositAmount_ErrorGettingThreshold(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1000), nil
			}
			if method == "s_activationThreshold" {
				return nil, errors.New("failed to get threshold")
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isSufficient, err := handler.checkDepositAmount(context.Background(), client, testOp.Hex())

	assert.Error(t, err, "Should return error")
	assert.False(t, isSufficient, "Should return false on error")
	assert.Contains(t, err.Error(), "failed to call s_activationThreshold")
}

func TestRegularNodeHandler_checkDepositAmount_ExactThreshold(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1000), nil
			}
			if method == "s_activationThreshold" {
				return big.NewInt(1000), nil // Exactly equal
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isSufficient, err := handler.checkDepositAmount(context.Background(), client, testOp.Hex())

	assert.NoError(t, err, "Should not return error")
	assert.True(t, isSufficient, "Deposit equal to threshold should be sufficient")
}

func TestRegularNodeHandler_SignatureVerification_Valid(t *testing.T) {
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)
	require.NotEmpty(t, eoaAddress)

	signature := generateValidSignatureForRegular(eoaAddress, privateKey)

	verifyReq := utils.Verification{
		EOAAddress: eoaAddress,
		Signature:  signature,
	}

	result := utils.VerifySignature(verifyReq)
	assert.True(t, result, "Valid signature should be verified")
}

func TestRegularNodeHandler_RoundDataManagement(t *testing.T) {
	handler := createTestRegularNodeHandler()

	uniqueKey := "100:1"

	// Test getting non-existent data
	_, exists := handler.regularNode.GetRoundData(uniqueKey)
	assert.False(t, exists, "RoundData should not exist initially")

	// Set data
	roundData := RoundData{
		MerkleRoot:   true,
		RandomNumber: false,
	}
	handler.regularNode.SetRoundData(uniqueKey, roundData)

	// Retrieve and verify
	retrieved, exists := handler.regularNode.GetRoundData(uniqueKey)
	assert.True(t, exists, "RoundData should exist after setting")
	assert.True(t, retrieved.MerkleRoot, "MerkleRoot should be true")
	assert.False(t, retrieved.RandomNumber, "RandomNumber should be false")

	// Update data
	roundData.RandomNumber = true
	handler.regularNode.SetRoundData(uniqueKey, roundData)

	retrieved, _ = handler.regularNode.GetRoundData(uniqueKey)
	assert.True(t, retrieved.RandomNumber, "RandomNumber should be updated")

	// Delete data
	handler.regularNode.DeleteRoundsData(uniqueKey)
	_, exists = handler.regularNode.GetRoundData(uniqueKey)
	assert.False(t, exists, "RoundData should not exist after deletion")
}

func TestRegularNodeHandler_ExecutionState(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Test default
	assert.False(t, handler.regularNode.GetExecution(), "Execution should be false by default")

	// Set to true
	handler.regularNode.SetExecution(true)
	assert.True(t, handler.regularNode.GetExecution(), "Execution should be true")

	// Set to false
	handler.regularNode.SetExecution(false)
	assert.False(t, handler.regularNode.GetExecution(), "Execution should be false")
}

func TestRegularNodeHandler_HaltedState(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Test default
	assert.False(t, handler.regularNode.GetHalted(), "Halted should be false by default")

	// Set to true
	handler.regularNode.SetHalted(true)
	assert.True(t, handler.regularNode.GetHalted(), "Halted should be true")

	// Set to false
	handler.regularNode.SetHalted(false)
	assert.False(t, handler.regularNode.GetHalted(), "Halted should be false")
}

func TestRegularNodeHandler_CurrentRoundAndTrial(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Test default
	assert.Empty(t, handler.regularNode.GetCurrentRound(), "CurrentRound should be empty by default")
	assert.Empty(t, handler.regularNode.GetCurrentTrialNum(), "CurrentTrialNum should be empty by default")

	// Set values
	handler.regularNode.SetCurrentRound("100")
	handler.regularNode.SetCurrentTrialNum("1")

	assert.Equal(t, "100", handler.regularNode.GetCurrentRound())
	assert.Equal(t, "1", handler.regularNode.GetCurrentTrialNum())

	// Update values
	handler.regularNode.SetCurrentRound("200")
	handler.regularNode.SetCurrentTrialNum("2")

	assert.Equal(t, "200", handler.regularNode.GetCurrentRound())
	assert.Equal(t, "2", handler.regularNode.GetCurrentTrialNum())
}

func TestRegularNodeHandler_CommitGeneration(t *testing.T) {
	round := "100"
	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set activated operators for GenerateCommit to work
	eth.SetActivatedOperatorsCached([]common.Address{testOp})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	secretValue, cos, cvs, err := commitreveal2.GenerateCommit(round, testOp.Hex())

	assert.NoError(t, err, "Should generate commit without error")
	assert.NotEqual(t, [32]byte{}, secretValue, "Secret value should not be empty")
	assert.NotEqual(t, [32]byte{}, cos, "COS should not be empty")
	assert.NotEqual(t, [32]byte{}, cvs, "CVS should not be empty")

	// Verify CVS is hash of COS + index
	recalculatedCvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], []byte{0}))
	assert.True(t, bytes.Equal(cvs[:], recalculatedCvs), "CVS should be hash of COS + index")
}

func TestRegularNodeHandler_CommitGeneration_DifferentRounds(t *testing.T) {
	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set activated operators for GenerateCommit to work
	eth.SetActivatedOperatorsCached([]common.Address{testOp})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	_, cos1, cvs1, _ := commitreveal2.GenerateCommit("100", testOp.Hex())
	_, cos2, cvs2, _ := commitreveal2.GenerateCommit("200", testOp.Hex())

	// Different rounds should produce different commits
	assert.False(t, bytes.Equal(cos1[:], cos2[:]), "Different rounds should produce different COS")
	assert.False(t, bytes.Equal(cvs1[:], cvs2[:]), "Different rounds should produce different CVS")
}

func TestRegularNodeHandler_CommitGeneration_DifferentEOAs(t *testing.T) {
	round := "100"
	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	// Set activated operators for GenerateCommit to work
	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	_, cos1, cvs1, _ := commitreveal2.GenerateCommit(round, testOp1.Hex())
	_, cos2, cvs2, _ := commitreveal2.GenerateCommit(round, testOp2.Hex())

	// Different EOAs should produce different commits
	assert.False(t, bytes.Equal(cos1[:], cos2[:]), "Different EOAs should produce different COS")
	assert.False(t, bytes.Equal(cvs1[:], cvs2[:]), "Different EOAs should produce different CVS")
}

func TestRegularNodeHandler_LeaderMonitoringActive(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Test default
	assert.False(t, handler.regularNode.GetLeaderMonitoringActive(), "LeaderMonitoringActive should be false by default")

	// Set to true
	handler.regularNode.SetLeaderMonitoringActive(true)
	assert.True(t, handler.regularNode.GetLeaderMonitoringActive(), "LeaderMonitoringActive should be true")

	// Set to false
	handler.regularNode.SetLeaderMonitoringActive(false)
	assert.False(t, handler.regularNode.GetLeaderMonitoringActive(), "LeaderMonitoringActive should be false")
}

func TestRegularNodeHandler_MerkleRootSubmittedEventEmitted(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Test default
	assert.False(t, handler.regularNode.GetMerkleRootSubmittedEventEmitted(), "MerkleRootSubmittedEventEmitted should be false by default")

	// Set to true
	handler.regularNode.SetMerkleRootSubmittedEventEmitted(true)
	assert.True(t, handler.regularNode.GetMerkleRootSubmittedEventEmitted(), "MerkleRootSubmittedEventEmitted should be true")

	// Set to false
	handler.regularNode.SetMerkleRootSubmittedEventEmitted(false)
	assert.False(t, handler.regularNode.GetMerkleRootSubmittedEventEmitted(), "MerkleRootSubmittedEventEmitted should be false")
}

func TestRegularNodeHandler_CvRequestIndices(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Test default
	indices := handler.regularNode.GetCvRequestIndices()
	assert.Empty(t, indices, "CvRequestIndices should be empty by default")

	// Set indices
	testIndices := []*big.Int{big.NewInt(0), big.NewInt(1), big.NewInt(2)}
	handler.regularNode.SetCvRequestIndices(testIndices)

	retrieved := handler.regularNode.GetCvRequestIndices()
	assert.Len(t, retrieved, 3, "Should have 3 indices")
	assert.Equal(t, big.NewInt(0), retrieved[0])
	assert.Equal(t, big.NewInt(1), retrieved[1])
	assert.Equal(t, big.NewInt(2), retrieved[2])

	// Clear indices
	handler.regularNode.ClearCvRequestIndices()
	cleared := handler.regularNode.GetCvRequestIndices()
	assert.Empty(t, cleared, "Indices should be empty after clearing")
}

func TestRegularNodeHandler_SubmittedCvIndices(t *testing.T) {
	handler := createTestRegularNodeHandler()

	uniqueKey := "100:1"

	// Test setting and getting individual values
	handler.regularNode.SetSubmittedCvIndicesValue(uniqueKey, "0", true)
	handler.regularNode.SetSubmittedCvIndicesValue(uniqueKey, "1", false)

	value0, exists0 := handler.regularNode.GetSubmittedCvIndicesValue(uniqueKey, "0")
	value1, exists1 := handler.regularNode.GetSubmittedCvIndicesValue(uniqueKey, "1")

	assert.True(t, exists0)
	assert.True(t, value0)
	assert.True(t, exists1)
	assert.False(t, value1)

	// Test getting entire map
	indicesMap, exists := handler.regularNode.GetSubmittedCvIndicesMap(uniqueKey)
	assert.True(t, exists)
	assert.Len(t, indicesMap, 2)

	// Test deletion
	handler.regularNode.DeleteSubmittedCvIndices(uniqueKey)
	_, exists = handler.regularNode.GetSubmittedCvIndicesMap(uniqueKey)
	assert.False(t, exists)
}

func createTestNodeForHandler() *RegularNode {
	// Create a test client with generated private key
	testPrivateKey, _ := crypto.GenerateKey()
	testContractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	testClient := &utils.Client{
		ContractAddress: testContractAddress,
		PrivateKey:      testPrivateKey,
	}

	return &RegularNode{
		client:                        testClient,
		submittedCvIndices:            make(map[string]map[string]bool),
		cleanupQueue:                  queue.New(),
		strictOrderWhileSecretRequest: make(map[string][]string),
		roundsData:                    make(map[string]RoundData),
		cosRecevied:                   sync.Map{},
	}
}

func TestRegularNodeHandler_StrictOrderManagement(t *testing.T) {
	// Create node with NewRegularNode to properly initialize maps
	node := createTestNodeForHandler()

	uniqueKey := "100:1"
	order := []string{"node1", "node2", "node3"}

	// Test getting non-existent
	_, exists := node.GetStrictOrder(uniqueKey)
	assert.False(t, exists)

	// Set order
	node.SetStrictOrder(uniqueKey, order)

	retrieved, exists := node.GetStrictOrder(uniqueKey)
	assert.True(t, exists)
	assert.Len(t, retrieved, 3)
	assert.Equal(t, "node1", retrieved[0])

	// Delete order
	node.DeleteStrictOrder(uniqueKey)
	_, exists = node.GetStrictOrder(uniqueKey)
	assert.False(t, exists)
}

func TestRegularNodeHandler_RegularNodePrivateKey(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Test default
	assert.Nil(t, handler.regularNode.GetRegularNodePrivateKey())

	// Generate and set key
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	handler.regularNode.SetRegularNodePrivateKey(privateKey)

	retrieved := handler.regularNode.GetRegularNodePrivateKey()
	assert.NotNil(t, retrieved)
	assert.Equal(t, privateKey, retrieved)

	// Set to nil
	handler.regularNode.SetRegularNodePrivateKey(nil)
	assert.Nil(t, handler.regularNode.GetRegularNodePrivateKey())
}

func TestRegularNodeHandler_RegularNodeEOA(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Test default
	assert.Empty(t, handler.regularNode.GetRegularNodeEOA())

	// Set EOA
	testEOA := "0x1234567890123456789012345678901234567890"
	handler.regularNode.SetRegularNodeEOA(testEOA)

	assert.Equal(t, testEOA, handler.regularNode.GetRegularNodeEOA())

	// Update EOA
	newEOA := "0xABCDEF1234567890123456789012345678901234"
	handler.regularNode.SetRegularNodeEOA(newEOA)

	assert.Equal(t, newEOA, handler.regularNode.GetRegularNodeEOA())
}

func TestRegularNodeHandler_ConcurrentStateAccess(t *testing.T) {
	handler := createTestRegularNodeHandler()
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			handler.regularNode.SetExecution(true)
			handler.regularNode.SetExecution(false)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = handler.regularNode.GetExecution()
		}
		done <- true
	}()

	<-done
	<-done

	// Test passes if no race condition occurs
}

func TestRegularNodeHandler_ConcurrentRoundDataAccess(t *testing.T) {
	handler := createTestRegularNodeHandler()
	done := make(chan bool, 3)

	uniqueKey := "100:1"

	go func() {
		for i := 0; i < 100; i++ {
			handler.regularNode.SetRoundData(uniqueKey, RoundData{MerkleRoot: true, RandomNumber: false})
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			handler.regularNode.GetRoundData(uniqueKey)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			handler.regularNode.DeleteRoundsData(uniqueKey)
		}
		done <- true
	}()

	<-done
	<-done
	<-done

	// Test passes if no race condition occurs
}

func TestRegularNodeHandler_DepositAndActivateFlags(t *testing.T) {
	// Test global flags
	depositCalledInThisRun = false
	activateCalledInThisRun = false

	assert.False(t, depositCalledInThisRun)
	assert.False(t, activateCalledInThisRun)

	depositCalledInThisRun = true
	assert.True(t, depositCalledInThisRun)

	activateCalledInThisRun = true
	assert.True(t, activateCalledInThisRun)

	// Reset for other tests
	depositCalledInThisRun = false
	activateCalledInThisRun = false
}

func TestRegularNodeHandler_CompleteCommitFlow(t *testing.T) {
	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set activated operators for GenerateCommit to work
	eth.SetActivatedOperatorsCached([]common.Address{testOp})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Generate commit
	secretValue, cos, cvs, err := commitreveal2.GenerateCommit(round, testOp.Hex())
	require.NoError(t, err)

	// Create commit data
	commitData := utils.CommitData{
		UniqueKey:       uniqueKey,
		Round:           round,
		TrialNum:        trialNum,
		SecretValue:     secretValue,
		Cos:             cos,
		Cvs:             cvs,
		SendToLeader:    false,
		SendCosToLeader: false,
	}

	assert.Equal(t, uniqueKey, commitData.UniqueKey)
	assert.Equal(t, round, commitData.Round)
	assert.Equal(t, trialNum, commitData.TrialNum)
	assert.Equal(t, secretValue, commitData.SecretValue)
	assert.Equal(t, cos, commitData.Cos)
	assert.Equal(t, cvs, commitData.Cvs)

	// Verify CVS is hash of COS + index
	opIndexByte := []byte{uint8(0)}
	recalculatedCvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	assert.True(t, bytes.Equal(cvs[:], recalculatedCvs), "CVS should match hash of COS + index")

	// Simulate sending to leader
	commitData.SendToLeader = true
	assert.True(t, commitData.SendToLeader)

	// Simulate sending COS
	commitData.SendCosToLeader = true
	assert.True(t, commitData.SendCosToLeader)
}

func TestRegularNodeHandler_COSVerification(t *testing.T) {
	round := "100"
	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set activated operators for GenerateCommit to work
	eth.SetActivatedOperatorsCached([]common.Address{testOp})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	secretValue, cos, cvs, err := commitreveal2.GenerateCommit(round, testOp.Hex())
	require.NoError(t, err)

	// Verify that COS is keccak256(abi.encode(secretValue))
	calculatedCos := commitreveal2.Keccak256(commitreveal2.AbiEncode(secretValue[:]))
	assert.True(t, bytes.Equal(cos[:], calculatedCos), "COS should be keccak256(abi.encode(secretValue))")

	// Verify CVS
	opIndexByte := []byte{uint8(0)}
	calculatedCvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	assert.True(t, bytes.Equal(cvs[:], calculatedCvs), "CVS should be hash of COS + index")
}

func TestRegularNodeHandler_MultipleOperatorsScenario(t *testing.T) {
	ops := []common.Address{
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
	}

	eth.SetActivatedOperatorsCached(ops)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Test each operator is activated
	for _, op := range ops {
		assert.True(t, isEOAActivated(op.Hex()), "Each operator should be activated")
	}

	// Test non-existent operator
	assert.False(t, isEOAActivated("0x4444444444444444444444444444444444444444"), "Non-existent operator should not be activated")
}

func TestRegularNodeHandler_MerkleRootSubmittedTime(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Test default
	assert.Nil(t, handler.regularNode.GetSubmitSMonitoringReferenceTime())

	// Set time
	testTime := big.NewInt(1234567890)
	handler.regularNode.SetSubmitSMonitoringReferenceTime(testTime)

	retrieved := handler.regularNode.GetSubmitSMonitoringReferenceTime()
	assert.NotNil(t, retrieved)
	assert.Equal(t, testTime, retrieved)

	// Update time
	newTime := big.NewInt(9876543210)
	handler.regularNode.SetSubmitSMonitoringReferenceTime(newTime)

	retrieved = handler.regularNode.GetSubmitSMonitoringReferenceTime()
	assert.Equal(t, newTime, retrieved)
}

func TestRegularNodeHandler_CommitDataPersistence(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	var secretValue, cos, cvs [32]byte
	copy(secretValue[:], []byte("secret"))
	copy(cos[:], []byte("cos"))
	copy(cvs[:], []byte("cvs"))

	commitData := &utils.CommitData{
		UniqueKey:       uniqueKey,
		Round:           round,
		TrialNum:        trialNum,
		SecretValue:     secretValue,
		Cos:             cos,
		Cvs:             cvs,
		SendToLeader:    false,
		SendCosToLeader: false,
	}

	// Mock expectations
	mockRepo.On("AddCommit", mock.Anything, commitData).Return(nil)

	err := handler.regularNode.AddCommit(context.Background(), commitData)
	assert.NoError(t, err)

	mockRepo.AssertExpectations(t)
}

func TestRegularNodeHandler_CommitDataUpdate(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	commitData := &utils.CommitData{
		UniqueKey:       uniqueKey,
		Round:           round,
		TrialNum:        trialNum,
		SendToLeader:    true,
		SendCosToLeader: false,
	}

	mockRepo.On("UpdateCommit", mock.Anything, commitData).Return(nil)

	err := handler.regularNode.UpdateCommit(context.Background(), commitData)
	assert.NoError(t, err)

	mockRepo.AssertExpectations(t)
}

func TestRegularNodeHandler_GetCommitByRound_NotFound(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	mockRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return((*utils.CommitData)(nil), errors.New("pg: no rows in result set"))

	commit, err := handler.regularNode.GetCommitByRound(context.Background(), "100", "1")

	assert.Error(t, err)
	assert.Nil(t, commit)

	mockRepo.AssertExpectations(t)
}

func TestRegularNodeHandler_GetCommitByRound_Success(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	expectedCommit := &utils.CommitData{
		Round:    "100",
		TrialNum: "1",
	}

	mockRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(expectedCommit, nil)

	commit, err := handler.regularNode.GetCommitByRound(context.Background(), "100", "1")

	assert.NoError(t, err)
	assert.NotNil(t, commit)
	assert.Equal(t, "100", commit.Round)

	mockRepo.AssertExpectations(t)
}

func TestRegularNodeHandler_AddNodeInfo_Success(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockRepo := handler.regularNode.nodeInfoRepository.(*MockNodeInfoRepository)

	nodeInfo := &utils.NodeInfo{
		IP:         "192.168.1.100",
		Port:       "8080",
		PeerID:     "test-peer-id",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}

	mockRepo.On("AddAndUpdateNodeInfo", mock.Anything, nodeInfo).Return(nil)

	err := handler.regularNode.AddNodeInfo(context.Background(), nodeInfo)

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestRegularNodeHandler_AddNodeInfo_Error(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockRepo := handler.regularNode.nodeInfoRepository.(*MockNodeInfoRepository)

	nodeInfo := &utils.NodeInfo{
		IP:         "192.168.1.100",
		Port:       "8080",
		PeerID:     "test-peer-id",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}

	mockRepo.On("AddAndUpdateNodeInfo", mock.Anything, nodeInfo).Return(errors.New("database error"))

	err := handler.regularNode.AddNodeInfo(context.Background(), nodeInfo)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
	mockRepo.AssertExpectations(t)
}

func TestRegularNodeHandler_SignInfoStructure(t *testing.T) {
	signInfo := utils.SignInfo{
		V: "27",
		R: "0x1234567890abcdef",
		S: "0xfedcba0987654321",
	}

	assert.Equal(t, "27", signInfo.V)
	assert.NotEmpty(t, signInfo.R)
	assert.NotEmpty(t, signInfo.S)
}

func TestRegularNodeHandler_SignInfoInCommitRequest(t *testing.T) {
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	var cvs [32]byte
	signature := generateValidSignatureForRegular(eoaAddress, privateKey)

	req := utils.CommitRequest{
		Round:      "100",
		TrialNum:   "1",
		Cvs:        cvs,
		EOAAddress: eoaAddress,
		Signature:  signature,
		Sign: utils.SignInfo{
			V: "27",
			R: hex.EncodeToString([]byte("test-r")),
			S: hex.EncodeToString([]byte("test-s")),
		},
	}

	// Read fields so writes are meaningful and keep vet happy
	assert.Equal(t, "100", req.Round)
	assert.Equal(t, "1", req.TrialNum)
	assert.Equal(t, cvs, req.Cvs)
	assert.Equal(t, eoaAddress, req.EOAAddress)
	assert.Equal(t, signature, req.Signature)
	assert.Equal(t, "27", req.Sign.V)
	assert.NotEmpty(t, req.Sign.R)
	assert.NotEmpty(t, req.Sign.S)
}

func TestRegularNodeHandler_CleanupQueueBehavior(t *testing.T) {
	// Use NewRegularNode to properly initialize cleanup queue
	node := createTestNodeForHandler()

	// Add keys below threshold
	for i := 1; i <= 4; i++ {
		node.EnqueueUniqueKeyForCleanup("round" + string(rune(i)) + ":1")
	}

	assert.Equal(t, 4, node.cleanupQueue.Length())

	// Add 5th key - should trigger cleanup
	node.EnqueueUniqueKeyForCleanup("round5:1")

	assert.True(t, node.cleanupQueue.Length() < 5, "Queue should be reduced after threshold")
}

func TestRegularNodeHandler_checkDepositAmount_ZeroDeposit(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(0), nil
			}
			if method == "s_activationThreshold" {
				return big.NewInt(1000), nil
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isSufficient, err := handler.checkDepositAmount(context.Background(), client, testOp.Hex())

	assert.NoError(t, err)
	assert.False(t, isSufficient, "Zero deposit should be insufficient")
}

func TestRegularNodeHandler_checkDepositAmount_ZeroThreshold(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(100), nil
			}
			if method == "s_activationThreshold" {
				return big.NewInt(0), nil // Zero threshold
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isSufficient, err := handler.checkDepositAmount(context.Background(), client, testOp.Hex())

	assert.NoError(t, err)
	assert.True(t, isSufficient, "Any deposit should be sufficient when threshold is zero")
}

func TestRegularNodeHandler_CompleteDataLifecycle(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set activated operators for GenerateCommit to work
	eth.SetActivatedOperatorsCached([]common.Address{testOp})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Generate commit
	secretValue, cos, cvs, err := commitreveal2.GenerateCommit(round, testOp.Hex())
	require.NoError(t, err)

	// Create initial commit data
	commitData := &utils.CommitData{
		UniqueKey:       uniqueKey,
		Round:           round,
		TrialNum:        trialNum,
		SecretValue:     secretValue,
		Cos:             cos,
		Cvs:             cvs,
		SendToLeader:    false,
		SendCosToLeader: false,
	}

	// Mock adding commit
	mockRepo.On("AddCommit", mock.Anything, commitData).Return(nil)
	err = handler.regularNode.AddCommit(context.Background(), commitData)
	assert.NoError(t, err)

	// Update SendToLeader flag
	commitData.SendToLeader = true
	mockRepo.On("UpdateCommit", mock.Anything, commitData).Return(nil)
	err = handler.regularNode.UpdateCommit(context.Background(), commitData)
	assert.NoError(t, err)

	// Update SendCosToLeader flag
	commitData.SendCosToLeader = true
	mockRepo.On("UpdateCommit", mock.Anything, commitData).Return(nil)
	err = handler.regularNode.UpdateCommit(context.Background(), commitData)
	assert.NoError(t, err)

	mockRepo.AssertExpectations(t)
}

func TestRegularNodeHandler_CommitDataAddError(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	commitData := &utils.CommitData{
		Round:    "100",
		TrialNum: "1",
	}

	mockRepo.On("AddCommit", mock.Anything, commitData).Return(errors.New("database error"))

	err := handler.regularNode.AddCommit(context.Background(), commitData)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestRegularNodeHandler_CommitDataUpdateError(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	commitData := &utils.CommitData{
		Round:        "100",
		TrialNum:     "1",
		SendToLeader: true,
	}

	mockRepo.On("UpdateCommit", mock.Anything, commitData).Return(errors.New("update failed"))

	err := handler.regularNode.UpdateCommit(context.Background(), commitData)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
}

func TestRegularNodeHandler_activateOnChain_Success(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockEth := &MockEthService{
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "activate" {
				return &types.Transaction{}, &bind.TransactOpts{}, nil
			}
			return nil, nil, errors.New("unknown method")
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	err := handler.activateOnChain(context.Background(), abiFilePath)
	assert.NoError(t, err)
}

// TestRegularNodeHandler_activateOnChain_InvalidPrivateKey is removed because
// the client is now created at node initialization, not per-call. Invalid private key
// errors would be caught during NewRegularNode(), not during activateOnChain().

func TestRegularNodeHandler_activateOnChain_TransactionError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockEth := &MockEthService{
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return nil, nil, errors.New("activation transaction failed")
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	err := handler.activateOnChain(context.Background(), abiFilePath)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to activate operator")
}

func TestRegularNodeHandler_sendCosToLeader_StreamCreationError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockHost := new(MockHost)
	mockCommitRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	commitData := utils.CommitData{
		Round:    "100",
		TrialNum: "1",
		Cos:      [32]byte{1, 2, 3},
	}

	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	// Mock stream creation failure
	mockHost.On("NewStream", mock.Anything, testPeerID, mock.Anything).
		Return(nil, errors.New("stream creation failed"))

	// Should not panic, just log error
	handler.sendCosToLeader(ctx, mockHost, testPeerID, commitData, eoaAddress, privateKey)

	mockHost.AssertExpectations(t)
	mockCommitRepo.AssertNotCalled(t, "UpdateCommit")
}

func TestRegularNodeHandler_sendCosToLeader_Success(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockHost := new(MockHost)
	mockStream := newMockStream()
	mockCommitRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	commitData := utils.CommitData{
		UniqueKey: "100:1",
		Round:     "100",
		TrialNum:  "1",
		Cos:       [32]byte{1, 2, 3},
	}

	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	mockHost.On("NewStream", mock.Anything, testPeerID, mock.Anything).
		Return(mockStream, nil)

	mockCommitRepo.On("UpdateCommit", mock.Anything, mock.Anything).Return(nil)

	handler.sendCosToLeader(ctx, mockHost, testPeerID, commitData, eoaAddress, privateKey)

	mockHost.AssertExpectations(t)
	assert.True(t, mockStream.closed)

	// Verify data was written to stream
	assert.NotEmpty(t, mockStream.writeBuffer.Bytes())
}

func TestRegularNodeHandler_sendCosToLeader_UpdateCommitError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockHost := new(MockHost)
	mockStream := newMockStream()
	mockCommitRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	commitData := utils.CommitData{
		UniqueKey: "100:1",
		Round:     "100",
		TrialNum:  "1",
		Cos:       [32]byte{1, 2, 3},
	}

	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	mockHost.On("NewStream", mock.Anything, testPeerID, mock.Anything).
		Return(mockStream, nil)

	mockCommitRepo.On("UpdateCommit", mock.Anything, mock.Anything).
		Return(errors.New("update failed"))

	// Should not panic even if update fails
	handler.sendCosToLeader(ctx, mockHost, testPeerID, commitData, eoaAddress, privateKey)

	mockHost.AssertExpectations(t)
	mockCommitRepo.AssertExpectations(t)
}
func TestRegularNodeHandler_sendRegistrationRequestToLeader_StreamError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	mockHost.On("ID").Return(peer.ID("test-peer"))
	mockHost.On("NewStream", mock.Anything, testPeerID, mock.Anything).
		Return(nil, errors.New("stream error"))
	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("Addrs", testPeerID).Return([]multiaddr.Multiaddr{})
	mockPeerstore.On("AddAddrs", testPeerID, mock.Anything, mock.Anything).Return()

	// Should not panic
	handler.sendRegistrationRequestToLeader(ctx, mockHost, testPeerID, eoaAddress, privateKey)

	mockHost.AssertExpectations(t)
}

func TestRegularNodeHandler_sendRegistrationRequestToLeader_Success(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockHost := new(MockHost)
	mockStream := newMockStream()
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	mockHost.On("ID").Return(peer.ID("test-peer"))
	mockHost.On("NewStream", mock.Anything, testPeerID, mock.Anything).
		Return(mockStream, nil)

	handler.sendRegistrationRequestToLeader(ctx, mockHost, testPeerID, eoaAddress, privateKey)

	mockHost.AssertExpectations(t)
	assert.True(t, mockStream.closed)

	// Verify registration data was sent
	var req utils.RegistrationRequest
	err := json.NewDecoder(mockStream.writeBuffer).Decode(&req)
	assert.NoError(t, err)
	assert.Equal(t, eoaAddress, req.EOAAddress)
	assert.NotEmpty(t, req.Signature)
}

func TestRegularNodeHandler_deposit_GetDepositAmountError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return nil, errors.New("contract call failed")
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ctx := context.Background()
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	txSent, err := handler.deposit(ctx, eoaAddress)

	assert.Error(t, err)
	assert.False(t, txSent)
	assert.Contains(t, err.Error(), "failed to call s_depositAmount")
}

func TestRegularNodeHandler_deposit_SufficientDeposit(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1000), nil // Sufficient
			}
			if method == "s_activationThreshold" {
				return big.NewInt(500), nil
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ctx := context.Background()
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	txSent, err := handler.deposit(ctx, eoaAddress)

	assert.NoError(t, err)
	assert.False(t, txSent) // False because deposit is already sufficient
}

func TestRegularNodeHandler_deposit_GetActivationThresholdError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(500), nil
			}
			if method == "s_activationThreshold" {
				return nil, errors.New("threshold call failed")
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ctx := context.Background()
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	txSent, err := handler.deposit(ctx, eoaAddress)

	assert.Error(t, err)
	assert.False(t, txSent)
	assert.Contains(t, err.Error(), "failed to call s_activationThreshold")
}

func TestRegularNodeHandler_deposit_InsufficientDeposit_BalanceCheckError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockFallbackClient := new(MockFallbackEthClient)
	handler.fallbackEthClient = mockFallbackClient

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(300), nil
			}
			if method == "s_activationThreshold" {
				return big.NewInt(1000), nil
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ctx := context.Background()
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	mockFallbackClient.On("BalanceAt", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("RPC connection timeout"))

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	txSent, err := handler.deposit(ctx, eoaAddress)

	assert.Error(t, err)
	assert.False(t, txSent)
	assert.Contains(t, err.Error(), "failed to fetch account balance")
	assert.Contains(t, err.Error(), "RPC connection timeout")
	mockFallbackClient.AssertExpectations(t)
}

func TestRegularNodeHandler_deposit_InsufficientDeposit_InsufficientAccountBalance(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockFallbackClient := new(MockFallbackEthClient)
	handler.fallbackEthClient = mockFallbackClient

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(200), nil
			}
			if method == "s_activationThreshold" {
				return big.NewInt(1000), nil
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ctx := context.Background()
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	mockFallbackClient.On("BalanceAt", mock.Anything, mock.Anything, mock.Anything).
		Return(big.NewInt(500), nil)

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	txSent, err := handler.deposit(ctx, eoaAddress)

	assert.Error(t, err)
	assert.False(t, txSent)
	assert.Contains(t, err.Error(), "insufficient balance")
	assert.Contains(t, err.Error(), "required 800")
	assert.Contains(t, err.Error(), "available 500")
	mockFallbackClient.AssertExpectations(t)
}

func TestRegularNodeHandler_deposit_InsufficientDeposit_TransactionSuccess(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockFallbackClient := new(MockFallbackEthClient)
	handler.fallbackEthClient = mockFallbackClient

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(400), nil
			}
			if method == "s_activationThreshold" {
				return big.NewInt(1000), nil
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "deposit" {

				assert.Equal(t, big.NewInt(600), value)
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unknown method")
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ctx := context.Background()
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	mockFallbackClient.On("BalanceAt", mock.Anything, mock.Anything, mock.Anything).
		Return(big.NewInt(1000), nil)

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	txSent, err := handler.deposit(ctx, eoaAddress)

	// TESTS: return true, nil
	assert.NoError(t, err)
	assert.True(t, txSent)
	mockFallbackClient.AssertExpectations(t)
}

func TestRegularNodeHandler_deposit_InsufficientDeposit_TransactionError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockFallbackClient := new(MockFallbackEthClient)
	handler.fallbackEthClient = mockFallbackClient

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(100), nil
			}
			if method == "s_activationThreshold" {
				return big.NewInt(1000), nil
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "deposit" {
				return nil, nil, errors.New("transaction reverted: gas too low")
			}
			return nil, nil, errors.New("unknown method")
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ctx := context.Background()
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	mockFallbackClient.On("BalanceAt", mock.Anything, mock.Anything, mock.Anything).
		Return(big.NewInt(2000), nil)

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	txSent, err := handler.deposit(ctx, eoaAddress)

	assert.Error(t, err)
	assert.False(t, txSent)
	assert.Contains(t, err.Error(), "failed to send deposit transaction")
	assert.Contains(t, err.Error(), "gas too low")
	mockFallbackClient.AssertExpectations(t)
}

func TestRegularNodeHandler_deposit_InsufficientDeposit_ExactBalance(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockFallbackClient := new(MockFallbackEthClient)
	handler.fallbackEthClient = mockFallbackClient

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(250), nil
			}
			if method == "s_activationThreshold" {
				return big.NewInt(1000), nil
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "deposit" {
				assert.Equal(t, big.NewInt(750), value)
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unknown method")
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ctx := context.Background()
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	// Mock BalanceAt to return exactly the needed amount
	mockFallbackClient.On("BalanceAt", mock.Anything, mock.Anything, mock.Anything).
		Return(big.NewInt(750), nil) // Exactly 750

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	txSent, err := handler.deposit(ctx, eoaAddress)

	assert.NoError(t, err)
	assert.True(t, txSent)
	mockFallbackClient.AssertExpectations(t)
}

func TestRegularNodeHandler_deposit_InsufficientDeposit_ZeroCurrentDeposit(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockFallbackClient := new(MockFallbackEthClient)
	handler.fallbackEthClient = mockFallbackClient

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(0), nil // Zero current deposit
			}
			if method == "s_activationThreshold" {
				return big.NewInt(1000), nil // Need full 1000
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "deposit" {
				assert.Equal(t, big.NewInt(1000), value)
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unknown method")
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ctx := context.Background()
	privateKey, eoaAddress := createTestKeyPairForRegular()
	require.NotNil(t, privateKey)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	mockFallbackClient.On("BalanceAt", mock.Anything, mock.Anything, mock.Anything).
		Return(big.NewInt(5000), nil)

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	txSent, err := handler.deposit(ctx, eoaAddress)

	assert.NoError(t, err)
	assert.True(t, txSent)
	mockFallbackClient.AssertExpectations(t)
}

func TestRegularNodeHandler_sendCommitToLeader_EmptyRound(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockHost := new(MockHost)
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	commitData := utils.CommitData{
		Round:    "", // Empty round
		TrialNum: "1",
	}

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	handler.sendCommitToLeader(ctx, mockHost, testPeerID, commitData, "", "", "test-eoa")

	mockHost.AssertNotCalled(t, "NewStream")
}

func TestRegularNodeHandler_sendCommitToLeader_EmptyTrialNum(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockHost := new(MockHost)
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	commitData := utils.CommitData{
		Round:    "100",
		TrialNum: "",
	}

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	// Should return early without creating stream
	handler.sendCommitToLeader(ctx, mockHost, testPeerID, commitData, "100", "", "test-eoa")

	mockHost.AssertNotCalled(t, "NewStream")
}

func TestRegularNodeHandler_sendCommitToLeader_PrivateKeyError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockHost := new(MockHost)
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	commitData := utils.CommitData{
		Round:    "100",
		TrialNum: "1",
	}

	os.Setenv("EOA_PRIVATE_KEY", "invalid-key")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	handler.sendCommitToLeader(ctx, mockHost, testPeerID, commitData, "100", "1", "test-eoa")

	mockHost.AssertNotCalled(t, "NewStream")
}

func TestRegularNodeHandler_sendCommitToLeader_StreamCreationError(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockCommitRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	mockHost := new(MockHost)
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	privateKey, eoaAddress := createTestKeyPairForRegular()

	commitData := utils.CommitData{
		UniqueKey: "100:1",
		Round:     "100",
		TrialNum:  "1",
		Cvs:       [32]byte{1, 2, 3},
	}

	os.Setenv("EOA_PRIVATE_KEY", hex.EncodeToString(crypto.FromECDSA(privateKey)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	mockCommitRepo.On("UpdateCommit", mock.Anything, mock.Anything).Return(nil)

	mockHost.On("NewStream", mock.Anything, testPeerID, mock.Anything).
		Return(nil, errors.New("stream error"))

	handler.sendCommitToLeader(ctx, mockHost, testPeerID, commitData, "100", "1", eoaAddress)

	mockHost.AssertExpectations(t)
}

func TestRegularNodeHandler_isEOAActivated_IntegrationStyle(t *testing.T) {
	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	testOp3 := common.HexToAddress("0x3333333333333333333333333333333333333333")

	// Test with multiple operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2, testOp3})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// All should be activated
	assert.True(t, isEOAActivated(testOp1.Hex()))
	assert.True(t, isEOAActivated(testOp2.Hex()))
	assert.True(t, isEOAActivated(testOp3.Hex()))

	// Non-existent should not be activated
	assert.False(t, isEOAActivated("0x4444444444444444444444444444444444444444"))
}

func TestRegularNodeHandler_checkDepositAmount_BoundaryConditions(t *testing.T) {
	handler := createTestRegularNodeHandler()

	tests := []struct {
		name       string
		deposit    int64
		threshold  int64
		wantResult bool
	}{
		{"Exact match", 1000, 1000, true},
		{"Above threshold", 1500, 1000, true},
		{"Below threshold", 500, 1000, false},
		{"Zero deposit", 0, 1000, false},
		{"Zero threshold", 100, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockEth := &MockEthService{
				CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
					if method == "s_depositAmount" {
						return big.NewInt(tt.deposit), nil
					}
					if method == "s_activationThreshold" {
						return big.NewInt(tt.threshold), nil
					}
					return nil, nil
				},
			}

			originalService := eth.Service
			eth.Service = mockEth
			defer func() { eth.Service = originalService }()

			privateKey, _ := crypto.GenerateKey()
			contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
			parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

			client := &utils.Client{
				ContractAddress: contractAddr,
				PrivateKey:      privateKey,
				ContractABI:     parsedABI,
			}

			result, err := handler.checkDepositAmount(context.Background(), client, "0xTest")

			assert.NoError(t, err)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestRegularNodeHandler_checkActivationStatus_EdgeCases(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	tests := []struct {
		name             string
		returnAddresses  []common.Address
		returnError      error
		wantNetworkError bool
		wantActivated    bool
	}{
		{
			name:             "Single activated operator",
			returnAddresses:  []common.Address{testOp},
			returnError:      nil,
			wantNetworkError: false,
			wantActivated:    true,
		},
		{
			name:             "Not in list",
			returnAddresses:  []common.Address{common.HexToAddress("0xOther")},
			returnError:      nil,
			wantNetworkError: false,
			wantActivated:    false,
		},
		{
			name:             "Empty list",
			returnAddresses:  []common.Address{},
			returnError:      nil,
			wantNetworkError: false,
			wantActivated:    false,
		},
		{
			name:             "Network error",
			returnAddresses:  nil,
			returnError:      errors.New("network failed"),
			wantNetworkError: true,
			wantActivated:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockEth := &MockEthService{
				CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
					if tt.returnError != nil {
						return nil, tt.returnError
					}
					return tt.returnAddresses, nil
				},
			}

			originalService := eth.Service
			eth.Service = mockEth
			defer func() { eth.Service = originalService }()

			privateKey, _ := crypto.GenerateKey()
			contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
			parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

			client := &utils.Client{
				ContractAddress: contractAddr,
				PrivateKey:      privateKey,
				ContractABI:     parsedABI,
			}

			isNetworkErr, isActivated := handler.checkActivationStatus(context.Background(), client, testOp.Hex())

			assert.Equal(t, tt.wantNetworkError, isNetworkErr)
			assert.Equal(t, tt.wantActivated, isActivated)
		})
	}
}

func TestRegularNodeHandler_ConcurrentDepositAmountChecks(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1000), nil
			}
			if method == "s_activationThreshold" {
				return big.NewInt(500), nil
			}
			return nil, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			_, err := handler.checkDepositAmount(context.Background(), client, "0xTest")
			assert.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestRegularNodeHandler_ConcurrentActivationChecks(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			return []common.Address{testOp}, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	done := make(chan bool, 10)

	// Run 10 concurrent checks
	for i := 0; i < 10; i++ {
		go func() {
			isNetworkErr, isActivated := handler.checkActivationStatus(context.Background(), client, testOp.Hex())
			assert.False(t, isNetworkErr)
			assert.True(t, isActivated)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestRegularNodeHandler_checkActivationStatus_MultipleMatches(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			return []common.Address{testOp, testOp, testOp}, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isNetworkErr, isActivated := handler.checkActivationStatus(context.Background(), client, testOp.Hex())

	assert.False(t, isNetworkErr)
	assert.True(t, isActivated)
}

func TestRegularNodeHandler_isEOAActivated_CaseSensitivity(t *testing.T) {
	testOp := common.HexToAddress("0xAbCdEf1234567890123456789012345678901234")

	eth.SetActivatedOperatorsCached([]common.Address{testOp})
	defer eth.SetActivatedOperatorsCached([]common.Address{})
	assert.True(t, isEOAActivated(testOp.Hex()))

	lowerCase := "0xabcdef1234567890123456789012345678901234"

	assert.False(t, isEOAActivated(lowerCase))
}
func TestRegularNodeHandler_sendCommitToLeader_GenerateSignatureError(t *testing.T) {
	handler := createTestRegularNodeHandler()

	mockHost := new(MockHost)
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	privateKey, eoaAddress := createTestKeyPairForRegular()

	commitData := utils.CommitData{
		UniqueKey: "100:1",
		Round:     "100",
		TrialNum:  "1",
		Cvs:       [32]byte{1, 2, 3},
	}

	os.Setenv("EOA_PRIVATE_KEY", hex.EncodeToString(crypto.FromECDSA(privateKey)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "invalid")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	handler.sendCommitToLeader(ctx, mockHost, testPeerID, commitData, "100", "1", eoaAddress)

	mockHost.AssertNotCalled(t, "NewStream")
}

func TestRegularNodeHandler_sendCommitToLeader_UpdateCommitError(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockCommitRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	mockHost := new(MockHost)
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	privateKey, eoaAddress := createTestKeyPairForRegular()

	commitData := utils.CommitData{
		UniqueKey: "100:1",
		Round:     "100",
		TrialNum:  "1",
		Cvs:       [32]byte{1, 2, 3},
	}

	os.Setenv("EOA_PRIVATE_KEY", hex.EncodeToString(crypto.FromECDSA(privateKey)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	mockCommitRepo.On("UpdateCommit", mock.Anything, mock.Anything).
		Return(errors.New("update error"))

	handler.sendCommitToLeader(ctx, mockHost, testPeerID, commitData, "100", "1", eoaAddress)

	mockHost.AssertNotCalled(t, "NewStream")
	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNodeHandler_sendCommitToLeader_EncodeSuccess_UpdateFails(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockCommitRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	mockHost := new(MockHost)
	mockStream := newMockStream()
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	privateKey, eoaAddress := createTestKeyPairForRegular()

	commitData := utils.CommitData{
		UniqueKey: "100:1",
		Round:     "100",
		TrialNum:  "1",
		Cvs:       [32]byte{1, 2, 3},
	}

	os.Setenv("EOA_PRIVATE_KEY", hex.EncodeToString(crypto.FromECDSA(privateKey)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	// First update succeeds, second fails
	mockCommitRepo.On("UpdateCommit", mock.Anything, mock.MatchedBy(func(c *utils.CommitData) bool {
		return c.SendToLeader == false // First update (with signature)
	})).Return(nil).Once()

	mockCommitRepo.On("UpdateCommit", mock.Anything, mock.MatchedBy(func(c *utils.CommitData) bool {
		return c.SendToLeader == true // Second update (after send)
	})).Return(errors.New("final update failed")).Once()

	mockHost.On("NewStream", mock.Anything, testPeerID, mock.Anything).
		Return(mockStream, nil)

	handler.sendCommitToLeader(ctx, mockHost, testPeerID, commitData, "100", "1", eoaAddress)

	mockHost.AssertExpectations(t)
	mockCommitRepo.AssertExpectations(t)
	assert.True(t, mockStream.closed)
}

type RegularHandlerTestSuite struct {
	suite.Suite
	db                 *pg.DB
	peerCommitRepo     *database.PeerCommitRepository
	revealOrderRepo    *database.RevealOrderRepository
	regularCommitRepo  *database.RegularCommitRepository
	batchRepo          *database.BatchRepository
	nodeInfoRepo       *database.NodeInfoRepository
	leaderCommitRepo   *database.LeaderCommitRepository
	revealOrderService *commitreveal2.RevealOrderService
	p2pClient          *libp2putils.P2PClient
	regularNodeHandler *RegularNodeHandler
}

// SetupSuite runs once before all tests
func (suite *RegularHandlerTestSuite) SetupSuite() {
	const (
		postgresHost     = "localhost"
		postgresUser     = "postgres"
		postgresPassword = "123"
		postgresDB       = "testdb"
		postgresPort     = "5433"
	)

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		postgresUser, postgresPassword, postgresHost, postgresPort, postgresDB)

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		suite.T().Skip("Skipping test suite: PostgreSQL database not available:", err)
		return
	}
	defer sqlDB.Close()

	err = sqlDB.Ping()
	if err != nil {
		suite.T().Skip("Skipping test suite: PostgreSQL database not available:", err)
		return
	}

	err = database.MigrationsUp(sqlDB)
	require.NoError(suite.T(), err, "Failed to run migrations")

	suite.db = pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", postgresHost, postgresPort),
		User:     postgresUser,
		Password: postgresPassword,
		Database: postgresDB,
	})

	err = suite.db.Ping(context.Background())
	require.NoError(suite.T(), err, "Failed to connect to test database")

	log.Println("Regular handler test suite initialized successfully")
}

// SetupTest runs before each test
func (suite *RegularHandlerTestSuite) SetupTest() {
	suite.peerCommitRepo = database.NewPeerCommitRepository(suite.db)
	suite.revealOrderRepo = database.NewRevealOrderRepository(suite.db)
	suite.regularCommitRepo = database.NewRegularCommitRepository(suite.db)
	suite.batchRepo = database.NewBatchRepository(suite.db)
	suite.nodeInfoRepo = database.NewNodeInfoRepository(suite.db)
	suite.leaderCommitRepo = database.NewLeaderCommitRepository(suite.db)

	suite.revealOrderService = commitreveal2.NewRevealOrderService(
		suite.revealOrderRepo,
		suite.peerCommitRepo,
		suite.leaderCommitRepo,
	)
	suite.p2pClient = libp2putils.NewP2PClient(suite.nodeInfoRepo)

	mockFallbackClient := new(MockFallbackEthClient)
	var err error
	suite.regularNodeHandler, err = NewRegularNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
}

// TearDownTest runs after each test to clean up data
func (suite *RegularHandlerTestSuite) TearDownTest() {
	// Clean up all tables to avoid constraint violations in subsequent tests
	if suite.db != nil {
		_, _ = suite.db.Exec("DELETE FROM commit_data_schemes")
		_, _ = suite.db.Exec("DELETE FROM peer_commit_schemes")
		_, _ = suite.db.Exec("DELETE FROM leader_commit_schemes")
		_, _ = suite.db.Exec("DELETE FROM reveal_order_schemes")
		_, _ = suite.db.Exec("DELETE FROM node_info_schemes")
		_, _ = suite.db.Exec("DELETE FROM broadcast_tracker_schemes")
	}
}

// TearDownSuite runs once after all tests
func (suite *RegularHandlerTestSuite) TearDownSuite() {
	if suite.db != nil {
		suite.db.Close()
		log.Println("Regular handler test suite cleaned up")
	}
}

// TestRegularHandler_NewRegularNodeHandler tests handler initialization
func (suite *RegularHandlerTestSuite) TestRegularHandler_NewRegularNodeHandler() {
	mockFallbackClient := new(MockFallbackEthClient)
	handler, err := NewRegularNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)

	require.NotNil(suite.T(), handler)
	assert.NotNil(suite.T(), handler.regularNode)
	assert.NotNil(suite.T(), handler.regularNode.peerCommitDataRepository)
	assert.NotNil(suite.T(), handler.regularNode.revealOrderRepository)
	assert.NotNil(suite.T(), handler.regularNode.regularCommitRepository)
	assert.NotNil(suite.T(), handler.regularNode.batchRepository)
	assert.NotNil(suite.T(), handler.regularNode.nodeInfoRepository)
	assert.NotNil(suite.T(), handler.regularNode.revealOrderService)
	assert.NotNil(suite.T(), handler.regularNode.p2pClient)
}

// TestRegularHandler_DatabaseOperations tests database CRUD operations
func (suite *RegularHandlerTestSuite) TestRegularHandler_DatabaseOperations() {
	testRound := "db_test_100"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*utils.CommitData)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
	defer suite.db.Model((*utils.CommitData)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	var secretValue, cos, cvs [32]byte
	copy(secretValue[:], []byte("secret123"))
	copy(cos[:], []byte("cos123"))
	copy(cvs[:], []byte("cvs123"))

	commitData := &utils.CommitData{
		UniqueKey:       uniqueKey,
		Round:           testRound,
		TrialNum:        testTrial,
		SecretValue:     secretValue,
		Cos:             cos,
		Cvs:             cvs,
		SendToLeader:    false,
		SendCosToLeader: false,
	}

	// Test Add
	err := suite.regularNodeHandler.regularNode.AddCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	// Test Get
	retrieved, err := suite.regularNodeHandler.regularNode.GetCommitByRound(context.Background(), testRound, testTrial)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), testRound, retrieved.Round)
	assert.Equal(suite.T(), secretValue, retrieved.SecretValue)

	// Test Update
	retrieved.SendToLeader = true
	err = suite.regularNodeHandler.regularNode.UpdateCommit(context.Background(), retrieved)
	require.NoError(suite.T(), err)

	updated, err := suite.regularNodeHandler.regularNode.GetCommitByRound(context.Background(), testRound, testTrial)
	require.NoError(suite.T(), err)
	assert.True(suite.T(), updated.SendToLeader)
}

// TestRegularHandler_NodeInfoOperations tests node info database operations
func (suite *RegularHandlerTestSuite) TestRegularHandler_NodeInfoOperations() {
	testEOA := "0xTestNodeInfo123"

	// Cleanup
	suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address = ?", testEOA).
		Delete()
	defer suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address = ?", testEOA).
		Delete()

	nodeInfo := &utils.NodeInfo{
		IP:         "192.168.1.100",
		Port:       "8080",
		PeerID:     "test-peer-id",
		EOAAddress: testEOA,
	}

	// Test Add
	err := suite.regularNodeHandler.regularNode.AddNodeInfo(context.Background(), nodeInfo)
	require.NoError(suite.T(), err)

	// Test Get all
	allNodes, err := suite.nodeInfoRepo.GetNodeInfos(context.Background())
	require.NoError(suite.T(), err)

	// Find our node
	found := false
	for _, node := range allNodes {
		if node.EOAAddress == testEOA {
			found = true
			assert.Equal(suite.T(), "192.168.1.100", node.IP)
			assert.Equal(suite.T(), "8080", node.Port)
			break
		}
	}
	assert.True(suite.T(), found, "Node info should be found in database")
}

// TestRegularHandler_PeerCommitDataFlow tests peer commit data flow
func (suite *RegularHandlerTestSuite) TestRegularHandler_PeerCommitDataFlow() {
	testRound := "peer_commit_101"
	testTrial := "1"
	testEOA := "0xPeerCommitTest"

	// Cleanup
	suite.db.Model((*database.PeerCommitDataScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testEOA).
		Delete()
	defer suite.db.Model((*database.PeerCommitDataScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testEOA).
		Delete()

	var cvs [32]byte
	copy(cvs[:], []byte("peer_cvs_data"))

	peerCommit := &database.PeerCommitDataScheme{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testEOA,
		Cvs:        cvs[:],
	}

	err := suite.peerCommitRepo.AddPeerCommitData(context.Background(), peerCommit)
	require.NoError(suite.T(), err)

	// Retrieve and verify
	retrieved, err := suite.peerCommitRepo.GetPeerCommitData(context.Background(), testRound, testTrial, testEOA)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), testEOA, retrieved.EOAAddress)
	assert.Equal(suite.T(), cvs[:], retrieved.Cvs)
}

// TestRegularHandlerTestSuite runs the database test suite
func TestRegularHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(RegularHandlerTestSuite))
}

func TestRegularNodeHandler_NewRegularNodeHandler_WithRealDB(t *testing.T) {
	const (
		postgresHost     = "localhost"
		postgresUser     = "postgres"
		postgresPassword = "123"
		postgresDB       = "testdb"
		postgresPort     = "5433"
	)

	db := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", postgresHost, postgresPort),
		User:     postgresUser,
		Password: postgresPassword,
		Database: postgresDB,
	})

	err := db.Ping(context.Background())
	if err != nil {
		t.Skip("Skipping test: PostgreSQL database not available:", err)
		return
	}
	defer db.Close()

	mockFallbackClient := new(MockFallbackEthClient)
	handler, err := NewRegularNodeHandler(mockFallbackClient, db)
	require.NoError(t, err)
	require.NotNil(t, handler)
	assert.NotNil(t, handler.regularNode)
	assert.NotNil(t, handler.regularNode.peerCommitDataRepository)
	assert.NotNil(t, handler.regularNode.revealOrderRepository)
	assert.NotNil(t, handler.regularNode.regularCommitRepository)
	assert.NotNil(t, handler.regularNode.batchRepository)
	assert.NotNil(t, handler.regularNode.nodeInfoRepository)
	assert.NotNil(t, handler.regularNode.revealOrderService)
	assert.NotNil(t, handler.regularNode.p2pClient)
}

func TestRegularNodeHandler_CompleteActivationFlow(t *testing.T) {
	handler := createTestRegularNodeHandler()

	// Setup complete mock flow
	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			switch method {
			case "getActivatedOperators":
				return []common.Address{}, nil // Not activated initially
			case "s_depositAmount":
				return big.NewInt(1000), nil
			case "s_activationThreshold":
				return big.NewInt(500), nil
			default:
				return nil, nil
			}
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, eoaAddress := createTestKeyPairForRegular()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	isNetworkErr, isActivated := handler.checkActivationStatus(context.Background(), client, eoaAddress)
	assert.False(t, isNetworkErr)
	assert.False(t, isActivated)

	isSufficient, err := handler.checkDepositAmount(context.Background(), client, eoaAddress)
	assert.NoError(t, err)
	assert.True(t, isSufficient)
}

func TestRegularNodeHandler_ConcurrentCheckActivationStatus(t *testing.T) {
	handler := createTestRegularNodeHandler()

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthService{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			return []common.Address{testOp}, nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	privateKey, _ := crypto.GenerateKey()
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parsedABI, _ := abi.JSON(bytes.NewReader([]byte(`[]`)))

	client := &utils.Client{
		ContractAddress: contractAddr,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	done := make(chan bool, 5)

	// Run 5 concurrent activation checks
	for i := 0; i < 5; i++ {
		go func() {
			isNetworkErr, isActivated := handler.checkActivationStatus(context.Background(), client, testOp.Hex())
			assert.False(t, isNetworkErr)
			assert.True(t, isActivated)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestRegularNodeHandler_ConcurrentSendCosToLeader(t *testing.T) {
	handler := createTestRegularNodeHandler()
	mockCommitRepo := handler.regularNode.regularCommitRepository.(*MockRegularCommitRepository)

	mockHost := new(MockHost)
	ctx := context.Background()
	testPeerID, _ := peer.Decode("12D3KooWTest")

	privateKey, eoaAddress := createTestKeyPairForRegular()

	done := make(chan bool, 3)

	mockCommitRepo.On("UpdateCommit", mock.Anything, mock.Anything).Return(nil)

	// Simulate concurrent COS sends
	for i := 0; i < 3; i++ {
		go func(idx int) {
			mockStream := newMockStream()
			mockHost.On("NewStream", mock.Anything, testPeerID, mock.Anything).
				Return(mockStream, nil).Once()

			commitData := utils.CommitData{
				UniqueKey: fmt.Sprintf("100:%d", idx),
				Round:     "100",
				TrialNum:  fmt.Sprintf("%d", idx),
				Cos:       [32]byte{byte(idx), 2, 3},
			}

			handler.sendCosToLeader(ctx, mockHost, testPeerID, commitData, eoaAddress, privateKey)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	mockHost.AssertExpectations(t)
}

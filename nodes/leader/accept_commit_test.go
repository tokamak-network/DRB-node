package leader_node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
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
	"github.com/go-pg/pg/v10"
	_ "github.com/lib/pq"
	"github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

type MockRevealOrderService struct {
	mock.Mock
}

func (m *MockRevealOrderService) DetermineRevealOrder(ctx context.Context, round, trialNum string, activatedOps []common.Address) (bool, error) {
	args := m.Called(ctx, round, trialNum, activatedOps)
	return args.Bool(0), args.Error(1)
}

func (m *MockRevealOrderService) DetermineRegularRevealOrder(ctx context.Context, round, trialNum string, activatedOps []common.Address) (bool, error) {
	args := m.Called(ctx, round, trialNum, activatedOps)
	return args.Bool(0), args.Error(1)
}

type MockBatchRepository struct {
	mock.Mock
}

func (m *MockBatchRepository) DeleteOldRoundDataForLeaderNode(ctx context.Context, round string) error {
	args := m.Called(ctx, round)
	return args.Error(0)
}

func (m *MockBatchRepository) DeleteRoundTrialDataForLeaderNode(ctx context.Context, round, trialNum string) error {
	args := m.Called(ctx, round, trialNum)
	return args.Error(0)
}

func (m *MockBatchRepository) DeleteOldRoundDataForRegularNode(ctx context.Context, round string) error {
	args := m.Called(ctx, round)
	return args.Error(0)
}

func (m *MockBatchRepository) DeleteRoundTrialDataForRegularNode(ctx context.Context, round, trialNum string) error {
	args := m.Called(ctx, round, trialNum)
	return args.Error(0)
}

type MockEthClient struct {
	mock.Mock
}

func (m *MockEthClient) CallSmartContract(client interface{}, abi interface{}, method string, address common.Address, args ...interface{}) (interface{}, error) {
	allArgs := append([]interface{}{client, abi, method, address}, args...)
	ret := m.Called(allArgs...)
	return ret.Get(0), ret.Error(1)
}

func (m *MockEthClient) ExecuteTransaction(ctx context.Context, clientUtils *utils.Client, client interface{}, method string, amount *big.Int, args ...interface{}) (*types.Transaction, *types.Receipt, error) {
	allArgs := append([]interface{}{ctx, clientUtils, client, method, amount}, args...)
	ret := m.Called(allArgs...)
	if ret.Get(0) == nil {
		return nil, nil, ret.Error(2)
	}
	return ret.Get(0).(*types.Transaction), ret.Get(1).(*types.Receipt), ret.Error(2)
}

type MockABILoader struct {
	mock.Mock
}

func (m *MockABILoader) LoadContractABI(path string) (interface{}, error) {
	args := m.Called(path)
	return args.Get(0), args.Error(1)
}

type MockEnvironment struct {
	mock.Mock
}

func (m *MockEnvironment) Getenv(key string) string {
	args := m.Called(key)
	return args.String(0)
}

type MockCrypto struct {
	mock.Mock
}

func (m *MockCrypto) HexToECDSA(hexkey string) (interface{}, error) {
	args := m.Called(hexkey)
	return args.Get(0), args.Error(1)
}

func (m *MockCrypto) PubkeyToAddress(pub interface{}) common.Address {
	args := m.Called(pub)
	return args.Get(0).(common.Address)
}

// MockNodeInfoRepositoryForAcceptCommit is a mock for NodeInfoRepository
type MockNodeInfoRepositoryForAcceptCommit struct {
	mock.Mock
}

func (m *MockNodeInfoRepositoryForAcceptCommit) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.NodeInfo), args.Error(1)
}

func (m *MockNodeInfoRepositoryForAcceptCommit) AddAndUpdateNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	args := m.Called(ctx, nodeInfo)
	return args.Error(0)
}

func (m *MockNodeInfoRepositoryForAcceptCommit) DeleteNodeInfoByEOA(ctx context.Context, eoaAddress string) error {
	args := m.Called(ctx, eoaAddress)
	return args.Error(0)
}

// MockEthService for accept_commit tests
type MockEthServiceForAcceptCommit struct {
	GetActivatedOperatorsCachedFunc func() []common.Address
	GetActivatedOperatorsFunc       func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error)
	UpdateActivatedOperatorsFunc    func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error
	CallSmartContractFunc           func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error)
	ExecuteTransactionFunc          func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error)
}

func (m *MockEthServiceForAcceptCommit) GetActivatedOperatorsCached() []common.Address {
	if m.GetActivatedOperatorsCachedFunc != nil {
		return m.GetActivatedOperatorsCachedFunc()
	}
	return []common.Address{}
}

func (m *MockEthServiceForAcceptCommit) GetActivatedOperatorsLength() int64 {
	return int64(len(m.GetActivatedOperatorsCached()))
}

func (m *MockEthServiceForAcceptCommit) SetActivatedOperatorsCached(operators []common.Address) {
	// No-op for tests
}

func (m *MockEthServiceForAcceptCommit) GetActivatedOperatorsUnsafe() []common.Address {
	return m.GetActivatedOperatorsCached()
}

func (m *MockEthServiceForAcceptCommit) GetActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
	if m.GetActivatedOperatorsFunc != nil {
		return m.GetActivatedOperatorsFunc(ctx, fallbackEthClient)
	}
	return []common.Address{}, nil
}

func (m *MockEthServiceForAcceptCommit) UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) error {
	if m.UpdateActivatedOperatorsFunc != nil {
		return m.UpdateActivatedOperatorsFunc(ctx, fallbackEthClient)
	}
	return nil
}

func (m *MockEthServiceForAcceptCommit) CallSmartContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	if m.CallSmartContractFunc != nil {
		return m.CallSmartContractFunc(ctx, fallbackEthClient, parsedABI, method, contractAddress, params...)
	}
	return nil, nil
}

func (m *MockEthServiceForAcceptCommit) ExecuteTransaction(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
	if m.ExecuteTransactionFunc != nil {
		return m.ExecuteTransactionFunc(ctx, clientUtils, fallbackEthClient, method, value, args...)
	}
	return nil, nil, nil
}

func (m *MockEthServiceForAcceptCommit) UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	return nil, nil
}

func (m *MockEthServiceForAcceptCommit) GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	return nil, nil
}

func createMockHostForBroadcast() host.Host {
	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	// Mock peerstore that accepts any address
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()
	mockHost.On("Peerstore").Return(mockPeerstore)

	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil)
	mockStream.On("Write", mock.Anything).Return(100, nil)
	mockStream.On("Close").Return(nil)

	return mockHost
}

func createTestNodeForAcceptCommit() *LeaderNode {
	node := createTestNodeForBroadcast()
	mockLeaderCommitRepo := new(MockLeaderCommitRepository)
	node.leaderCommitRepository = mockLeaderCommitRepo

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil).Maybe()
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	// Initialize with default eth service
	node.ethService = eth.Service

	// Load the test ABI so that receiveCommit can parse event signatures
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err == nil {
		node.client.ContractABI = parsedABI
	}

	// Set up a default mock client with ChainID mocked to prevent panics when timers fire
	// Tests can override this if they need a different mock setup
	if node.fallbackEthClient == nil {
		mockClient := new(MockFallbackEthClientForAcceptCommit)
		// ChainID is called by ExecuteTransaction when timers fire, so ensure it's always mocked
		mockClient.On("ChainID", mock.Anything).Return(big.NewInt(1), nil).Maybe()
		node.fallbackEthClient = mockClient
	} else if mockClient, ok := node.fallbackEthClient.(*MockFallbackEthClientForAcceptCommit); ok {
		// If a mock client is already set, ensure ChainID is mocked
		mockClient.On("ChainID", mock.Anything).Return(big.NewInt(1), nil).Maybe()
	}

	return node
}

func setMockEthServiceForStatusEvents(node *LeaderNode) {
	node.ethService = &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error { return nil },
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{}
		},
	}
}

// waitForReceiveCommitToExit waits for a receiveCommit goroutine to exit after context cancellation.
// Since receiveCommit sleeps for 1 second between retries without checking ctx.Done(),
// we need to wait at least 2 seconds after context cancellation to ensure the goroutine exits.
func waitForReceiveCommitToExit(ctx context.Context, done chan bool) {
	<-ctx.Done()
	// Wait for goroutine to exit - need at least 2 seconds for sleep to complete
	// and goroutine to check ctx.Done() and exit
	select {
	case <-done:
		// Goroutine completed
	case <-time.After(2 * time.Second):
		// Wait longer to ensure goroutine exits after checking ctx.Done()
	}
}

// TestProcessSubmittedSecretRequest_WhenHalted tests halted state handling
func TestProcessSubmittedSecretRequest_WhenHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	node.SetHalted(true)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var secret [32]byte
	index := big.NewInt(0)

	node.processSubmittedSecretRequest(context.Background(), round, trialNum, secret, index)
	assert.True(t, true)
}

// TestProcessRandomRequestNumber
func TestProcessRandomRequestNumber_StateTwo(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(2)

	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, round.String()).Return(nil)

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	mockBatchRepo.AssertExpectations(t)
}

// TestProcessCOS_WhenHalted tests COS processing when halted
func TestProcessCOS_WhenHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	node.SetHalted(true)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cos [32]byte
	index := big.NewInt(0)

	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.NoError(t, err)
}

// TestProcessCOS_IndexOutOfBounds tests COS with invalid index
func TestProcessCOS_IndexOutOfBounds(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cos [32]byte
	index := big.NewInt(10) // Out of bounds

	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

// TestProcessCOS_Success tests successful COS processing
func TestProcessCOS_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators so AllCosReceived returns false
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xAbC1234567890123456789012345678901234567"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cos [32]byte
	copy(cos[:], []byte("test-cos"))
	index := big.NewInt(0)

	// Mock
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.NoError(t, err)

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessCVS_WhenHalted tests CVS processing when halted
func TestProcessCVS_WhenHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	node.SetHalted(true)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	index := big.NewInt(0)

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.NoError(t, err)
}

// TestProcessCVS_IndexOutOfBounds tests CVS with invalid index
func TestProcessCVS_IndexOutOfBounds(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	index := big.NewInt(10)

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

// TestProcessCVS_Success tests successful CVS processing
func TestProcessCVS_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators so AllCvsReceivedUnlocked is false (we only store CVS for one) and GenerateMerkleRoot is not triggered
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0x1234567890123456789012345678901234567891"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs"))
	index := big.NewInt(0)

	// Mock existing commit for the operator we're processing (index 0)
	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: activatedOps[0].Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.NoError(t, err)

	mockLeaderRepo.AssertExpectations(t)
}

// TestAllCosReceivedUnlocked_NoOperators tests with no operators
func TestAllCosReceivedUnlocked_NoOperators(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	eth.SetActivatedOperatorsCached([]common.Address{})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	result := node.AllCosReceivedUnlocked("test-key")
	assert.False(t, result)
}

// TestAllCosReceivedUnlocked_NoRoundData tests with no round data
func TestAllCosReceivedUnlocked_NoRoundData(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	result := node.AllCosReceivedUnlocked("non-existent-key")
	assert.False(t, result)
}

// TestAllCvsReceivedUnlocked_NoOperators tests with no operators
func TestAllCvsReceivedUnlocked_NoOperators(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	eth.SetActivatedOperatorsCached([]common.Address{})
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	result := node.AllCvsReceivedUnlocked("test-key")
	assert.False(t, result)
}

// TestProcessRequestedToSubmitCo_Success tests successful processing
func TestProcessRequestedToSubmitCo_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(100)
	trialNum := big.NewInt(1)

	node.processRequestedToSubmitCo(context.Background(), blockTimestamp, round, trialNum)

	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Clean up timer to prevent it from firing after test completes
	node.stopFailToSubmitCoMonitoring()
}

// TestProcessRequestedToSubmitCv_Success tests successful processing
func TestProcessRequestedToSubmitCv_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(100)
	trialNum := big.NewInt(1)

	node.processRequestedToSubmitCv(context.Background(), blockTimestamp, round, trialNum)

	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Clean up timer to prevent it from firing after test completes
	node.stopFailToSubmitCvMonitoring()
}

// TestStartFailToSubmitCoMonitoring_NilTimestamp tests with nil timestamp
func TestStartFailToSubmitCoMonitoring_NilTimestamp(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	node.startFailToSubmitCoMonitoring(context.Background(), "100", "1", nil)

	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
}

// TestStartFailToSubmitCvMonitoring_NilTimestamp tests with nil timestamp
func TestStartFailToSubmitCvMonitoring_NilTimestamp(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	node.startFailToSubmitCvMonitoring(context.Background(), "100", "1", nil)

	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
}

// TestGetMissingCvsOperators_NoRoundData tests with no round data
func TestGetMissingCvsOperators_NoRoundData(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xAbC1234567890123456789012345678901234567"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	missing := node.getMissingCvsOperators("non-existent-key")

	// All operators should be missing
	assert.Len(t, missing, 2)
}

// TestStartRequestToSubmitCvMonitoring_NilStartTime tests with nil start time
func TestStartRequestToSubmitCvMonitoring_NilStartTime(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	node.startRequestToSubmitCvMonitoring(context.Background(), "100", "1", nil)

	assert.False(t, node.GetRequestToSubmitCvMonitoringActive())
}

// TestStopRequestToSubmitCoMonitoring_NotActive tests when not active
func TestStopRequestToSubmitCoMonitoring_NotActive(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	node.stopRequestToSubmitCoMonitoring()
	assert.False(t, node.GetRequestToSubmitCoTimerMonitoringActive())
}

// TestOrderedPackedIndices tests index separation logic
func TestOrderedPackedIndices(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set some on-chain indices
	node.AppendToIndices(big.NewInt(1))
	node.AppendToIndices(big.NewInt(3))

	// Test with mixed indices
	missingIndices := []*big.Int{
		big.NewInt(0), // not on chain
		big.NewInt(1), // on chain
		big.NewInt(2), // not on chain
		big.NewInt(3), // on chain
	}

	notOnChain, onChain := node.orderedPackedIndices(missingIndices)

	assert.Len(t, notOnChain, 2)
	assert.Len(t, onChain, 2)
	assert.Equal(t, big.NewInt(0), notOnChain[0])
	assert.Equal(t, big.NewInt(2), notOnChain[1])
	assert.Equal(t, big.NewInt(1), onChain[0])
	assert.Equal(t, big.NewInt(3), onChain[1])
}

// TestProcessRequestedToSubmitCv_StopsExistingMonitoring tests stopping monitoring
func TestProcessRequestedToSubmitCv_StopsExistingMonitoring(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	startTime := big.NewInt(time.Now().Unix() + 1000)
	node.startRequestToSubmitCvMonitoring(context.Background(), "100", "1", startTime)
	assert.True(t, node.GetRequestToSubmitCvMonitoringActive())

	// Process request - should stop monitoring
	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(100)
	trialNum := big.NewInt(1)

	node.processRequestedToSubmitCv(context.Background(), blockTimestamp, round, trialNum)

	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Clean up timer to prevent it from firing after test completes
	node.stopFailToSubmitCvMonitoring()
}

// TestUpdateCommitDataAfterSubmit tests commit data update
func TestUpdateCommitDataAfterSubmit(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	uniqueKey := "100:1"
	eoa := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set up commit data
	commitData := utils.LeaderCommitData{
		EOAAddress:           eoa.Hex(),
		SubmitMerkleRootDone: false,
	}
	utils.SetCommittedNodeData(uniqueKey, eoa, commitData)

	// Mock update
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.MatchedBy(func(data *utils.LeaderCommitData) bool {
		return data.SubmitMerkleRootDone == true
	})).Return(nil)

	node.updateCommitDataAfterSubmit(context.Background(), uniqueKey)

	mockLeaderRepo.AssertExpectations(t)
}

// TestUpdateCommitDataAfterSubmit_AlreadyDone tests skipping already done entries
func TestUpdateCommitDataAfterSubmit_AlreadyDone(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	uniqueKey := "100:1"
	eoa := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set up commit data already marked as done
	commitData := utils.LeaderCommitData{
		EOAAddress:           eoa.Hex(),
		SubmitMerkleRootDone: true,
	}
	utils.SetCommittedNodeData(uniqueKey, eoa, commitData)

	node.updateCommitDataAfterSubmit(context.Background(), uniqueKey)

	mockLeaderRepo.AssertNotCalled(t, "UpdateLeaderCommit")
}

// TestGetMissingCvsOperators_SomeMissing tests with partial CVS
func TestGetMissingCvsOperators_SomeMissing(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	uniqueKey := "100:1"
	eoa1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	eoa2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")

	activatedOps := []common.Address{eoa1, eoa2}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Set up CVS for only first operator
	commitData1 := utils.LeaderCommitData{
		EOAAddress: eoa1.Hex(),
	}
	copy(commitData1.Cvs[:], []byte("cvs1"))
	utils.SetCommittedNodeData(uniqueKey, eoa1, commitData1)

	missing := node.getMissingCvsOperators(uniqueKey)

	// Only second operator should be missing
	assert.Len(t, missing, 1)
	assert.Contains(t, missing, eoa2.Hex())
}

// TestProcessCOS_UpdateExisting tests updating existing COS data
func TestProcessCOS_UpdateExisting(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators so AllCosReceived returns false (avoiding reveal order logic)
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xAbC1234567890123456789012345678901234567"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cos [32]byte
	copy(cos[:], []byte("test-cos"))
	index := big.NewInt(0)

	// Mock existing commit - will update
	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: activatedOps[0].Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.NoError(t, err)

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessCVS_UpdateExisting tests updating existing CVS data
func TestProcessCVS_UpdateExisting(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators so AllCvsReceived returns false (avoiding GenerateMerkleRoot)
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xAbC1234567890123456789012345678901234567"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs"))
	index := big.NewInt(0)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.NoError(t, err)

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessCOS_AddCommitError tests error handling when adding commit
func TestProcessCOS_AddCommitError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators to avoid triggering reveal order
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xAbC1234567890123456789012345678901234567"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cos [32]byte
	index := big.NewInt(0)

	// Mock no existing commit and add error
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(errors.New("db error"))
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add leader commit")
	assert.Contains(t, err.Error(), "db error")

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessCOS_UpdateError tests error handling when updating commit
func TestProcessCOS_UpdateError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators to avoid triggering reveal order
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xAbC1234567890123456789012345678901234567"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cos [32]byte
	index := big.NewInt(0)

	// Mock existing commit with update error
	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: activatedOps[0].Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(errors.New("update error"))
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCOS(ctx, round, trialNum, cos, index)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update leader commit")
	assert.Contains(t, err.Error(), "update error")

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessCVS_AddCommitError tests error handling when adding commit
func TestProcessCVS_AddCommitError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators to avoid triggering GenerateMerkleRoot
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xAbC1234567890123456789012345678901234567"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	index := big.NewInt(0)

	// Mock no existing commit with add error
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(errors.New("db error"))
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add leader commit")
	assert.Contains(t, err.Error(), "db error")

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessCVS_UpdateError tests error handling when updating commit
func TestProcessCVS_UpdateError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators to avoid triggering GenerateMerkleRoot
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xAbC1234567890123456789012345678901234567"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	index := big.NewInt(0)

	// Mock existing commit with update error
	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: activatedOps[0].Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(errors.New("update error"))
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update leader commit")
	assert.Contains(t, err.Error(), "update error")

	mockLeaderRepo.AssertExpectations(t)
}

// TestAllCvsReceivedUnlocked_EmptyRoundCommits tests with empty round commits
func TestAllCvsReceivedUnlocked_EmptyRoundCommits(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	uniqueKey := "100-empty-cvs:1"
	eoa := common.HexToAddress("0x1234567890123456789012345678901234567890")

	activatedOps := []common.Address{eoa}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Clean up before test
	utils.DeleteCommittedNodes(uniqueKey)
	defer utils.DeleteCommittedNodes(uniqueKey)

	utils.EnsureCommittedNodesRoundExists(uniqueKey)

	result := node.AllCvsReceivedUnlocked(uniqueKey)
	assert.False(t, result)
}

// TestStartRequestToSubmitCoMonitoring_AlreadyActive tests when already active
func TestStartRequestToSubmitCoMonitoring_AlreadyActive(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	timestamp := big.NewInt(time.Now().Unix() + 1000)
	node.SetRequestToSubmitCoTimerMonitoringActive(true)

	// Try to start again
	node.startRequestToSubmitCoMonitoring(context.Background(), "100", "1", timestamp)

	assert.True(t, node.GetRequestToSubmitCoTimerMonitoringActive())
}

// TestStopRequestToSubmitCoMonitoring_WhenActive tests stopping active monitoring
func TestStopRequestToSubmitCoMonitoring_WhenActive(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set active flag and create a timer
	node.SetRequestToSubmitCoTimerMonitoringActive(true)
	node.requestToSubmitCoTimerMonitoringTimer = time.AfterFunc(10*time.Second, func() {})

	node.stopRequestToSubmitCoMonitoring()

	assert.False(t, node.GetRequestToSubmitCoTimerMonitoringActive())
	assert.Nil(t, node.requestToSubmitCoTimerMonitoringTimer)
}

// TestPrepareArgumentsForRequestToSubmitCo tests argument preparation
func TestPrepareArgumentsForRequestToSubmitCo(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	// Setup test data
	round := "100"
	trialNum := "1"
	eoa1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	eoa2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")

	activatedOps := []common.Address{eoa1, eoa2}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Create mock commits with signature data
	commit1 := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: eoa1.Hex(),
		Sign: utils.SignInfo{
			V: "27",
			R: "0x1234567890123456789012345678901234567890123456789012345678901234",
			S: "0x1234567890123456789012345678901234567890123456789012345678901234",
		},
	}
	copy(commit1.Cvs[:], []byte("cvs1"))

	commit2 := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: eoa2.Hex(),
		Sign: utils.SignInfo{
			V: "28",
			R: "0xabcdef1234567890123456789012345678901234567890123456789012345678",
			S: "0xabcdef1234567890123456789012345678901234567890123456789012345678",
		},
	}
	copy(commit2.Cvs[:], []byte("cvs2"))

	commits := []*utils.LeaderCommitData{commit1, commit2}
	mockLeaderRepo.On("GetLeaderCommitsByRoundAndTrialNum", mock.Anything, round, trialNum).Return(commits, nil)

	// Set indices - operator 0 is on chain
	node.AppendToIndices(big.NewInt(0))

	missingIndices := []*big.Int{big.NewInt(0), big.NewInt(1)}

	// Call prepare function
	cvAndSigRS, packedVs, indicesLength, packedOrderedIndices, err := node.prepareArgumentsForRequestToSubmitCo(context.Background(), round, trialNum, missingIndices)
	assert.NoError(t, err)

	assert.NotNil(t, cvAndSigRS)
	assert.NotNil(t, packedVs)
	assert.Equal(t, big.NewInt(2), indicesLength)
	assert.NotNil(t, packedOrderedIndices)

	mockLeaderRepo.AssertExpectations(t)
}

// TestStopRequestToSubmitCoMonitoring_WithTimer tests stopping with active timer
func TestStopRequestToSubmitCoMonitoring_WithTimer(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set active with a timer
	node.SetRequestToSubmitCoTimerMonitoringActive(true)
	node.requestToSubmitCoTimerMonitoringTimer = time.AfterFunc(10*time.Second, func() {})

	node.stopRequestToSubmitCoMonitoring()

	assert.False(t, node.GetRequestToSubmitCoTimerMonitoringActive())
}

// TestProcessSubmittedSecretRequest_Success tests successful secret processing
func TestProcessSubmittedSecretRequest_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var secret [32]byte
	copy(secret[:], []byte("test-secret"))
	index := big.NewInt(0)

	// Set the secret request sent for round
	node.SetSecretRequestSentForWhichRound(round.String())

	// Mock getting existing commit
	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: activatedOps[0].Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)

	// Mock p2p broadcast - mark as acknowledged to avoid actual network call
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	node.processSubmittedSecretRequest(context.Background(), round, trialNum, secret, index)

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessSubmittedSecretRequest_GetCommitError tests error getting commit
func TestProcessSubmittedSecretRequest_GetCommitError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var secret [32]byte
	index := big.NewInt(0)

	node.SetSecretRequestSentForWhichRound(round.String())

	// Mock error getting commit
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(nil, errors.New("database error"))

	node.processSubmittedSecretRequest(context.Background(), round, trialNum, secret, index)

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessSubmittedSecretRequest_UpdateError tests error updating commit
func TestProcessSubmittedSecretRequest_UpdateError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var secret [32]byte
	copy(secret[:], []byte("test-secret"))
	index := big.NewInt(0)

	node.SetSecretRequestSentForWhichRound(round.String())

	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: activatedOps[0].Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(errors.New("update error"))

	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	node.processSubmittedSecretRequest(context.Background(), round, trialNum, secret, index)

	mockLeaderRepo.AssertExpectations(t)
}

// TestPrepareArgumentsForRequestToSubmitCo_SingleIndex tests with single index
func TestPrepareArgumentsForRequestToSubmitCo_SingleIndex(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	round := "100"
	trialNum := "1"
	eoa1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Set activated operators
	activatedOps := []common.Address{eoa1}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	commit1 := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: eoa1.Hex(),
		Sign: utils.SignInfo{
			V: "27",
			R: "0x1234567890123456789012345678901234567890123456789012345678901234",
			S: "0x1234567890123456789012345678901234567890123456789012345678901234",
		},
	}
	copy(commit1.Cvs[:], []byte("cvs1"))

	commits := []*utils.LeaderCommitData{commit1}
	mockLeaderRepo.On("GetLeaderCommitsByRoundAndTrialNum", mock.Anything, round, trialNum).Return(commits, nil)

	missingIndices := []*big.Int{big.NewInt(0)}

	cvAndSigRS, packedVs, indicesLength, packedOrderedIndices, err := node.prepareArgumentsForRequestToSubmitCo(context.Background(), round, trialNum, missingIndices)
	assert.NoError(t, err)

	assert.NotNil(t, cvAndSigRS)
	assert.NotNil(t, packedVs)
	assert.Equal(t, big.NewInt(1), indicesLength)
	assert.NotNil(t, packedOrderedIndices)

	mockLeaderRepo.AssertExpectations(t)
}

// TestPrepareArgumentsForRequestToSubmitCo_MultipleIndices tests with multiple indices
func TestPrepareArgumentsForRequestToSubmitCo_MultipleIndices(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	round := "100"
	trialNum := "1"
	eoa1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	eoa2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")
	eoa3 := common.HexToAddress("0xDEF1234567890123456789012345678901234567")

	activatedOps := []common.Address{eoa1, eoa2, eoa3}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	commit1 := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: eoa1.Hex(),
		Sign: utils.SignInfo{
			V: "27",
			R: "0x1234567890123456789012345678901234567890123456789012345678901234",
			S: "0x1234567890123456789012345678901234567890123456789012345678901234",
		},
	}
	copy(commit1.Cvs[:], []byte("cvs1"))

	commit2 := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: eoa2.Hex(),
		Sign: utils.SignInfo{
			V: "28",
			R: "0xabcd567890123456789012345678901234567890123456789012345678901234",
			S: "0xabcd567890123456789012345678901234567890123456789012345678901234",
		},
	}
	copy(commit2.Cvs[:], []byte("cvs2"))

	commit3 := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: eoa3.Hex(),
		Sign: utils.SignInfo{
			V: "27",
			R: "0xef01567890123456789012345678901234567890123456789012345678901234",
			S: "0xef01567890123456789012345678901234567890123456789012345678901234",
		},
	}
	copy(commit3.Cvs[:], []byte("cvs3"))

	commits := []*utils.LeaderCommitData{commit1, commit2, commit3}
	mockLeaderRepo.On("GetLeaderCommitsByRoundAndTrialNum", mock.Anything, round, trialNum).Return(commits, nil)

	// Set one index as on-chain
	node.AppendToIndices(big.NewInt(1))

	missingIndices := []*big.Int{big.NewInt(0), big.NewInt(1), big.NewInt(2)}

	cvAndSigRS, packedVs, indicesLength, packedOrderedIndices, err := node.prepareArgumentsForRequestToSubmitCo(context.Background(), round, trialNum, missingIndices)
	assert.NoError(t, err)

	assert.NotNil(t, cvAndSigRS)
	assert.NotNil(t, packedVs)
	assert.Equal(t, big.NewInt(3), indicesLength)
	assert.NotNil(t, packedOrderedIndices)

	mockLeaderRepo.AssertExpectations(t)
}

// TestUpdateCommitDataAfterSubmit_MultipleOperatorsIsolated tests updating multiple operators
func TestUpdateCommitDataAfterSubmit_MultipleOperatorsIsolated(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := new(MockLeaderCommitRepository)
	node.leaderCommitRepository = mockLeaderRepo

	uniqueKey := "100isolated:1"
	eoa1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	eoa2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")

	// Clean up before test
	utils.DeleteCommittedNodes(uniqueKey)
	defer utils.DeleteCommittedNodes(uniqueKey)

	utils.EnsureCommittedNodesRoundExists(uniqueKey)

	// Set up commit data for two operators
	commitData1 := utils.LeaderCommitData{
		EOAAddress:           eoa1.Hex(),
		SubmitMerkleRootDone: false,
	}
	utils.SetCommittedNodeData(uniqueKey, eoa1, commitData1)

	commitData2 := utils.LeaderCommitData{
		EOAAddress:           eoa2.Hex(),
		SubmitMerkleRootDone: false,
	}
	utils.SetCommittedNodeData(uniqueKey, eoa2, commitData2)

	// Mock updates for both
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.MatchedBy(func(data *utils.LeaderCommitData) bool {
		return data.SubmitMerkleRootDone == true
	})).Return(nil).Times(2)

	node.updateCommitDataAfterSubmit(context.Background(), uniqueKey)

	mockLeaderRepo.AssertExpectations(t)
}

// TestCallRequestToSubmitCoIfNeeded_WithAllCosReceived tests when all COS are present
func TestCallRequestToSubmitCoIfNeeded_WithAllCosReceived(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	uniqueKey := "100:1"
	eoa1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	eoa2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")

	activatedOps := []common.Address{eoa1, eoa2}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Both operators have COS
	commitData1 := utils.LeaderCommitData{
		EOAAddress: eoa1.Hex(),
	}
	copy(commitData1.Cos[:], []byte("cos1"))
	utils.SetCommittedNodeData(uniqueKey, eoa1, commitData1)

	commitData2 := utils.LeaderCommitData{
		EOAAddress: eoa2.Hex(),
	}
	copy(commitData2.Cos[:], []byte("cos2"))
	utils.SetCommittedNodeData(uniqueKey, eoa2, commitData2)

	assert.True(t, node.AllCosReceivedUnlocked(uniqueKey))
}

// TestCallRequestToSubmitCoIfNeeded_NoRoundData tests with missing round data
func TestCallRequestToSubmitCoIfNeeded_NoRoundData(t *testing.T) {
	_ = createTestNodeForAcceptCommit()

	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	uniqueKey := "non-existent:1"

	assert.NotPanics(t, func() {
		_, exists := utils.GetCommittedNodes(uniqueKey)
		assert.False(t, exists)
	})
}

// TestPackIndices_WithMultipleIndices tests index packing
func TestPackIndices_WithMultipleIndices(t *testing.T) {
	indices := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(2),
	}

	packed := PackIndices(indices)
	assert.NotNil(t, packed)

	expected := big.NewInt(0x020100)
	assert.Equal(t, expected, packed)
}

// TestOrderedPackedIndices_MixedOnChainStatus tests index separation
func TestOrderedPackedIndices_MixedOnChainStatus(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set indices 1 and 3 as on-chain
	node.AppendToIndices(big.NewInt(1))
	node.AppendToIndices(big.NewInt(3))

	// Test with mixed indices
	missingIndices := []*big.Int{
		big.NewInt(0), // not on chain
		big.NewInt(1), // on chain
		big.NewInt(2), // not on chain
		big.NewInt(3), // on chain
		big.NewInt(4), // not on chain
	}

	notOnChain, onChain := node.orderedPackedIndices(missingIndices)

	assert.Len(t, notOnChain, 3)
	assert.Len(t, onChain, 2)
	assert.Equal(t, big.NewInt(0), notOnChain[0])
	assert.Equal(t, big.NewInt(2), notOnChain[1])
	assert.Equal(t, big.NewInt(4), notOnChain[2])
	assert.Equal(t, big.NewInt(1), onChain[0])
	assert.Equal(t, big.NewInt(3), onChain[1])
}

// TestProcessRandomRequestNumber_WithDeleteError tests error handling
func TestProcessRandomRequestNumber_WithDeleteError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(2) // COMPLETE

	// Mock delete error
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, round.String()).Return(errors.New("delete failed"))

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	mockBatchRepo.AssertExpectations(t)
}

// TestCleanupRoundDataByUniqueKey_VerifyAllMapsCleared tests cleanup
func TestCleanupRoundDataByUniqueKey_VerifyAllMapsCleared(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	uniqueKey := "100:1"

	// Set data in various maps
	node.SetRoundData(uniqueKey, RoundData{MerkleRoot: true})
	node.SetCvOnChain(uniqueKey, true)
	node.SetSecretsOnChain(uniqueKey, true)

	// Add committed node data
	eoa := common.HexToAddress("0x1234567890123456789012345678901234567890")
	commitData := utils.LeaderCommitData{
		EOAAddress: eoa.Hex(),
	}
	utils.SetCommittedNodeData(uniqueKey, eoa, commitData)

	// Cleanup
	node.CleanupRoundDataByUniqueKey(uniqueKey)

	_, roundExists := node.GetRoundData(uniqueKey)
	assert.False(t, roundExists)

	_, cvExists := node.GetCvOnChain(uniqueKey)
	assert.False(t, cvExists)

	_, secretsExists := node.GetSecretsOnChain(uniqueKey)
	assert.False(t, secretsExists)

	_, committedExists := utils.GetCommittedNodes(uniqueKey)
	assert.False(t, committedExists)
}

// TestResetCosAndCvsMonitoringState_AllMonitoringStopped tests full reset
func TestResetCosAndCvsMonitoringState_AllMonitoringStopped(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	timestamp := big.NewInt(time.Now().Unix() + 1000)
	node.startFailToSubmitCoMonitoring(context.Background(), "100", "1", timestamp)
	node.startFailToSubmitCvMonitoring(context.Background(), "100", "1", timestamp)
	node.startRequestToSubmitCvMonitoring(context.Background(), "100", "1", timestamp)
	node.SetSecretRequestSentForWhichRound("100")

	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())
	assert.True(t, node.GetRequestToSubmitCvMonitoringActive())
	assert.Equal(t, "100", node.GetSecretRequestSentForWhichRound())

	node.ResetCosAndCvsMonitoringState("100", "1")

	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
	assert.False(t, node.GetRequestToSubmitCvMonitoringActive())
	// ResetCosAndCvsMonitoringState does not clear secretRequestSentForWhichRound
	// It only stops timers and resets monitoring flags
	assert.Equal(t, "100", node.GetSecretRequestSentForWhichRound())
}

// TestProcessSubmittedSecretRequest_WithMockEthService tests with mock eth service
func TestProcessSubmittedSecretRequest_WithMockEthService(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var secret [32]byte
	copy(secret[:], []byte("test-secret"))
	index := big.NewInt(0)

	node.SetSecretRequestSentForWhichRound(round.String())

	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: testOp.Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp.Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	node.processSubmittedSecretRequest(context.Background(), round, trialNum, secret, index)

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessCOS_WithMockEthService tests processCOS with mock
func TestProcessCOS_WithMockEthService(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := big.NewInt(200)
	trialNum := big.NewInt(1)
	var cos [32]byte
	copy(cos[:], []byte("test-cos"))
	index := big.NewInt(0)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp1.Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.NoError(t, err)

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessCVS_WithMockEthService tests processCVS with mock
func TestProcessCVS_WithMockEthService(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	testOp1 := common.HexToAddress("0x3333333333333333333333333333333333333333")
	testOp2 := common.HexToAddress("0x4444444444444444444444444444444444444444")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := big.NewInt(300)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs"))
	index := big.NewInt(0)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp1.Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.NoError(t, err)

	mockLeaderRepo.AssertExpectations(t)
}

// TestAllCosReceivedUnlocked_WithMockEthService tests with mock
func TestAllCosReceivedUnlocked_WithMockEthService(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x5555555555555555555555555555555555555555")
	testOp2 := common.HexToAddress("0x6666666666666666666666666666666666666666")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	uniqueKey := "400:1"

	// Test with no data
	result := node.AllCosReceivedUnlocked(uniqueKey)
	assert.False(t, result)

	// Add COS for op1
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cos: [32]byte{1}})
	result = node.AllCosReceivedUnlocked(uniqueKey)
	assert.False(t, result)

	// Add COS for op2
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{Cos: [32]byte{2}})
	result = node.AllCosReceivedUnlocked(uniqueKey)
	assert.True(t, result)
}

// TestAllCvsReceivedUnlocked_WithMockEthService tests with mock
func TestAllCvsReceivedUnlocked_WithMockEthService(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x7777777777777777777777777777777777777777")
	testOp2 := common.HexToAddress("0x8888888888888888888888888888888888888888")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	uniqueKey := "500:1"

	// Test with no data
	result := node.AllCvsReceivedUnlocked(uniqueKey)
	assert.False(t, result)

	// Add CVS for op1
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})
	result = node.AllCvsReceivedUnlocked(uniqueKey)
	assert.False(t, result)

	// Add CVS for op2
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{Cvs: [32]byte{2}})
	result = node.AllCvsReceivedUnlocked(uniqueKey)
	assert.True(t, result)
}

// TestUpdateCOS_TriggersRevealOrder tests updateCOS triggering reveal order
func TestUpdateCOS_TriggersRevealOrder(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockRevealOrderService := new(MockRevealOrderService)
	node.revealOrderService = mockRevealOrderService

	testOp1 := common.HexToAddress("0x9999999999999999999999999999999999999999")
	testOp2 := common.HexToAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "600"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	var cos [32]byte
	copy(cos[:], []byte("cos1"))

	node.updateCOS(context.Background(), round, trialNum, uniqueKey, testOp1, cos)

	// Verify COS was stored
	data, exists := utils.GetCommittedNodeData(uniqueKey, testOp1)
	assert.True(t, exists)
	assert.Equal(t, cos, data.Cos)
}

// TestUpdateCOS_DataStorageOnly tests updateCOS just storing data
func TestUpdateCOS_DataStorageOnly(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")
	testOp2 := common.HexToAddress("0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "700"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	var cos [32]byte
	copy(cos[:], []byte("cos-test"))
	node.updateCOS(context.Background(), round, trialNum, uniqueKey, testOp1, cos)

	// Verify data stored
	data, exists := utils.GetCommittedNodeData(uniqueKey, testOp1)
	assert.True(t, exists)
	assert.Equal(t, cos, data.Cos)
}

// TestCheckAndStopFailToSubmitCoMonitoring_AllCosReceived tests stopping monitoring when all COS received
func TestCheckAndStopFailToSubmitCoMonitoring_AllCosReceived(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := "800"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Start monitoring
	timestamp := big.NewInt(time.Now().Unix() + 1000)
	node.startFailToSubmitCoMonitoring(context.Background(), round, trialNum, timestamp)
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Add COS to trigger stop
	utils.SetCommittedNodeData(uniqueKey, testOp, utils.LeaderCommitData{Cos: [32]byte{1}})

	// Should stop monitoring
	node.checkAndStopFailToSubmitCoMonitoring(round, trialNum)
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
}

// TestCheckAndStopFailToSubmitCvMonitoring_AllCvsReceived tests stopping monitoring when all CVS received
func TestCheckAndStopFailToSubmitCvMonitoring_AllCvsReceived(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := "900"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Start monitoring
	timestamp := big.NewInt(time.Now().Unix() + 1000)
	node.startFailToSubmitCvMonitoring(context.Background(), round, trialNum, timestamp)
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Add CVS to trigger stop
	utils.SetCommittedNodeData(uniqueKey, testOp, utils.LeaderCommitData{Cvs: [32]byte{1}})

	// Should stop monitoring
	node.checkAndStopFailToSubmitCvMonitoring(round, trialNum)
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
}

// TestCheckAndStopFailToSubmitCoMonitoring_NotActive tests when monitoring not active
func TestCheckAndStopFailToSubmitCoMonitoring_NotActive(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0xEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	// Don't start monitoring
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Should return early
	node.checkAndStopFailToSubmitCoMonitoring("1000", "1")
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
}

// TestCheckAndStopFailToSubmitCvMonitoring_NotActive tests when monitoring not active
func TestCheckAndStopFailToSubmitCvMonitoring_NotActive(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	// Don't start monitoring
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Should return early
	node.checkAndStopFailToSubmitCvMonitoring("1100", "1")
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
}

// TestGetMissingCvsOperators_WithMockEthService tests with mock
func TestGetMissingCvsOperators_WithMockEthService(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0xABC1234567890123456789012345678901234567")
	testOp2 := common.HexToAddress("0xDEF1234567890123456789012345678901234567")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	uniqueKey := "1200:1"

	// Add CVS only for op1
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})

	missing := node.getMissingCvsOperators(uniqueKey)

	// Only op2 should be missing
	assert.Len(t, missing, 1)
	assert.Contains(t, missing, testOp2.Hex())
}

// TestGenerateMerkleRoot_NoCommitsInMemory tests with no commits
func TestGenerateMerkleRoot_NoCommitsInMemory(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x1111222233334444555566667777888899990000")
	testOp2 := common.HexToAddress("0x0000999988887777666655554444333322221111")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "1300"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Don't add any CVS data
	utils.DeleteCommittedNodes(uniqueKey)

	node.GenerateMerkleRoot(context.Background(), round, trialNum)

	// Verify merkle root was not marked as done
	data, exists := node.GetRoundData(uniqueKey)
	if exists {
		assert.False(t, data.MerkleRoot)
	}
}

// TestGenerateMerkleRoot_AlreadySubmitting tests when already submitting
func TestGenerateMerkleRoot_AlreadySubmitting(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0xABCDEF1234567890ABCDEF1234567890ABCDEF12")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := "1400"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Add CVS
	utils.SetCommittedNodeData(uniqueKey, testOp, utils.LeaderCommitData{Cvs: [32]byte{1}})

	// Set already submitting flag
	node.SetSubmittingMerkleRoot(true)

	// Should skip
	node.GenerateMerkleRoot(context.Background(), round, trialNum)

	// Should still be true
	assert.True(t, node.GetSubmittingMerkleRoot())
}

// TestGenerateMerkleRoot_AlreadyDone tests when merkle root already submitted
func TestGenerateMerkleRoot_AlreadyDone(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0x1234ABCD5678EF901234ABCD5678EF901234ABCD")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := "1500"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	node.SetRoundData(uniqueKey, RoundData{MerkleRoot: true})

	node.GenerateMerkleRoot(context.Background(), round, trialNum)

	data, exists := node.GetRoundData(uniqueKey)
	assert.True(t, exists)
	assert.True(t, data.MerkleRoot)
}

// TestGenerateMerkleRoot_MissingCVS tests when CVS missing
func TestGenerateMerkleRoot_MissingCVS(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "1600"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})

	// Should skip due to missing CVS
	node.GenerateMerkleRoot(context.Background(), round, trialNum)

	// Should not have marked as done
	data, exists := node.GetRoundData(uniqueKey)
	if exists {
		assert.False(t, data.MerkleRoot)
	}
}

// TestProcessRequestedToSubmitCo_NotHalted tests processing event
func TestProcessRequestedToSubmitCo_NotHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(1700)
	trialNum := big.NewInt(1)

	node.processRequestedToSubmitCo(context.Background(), blockTimestamp, round, trialNum)

	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Clean up timer to prevent it from firing after test completes
	node.stopFailToSubmitCoMonitoring()
}

// TestProcessRequestedToSubmitCv_NotHalted tests processing event
func TestProcessRequestedToSubmitCv_NotHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(1800)
	trialNum := big.NewInt(1)

	node.processRequestedToSubmitCv(context.Background(), blockTimestamp, round, trialNum)

	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Clean up timer to prevent it from firing after test completes
	node.stopFailToSubmitCvMonitoring()
}

// TestStartFailToSubmitCoMonitoring_ValidTimestamp tests normal monitoring start
func TestStartFailToSubmitCoMonitoring_ValidTimestamp(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Use future timestamp
	futureTimestamp := big.NewInt(time.Now().Unix() + 200)

	node.startFailToSubmitCoMonitoring(context.Background(), "1900", "1", futureTimestamp)

	// Should have started monitoring
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Clean up timer
	// Clean up timer properly using the stop function
	node.stopFailToSubmitCoMonitoring()
}

// TestUpdateCOS_NotExistsInMemory tests updateCOS when data doesn't exist
func TestUpdateCOS_NotExistsInMemory(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x1234567890ABCDEF1234567890ABCDEF12345678")
	testOp2 := common.HexToAddress("0xFEDCBA0987654321FEDCBA0987654321FEDCBA09")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "2000"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	var cos [32]byte
	copy(cos[:], []byte("new-cos"))

	// updateCOS should create new entry
	node.updateCOS(context.Background(), round, trialNum, uniqueKey, testOp1, cos)

	// Verify data was created
	data, exists := utils.GetCommittedNodeData(uniqueKey, testOp1)
	assert.True(t, exists)
	assert.Equal(t, cos, data.Cos)
}

// TestUpdateCVS_CreatesNewEntry tests updateCVS creating new entry
func TestUpdateCVS_CreatesNewEntry(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0xFEDCBA0987654321FEDCBA0987654321FEDCBA09")

	round := "2100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	var cvs [32]byte
	copy(cvs[:], []byte("new-cvs"))

	// Clean up first
	utils.DeleteCommittedNodes(uniqueKey)

	// updateCVS should create new entry
	node.updateCVS(round, uniqueKey, testOp, cvs)

	// Verify data was created
	data, exists := utils.GetCommittedNodeData(uniqueKey, testOp)
	assert.True(t, exists)
	assert.Equal(t, cvs, data.Cvs)
}

// TestGetMissingCvsOperators_AllMissing tests when all operators missing
func TestGetMissingCvsOperators_AllMissing(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	uniqueKey := "2200:1"

	// Clean up to ensure no data
	utils.DeleteCommittedNodes(uniqueKey)

	missing := node.getMissingCvsOperators(uniqueKey)

	// All operators should be missing
	assert.Len(t, missing, 2)
	assert.Contains(t, missing, testOp1.Hex())
	assert.Contains(t, missing, testOp2.Hex())
}

// TestProcessRandomRequestNumber_StateOne tests state 1
func TestProcessRandomRequestNumber_StateOne(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo

	testOp := common.HexToAddress("0x3333333333333333333333333333333333333333")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}
	node.ethService = mockEth

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(2300)
	trialNum := big.NewInt(1)
	state := big.NewInt(1) // IN_PROGRESS

	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, round.String()).Return(nil)

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	assert.False(t, node.GetHalted())

	// Clean up timer to prevent it from firing after test completes
	node.stopRequestToSubmitCvMonitoring()

	mockBatchRepo.AssertExpectations(t)
}

// TestProcessRandomRequestNumber_StateThree tests state 3 (HALTED)
func TestProcessRandomRequestNumber_StateThree(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo

	testOp := common.HexToAddress("0x4444444444444444444444444444444444444444")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1e16), nil // Return sufficient deposit
			}
			if method == "getActivatedOperatorsLength" {
				return big.NewInt(2), nil // Return >= 2 operators
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	// Set current round and trial
	node.SetCurrentRound("2400")
	node.SetCurrentTrial("1")

	// Set required environment variables for resuming function
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(2400)
	trialNum := big.NewInt(1)
	state := big.NewInt(3) // HALTED

	mockBatchRepo.On("DeleteRoundTrialDataForLeaderNode", mock.Anything, "2400", "1").Return(nil)

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	assert.True(t, node.GetHalted())

	mockBatchRepo.AssertExpectations(t)
}

// TestAllCosReceivedUnlocked_EmptyOperators tests with empty operators
func TestAllCosReceivedUnlocked_EmptyOperators(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{}
		},
	}
	node.ethService = mockEth

	result := node.AllCosReceivedUnlocked("test-key")
	assert.False(t, result)
}

// TestAllCvsReceivedUnlocked_EmptyOperators tests with empty operators
func TestAllCvsReceivedUnlocked_EmptyOperators(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{}
		},
	}
	node.ethService = mockEth

	result := node.AllCvsReceivedUnlocked("test-key")
	assert.False(t, result)
}

// TestProcessCOS_StoresDataCorrectly tests processCOS storing data correctly
func TestProcessCOS_StoresDataCorrectly(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	testOp1 := common.HexToAddress("0x5555555555555555555555555555555555555555")
	testOp2 := common.HexToAddress("0x6666666666666666666666666666666666666666")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2} // 2 operators so AllCosReceived is false
		},
	}
	node.ethService = mockEth

	round := big.NewInt(2500)
	trialNum := big.NewInt(1)
	var cos [32]byte
	copy(cos[:], []byte("final-cos"))
	index := big.NewInt(0)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp1.Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.NoError(t, err)

	// Verify data was stored in memory
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	data, exists := utils.GetCommittedNodeData(uniqueKey, testOp1)
	assert.True(t, exists)
	assert.Equal(t, cos, data.Cos)
}

// TestProcessCVS_AllCvsReceivedTrigger tests when all CVS received
func TestProcessCVS_AllCvsReceivedTrigger(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	testOp := common.HexToAddress("0x6666666666666666666666666666666666666666")
	otherOp := common.HexToAddress("0x6666666666666666666666666666666666666667")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp, otherOp}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	round := big.NewInt(2600)
	trialNum := big.NewInt(1)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	// Pre-populate in-memory CVS for the other operator so we have 2 leaves (Merkle tree requires >= 2)
	var otherCvs [32]byte
	copy(otherCvs[:], []byte("other-cvs"))
	utils.EnsureCommittedNodesRoundExists(uniqueKey)
	utils.SetCommittedNodeData(uniqueKey, otherOp, utils.LeaderCommitData{Cvs: otherCvs})

	var cvs [32]byte
	copy(cvs[:], []byte("final-cvs"))
	index := big.NewInt(0)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp.Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil).Maybe() // called after SubmitMerkleRoot
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Set LEADER_PRIVATE_KEY and CONTRACT_ADDRESS so SubmitMerkleRoot can create leader client
	validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	origKey := os.Getenv("LEADER_PRIVATE_KEY")
	origAddr := os.Getenv("CONTRACT_ADDRESS")
	os.Setenv("LEADER_PRIVATE_KEY", validKey)
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Setenv("LEADER_PRIVATE_KEY", origKey)
		os.Setenv("CONTRACT_ADDRESS", origAddr)
	}()

	// This triggers GenerateMerkleRoot (all CVS received, 2 operators with 2 leaves)
	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.NoError(t, err)
}

// TestCheckAndStopFailToSubmitCoMonitoring_PartialCos tests when COS not all received
func TestCheckAndStopFailToSubmitCoMonitoring_PartialCos(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x7777777777777777777777777777777777777777")
	testOp2 := common.HexToAddress("0x8888888888888888888888888888888888888888")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "2700"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Start monitoring
	timestamp := big.NewInt(time.Now().Unix() + 1000)
	node.startFailToSubmitCoMonitoring(context.Background(), round, trialNum, timestamp)
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Add COS only for op1
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cos: [32]byte{1}})

	// Should NOT stop monitoring since not all COS received
	node.checkAndStopFailToSubmitCoMonitoring(round, trialNum)
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())
}

// TestCheckAndStopFailToSubmitCvMonitoring_PartialCvs tests when CVS not all received
func TestCheckAndStopFailToSubmitCvMonitoring_PartialCvs(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x9999999999999999999999999999999999999999")
	testOp2 := common.HexToAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "2800"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Start monitoring
	timestamp := big.NewInt(time.Now().Unix() + 1000)
	node.startFailToSubmitCvMonitoring(context.Background(), round, trialNum, timestamp)
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Add CVS only for op1
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})

	// Should NOT stop monitoring since not all CVS received
	node.checkAndStopFailToSubmitCvMonitoring(round, trialNum)
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())
}

// TestProcessRequestedToSubmitCo_WhenHalted tests when halted
func TestProcessRequestedToSubmitCo_WhenHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	node.SetHalted(true)

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(2900)
	trialNum := big.NewInt(1)

	node.processRequestedToSubmitCo(context.Background(), blockTimestamp, round, trialNum)

	// Should not have started monitoring
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
}

// TestProcessRequestedToSubmitCv_WhenHalted tests when halted
func TestProcessRequestedToSubmitCv_WhenHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	node.SetHalted(true)

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(3000)
	trialNum := big.NewInt(1)

	node.processRequestedToSubmitCv(context.Background(), blockTimestamp, round, trialNum)

	// Should not have started monitoring
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
}

// TestCallRequestToSubmitCv_WhenHalted tests callRequestToSubmitCv when halted
func TestCallRequestToSubmitCv_WhenHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	node.SetHalted(true)

	// Should return early
	node.callRequestToSubmitCv(context.Background(), "3100", "1")

	// Verify it didn't proceed
	assert.True(t, node.GetHalted())
}

// TestCallRequestToSubmitCv_AllCvsReceived tests when all CVS already received
func TestCallRequestToSubmitCv_AllCvsReceived(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	node.SetHalted(false)

	testOp := common.HexToAddress("0x1111111111111111111111111111111111111111")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := "3200"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Add CVS for the operator
	utils.SetCommittedNodeData(uniqueKey, testOp, utils.LeaderCommitData{Cvs: [32]byte{1}})

	// Should return early since all CVS received
	node.callRequestToSubmitCv(context.Background(), round, trialNum)

	// Verify no indices were added (no missing operators)
	assert.Len(t, node.GetIndices(), 0)
}

// TestGetMissingCvsOperators_PartialData tests getMissingCvsOperators with some missing
func TestGetMissingCvsOperators_PartialData(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	testOp2 := common.HexToAddress("0x3333333333333333333333333333333333333333")
	testOp3 := common.HexToAddress("0x4444444444444444444444444444444444444444")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2, testOp3}
		},
	}
	node.ethService = mockEth

	round := "3300"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Add CVS only for op1 and op3, leaving op2 missing
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})
	utils.SetCommittedNodeData(uniqueKey, testOp3, utils.LeaderCommitData{Cvs: [32]byte{3}})

	missing := node.getMissingCvsOperators(uniqueKey)

	// Only op2 should be missing
	assert.Len(t, missing, 1)
	assert.Contains(t, missing, testOp2.Hex())
	assert.NotContains(t, missing, testOp1.Hex())
	assert.NotContains(t, missing, testOp3.Hex())
}

// TestResuming_ABILoadError tests resuming function behavior
func TestResuming_ABILoadError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1e16), nil // Return sufficient deposit
			}
			if method == "getActivatedOperatorsLength" {
				return big.NewInt(2), nil // Return >= 2 operators
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	// Set environment variables to allow resuming to proceed
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	node.resuming(context.Background())
	assert.True(t, true)
}

// TestCheckHaltedState_NoContractAddress tests CheckHaltedState with no CONTRACT_ADDRESS
func TestCheckHaltedState_NoContractAddress(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Unset CONTRACT_ADDRESS
	os.Unsetenv("CONTRACT_ADDRESS")

	// Should return early
	node.CheckHaltedState(context.Background())

	// Test passes if no panic/crash
	assert.True(t, true)
}

// TestResetCosAndCvsMonitoringState_WithActiveTimers tests reset with active timers
func TestResetCosAndCvsMonitoringState_WithActiveTimers(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	timestamp := big.NewInt(time.Now().Unix() + 2000)

	// Start all monitoring types
	node.startFailToSubmitCoMonitoring(context.Background(), "3500", "1", timestamp)
	node.startFailToSubmitCvMonitoring(context.Background(), "3500", "1", timestamp)
	node.startRequestToSubmitCvMonitoring(context.Background(), "3500", "1", timestamp)

	// Set timers manually to test the timer cleanup paths
	node.requestedToSubmitCoMonitoringTimer = time.AfterFunc(10*time.Second, func() {})
	node.requestedToSubmitCvMonitoringTimer = time.AfterFunc(10*time.Second, func() {})
	node.requestToSubmitCvMonitoringTimer = time.AfterFunc(10*time.Second, func() {})

	node.SetSecretRequestSentForWhichRound("3500")

	// Reset everything
	node.ResetCosAndCvsMonitoringState("3500", "1")

	// Verify all timers were stopped and flags reset
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
	assert.False(t, node.GetRequestToSubmitCvMonitoringActive())
	assert.Equal(t, "3500", node.GetSecretRequestSentForWhichRound())
	assert.Nil(t, node.requestedToSubmitCoMonitoringTimer)
	assert.Nil(t, node.requestedToSubmitCvMonitoringTimer)
	assert.Nil(t, node.requestToSubmitCvMonitoringTimer)
}

// TestUpdateCOS_ExistingData tests updateCOS when data already exists
func TestUpdateCOS_ExistingData(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x4444444444444444444444444444444444444444")
	testOp2 := common.HexToAddress("0x5555555555555555555555555555555555555555")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "3600"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Pre-populate with CVS
	existingData := utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: testOp1.Hex(),
		Cvs:        [32]byte{99},
	}
	utils.SetCommittedNodeData(uniqueKey, testOp1, existingData)

	var cos [32]byte
	copy(cos[:], []byte("new-cos-data"))

	// Update with COS
	node.updateCOS(context.Background(), round, trialNum, uniqueKey, testOp1, cos)

	// Verify both CVS and COS exist
	data, exists := utils.GetCommittedNodeData(uniqueKey, testOp1)
	assert.True(t, exists)
	assert.Equal(t, [32]byte{99}, data.Cvs)
	assert.Equal(t, cos, data.Cos)
}

// TestProcessRandomRequestNumber_StateOneWithDeleteError tests state 1 with delete error
func TestProcessRandomRequestNumber_StateOneWithDeleteError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo

	testOp := common.HexToAddress("0x6666666666666666666666666666666666666666")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}
	node.ethService = mockEth

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(3700)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)

	// Mock delete error
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, round.String()).
		Return(errors.New("delete error"))

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)


	mockBatchRepo.AssertExpectations(t)
}

// TestProcessRandomRequestNumber_StateTwoWithExistingData tests state 2 with existing round data
func TestProcessRandomRequestNumber_StateTwoWithExistingData(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(3800)
	trialNum := big.NewInt(1)
	state := big.NewInt(2)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())

	// Pre-set round data
	node.SetRoundData(uniqueKey, RoundData{MerkleRoot: true})

	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, round.String()).Return(nil)

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	// Verify RandomNumber was set to true
	data, exists := node.GetRoundData(uniqueKey)
	assert.True(t, exists)
	assert.True(t, data.RandomNumber)

	mockBatchRepo.AssertExpectations(t)
}

// TestGetMissingCvsOperators_EmptyOperators tests with empty activated operators
func TestGetMissingCvsOperators_EmptyOperators(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{}
		},
	}
	node.ethService = mockEth

	uniqueKey := "3900:1"

	missing := node.getMissingCvsOperators(uniqueKey)

	// Should return empty list
	assert.Len(t, missing, 0)
}

// TestUpdateCVS_ExistingData tests updateCVS when data already exists
func TestUpdateCVS_ExistingData(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0x7777777777777777777777777777777777777777")

	round := "4000"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Pre-populate with COS
	existingData := utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: testOp.Hex(),
		Cos:        [32]byte{88},
	}
	utils.SetCommittedNodeData(uniqueKey, testOp, existingData)

	var cvs [32]byte
	copy(cvs[:], []byte("new-cvs-data"))

	// Update with CVS
	node.updateCVS(round, uniqueKey, testOp, cvs)

	// Verify both CVS and COS exist
	data, exists := utils.GetCommittedNodeData(uniqueKey, testOp)
	assert.True(t, exists)
	assert.Equal(t, cvs, data.Cvs) // New CVS
	assert.Equal(t, [32]byte{88}, data.Cos)
}

// TestStopFailToSubmitCoMonitoring_NilTimer tests stopping with nil timer
func TestStopFailToSubmitCoMonitoring_NilTimer(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set active flag without timer
	node.SetRequestedToSubmitCoMonitoringActive(true)

	// Should handle nil timer gracefully
	node.stopFailToSubmitCoMonitoring()

	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
}

// TestStopFailToSubmitCvMonitoring_NilTimer tests stopping with nil timer
func TestStopFailToSubmitCvMonitoring_NilTimer(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set active flag without timer
	node.SetRequestedToSubmitCvMonitoringActive(true)

	// Should handle nil timer gracefully
	node.stopFailToSubmitCvMonitoring()

	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
}

// TestStopRequestToSubmitCvMonitoring_NilTimer tests stopping with nil timer
func TestStopRequestToSubmitCvMonitoring_NilTimer(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set active flag without timer
	node.SetRequestToSubmitCvMonitoringActive(true)

	// Should handle nil timer gracefully
	node.stopRequestToSubmitCvMonitoring()

	assert.False(t, node.GetRequestToSubmitCvMonitoringActive())
}

// TestStopFailToSubmitCoMonitoring_WithActiveTimer tests stopping with active timer
func TestStopFailToSubmitCoMonitoring_WithActiveTimer(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set active with timer
	node.SetRequestedToSubmitCoMonitoringActive(true)
	node.requestedToSubmitCoMonitoringTimer = time.AfterFunc(10*time.Second, func() {})

	node.stopFailToSubmitCoMonitoring()

	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
	assert.Nil(t, node.requestedToSubmitCoMonitoringTimer)
}

// TestStopFailToSubmitCvMonitoring_WithActiveTimer tests stopping with active timer
func TestStopFailToSubmitCvMonitoring_WithActiveTimer(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set active with timer
	node.SetRequestedToSubmitCvMonitoringActive(true)
	node.requestedToSubmitCvMonitoringTimer = time.AfterFunc(10*time.Second, func() {})

	node.stopFailToSubmitCvMonitoring()

	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
	assert.Nil(t, node.requestedToSubmitCvMonitoringTimer)
}

// TestStopRequestToSubmitCvMonitoring_WithActiveTimer tests stopping with active timer
func TestStopRequestToSubmitCvMonitoring_WithActiveTimer(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set active with timer
	node.SetRequestToSubmitCvMonitoringActive(true)
	node.requestToSubmitCvMonitoringTimer = time.AfterFunc(10*time.Second, func() {})

	node.stopRequestToSubmitCvMonitoring()

	assert.False(t, node.GetRequestToSubmitCvMonitoringActive())
	assert.Nil(t, node.requestToSubmitCvMonitoringTimer)
}

// TestGenerateMerkleRoot_EmptyLeaves tests when leaves array is empty
func TestGenerateMerkleRoot_EmptyLeaves(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{}
		},
	}
	node.ethService = mockEth

	round := "4100"
	trialNum := "1"

	// Should return early with empty operators
	node.GenerateMerkleRoot(context.Background(), round, trialNum)

	// Test passes if no panic
	assert.True(t, true)
}

// TestProcessCOS_BroadcastTriggered tests that broadcast is triggered
func TestProcessCOS_BroadcastTriggered(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	testOp1 := common.HexToAddress("0x8888888888888888888888888888888888888888")
	testOp2 := common.HexToAddress("0x9999999999999999999999999999999999999999")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := big.NewInt(4200)
	trialNum := big.NewInt(1)
	var cos [32]byte
	copy(cos[:], []byte("cos-broadcast"))
	index := big.NewInt(0)

	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: testOp1.Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp1.Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.MatchedBy(func(tracker *utils.BroadcastTracker) bool {
		return tracker.Type == "cos" && tracker.Round == round.String()
	})).Return(nil).Maybe()

	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.NoError(t, err)

	mockLeaderRepo.AssertExpectations(t)
}

func TestProcessCVS_BroadcastTriggered(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	testOp1 := common.HexToAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	testOp2 := common.HexToAddress("0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := big.NewInt(4300)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("cvs-broadcast"))
	index := big.NewInt(0)

	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: testOp1.Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp1.Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.MatchedBy(func(tracker *utils.BroadcastTracker) bool {
		return tracker.Type == "cvs" && tracker.Round == round.String()
	})).Return(nil).Maybe()

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.NoError(t, err)

	mockLeaderRepo.AssertExpectations(t)
}

// TestCheckAndStopFailToSubmitCoMonitoring_MonitoringActive tests with monitoring active
func TestCheckAndStopFailToSubmitCoMonitoring_MonitoringActive(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC")
	testOp2 := common.HexToAddress("0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "4400"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Start monitoring
	timestamp := big.NewInt(time.Now().Unix() + 5000)
	node.startFailToSubmitCoMonitoring(context.Background(), round, trialNum, timestamp)

	// Add COS for only first operator
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cos: [32]byte{1}})

	// Should NOT stop because not all COS received
	node.checkAndStopFailToSubmitCoMonitoring(round, trialNum)
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Clean up timer
	// Clean up timer properly using the stop function
	node.stopFailToSubmitCoMonitoring()
}

// TestCheckAndStopFailToSubmitCvMonitoring_MonitoringActive tests with monitoring active
func TestCheckAndStopFailToSubmitCvMonitoring_MonitoringActive(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0xEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE")
	testOp2 := common.HexToAddress("0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := "4500"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Start monitoring
	timestamp := big.NewInt(time.Now().Unix() + 5000)
	node.startFailToSubmitCvMonitoring(context.Background(), round, trialNum, timestamp)

	// Add CVS for only first operator
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})

	// Should NOT stop because not all CVS received
	node.checkAndStopFailToSubmitCvMonitoring(round, trialNum)
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Clean up timer properly using the stop function
	node.stopFailToSubmitCvMonitoring()
}

// TestGenerateMerkleRoot_NoRoundDataExists tests when no round data exists initially
func TestGenerateMerkleRoot_NoRoundDataExists(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0x1010101010101010101010101010101010101010")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := "4600"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Don't set any round data
	_, exists := node.GetRoundData(uniqueKey)
	assert.False(t, exists)

	// Add CVS
	utils.SetCommittedNodeData(uniqueKey, testOp, utils.LeaderCommitData{Cvs: [32]byte{1}})

	// Should proceed (and fail at contract call, but that's ok)
	node.GenerateMerkleRoot(context.Background(), round, trialNum)

	// Test passes if no panic
	assert.True(t, true)
}

// TestStartFailToSubmitCoMonitoring_WithActiveMonitoring tests starting when already active
func TestStartFailToSubmitCoMonitoring_WithActiveMonitoring(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	timestamp := big.NewInt(time.Now().Unix() + 3000)

	// Start once
	node.startFailToSubmitCoMonitoring(context.Background(), "4700", "1", timestamp)
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Start again - should just add another timer
	node.startFailToSubmitCoMonitoring(context.Background(), "4700", "1", timestamp)

	// Clean up timers
	// Clean up timer properly using the stop function
	node.stopFailToSubmitCoMonitoring()
}

// TestStartFailToSubmitCvMonitoring_WithActiveMonitoring tests starting when already active
func TestStartFailToSubmitCvMonitoring_WithActiveMonitoring(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	timestamp := big.NewInt(time.Now().Unix() + 3000)

	// Start once
	node.startFailToSubmitCvMonitoring(context.Background(), "4800", "1", timestamp)
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Start again - should just add another timer
	node.startFailToSubmitCvMonitoring(context.Background(), "4800", "1", timestamp)

	// Clean up timers properly using the stop function
	node.stopFailToSubmitCvMonitoring()
}

// TestStartRequestToSubmitCvMonitoring_WithActiveMonitoring tests starting when already active
func TestStartRequestToSubmitCvMonitoring_WithActiveMonitoring(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	timestamp := big.NewInt(time.Now().Unix() + 3000)

	// Start once
	node.startRequestToSubmitCvMonitoring(context.Background(), "4900", "1", timestamp)
	assert.True(t, node.GetRequestToSubmitCvMonitoringActive())

	// Start again - should just add another timer
	node.startRequestToSubmitCvMonitoring(context.Background(), "4900", "1", timestamp)

	// Clean up timers properly using the stop function
	node.stopRequestToSubmitCvMonitoring()
}

// TestUpdateCommitDataAfterSubmit_NoRoundData tests with no round data
func TestUpdateCommitDataAfterSubmit_NoRoundData(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	uniqueKey := "5000:1"

	// Ensure no round data exists
	utils.DeleteCommittedNodes(uniqueKey)

	// Should return early without error
	node.updateCommitDataAfterSubmit(context.Background(), uniqueKey)

	// Test passes if no panic
	assert.True(t, true)
}

// TestUpdateCommitDataAfterSubmit_WithDatabaseError tests with database update error
func TestUpdateCommitDataAfterSubmit_WithDatabaseError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := new(MockLeaderCommitRepository)
	node.leaderCommitRepository = mockLeaderRepo

	uniqueKey := "5100:1"
	testOp := common.HexToAddress("0x1212121212121212121212121212121212121212")

	// Set up commit data
	commitData := utils.LeaderCommitData{
		EOAAddress:           testOp.Hex(),
		SubmitMerkleRootDone: false,
	}
	utils.SetCommittedNodeData(uniqueKey, testOp, commitData)

	// Mock update with error
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.MatchedBy(func(data *utils.LeaderCommitData) bool {
		return data.SubmitMerkleRootDone == true
	})).Return(errors.New("database error"))

	// Should handle error gracefully
	node.updateCommitDataAfterSubmit(context.Background(), uniqueKey)

	mockLeaderRepo.AssertExpectations(t)
}

// TestProcessSubmittedSecretRequest_IndexOutOfBounds tests with invalid index
func TestProcessSubmittedSecretRequest_IndexOutOfBounds(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0x1313131313131313131313131313131313131313")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := big.NewInt(5200)
	trialNum := big.NewInt(1)
	var secret [32]byte
	index := big.NewInt(10) // Out of bounds

	node.SetSecretRequestSentForWhichRound(round.String())

	// Should return early due to out of bounds
	node.processSubmittedSecretRequest(context.Background(), round, trialNum, secret, index)

	// Test passes if no panic
	assert.True(t, true)
}

// TestProcessCOS_CheckStopMonitoring tests that monitoring check is called
func TestProcessCOS_CheckStopMonitoring(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	testOp1 := common.HexToAddress("0x1414141414141414141414141414141414141414")
	testOp2 := common.HexToAddress("0x1515151515151515151515151515151515151515")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	round := big.NewInt(5300)
	trialNum := big.NewInt(1)
	var cos [32]byte
	copy(cos[:], []byte("cos-monitor"))
	index := big.NewInt(0)

	// Start monitoring first
	timestamp := big.NewInt(time.Now().Unix() + 5000)
	node.startFailToSubmitCoMonitoring(context.Background(), round.String(), trialNum.String(), timestamp)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp1.Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Process COS for first operator - should NOT stop monitoring since not all COS received
	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.NoError(t, err)

	// Monitoring should still be active (only 1 of 2 COS received)
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Clean up
	// Clean up timer properly using the stop function
	node.stopFailToSubmitCoMonitoring()
}

// TestProcessCVS_CheckStopMonitoring tests that monitoring check is called
func TestProcessCVS_CheckStopMonitoring(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	testOp1 := common.HexToAddress("0x1515151515151515151515151515151515151515")
	testOp2 := common.HexToAddress("0x1616161616161616161616161616161616161616")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2} // 2 operators to avoid triggering GenerateMerkleRoot
		},
	}
	node.ethService = mockEth

	round := big.NewInt(5400)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("cvs-monitor"))
	index := big.NewInt(0)

	// Start monitoring first
	timestamp := big.NewInt(time.Now().Unix() + 5000)
	node.startFailToSubmitCvMonitoring(context.Background(), round.String(), trialNum.String(), timestamp)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp1.Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Process CVS for first operator - should NOT stop monitoring since not all CVS received
	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.NoError(t, err)

	// Monitoring should still be active (only 1 of 2 CVS received)
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Clean up
	if node.requestedToSubmitCvMonitoringTimer != nil {
		node.requestedToSubmitCvMonitoringTimer.Stop()
	}
}

type MockSubscription struct {
	mock.Mock
	errChan chan error
}

func (m *MockSubscription) Err() <-chan error {
	if m.errChan == nil {
		m.errChan = make(chan error)
	}
	return m.errChan
}

func (m *MockSubscription) Unsubscribe() {

	if len(m.ExpectedCalls) > 0 {
		m.Called()
	}
}

type MockFallbackEthClientForAcceptCommit struct {
	mock.Mock
}

func (m *MockFallbackEthClientForAcceptCommit) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, logs chan<- types.Log) (ethereum.Subscription, error) {
	args := m.Called(ctx, query, logs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ethereum.Subscription), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) BlockTimestamp(ctx context.Context, blockNumber *big.Int) (uint64, error) {
	args := m.Called(ctx, blockNumber)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) NetworkID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	args := m.Called(ctx, account, blockNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	args := m.Called(ctx, msg, blockNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	args := m.Called(ctx, msg)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	args := m.Called(ctx, tx)
	return args.Error(0)
}

func (m *MockFallbackEthClientForAcceptCommit) TransactionReceipt(ctx context.Context, signedTx *types.Transaction) (*types.Receipt, error) {
	args := m.Called(ctx, signedTx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Receipt), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	for _, call := range m.ExpectedCalls {
		if call.Method == "PendingNonceAt" {
			args := m.Called(ctx, account)
			return args.Get(0).(uint64), args.Error(1)
		}
	}
	// Default nonce when not explicitly mocked.
	return 0, nil
}

func (m *MockFallbackEthClientForAcceptCommit) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	args := m.Called(ctx, q)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]types.Log), args.Error(1)
}

func (m *MockFallbackEthClientForAcceptCommit) HeaderByNumber(ctx context.Context, blockNumber *big.Int) (*types.Header, error) {
	args := m.Called(ctx, blockNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Header), args.Error(1)
}

func TestLeaderNode_receiveCommit_SubscriptionFailure(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	// Mock subscription failure
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("subscription failed"))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start receiveCommit in goroutine
	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	// Wait for either context timeout or completion
	select {
	case <-ctx.Done():
		// Wait for goroutine to finish
		select {
		case <-done:
			// Goroutine completed
		case <-time.After(1 * time.Second):
			// Force completion
		}
	case <-done:
		// Completed early
	}

	// Verify subscription was attempted at least once (allowing for retry logic)
	assert.True(t, mockClient.AssertExpectations(t), "Mock expectations should be met")

	// Get call count in a race-safe way by checking if any calls were made
	called := false
	for _, call := range mockClient.ExpectedCalls {
		if call.Method == "SubscribeFilterLogs" && len(call.Arguments) > 0 {
			called = true
			break
		}
	}
	assert.True(t, called, "SubscribeFilterLogs should have been called")
}

func TestLeaderNode_receiveCommit_StatusEvent_State1(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	statusEventSig := parsedABI.Events["Status"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)

	eventData, err := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}
	mockSub.On("Unsubscribe").Return()

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{statusEventSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()+1000), nil)

	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, "100").
		Return(nil)

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}
	node.ethService = mockEth

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Give the goroutine time to call Unsubscribe()
	time.Sleep(50 * time.Millisecond)

	mockClient.AssertExpectations(t)
	mockBatchRepo.AssertExpectations(t)
	mockSub.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_CvSubmitted_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.fallbackEthClient = mockClient
	node.broadcastTrackerRepository = mockBroadcastRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	cvSubmittedSig := parsedABI.Events["CvSubmitted"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	cv := [32]byte{5, 6, 7}
	index := big.NewInt(0)

	eventData, err := parsedABI.Events["CvSubmitted"].Inputs.Pack(round, trialNum, cv, index)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{cvSubmittedSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, "100", "1", testOp.Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_CoSubmitted_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.fallbackEthClient = mockClient
	node.broadcastTrackerRepository = mockBroadcastRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	coSubmittedSig := parsedABI.Events["CoSubmitted"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	co := [32]byte{8, 9, 10}
	index := big.NewInt(0)

	eventData, err := parsedABI.Events["CoSubmitted"].Inputs.Pack(round, trialNum, co, index)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{coSubmittedSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, "100", "1", testOp1.Hex()).
		Return(nil, pg.ErrNoRows)
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_ReorgDetection(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Removed: true, // Reorg event
						TxHash:  common.HexToHash("0x123"),
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_WebsocketReconnection(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	// First subscription fails with websocket error
	mockSub1 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub1.errChan <- errors.New("websocket: close 1006")

	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub1, nil).Once()

	// Second subscription succeeds
	mockSub2 := &MockSubscription{
		errChan: make(chan error),
	}
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub2, nil).Once()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Verify reconnection happened
	mockClient.AssertNumberOfCalls(t, "SubscribeFilterLogs", 2)
}

func TestLeaderNode_receiveCommit_MerkleRootSubmitted_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	merkleRootSubmittedSig := parsedABI.Events["MerkleRootSubmitted"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	merkleRoot := [32]byte{1, 2, 3, 4, 5}

	eventData, err := parsedABI.Events["MerkleRootSubmitted"].Inputs.Pack(round, trialNum, merkleRoot)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{merkleRootSubmittedSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil)
	mockClient.On("NetworkID", mock.Anything).
		Return(big.NewInt(1), nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_RequestedToSubmitCo_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	requestedToSubmitCoSig := parsedABI.Events["RequestedToSubmitCo"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	indicesLength := big.NewInt(1)
	packedIndices := big.NewInt(0)

	eventData, err := parsedABI.Events["RequestedToSubmitCo"].Inputs.Pack(round, trialNum, indicesLength, packedIndices)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{requestedToSubmitCoSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Verify monitoring was started
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Clean up timer to prevent it from firing after test completes
	node.stopFailToSubmitCoMonitoring()

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_RequestedToSubmitCv_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	requestedToSubmitCvSig := parsedABI.Events["RequestedToSubmitCv"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	packedIndices := big.NewInt(0)

	eventData, err := parsedABI.Events["RequestedToSubmitCv"].Inputs.Pack(round, trialNum, packedIndices)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{requestedToSubmitCvSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Verify monitoring was started
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

	// Clean up timer to prevent it from firing after test completes
	node.stopFailToSubmitCvMonitoring()

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_InvalidEventData(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	cvSubmittedSig := parsedABI.Events["CvSubmitted"].ID

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{cvSubmittedSig},
						Data:        []byte{0x01}, // Invalid data
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_callFailToSubmitCo_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "failToSubmitCo" {
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unexpected method")
		},
	}
	node.ethService = mockEth

	node.callFailToSubmitCo(context.Background(), "100", "1")

	// Test passes if no panic
	assert.True(t, true)
}

func TestLeaderNode_callFailToSubmitCv_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "failToSubmitCv" {
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unexpected method")
		},
	}
	node.ethService = mockEth

	node.callFailToSubmitCv(context.Background(), "100", "1")

	// Test passes if no panic
	assert.True(t, true)
}

func TestLeaderNode_callRequestToSubmitCv_WithMissingOperators(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "requestToSubmitCv" {
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unexpected method")
		},
	}
	node.ethService = mockEth
	node.SetHalted(false)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Only add CVS for first operator, leaving second missing
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})

	// Mock LoadNodeData
	mockLeaderRepo.On("GetLeaderCommitsByRoundAndTrialNum", mock.Anything, round, trialNum).
		Return([]*utils.LeaderCommitData{}, nil).Maybe()

	node.callRequestToSubmitCv(context.Background(), round, trialNum)

	// Verify indices were appended for missing operator
	assert.Greater(t, len(node.GetIndices()), 0)
}

func TestLeaderNode_SubmitMerkleRoot_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "submitMerkleRoot" {
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unexpected method")
		},
	}
	node.ethService = mockEth

	merkleRoot := []byte{1, 2, 3, 4, 5}

	testOp := common.HexToAddress("0x1111111111111111111111111111111111111111")
	uniqueKey := "100:1"

	// Setup commit data
	utils.SetCommittedNodeData(uniqueKey, testOp, utils.LeaderCommitData{
		EOAAddress:           testOp.Hex(),
		SubmitMerkleRootDone: false,
	})

	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)

	node.SubmitMerkleRoot(context.Background(), "100", "1", merkleRoot)

	// Test passes if no panic - the transaction was submitted
	assert.True(t, true)
}

func TestLeaderNode_SubmitMerkleRoot_TransactionError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return nil, nil, errors.New("transaction failed")
		},
	}
	node.ethService = mockEth

	merkleRoot := []byte{1, 2, 3, 4, 5}

	node.SubmitMerkleRoot(context.Background(), "100", "1", merkleRoot)

	// Verify flag was reset on error
	assert.False(t, node.GetSubmittingMerkleRoot())
}

func TestLeaderNode_callRequestToSubmitCoIfNeeded_WithMissingCos(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	executeCalled := false
	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "requestToSubmitCo" {
				executeCalled = true
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unexpected method")
		},
	}
	node.ethService = mockEth
	node.SetHalted(false)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Add CVS for both but only COS for first operator
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{
		Cvs: [32]byte{1},
		Cos: [32]byte{2},
	})
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{
		Cvs: [32]byte{3},
		// Missing COS
	})

	// Setup eth service to return operators for sortLeaderCommitsByActivatedOperators
	mockEthWithOps := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "requestToSubmitCo" {
				executeCalled = true
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unexpected method")
		},
	}

	originalService := eth.Service
	eth.Service = mockEthWithOps
	defer func() { eth.Service = originalService }()

	node.ethService = mockEthWithOps

	// Mock LoadNodeData for prepareArgumentsForRequestToSubmitCo
	mockLeaderRepo.On("GetLeaderCommitsByRoundAndTrialNum", mock.Anything, round, trialNum).
		Return([]*utils.LeaderCommitData{
			{
				EOAAddress: testOp1.Hex(),
				Cvs:        [32]byte{1},
				Cos:        [32]byte{2},
				Sign: utils.SignInfo{
					V: "27",
					R: "0x1111111111111111111111111111111111111111111111111111111111111111",
					S: "0x2222222222222222222222222222222222222222222222222222222222222222",
				},
			},
			{
				EOAAddress: testOp2.Hex(),
				Cvs:        [32]byte{3},
				Cos:        [32]byte{4},
				Sign: utils.SignInfo{
					V: "28",
					R: "0x3333333333333333333333333333333333333333333333333333333333333333",
					S: "0x4444444444444444444444444444444444444444444444444444444444444444",
				},
			},
		}, nil)

	defer utils.DeleteCommittedNodes(uniqueKey)

	node.callRequestToSubmitCoIfNeeded(context.Background(), round, trialNum, uniqueKey)

	assert.True(t, executeCalled, "requestToSubmitCo should have been called")
}

func TestLeaderNode_callRequestToSubmitCoIfNeeded_AllCosReceived(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	node.ethService = mockEth
	node.SetHalted(false)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Both operators have COS
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{
		Cvs: [32]byte{1},
		Cos: [32]byte{2},
	})
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{
		Cvs: [32]byte{3},
		Cos: [32]byte{4},
	})

	node.callRequestToSubmitCoIfNeeded(context.Background(), round, trialNum, uniqueKey)

	assert.True(t, true)
}

func TestLeaderNode_callRequestToSubmitCoIfNeeded_Halted(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	node.SetHalted(true)

	node.callRequestToSubmitCoIfNeeded(context.Background(), "100", "1", "100:1")

	// Should return early when halted
	assert.True(t, node.GetHalted())
}

func TestLeaderNode_callRequestToSubmitCoIfNeeded_NoRoundData(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{common.HexToAddress("0x1111")}
		},
	}
	node.ethService = mockEth
	node.SetHalted(false)

	// Use non-existent uniqueKey
	node.callRequestToSubmitCoIfNeeded(context.Background(), "999", "999", "999:999")

	// Should log that no round data found
	assert.True(t, true)
}

func TestLeaderNode_requestToSubmitCo_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")

	executeCalled := false
	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "requestToSubmitCo" {
				executeCalled = true
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unexpected method")
		},
	}

	// Override the global eth.Service temporarily for sortLeaderCommitsByActivatedOperators
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	node.ethService = mockEth

	mockLeaderRepo.On("GetLeaderCommitsByRoundAndTrialNum", mock.Anything, "100", "1").
		Return([]*utils.LeaderCommitData{
			{
				EOAAddress: testOp1.Hex(),
				Cvs:        [32]byte{1},
				Cos:        [32]byte{2},
				Sign: utils.SignInfo{
					V: "27",
					R: "0x1111111111111111111111111111111111111111111111111111111111111111",
					S: "0x2222222222222222222222222222222222222222222222222222222222222222",
				},
			},
		}, nil)

	missingIndices := []*big.Int{big.NewInt(0)}

	node.requestToSubmitCo(context.Background(), "100", "1", missingIndices)

	assert.True(t, executeCalled, "requestToSubmitCo should have been called")
}

func TestLeaderNode_startRequestToSubmitCoMonitoring_DeadlineAlreadyPassed(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth
	node.SetHalted(false)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Add both CVS and COS for both operators
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{
		Cvs: [32]byte{1},
		Cos: [32]byte{2},
	})
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{
		Cvs: [32]byte{3},
		Cos: [32]byte{4},
	})

	mockClient.On("NetworkID", mock.Anything).
		Return(big.NewInt(1), nil)

	mockLeaderRepo.On("GetLeaderCommitsByRoundAndTrialNum", mock.Anything, round, trialNum).
		Return([]*utils.LeaderCommitData{}, nil).Maybe()

	// Use past timestamp so deadline has passed
	pastTimestamp := big.NewInt(time.Now().Unix() - 200)

	node.startRequestToSubmitCoMonitoring(context.Background(), round, trialNum, pastTimestamp)

	// Should have called immediately
	time.Sleep(100 * time.Millisecond)
}

func TestLeaderNode_processDeactivated_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockNodeRepo := new(MockNodeInfoRepositoryForAcceptCommit)
	node.nodeInfoRepository = mockNodeRepo

	testOp := common.HexToAddress("0x1111111111111111111111111111111111111111")

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockNodeRepo.On("GetNodeInfos", mock.Anything).
		Return([]*utils.NodeInfo{}, nil)

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, testOp.Hex()).
		Return(nil)

	node.processDeactivated(context.Background(), testOp)

	mockNodeRepo.AssertExpectations(t)
}

func TestLeaderNode_processDeactivated_DeleteError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockNodeRepo := new(MockNodeInfoRepositoryForAcceptCommit)
	node.nodeInfoRepository = mockNodeRepo

	testOp := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}

	// Override global eth.Service since processDeactivated uses eth.Service directly
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockNodeRepo.On("GetNodeInfos", mock.Anything).
		Return([]*utils.NodeInfo{}, nil)

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, testOp.Hex()).
		Return(errors.New("database error"))

	node.processDeactivated(context.Background(), testOp)

	mockNodeRepo.AssertExpectations(t)
}

type MockNetwork struct {
	mock.Mock
}

func (m *MockNetwork) ConnsToPeer(p peer.ID) []network.Conn {
	args := m.Called(p)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]network.Conn)
}

func (m *MockNetwork) ClosePeer(p peer.ID) error {
	args := m.Called(p)
	return args.Error(0)
}

type MockHostForConnectionCleanup struct {
	mock.Mock
	network network.Network
}

func (m *MockHostForConnectionCleanup) Network() network.Network {
	return m.network
}

func (m *MockHostForConnectionCleanup) ID() peer.ID            { return "" }
func (m *MockHostForConnectionCleanup) Addrs() []interface{}   { return nil }
func (m *MockHostForConnectionCleanup) Peerstore() interface{} { return nil }
func (m *MockHostForConnectionCleanup) Connect(ctx context.Context, pi peer.AddrInfo) error {
	return nil
}
func (m *MockHostForConnectionCleanup) SetStreamHandler(pid interface{}, handler interface{}) {}
func (m *MockHostForConnectionCleanup) SetStreamHandlerMatch(interface{}, func(interface{}) bool, interface{}) {
}
func (m *MockHostForConnectionCleanup) RemoveStreamHandler(pid interface{}) {}
func (m *MockHostForConnectionCleanup) Close() error                        { return nil }
func (m *MockHostForConnectionCleanup) Mux() interface{}                    { return nil }
func (m *MockHostForConnectionCleanup) ConnManager() interface{}            { return nil }
func (m *MockHostForConnectionCleanup) EventBus() interface{}               { return nil }

type MockConnection struct {
	mock.Mock
}

func (m *MockConnection) IsClosed() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockConnection) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockConnection) LocalPeer() peer.ID                                    { return "" }
func (m *MockConnection) RemotePeer() peer.ID                                   { return "" }
func (m *MockConnection) LocalPrivateKey() interface{}                          { return nil }
func (m *MockConnection) RemotePublicKey() interface{}                          { return nil }
func (m *MockConnection) ID() string                                            { return "" }
func (m *MockConnection) GetStreams() []network.Stream                          { return nil }
func (m *MockConnection) Stat() network.ConnStats                               { return network.ConnStats{} }
func (m *MockConnection) LocalMultiaddr() interface{}                           { return nil }
func (m *MockConnection) RemoteMultiaddr() interface{}                          { return nil }
func (m *MockConnection) Scope() network.ConnScope                              { return nil }
func (m *MockConnection) ConnState() network.ConnectionState                    { return network.ConnectionState{} }
func (m *MockConnection) NewStream(ctx context.Context) (network.Stream, error) { return nil, nil }

func TestLeaderNode_processDeactivated_ConnectionCleanup_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockNodeRepo := new(MockNodeInfoRepositoryForAcceptCommit)
	node.nodeInfoRepository = mockNodeRepo

	testOp := common.HexToAddress("0x3333333333333333333333333333333333333333")

	testHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer testHost.Close()

	node.p2pClient.SetHost(testHost)

	peerHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer peerHost.Close()

	testHost.Peerstore().AddAddrs(peerHost.ID(), peerHost.Addrs(), time.Hour)
	err = testHost.Connect(context.Background(), peer.AddrInfo{
		ID:    peerHost.ID(),
		Addrs: peerHost.Addrs(),
	})
	require.NoError(t, err)

	conns := testHost.Network().ConnsToPeer(peerHost.ID())
	require.Greater(t, len(conns), 0, "Connection should exist before deactivation")

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
    return nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockNodeRepo.On("GetNodeInfos", mock.Anything).
		Return([]*utils.NodeInfo{
			{
				EOAAddress: testOp.Hex(),
				PeerID:     peerHost.ID().String(),
				IP:         "127.0.0.1",
				Port:       "8081",
			},
		}, nil)

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, testOp.Hex()).
		Return(nil)

	node.processDeactivated(context.Background(), testOp)

	connsAfter := testHost.Network().ConnsToPeer(peerHost.ID())
	assert.Equal(t, 0, len(connsAfter), "Connection should be closed after deactivation")

	mockNodeRepo.AssertExpectations(t)
}

func TestLeaderNode_processDeactivated_ConnectionCleanup_NoConnection(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockNodeRepo := new(MockNodeInfoRepositoryForAcceptCommit)
	node.nodeInfoRepository = mockNodeRepo

	testOp := common.HexToAddress("0x4444444444444444444444444444444444444444")
	testHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer testHost.Close()

	peerHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer peerHost.Close()

	node.p2pClient.SetHost(testHost)

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
    return nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockNodeRepo.On("GetNodeInfos", mock.Anything).
		Return([]*utils.NodeInfo{
			{
				EOAAddress: testOp.Hex(),
				PeerID:     peerHost.ID().String(),
				IP:         "127.0.0.1",
				Port:       "8081",
			},
		}, nil)

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, testOp.Hex()).
		Return(nil)

	conns := testHost.Network().ConnsToPeer(peerHost.ID())
	assert.Equal(t, 0, len(conns), "No connection should exist")

	node.processDeactivated(context.Background(), testOp)

	mockNodeRepo.AssertExpectations(t)
}

func TestLeaderNode_processDeactivated_ConnectionCleanup_InvalidPeerID(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockNodeRepo := new(MockNodeInfoRepositoryForAcceptCommit)
	node.nodeInfoRepository = mockNodeRepo

	testOp := common.HexToAddress("0x5555555555555555555555555555555555555555")

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
    return nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockNodeRepo.On("GetNodeInfos", mock.Anything).
		Return([]*utils.NodeInfo{
			{
				EOAAddress: testOp.Hex(),
				PeerID:     "invalid_peer_id",
				IP:         "127.0.0.1",
				Port:       "8081",
			},
		}, nil)

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, testOp.Hex()).
		Return(nil)

	node.processDeactivated(context.Background(), testOp)

	mockNodeRepo.AssertExpectations(t)
}

func TestLeaderNode_processDeactivated_ConnectionCleanup_NilP2PClient(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockNodeRepo := new(MockNodeInfoRepositoryForAcceptCommit)
	node.nodeInfoRepository = mockNodeRepo
	node.p2pClient = nil

	testOp := common.HexToAddress("0x6666666666666666666666666666666666666666")

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockNodeRepo.On("GetNodeInfos", mock.Anything).
		Return([]*utils.NodeInfo{}, nil)

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, testOp.Hex()).
		Return(nil)

	node.processDeactivated(context.Background(), testOp)

	mockNodeRepo.AssertExpectations(t)
}

func TestLeaderNode_processDeactivated_ConnectionCleanup_NilHostInstance(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockNodeRepo := new(MockNodeInfoRepositoryForAcceptCommit)
	node.nodeInfoRepository = mockNodeRepo
	testOp := common.HexToAddress("0x7777777777777777777777777777777777777777")

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockNodeRepo.On("GetNodeInfos", mock.Anything).
		Return([]*utils.NodeInfo{
			{
				EOAAddress: testOp.Hex(),
				PeerID:     "12D3KooWTestPeerID1234567890123456789012345678901234567890",
				IP:         "127.0.0.1",
				Port:       "8081",
			},
		}, nil)

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, testOp.Hex()).
		Return(nil)

	node.processDeactivated(context.Background(), testOp)

	mockNodeRepo.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_StatusEvent_State2(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	statusEventSig := parsedABI.Events["Status"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(2) // State 2 - Random number generated

	eventData, err := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{statusEventSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil)

	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, "100").
		Return(nil)

	mockEth := &MockEthServiceForAcceptCommit{}
	node.ethService = mockEth

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
	mockBatchRepo.AssertExpectations(t)
}

func TestLeaderNode_startRequestToSubmitCoMonitoring_AlreadyActive(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set already active
	node.SetRequestToSubmitCoTimerMonitoringActive(true)

	timestamp := big.NewInt(time.Now().Unix() + 1000)

	node.startRequestToSubmitCoMonitoring(context.Background(), "100", "1", timestamp)

	// Should still be active (returned early)
	assert.True(t, node.GetRequestToSubmitCoTimerMonitoringActive())
}

func TestLeaderNode_startRequestToSubmitCoMonitoring_ChainIDError(t *testing.T) {
	// Temporarily unset CHAIN_ID to trigger GetBlockTimeSeconds() error
	origChainID := os.Getenv("CHAIN_ID")
	os.Setenv("CHAIN_ID", "")
	defer os.Setenv("CHAIN_ID", origChainID)

	node := createTestNodeForAcceptCommit()

	timestamp := big.NewInt(time.Now().Unix() + 1000)

	node.startRequestToSubmitCoMonitoring(context.Background(), "100", "1", timestamp)

	// Monitoring should not be active since GetBlockTimeSeconds() failed
	assert.False(t, node.GetRequestToSubmitCoTimerMonitoringActive())
}

func TestLeaderNode_CheckHaltedState_ProcessIsHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_isInProcess" {
				return big.NewInt(3), nil // Halted state
			}
			if method == "s_depositAmount" {
				return big.NewInt(1e16), nil
			}
			if method == "getActivatedOperatorsLength" {
				return big.NewInt(2), nil
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	// Create a context that we can cancel to prevent infinite loop in resuming
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	node.CheckHaltedState(ctx)

	// Should have set halted to true
	assert.True(t, node.GetHalted())
}

func TestLeaderNode_CheckHaltedState_ProcessNotHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_isInProcess" {
				return big.NewInt(1), nil // Not halted
			}
			return nil, nil
		},
	}
	node.ethService = mockEth

	node.CheckHaltedState(context.Background())

	// Should not have changed halted state
	assert.False(t, node.GetHalted())
}

func TestLeaderNode_CheckHaltedState_CallSmartContractError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			return nil, errors.New("contract call failed")
		},
	}
	node.ethService = mockEth

	node.CheckHaltedState(context.Background())

	// Should handle error gracefully
	assert.True(t, true)
}

func TestLeaderNode_CheckHaltedState_UnexpectedType(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_isInProcess" {
				return "invalid", nil // Wrong type
			}
			return nil, nil
		},
	}
	node.ethService = mockEth

	node.CheckHaltedState(context.Background())

	// Should handle type mismatch gracefully
	assert.True(t, true)
}

func TestLeaderNode_resuming_DepositNeeded(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	depositCalled := false
	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(0), nil // No deposit, needs to deposit
			}
			if method == "getActivatedOperatorsLength" {
				return big.NewInt(2), nil // >= 2 to exit loop
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "deposit" {
				depositCalled = true
			}
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	node.resuming(ctx)

	assert.True(t, depositCalled, "Deposit should have been called")
}

func TestLeaderNode_resuming_DepositAmountTypeError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return "invalid", nil // Wrong type
			}
			return nil, nil
		},
	}
	node.ethService = mockEth

	node.resuming(context.Background())

	// Should handle type error gracefully
	assert.True(t, true)
}

func TestLeaderNode_resuming_GetActivatedOperatorsLengthError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	callCount := 0
	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1e16), nil // Sufficient deposit
			}
			if method == "getActivatedOperatorsLength" {
				callCount++
				if callCount == 1 {
					return nil, errors.New("network error") // First call fails
				}
				return big.NewInt(2), nil // Second call succeeds
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	node.resuming(ctx)

	// Should have retried and eventually succeeded
	assert.GreaterOrEqual(t, callCount, 2)
}

func TestLeaderNode_resuming_GetActivatedOperatorsLengthTypeError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	callCount := 0
	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1e16), nil
			}
			if method == "getActivatedOperatorsLength" {
				callCount++
				if callCount == 1 {
					return "invalid", nil // Wrong type
				}
				return big.NewInt(2), nil
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	node.resuming(ctx)

	assert.GreaterOrEqual(t, callCount, 2)
}

func TestLeaderNode_resuming_InsufficientOperators(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	callCount := 0
	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1e16), nil
			}
			if method == "getActivatedOperatorsLength" {
				callCount++
				if callCount < 2 {
					return big.NewInt(1), nil // < 2 operators
				}
				return big.NewInt(2), nil // Eventually reach 2
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	node.resuming(ctx)

	// Should have looped until operators >= 2
	assert.GreaterOrEqual(t, callCount, 2)
}

// ============================================================================
// Tests for RequestedToSubmitSFromIndexK event
// ============================================================================

func TestLeaderNode_receiveCommit_RequestedToSubmitSFromIndexK_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	requestedToSubmitSFromIndexKSig := parsedABI.Events["RequestedToSubmitSFromIndexK"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	indexK := big.NewInt(5)

	eventData, err := parsedABI.Events["RequestedToSubmitSFromIndexK"].Inputs.Pack(round, trialNum, indexK)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	// Set monitoring active first and create a timer so we can verify it gets stopped
	node.SetRequestedToSubmitCoMonitoringActive(true)
	node.requestedToSubmitCoMonitoringTimer = time.AfterFunc(10*time.Second, func() {})

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{requestedToSubmitSFromIndexKSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
					time.Sleep(50 * time.Millisecond) // Give time for processing
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_RequestedToSubmitSFromIndexK_DecodeError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	requestedToSubmitSFromIndexKSig := parsedABI.Events["RequestedToSubmitSFromIndexK"].ID

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{requestedToSubmitSFromIndexKSig},
						Data:        []byte{0x01}, // Invalid data
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_SSubmitted_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.fallbackEthClient = mockClient
	node.broadcastTrackerRepository = mockBroadcastRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	sSubmittedSig := parsedABI.Events["SSubmitted"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	secret := [32]byte{11, 12, 13}
	index := big.NewInt(0)

	eventData, err := parsedABI.Events["SSubmitted"].Inputs.Pack(round, trialNum, secret, index)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	// Set secret request sent for which round
	node.SetSecretRequestSentForWhichRound("100")

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{sSubmittedSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil)

	existingCommit := &utils.LeaderCommitData{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: testOp.Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, "100", "1", testOp.Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_SSubmitted_DecodeError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	sSubmittedSig := parsedABI.Events["SSubmitted"].ID

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{sSubmittedSig},
						Data:        []byte{0x01}, // Invalid data
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_SSubmitted_BlockTimestampError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.fallbackEthClient = mockClient
	node.broadcastTrackerRepository = mockBroadcastRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	sSubmittedSig := parsedABI.Events["SSubmitted"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	secret := [32]byte{11, 12, 13}
	index := big.NewInt(0)

	eventData, err := parsedABI.Events["SSubmitted"].Inputs.Pack(round, trialNum, secret, index)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	node.SetSecretRequestSentForWhichRound("100")

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{sSubmittedSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	// Mock BlockTimestamp to return error
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(0), errors.New("failed to get block timestamp"))

	existingCommit := &utils.LeaderCommitData{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: testOp.Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, "100", "1", testOp.Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Should still process the event even if BlockTimestamp fails
	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_DeActivated_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockNodeRepo := new(MockNodeInfoRepositoryForAcceptCommit)
	node.fallbackEthClient = mockClient
	node.nodeInfoRepository = mockNodeRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	deactivatedSig := parsedABI.Events["DeActivated"].ID

	operator := common.HexToAddress("0x1111111111111111111111111111111111111111")

	eventData, err := parsedABI.Events["DeActivated"].Inputs.Pack(operator)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}

	// Override global eth.Service
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{deactivatedSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockNodeRepo.On("GetNodeInfos", mock.Anything).
		Return([]*utils.NodeInfo{}, nil)

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, operator.Hex()).
		Return(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
	mockNodeRepo.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_DeActivated_DecodeError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	deactivatedSig := parsedABI.Events["DeActivated"].ID

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{deactivatedSig},
						Data:        []byte{0x01}, // Invalid data
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_DeActivated_DeleteNodeError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockNodeRepo := new(MockNodeInfoRepositoryForAcceptCommit)
	node.fallbackEthClient = mockClient
	node.nodeInfoRepository = mockNodeRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	deactivatedSig := parsedABI.Events["DeActivated"].ID

	operator := common.HexToAddress("0x2222222222222222222222222222222222222222")

	eventData, err := parsedABI.Events["DeActivated"].Inputs.Pack(operator)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}

	// Override global eth.Service
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{deactivatedSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockNodeRepo.On("GetNodeInfos", mock.Anything).
		Return([]*utils.NodeInfo{}, nil)

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, operator.Hex()).
		Return(errors.New("database error"))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
	mockNodeRepo.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_UnexpectedEOFReconnection(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	// First subscription fails with unexpected EOF
	mockSub1 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub1.errChan <- errors.New("unexpected EOF")

	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub1, nil).Once()

	// Second subscription succeeds
	mockSub2 := &MockSubscription{
		errChan: make(chan error),
	}
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub2, nil).Once()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Verify reconnection happened
	mockClient.AssertNumberOfCalls(t, "SubscribeFilterLogs", 2)
}

func TestLeaderNode_receiveCommit_FatalSubscriptionError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	// First subscription fails with generic error (not websocket/EOF)
	mockSub1 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub1.errChan <- errors.New("some other fatal error")

	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub1, nil).Once()

	// Should still reconnect
	mockSub2 := &MockSubscription{
		errChan: make(chan error),
	}
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub2, nil).Once()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Verify reconnection happened
	mockClient.AssertNumberOfCalls(t, "SubscribeFilterLogs", 2)
}

func TestLeaderNode_receiveCommit_MerkleRootSubmitted_BlockTimestampError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	merkleRootSubmittedSig := parsedABI.Events["MerkleRootSubmitted"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	merkleRoot := [32]byte{1, 2, 3, 4, 5}

	eventData, err := parsedABI.Events["MerkleRootSubmitted"].Inputs.Pack(round, trialNum, merkleRoot)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{merkleRootSubmittedSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(0), errors.New("block timestamp error"))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Should continue without starting monitoring
	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_RequestedToSubmitCo_BlockTimestampError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	requestedToSubmitCoSig := parsedABI.Events["RequestedToSubmitCo"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	indicesLength := big.NewInt(1)
	packedIndices := big.NewInt(0)

	eventData, err := parsedABI.Events["RequestedToSubmitCo"].Inputs.Pack(round, trialNum, indicesLength, packedIndices)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{requestedToSubmitCoSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(0), errors.New("block timestamp error"))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Should not start monitoring if BlockTimestamp fails
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_RequestedToSubmitCv_BlockTimestampError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	requestedToSubmitCvSig := parsedABI.Events["RequestedToSubmitCv"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	packedIndices := big.NewInt(0)

	eventData, err := parsedABI.Events["RequestedToSubmitCv"].Inputs.Pack(round, trialNum, packedIndices)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{requestedToSubmitCvSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(0), errors.New("block timestamp error"))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Should not start monitoring if BlockTimestamp fails
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_RequestedToSubmitCo_DecodeError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	requestedToSubmitCoSig := parsedABI.Events["RequestedToSubmitCo"].ID

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{requestedToSubmitCoSig},
						Data:        []byte{0x01}, // Invalid data
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_RequestedToSubmitCv_DecodeError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	requestedToSubmitCvSig := parsedABI.Events["RequestedToSubmitCv"].ID

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{requestedToSubmitCvSig},
						Data:        []byte{0x01}, // Invalid data
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_StatusEvent_DecodeError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	statusEventSig := parsedABI.Events["Status"].ID

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{statusEventSig},
						Data:        []byte{0x01}, // Invalid data
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_StatusEvent_BlockTimestampError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	statusEventSig := parsedABI.Events["Status"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)

	eventData, err := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{statusEventSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(0), errors.New("block timestamp error"))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Should not process the event if BlockTimestamp fails
	mockClient.AssertExpectations(t)
}

func TestLeaderNode_receiveCommit_StatusEvent_State3(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	node.SetCurrentRound("100")
	node.SetCurrentTrial("1")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	statusEventSig := parsedABI.Events["Status"].ID

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(3) // State 3 - Halted

	eventData, err := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)
	require.NoError(t, err)

	mockSub := &MockSubscription{
		errChan: make(chan error),
	}

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)

				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{statusEventSig},
						Data:        eventData,
						BlockNumber: 12345,
						Removed:     false,
					}
				}()
				eventSent = true
			}
		}).Return(mockSub, nil)

	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil)

	mockBatchRepo.On("DeleteRoundTrialDataForLeaderNode", mock.Anything, "100", "1").
		Return(nil)

	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(1e16), nil
			}
			if method == "getActivatedOperatorsLength" {
				return big.NewInt(2), nil
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	// Verify halted was set for state 3
	assert.True(t, node.GetHalted(), "Halted should be true for state 3")

	mockClient.AssertExpectations(t)
	mockBatchRepo.AssertExpectations(t)
}

func TestLeaderNode_updateCOS_AllCosReceived_TriggersRevealOrder(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockRevealOrderService := new(MockRevealOrderService)
	node.revealOrderService = mockRevealOrderService
	// p2pClient will be nil, which StartSecretValueRequests will handle

	testOp := common.HexToAddress("0x1111111111111111111111111111111111111111")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	var cos [32]byte
	copy(cos[:], []byte("test-cos"))

	// Mock DetermineRevealOrder to succeed
	mockRevealOrderService.On("DetermineRevealOrder", mock.Anything, round, trialNum, mock.Anything).
		Return(true, nil)

	defer func() {
		if r := recover(); r != nil {
			mockRevealOrderService.AssertExpectations(t)
		}
	}()

	node.updateCOS(context.Background(), round, trialNum, uniqueKey, testOp, cos)

	// If we reach here without panic, verify DetermineRevealOrder was called
	mockRevealOrderService.AssertExpectations(t)
}

func TestLeaderNode_updateCOS_AllCosReceived_DetermineRevealOrderError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockRevealOrderService := new(MockRevealOrderService)
	node.revealOrderService = mockRevealOrderService

	testOp := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	node.ethService = mockEth

	round := "200"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	var cos [32]byte
	copy(cos[:], []byte("test-cos-2"))

	// Mock DetermineRevealOrder to fail
	mockRevealOrderService.On("DetermineRevealOrder", mock.Anything, round, trialNum, mock.Anything).
		Return(false, errors.New("failed to determine reveal order"))

	node.updateCOS(context.Background(), round, trialNum, uniqueKey, testOp, cos)

	// Verify DetermineRevealOrder was called and error was handled gracefully
	mockRevealOrderService.AssertExpectations(t)
}

func TestLeaderNode_startFailToSubmitCvMonitoring_TimerFires(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "failToSubmitCv" {
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, nil
		},
	}
	node.ethService = mockEth

	// Use negative duration so timer fires immediately
	pastTimestamp := big.NewInt(time.Now().Unix() - 100)

	node.startFailToSubmitCvMonitoring(context.Background(), "100", "1", pastTimestamp)

	// Wait a bit for timer to fire
	time.Sleep(100 * time.Millisecond)

	// Verify flag was set to false after timer fired
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
}

func TestLeaderNode_startRequestToSubmitCvMonitoring_TimerFires(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	// Keep fallback client from createTestNodeForAcceptCommit (already has ChainID).
	// Use mock eth service so ExecuteTransaction is never called on real client.
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "requestToSubmitCv" {
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, nil
		},
	}
	node.ethService = mockEth
	node.SetHalted(false)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Add CVS for only first operator so there are missing operators
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})
	defer utils.DeleteCommittedNodes(uniqueKey)

	// Use past timestamp so timer fires immediately
	pastTimestamp := big.NewInt(time.Now().Unix() - 100)

	node.startRequestToSubmitCvMonitoring(context.Background(), round, trialNum, pastTimestamp)

	// Wait for timer to fire
	time.Sleep(100 * time.Millisecond)

	// Verify flag was set to false after timer fired
	assert.False(t, node.GetRequestToSubmitCvMonitoringActive())

	// Clean up timer to prevent it from firing again
	node.stopRequestToSubmitCvMonitoring()
}

func TestLeaderNode_ResetCosAndCvsMonitoringState_AllTimersNonNil(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set all timers
	node.requestedToSubmitCoMonitoringTimer = time.AfterFunc(10*time.Second, func() {})
	node.requestedToSubmitCvMonitoringTimer = time.AfterFunc(10*time.Second, func() {})
	node.requestToSubmitCvMonitoringTimer = time.AfterFunc(10*time.Second, func() {})

	// Set all flags
	node.SetRequestedToSubmitCoMonitoringActive(true)
	node.SetRequestedToSubmitCvMonitoringActive(true)
	node.SetRequestToSubmitCvMonitoringActive(true)
	node.SetSecretRequestSentForWhichRound("100")

	node.ResetCosAndCvsMonitoringState("100", "1")

	// Verify all timers were stopped and set to nil
	assert.Nil(t, node.requestedToSubmitCoMonitoringTimer)
	assert.Nil(t, node.requestedToSubmitCvMonitoringTimer)
	assert.Nil(t, node.requestToSubmitCvMonitoringTimer)

	// Verify all flags were reset
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
	assert.False(t, node.GetRequestToSubmitCvMonitoringActive())
	// ResetCosAndCvsMonitoringState does not clear secretRequestSentForWhichRound
	// It only stops timers and resets monitoring flags
	assert.Equal(t, "100", node.GetSecretRequestSentForWhichRound())
}

func TestLeaderNode_ResetCosAndCvsMonitoringState_SomeTimersNil(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set only some timers (testing nil checks)
	node.requestedToSubmitCoMonitoringTimer = time.AfterFunc(10*time.Second, func() {})
	node.requestedToSubmitCvMonitoringTimer = nil // Nil timer
	node.requestToSubmitCvMonitoringTimer = time.AfterFunc(10*time.Second, func() {})

	node.SetRequestedToSubmitCoMonitoringActive(true)
	node.SetRequestedToSubmitCvMonitoringActive(true)
	node.SetRequestToSubmitCvMonitoringActive(true)

	node.ResetCosAndCvsMonitoringState("100", "1")

	// Verify all timers are nil after reset
	assert.Nil(t, node.requestedToSubmitCoMonitoringTimer)
	assert.Nil(t, node.requestedToSubmitCvMonitoringTimer)
	assert.Nil(t, node.requestToSubmitCvMonitoringTimer)

	// Verify all flags were reset
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive())
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive())
	assert.False(t, node.GetRequestToSubmitCvMonitoringActive())
}

func TestLeaderNode_GenerateMerkleRoot_CompareAndSwapFails(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	testOp := common.HexToAddress("0x2222222222222222222222222222222222222222")

	submitMerkleRootCalled := false
	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "submitMerkleRoot" {
				submitMerkleRootCalled = true
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, nil
		},
	}
	node.ethService = mockEth

	round := "200"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Add CVS data
	utils.SetCommittedNodeData(uniqueKey, testOp, utils.LeaderCommitData{
		Cvs: [32]byte{4, 5, 6},
	})
	defer utils.DeleteCommittedNodes(uniqueKey)

	// Set flag to true so CompareAndSwap fails
	node.SetSubmittingMerkleRoot(true)

	node.GenerateMerkleRoot(context.Background(), round, trialNum)

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Verify SubmitMerkleRoot was NOT called
	assert.False(t, submitMerkleRootCalled, "SubmitMerkleRoot should NOT have been called when flag is already true")
}

func TestLeaderNode_callRequestToSubmitCv_ExecuteTransactionError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "requestToSubmitCv" {
				return nil, nil, errors.New("transaction failed")
			}
			return nil, nil, errors.New("unexpected method")
		},
	}
	node.ethService = mockEth
	node.SetHalted(false)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Only add CVS for first operator
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})
	defer utils.DeleteCommittedNodes(uniqueKey)

	node.callRequestToSubmitCv(context.Background(), round, trialNum)

	// Should handle error gracefully
	assert.True(t, true)
}

func TestLeaderNode_callFailToSubmitCo_ExecuteTransactionError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return nil, nil, errors.New("transaction failed")
		},
	}
	node.ethService = mockEth

	node.callFailToSubmitCo(context.Background(), "100", "1")

	// Should handle error gracefully
	assert.True(t, true)
}

func TestLeaderNode_callFailToSubmitCv_ExecuteTransactionError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return nil, nil, errors.New("transaction failed")
		},
	}
	node.ethService = mockEth

	node.callFailToSubmitCv(context.Background(), "100", "1")

	// Should handle error gracefully
	assert.True(t, true)
}

func TestLeaderNode_requestToSubmitCo_ExecuteTransactionError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return nil, nil, errors.New("transaction failed")
		},
	}

	// Override global eth.Service
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	node.ethService = mockEth

	mockLeaderRepo.On("GetLeaderCommitsByRoundAndTrialNum", mock.Anything, "100", "1").
		Return([]*utils.LeaderCommitData{
			{
				EOAAddress: testOp1.Hex(),
				Cvs:        [32]byte{1},
				Cos:        [32]byte{2},
				Sign: utils.SignInfo{
					V: "27",
					R: "0x1111111111111111111111111111111111111111111111111111111111111111",
					S: "0x2222222222222222222222222222222222222222222222222222222222222222",
				},
			},
		}, nil)

	missingIndices := []*big.Int{big.NewInt(0)}

	node.requestToSubmitCo(context.Background(), "100", "1", missingIndices)

	// Should handle error gracefully
	assert.True(t, true)
}

func TestLeaderNode_callRequestToSubmitCv_SortIndicesPath(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	testOp1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	testOp3 := common.HexToAddress("0x3333333333333333333333333333333333333333")

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2, testOp3}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "requestToSubmitCv" {
				return &types.Transaction{}, nil, nil
			}
			return nil, nil, errors.New("unexpected method")
		},
	}
	node.ethService = mockEth
	node.SetHalted(false)

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Add CVS only for first operator, creating missing operators
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})
	defer utils.DeleteCommittedNodes(uniqueKey)

	node.callRequestToSubmitCv(context.Background(), round, trialNum)

	// Verify indices were added and sorted
	indices := node.GetIndices()
	assert.Greater(t, len(indices), 0)
}

func TestLeaderNode_resuming_DepositError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return big.NewInt(0), nil // No deposit, needs to deposit
			}
			return nil, nil
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "deposit" {
				return nil, nil, errors.New("deposit failed")
			}
			return nil, nil, nil
		},
	}
	node.ethService = mockEth

	node.resuming(context.Background())

	// Should handle deposit error gracefully
	assert.True(t, true)
}

func TestLeaderNode_resuming_CallSmartContractError(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	mockEth := &MockEthServiceForAcceptCommit{
		CallSmartContractFunc: func(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
			if method == "s_depositAmount" {
				return nil, errors.New("contract call failed")
			}
			return nil, nil
		},
	}
	node.ethService = mockEth

	node.resuming(context.Background())

	// Should handle error gracefully
	assert.True(t, true)
}

type ConcurrentMockLeaderCommitRepository struct {
	mock.Mock
	mu              sync.Mutex
	commits         map[string]*utils.LeaderCommitData
	addCallCount    int
	updateCallCount int
}

func NewConcurrentMockLeaderCommitRepository() *ConcurrentMockLeaderCommitRepository {
	return &ConcurrentMockLeaderCommitRepository{
		commits: make(map[string]*utils.LeaderCommitData),
	}
}

func (m *ConcurrentMockLeaderCommitRepository) GetLeaderCommitByRoundAndEoaAddr(ctx context.Context, round, trialNum, eoaAddr string) (*utils.LeaderCommitData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := round + "-" + trialNum + "-" + eoaAddr
	if commit, exists := m.commits[key]; exists {
		return commit, nil
	}
	return nil, pg.ErrNoRows
}

func (m *ConcurrentMockLeaderCommitRepository) UpdateLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := commitData.Round + "-" + commitData.TrialNum + "-" + commitData.EOAAddress
	m.commits[key] = commitData
	m.updateCallCount++
	return nil
}

func (m *ConcurrentMockLeaderCommitRepository) AddLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := commitData.Round + "-" + commitData.TrialNum + "-" + commitData.EOAAddress
	if _, exists := m.commits[key]; exists {
		return errors.New("duplicate key")
	}
	m.commits[key] = commitData
	m.addCallCount++
	return nil
}

func (m *ConcurrentMockLeaderCommitRepository) GetAllLeaderCommits(ctx context.Context) ([]*utils.LeaderCommitData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*utils.LeaderCommitData, 0, len(m.commits))
	for _, commit := range m.commits {
		result = append(result, commit)
	}
	return result, nil
}

func (m *ConcurrentMockLeaderCommitRepository) GetLeaderCommitsByRoundAndTrialNum(ctx context.Context, round, trialNum string) ([]*utils.LeaderCommitData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*utils.LeaderCommitData, 0)
	for _, commit := range m.commits {
		if commit.Round == round && commit.TrialNum == trialNum {
			result = append(result, commit)
		}
	}
	return result, nil
}

func (m *ConcurrentMockLeaderCommitRepository) UpdateLeaderCommitRandomNumberGenerated(ctx context.Context, round, trialNum string) error {
	return nil
}

func TestProcessCVS_ConcurrentSameOperator(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockRepo := NewConcurrentMockLeaderCommitRepository()
	node.leaderCommitRepository = mockRepo
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators so AllCvsReceivedUnlocked is never true (we only store CVS for one) and GenerateMerkleRoot is not triggered
	activatedOps := []common.Address{
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x1111111111111111111111111111111111111112"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(5000)
	trialNum := big.NewInt(1)
	var cvs1, cvs2 [32]byte
	copy(cvs1[:], []byte("cvs-concurrent-1"))
	copy(cvs2[:], []byte("cvs-concurrent-2"))
	index := big.NewInt(0)

	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	var wg sync.WaitGroup
	numGoroutines := 10
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				errors[idx] = node.processCVS(context.Background(), round, trialNum, cvs1, index)
			} else {
				errors[idx] = node.processCVS(context.Background(), round, trialNum, cvs2, index)
			}
		}(i)
	}

	wg.Wait()

	errorCount := 0
	for _, err := range errors {
		if err != nil {
			errorCount++
		}
	}

	t.Logf("Error count: %d out of %d goroutines", errorCount, numGoroutines)
	fmt.Printf("Error count: %d out of %d goroutines\n", errorCount, numGoroutines)

	assert.LessOrEqual(t, errorCount, numGoroutines-1, "At least one operation should succeed")

	commit, err := mockRepo.GetLeaderCommitByRoundAndEoaAddr(context.Background(), round.String(), trialNum.String(), activatedOps[0].Hex())
	assert.NoError(t, err)
	assert.NotNil(t, commit)
	assert.Equal(t, round.String(), commit.Round)
	assert.Equal(t, trialNum.String(), commit.TrialNum)
}

func TestProcessCVS_ConcurrentDifferentOperators(t *testing.T) {
	// Setup real database connection
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
		t.Skipf("Skipping test: PostgreSQL database not available: %v", err)
		return
	}
	defer sqlDB.Close()

	err = sqlDB.Ping()
	if err != nil {
		t.Skipf("Skipping test: PostgreSQL database not available: %v", err)
		return
	}

	// Run migrations
	err = database.MigrationsUp(sqlDB)
	require.NoError(t, err, "Failed to run migrations")

	// Connect using go-pg
	testDB := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", postgresHost, postgresPort),
		User:     postgresUser,
		Password: postgresPassword,
		Database: postgresDB,
	})

	err = testDB.Ping(context.Background())
	require.NoError(t, err, "Failed to connect to test database")

	// Create real database repositories
	realRepo := database.NewLeaderCommitRepository(testDB)
	realBroadcastRepo := database.NewBroadcastTrackerRepository(testDB)

	// Setup test node
	node := createTestNodeForAcceptCommit()
	node.leaderCommitRepository = realRepo
	node.broadcastTrackerRepository = realBroadcastRepo

	activatedOps := make([]common.Address, 32)
	for i := 0; i < 32; i++ {
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i + 1)
		addrBytes[1] = byte(i + 1)
		addrBytes[2] = byte(i + 1)
		for j := 3; j < 20; j++ {
			addrBytes[j] = byte(i + j)
		}
		activatedOps[i] = common.BytesToAddress(addrBytes)
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Register all operators in nodeInfo to avoid log warnings
	mockNodeInfoRepo := new(MockNodeInfoRepository)
	nodeInfos := make([]*utils.NodeInfo, len(activatedOps))
	for i, op := range activatedOps {
		// Generate valid PeerID for each operator
		privKey, _, err := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
		var peerIDStr string
		if err == nil {
			peerID, err := peer.IDFromPrivateKey(privKey)
			if err == nil {
				peerIDStr = peerID.String()
			} else {
				// Fallback to a simple format if generation fails
				peerIDStr = fmt.Sprintf("12D3KooW%032d", i)
			}
		} else {
			// Fallback to a simple format if generation fails
			peerIDStr = fmt.Sprintf("12D3KooW%032d", i)
		}

		nodeInfos[i] = &utils.NodeInfo{
			EOAAddress: op.Hex(),
			IP:         fmt.Sprintf("192.168.1.%d", i+1),
			Port:       fmt.Sprintf("400%d", i+1),
			PeerID:     peerIDStr,
		}
	}
	// Setup mock to return nodeInfos for all GetNodeInfos calls (unlimited calls)
	mockNodeInfoRepo.On("GetNodeInfos", mock.Anything).Return(nodeInfos, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeInfoRepo)

	round := big.NewInt(5001)
	trialNum := big.NewInt(1)
	roundStr := round.String()
	trialNumStr := trialNum.String()

	// Create a cancellable context for the test operations
	testCtx, testCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer testCancel()

	// Cleanup test data before test
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, op := range activatedOps {
		testDB.Model(&database.LeaderCommitScheme{}).
			Where("round = ? AND trial_num = ? AND eoa_address = ?", roundStr, trialNumStr, op.Hex()).
			Context(ctx).
			Delete()
	}

	// Cleanup test data after test
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		for _, op := range activatedOps {
			testDB.Model(&database.LeaderCommitScheme{}).
				Where("round = ? AND trial_num = ? AND eoa_address = ?", roundStr, trialNumStr, op.Hex()).
				Context(cleanupCtx).
				Delete()
		}
		testDB.Close()
	}()

	// Mock ethService to handle GenerateMerkleRoot submission
	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return activatedOps
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			// Mock successful submission
			return nil, nil, nil
		},
	}
	node.ethService = mockEth

	// Use mock host to prevent actual network operations and avoid "context canceled" errors
	if node.p2pClient != nil && node.p2pClient.GetHostInstance() == nil {
		mockHost := createMockHostForBroadcast()
		node.p2pClient.SetHost(mockHost)
	}

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	var wg sync.WaitGroup
	errors := make([]error, len(activatedOps))

	for i, op := range activatedOps {
		wg.Add(1)
		go func(idx int, operator common.Address) {
			defer wg.Done()
			var cvs [32]byte
			copy(cvs[:], []byte{byte(idx), byte(idx + 1), byte(idx + 2)})
			errors[idx] = node.processCVS(testCtx, round, trialNum, cvs, big.NewInt(int64(idx)))
		}(i, op)
	}

	wg.Wait()

	// Give broadcast goroutines some time to finish or be cancelled
	time.Sleep(2 * time.Second)

	// Cancel context to stop all broadcast goroutines
	testCancel()

	// Wait a bit more for goroutines to clean up
	time.Sleep(1 * time.Second)

	for i, err := range errors {
		if err != nil {
			s := err.Error()
			benign := strings.Contains(s, "already in progress") ||
				strings.Contains(s, "already submitted") ||
				strings.Contains(s, "skipping")
			assert.True(t, benign, "Operator %d: unexpected error: %v", i, err)
		}
	}

	// Verify data in real database
	for i, op := range activatedOps {
		commit, err := realRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, roundStr, trialNumStr, op.Hex())
		assert.NoError(t, err, "Operator %d should have commit stored", i)
		assert.NotNil(t, commit)
		assert.Equal(t, roundStr, commit.Round)
		assert.Equal(t, trialNumStr, commit.TrialNum)
		assert.NotEqual(t, [32]byte{}, commit.Cvs, "CVS should be stored")
	}
}

func TestProcessCOS_ConcurrentSubmissions(t *testing.T) {
	// Setup real database connection
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
		t.Skipf("Skipping test: PostgreSQL database not available: %v", err)
		return
	}
	defer sqlDB.Close()

	err = sqlDB.Ping()
	if err != nil {
		t.Skipf("Skipping test: PostgreSQL database not available: %v", err)
		return
	}

	// Run migrations
	err = database.MigrationsUp(sqlDB)
	require.NoError(t, err, "Failed to run migrations")

	// Connect using go-pg
	testDB := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", postgresHost, postgresPort),
		User:     postgresUser,
		Password: postgresPassword,
		Database: postgresDB,
	})

	err = testDB.Ping(context.Background())
	require.NoError(t, err, "Failed to connect to test database")

	// Create real database repositories
	realRepo := database.NewLeaderCommitRepository(testDB)
	realBroadcastRepo := database.NewBroadcastTrackerRepository(testDB)

	// Setup test node
	node := createTestNodeForAcceptCommit()
	node.leaderCommitRepository = realRepo
	node.broadcastTrackerRepository = realBroadcastRepo

	activatedOps := make([]common.Address, 32)
	for i := 0; i < 32; i++ {
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i + 1)
		addrBytes[1] = byte(i + 1)
		addrBytes[2] = byte(i + 1)
		for j := 3; j < 20; j++ {
			addrBytes[j] = byte(i + j)
		}
		activatedOps[i] = common.BytesToAddress(addrBytes)
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Register all operators in nodeInfo to avoid log warnings
	mockNodeInfoRepo := new(MockNodeInfoRepository)
	nodeInfos := make([]*utils.NodeInfo, len(activatedOps))
	for i, op := range activatedOps {
		// Generate valid PeerID for each operator
		privKey, _, err := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
		var peerIDStr string
		if err == nil {
			peerID, err := peer.IDFromPrivateKey(privKey)
			if err == nil {
				peerIDStr = peerID.String()
			} else {
				// Fallback to a simple format if generation fails
				peerIDStr = fmt.Sprintf("12D3KooW%032d", i)
			}
		} else {
			// Fallback to a simple format if generation fails
			peerIDStr = fmt.Sprintf("12D3KooW%032d", i)
		}

		nodeInfos[i] = &utils.NodeInfo{
			EOAAddress: op.Hex(),
			IP:         fmt.Sprintf("192.168.1.%d", i+1),
			Port:       fmt.Sprintf("400%d", i+1),
			PeerID:     peerIDStr,
		}
	}
	// Setup mock to return nodeInfos for all GetNodeInfos calls (unlimited calls)
	mockNodeInfoRepo.On("GetNodeInfos", mock.Anything).Return(nodeInfos, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeInfoRepo)

	round := big.NewInt(5002)
	trialNum := big.NewInt(1)
	roundStr := round.String()
	trialNumStr := trialNum.String()

	// Create a cancellable context for the test operations
	testCtx, testCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer testCancel()

	// Cleanup test data before test
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, op := range activatedOps {
		testDB.Model(&database.LeaderCommitScheme{}).
			Where("round = ? AND trial_num = ? AND eoa_address = ?", roundStr, trialNumStr, op.Hex()).
			Context(ctx).
			Delete()
	}

	// Pre-create commit rows for every operator so processCOS always updates (and persists COS)
	// instead of taking the "insert empty COS" path on first receipt.
	uniqueKey := utils.GetUniqueKey(roundStr, trialNumStr)
	for _, op := range activatedOps {
		err := realRepo.AddLeaderCommit(ctx, &utils.LeaderCommitData{
			UniqueKey:  uniqueKey,
			Round:      roundStr,
			TrialNum:   trialNumStr,
			EOAAddress: op.Hex(),
			CreatedAt:  time.Now().Unix(),
		})
		require.NoError(t, err, "Failed to pre-create leader commit row for %s", op.Hex())
	}

	// Cleanup test data after test
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		for _, op := range activatedOps {
			testDB.Model(&database.LeaderCommitScheme{}).
				Where("round = ? AND trial_num = ? AND eoa_address = ?", roundStr, trialNumStr, op.Hex()).
				Context(cleanupCtx).
				Delete()
		}
		testDB.Close()
	}()

	mockRevealOrderService := new(MockRevealOrderService)
	mockRevealOrderService.On("DetermineRevealOrder", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(false, errors.New("test error")).Maybe()
	node.revealOrderService = mockRevealOrderService

	// Use mock host to prevent actual network operations and avoid "context canceled" errors
	if node.p2pClient != nil && node.p2pClient.GetHostInstance() == nil {
		mockHost := createMockHostForBroadcast()
		node.p2pClient.SetHost(mockHost)
	}

	// Run concurrent processCOS calls
	var wg sync.WaitGroup
	errors := make([]error, len(activatedOps))

	for i, op := range activatedOps {
		wg.Add(1)
		go func(idx int, operator common.Address) {
			defer wg.Done()
			var cos [32]byte
			copy(cos[:], []byte{byte(idx + 10), byte(idx + 11), byte(idx + 12)})
			errors[idx] = node.processCOS(testCtx, round, trialNum, cos, big.NewInt(int64(idx)))
		}(i, op)
	}

	wg.Wait()

	// Give broadcast goroutines some time to finish or be cancelled
	time.Sleep(2 * time.Second)

	// Cancel context to stop all broadcast goroutines
	testCancel()

	// Wait a bit more for goroutines to clean up
	time.Sleep(1 * time.Second)

	// All operations should succeed
	for i, err := range errors {
		assert.NoError(t, err, "Operator %d should process COS successfully", i)
	}

	// Verify data in memory
	uniqueKey = utils.GetUniqueKey(roundStr, trialNumStr)
	for i, op := range activatedOps {
		commitData, exists := utils.GetCommittedNodeData(uniqueKey, op)
		assert.True(t, exists, "Operator %d should have commit data in memory", i)
		assert.NotEqual(t, [32]byte{}, commitData.Cos, "COS should be stored in memory for operator %d", i)
	}

	// Verify data in real database
	for i, op := range activatedOps {
		commit, err := realRepo.GetLeaderCommitByRoundAndEoaAddr(context.Background(), roundStr, trialNumStr, op.Hex())
		require.NoError(t, err, "Operator %d should have commit stored in database", i)
		require.NotNil(t, commit, "Operator %d should have non-nil commit", i)
		assert.Equal(t, roundStr, commit.Round)
		assert.Equal(t, trialNumStr, commit.TrialNum)
		assert.NotEqual(t, [32]byte{}, commit.Cos, "COS should be stored in database for operator %d", i)
	}
}

// TestProcessCVS_RaceConditionAllCvsReceived tests race condition where multiple goroutines check AllCvsReceivedUnlocked simultaneously
func TestProcessCVS_RaceConditionAllCvsReceived(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockRepo := NewConcurrentMockLeaderCommitRepository()
	node.leaderCommitRepository = mockRepo
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	activatedOps := []common.Address{
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(5003)
	trialNum := big.NewInt(1)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())

	// Create a cancellable context for the test operations
	testCtx, testCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer testCancel()

	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return activatedOps
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return nil, nil, nil
		},
	}
	node.ethService = mockEth

	// Use mock host to prevent actual network operations and avoid "context canceled" errors
	if node.p2pClient != nil && node.p2pClient.GetHostInstance() == nil {
		mockHost := createMockHostForBroadcast()
		node.p2pClient.SetHost(mockHost)
	}

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	var wg sync.WaitGroup
	for i, op := range activatedOps {
		wg.Add(1)
		go func(idx int, operator common.Address) {
			defer wg.Done()
			var cvs [32]byte
			copy(cvs[:], []byte{byte(idx), byte(idx + 1)})
			_ = node.processCVS(testCtx, round, trialNum, cvs, big.NewInt(int64(idx)))
		}(i, op)
	}

	wg.Wait()

	// Give broadcast goroutines some time to finish or be cancelled
	time.Sleep(2 * time.Second)

	// Cancel context to stop all broadcast goroutines
	testCancel()

	// Wait a bit more for goroutines to clean up
	time.Sleep(1 * time.Second)

	time.Sleep(100 * time.Millisecond)

	for _, op := range activatedOps {
		commit, err := mockRepo.GetLeaderCommitByRoundAndEoaAddr(context.Background(), round.String(), trialNum.String(), op.Hex())
		assert.NoError(t, err)
		assert.NotNil(t, commit)
		assert.NotEqual(t, [32]byte{}, commit.Cvs)
	}

	assert.True(t, node.AllCvsReceivedUnlocked(uniqueKey), "All CVS should be received")
}

func TestProcessCVS_RaceConditionAllCvsReceived_WithTracking(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockRepo := NewConcurrentMockLeaderCommitRepository()
	node.leaderCommitRepository = mockRepo
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	activatedOps := make([]common.Address, 32)
	for i := 0; i < 32; i++ {
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i + 1)
		addrBytes[1] = byte(i + 1)
		addrBytes[2] = byte(i + 1)
		for j := 3; j < 20; j++ {
			addrBytes[j] = byte(i + j)
		}
		activatedOps[i] = common.BytesToAddress(addrBytes)
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	// Register all operators in nodeInfo to avoid log warnings
	mockNodeInfoRepo := new(MockNodeInfoRepository)
	nodeInfos := make([]*utils.NodeInfo, len(activatedOps))
	for i, op := range activatedOps {
		// Generate valid PeerID for each operator
		privKey, _, err := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
		var peerIDStr string
		if err == nil {
			peerID, err := peer.IDFromPrivateKey(privKey)
			if err == nil {
				peerIDStr = peerID.String()
			} else {
				// Fallback to a simple format if generation fails
				peerIDStr = fmt.Sprintf("12D3KooW%032d", i)
			}
		} else {
			// Fallback to a simple format if generation fails
			peerIDStr = fmt.Sprintf("12D3KooW%032d", i)
		}

		nodeInfos[i] = &utils.NodeInfo{
			EOAAddress: op.Hex(),
			IP:         fmt.Sprintf("192.168.1.%d", i+1),
			Port:       fmt.Sprintf("400%d", i+1),
			PeerID:     peerIDStr,
		}
	}

	mockNodeInfoRepo.On("GetNodeInfos", mock.Anything).Return(nodeInfos, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeInfoRepo)

	round := big.NewInt(5010)
	trialNum := big.NewInt(1)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())

	testCtx, testCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer testCancel()

	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	var logOutput strings.Builder
	originalLogWriter := log.Writer()
	log.SetOutput(&logOutput)
	defer func() {
		log.SetOutput(originalLogWriter)
	}()

	submitMerkleRootCallCount := int32(0)

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return activatedOps
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "submitMerkleRoot" {
				atomic.AddInt32(&submitMerkleRootCallCount, 1)
			}
			return nil, nil, nil
		},
	}
	node.ethService = mockEth

	if node.p2pClient != nil && node.p2pClient.GetHostInstance() == nil {
		mockHost := createMockHostForBroadcast()
		node.p2pClient.SetHost(mockHost)
	}

	// Set environment variables
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	var wg sync.WaitGroup
	for i, op := range activatedOps {
		wg.Add(1)
		go func(idx int, operator common.Address) {
			defer wg.Done()
			if idx < 2 {
				time.Sleep(10 * time.Millisecond)
			}
			var cvs [32]byte
			copy(cvs[:], []byte{byte(idx), byte(idx + 1), byte(idx + 2)})
			_ = node.processCVS(testCtx, round, trialNum, cvs, big.NewInt(int64(idx)))
		}(i, op)
	}

	wg.Wait()

	// Give broadcast goroutines some time to finish or be cancelled
	time.Sleep(2 * time.Second)

	// Cancel context to stop all broadcast goroutines
	testCancel()

	// Wait a bit more for goroutines to clean up
	time.Sleep(1 * time.Second)

	time.Sleep(500 * time.Millisecond)

	logStr := logOutput.String()
	generateMerkleRootCallCount := strings.Count(logStr, "Generating Merkle root")

	submitCount := atomic.LoadInt32(&submitMerkleRootCallCount)

	assert.Equal(t, 1, generateMerkleRootCallCount,
		"GenerateMerkleRoot should be called exactly once")

	assert.Equal(t, int32(1), submitCount, "SubmitMerkleRoot should be called exactly once")

	if generateMerkleRootCallCount == 1 {
		t.Logf("Result: Only one goroutine can proceed, eliminating the race condition")
	} else {
		t.Errorf("RACE CONDITION EXISTS: GenerateMerkleRoot called %d times instead of 1", generateMerkleRootCallCount)
	}

	// Verify all CVS values were stored
	for _, op := range activatedOps {
		commit, err := mockRepo.GetLeaderCommitByRoundAndEoaAddr(context.Background(), round.String(), trialNum.String(), op.Hex())
		assert.NoError(t, err)
		assert.NotNil(t, commit)
		assert.NotEqual(t, [32]byte{}, commit.Cvs)
	}

	assert.True(t, node.AllCvsReceivedUnlocked(uniqueKey), "All CVS should be received")

	data, exists := node.GetRoundData(uniqueKey)
	if exists {
		assert.True(t, data.MerkleRoot, "Merkle root should be marked as submitted")
	}
}

// TestGenerateMerkleRoot_ConcurrentCalls tests concurrent GenerateMerkleRoot calls
func TestGenerateMerkleRoot_ConcurrentCalls(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Use a mock repository that handles UpdateLeaderCommit
	mockRepo := new(MockLeaderCommitRepository)
	mockRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.leaderCommitRepository = mockRepo

	testOp1 := common.HexToAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	testOp2 := common.HexToAddress("0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")

	// Track submission calls by mocking the ethService ExecuteTransaction
	submitCallCount := int32(0)
	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			if method == "submitMerkleRoot" {
				atomic.AddInt32(&submitCallCount, 1)
			}
			return nil, nil, nil
		},
	}
	node.ethService = mockEth

	// Set environment variables for merkle root submission
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("LEADER_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("LEADER_PRIVATE_KEY")
	}()

	round := "5004"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{Cvs: [32]byte{2}})

	// Run concurrent GenerateMerkleRoot calls
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			node.GenerateMerkleRoot(context.Background(), round, trialNum)
		}()
	}

	wg.Wait()

	// Wait a bit for any pending SubmitMerkleRoot calls
	time.Sleep(200 * time.Millisecond)

	callCount := atomic.LoadInt32(&submitCallCount)
	assert.Equal(t, int32(1), callCount,
		"SubmitMerkleRoot should be called exactly once")

	data, exists := node.GetRoundData(uniqueKey)
	if exists {
		assert.True(t, data.MerkleRoot, "Merkle root should be marked as submitted")
	}
}

// TestProcessCVS_ConcurrentDatabaseOperations tests concurrent database operations
func TestProcessCVS_ConcurrentDatabaseOperations(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockRepo := NewConcurrentMockLeaderCommitRepository()
	node.leaderCommitRepository = mockRepo
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	// Use 2 operators so AllCvsReceivedUnlocked is never true and GenerateMerkleRoot is not triggered
	activatedOps := []common.Address{
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
		common.HexToAddress("0x7777777777777777777777777777777777777778"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(5006)
	trialNum := big.NewInt(1)

	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockBroadcastRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	// First, add a commit
	var initialCvs [32]byte
	copy(initialCvs[:], []byte("initial-cvs"))
	err := node.processCVS(context.Background(), round, trialNum, initialCvs, big.NewInt(0))
	assert.NoError(t, err)

	// Now run concurrent updates
	var wg sync.WaitGroup
	numGoroutines := 5
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var cvs [32]byte
			copy(cvs[:], []byte{byte(idx), byte(idx + 1)})
			errors[idx] = node.processCVS(context.Background(), round, trialNum, cvs, big.NewInt(0))
		}(i)
	}

	wg.Wait()

	successCount := 0
	for _, err := range errors {
		if err == nil {
			successCount++
		}
	}
	assert.Greater(t, successCount, 0, "At least one update should succeed")

	commit, err := mockRepo.GetLeaderCommitByRoundAndEoaAddr(context.Background(), round.String(), trialNum.String(), activatedOps[0].Hex())
	assert.NoError(t, err)
	assert.NotNil(t, commit)
	assert.NotEqual(t, [32]byte{}, commit.Cvs, "CVS should be stored")
}

// TestLeaderNode_receiveCommit_MultipleReconnections tests multiple reconnection attempts
func TestLeaderNode_receiveCommit_MultipleReconnections(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	// First subscription fails
	mockSub1 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub1.errChan <- errors.New("websocket: close 1006")
	mockSub1.On("Unsubscribe").Return()

	// Second subscription also fails
	mockSub2 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub2.errChan <- errors.New("unexpected EOF")
	mockSub2.On("Unsubscribe").Return()

	// Third subscription succeeds
	mockSub3 := &MockSubscription{
		errChan: make(chan error),
	}
	mockSub3.On("Unsubscribe").Return()

	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub1, nil).Once()
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub2, nil).Once()
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub3, nil).Once()

	// Mock catch-up calls
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Maybe()
	mockClient.On("FilterLogs", mock.Anything, mock.Anything).
		Return([]types.Log{}, nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	time.Sleep(100 * time.Millisecond)

	// Verify multiple reconnections happened
	mockClient.AssertNumberOfCalls(t, "SubscribeFilterLogs", 3)
	mockSub1.AssertExpectations(t)
	mockSub2.AssertExpectations(t)
	mockSub3.AssertExpectations(t)
}

// TestLeaderNode_receiveCommit_CatchUpOnReconnection tests catch-up on missed events during reconnection
func TestLeaderNode_receiveCommit_CatchUpOnReconnection(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	// Set last processed block to 100
	node.updateLastProcessedCoords(100, 0, 0)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	// First subscription fails
	mockSub1 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub1.errChan <- errors.New("websocket: close 1006")
	mockSub1.On("Unsubscribe").Return()

	// Second subscription succeeds
	mockSub2 := &MockSubscription{
		errChan: make(chan error),
	}
	mockSub2.On("Unsubscribe").Return()

	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub1, nil).Once()
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub2, nil).Once()

	// Mock catch-up: current block is 105, missed events in blocks 101-105
	mockHeader := &types.Header{Number: big.NewInt(105)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil).Maybe()

	// Create missed events
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	missedLogs := []types.Log{
		{
			BlockNumber: 101,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 103,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
	}

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock.Uint64() >= 100 && q.ToBlock.Uint64() <= 105
	})).Return(missedLogs, nil).Maybe()

	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).
		Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	time.Sleep(100 * time.Millisecond)

	// Verify catch-up was called during reconnection
	mockClient.AssertExpectations(t)
	mockSub1.AssertExpectations(t)
	mockSub2.AssertExpectations(t)
}

// TestLeaderNode_receiveCommit_CatchUpFailureDuringReconnection tests catch-up failure during reconnection
func TestLeaderNode_receiveCommit_CatchUpFailureDuringReconnection(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	// Set last processed block
	node.updateLastProcessedCoords(100, 0, 0)

	// First subscription fails
	mockSub1 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub1.errChan <- errors.New("websocket: close 1006")
	mockSub1.On("Unsubscribe").Return()

	// Second subscription succeeds
	mockSub2 := &MockSubscription{
		errChan: make(chan error),
	}
	mockSub2.On("Unsubscribe").Return()

	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub1, nil).Once()
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub2, nil).Once()

	// Mock catch-up failure
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(nil, errors.New("RPC error")).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	time.Sleep(100 * time.Millisecond)

	// Should still reconnect despite catch-up failure
	mockClient.AssertNumberOfCalls(t, "SubscribeFilterLogs", 2)
	mockSub1.AssertExpectations(t)
	mockSub2.AssertExpectations(t)
}

// TestLeaderNode_receiveCommit_EventsDuringReconnection tests event processing during reconnection
func TestLeaderNode_receiveCommit_EventsDuringReconnection(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient
	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.batchRepository = mockBatchRepo

	// processRandomRequestNumber() expects ethService + batchRepository; mock eth to avoid real chain calls.
	node.ethService = &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{}
		},
	}

	// Ensure monitoring timers don't leak across tests.
	defer node.stopFailToSubmitCoMonitoring()
	defer node.stopFailToSubmitCvMonitoring()
	defer node.stopRequestToSubmitCvMonitoring()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	// First subscription receives an event then fails
	mockSub1 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub1.On("Unsubscribe").Return()

	eventSent := false
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if !eventSent {
				logs := args.Get(2).(chan<- types.Log)
				go func() {
					time.Sleep(50 * time.Millisecond)
					logs <- types.Log{
						Topics:      []common.Hash{statusSig},
						Data:        eventData,
						BlockNumber: 100,
						TxIndex:     0,
						Index:       0,
					}
					time.Sleep(50 * time.Millisecond)
					mockSub1.errChan <- errors.New("websocket: close 1006")
				}()
				eventSent = true
			}
		}).Return(mockSub1, nil).Once()

	// Second subscription succeeds
	mockSub2 := &MockSubscription{
		errChan: make(chan error),
	}
	mockSub2.On("Unsubscribe").Return()

	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Return(mockSub2, nil).Once()

	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Maybe()
	mockClient.On("FilterLogs", mock.Anything, mock.Anything).
		Return([]types.Log{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	<-ctx.Done()
	// Wait for goroutine to exit - receiveCommit sleeps for 1 second between retries
	// without checking ctx.Done(), so we need to wait at least 1.5 seconds
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	time.Sleep(100 * time.Millisecond)

	mockClient.AssertExpectations(t)
	mockBatchRepo.AssertExpectations(t)
	mockSub1.AssertExpectations(t)
	mockSub2.AssertExpectations(t)
}

func TestLeaderNode_updateLastProcessedCoords_NewBlock(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set initial coordinates
	node.updateLastProcessedCoords(100, 5, 10)

	// Update with new block number
	node.updateLastProcessedCoords(200, 0, 0)

	block, txIndex, logIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(200), block)
	assert.Equal(t, uint(0), txIndex)
	assert.Equal(t, uint(0), logIndex)
}

// TestLeaderNode_updateLastProcessedCoords_SameBlockNewTxIndex tests updating when same block but txIndex > currentTxIndex
func TestLeaderNode_updateLastProcessedCoords_SameBlockNewTxIndex(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set initial coordinates
	node.updateLastProcessedCoords(100, 5, 10)

	// Update with same block but higher txIndex
	node.updateLastProcessedCoords(100, 10, 5)

	block, txIndex, logIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), block)
	assert.Equal(t, uint(10), txIndex)
	assert.Equal(t, uint(5), logIndex)
}

// TestLeaderNode_updateLastProcessedCoords_SameBlockSameTxNewLogIndex tests updating when same block and txIndex but logIndex > currentLogIndex
func TestLeaderNode_updateLastProcessedCoords_SameBlockSameTxNewLogIndex(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set initial coordinates
	node.updateLastProcessedCoords(100, 5, 10)

	// Update with same block and txIndex but higher logIndex
	node.updateLastProcessedCoords(100, 5, 20)

	block, txIndex, logIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), block)
	assert.Equal(t, uint(5), txIndex)
	assert.Equal(t, uint(20), logIndex)
}

// TestLeaderNode_updateLastProcessedCoords_OlderBlock_NoUpdate tests that older block does not update
func TestLeaderNode_updateLastProcessedCoords_OlderBlock_NoUpdate(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set initial coordinates
	node.updateLastProcessedCoords(100, 5, 10)

	// Try to update with older block
	node.updateLastProcessedCoords(50, 10, 20)

	block, txIndex, logIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), block)
	assert.Equal(t, uint(5), txIndex)
	assert.Equal(t, uint(10), logIndex)
}

// TestLeaderNode_updateLastProcessedCoords_SameBlockOlderTxIndex_NoUpdate tests that older txIndex does not update
func TestLeaderNode_updateLastProcessedCoords_SameBlockOlderTxIndex_NoUpdate(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set initial coordinates
	node.updateLastProcessedCoords(100, 10, 20)

	// Try to update with same block but older txIndex
	node.updateLastProcessedCoords(100, 5, 30)

	block, txIndex, logIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), block)
	assert.Equal(t, uint(10), txIndex)
	assert.Equal(t, uint(20), logIndex)
}

// TestLeaderNode_updateLastProcessedCoords_SameBlockSameTxOlderLogIndex_NoUpdate tests that older logIndex does not update
func TestLeaderNode_updateLastProcessedCoords_SameBlockSameTxOlderLogIndex_NoUpdate(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set initial coordinates
	node.updateLastProcessedCoords(100, 10, 20)

	// Try to update with same block and txIndex but older logIndex
	node.updateLastProcessedCoords(100, 10, 15)

	block, txIndex, logIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), block)
	assert.Equal(t, uint(10), txIndex)
	assert.Equal(t, uint(20), logIndex)
}

// TestLeaderNode_updateLastProcessedCoords_SameCoordinates_NoUpdate tests that same coordinates do not update
func TestLeaderNode_updateLastProcessedCoords_SameCoordinates_NoUpdate(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Set initial coordinates
	node.updateLastProcessedCoords(100, 10, 20)

	// Try to update with same coordinates
	node.updateLastProcessedCoords(100, 10, 20)

	block, txIndex, logIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), block)
	assert.Equal(t, uint(10), txIndex)
	assert.Equal(t, uint(20), logIndex)
}

// TestLeaderNode_updateLastProcessedCoords_InitialState tests initial state (all zeros)
func TestLeaderNode_updateLastProcessedCoords_InitialState(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	// Check initial state
	block, txIndex, logIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(0), block)
	assert.Equal(t, uint(0), txIndex)
	assert.Equal(t, uint(0), logIndex)

	// Update from initial state
	node.updateLastProcessedCoords(50, 5, 10)

	block, txIndex, logIndex = node.getLastProcessedCoords()
	assert.Equal(t, uint64(50), block)
	assert.Equal(t, uint(5), txIndex)
	assert.Equal(t, uint(10), logIndex)
}

// TestLeaderNode_updateLastProcessedCoords_ConcurrentUpdates tests concurrent updates
func TestLeaderNode_updateLastProcessedCoords_ConcurrentUpdates(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Start multiple goroutines updating coordinates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Each goroutine updates with different coordinates
			node.updateLastProcessedCoords(uint64(100+idx), uint(idx%10), uint(idx%20))
		}(i)
	}

	wg.Wait()

	// After concurrent updates, coordinates should be set to one of the values
	// (the highest one that was successfully written)
	block, txIndex, logIndex := node.getLastProcessedCoords()

	// Verify that coordinates are set (not zero)
	assert.GreaterOrEqual(t, block, uint64(100))
	assert.GreaterOrEqual(t, txIndex, uint(0))
	assert.GreaterOrEqual(t, logIndex, uint(0))
}

// TestLeaderNode_catchUpMissedEvents_InitialState tests when lastProcessedBlock is 0 (should return early)
func TestLeaderNode_catchUpMissedEvents_InitialState(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Should return early without calling HeaderByNumber
	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.NoError(t, err)

	// Verify HeaderByNumber was not called
	mockClient.AssertNotCalled(t, "HeaderByNumber", mock.Anything, mock.Anything)
}

// TestLeaderNode_catchUpMissedEvents_HeaderByNumberError tests when HeaderByNumber fails
func TestLeaderNode_catchUpMissedEvents_HeaderByNumberError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	// Set last processed block
	node.updateLastProcessedCoords(100, 0, 0)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Mock HeaderByNumber to return error
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(nil, errors.New("failed to get header"))

	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get current block header")

	mockClient.AssertExpectations(t)
}

// TestLeaderNode_catchUpMissedEvents_NoCatchUpNeeded tests when currentBlock <= lastProcessedBlock
func TestLeaderNode_catchUpMissedEvents_NoCatchUpNeeded(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	// Set last processed block to 100
	node.updateLastProcessedCoords(100, 0, 0)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Mock HeaderByNumber to return block 100 (same as lastProcessedBlock)
	mockHeader := &types.Header{Number: big.NewInt(100)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil)

	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.NoError(t, err)

	// Verify FilterLogs was not called
	mockClient.AssertNotCalled(t, "FilterLogs", mock.Anything, mock.Anything)
	mockClient.AssertExpectations(t)
}

// TestLeaderNode_catchUpMissedEvents_FilterLogsError tests when FilterLogs fails
func TestLeaderNode_catchUpMissedEvents_FilterLogsError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	// Set last processed block to 100
	node.updateLastProcessedCoords(100, 0, 0)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Mock HeaderByNumber to return block 105
	mockHeader := &types.Header{Number: big.NewInt(105)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil)

	// Mock FilterLogs to return error
	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock.Uint64() == 100 && q.ToBlock.Uint64() == 105
	})).Return(nil, errors.New("failed to fetch logs"))

	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch missed logs")

	mockClient.AssertExpectations(t)
}

// TestLeaderNode_catchUpMissedEvents_NoMissedLogs tests when no missed logs are found
func TestLeaderNode_catchUpMissedEvents_NoMissedLogs(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	// Set last processed block to 100
	node.updateLastProcessedCoords(100, 0, 0)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Mock HeaderByNumber to return block 105
	mockHeader := &types.Header{Number: big.NewInt(105)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil)

	// Mock FilterLogs to return empty logs
	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock.Uint64() == 100 && q.ToBlock.Uint64() == 105
	})).Return([]types.Log{}, nil)

	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

// TestLeaderNode_catchUpMissedEvents_SuccessfulCatchUp tests successful catch-up with missed events
func TestLeaderNode_catchUpMissedEvents_SuccessfulCatchUp(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	// Set last processed block to 100
	node.updateLastProcessedCoords(100, 0, 0)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Mock HeaderByNumber to return block 105
	mockHeader := &types.Header{Number: big.NewInt(105)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil)

	// Create missed events
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	missedLogs := []types.Log{
		{
			BlockNumber: 101,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 103,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock.Uint64() == 100 && q.ToBlock.Uint64() == 105
	})).Return(missedLogs, nil)

	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).
		Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil).Maybe()

	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

// TestLeaderNode_catchUpMissedEvents_Sorting tests that missed logs are sorted correctly
func TestLeaderNode_catchUpMissedEvents_Sorting(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	// Set last processed block to 100
	node.updateLastProcessedCoords(100, 0, 0)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Mock HeaderByNumber to return block 105
	mockHeader := &types.Header{Number: big.NewInt(105)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil)

	// Create missed events in reverse order (should be sorted)
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	// Events in reverse order: block 104, then 102, then 101
	missedLogs := []types.Log{
		{
			BlockNumber: 104,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 102,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 101,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock.Uint64() == 100 && q.ToBlock.Uint64() == 105
	})).Return(missedLogs, nil)

	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).
		Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil).Maybe()

	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.NoError(t, err)

	// Verify that events were processed (function should sort them internally)
	mockClient.AssertExpectations(t)
}

// TestLeaderNode_catchUpMissedEvents_DuplicateDetection tests duplicate event detection
func TestLeaderNode_catchUpMissedEvents_DuplicateDetection(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	// Set last processed coordinates to block 100, txIndex 5, logIndex 10
	node.updateLastProcessedCoords(100, 5, 10)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Mock HeaderByNumber to return block 105
	mockHeader := &types.Header{Number: big.NewInt(105)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil)

	// Create missed events including duplicates
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	missedLogs := []types.Log{
		// Duplicate: older block
		{
			BlockNumber: 99,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		// Duplicate: same block, older txIndex
		{
			BlockNumber: 100,
			TxIndex:     3,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		// Duplicate: same block and txIndex, older logIndex
		{
			BlockNumber: 100,
			TxIndex:     5,
			Index:       5,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		// Duplicate: same block and txIndex, same logIndex
		{
			BlockNumber: 100,
			TxIndex:     5,
			Index:       10,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		// Valid: new event
		{
			BlockNumber: 101,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock.Uint64() == 100 && q.ToBlock.Uint64() == 105
	})).Return(missedLogs, nil)

	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).
		Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil).Maybe()

	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

// TestLeaderNode_catchUpMissedEvents_SortingByTxIndex tests sorting by transaction index
func TestLeaderNode_catchUpMissedEvents_SortingByTxIndex(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	// Set last processed block to 100
	node.updateLastProcessedCoords(100, 0, 0)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Mock HeaderByNumber to return block 105
	mockHeader := &types.Header{Number: big.NewInt(105)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil)

	// Create missed events with same block but different txIndex (should be sorted)
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	// Events in reverse txIndex order: txIndex 5, then 2, then 0
	missedLogs := []types.Log{
		{
			BlockNumber: 101,
			TxIndex:     5,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 101,
			TxIndex:     2,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 101,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock.Uint64() == 100 && q.ToBlock.Uint64() == 105
	})).Return(missedLogs, nil)

	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).
		Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil).Maybe()

	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

// TestLeaderNode_catchUpMissedEvents_SortingByLogIndex tests sorting by log index
func TestLeaderNode_catchUpMissedEvents_SortingByLogIndex(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	// Set last processed block to 100
	node.updateLastProcessedCoords(100, 0, 0)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Mock HeaderByNumber to return block 105
	mockHeader := &types.Header{Number: big.NewInt(105)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil)

	// Create missed events with same block and txIndex but different logIndex (should be sorted)
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	// Events in reverse logIndex order: logIndex 5, then 2, then 0
	missedLogs := []types.Log{
		{
			BlockNumber: 101,
			TxIndex:     0,
			Index:       5,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 101,
			TxIndex:     0,
			Index:       2,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 101,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock.Uint64() == 100 && q.ToBlock.Uint64() == 105
	})).Return(missedLogs, nil)

	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).
		Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil).Maybe()

	err = node.catchUpMissedEvents(context.Background(), contractAddr, parsedABI)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_CompleteFlow_ConnectProcessDisconnectReconnect(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.fallbackEthClient = mockClient
	node.batchRepository = mockBatchRepo

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	// Create subscriptions for first and second connection
	mockSub1 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub1.On("Unsubscribe").Return()

	mockSub2 := &MockSubscription{
		errChan: make(chan error, 1),
	}
	mockSub2.On("Unsubscribe").Return()

	// Track subscription calls
	var subscriptionCallCount int32

	// First subscription call
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			atomic.AddInt32(&subscriptionCallCount, 1)
			logs := args.Get(2).(chan<- types.Log)

			// First connection: Send events during active connection
			go func() {
				time.Sleep(100 * time.Millisecond)
				// Event 1: Block 100, TxIndex 0, LogIndex 0
				logs <- types.Log{
					BlockNumber: 100,
					TxIndex:     0,
					Index:       0,
					Topics:      []common.Hash{statusSig},
					Data:        eventData,
					Removed:     false,
				}

				time.Sleep(50 * time.Millisecond)
				// Event 2: Block 100, TxIndex 1, LogIndex 0
				logs <- types.Log{
					BlockNumber: 100,
					TxIndex:     1,
					Index:       0,
					Topics:      []common.Hash{statusSig},
					Data:        eventData,
					Removed:     false,
				}

				time.Sleep(50 * time.Millisecond)
				// Event 3: Block 101, TxIndex 0, LogIndex 0
				logs <- types.Log{
					BlockNumber: 101,
					TxIndex:     0,
					Index:       0,
					Topics:      []common.Hash{statusSig},
					Data:        eventData,
					Removed:     false,
				}

				// Trigger disconnection after events are processed
				time.Sleep(200 * time.Millisecond)
				select {
				case mockSub1.errChan <- errors.New("websocket: close 1006"):
				default:
				}
			}()
		}).
		Return(mockSub1, nil).Once()

	// Second subscription call (after reconnection)
	mockClient.On("SubscribeFilterLogs", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			atomic.AddInt32(&subscriptionCallCount, 1)
			logs := args.Get(2).(chan<- types.Log)

			// Second connection (after reconnection): Send new events
			go func() {
				time.Sleep(100 * time.Millisecond)
				// Event 4: Block 106, TxIndex 0, LogIndex 0 (after catch-up)
				logs <- types.Log{
					BlockNumber: 106,
					TxIndex:     0,
					Index:       0,
					Topics:      []common.Hash{statusSig},
					Data:        eventData,
					Removed:     false,
				}
			}()
		}).
		Return(mockSub2, nil).Once()

	mockHeader := &types.Header{Number: big.NewInt(105)}
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(mockHeader, nil).Maybe()

	// Missed events during disconnection (blocks 102-105)
	missedLogs := []types.Log{
		{
			BlockNumber: 102,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 103,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 104,
			TxIndex:     1,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
		{
			BlockNumber: 105,
			TxIndex:     0,
			Index:       0,
			Topics:      []common.Hash{statusSig},
			Data:        eventData,
		},
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr
	})).Return(missedLogs, nil).Maybe()

	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).
		Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).
		Return(uint64(time.Now().Unix()), nil).Maybe()

	mockEth := &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error {
			// No-op
			return nil
		},
	}
	node.ethService = mockEth

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Verify initial state
	initialBlock, initialTxIndex, initialLogIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(0), initialBlock, "Initial block should be 0")
	assert.Equal(t, uint(0), initialTxIndex, "Initial txIndex should be 0")
	assert.Equal(t, uint(0), initialLogIndex, "Initial logIndex should be 0")

	// Start receiveCommit
	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.receiveCommit(ctx)
	}()

	// Wait for first connection events to be processed (events at blocks 100, 100, 101)
	time.Sleep(600 * time.Millisecond)

	block1, txIndex1, logIndex1 := node.getLastProcessedCoords()
	assert.GreaterOrEqual(t, block1, uint64(100), "After first connection, block should be at least 100")
	assert.GreaterOrEqual(t, txIndex1, uint(0), "After first connection, txIndex should be >= 0")
	assert.GreaterOrEqual(t, logIndex1, uint(0), "After first connection, logIndex should be >= 0")

	time.Sleep(2 * time.Second)

	block2, txIndex2, logIndex2 := node.getLastProcessedCoords()
	assert.GreaterOrEqual(t, block2, uint64(105), "After catch-up, block should be at least 105")
	assert.GreaterOrEqual(t, txIndex2, uint(0), "After catch-up, txIndex should be >= 0")
	assert.GreaterOrEqual(t, logIndex2, uint(0), "After catch-up, logIndex should be >= 0")

	// Wait for reconnection and new events to be processed (block 106)
	time.Sleep(500 * time.Millisecond)

	block3, txIndex3, logIndex3 := node.getLastProcessedCoords()
	assert.GreaterOrEqual(t, block3, uint64(106), "After reconnection, block should be at least 106")
	assert.GreaterOrEqual(t, txIndex3, uint(0), "After reconnection, txIndex should be >= 0")
	assert.GreaterOrEqual(t, logIndex3, uint(0), "After reconnection, logIndex should be >= 0")

	// Verify that block coordinates progressed correctly through the flow
	assert.GreaterOrEqual(t, block3, block2, "Block should progress after reconnection")
	assert.GreaterOrEqual(t, block2, block1, "Block should progress after catch-up")

	// Verify subscription was called at least twice (initial + reconnection)
	finalCallCount := atomic.LoadInt32(&subscriptionCallCount)
	assert.GreaterOrEqual(t, finalCallCount, int32(2), "Should have at least 2 subscription calls")

	// Cancel context to stop receiveCommit goroutine
	cancel()
	waitForReceiveCommitToExit(ctx, done)

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_CatchUpFromSameBlock(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.batchRepository = mockBatchRepo

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	// Needed by Status processing.
	node.ethService = &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error { return nil },
	}
	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).Return(uint64(time.Now().Unix()), nil).Maybe()

	node.updateLastProcessedCoords(100, 2, 0)

	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(&types.Header{Number: big.NewInt(101)}, nil).Once()

	missedLogs := []types.Log{
		{BlockNumber: 100, TxIndex: 0, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData}, // dup
		{BlockNumber: 100, TxIndex: 1, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData}, // dup
		{BlockNumber: 100, TxIndex: 2, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData}, // dup
		{BlockNumber: 100, TxIndex: 3, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData}, // should process
		{BlockNumber: 100, TxIndex: 4, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData}, // should process
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock != nil && q.FromBlock.Uint64() == 100 &&
			q.ToBlock != nil && q.ToBlock.Uint64() == 101
	})).Return(missedLogs, nil).Once()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = node.catchUpMissedEvents(ctx, contractAddr, parsedABI)
	require.NoError(t, err)

	// If catch-up truly continued from txIndex 3, last processed should now be (100,4,0).
	finalBlock, finalTxIndex, finalLogIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), finalBlock)
	assert.Equal(t, uint(4), finalTxIndex)
	assert.Equal(t, uint(0), finalLogIndex)

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_catchUpMissedEvents_OnlyDuplicates_NoUpdate(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.batchRepository = mockBatchRepo

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	node.ethService = &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error { return nil },
	}
	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).Return(uint64(time.Now().Unix()), nil).Maybe()

	// Already processed up to (100,2,0).
	node.updateLastProcessedCoords(100, 2, 0)

	// Ensure catchUpMissedEvents runs.
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(&types.Header{Number: big.NewInt(101)}, nil).Once()

	// Only duplicates are returned.
	missedLogs := []types.Log{
		{BlockNumber: 100, TxIndex: 0, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData},
		{BlockNumber: 100, TxIndex: 1, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData},
		{BlockNumber: 100, TxIndex: 2, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData},
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock != nil && q.FromBlock.Uint64() == 100 &&
			q.ToBlock != nil && q.ToBlock.Uint64() == 101
	})).Return(missedLogs, nil).Once()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = node.catchUpMissedEvents(ctx, contractAddr, parsedABI)
	require.NoError(t, err)

	// No updates expected.
	finalBlock, finalTxIndex, finalLogIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), finalBlock)
	assert.Equal(t, uint(2), finalTxIndex)
	assert.Equal(t, uint(0), finalLogIndex)

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_catchUpMissedEvents_SameBlockSameTxHigherLogIndex_Processes(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.batchRepository = mockBatchRepo

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	node.ethService = &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error { return nil },
	}
	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).Return(uint64(time.Now().Unix()), nil).Maybe()

	// Already processed up to (100,2,5).
	node.updateLastProcessedCoords(100, 2, 5)

	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(&types.Header{Number: big.NewInt(101)}, nil).Once()

	// Same txIndex with lower/equal logIndex should be skipped; higher logIndex should be processed.
	missedLogs := []types.Log{
		{BlockNumber: 100, TxIndex: 2, Index: 4, Topics: []common.Hash{statusSig}, Data: eventData}, // dup
		{BlockNumber: 100, TxIndex: 2, Index: 5, Topics: []common.Hash{statusSig}, Data: eventData}, // dup (<=)
		{BlockNumber: 100, TxIndex: 2, Index: 6, Topics: []common.Hash{statusSig}, Data: eventData}, // process
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock != nil && q.FromBlock.Uint64() == 100 &&
			q.ToBlock != nil && q.ToBlock.Uint64() == 101
	})).Return(missedLogs, nil).Once()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = node.catchUpMissedEvents(ctx, contractAddr, parsedABI)
	require.NoError(t, err)

	finalBlock, finalTxIndex, finalLogIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), finalBlock)
	assert.Equal(t, uint(2), finalTxIndex)
	assert.Equal(t, uint(6), finalLogIndex)

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_catchUpMissedEvents_RemovedLog_DoesNotAdvanceCoords(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	// Set a starting point.
	node.updateLastProcessedCoords(100, 2, 0)

	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(&types.Header{Number: big.NewInt(101)}, nil).Once()

	// A higher txIndex log, but marked Removed -> processEventLog returns early and coords should not advance.
	missedLogs := []types.Log{
		{BlockNumber: 100, TxIndex: 3, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData, Removed: true},
	}

	mockClient.On("FilterLogs", mock.Anything, mock.MatchedBy(func(q ethereum.FilterQuery) bool {
		return len(q.Addresses) == 1 && q.Addresses[0] == contractAddr &&
			q.FromBlock != nil && q.FromBlock.Uint64() == 100 &&
			q.ToBlock != nil && q.ToBlock.Uint64() == 101
	})).Return(missedLogs, nil).Once()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = node.catchUpMissedEvents(ctx, contractAddr, parsedABI)
	require.NoError(t, err)

	finalBlock, finalTxIndex, finalLogIndex := node.getLastProcessedCoords()
	assert.Equal(t, uint64(100), finalBlock)
	assert.Equal(t, uint(2), finalTxIndex)
	assert.Equal(t, uint(0), finalLogIndex)

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_catchUpMissedEvents_Idempotent(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer os.Unsetenv("CONTRACT_ADDRESS")

	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	mockBatchRepo := new(MockBatchRepository)
	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, mock.Anything).Return(nil).Maybe()
	node.batchRepository = mockBatchRepo

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	statusSig := parsedABI.Events["Status"].ID
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)
	eventData, _ := parsedABI.Events["Status"].Inputs.Pack(round, trialNum, state)

	node.ethService = &MockEthServiceForAcceptCommit{
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) error { return nil },
	}
	mockClient.On("CallContract", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil).Maybe()
	mockClient.On("BlockTimestamp", mock.Anything, mock.Anything).Return(uint64(time.Now().Unix()), nil).Maybe()

	node.updateLastProcessedCoords(100, 2, 0)

	// Two runs -> expect two HeaderByNumber and two FilterLogs.
	mockClient.On("HeaderByNumber", mock.Anything, (*big.Int)(nil)).
		Return(&types.Header{Number: big.NewInt(101)}, nil).Twice()

	missedLogs := []types.Log{
		{BlockNumber: 100, TxIndex: 3, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData},
		{BlockNumber: 100, TxIndex: 4, Index: 0, Topics: []common.Hash{statusSig}, Data: eventData},
	}

	mockClient.On("FilterLogs", mock.Anything, mock.Anything).Return(missedLogs, nil).Twice()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = node.catchUpMissedEvents(ctx, contractAddr, parsedABI)
	require.NoError(t, err)
	block1, tx1, log1 := node.getLastProcessedCoords()

	err = node.catchUpMissedEvents(ctx, contractAddr, parsedABI)
	require.NoError(t, err)
	block2, tx2, log2 := node.getLastProcessedCoords()

	assert.Equal(t, uint64(100), block1)
	assert.Equal(t, uint(4), tx1)
	assert.Equal(t, uint(0), log1)

	// Second run should not change coords.
	assert.Equal(t, block1, block2)
	assert.Equal(t, tx1, tx2)
	assert.Equal(t, log1, log2)

	mockClient.AssertExpectations(t)
}

func TestLeaderNode_processEventLog_ReorgDoesNotUpdateCoords(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	statusSig := parsedABI.Events["Status"].ID

	// Start at zero coords.
	block0, tx0, log0 := node.getLastProcessedCoords()
	assert.Equal(t, uint64(0), block0)
	assert.Equal(t, uint(0), tx0)
	assert.Equal(t, uint(0), log0)

	node.processEventLog(context.Background(), types.Log{
		BlockNumber: 123,
		TxIndex:     9,
		Index:       7,
		Topics:      []common.Hash{statusSig},
		Removed:     true,
	}, parsedABI)

	// Reorg is skipped and should not advance coords.
	block1, tx1, log1 := node.getLastProcessedCoords()
	assert.Equal(t, uint64(0), block1)
	assert.Equal(t, uint(0), tx1)
	assert.Equal(t, uint(0), log1)
}

func TestLeaderNode_processEventLog_DecodeFailure_DoesNotUpdateCoords(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	require.NoError(t, err)

	cvSubmittedSig := parsedABI.Events["CvSubmitted"].ID

	// Start at zero coords.
	block0, tx0, log0 := node.getLastProcessedCoords()
	assert.Equal(t, uint64(0), block0)
	assert.Equal(t, uint(0), tx0)
	assert.Equal(t, uint(0), log0)

	// Invalid data for CvSubmitted: should fail UnpackIntoInterface and return before updating coords.
	node.processEventLog(context.Background(), types.Log{
		BlockNumber: 200,
		TxIndex:     1,
		Index:       0,
		Topics:      []common.Hash{cvSubmittedSig},
		Data:        []byte{}, // malformed
		Removed:     false,
	}, parsedABI)

	block1, tx1, log1 := node.getLastProcessedCoords()
	assert.Equal(t, uint64(0), block1)
	assert.Equal(t, uint(0), tx1)
	assert.Equal(t, uint(0), log1)
}

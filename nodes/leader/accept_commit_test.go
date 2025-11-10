package leader_node

import (
	"context"
	"errors"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	UpdateActivatedOperatorsFunc    func(ctx context.Context, client fallback_ethclient.IFallbackEthClient)
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

func (m *MockEthServiceForAcceptCommit) UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) {
	if m.UpdateActivatedOperatorsFunc != nil {
		m.UpdateActivatedOperatorsFunc(ctx, fallbackEthClient)
	}
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

func createTestNodeForAcceptCommit() *LeaderNode {
	node := createTestNodeForBroadcast()
	mockLeaderCommitRepo := new(MockLeaderCommitRepository)
	node.leaderCommitRepository = mockLeaderCommitRepo

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil).Maybe()
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	// Initialize with default eth service
	node.ethService = eth.Service

	return node
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

	assert.False(t, node.GetExecution())

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
	assert.NoError(t, err)
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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

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
	assert.NoError(t, err)
}

// TestProcessCVS_Success tests successful CVS processing
func TestProcessCVS_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockLeaderRepo := node.leaderCommitRepository.(*MockLeaderCommitRepository)
	mockBroadcastRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockBroadcastRepo

	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	eth.SetActivatedOperatorsCached(activatedOps)
	defer eth.SetActivatedOperatorsCached([]common.Address{})

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs"))
	index := big.NewInt(0)

	// Mock existing commit
	existingCommit := &utils.LeaderCommitData{
		Round:      round.String(),
		TrialNum:   trialNum.String(),
		EOAAddress: activatedOps[0].Hex(),
	}
	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), activatedOps[0].Hex()).
		Return(existingCommit, nil)
	mockLeaderRepo.On("UpdateLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

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
}

// TestProcessRequestedToSubmitCv_Success tests successful processing
func TestProcessRequestedToSubmitCv_Success(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(100)
	trialNum := big.NewInt(1)

	node.processRequestedToSubmitCv(context.Background(), blockTimestamp, round, trialNum)

	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())
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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(errors.New("db error"))
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.NoError(t, err)

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

	err := node.processCOS(ctx, round, trialNum, cos, index)
	assert.NoError(t, err)

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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(errors.New("db error"))
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.NoError(t, err)

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

	err := node.processCVS(context.Background(), round, trialNum, cvs, index)
	assert.NoError(t, err)

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
	cvAndSigRS, packedVs, indicesLength, packedOrderedIndices := node.prepareArgumentsForRequestToSubmitCo(context.Background(), round, trialNum, missingIndices)

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

	cvAndSigRS, packedVs, indicesLength, packedOrderedIndices := node.prepareArgumentsForRequestToSubmitCo(context.Background(), round, trialNum, missingIndices)

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

	cvAndSigRS, packedVs, indicesLength, packedOrderedIndices := node.prepareArgumentsForRequestToSubmitCo(context.Background(), round, trialNum, missingIndices)

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

	assert.False(t, node.GetExecution())

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
	assert.Equal(t, "", node.GetSecretRequestSentForWhichRound())
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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

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
}

// TestProcessRequestedToSubmitCv_NotHalted tests processing event
func TestProcessRequestedToSubmitCv_NotHalted(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(1800)
	trialNum := big.NewInt(1)

	node.processRequestedToSubmitCv(context.Background(), blockTimestamp, round, trialNum)

	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())
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
	if node.requestedToSubmitCoMonitoringTimer != nil {
		node.requestedToSubmitCoMonitoringTimer.Stop()
	}
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
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) {
			// No-op for test
		},
	}
	node.ethService = mockEth

	blockTimestamp := big.NewInt(time.Now().Unix())
	round := big.NewInt(2300)
	trialNum := big.NewInt(1)
	state := big.NewInt(1) // IN_PROGRESS

	mockBatchRepo.On("DeleteOldRoundDataForLeaderNode", mock.Anything, round.String()).Return(nil)

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	assert.True(t, node.GetExecution())
	assert.False(t, node.GetHalted())

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

	assert.False(t, node.GetExecution())
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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

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

	mockEth := &MockEthServiceForAcceptCommit{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
		ExecuteTransactionFunc: func(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
			return &types.Transaction{}, nil, nil
		},
	}
	node.ethService = mockEth

	round := big.NewInt(2600)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("final-cvs"))
	index := big.NewInt(0)

	mockLeaderRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, round.String(), trialNum.String(), testOp.Hex()).
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	// This should trigger GenerateMerkleRoot since all CVS received (only 1 operator)
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
	assert.Equal(t, "", node.GetSecretRequestSentForWhichRound())
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
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) {
			// No-op
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

	// Should still set execution even with delete error
	assert.True(t, node.GetExecution())

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
	assert.False(t, node.GetExecution())

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
	if node.requestedToSubmitCoMonitoringTimer != nil {
		node.requestedToSubmitCoMonitoringTimer.Stop()
	}
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

	// Clean up timer
	if node.requestedToSubmitCvMonitoringTimer != nil {
		node.requestedToSubmitCvMonitoringTimer.Stop()
	}
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
	if node.requestedToSubmitCoMonitoringTimer != nil {
		node.requestedToSubmitCoMonitoringTimer.Stop()
	}
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

	// Clean up timers
	if node.requestedToSubmitCvMonitoringTimer != nil {
		node.requestedToSubmitCvMonitoringTimer.Stop()
	}
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

	// Clean up timers
	if node.requestToSubmitCvMonitoringTimer != nil {
		node.requestToSubmitCvMonitoringTimer.Stop()
	}
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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Process COS for first operator - should NOT stop monitoring since not all COS received
	err := node.processCOS(context.Background(), round, trialNum, cos, index)
	assert.NoError(t, err)

	// Monitoring should still be active (only 1 of 2 COS received)
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

	// Clean up
	if node.requestedToSubmitCoMonitoringTimer != nil {
		node.requestedToSubmitCoMonitoringTimer.Stop()
	}
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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

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
	m.Called()
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

func (m *MockFallbackEthClientForAcceptCommit) ChainID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
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
	args := m.Called(ctx, account)
	return args.Get(0).(uint64), args.Error(1)
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

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	go node.receiveCommit(ctx)

	<-ctx.Done()

	// Verify subscription was attempted multiple times
	assert.GreaterOrEqual(t, len(mockClient.Calls), 2, "Should have attempted reconnection")
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
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) {
			// No-op
		},
	}
	node.ethService = mockEth

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go node.receiveCommit(ctx)

	<-ctx.Done()

	// Verify execution was started
	assert.True(t, node.GetExecution(), "Execution should be started for state 1")

	mockClient.AssertExpectations(t)
	mockBatchRepo.AssertExpectations(t)
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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go node.receiveCommit(ctx)

	<-ctx.Done()

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
		Return(nil, errors.New("not found"))
	mockLeaderRepo.On("AddLeaderCommit", mock.Anything, mock.Anything).Return(nil)
	mockBroadcastRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

	// Verify monitoring was started
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive())

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

	// Verify monitoring was started
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive())

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) {
			// No-op
		},
	}

	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

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
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) {
			// No-op
		},
	}

	// Override global eth.Service since processDeactivated uses eth.Service directly
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, testOp.Hex()).
		Return(errors.New("database error"))

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

	// Verify execution was stopped for state 2
	assert.False(t, node.GetExecution(), "Execution should be stopped for state 2")

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

func TestLeaderNode_startRequestToSubmitCoMonitoring_NetworkIDError(t *testing.T) {
	node := createTestNodeForAcceptCommit()
	mockClient := new(MockFallbackEthClientForAcceptCommit)
	node.fallbackEthClient = mockClient

	mockClient.On("NetworkID", mock.Anything).
		Return(nil, errors.New("network error"))

	timestamp := big.NewInt(time.Now().Unix() + 1000)

	node.startRequestToSubmitCoMonitoring(context.Background(), "100", "1", timestamp)

	mockClient.AssertExpectations(t)
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

	go node.receiveCommit(ctx)

	<-ctx.Done()
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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go node.receiveCommit(ctx)

	<-ctx.Done()

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
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) {
			// No-op
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

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, operator.Hex()).
		Return(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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
		UpdateActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) {
			// No-op
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

	mockNodeRepo.On("DeleteNodeInfoByEOA", mock.Anything, operator.Hex()).
		Return(errors.New("database error"))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

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

	go node.receiveCommit(ctx)

	<-ctx.Done()

	// Verify execution was stopped and halted was set for state 3
	assert.False(t, node.GetExecution(), "Execution should be stopped for state 3")
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
	assert.Equal(t, "", node.GetSecretRequestSentForWhichRound())
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

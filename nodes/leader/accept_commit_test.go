package leader_node

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/utils"
)

type MockRevealOrderService struct {
	mock.Mock
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

func (m *MockRevealOrderService) DetermineRevealOrder(round, trialNum string, activatedOps []common.Address) (bool, error) {
	args := m.Called(round, trialNum, activatedOps)
	return args.Bool(0), args.Error(1)
}

func createTestNodeForAcceptCommit() *LeaderNode {
	node := createTestNodeForBroadcast()
	mockLeaderCommitRepo := new(MockLeaderCommitRepository)
	node.leaderCommitRepository = mockLeaderCommitRepo

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil).Maybe()
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

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

// TestStartRequestToSubmitCoMonitoring_NilTimestamp tests with nil timestamp
func TestStartRequestToSubmitCoMonitoring_NilTimestamp(t *testing.T) {
	node := createTestNodeForAcceptCommit()

	node.startRequestToSubmitCoMonitoring(context.Background(), "100", "1", nil)

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

	// Old monitoring should be stopped, new monitoring for failToSubmit should be active
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

	// Mock no existing commit - will create
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

	// Try to start again - should return early
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

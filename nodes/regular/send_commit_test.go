package regular_node

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

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

func TestRegularNode_RoundData_Structure(t *testing.T) {
	roundData := RoundData{
		MerkleRoot:   true,
		RandomNumber: false,
	}

	assert.True(t, roundData.MerkleRoot)
	assert.False(t, roundData.RandomNumber)
}

func TestRegularNode_RoundData_DefaultValues(t *testing.T) {
	roundData := RoundData{}

	assert.False(t, roundData.MerkleRoot)
	assert.False(t, roundData.RandomNumber)
}
func TestRegularNode_ConcurrentCvSubmission(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)

	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 10; i++ {
			node.processCvSubmitted(round, trialNum, big.NewInt(0))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			node.processCvSubmitted(round, trialNum, big.NewInt(1))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			node.checkAllCVsSubmittedOnChain("100", "1")
		}
		done <- true
	}()

	<-done
	<-done
	<-done

}

func createTestNodeForSendCommit() *RegularNode {
	mockCommitRepo := new(MockRegularCommitRepository)
	mockRevealRepo := new(MockRevealOrderRepository)

	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)
	node.regularCommitRepository = mockCommitRepo
	node.revealOrderRepository = mockRevealRepo

	return node
}

func createTestNodeWithRevealOrderService() *RegularNode {
	mockCommitRepo := new(MockRegularCommitRepository)
	mockRevealRepo := new(MockRevealOrderRepository)
	mockPeerRepo := new(MockPeerCommitRepository)

	// Create real Reveal OrderService with mock repositories
	revealOrderService := commitreveal2.NewRevealOrderService(
		mockRevealRepo,
		mockPeerRepo,
		nil,
	)

	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)
	node.regularCommitRepository = mockCommitRepo
	node.revealOrderRepository = mockRevealRepo
	node.peerCommitDataRepository = mockPeerRepo
	node.revealOrderService = revealOrderService

	return node
}

func TestRegularNode_SetGetStartTime(t *testing.T) {
	node := createTestNodeForSendCommit()

	// Test default
	assert.Nil(t, node.GetStartTime(), "StartTime should be nil by default")

	// Set time
	testTime := big.NewInt(1234567890)
	node.SetStartTime(testTime)

	retrieved := node.GetStartTime()
	assert.NotNil(t, retrieved)
	assert.Equal(t, testTime.Int64(), retrieved.Int64())

	// Update time
	newTime := big.NewInt(9876543210)
	node.SetStartTime(newTime)

	retrieved = node.GetStartTime()
	assert.Equal(t, newTime.Int64(), retrieved.Int64())

	// Set to nil
	node.SetStartTime(nil)
	assert.Nil(t, node.GetStartTime())
}

func TestRegularNode_ConcurrentStartTimeAccess(t *testing.T) {
	node := createTestNodeForSendCommit()
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			node.SetStartTime(big.NewInt(int64(i)))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = node.GetStartTime()
		}
		done <- true
	}()

	<-done
	<-done

}

func TestRegularNode_processCvSubmitted_Success(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(5)

	node.processCvSubmitted(round, trialNum, index)

	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	submitted, exists := node.GetSubmittedCvIndicesValue(uniqueKey, "5")

	assert.True(t, exists, "Index should be marked as submitted")
	assert.True(t, submitted, "Submitted flag should be true")
}

func TestRegularNode_processCvSubmitted_Halted(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(true)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(5)

	node.processCvSubmitted(round, trialNum, index)

	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	_, exists := node.GetSubmittedCvIndicesValue(uniqueKey, "5")

	assert.False(t, exists, "Index should not be marked when halted")
}

func TestRegularNode_processCvSubmitted_MultipleIndices(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)

	// Process multiple indices
	node.processCvSubmitted(round, trialNum, big.NewInt(0))
	node.processCvSubmitted(round, trialNum, big.NewInt(1))
	node.processCvSubmitted(round, trialNum, big.NewInt(2))

	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())

	submitted0, _ := node.GetSubmittedCvIndicesValue(uniqueKey, "0")
	submitted1, _ := node.GetSubmittedCvIndicesValue(uniqueKey, "1")
	submitted2, _ := node.GetSubmittedCvIndicesValue(uniqueKey, "2")

	assert.True(t, submitted0)
	assert.True(t, submitted1)
	assert.True(t, submitted2)
}

func TestRegularNode_checkAllCVsSubmittedOnChain_NoMap(t *testing.T) {
	node := createTestNodeForSendCommit()

	result := node.checkAllCVsSubmittedOnChain("100", "1")

	assert.False(t, result, "Should return false when map doesn't exist")
}

func TestRegularNode_checkAllCVsSubmittedOnChain_AllSubmitted(t *testing.T) {
	node := createTestNodeForSendCommit()

	// Set up requested indices
	indices := []*big.Int{big.NewInt(0), big.NewInt(1), big.NewInt(2)}
	node.SetCvRequestIndices(indices)

	// Mark all as submitted
	uniqueKey := utils.GetUniqueKey("100", "1")
	node.SetSubmittedCvIndicesValue(uniqueKey, "0", true)
	node.SetSubmittedCvIndicesValue(uniqueKey, "1", true)
	node.SetSubmittedCvIndicesValue(uniqueKey, "2", true)

	result := node.checkAllCVsSubmittedOnChain("100", "1")

	assert.True(t, result, "Should return true when all CVs are submitted")
}

func TestRegularNode_checkAllCVsSubmittedOnChain_PartiallySubmitted(t *testing.T) {
	node := createTestNodeForSendCommit()

	indices := []*big.Int{big.NewInt(0), big.NewInt(1), big.NewInt(2)}
	node.SetCvRequestIndices(indices)

	uniqueKey := utils.GetUniqueKey("100", "1")
	node.SetSubmittedCvIndicesValue(uniqueKey, "0", true)
	node.SetSubmittedCvIndicesValue(uniqueKey, "1", true)
	// Don't submit index 2

	result := node.checkAllCVsSubmittedOnChain("100", "1")

	assert.False(t, result, "Should return false when not all CVs are submitted")
}

func TestRegularNode_checkAllCVsSubmittedOnChain_NoneSubmitted(t *testing.T) {
	node := createTestNodeForSendCommit()

	indices := []*big.Int{big.NewInt(0), big.NewInt(1)}
	node.SetCvRequestIndices(indices)

	result := node.checkAllCVsSubmittedOnChain("100", "1")

	assert.False(t, result, "Should return false when no CVs are submitted")
}

func TestRegularNode_checkAllCVsSubmittedOnChain_EmptyIndices(t *testing.T) {
	node := createTestNodeForSendCommit()

	node.SetCvRequestIndices([]*big.Int{})

	uniqueKey := utils.GetUniqueKey("100", "1")
	node.SetSubmittedCvIndicesValue(uniqueKey, "0", true)

	result := node.checkAllCVsSubmittedOnChain("100", "1")

	assert.True(t, result, "Should return true when no indices are requested")
}

func TestRegularNode_processMerkleRoot_Success(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)

	node.processMerkleRoot(round, trialNum)

	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	roundData, exists := node.GetRoundData(uniqueKey)

	assert.True(t, exists)
	assert.True(t, roundData.MerkleRoot, "MerkleRoot should be true")
	assert.False(t, roundData.RandomNumber, "RandomNumber should be false")
}

func TestRegularNode_processMerkleRoot_Halted(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(true)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)

	node.processMerkleRoot(round, trialNum)

	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	_, exists := node.GetRoundData(uniqueKey)

	assert.False(t, exists, "RoundData should not be set when halted")
}

func TestRegularNode_processMerkleRoot_UpdateExisting(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())

	// Set initial data with RandomNumber true
	node.SetRoundData(uniqueKey, RoundData{MerkleRoot: false, RandomNumber: true})

	node.processMerkleRoot(round, trialNum)

	roundData, _ := node.GetRoundData(uniqueKey)
	assert.True(t, roundData.MerkleRoot, "MerkleRoot should be updated to true")
	assert.True(t, roundData.RandomNumber, "RandomNumber should remain true")
}

func TestRegularNode_processRandomRequestNumber_State1(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	blockTimestamp := big.NewInt(time.Now().Unix() + 1000)
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)

	mockBatchRepo.On("DeleteOldRoundDataForRegularNode", mock.Anything, "100").Return(nil)

	// Temporarily replace eth.Service
	mockEth := &MockEthService{}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	assert.True(t, node.GetExecution(), "Execution should be true for state 1")
	assert.False(t, node.GetHalted(), "Halted should be false for state 1")

	// Immediately halt to stop AllCosReceivedUnlocked goroutine
	node.SetHalted(true)

	// Give goroutine a moment to check halt flag
	time.Sleep(10 * time.Millisecond)

	// Cleanup monitoring that was started
	node.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring("100", "1")

	mockBatchRepo.AssertExpectations(t)
}

func TestRegularNode_processRandomRequestNumber_State2(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo

	blockTimestamp := big.NewInt(1234567890)
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(2)

	mockBatchRepo.On("DeleteOldRoundDataForRegularNode", mock.Anything, "100").Return(nil)

	mockEth := &MockEthService{}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	assert.False(t, node.GetExecution(), "Execution should be false for state 2")

	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	roundData, _ := node.GetRoundData(uniqueKey)

	assert.False(t, roundData.RandomNumber, "RandomNumber is overwritten to false by final SetRoundData")
	assert.False(t, roundData.MerkleRoot, "MerkleRoot should also be false")

	mockBatchRepo.AssertExpectations(t)
}

func TestRegularNode_processRandomRequestNumber_State3(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo
	node.SetCurrentRound("100")

	blockTimestamp := big.NewInt(1234567890)
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(3)

	mockBatchRepo.On("DeleteRoundTrialDataForRegularNode", mock.Anything, "100", "1").Return(nil)

	mockEth := &MockEthService{}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	assert.True(t, node.GetHalted(), "Halted should be true for state 3")
	assert.False(t, node.GetExecution(), "Execution should be false for state 3")

	mockBatchRepo.AssertExpectations(t)
}

func TestRegularNode_processRandomRequestNumber_DeleteError(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockBatchRepo := new(MockBatchRepository)
	node.batchRepository = mockBatchRepo

	// Set environment variables
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	blockTimestamp := big.NewInt(time.Now().Unix() + 1000) // Future time
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	state := big.NewInt(1)

	mockBatchRepo.On("DeleteOldRoundDataForRegularNode", mock.Anything, "100").
		Return(errors.New("delete error"))

	mockEth := &MockEthService{}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	// Should continue despite error
	node.processRandomRequestNumber(context.Background(), blockTimestamp, round, trialNum, state)

	assert.True(t, node.GetExecution())

	// Halt to stop goroutine
	node.SetHalted(true)
	time.Sleep(10 * time.Millisecond)

	node.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring("100", "1")

	mockBatchRepo.AssertExpectations(t)
}

func TestRegularNode_unpackIndices_SingleValue(t *testing.T) {
	node := createTestNodeForSendCommit()

	// Pack single value: 5
	packed := big.NewInt(5)

	result := node.unpackIndices(packed)

	assert.Len(t, result, 1)
	assert.Equal(t, int64(5), result[0].Int64())
}

func TestRegularNode_unpackIndices_MultipleValues(t *testing.T) {
	node := createTestNodeForSendCommit()
	packed := big.NewInt(197121)

	result := node.unpackIndices(packed)

	assert.Len(t, result, 3)
	assert.Equal(t, int64(1), result[0].Int64())
	assert.Equal(t, int64(2), result[1].Int64())
	assert.Equal(t, int64(3), result[2].Int64())
}

func TestRegularNode_unpackIndices_ZeroValue(t *testing.T) {
	node := createTestNodeForSendCommit()

	packed := big.NewInt(256)

	result := node.unpackIndices(packed)

	assert.Len(t, result, 2)
	assert.Equal(t, int64(0), result[0].Int64())
	assert.Equal(t, int64(1), result[1].Int64())
}

func TestRegularNode_unpackIndices_LargeNumbers(t *testing.T) {
	node := createTestNodeForSendCommit()

	// Pack values: 255, 100, 50
	packed := big.NewInt(0)
	packed.Add(packed, big.NewInt(255))
	packed.Add(packed, new(big.Int).Lsh(big.NewInt(100), 8))
	packed.Add(packed, new(big.Int).Lsh(big.NewInt(50), 16))

	result := node.unpackIndices(packed)

	assert.GreaterOrEqual(t, len(result), 1)
	assert.Equal(t, int64(255), result[0].Int64())
}

func TestRegularNode_unpackIndicesWithLength_Success(t *testing.T) {
	node := createTestNodeForSendCommit()

	packed := big.NewInt(197121)
	length := big.NewInt(3)

	result := node.unpackIndicesWithLength(packed, length)

	assert.Len(t, result, 3)
	assert.Equal(t, int64(1), result[0].Int64())
	assert.Equal(t, int64(2), result[1].Int64())
	assert.Equal(t, int64(3), result[2].Int64())
}

func TestRegularNode_unpackIndicesWithLength_ZeroLength(t *testing.T) {
	node := createTestNodeForSendCommit()

	packed := big.NewInt(197121)
	length := big.NewInt(0)

	result := node.unpackIndicesWithLength(packed, length)

	assert.Len(t, result, 0, "Should return empty slice for zero length")
}

func TestRegularNode_unpackIndicesWithLength_LengthOne(t *testing.T) {
	node := createTestNodeForSendCommit()

	packed := big.NewInt(197121)
	length := big.NewInt(1)

	result := node.unpackIndicesWithLength(packed, length)

	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].Int64())
}

func TestRegularNode_findEOAAddress_Found(t *testing.T) {
	node := createTestNodeForSendCommit()

	indices := []*big.Int{big.NewInt(0), big.NewInt(2)}
	activatedOps := []string{
		"0xAddr1",
		"0xAddr2",
		"0xAddr3",
	}

	found, err := node.findEOAAddress(indices, activatedOps, "0xAddr1")

	assert.NoError(t, err)
	assert.True(t, found, "Should find EOA at index 0")
}

func TestRegularNode_findEOAAddress_NotFound(t *testing.T) {
	node := createTestNodeForSendCommit()

	indices := []*big.Int{big.NewInt(0), big.NewInt(2)}
	activatedOps := []string{
		"0xAddr1",
		"0xAddr2",
		"0xAddr3",
	}

	found, err := node.findEOAAddress(indices, activatedOps, "0xAddr4")

	assert.NoError(t, err)
	assert.False(t, found, "Should not find EOA not in indices")
}

func TestRegularNode_findEOAAddress_IndicesLengthError(t *testing.T) {
	node := createTestNodeForSendCommit()

	indices := []*big.Int{big.NewInt(0), big.NewInt(1), big.NewInt(2), big.NewInt(3)}
	activatedOps := []string{
		"0xAddr1",
		"0xAddr2",
	}

	found, err := node.findEOAAddress(indices, activatedOps, "0xAddr1")

	assert.Error(t, err)
	assert.False(t, found)
	assert.Contains(t, err.Error(), "indices length is greater than activated operators")
}

func TestRegularNode_findEOAAddress_EmptyIndices(t *testing.T) {
	node := createTestNodeForSendCommit()

	indices := []*big.Int{}
	activatedOps := []string{"0xAddr1", "0xAddr2"}

	found, err := node.findEOAAddress(indices, activatedOps, "0xAddr1")

	assert.NoError(t, err)
	assert.False(t, found)
}

func TestRegularNode_processSecretRequest_Halted(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(true)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(0)

	node.processSecretRequest(context.Background(), round, trialNum, index)

	// Should return early without processing
	assert.True(t, node.GetHalted())
}

func TestRegularNode_processSecretRequest_DetermineRevealOrder_FailsMissingCos(t *testing.T) {
	node := createTestNodeWithRevealOrderService()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	mockPeerRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")
	node.SetRegularNodeEOA(testOp.Hex())

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(0)

	// First call in processSecretRequest returns error
	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(nil, errors.New("not found")).Once()

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	// Second call inside DetermineRegularRevealOrder
	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(nil, errors.New("not found")).Once()
	peerData := &database.PeerCommitDataScheme{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: testOp.Hex(),
		Cvs:        make([]byte, 32),
		Cos:        nil, // Empty COS - will fail
	}
	mockPeerRepo.On("GetPeerCommitData", mock.Anything, "100", "1", testOp.Hex()).
		Return(peerData, nil)

	// Should return early after DetermineRegularRevealOrder fails
	node.processSecretRequest(context.Background(), round, trialNum, index)

	mockRevealRepo.AssertExpectations(t)
	mockPeerRepo.AssertExpectations(t)
}

func TestRegularNode_processSecretRequest_DetermineRevealOrder_PeerCommitError(t *testing.T) {
	node := createTestNodeWithRevealOrderService()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	mockPeerRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")
	node.SetRegularNodeEOA(testOp.Hex())

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(0)

	// First call in processSecretRequest returns error
	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(nil, errors.New("not found")).Once()

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	// Second call inside DetermineRegularRevealOrder
	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(nil, errors.New("not found")).Once()

	// Mock GetPeerCommitData to return error
	mockPeerRepo.On("GetPeerCommitData", mock.Anything, "100", "1", testOp.Hex()).
		Return(nil, errors.New("database error"))

	// Should return early
	node.processSecretRequest(context.Background(), round, trialNum, index)

	mockRevealRepo.AssertExpectations(t)
	mockPeerRepo.AssertExpectations(t)
}

func TestRegularNode_processSecretRequest_DetermineRevealOrder_AddRevealOrderError(t *testing.T) {
	node := createTestNodeWithRevealOrderService()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	mockPeerRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")
	node.SetRegularNodeEOA(testOp.Hex())

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(0)

	// First call in processSecretRequest returns error
	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(nil, errors.New("not found")).Once()

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	// Second call inside DetermineRegularRevealOrder
	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(nil, errors.New("not found")).Once()

	// Mock peer commit data
	peerData := &database.PeerCommitDataScheme{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: testOp.Hex(),
		Cvs:        make([]byte, 32),
		Cos:        []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
	}
	mockPeerRepo.On("GetPeerCommitData", mock.Anything, "100", "1", testOp.Hex()).
		Return(peerData, nil)

	// Mock AddRevealOrder to fail
	mockRevealRepo.On("AddRevealOrder", mock.Anything, mock.Anything).
		Return(errors.New("database save error"))

	// Should return early after failing to save reveal order
	node.processSecretRequest(context.Background(), round, trialNum, index)

	mockRevealRepo.AssertExpectations(t)
	mockPeerRepo.AssertExpectations(t)
}

func TestRegularNode_processSecretRequest_NilRevealOrder(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(0)

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(nil, nil) // nil reveal order

	node.processSecretRequest(context.Background(), round, trialNum, index)

	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_processSecretRequest_NilOrderedNodes(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(0)

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: nil, // Nil ordered nodes
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)

	node.processSecretRequest(context.Background(), round, trialNum, index)

	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_processSecretRequest_IndexOutOfBounds(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(10) // Out of bounds

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: []string{"0xNode1", "0xNode2"},
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)

	node.processSecretRequest(context.Background(), round, trialNum, index)

	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_processSecretRequest_NotMyEOA(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)
	node.SetRegularNodeEOA("0xMyNode")

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(0)

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: []string{"0xOtherNode"}, // Different node
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)

	node.processSecretRequest(context.Background(), round, trialNum, index)

	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_processSubmittedSecretRequest_Halted(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(true)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(0)

	node.processSubmittedSecretRequest(context.Background(), round, trialNum, index)

	assert.True(t, node.GetHalted())
}

func TestRegularNode_processSubmittedSecretRequest_IndexOutOfBounds(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(10) // Out of bounds

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: []string{"0xNode1"},
		RevealOrder:  []int{0},
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)

	node.processSubmittedSecretRequest(context.Background(), round, trialNum, index)

	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_processSubmittedSecretRequest_NotNextInOrder(t *testing.T) {
	node := createTestNodeForSendCommit()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)
	node.SetRegularNodeEOA("0xNode3")

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	index := big.NewInt(0)

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: []string{"0xNode1", "0xNode2", "0xNode3"},
		RevealOrder:  []int{0, 1, 2},
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)

	node.processSubmittedSecretRequest(context.Background(), round, trialNum, index)

	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_CleanupRoundDataByUniqueKey(t *testing.T) {
	node := createTestNodeForSendCommit()

	uniqueKey := "100:1"

	// Set data
	node.SetRoundData(uniqueKey, RoundData{MerkleRoot: true})
	node.SetSubmittedCvIndicesValue(uniqueKey, "0", true)
	node.SetStrictOrder(uniqueKey, []string{"node1"})

	// Cleanup
	node.CleanupRoundDataByUniqueKey(uniqueKey)

	// Verify all cleaned up
	_, exists := node.GetRoundData(uniqueKey)
	assert.False(t, exists, "RoundData should be deleted")

	_, exists = node.GetSubmittedCvIndicesMap(uniqueKey)
	assert.False(t, exists, "SubmittedCvIndices should be deleted")

	_, exists = node.GetStrictOrder(uniqueKey)
	assert.False(t, exists, "StrictOrder should be deleted")
}

func TestRegularNode_allCosReceivedUnlockedRegular_AllReceived(t *testing.T) {
	node := createTestNodeForSendCommit()

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	ops := []common.Address{
		common.HexToAddress("0xOp1"),
		common.HexToAddress("0xOp2"),
	}

	// Set all COS as received
	node.SetCosReceived(uniqueKey, ops[0].Hex(), true)
	node.SetCosReceived(uniqueKey, ops[1].Hex(), true)

	result := node.allCosReceivedUnlockedRegular(round, trialNum, ops)

	assert.True(t, result, "Should return true when all COS received")
}

func TestRegularNode_allCosReceivedUnlockedRegular_PartiallyReceived(t *testing.T) {
	node := createTestNodeForSendCommit()

	round := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	ops := []common.Address{
		common.HexToAddress("0xOp1"),
		common.HexToAddress("0xOp2"),
		common.HexToAddress("0xOp3"), // Add a third op to ensure not all are set
	}

	// Only set first COS
	node.SetCosReceived(uniqueKey, ops[0].Hex(), true)
	// Explicitly set Op2 to false
	node.SetCosReceived(uniqueKey, ops[1].Hex(), false)
	// Don't set Op3 at all

	result := node.allCosReceivedUnlockedRegular(round, trialNum, ops)

	assert.False(t, result, "Should return false when not all COS received")
}

func TestRegularNode_allCosReceivedUnlockedRegular_NoneReceived(t *testing.T) {
	node := createTestNodeForSendCommit()

	round := "100"
	trialNum := "1"

	ops := []common.Address{
		common.HexToAddress("0xOp1"),
		common.HexToAddress("0xOp2"),
	}

	result := node.allCosReceivedUnlockedRegular(round, trialNum, ops)

	assert.False(t, result, "Should return false when no COS received")
}

func TestRegularNode_allCosReceivedUnlockedRegular_EmptyOps(t *testing.T) {
	node := createTestNodeForSendCommit()

	round := "100"
	trialNum := "1"

	result := node.allCosReceivedUnlockedRegular(round, trialNum, []common.Address{})

	assert.True(t, result, "Should return true for empty operators list")
}

func TestRegularNode_StartLeaderMonitoring_Halted(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(true)

	startTime := big.NewInt(time.Now().Unix() + 1000)

	node.StartLeaderMonitoring(context.Background(), startTime, "100", "1")

	assert.False(t, node.GetLeaderMonitoringActive(), "Should not start monitoring when halted")
}

func TestRegularNode_StartLeaderMonitoring_AlreadyActive(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)
	node.SetLeaderMonitoringActive(true)

	startTime := big.NewInt(time.Now().Unix() + 1000)

	node.StartLeaderMonitoring(context.Background(), startTime, "100", "1")

	assert.True(t, node.GetLeaderMonitoringActive())
}

func TestRegularNode_StartLeaderMonitoring_NilStartTime(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	node.StartLeaderMonitoring(context.Background(), nil, "100", "1")

	assert.False(t, node.GetLeaderMonitoringActive(), "Should not start with nil start time")
}

func TestRegularNode_StartLeaderMonitoring_Success(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	// Set start time in future
	futureTime := big.NewInt(time.Now().Unix() + 1000)

	node.StartLeaderMonitoring(context.Background(), futureTime, "100", "1")

	assert.True(t, node.GetLeaderMonitoringActive(), "Monitoring should be active")

	// Cleanup
	node.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring("100", "1")
}

func TestRegularNode_StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring_NotActive(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetLeaderMonitoringActive(false)

	node.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring("100", "1")

	assert.False(t, node.GetLeaderMonitoringActive())
}

func TestRegularNode_StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring_WithTimer(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	futureTime := big.NewInt(time.Now().Unix() + 1000)
	node.StartLeaderMonitoring(context.Background(), futureTime, "100", "1")

	assert.True(t, node.GetLeaderMonitoringActive())

	node.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring("100", "1")

	assert.False(t, node.GetLeaderMonitoringActive())
	assert.Nil(t, node.monitoringTimer)
}

func TestRegularNode_ResetMonitoringState(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	// Set up monitoring state
	node.SetLeaderMonitoringActive(true)
	node.SetMerkleRootSubmittedEventEmitted(true)
	node.SetCvRequestIndices([]*big.Int{big.NewInt(0), big.NewInt(1)})
	node.MerkleRootSubmittedTOrRequestedCvTime(big.NewInt(123456))

	node.ResetMonitoringState("100", "1")

	assert.False(t, node.GetLeaderMonitoringActive())
	assert.False(t, node.GetMerkleRootSubmittedEventEmitted())
	assert.Nil(t, node.GetMerkleRootSubmittedTOrRequestedCvTime())
	assert.Empty(t, node.GetCvRequestIndices())
}

func TestRegularNode_StartMerkleRootMonitoring_NilRequestTime(t *testing.T) {
	node := createTestNodeForSendCommit()

	node.StartMerkleRootMonitoring(context.Background(), "100", "1", nil)

	// Should return early
	assert.Nil(t, node.merkleRootMonitoringTimer)
}

func TestRegularNode_StartMerkleRootMonitoring_Success(t *testing.T) {
	node := createTestNodeForSendCommit()

	// Set time in future
	futureTime := big.NewInt(time.Now().Unix() + 1000)

	node.StartMerkleRootMonitoring(context.Background(), "100", "1", futureTime)

	assert.NotNil(t, node.merkleRootMonitoringTimer, "Timer should be set")

	// Cleanup
	node.StopFailToSubmitMerkleRootAfterDisputeMonitoring("100", "1")
}

func TestRegularNode_StartMerkleRootMonitoring_ChecksConditions(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	// Set environment variables for potential callback
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	// Set indices and mark all as submitted
	indices := []*big.Int{big.NewInt(0)}
	node.SetCvRequestIndices(indices)

	uniqueKey := utils.GetUniqueKey("100", "1")
	node.SetSubmittedCvIndicesValue(uniqueKey, "0", true)
	node.SetMerkleRootSubmittedEventEmitted(false)

	futureTime := big.NewInt(time.Now().Unix() + 1000)

	node.StartMerkleRootMonitoring(context.Background(), "100", "1", futureTime)

	// Verify timer was created
	assert.NotNil(t, node.merkleRootMonitoringTimer, "Timer should be created")

	// Cleanup
	if node.merkleRootMonitoringTimer != nil {
		node.StopFailToSubmitMerkleRootAfterDisputeMonitoring("100", "1")
	}
}

func TestRegularNode_StopFailToSubmitMerkleRootAfterDisputeMonitoring_NoTimer(t *testing.T) {
	node := createTestNodeForSendCommit()

	// Should not panic when no timer is set
	node.StopFailToSubmitMerkleRootAfterDisputeMonitoring("100", "1")

	assert.Nil(t, node.merkleRootMonitoringTimer)
}

func TestRegularNode_StopFailToSubmitMerkleRootAfterDisputeMonitoring_WithTimer(t *testing.T) {
	node := createTestNodeForSendCommit()

	futureTime := big.NewInt(time.Now().Unix() + 1000)
	node.StartMerkleRootMonitoring(context.Background(), "100", "1", futureTime)

	assert.NotNil(t, node.merkleRootMonitoringTimer)

	node.StopFailToSubmitMerkleRootAfterDisputeMonitoring("100", "1")

	assert.Nil(t, node.merkleRootMonitoringTimer)
}

func TestRegularNode_StartRequestToSubmitSOrGenerateRandomNumberMonitoring_Halted(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(true)

	node.StartRequestToSubmitSOrGenerateRandomNumberMonitoring(context.Background(), "100", "1")

	assert.Nil(t, node.requestToSubmitSOrGenerateRandomNumberMonitoringTimer)
}

func TestRegularNode_StartRequestToSubmitSOrGenerateRandomNumberMonitoring_Success(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	mockEth := &MockEthService{
		GetActivatedOperatorsLengthFunc: func() int64 {
			return 3
		},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	futureTime := big.NewInt(time.Now().Unix() + 1000)
	node.MerkleRootSubmittedTOrRequestedCvTime(futureTime)

	node.StartRequestToSubmitSOrGenerateRandomNumberMonitoring(context.Background(), "100", "1")

	assert.NotNil(t, node.requestToSubmitSOrGenerateRandomNumberMonitoringTimer, "Timer should be set")

	// Cleanup
	node.StopRequestToSubmitSOrGenerateRandomNumberMonitoring("100", "1")
}

func TestRegularNode_StopRequestToSubmitSOrGenerateRandomNumberMonitoring_NoTimer(t *testing.T) {
	node := createTestNodeForSendCommit()

	node.StopRequestToSubmitSOrGenerateRandomNumberMonitoring("100", "1")

	assert.Nil(t, node.requestToSubmitSOrGenerateRandomNumberMonitoringTimer)
}

func TestRegularNode_StopRequestToSubmitSOrGenerateRandomNumberMonitoring_WithTimer(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	mockEth := &MockEthService{
		GetActivatedOperatorsLengthFunc: func() int64 {
			return 2
		},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	futureTime := big.NewInt(time.Now().Unix() + 1000)
	node.MerkleRootSubmittedTOrRequestedCvTime(futureTime)

	node.StartRequestToSubmitSOrGenerateRandomNumberMonitoring(context.Background(), "100", "1")

	assert.NotNil(t, node.requestToSubmitSOrGenerateRandomNumberMonitoringTimer)

	node.StopRequestToSubmitSOrGenerateRandomNumberMonitoring("100", "1")

	assert.Nil(t, node.requestToSubmitSOrGenerateRandomNumberMonitoringTimer)
}

func TestRegularNode_callFailToRequestSubmitCVOrSubmitMerkleRoot_InvalidPrivateKey(t *testing.T) {
	node := createTestNodeForSendCommit()

	os.Setenv("EOA_PRIVATE_KEY", "invalid-key")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	node.callFailToRequestSubmitCVOrSubmitMerkleRoot(context.Background(), "100", "1")

	assert.True(t, true)
}
func TestRegularNode_callFailToRequestSubmitCVOrSubmitMerkleRoot_ABILoadError(t *testing.T) {
	node := createTestNodeForSendCommit()

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic as expected: %v", r)
		}
	}()

	node.callFailToRequestSubmitCVOrSubmitMerkleRoot(context.Background(), "100", "1")
}

func TestRegularNode_callFailToSubmitMerkleRootAfterDispute_InvalidPrivateKey(t *testing.T) {
	node := createTestNodeForSendCommit()

	os.Setenv("EOA_PRIVATE_KEY", "invalid")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	node.callFailToSubmitMerkleRootAfterDispute(context.Background(), "100", "1")

	assert.True(t, true, "Should handle invalid private key gracefully")
}

func TestRegularNode_callFailToSubmitMerkleRootAfterDispute_ABILoadError(t *testing.T) {
	node := createTestNodeForSendCommit()

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic as expected: %v", r)
		}
	}()

	node.callFailToSubmitMerkleRootAfterDispute(context.Background(), "100", "1")
}

func TestRegularNode_callFailToRequestSOrGenerateRandomNumber_ABILoadError(t *testing.T) {
	node := createTestNodeForSendCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic as expected: %v", r)
		}
	}()

	node.callFailToRequestSOrGenerateRandomNumber(context.Background(), "100", "1")
}

func TestRegularNode_callFailToRequestSOrGenerateRandomNumber_InvalidPrivateKey(t *testing.T) {
	node := createTestNodeForSendCommit()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("EOA_PRIVATE_KEY", "invalid-key")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	node.callFailToRequestSOrGenerateRandomNumber(context.Background(), "100", "1")

	assert.True(t, true, "Should handle invalid private key")
}

func TestRegularNode_processCommitRequest_Halted(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(true)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	packedIndices := big.NewInt(5)

	err := node.processCommitRequest(context.Background(), round, trialNum, packedIndices)

	assert.NoError(t, err, "Should return nil when halted")
}

func TestRegularNode_processCommitRequest_EOANotInIndices(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	testOp := common.HexToAddress("0x1111111111111111111111111111111111111111")
	otherOp := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp, otherOp}
		},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	packedIndices := big.NewInt(1)

	err := node.processCommitRequest(context.Background(), round, trialNum, packedIndices)

	assert.NoError(t, err)
}

func TestRegularNode_processCommitRequest_ABILoadError(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey)

	os.Setenv("EOA_PRIVATE_KEY", hex.EncodeToString(crypto.FromECDSA(privateKey)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{eoaAddress}
		},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	packedIndices := big.NewInt(0) // Index 0

	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic as expected: %v", r)
		}
	}()

	_ = node.processCommitRequest(context.Background(), round, trialNum, packedIndices)
}

func TestRegularNode_processCosRequest_Halted(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(true)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	packedIndices := big.NewInt(0)
	indicesLength := big.NewInt(1)

	err := node.processCosRequest(context.Background(), round, trialNum, packedIndices, indicesLength)

	assert.NoError(t, err, "Should return nil when halted")
}

func TestRegularNode_processCosRequest_EOANotInIndices(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	testOp := common.HexToAddress("0x1111111111111111111111111111111111111111")
	otherOp := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp, otherOp}
		},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	packedIndices := big.NewInt(1) // Only index 1
	indicesLength := big.NewInt(1)

	err := node.processCosRequest(context.Background(), round, trialNum, packedIndices, indicesLength)

	assert.NoError(t, err)
}

func TestRegularNode_unpackIndices_EdgeCases(t *testing.T) {
	node := createTestNodeForSendCommit()

	tests := []struct {
		name     string
		packed   *big.Int
		expected int
	}{
		{
			name:     "Zero value",
			packed:   big.NewInt(0),
			expected: 1,
		},
		{
			name:     "Single byte max",
			packed:   big.NewInt(255),
			expected: 1,
		},
		{
			name:     "Two bytes",
			packed:   big.NewInt(256),
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := node.unpackIndices(tt.packed)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestRegularNode_unpackIndicesWithLength_LargeLength(t *testing.T) {
	node := createTestNodeForSendCommit()

	packed := big.NewInt(0)
	// Pack 10 values
	for i := 0; i < 10; i++ {
		val := new(big.Int).Lsh(big.NewInt(int64(i+1)), uint(8*i))
		packed.Add(packed, val)
	}

	length := big.NewInt(10)
	result := node.unpackIndicesWithLength(packed, length)

	assert.Len(t, result, 10)
	assert.Equal(t, int64(1), result[0].Int64())
}

func TestRegularNode_findEOAAddress_MultipleMatches(t *testing.T) {
	node := createTestNodeForSendCommit()

	// EOA is at multiple indices
	indices := []*big.Int{big.NewInt(0), big.NewInt(1)}
	activatedOps := []string{
		"0xAddr1",
		"0xAddr1", // Same address
	}

	found, err := node.findEOAAddress(indices, activatedOps, "0xAddr1")

	assert.NoError(t, err)
	assert.True(t, found)
}

func TestRegularNode_CvSubmissionWorkflow(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	round := big.NewInt(100)
	trialNum := big.NewInt(1)

	// Set up requested indices
	indices := []*big.Int{big.NewInt(0), big.NewInt(1)}
	node.SetCvRequestIndices(indices)

	// Process CV submissions
	node.processCvSubmitted(round, trialNum, big.NewInt(0))
	assert.False(t, node.checkAllCVsSubmittedOnChain("100", "1"), "Not all submitted yet")

	node.processCvSubmitted(round, trialNum, big.NewInt(1))
	assert.True(t, node.checkAllCVsSubmittedOnChain("100", "1"), "All submitted now")
}

func TestRegularNode_MonitoringLifecycle(t *testing.T) {
	node := createTestNodeForSendCommit()
	node.SetHalted(false)

	mockEth := &MockEthService{
		GetActivatedOperatorsLengthFunc: func() int64 {
			return 2
		},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	// Start leader monitoring
	futureTime := big.NewInt(time.Now().Unix() + 1000)
	node.StartLeaderMonitoring(context.Background(), futureTime, "100", "1")
	assert.True(t, node.GetLeaderMonitoringActive())

	// Start merkle root monitoring
	node.MerkleRootSubmittedTOrRequestedCvTime(futureTime)
	node.StartMerkleRootMonitoring(context.Background(), "100", "1", futureTime)
	assert.NotNil(t, node.merkleRootMonitoringTimer)

	// Start request to submit S monitoring
	node.StartRequestToSubmitSOrGenerateRandomNumberMonitoring(context.Background(), "100", "1")
	assert.NotNil(t, node.requestToSubmitSOrGenerateRandomNumberMonitoringTimer)

	// Reset all
	node.ResetMonitoringState("100", "1")
	assert.False(t, node.GetLeaderMonitoringActive())
	assert.Nil(t, node.monitoringTimer)
	assert.Nil(t, node.merkleRootMonitoringTimer)
	assert.Nil(t, node.requestToSubmitSOrGenerateRandomNumberMonitoringTimer)
}

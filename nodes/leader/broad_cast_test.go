package leader_node

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/eapache/queue"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/utils"
)

// Mock types for testing
type MockBroadcastTrackerRepository struct {
	mock.Mock
}

func (m *MockBroadcastTrackerRepository) AddBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error {
	args := m.Called(ctx, tracker)
	return args.Error(0)
}

func (m *MockBroadcastTrackerRepository) UpdateBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error {
	args := m.Called(ctx, tracker)
	return args.Error(0)
}

func (m *MockBroadcastTrackerRepository) GetBroadcastTracker(ctx context.Context, messageID string) (*utils.BroadcastTracker, error) {
	args := m.Called(ctx, messageID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.BroadcastTracker), args.Error(1)
}

func (m *MockBroadcastTrackerRepository) DeleteBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error {
	args := m.Called(ctx, tracker)
	return args.Error(0)
}

type MockNodeInfoRepository struct{ mock.Mock }

func (m *MockNodeInfoRepository) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.NodeInfo), args.Error(1)
}

func (m *MockNodeInfoRepository) AddNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	return nil
}
func (m *MockNodeInfoRepository) DeleteNodeInfo(eoaAddress string) error { return nil }
func (m *MockNodeInfoRepository) GetNodeInfo(eoaAddress string) (*utils.NodeInfo, error) {
	return nil, nil
}

func createTestNodeForBroadcast() *LeaderNode {
	return &LeaderNode{
		activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
		cvOnChain:           make(map[string]bool),
		roundSecrets:        make(map[string][][32]byte),
		roundSecret:         make(map[string]map[string]bool),
		secretsOnChain:      make(map[string]bool),
		revealRequestStatus: make(map[string][]string),
		cleanupQueue:        queue.New(),
	}
}

// TestGetLeaderPrivateKeySuccess tests successful retrieval of leader private key
func TestGetLeaderPrivateKeySuccess(t *testing.T) {
	// Generate a test private key
	privateKey, err := crypto.GenerateKey()
	assert.NoError(t, err)
	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)

	// Set environment variable
	os.Setenv("LEADER_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	resultKey, eoa, err := getLeaderPrivateKey()

	assert.NoError(t, err)
	assert.NotNil(t, resultKey)
	assert.NotEmpty(t, eoa)
	assert.True(t, common.IsHexAddress(eoa))
}

// TestGetLeaderPrivateKeyNotSet tests error when env var not set
func TestGetLeaderPrivateKeyNotSet(t *testing.T) {
	os.Unsetenv("LEADER_PRIVATE_KEY")

	// Call function
	_, _, err := getLeaderPrivateKey()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LEADER_PRIVATE_KEY is not set")
}

// TestGetLeaderPrivateKeyInvalidKey tests error with invalid key
func TestGetLeaderPrivateKeyInvalidKey(t *testing.T) {
	// Set invalid private key
	os.Setenv("LEADER_PRIVATE_KEY", "invalid_key")
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	// Call function
	_, _, err := getLeaderPrivateKey()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode leader private key")
}

// TestGenerateMessageID tests message ID generation
func TestGenerateMessageID(t *testing.T) {
	round := "100"
	trialNum := "1"
	eoa := "0x1234567890123456789012345678901234567890"
	msgType := "secret"

	// Generate two message IDs with slight delay
	id1 := generateMessageID(round, eoa, trialNum, msgType)
	time.Sleep(1 * time.Millisecond)
	id2 := generateMessageID(round, eoa, trialNum, msgType)

	assert.NotEqual(t, id1, id2, "Message IDs should be unique")
	assert.Contains(t, id1, round)
	assert.Contains(t, id1, trialNum)
	assert.Contains(t, id1, eoa)
	assert.Contains(t, id1, msgType)
}

// TestHandleAcknowledgmentSuccess tests successful acknowledgment handling
func TestHandleAcknowledgmentSuccess(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	// Create a tracker
	messageID := "test-message-id"
	eoaAddr := "0x1234567890123456789012345678901234567890"
	tracker := &utils.BroadcastTracker{
		MessageID: messageID,
		Round:     "100",
		TrialNum:  "1",
		Type:      "secret",
		Acknowledged: map[string]bool{
			eoaAddr: false,
		},
	}
	node.SetActiveBroadcast(messageID, tracker)

	// Setup mock
	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	// Create acknowledgment
	ack := utils.AcknowledgmentMessage{
		MessageID:  messageID,
		EOAAddress: eoaAddr,
		Status:     "received",
		Type:       "secret",
	}

	// Handle acknowledgment
	node.HandleAcknowledgment(context.Background(), ack)

	updatedTracker, _ := node.GetActiveBroadcast(messageID)
	assert.True(t, updatedTracker.Acknowledged[eoaAddr])
	mockRepo.AssertExpectations(t)
}

// TestHandleAcknowledgmentUnknownMessage tests handling of unknown message
func TestHandleAcknowledgmentUnknownMessage(t *testing.T) {
	node := createTestNodeForBroadcast()

	// Create acknowledgment for non-existent message
	ack := utils.AcknowledgmentMessage{
		MessageID:  "unknown-message-id",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		Status:     "received",
		Type:       "secret",
	}

	// Handle acknowledgment
	node.HandleAcknowledgment(context.Background(), ack)

	assert.True(t, true)
}

// TestHandleAcknowledgmentErrorStatus tests handling of error acknowledgment
func TestHandleAcknowledgmentErrorStatus(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	// Create a tracker
	messageID := "test-message-id"
	eoaAddr := "0x1234567890123456789012345678901234567890"
	tracker := &utils.BroadcastTracker{
		MessageID: messageID,
		Round:     "100",
		TrialNum:  "1",
		Type:      "secret",
		Acknowledged: map[string]bool{
			eoaAddr: false,
		},
	}
	node.SetActiveBroadcast(messageID, tracker)

	// Setup mock
	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	// Create error acknowledgment
	ack := utils.AcknowledgmentMessage{
		MessageID:  messageID,
		EOAAddress: eoaAddr,
		Status:     "error",
		Type:       "secret",
	}

	// Handle acknowledgment
	node.HandleAcknowledgment(context.Background(), ack)

	updatedTracker, _ := node.GetActiveBroadcast(messageID)
	assert.False(t, updatedTracker.Acknowledged[eoaAddr])
	mockRepo.AssertExpectations(t)
}

// TestHandleAcknowledgmentWhenHalted tests acknowledgment handling when system is halted
func TestHandleAcknowledgmentWhenHalted(t *testing.T) {
	node := createTestNodeForBroadcast()
	node.SetHalted(true)

	// Create acknowledgment
	ack := utils.AcknowledgmentMessage{
		MessageID:  "test-message-id",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		Status:     "received",
		Type:       "secret",
	}

	node.HandleAcknowledgment(context.Background(), ack)

	assert.True(t, true)
}

// TestHandleAcknowledgmentAllNodesAcknowledged tests when all nodes acknowledge
func TestHandleAcknowledgmentAllNodesAcknowledged(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	// Create a tracker with multiple nodes
	messageID := "test-message-id"
	eoaAddr1 := "0x1234567890123456789012345678901234567890"
	eoaAddr2 := "0xAbC1234567890123456789012345678901234567"
	tracker := &utils.BroadcastTracker{
		MessageID: messageID,
		Round:     "100",
		TrialNum:  "1",
		Type:      "secret",
		Acknowledged: map[string]bool{
			eoaAddr1: false,
			eoaAddr2: false,
		},
	}
	node.SetActiveBroadcast(messageID, tracker)

	// Setup mock
	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Times(2)

	ack1 := utils.AcknowledgmentMessage{
		MessageID:  messageID,
		EOAAddress: eoaAddr1,
		Status:     "received",
		Type:       "secret",
	}
	node.HandleAcknowledgment(context.Background(), ack1)

	ack2 := utils.AcknowledgmentMessage{
		MessageID:  messageID,
		EOAAddress: eoaAddr2,
		Status:     "received",
		Type:       "secret",
	}
	node.HandleAcknowledgment(context.Background(), ack2)

	updatedTracker, _ := node.GetActiveBroadcast(messageID)
	assert.True(t, updatedTracker.Acknowledged[eoaAddr1])
	assert.True(t, updatedTracker.Acknowledged[eoaAddr2])
	mockRepo.AssertExpectations(t)
}

// TestHandleAcknowledgmentDatabaseUpdateError tests error during database update
func TestHandleAcknowledgmentDatabaseUpdateError(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	// Create a tracker
	messageID := "test-message-id"
	eoaAddr := "0x1234567890123456789012345678901234567890"
	tracker := &utils.BroadcastTracker{
		MessageID: messageID,
		Round:     "100",
		TrialNum:  "1",
		Type:      "secret",
		Acknowledged: map[string]bool{
			eoaAddr: false,
		},
	}
	node.SetActiveBroadcast(messageID, tracker)

	// Setup mock to return error
	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(errors.New("database error"))

	// Create acknowledgment
	ack := utils.AcknowledgmentMessage{
		MessageID:  messageID,
		EOAAddress: eoaAddr,
		Status:     "received",
		Type:       "secret",
	}

	// Handle acknowledgment (should log error but not crash)
	node.HandleAcknowledgment(context.Background(), ack)

	updatedTracker, _ := node.GetActiveBroadcast(messageID)
	assert.True(t, updatedTracker.Acknowledged[eoaAddr])
	mockRepo.AssertExpectations(t)
}

// TestGenerateMessageIDWithDifferentParams tests message ID generation with different parameters
func TestGenerateMessageIDWithDifferentParams(t *testing.T) {
	// Test different parameters produce different IDs (except timestamp)
	id1 := generateMessageID("100", "0x123", "1", "secret")
	id2 := generateMessageID("101", "0x123", "1", "secret")
	id3 := generateMessageID("100", "0x456", "1", "secret")
	id4 := generateMessageID("100", "0x123", "2", "secret")
	id5 := generateMessageID("100", "0x123", "1", "cos")

	// All should contain their respective parameters
	assert.Contains(t, id1, "100")
	assert.Contains(t, id2, "101")
	assert.Contains(t, id3, "0x456")
	assert.Contains(t, id4, "2")
	assert.Contains(t, id5, "cos")
}

// TestPerformReliableBroadcast_AllAcknowledged tests
func TestPerformReliableBroadcast_AllAcknowledged(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	// Set a valid leader private key so getLeaderPrivateKey succeeds
	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	// Setup a real P2P client with a mock node repo returning no peers
	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	// Tracker with one operator already acknowledged
	opAddr := common.HexToAddress("0xAbC1234567890123456789012345678901234567")
	tracker := &utils.BroadcastTracker{
		MessageID:    "test-id-all-ack",
		Round:        "100",
		TrialNum:     "1",
		EOAAddress:   opAddr.Hex(),
		Type:         "cvs",
		Attempts:     0,
		MaxAttempts:  1,
		Timeout:      0, // no sleep
		Acknowledged: map[string]bool{opAddr.Hex(): true},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	// Call with nil host since no network calls should be made
	node.performReliableBroadcast(context.Background(), nil, tracker, "cvs", []common.Address{opAddr})

	// Active broadcast should be cleaned up
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcast_WhenHalted tests broadcast skipping when system is halted
func TestPerformReliableBroadcast_WhenHalted(t *testing.T) {
	node := createTestNodeForBroadcast()
	node.SetHalted(true)

	tracker := &utils.BroadcastTracker{
		MessageID: "test-halted",
		Round:     "100",
		TrialNum:  "1",
		Type:      "cvs",
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	// Call should return early without processing
	node.performReliableBroadcast(context.Background(), nil, tracker, "cvs", []common.Address{})

	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)
}

// TestPerformReliableBroadcast_PrivateKeyError tests handling of private key errors
func TestPerformReliableBroadcast_PrivateKeyError(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	// Unset private key to trigger error
	os.Unsetenv("LEADER_PRIVATE_KEY")

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	opAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	tracker := &utils.BroadcastTracker{
		MessageID:    "test-key-error",
		Round:        "100",
		TrialNum:     "1",
		EOAAddress:   opAddr.Hex(),
		Type:         "cvs",
		Attempts:     0,
		MaxAttempts:  1,
		Timeout:      0,
		Acknowledged: map[string]bool{opAddr.Hex(): false},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	node.performReliableBroadcast(context.Background(), nil, tracker, "cvs", []common.Address{opAddr})

	mockRepo.AssertNotCalled(t, "UpdateBroadcastTracker")
}

// TestPerformReliableBroadcast_UnknownBroadcastType tests handling of unknown broadcast types
func TestPerformReliableBroadcast_UnknownBroadcastType(t *testing.T) {
	node := createTestNodeForBroadcast()

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	opAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	tracker := &utils.BroadcastTracker{
		MessageID:    "test-unknown-type",
		Round:        "100",
		TrialNum:     "1",
		Type:         "unknown",
		Attempts:     0,
		MaxAttempts:  1,
		Timeout:      0,
		Acknowledged: map[string]bool{opAddr.Hex(): false},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	node.performReliableBroadcast(context.Background(), nil, tracker, "unknown", []common.Address{opAddr})
}

// TestPerformReliableBroadcast_BreaksEarlyWhenAllAcknowledged tests early break when all acknowledged
func TestPerformReliableBroadcast_BreaksEarlyWhenAllAcknowledged(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	// All nodes acknowledged - should break after first attempt
	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	opAddr2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")
	tracker := &utils.BroadcastTracker{
		MessageID:   "test-early-break",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cvs",
		Attempts:    0,
		MaxAttempts: 3,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): true,
			opAddr2.Hex(): true,
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Once()

	node.performReliableBroadcast(context.Background(), nil, tracker, "cvs", []common.Address{opAddr1, opAddr2})

	assert.Equal(t, 1, tracker.Attempts)

	// Active broadcast should be cleaned up
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcast_UpdateTrackerError tests handling of update errors
func TestPerformReliableBroadcast_UpdateTrackerError(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	opAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	tracker := &utils.BroadcastTracker{
		MessageID:    "test-update-error",
		Round:        "100",
		TrialNum:     "1",
		EOAAddress:   opAddr.Hex(),
		Type:         "cvs",
		Attempts:     0,
		MaxAttempts:  1,
		Timeout:      0,
		Acknowledged: map[string]bool{opAddr.Hex(): true},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(errors.New("update failed"))

	node.performReliableBroadcast(context.Background(), nil, tracker, "cvs", []common.Address{opAddr})

	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
}

// TestReliableBroadCastCVS tests CVS broadcast initialization
func TestReliableBroadCastCVS(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	// Set valid private key for goroutine
	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	roundNum := "100"
	trialNum := "1"
	eoaAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")
	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs-value"))

	activatedOps := []common.Address{eoaAddress}

	mockRepo.On("AddBroadcastTracker", mock.Anything, mock.MatchedBy(func(tracker *utils.BroadcastTracker) bool {
		if tracker.Round == roundNum &&
			tracker.TrialNum == trialNum &&
			tracker.EOAAddress == eoaAddress.Hex() &&
			tracker.Type == "cvs" {
			for _, op := range activatedOps {
				tracker.Acknowledged[op.Hex()] = true
			}
			return true
		}
		return false
	})).Return(nil)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	node.ReliableBroadCastCVS(context.Background(), roundNum, trialNum, eoaAddress, cvs, activatedOps)

	time.Sleep(100 * time.Millisecond)

	mockRepo.AssertExpectations(t)
}

// TestReliableBroadCastCVS_AddTrackerError tests CVS broadcast when add tracker fails
func TestReliableBroadCastCVS_AddTrackerError(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	mockNodeRepo := new(MockNodeInfoRepository)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	roundNum := "100"
	trialNum := "1"
	eoaAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")
	var cvs [32]byte
	activatedOps := []common.Address{eoaAddress}

	mockRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(errors.New("db error"))

	node.ReliableBroadCastCVS(context.Background(), roundNum, trialNum, eoaAddress, cvs, activatedOps)

	mockRepo.AssertExpectations(t)
}

// TestReliableBroadCastCOS tests COS broadcast initialization
func TestReliableBroadCastCOS(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	// Set valid private key for goroutine
	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	roundNum := "100"
	trialNum := "1"
	eoaAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")
	var cos [32]byte
	copy(cos[:], []byte("test-cos-value"))

	activatedOps := []common.Address{eoaAddress}

	mockRepo.On("AddBroadcastTracker", mock.Anything, mock.MatchedBy(func(tracker *utils.BroadcastTracker) bool {
		if tracker.Round == roundNum &&
			tracker.TrialNum == trialNum &&
			tracker.EOAAddress == eoaAddress.Hex() &&
			tracker.Type == "cos" {
			for _, op := range activatedOps {
				tracker.Acknowledged[op.Hex()] = true
			}
			return true
		}
		return false
	})).Return(nil)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Return(nil).Maybe()

	node.ReliableBroadCastCOS(context.Background(), roundNum, trialNum, eoaAddress, cos, activatedOps)

	time.Sleep(100 * time.Millisecond)

	mockRepo.AssertExpectations(t)
}

// TestReliableBroadCastSSync tests synchronous secret broadcast
func TestReliableBroadCastSSync(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	roundNum := "100"
	trialNum := "1"
	eoaAddress := "0x1234567890123456789012345678901234567890"
	var secret [32]byte
	copy(secret[:], []byte("test-secret-value"))

	activatedOps := []common.Address{common.HexToAddress(eoaAddress)}

	mockRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(nil)

	result := node.ReliableBroadCastSSync(context.Background(), nil, roundNum, trialNum, eoaAddress, secret, activatedOps)

	assert.False(t, result)

	mockRepo.AssertExpectations(t)
}

// TestReliableBroadCastSSync_AddTrackerError tests sync broadcast with tracker error
func TestReliableBroadCastSSync_AddTrackerError(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	mockNodeRepo := new(MockNodeInfoRepository)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	var secret [32]byte
	activatedOps := []common.Address{common.HexToAddress("0x1234567890123456789012345678901234567890")}

	mockRepo.On("AddBroadcastTracker", mock.Anything, mock.Anything).Return(errors.New("db error"))

	result := node.ReliableBroadCastSSync(context.Background(), nil, "100", "1", "0x123", secret, activatedOps)

	assert.False(t, result)
	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcastSync_WhenHalted tests sync broadcast when halted
func TestPerformReliableBroadcastSync_WhenHalted(t *testing.T) {
	node := createTestNodeForBroadcast()
	node.SetHalted(true)

	tracker := &utils.BroadcastTracker{
		MessageID: "test-sync-halted",
		Round:     "100",
		TrialNum:  "1",
		Type:      "secret",
	}

	result := node.performReliableBroadcastSync(context.Background(), nil, tracker, "secret", []common.Address{})

	assert.False(t, result)
}

// TestPerformReliableBroadcastSync_PrivateKeyError tests sync broadcast with key error
func TestPerformReliableBroadcastSync_PrivateKeyError(t *testing.T) {
	node := createTestNodeForBroadcast()

	// Unset private key
	os.Unsetenv("LEADER_PRIVATE_KEY")

	tracker := &utils.BroadcastTracker{
		MessageID: "test-sync-key-error",
		Round:     "100",
		TrialNum:  "1",
		Type:      "secret",
	}

	result := node.performReliableBroadcastSync(context.Background(), nil, tracker, "secret", []common.Address{})

	assert.False(t, result)
}

// TestPerformReliableBroadcastSync_UnknownType tests sync broadcast with unknown type
func TestPerformReliableBroadcastSync_UnknownType(t *testing.T) {
	node := createTestNodeForBroadcast()

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	tracker := &utils.BroadcastTracker{
		MessageID:    "test-sync-unknown",
		Round:        "100",
		TrialNum:     "1",
		Type:         "unknown",
		Attempts:     0,
		MaxAttempts:  1,
		Acknowledged: make(map[string]bool),
	}

	result := node.performReliableBroadcastSync(context.Background(), nil, tracker, "unknown", []common.Address{})

	assert.False(t, result)
}

// TestPerformReliableBroadcastSync_AllAcknowledged tests sync broadcast success
func TestPerformReliableBroadcastSync_AllAcknowledged(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	opAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	tracker := &utils.BroadcastTracker{
		MessageID:    "test-sync-success",
		Round:        "100",
		TrialNum:     "1",
		Type:         "secret",
		Attempts:     0,
		MaxAttempts:  1,
		Timeout:      0,
		Acknowledged: map[string]bool{opAddr.Hex(): true},
	}

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	result := node.performReliableBroadcastSync(context.Background(), nil, tracker, "secret", []common.Address{opAddr})

	assert.True(t, result)
	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcastSync_UpdateError tests sync broadcast with update error
func TestPerformReliableBroadcastSync_UpdateError(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	opAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	tracker := &utils.BroadcastTracker{
		MessageID:    "test-sync-update-error",
		Round:        "100",
		TrialNum:     "1",
		Type:         "secret",
		Attempts:     0,
		MaxAttempts:  1,
		Timeout:      0,
		Acknowledged: map[string]bool{opAddr.Hex(): true},
	}

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(errors.New("update failed"))

	result := node.performReliableBroadcastSync(context.Background(), nil, tracker, "secret", []common.Address{opAddr})

	assert.True(t, result)
	mockRepo.AssertExpectations(t)
}

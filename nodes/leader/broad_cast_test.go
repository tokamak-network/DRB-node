package leader_node

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/eapache/queue"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/connmgr"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/utils"
)

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

func (m *MockNodeInfoRepository) AddAndUpdateNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	return nil
}
func (m *MockNodeInfoRepository) DeleteNodeInfo(eoaAddress string) error { return nil }
func (m *MockNodeInfoRepository) DeleteNodeInfoByEOA(ctx context.Context, eoaAddress string) error {
	return nil
}
func (m *MockNodeInfoRepository) GetNodeInfo(eoaAddress string) (*utils.NodeInfo, error) {
	return nil, nil
}

// Mock libp2p interfaces for network testing
type MockHost struct {
	mock.Mock
}

func (m *MockHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
	args := m.Called(ctx, p, pids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(network.Stream), args.Error(1)
}

func (m *MockHost) Peerstore() peerstore.Peerstore {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(peerstore.Peerstore)
}

// Implementing minimal host.Host interface methods (only what's needed for tests)
func (m *MockHost) ID() peer.ID                                                     { return "" }
func (m *MockHost) Addrs() []multiaddr.Multiaddr                                    { return nil }
func (m *MockHost) Network() network.Network                                        { return nil }
func (m *MockHost) Mux() protocol.Switch                                            { return nil }
func (m *MockHost) Connect(ctx context.Context, pi peer.AddrInfo) error             { return nil }
func (m *MockHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {}
func (m *MockHost) SetStreamHandlerMatch(protocol.ID, func(protocol.ID) bool, network.StreamHandler) {
}
func (m *MockHost) RemoveStreamHandler(pid protocol.ID) {}
func (m *MockHost) Close() error                        { return nil }
func (m *MockHost) ConnManager() connmgr.ConnManager    { return nil } // Required by host.Host interface
func (m *MockHost) EventBus() event.Bus                 { return nil } // Required by host.Host interface

type MockPeerstore struct {
	mock.Mock
}

func (m *MockPeerstore) AddAddr(p peer.ID, addr multiaddr.Multiaddr, ttl time.Duration) {
	m.Called(p, addr, ttl)
}

// Implementing peerstore.Peerstore interface methods
func (m *MockPeerstore) AddAddrs(peer.ID, []multiaddr.Multiaddr, time.Duration)         {}
func (m *MockPeerstore) SetAddr(peer.ID, multiaddr.Multiaddr, time.Duration)            {}
func (m *MockPeerstore) SetAddrs(peer.ID, []multiaddr.Multiaddr, time.Duration)         {}
func (m *MockPeerstore) UpdateAddrs(peer.ID, time.Duration, time.Duration)              {}
func (m *MockPeerstore) Addrs(peer.ID) []multiaddr.Multiaddr                            { return nil }
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

type MockStreamForBroadcast struct {
	mock.Mock
}

func (m *MockStreamForBroadcast) Write(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockStreamForBroadcast) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStreamForBroadcast) Read([]byte) (int, error)         { return 0, io.EOF }
func (m *MockStreamForBroadcast) Reset() error                     { return nil }
func (m *MockStreamForBroadcast) SetDeadline(time.Time) error      { return nil }
func (m *MockStreamForBroadcast) SetReadDeadline(time.Time) error  { return nil }
func (m *MockStreamForBroadcast) SetWriteDeadline(time.Time) error { return nil }
func (m *MockStreamForBroadcast) CloseWrite() error                { return nil }
func (m *MockStreamForBroadcast) CloseRead() error                 { return nil }
func (m *MockStreamForBroadcast) ID() string                       { return "" }
func (m *MockStreamForBroadcast) Protocol() protocol.ID            { return "" }
func (m *MockStreamForBroadcast) SetProtocol(protocol.ID) error    { return nil }
func (m *MockStreamForBroadcast) Stat() network.Stats              { return network.Stats{} }
func (m *MockStreamForBroadcast) Conn() network.Conn               { return nil }
func (m *MockStreamForBroadcast) Scope() network.StreamScope       { return nil }

// MockP2PClientWithNodeInfo extends the existing MockP2PClient to support custom node info
type MockP2PClientWithNodeInfo struct {
	nodeInfo map[string]*utils.NodeInfo
}

func (m *MockP2PClientWithNodeInfo) GetConnectedPeers(ctx context.Context) map[string]*utils.NodeInfo {
	return m.nodeInfo
}

func (m *MockP2PClientWithNodeInfo) GetHostInstance() host.Host {
	return nil
}

func createTestNodeForBroadcast() *LeaderNode {
	// Create a test client with generated private key
	testPrivateKey, _ := crypto.GenerateKey()
	testContractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	testClient := &utils.Client{
		ContractAddress: testContractAddress,
		PrivateKey:      testPrivateKey,
	}

	return &LeaderNode{
		client:              testClient,
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

// TestPerformReliableBroadcast_NotAllAcknowledged tests retry logic when not all nodes acknowledge
func TestPerformReliableBroadcast_NotAllAcknowledged(t *testing.T) {
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

	// Create tracker where operators start unacknowledged but get acknowledged during first attempt
	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	opAddr2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")
	tracker := &utils.BroadcastTracker{
		MessageID:   "test-not-ack",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cvs",
		Attempts:    0,
		MaxAttempts: 2,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
			opAddr2.Hex(): false, // Not acknowledged initially
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	// Mock to acknowledge one node after first update, but not all
	firstCall := true
	mockRepo.On("UpdateBroadcastTracker", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		if firstCall {
			// Acknowledge one operator after first attempt
			tracker.Acknowledged[opAddr1.Hex()] = true
			firstCall = false
		}
	}).Return(nil).Times(2)

	node.performReliableBroadcast(context.Background(), nil, tracker, "cvs", []common.Address{})

	// Should attempt MaxAttempts times since not all acknowledged
	assert.Equal(t, 2, tracker.Attempts)

	// Active broadcast should be cleaned up
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcast_MaxAttemptsReachedWithUnacknowledged tests logging when max attempts reached
func TestPerformReliableBroadcast_MaxAttemptsReachedWithUnacknowledged(t *testing.T) {
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

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	opAddr2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")
	tracker := &utils.BroadcastTracker{
		MessageID:   "test-max-attempts",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cos",
		Attempts:    0,
		MaxAttempts: 3,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false, // Never acknowledges
			opAddr2.Hex(): false, // Never acknowledges
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Times(3)

	// Pass empty array to avoid network calls - focus on retry and max attempts logic
	node.performReliableBroadcast(context.Background(), nil, tracker, "cos", []common.Address{})

	// Verify max attempts reached
	assert.Equal(t, 3, tracker.Attempts)
	assert.False(t, tracker.Acknowledged[opAddr1.Hex()])
	assert.False(t, tracker.Acknowledged[opAddr2.Hex()])

	// Active broadcast should be cleaned up
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcast_PartialAcknowledgment tests scenario with partial acknowledgment
func TestPerformReliableBroadcast_PartialAcknowledgment(t *testing.T) {
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

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	opAddr2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")
	opAddr3 := common.HexToAddress("0xDeF1234567890123456789012345678901234567")
	tracker := &utils.BroadcastTracker{
		MessageID:   "test-partial-ack",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cvs",
		Attempts:    0,
		MaxAttempts: 2,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): true,  // Acknowledged
			opAddr2.Hex(): false, // Not acknowledged
			opAddr3.Hex(): false, // Not acknowledged
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Times(2)

	// Pass empty array to avoid network calls
	node.performReliableBroadcast(context.Background(), nil, tracker, "cvs", []common.Address{})

	// Should reach max attempts
	assert.Equal(t, 2, tracker.Attempts)

	// Active broadcast should be cleaned up
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcastSync_NotAllAcknowledged tests sync broadcast with unacknowledged nodes
func TestPerformReliableBroadcastSync_NotAllAcknowledged(t *testing.T) {
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
		MessageID:    "test-sync-not-ack",
		Round:        "100",
		TrialNum:     "1",
		Type:         "secret",
		Attempts:     0,
		MaxAttempts:  3,
		Timeout:      0,
		Acknowledged: map[string]bool{opAddr.Hex(): false},
	}

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Times(3)

	// Pass empty array to avoid network calls
	result := node.performReliableBroadcastSync(context.Background(), nil, tracker, "secret", []common.Address{})

	assert.False(t, result)
	assert.Equal(t, 3, tracker.Attempts)
	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcastSync_MaxAttemptsReached tests sync broadcast max attempts
func TestPerformReliableBroadcastSync_MaxAttemptsReached(t *testing.T) {
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

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	opAddr2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")
	tracker := &utils.BroadcastTracker{
		MessageID:   "test-sync-max",
		Round:       "100",
		TrialNum:    "1",
		Type:        "secret",
		Attempts:    0,
		MaxAttempts: 2,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
			opAddr2.Hex(): false,
		},
	}

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Times(2)

	// Pass empty array to avoid network calls
	result := node.performReliableBroadcastSync(context.Background(), nil, tracker, "secret", []common.Address{})

	// Should return false after max attempts with no acknowledgments
	assert.False(t, result)
	assert.Equal(t, 2, tracker.Attempts)
	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcast_WithNetworkAttempt tests the network streaming code path
func TestPerformReliableBroadcast_WithNetworkAttempt(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	// Create operator addresses
	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	privKey, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID, _ := peer.IDFromPrivateKey(privKey)

	// Create a mock node repo that returns specific node info
	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{
			PeerID:     peerID.String(), 
			IP:         "127.0.0.1",
			Port:       "8080",
			EOAAddress: opAddr1.Hex(),
		},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	// Create mocks for Host and Peerstore
	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)

	// Setup mock expectations
	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()

	// Mock NewStream to return error (simulating stream creation failure)
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("stream creation failed"))

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-network-fail",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cvs",
		Attempts:    0,
		MaxAttempts: 1,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false, // Not acknowledged - will try to send
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)
	node.performReliableBroadcast(context.Background(), mockHost, tracker, "cvs", []common.Address{opAddr1})

	// Verify the function ran and cleaned up
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)
	assert.Equal(t, 1, tracker.Attempts)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockPeerstore.AssertExpectations(t)
}

// TestPerformReliableBroadcast_WithSuccessfulStream tests successful stream creation and encoding
func TestPerformReliableBroadcast_WithSuccessfulStream(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	// Create operator addresses
	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	privKey, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID, _ := peer.IDFromPrivateKey(privKey)

	// Create a mock node repo that returns specific node info
	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{
			PeerID:     peerID.String(), 
			IP:         "127.0.0.1",
			Port:       "8080",
			EOAAddress: opAddr1.Hex(),
		},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	// Create mocks
	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	// Setup mock expectations
	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()

	// Mock NewStream to return successful stream
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil)

	// Mock stream operations - Write will be called by json.NewEncoder().Encode()
	mockStream.On("Write", mock.Anything).Return(len([]byte{}), nil)
	mockStream.On("Close").Return(nil)

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-network-success",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cos",
		Attempts:    0,
		MaxAttempts: 1,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false, // Not acknowledged - will try to send
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	// Call with mock host - should successfully send
	node.performReliableBroadcast(context.Background(), mockHost, tracker, "cos", []common.Address{opAddr1})

	// Verify the function ran and cleaned up
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)
	assert.Equal(t, 1, tracker.Attempts)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockPeerstore.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

// TestPerformReliableBroadcastSync_WithNetworkAttempt tests sync broadcast network streaming code path
func TestPerformReliableBroadcastSync_WithNetworkAttempt(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	// Create operator addresses
	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	privKey, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID, _ := peer.IDFromPrivateKey(privKey)

	// Create a mock node repo that returns specific node info
	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{
			PeerID:     peerID.String(), 
			IP:         "127.0.0.1",
			Port:       "8080",
			EOAAddress: opAddr1.Hex(),
		},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	// Create mocks for Host and Peerstore
	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)

	// Setup mock expectations
	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()

	// Mock NewStream to return error (simulating stream creation failure)
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("stream creation failed"))

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-sync-network-fail",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "secret",
		Attempts:    0,
		MaxAttempts: 1,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false, // Not acknowledged - will try to send
		},
	}

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	// Call with mock host - will fail when trying to create stream but will cover the code path
	result := node.performReliableBroadcastSync(context.Background(), mockHost, tracker, "secret", []common.Address{opAddr1})

	// Should return false due to unacknowledged node
	assert.False(t, result)
	assert.Equal(t, 1, tracker.Attempts)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockPeerstore.AssertExpectations(t)
}

// TestPerformReliableBroadcastSync_WithSuccessfulStream tests sync broadcast successful stream creation
func TestPerformReliableBroadcastSync_WithSuccessfulStream(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	// Create operator addresses
	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	privKey, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID, _ := peer.IDFromPrivateKey(privKey)

	// Create a mock node repo that returns specific node info
	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{
			PeerID:     peerID.String(), 
			IP:         "127.0.0.1",
			Port:       "8080",
			EOAAddress: opAddr1.Hex(),
		},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	// Create mocks
	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	// Setup mock expectations
	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()

	// Mock NewStream to return successful stream
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil)

	// Mock stream operations - Write will be called by json.NewEncoder().Encode()
	mockStream.On("Write", mock.Anything).Return(100, nil)
	mockStream.On("Close").Return(nil)

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-sync-network-success",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "secret",
		Attempts:    0,
		MaxAttempts: 1,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false, // Not acknowledged - will try to send
		},
	}

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	// Call with mock host - should successfully send
	result := node.performReliableBroadcastSync(context.Background(), mockHost, tracker, "secret", []common.Address{opAddr1})

	// Should return false since node is not acknowledged
	assert.False(t, result)
	assert.Equal(t, 1, tracker.Attempts)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockPeerstore.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

// TestPerformReliableBroadcastSync_WithEncodingError tests sync broadcast with encoding error
func TestPerformReliableBroadcastSync_WithEncodingError(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	// Create operator addresses
	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	privKey, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID, _ := peer.IDFromPrivateKey(privKey)

	// Create a mock node repo that returns specific node info
	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{
			PeerID:     peerID.String(),
			IP:         "127.0.0.1",
			Port:       "8080",
			EOAAddress: opAddr1.Hex(),
		},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	// Create mocks
	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	// Setup mock expectations
	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()

	// Mock NewStream to return successful stream
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil)

	// Mock stream Write to fail (simulating encoding error)
	mockStream.On("Write", mock.Anything).Return(0, errors.New("write error"))
	mockStream.On("Close").Return(nil)

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-sync-encode-error",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "secret",
		Attempts:    0,
		MaxAttempts: 1,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
		},
	}

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	// Call with mock host - encoding should fail
	result := node.performReliableBroadcastSync(context.Background(), mockHost, tracker, "secret", []common.Address{opAddr1})

	// Should return false due to encoding failure and unacknowledged node
	assert.False(t, result)
	assert.Equal(t, 1, tracker.Attempts)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockPeerstore.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

// TestPerformReliableBroadcast_SecretBroadcastType tests "secret" broadcast type with network
func TestPerformReliableBroadcast_SecretBroadcastType(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	privKey, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID, _ := peer.IDFromPrivateKey(privKey)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{
			PeerID:     peerID.String(),
			IP:         "127.0.0.1",
			Port:       "8080",
			EOAAddress: opAddr1.Hex(),
		},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil)
	mockStream.On("Write", mock.Anything).Return(100, nil)
	mockStream.On("Close").Return(nil)

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-secret-type",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "secret",
		Attempts:    0,
		MaxAttempts: 1,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	node.performReliableBroadcast(context.Background(), mockHost, tracker, "secret", []common.Address{opAddr1})

	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)
	assert.Equal(t, 1, tracker.Attempts)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

// TestPerformReliableBroadcastSync_CvsBroadcastType tests "cvs" broadcast type in sync function
func TestPerformReliableBroadcastSync_CvsBroadcastType(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	privKey, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID, _ := peer.IDFromPrivateKey(privKey)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{
			PeerID:     peerID.String(),
			IP:         "127.0.0.1",
			Port:       "8080",
			EOAAddress: opAddr1.Hex(),
		},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil)
	mockStream.On("Write", mock.Anything).Return(100, nil)
	mockStream.On("Close").Return(nil)

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-sync-cvs-type",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cvs",
		Attempts:    0,
		MaxAttempts: 1,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
		},
	}

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	result := node.performReliableBroadcastSync(context.Background(), mockHost, tracker, "cvs", []common.Address{opAddr1})

	assert.False(t, result)
	assert.Equal(t, 1, tracker.Attempts)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

// TestPerformReliableBroadcastSync_CosBroadcastType tests "cos" broadcast type in sync function
func TestPerformReliableBroadcastSync_CosBroadcastType(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	privKey, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID, _ := peer.IDFromPrivateKey(privKey)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{
			PeerID:     peerID.String(),
			IP:         "127.0.0.1",
			Port:       "8080",
			EOAAddress: opAddr1.Hex(),
		},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil)
	mockStream.On("Write", mock.Anything).Return(100, nil)
	mockStream.On("Close").Return(nil)

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-sync-cos-type",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cos",
		Attempts:    0,
		MaxAttempts: 1,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
		},
	}

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)

	result := node.performReliableBroadcastSync(context.Background(), mockHost, tracker, "cos", []common.Address{opAddr1})

	assert.False(t, result)
	assert.Equal(t, 1, tracker.Attempts)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

func TestPerformReliableBroadcast_MixedSuccessFailureInSingleAttempt(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")


	opAddr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	opAddr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	opAddr3 := common.HexToAddress("0x3333333333333333333333333333333333333333")

	
	privKey1, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	privKey2, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	privKey3, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID1, _ := peer.IDFromPrivateKey(privKey1)
	peerID2, _ := peer.IDFromPrivateKey(privKey2)
	peerID3, _ := peer.IDFromPrivateKey(privKey3)


	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{PeerID: peerID1.String(), IP: "127.0.0.1", Port: "8081", EOAAddress: opAddr1.Hex()},
		{PeerID: peerID2.String(), IP: "127.0.0.1", Port: "8082", EOAAddress: opAddr2.Hex()},
		{PeerID: peerID3.String(), IP: "127.0.0.1", Port: "8083", EOAAddress: opAddr3.Hex()},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)


	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream1 := new(MockStreamForBroadcast) 
	mockStream3 := new(MockStreamForBroadcast) 

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream1, nil).Maybe()                      
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("connection refused")).Maybe() // Node 2 fail
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream3, nil).Maybe()                
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream1, nil).Maybe()                      // Node 2 retry

	mockStream1.On("Write", mock.Anything).Return(100, nil)
	mockStream1.On("Close").Return(nil)
	mockStream3.On("Write", mock.Anything).Return(100, nil)
	mockStream3.On("Close").Return(nil)

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-mixed-success-failure",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cvs",
		Attempts:    0,
		MaxAttempts: 2,
		Timeout:     0, // No sleep for fast test
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
			opAddr2.Hex(): false, // This one will fail
			opAddr3.Hex(): false,
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)


	acknowledgeNodes := func() {
		time.Sleep(10 * time.Millisecond)
		tracker.Acknowledged[opAddr1.Hex()] = true
		tracker.Acknowledged[opAddr3.Hex()] = true
	}
	go acknowledgeNodes()

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Maybe()

	node.performReliableBroadcast(context.Background(), mockHost, tracker, "cvs", []common.Address{opAddr1, opAddr2, opAddr3})

	assert.Equal(t, 2, tracker.Attempts)
	
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)

}

func TestPerformReliableBroadcast_StreamClosesMidTransmission(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	privKey1, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID1, _ := peer.IDFromPrivateKey(privKey1)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{PeerID: peerID1.String(), IP: "127.0.0.1", Port: "8080", EOAAddress: opAddr1.Hex()},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil).Maybe()

	mockStream.On("Write", mock.Anything).Return(50, nil).Maybe()
	mockStream.On("Write", mock.Anything).Return(0, errors.New("stream closed")).Maybe()
	mockStream.On("Close").Return(nil).Maybe()

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-stream-closes-mid",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "secret",
		Attempts:    0,
		MaxAttempts: 2,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Maybe()

	// Second attempt should retry
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil).Maybe()
	mockStream.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream.On("Close").Return(nil).Maybe()

	node.performReliableBroadcast(context.Background(), mockHost, tracker, "secret", []common.Address{opAddr1})

	assert.Equal(t, 2, tracker.Attempts)
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

func TestPerformReliableBroadcast_LateAcknowledgmentDuringRetry(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	opAddr2 := common.HexToAddress("0xAbC1234567890123456789012345678901234567")


	privKey1, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	privKey2, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID1, _ := peer.IDFromPrivateKey(privKey1)
	peerID2, _ := peer.IDFromPrivateKey(privKey2)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{PeerID: peerID1.String(), IP: "127.0.0.1", Port: "8081", EOAAddress: opAddr1.Hex()},
		{PeerID: peerID2.String(), IP: "127.0.0.1", Port: "8082", EOAAddress: opAddr2.Hex()},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream1 := new(MockStreamForBroadcast)
	mockStream2 := new(MockStreamForBroadcast)

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()

	// Both nodes succeed in first attempt
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream1, nil).Maybe()
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream2, nil).Maybe()
	mockStream1.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream1.On("Close").Return(nil).Maybe()
	mockStream2.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream2.On("Close").Return(nil).Maybe()

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-late-ack",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cos",
		Attempts:    0,
		MaxAttempts: 3,
		Timeout:     10, 
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
			opAddr2.Hex(): false,
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	go func() {
		time.Sleep(15 * time.Millisecond) 
		tracker.Acknowledged[opAddr1.Hex()] = true
	}()

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Maybe()


	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	node.performReliableBroadcast(ctx, mockHost, tracker, "cos", []common.Address{opAddr1, opAddr2})

	time.Sleep(50 * time.Millisecond)

	assert.True(t, tracker.Acknowledged[opAddr1.Hex()])
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
}

func TestPerformReliableBroadcast_RaceConditionAcknowledgment(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	// Generate valid peer ID
	privKey1, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID1, _ := peer.IDFromPrivateKey(privKey1)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{PeerID: peerID1.String(), IP: "127.0.0.1", Port: "8080", EOAAddress: opAddr1.Hex()},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()

	// First attempt succeeds
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil).Maybe()
	mockStream.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream.On("Close").Return(nil).Maybe()

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-race-condition",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cvs",
		Attempts:    0,
		MaxAttempts: 2,
		Timeout:     0, // No timeout for fast test
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	go func() {
		time.Sleep(5 * time.Millisecond)
		tracker.Acknowledged[opAddr1.Hex()] = true
	}()

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Maybe()

	node.performReliableBroadcast(context.Background(), mockHost, tracker, "cvs", []common.Address{opAddr1})

	// Should break early if acknowledgment received
	time.Sleep(20 * time.Millisecond)
	assert.True(t, tracker.Acknowledged[opAddr1.Hex()])
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
}

func TestPerformReliableBroadcast_MultipleNodesMixedNetworkConditions(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	// Create 4 operator addresses
	opAddr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	opAddr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	opAddr3 := common.HexToAddress("0x3333333333333333333333333333333333333333")
	opAddr4 := common.HexToAddress("0x4444444444444444444444444444444444444444")

	// Generate valid peer IDs
	privKey1, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	privKey2, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	privKey3, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	privKey4, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID1, _ := peer.IDFromPrivateKey(privKey1)
	peerID2, _ := peer.IDFromPrivateKey(privKey2)
	peerID3, _ := peer.IDFromPrivateKey(privKey3)
	peerID4, _ := peer.IDFromPrivateKey(privKey4)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{PeerID: peerID1.String(), IP: "127.0.0.1", Port: "8081", EOAAddress: opAddr1.Hex()},
		{PeerID: peerID2.String(), IP: "127.0.0.1", Port: "8082", EOAAddress: opAddr2.Hex()},
		{PeerID: peerID3.String(), IP: "127.0.0.1", Port: "8083", EOAAddress: opAddr3.Hex()},
		{PeerID: peerID4.String(), IP: "127.0.0.1", Port: "8084", EOAAddress: opAddr4.Hex()},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream1 := new(MockStreamForBroadcast) // Success
	mockStream3 := new(MockStreamForBroadcast) // Success
	mockStream4 := new(MockStreamForBroadcast) // Success

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream1, nil).Maybe()
	mockStream1.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream1.On("Close").Return(nil).Maybe()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("connection refused")).Maybe()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream3, nil).Maybe()
	mockStream3.On("Write", mock.Anything).Return(0, errors.New("write failed")).Maybe()
	mockStream3.On("Close").Return(nil).Maybe()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream4, nil).Maybe()
	mockStream4.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream4.On("Close").Return(nil).Maybe()

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-mixed-network",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "secret",
		Attempts:    0,
		MaxAttempts: 3,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
			opAddr2.Hex(): false, 
			opAddr3.Hex(): false, 
			opAddr4.Hex(): false,
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)


	go func() {
		time.Sleep(10 * time.Millisecond)
		tracker.Acknowledged[opAddr1.Hex()] = true
		tracker.Acknowledged[opAddr4.Hex()] = true
	}()

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Maybe()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream1, nil).Maybe()
	mockStream1.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream1.On("Close").Return(nil).Maybe()

	
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream3, nil).Maybe()
	mockStream3.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream3.On("Close").Return(nil).Maybe()

	
	go func() {
		time.Sleep(20 * time.Millisecond)
		tracker.Acknowledged[opAddr2.Hex()] = true
		tracker.Acknowledged[opAddr3.Hex()] = true
	}()

	node.performReliableBroadcast(context.Background(), mockHost, tracker, "secret", []common.Address{opAddr1, opAddr2, opAddr3, opAddr4})

	time.Sleep(50 * time.Millisecond)

	assert.GreaterOrEqual(t, tracker.Attempts, 2)               
	assert.LessOrEqual(t, tracker.Attempts, tracker.MaxAttempts) 
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
}

// TestPerformReliableBroadcast_PartialStreamWrite tests handling of partial stream writes
func TestPerformReliableBroadcast_PartialStreamWrite(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	opAddr1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	// Generate valid peer ID
	privKey1, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID1, _ := peer.IDFromPrivateKey(privKey1)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{PeerID: peerID1.String(), IP: "127.0.0.1", Port: "8080", EOAAddress: opAddr1.Hex()},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream := new(MockStreamForBroadcast)

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()

	
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil).Maybe()
	
	mockStream.On("Write", mock.Anything).Return(30, nil).Maybe()

	mockStream.On("Write", mock.Anything).Return(0, errors.New("connection reset")).Maybe()
	mockStream.On("Close").Return(nil).Maybe()

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-partial-write",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cvs",
		Attempts:    0,
		MaxAttempts: 2,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Maybe()

	
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream, nil).Maybe()
	mockStream.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream.On("Close").Return(nil).Maybe()

	node.performReliableBroadcast(context.Background(), mockHost, tracker, "cvs", []common.Address{opAddr1})

	assert.Equal(t, 2, tracker.Attempts)
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}


func TestPerformReliableBroadcastSync_MixedSuccessFailureInSingleAttempt(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	opAddr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	opAddr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	opAddr3 := common.HexToAddress("0x3333333333333333333333333333333333333333")

	
	privKey1, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	privKey2, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	privKey3, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID1, _ := peer.IDFromPrivateKey(privKey1)
	peerID2, _ := peer.IDFromPrivateKey(privKey2)
	peerID3, _ := peer.IDFromPrivateKey(privKey3)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{PeerID: peerID1.String(), IP: "127.0.0.1", Port: "8081", EOAAddress: opAddr1.Hex()},
		{PeerID: peerID2.String(), IP: "127.0.0.1", Port: "8082", EOAAddress: opAddr2.Hex()},
		{PeerID: peerID3.String(), IP: "127.0.0.1", Port: "8083", EOAAddress: opAddr3.Hex()},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream1 := new(MockStreamForBroadcast)
	mockStream3 := new(MockStreamForBroadcast)

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()

	
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream1, nil).Maybe()
	mockStream1.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream1.On("Close").Return(nil).Maybe()

	
	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("connection refused")).Maybe()

	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream3, nil).Maybe()
	mockStream3.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream3.On("Close").Return(nil).Maybe()

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-sync-mixed",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "secret",
		Attempts:    0,
		MaxAttempts: 2,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
			opAddr2.Hex(): false,
			opAddr3.Hex(): false,
		},
	}

	
	go func() {
		time.Sleep(10 * time.Millisecond)
		tracker.Acknowledged[opAddr1.Hex()] = true
		tracker.Acknowledged[opAddr3.Hex()] = true
	}()

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Maybe()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream1, nil).Maybe()
	mockStream1.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream1.On("Close").Return(nil).Maybe()

	result := node.performReliableBroadcastSync(context.Background(), mockHost, tracker, "secret", []common.Address{opAddr1, opAddr2, opAddr3})

	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, 2, tracker.Attempts)
	assert.False(t, result) 

	mockRepo.AssertExpectations(t)
	mockHost.AssertExpectations(t)
}

func TestHandleAcknowledgment_AfterPartialDeliveryFailure(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	messageID := "test-partial-delivery-ack"
	opAddr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	opAddr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	tracker := &utils.BroadcastTracker{
		MessageID: messageID,
		Round:     "100",
		TrialNum:  "1",
		Type:      "cvs",
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false, 
			opAddr2.Hex(): true,  
		},
	}
	node.SetActiveBroadcast(messageID, tracker)

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil)


	ack := utils.AcknowledgmentMessage{
		MessageID:  messageID,
		EOAAddress: opAddr1.Hex(),
		Status:     "received",
		Type:       "cvs",
	}

	node.HandleAcknowledgment(context.Background(), ack)

	updatedTracker, _ := node.GetActiveBroadcast(messageID)
	assert.True(t, updatedTracker.Acknowledged[opAddr1.Hex()])
	assert.True(t, updatedTracker.Acknowledged[opAddr2.Hex()])

	mockRepo.AssertExpectations(t)
}

func TestPerformReliableBroadcast_EncodingFailureAfterStreamCreation(t *testing.T) {
	node := createTestNodeForBroadcast()
	mockRepo := new(MockBroadcastTrackerRepository)
	node.broadcastTrackerRepository = mockRepo

	pk, _ := crypto.GenerateKey()
	pkHex := hex.EncodeToString(crypto.FromECDSA(pk))
	os.Setenv("LEADER_PRIVATE_KEY", pkHex)
	defer os.Unsetenv("LEADER_PRIVATE_KEY")

	opAddr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	opAddr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	// Generate valid peer IDs
	privKey1, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	privKey2, _, _ := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 256)
	peerID1, _ := peer.IDFromPrivateKey(privKey1)
	peerID2, _ := peer.IDFromPrivateKey(privKey2)

	mockNodeRepo := new(MockNodeInfoRepository)
	mockNodeRepo.On("GetNodeInfos", mock.Anything).Return([]*utils.NodeInfo{
		{PeerID: peerID1.String(), IP: "127.0.0.1", Port: "8081", EOAAddress: opAddr1.Hex()},
		{PeerID: peerID2.String(), IP: "127.0.0.1", Port: "8082", EOAAddress: opAddr2.Hex()},
	}, nil)
	node.p2pClient = libp2putils.NewP2PClient(mockNodeRepo)

	mockHost := new(MockHost)
	mockPeerstore := new(MockPeerstore)
	mockStream1 := new(MockStreamForBroadcast)
	mockStream2 := new(MockStreamForBroadcast)

	mockHost.On("Peerstore").Return(mockPeerstore)
	mockPeerstore.On("AddAddr", mock.Anything, mock.Anything, mock.Anything).Return()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream1, nil).Maybe()
	mockStream1.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream1.On("Close").Return(nil).Maybe()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream2, nil).Maybe()
	mockStream2.On("Write", mock.Anything).Return(0, errors.New("encoding error")).Maybe()
	mockStream2.On("Close").Return(nil).Maybe()

	tracker := &utils.BroadcastTracker{
		MessageID:   "test-encoding-failure",
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  opAddr1.Hex(),
		Type:        "cos",
		Attempts:    0,
		MaxAttempts: 2,
		Timeout:     0,
		Acknowledged: map[string]bool{
			opAddr1.Hex(): false,
			opAddr2.Hex(): false,
		},
	}
	node.SetActiveBroadcast(tracker.MessageID, tracker)


	go func() {
		time.Sleep(10 * time.Millisecond)
		tracker.Acknowledged[opAddr1.Hex()] = true
	}()

	mockRepo.On("UpdateBroadcastTracker", mock.Anything, tracker).Return(nil).Maybe()


	mockHost.On("NewStream", mock.Anything, mock.Anything, mock.Anything).Return(mockStream2, nil).Maybe()
	mockStream2.On("Write", mock.Anything).Return(100, nil).Maybe()
	mockStream2.On("Close").Return(nil).Maybe()

	node.performReliableBroadcast(context.Background(), mockHost, tracker, "cos", []common.Address{opAddr1, opAddr2})

	assert.Equal(t, 2, tracker.Attempts)
	_, exists := node.GetActiveBroadcast(tracker.MessageID)
	assert.False(t, exists)

	mockRepo.AssertExpectations(t)

}

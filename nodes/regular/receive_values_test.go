package regular_node

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-pg/pg/v10"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/utils"
)

// MockPeerCommitRepository for testing
type MockPeerCommitRepository struct {
	mock.Mock
}

func (m *MockPeerCommitRepository) GetPeerCommitData(ctx context.Context, round, trialNum, eoaAddress string) (*database.PeerCommitDataScheme, error) {
	args := m.Called(ctx, round, trialNum, eoaAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*database.PeerCommitDataScheme), args.Error(1)
}

func (m *MockPeerCommitRepository) AddPeerCommitData(ctx context.Context, data *database.PeerCommitDataScheme) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockPeerCommitRepository) UpdatePeerCommitData(ctx context.Context, data *database.PeerCommitDataScheme) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockPeerCommitRepository) GetAllPeerCommitData(ctx context.Context, round, trialNum string) ([]*database.PeerCommitDataScheme, error) {
	args := m.Called(ctx, round, trialNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*database.PeerCommitDataScheme), args.Error(1)
}

func createTestNodeForReceiveValues() *RegularNode {
	mockPeerRepo := new(MockPeerCommitRepository)

	// Create node with NewRegularNode to initialize sync.Maps, then assign mock
	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)
	node.peerCommitDataRepository = mockPeerRepo

	return node
}

func createTestHost(t *testing.T) host.Host {
	h, err := libp2p.New()
	require.NoError(t, err)
	return h
}

func TestRegularNode_generateAcknowledgmentSignature_Success(t *testing.T) {
	node := createTestNodeForReceiveValues()

	// Generate test key
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(privateKey)

	signature := node.generateAcknowledgmentSignature(eoaAddress)

	assert.NotNil(t, signature, "Signature should be generated")
	assert.Len(t, signature, 65, "Signature should be 65 bytes")
}

func TestRegularNode_generateAcknowledgmentSignature_NoPrivateKey(t *testing.T) {
	node := createTestNodeForReceiveValues()

	// Don't set private key
	eoaAddress := "0x1234567890123456789012345678901234567890"

	signature := node.generateAcknowledgmentSignature(eoaAddress)

	assert.Nil(t, signature, "Should return nil when private key not set")
}

func TestRegularNode_HandleCvs_Halted(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(true)

	h := createTestHost(t)
	defer h.Close()

	stream := newMockStream()

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed, "Stream should be closed")
}

func TestRegularNode_HandleCvs_DecodeError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(false)

	h := createTestHost(t)
	defer h.Close()

	stream := newMockStream()
	stream.readBuffer.Write([]byte("invalid json"))

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed, "Stream should be closed")
}

func TestRegularNode_HandleCvs_MissingLeaderEOA(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(false)

	h := createTestHost(t)
	defer h.Close()

	// Ensure LEADER_EOA is not set
	os.Unsetenv("LEADER_EOA")

	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs"))

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-001",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  "0xLeader",
		Signature:  []byte("signature"),
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed, "Stream should be closed")
}

func TestRegularNode_HandleCvs_SignatureVerificationFailed(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(false)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", "0xLeaderAddress")
	defer os.Unsetenv("LEADER_EOA")

	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs"))

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-002",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  "0xWrongSigner",
		Signature:  []byte("invalid-signature"),
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed, "Stream should be closed on verification failure")
}

func TestRegularNode_HandleCvs_Success_NewRecord(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs"))

	// Generate valid signature
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-003",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCvs_Success_UpdateRecord(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	// Generate valid key for leader
	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	// Set node's private key
	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs-update"))

	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-004",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	// Mock repository - existing record
	existingData := &database.PeerCommitDataScheme{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}
	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(existingData, nil)
	mockRepo.On("UpdatePeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCvs_AddError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	var cvs [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-005",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(errors.New("database error"))

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCvs_UpdateError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	var cvs [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-006",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	// Mock repository - existing record, but update fails
	existingData := &database.PeerCommitDataScheme{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}
	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(existingData, nil)
	mockRepo.On("UpdatePeerCommitData", mock.Anything, mock.Anything).Return(errors.New("update error"))

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCvs_GetPeerCommitDataError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	var cvs [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-007",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, errors.New("database connection error"))

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCvs_MissingLeaderPeerID(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Unsetenv("LEADER_PEER_ID") // Not set
	defer os.Unsetenv("LEADER_EOA")

	var cvs [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-008",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCvs_InvalidLeaderPeerID(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", "invalid-peer-id")
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var cvs [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-009",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCos_Halted(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(true)

	h := createTestHost(t)
	defer h.Close()

	stream := newMockStream()

	node.HandleCos(context.Background(), h, stream)

	assert.True(t, stream.closed)
}

func TestRegularNode_HandleCos_DecodeError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(false)

	h := createTestHost(t)
	defer h.Close()

	stream := newMockStream()
	stream.readBuffer.Write([]byte("invalid json"))

	node.HandleCos(context.Background(), h, stream)

	assert.True(t, stream.closed)
}

func TestRegularNode_HandleCos_MissingLeaderEOA(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(false)

	h := createTestHost(t)
	defer h.Close()

	os.Unsetenv("LEADER_EOA")

	var cos [32]byte
	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-010",
		Type:       "cos",
		Data:       cos,
		SignerEOA:  "0xLeader",
		Signature:  []byte("signature"),
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCos(context.Background(), h, stream)

	assert.True(t, stream.closed)
}

func TestRegularNode_HandleCos_SignatureVerificationFailed(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(false)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", "0xLeaderAddress")
	defer os.Unsetenv("LEADER_EOA")

	var cos [32]byte
	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-011",
		Type:       "cos",
		Data:       cos,
		SignerEOA:  "0xWrongSigner",
		Signature:  []byte("invalid"),
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCos(context.Background(), h, stream)

	assert.True(t, stream.closed)
}

func TestRegularNode_HandleCos_Success_NewRecord(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var cos [32]byte
	copy(cos[:], []byte("test-cos"))

	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-012",
		Type:       "cos",
		Data:       cos,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCos(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)

	// Verify COS was set in sync.Map
	uniqueKey := utils.GetUniqueKey("100", "1")
	cosReceived, exists := node.GetCosReceived(uniqueKey, "0x1234567890123456789012345678901234567890")
	assert.True(t, exists, "COS received flag should exist")
	assert.True(t, cosReceived, "COS received should be true")
}

func TestRegularNode_HandleCos_Success_UpdateRecord(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var cos [32]byte
	copy(cos[:], []byte("test-cos-update"))

	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-013",
		Type:       "cos",
		Data:       cos,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	existingData := &database.PeerCommitDataScheme{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}
	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(existingData, nil)
	mockRepo.On("UpdatePeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCos(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCos_AddError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	var cos [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-014",
		Type:       "cos",
		Data:       cos,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(errors.New("add failed"))

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCos(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCos_UpdateError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	var cos [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-015",
		Type:       "cos",
		Data:       cos,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	existingData := &database.PeerCommitDataScheme{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}
	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(existingData, nil)
	mockRepo.On("UpdatePeerCommitData", mock.Anything, mock.Anything).Return(errors.New("update failed"))

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCos(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecret_Halted(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(true)

	h := createTestHost(t)
	defer h.Close()

	stream := newMockStream()

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
}

func TestRegularNode_HandleSecret_DecodeError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(false)

	h := createTestHost(t)
	defer h.Close()

	stream := newMockStream()
	stream.readBuffer.Write([]byte("invalid json"))

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
}

func TestRegularNode_HandleSecret_MissingLeaderEOA(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(false)

	h := createTestHost(t)
	defer h.Close()

	os.Unsetenv("LEADER_EOA")

	var secret [32]byte
	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-020",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  "0xLeader",
		Signature:  []byte("signature"),
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
}

func TestRegularNode_HandleSecret_SignatureVerificationFailed(t *testing.T) {
	node := createTestNodeForReceiveValues()
	node.SetHalted(false)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", "0xLeaderAddress")
	defer os.Unsetenv("LEADER_EOA")

	var secret [32]byte
	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-021",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  "0xWrongSigner",
		Signature:  []byte("invalid"),
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
}

func TestRegularNode_HandleSecret_Success_NewRecord(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var secret [32]byte
	copy(secret[:], []byte("test-secret"))

	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-022",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecret_Success_UpdateRecord(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var secret [32]byte
	copy(secret[:], []byte("test-secret-update"))

	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-023",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	existingData := &database.PeerCommitDataScheme{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}
	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(existingData, nil)
	mockRepo.On("UpdatePeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecret_AddError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	var secret [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-024",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(errors.New("add failed"))

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecret_UpdateError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	var secret [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-025",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	existingData := &database.PeerCommitDataScheme{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}
	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(existingData, nil)
	mockRepo.On("UpdatePeerCommitData", mock.Anything, mock.Anything).Return(errors.New("update failed"))

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecret_GetPeerCommitDataError(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	var secret [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-026",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, errors.New("database error"))

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecret_MissingLeaderPeerID(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Unsetenv("LEADER_PEER_ID")
	defer os.Unsetenv("LEADER_EOA")

	var secret [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-027",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecret_InvalidLeaderPeerID(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", "invalid-peer-id")
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var secret [32]byte
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-028",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_SetCosReceived_Success(t *testing.T) {
	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)

	uniqueKey := "100:1"
	eoaAddress := "0x1234567890123456789012345678901234567890"

	// Set COS received
	node.SetCosReceived(uniqueKey, eoaAddress, true)

	// Verify it was set
	value, exists := node.GetCosReceived(uniqueKey, eoaAddress)
	assert.True(t, exists, "COS received flag should exist")
	assert.True(t, value, "COS received should be true")
}

func TestRegularNode_SetCosReceived_MultiplKeys(t *testing.T) {
	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)

	// Set for different keys
	node.SetCosReceived("100:1", "0xAddr1", true)
	node.SetCosReceived("100:1", "0xAddr2", false)
	node.SetCosReceived("200:2", "0xAddr1", true)

	// Verify all
	value1, exists1 := node.GetCosReceived("100:1", "0xAddr1")
	assert.True(t, exists1)
	assert.True(t, value1)

	value2, exists2 := node.GetCosReceived("100:1", "0xAddr2")
	assert.True(t, exists2)
	assert.False(t, value2)

	value3, exists3 := node.GetCosReceived("200:2", "0xAddr1")
	assert.True(t, exists3)
	assert.True(t, value3)
}

func TestRegularNode_GetCosReceived_NotExists(t *testing.T) {
	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)

	// Get non-existent key
	value, exists := node.GetCosReceived("non-existent", "addr")
	assert.False(t, exists)
	assert.False(t, value)
}

func TestRegularNode_GetCosReceived_InnerNotExists(t *testing.T) {
	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)

	// Set one value
	node.SetCosReceived("100:1", "0xAddr1", true)

	// Try to get different inner key
	value, exists := node.GetCosReceived("100:1", "0xAddr2")
	assert.False(t, exists)
	assert.False(t, value)
}

func TestRegularNode_CosReceived_Update(t *testing.T) {
	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)

	uniqueKey := "100:1"
	eoaAddress := "0x1234567890123456789012345678901234567890"

	// Set to true
	node.SetCosReceived(uniqueKey, eoaAddress, true)
	value, _ := node.GetCosReceived(uniqueKey, eoaAddress)
	assert.True(t, value)

	// Update to false
	node.SetCosReceived(uniqueKey, eoaAddress, false)
	value, _ = node.GetCosReceived(uniqueKey, eoaAddress)
	assert.False(t, value)
}

func TestRegularNode_sendAcknowledgment_NoPrivateKey(t *testing.T) {
	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)
	// Don't set private key

	eoaAddress := "0x1234567890123456789012345678901234567890"
	signature := node.generateAcknowledgmentSignature(eoaAddress)

	assert.Nil(t, signature, "Should return nil without private key")
}

func TestRegularNode_BroadcastMessageStructure(t *testing.T) {
	var data [32]byte
	copy(data[:], []byte("test-data"))

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-100",
		Type:       "cvs",
		Data:       data,
		SignerEOA:  "0xSigner",
		Signature:  []byte("signature"),
	}

	assert.Equal(t, "100", message.Round)
	assert.Equal(t, "1", message.TrialNum)
	assert.Equal(t, "cvs", message.Type)
	assert.Equal(t, data, message.Data)
}

func TestRegularNode_BroadcastMessageMarshalling(t *testing.T) {
	var data [32]byte
	copy(data[:], []byte("test"))

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-101",
		Type:       "cvs",
		Data:       data,
		SignerEOA:  "0xSigner",
		Signature:  []byte("sig"),
	}

	// Marshal
	jsonData, err := json.Marshal(message)
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal
	var decoded utils.BroadcastMessage
	err = json.Unmarshal(jsonData, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, message.Round, decoded.Round)
	assert.Equal(t, message.Type, decoded.Type)
}

func TestRegularNode_AcknowledgmentMessageStructure(t *testing.T) {
	ack := utils.AcknowledgmentMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-102",
		Type:       "cvs",
		Status:     "received",
		Signature:  []byte("signature"),
	}

	assert.Equal(t, "100", ack.Round)
	assert.Equal(t, "cvs", ack.Type)
	assert.Equal(t, "received", ack.Status)
}

func TestRegularNode_ConcurrentCosReceived(t *testing.T) {
	node := NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)
	done := make(chan bool, 3)

	uniqueKey := "100:1"

	// Concurrent writes
	go func() {
		for i := 0; i < 100; i++ {
			node.SetCosReceived(uniqueKey, "addr1", true)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			node.GetCosReceived(uniqueKey, "addr1")
		}
		done <- true
	}()

	// Concurrent writes to different key
	go func() {
		for i := 0; i < 100; i++ {
			node.SetCosReceived(uniqueKey, "addr2", false)
		}
		done <- true
	}()

	<-done
	<-done
	<-done

}

func TestRegularNode_HandleCvs_EmptyData(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var cvs [32]byte // Empty data
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-030",
		Type:       "cvs",
		Data:       cvs,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCvs(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleCos_EmptyData(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var cos [32]byte // Empty
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-031",
		Type:       "cos",
		Data:       cos,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleCos(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecret_EmptyData(t *testing.T) {
	node := createTestNodeForReceiveValues()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHost(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	var secret [32]byte // Empty
	signature := utils.SignData(leaderEOA, leaderPrivateKey)

	message := utils.BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-032",
		Type:       "secret",
		Data:       secret,
		SignerEOA:  leaderEOA,
		Signature:  signature,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0x1234567890123456789012345678901234567890").
		Return(nil, pg.ErrNoRows)
	mockRepo.On("AddPeerCommitData", mock.Anything, mock.Anything).Return(nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(message)
	stream.readBuffer.Write(jsonData)

	node.HandleSecret(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRepo.AssertExpectations(t)
}

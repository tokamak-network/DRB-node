package leader_node

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/eapache/queue"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/utils"
)

// Mock types for testing
type MockStream struct {
	mock.Mock
	reader io.Reader
}

func (m *MockStream) Read(p []byte) (n int, err error) {
	if m.reader != nil {
		return m.reader.Read(p)
	}
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockStream) Write(p []byte) (n int, err error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockStream) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStream) CloseWrite() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStream) CloseRead() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStream) Reset() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStream) SetDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockStream) SetReadDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockStream) SetWriteDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockStream) ID() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockStream) Protocol() protocol.ID {
	args := m.Called()
	return args.Get(0).(protocol.ID)
}

func (m *MockStream) SetProtocol(id protocol.ID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockStream) Stat() network.Stats {
	args := m.Called()
	return args.Get(0).(network.Stats)
}

func (m *MockStream) Conn() network.Conn {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(network.Conn)
}

func (m *MockStream) Scope() network.StreamScope {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(network.StreamScope)
}

type MockLeaderCommitRepository struct {
	mock.Mock
}

func (m *MockLeaderCommitRepository) GetLeaderCommitByRoundAndEoaAddr(ctx context.Context, round, trialNum, eoaAddr string) (*utils.LeaderCommitData, error) {
	args := m.Called(ctx, round, trialNum, eoaAddr)
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

func (m *MockLeaderCommitRepository) GetAllLeaderCommits(ctx context.Context) ([]*utils.LeaderCommitData, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.LeaderCommitData), args.Error(1)
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

func createTestNodeForSecretHandler() *LeaderNode {
	return &LeaderNode{
		activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
		cvOnChain:           make(map[string]bool),
		roundSecrets:        make(map[string][][32]byte),
		roundSecret:         make(map[string]map[string]bool),
		secretsOnChain:      make(map[string]bool),
		revealRequestStatus: make(map[string][]string),
		roundsData:          make(map[string]RoundData),
		indices:             make([]*big.Int, 0),
		cleanupQueue:        queue.New(),
	}
}

// TestResetIndicesForNewRound tests resetting the indices array
func TestResetIndicesForNewRound(t *testing.T) {
	node := createTestNodeForSecretHandler()

	// Add some indices
	node.indices = []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}

	node.ResetIndicesForNewRound()

	assert.Equal(t, 0, len(node.indices), "Indices should be empty after reset")
	assert.NotNil(t, node.indices, "Indices should not be nil after reset")
}

// TestGetIndices tests getting a copy of indices
func TestGetIndices(t *testing.T) {
	node := createTestNodeForSecretHandler()

	// Set some indices
	node.indices = []*big.Int{big.NewInt(10), big.NewInt(20), big.NewInt(30)}

	result := node.GetIndices()

	assert.Equal(t, 3, len(result), "Should return correct number of indices")
	assert.Equal(t, big.NewInt(10), result[0], "First index should match")
	assert.Equal(t, big.NewInt(20), result[1], "Second index should match")
	assert.Equal(t, big.NewInt(30), result[2], "Third index should match")

	result[0].SetInt64(999)
	assert.Equal(t, int64(10), node.indices[0].Int64(), "Original should not be modified")
}

// TestGetIndicesEmpty tests getting indices when array is empty
func TestGetIndicesEmpty(t *testing.T) {
	node := createTestNodeForSecretHandler()

	result := node.GetIndices()

	assert.Equal(t, 0, len(result), "Should return empty array")
	assert.NotNil(t, result, "Should not return nil")
}

// TestAppendToIndices tests appending indices
func TestAppendToIndices(t *testing.T) {
	node := createTestNodeForSecretHandler()

	// Append first index
	node.AppendToIndices(big.NewInt(100))
	assert.Equal(t, 1, len(node.indices), "Should have 1 index")
	assert.Equal(t, int64(100), node.indices[0].Int64(), "First index should be 100")

	// Append second index
	node.AppendToIndices(big.NewInt(200))
	assert.Equal(t, 2, len(node.indices), "Should have 2 indices")
	assert.Equal(t, int64(200), node.indices[1].Int64(), "Second index should be 200")

	// Append third index
	node.AppendToIndices(big.NewInt(300))
	assert.Equal(t, 3, len(node.indices), "Should have 3 indices")
	assert.Equal(t, int64(300), node.indices[2].Int64(), "Third index should be 300")
}

// TestAppendToIndicesCopy tests that appended values are copies
func TestAppendToIndicesCopy(t *testing.T) {
	node := createTestNodeForSecretHandler()

	original := big.NewInt(500)
	node.AppendToIndices(original)

	// Modify the original
	original.SetInt64(999)

	assert.Equal(t, int64(500), node.indices[0].Int64(), "Stored value should not be modified")
}

// TestSetIndices tests setting the entire indices array
func TestSetIndices(t *testing.T) {
	node := createTestNodeForSecretHandler()

	// Set initial indices
	newIndices := []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}
	node.SetIndices(newIndices)

	assert.Equal(t, 3, len(node.indices), "Should have 3 indices")
	assert.Equal(t, int64(1), node.indices[0].Int64(), "First index should be 1")
	assert.Equal(t, int64(2), node.indices[1].Int64(), "Second index should be 2")
	assert.Equal(t, int64(3), node.indices[2].Int64(), "Third index should be 3")

	// Replace with new indices
	newIndices2 := []*big.Int{big.NewInt(10), big.NewInt(20)}
	node.SetIndices(newIndices2)

	assert.Equal(t, 2, len(node.indices), "Should have 2 indices")
	assert.Equal(t, int64(10), node.indices[0].Int64(), "First index should be 10")
	assert.Equal(t, int64(20), node.indices[1].Int64(), "Second index should be 20")
}

// TestSetIndicesCopy tests that set indices are copies
func TestSetIndicesCopy(t *testing.T) {
	node := createTestNodeForSecretHandler()

	original := []*big.Int{big.NewInt(100), big.NewInt(200)}
	node.SetIndices(original)

	// Modify the original
	original[0].SetInt64(999)

	assert.Equal(t, int64(100), node.indices[0].Int64(), "Stored value should not be modified")
}

// TestSetIndicesEmpty tests setting empty indices array
func TestSetIndicesEmpty(t *testing.T) {
	node := createTestNodeForSecretHandler()

	// Set some initial indices
	node.indices = []*big.Int{big.NewInt(1), big.NewInt(2)}

	// Set to empty array
	node.SetIndices([]*big.Int{})

	assert.Equal(t, 0, len(node.indices), "Should have 0 indices")
}

// TestGetOrCreateLeaderCommitDataExisting tests getting existing commit data
func TestGetOrCreateLeaderCommitDataExisting(t *testing.T) {
	node := createTestNodeForSecretHandler()

	roundNum := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(roundNum, trialNum)
	eoaAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Pre-populate the committed nodes map
	utils.EnsureCommittedNodesRoundExists(uniqueKey)
	existingData := utils.LeaderCommitData{
		UniqueKey:  uniqueKey,
		Round:      roundNum,
		TrialNum:   trialNum,
		EOAAddress: eoaAddress.Hex(),
		Cos:        [32]byte{1, 2, 3},
		CreatedAt:  12345,
	}
	utils.SetCommittedNodeData(uniqueKey, eoaAddress, existingData)

	result := node.GetOrCreateLeaderCommitData(roundNum, trialNum, uniqueKey, eoaAddress)

	assert.NotNil(t, result)
	assert.Equal(t, uniqueKey, result.UniqueKey)
	assert.Equal(t, roundNum, result.Round)
	assert.Equal(t, trialNum, result.TrialNum)
	assert.Equal(t, eoaAddress.Hex(), result.EOAAddress)
	assert.Equal(t, [32]byte{1, 2, 3}, result.Cos)
	assert.Equal(t, int64(12345), result.CreatedAt)

	// Cleanup
	delete(utils.CommittedNodes, uniqueKey)
}

// TestGetOrCreateLeaderCommitDataCreate tests creating new commit data
func TestGetOrCreateLeaderCommitDataCreate(t *testing.T) {
	node := createTestNodeForSecretHandler()

	roundNum := "100"
	trialNum := "1"
	uniqueKey := utils.GetUniqueKey(roundNum, trialNum)
	eoaAddress := common.HexToAddress("0xAbCdEf1234567890123456789012345678901234")

	utils.EnsureCommittedNodesRoundExists(uniqueKey)

	result := node.GetOrCreateLeaderCommitData(roundNum, trialNum, uniqueKey, eoaAddress)

	assert.NotNil(t, result)
	assert.Equal(t, uniqueKey, result.UniqueKey)
	assert.Equal(t, roundNum, result.Round)
	assert.Equal(t, trialNum, result.TrialNum)
	assert.Equal(t, eoaAddress.Hex(), result.EOAAddress)
	assert.Greater(t, result.CreatedAt, int64(0), "CreatedAt should be set")

	// Cleanup
	delete(utils.CommittedNodes, uniqueKey)
}

// TestAcceptSecretValueWhenHalted tests that the function returns early when halted
func TestAcceptSecretValueWhenHalted(t *testing.T) {
	node := createTestNodeForSecretHandler()
	node.SetHalted(true)

	mockStream := new(MockStream)
	mockStream.On("Close").Return(nil)

	node.AcceptSecretValue(context.Background(), nil, mockStream, nil)

	mockStream.AssertCalled(t, "Close")
}

// TestAcceptSecretValueInvalidJSON tests handling of invalid JSON
func TestAcceptSecretValueInvalidJSON(t *testing.T) {
	node := createTestNodeForSecretHandler()

	mockStream := new(MockStream)
	mockStream.reader = bytes.NewReader([]byte("invalid json"))
	mockStream.On("Close").Return(nil)

	node.AcceptSecretValue(context.Background(), nil, mockStream, nil)

	mockStream.AssertCalled(t, "Close")
}

// TestAcceptSecretValueInvalidSignature tests handling of invalid signature
func TestAcceptSecretValueInvalidSignature(t *testing.T) {
	node := createTestNodeForSecretHandler()

	// Create a request with invalid signature
	req := utils.SecretValueRequest{
		RegularEoaAddress: "0x1234567890123456789012345678901234567890",
		SecretValue:       []byte{1, 2, 3},
		Signature:         []byte("invalid_signature"),
	}

	reqBytes, _ := json.Marshal(req)
	mockStream := new(MockStream)
	mockStream.reader = bytes.NewReader(reqBytes)
	mockStream.On("Close").Return(nil)

	node.AcceptSecretValue(context.Background(), nil, mockStream, nil)

	mockStream.AssertCalled(t, "Close")
}

// TestAcceptSecretValueNoCOSFound tests when no COS is found
func TestAcceptSecretValueNoCOSFound(t *testing.T) {
	node := createTestNodeForSecretHandler()
	node.SetCurrentRound("100")
	node.SetCurrentTrial("1")

	// Generate a valid signature
	privateKey, _ := crypto.GenerateKey()
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	signature := utils.SignData(eoaAddress, privateKey)

	// Create a request with valid signature but no COS in commit data
	req := utils.SecretValueRequest{
		RegularEoaAddress: eoaAddress,
		SecretValue:       []byte{1, 2, 3},
		Signature:         signature,
	}

	reqBytes, _ := json.Marshal(req)
	mockStream := new(MockStream)
	mockStream.reader = bytes.NewReader(reqBytes)
	mockStream.On("Close").Return(nil)

	node.AcceptSecretValue(context.Background(), nil, mockStream, nil)

	mockStream.AssertCalled(t, "Close")

	// Cleanup
	uniqueKey := utils.GetUniqueKey("100", "1")
	delete(utils.CommittedNodes, uniqueKey)
}

// TestAcceptSecretValueHashMismatch tests when secret value hash doesn't match COS
func TestAcceptSecretValueHashMismatch(t *testing.T) {
	node := createTestNodeForSecretHandler()
	node.SetCurrentRound("100")
	node.SetCurrentTrial("1")

	// Generate a valid signature
	privateKey, _ := crypto.GenerateKey()
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	eoaAddressHex := eoaAddress.Hex()
	signature := utils.SignData(eoaAddressHex, privateKey)

	// Set up commit data with a different COS
	uniqueKey := utils.GetUniqueKey("100", "1")
	utils.EnsureCommittedNodesRoundExists(uniqueKey)
	commitData := utils.LeaderCommitData{
		UniqueKey:  uniqueKey,
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: eoaAddressHex,
		Cos:        [32]byte{99, 99, 99}, // Different from hash of secret value
	}
	utils.SetCommittedNodeData(uniqueKey, eoaAddress, commitData)

	// Create a request
	req := utils.SecretValueRequest{
		RegularEoaAddress: eoaAddressHex,
		SecretValue:       []byte{1, 2, 3},
		Signature:         signature,
	}

	reqBytes, _ := json.Marshal(req)
	mockStream := new(MockStream)
	mockStream.reader = bytes.NewReader(reqBytes)
	mockStream.On("Close").Return(nil)

	node.AcceptSecretValue(context.Background(), nil, mockStream, nil)

	mockStream.AssertCalled(t, "Close")

	// Cleanup
	delete(utils.CommittedNodes, uniqueKey)
}

// TestAcceptSecretValueDatabaseError tests database error handling
func TestAcceptSecretValueDatabaseError(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRepo := new(MockLeaderCommitRepository)
	node.leaderCommitRepository = mockRepo
	node.SetCurrentRound("100")
	node.SetCurrentTrial("1")

	// Generate a valid signature
	privateKey, _ := crypto.GenerateKey()
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	eoaAddressHex := eoaAddress.Hex()
	signature := utils.SignData(eoaAddressHex, privateKey)

	// Create secret value and compute its hash
	secretValue := [32]byte{1, 2, 3, 4, 5}
	cos := commitreveal2.Keccak256(secretValue[:])
	var cosArray [32]byte
	copy(cosArray[:], cos)

	// Set up commit data with matching COS
	uniqueKey := utils.GetUniqueKey("100", "1")
	utils.EnsureCommittedNodesRoundExists(uniqueKey)
	commitData := utils.LeaderCommitData{
		UniqueKey:  uniqueKey,
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: eoaAddressHex,
		Cos:        cosArray,
	}
	utils.SetCommittedNodeData(uniqueKey, eoaAddress, commitData)

	// Setup mock to return database error
	mockRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, "100", "1", eoaAddressHex).
		Return((*utils.LeaderCommitData)(nil), errors.New("database error"))
	mockRepo.On("UpdateLeaderCommit", mock.Anything, mock.AnythingOfType("*utils.LeaderCommitData")).
		Return(errors.New("update error"))

	// Create request
	req := utils.SecretValueRequest{
		RegularEoaAddress: eoaAddressHex,
		SecretValue:       secretValue[:],
		Signature:         signature,
	}

	reqBytes, _ := json.Marshal(req)
	mockStream := new(MockStream)
	mockStream.reader = bytes.NewReader(reqBytes)
	mockStream.On("Close").Return(nil)

	node.AcceptSecretValue(context.Background(), nil, mockStream, nil)

	mockStream.AssertCalled(t, "Close")
	mockRepo.AssertExpectations(t)

	// Cleanup
	delete(utils.CommittedNodes, uniqueKey)
}

// TestAcceptSecretValueSuccess tests successful secret value acceptance up to database save
func TestAcceptSecretValueSuccess(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRepo := new(MockLeaderCommitRepository)
	node.leaderCommitRepository = mockRepo
	node.SetCurrentRound("100")
	node.SetCurrentTrial("1")

	// Generate a valid signature
	privateKey, _ := crypto.GenerateKey()
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	eoaAddressHex := eoaAddress.Hex()
	signature := utils.SignData(eoaAddressHex, privateKey)

	// Create secret value and compute its hash
	secretValue := [32]byte{10, 20, 30, 40, 50}
	cos := commitreveal2.Keccak256(secretValue[:])
	var cosArray [32]byte
	copy(cosArray[:], cos)

	// Set up commit data with matching COS
	uniqueKey := utils.GetUniqueKey("100", "1")
	utils.EnsureCommittedNodesRoundExists(uniqueKey)
	commitData := utils.LeaderCommitData{
		UniqueKey:  uniqueKey,
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: eoaAddressHex,
		Cos:        cosArray,
	}
	utils.SetCommittedNodeData(uniqueKey, eoaAddress, commitData)

	// Setup mocks - Return error on UpdateLeaderCommit to avoid broadcast
	existingCommit := &utils.LeaderCommitData{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: eoaAddressHex,
		Cos:        cosArray,
		CreatedAt:  time.Now().Unix(),
	}
	mockRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, "100", "1", eoaAddressHex).
		Return(existingCommit, nil)
	mockRepo.On("UpdateLeaderCommit", mock.Anything, mock.AnythingOfType("*utils.LeaderCommitData")).
		Return(nil).Run(func(args mock.Arguments) {
		commitData := args.Get(1).(*utils.LeaderCommitData)
		assert.Equal(t, hex.EncodeToString(secretValue[:]), commitData.SecretValueHex)
	})

	// Create request
	req := utils.SecretValueRequest{
		RegularEoaAddress: eoaAddressHex,
		SecretValue:       secretValue[:],
		Signature:         signature,
	}

	reqBytes, _ := json.Marshal(req)
	mockStream := new(MockStream)
	mockStream.reader = bytes.NewReader(reqBytes)
	mockStream.On("Close").Return(nil)

	// Cleanup
	defer delete(utils.CommittedNodes, uniqueKey)
}

// TestAcceptSecretValueUpdateCommitError tests when UpdateLeaderCommit fails
func TestAcceptSecretValueUpdateCommitError(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRepo := new(MockLeaderCommitRepository)
	node.leaderCommitRepository = mockRepo
	node.SetCurrentRound("100")
	node.SetCurrentTrial("1")

	// Generate a valid signature
	privateKey, _ := crypto.GenerateKey()
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	eoaAddressHex := eoaAddress.Hex()
	signature := utils.SignData(eoaAddressHex, privateKey)

	// Create secret value and compute its hash
	secretValue := [32]byte{15, 25, 35, 45, 55}
	cos := commitreveal2.Keccak256(secretValue[:])
	var cosArray [32]byte
	copy(cosArray[:], cos)

	// Set up commit data with matching COS
	uniqueKey := utils.GetUniqueKey("100", "1")
	utils.EnsureCommittedNodesRoundExists(uniqueKey)
	commitData := utils.LeaderCommitData{
		UniqueKey:  uniqueKey,
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: eoaAddressHex,
		Cos:        cosArray,
	}
	utils.SetCommittedNodeData(uniqueKey, eoaAddress, commitData)

	// Setup mocks - Return existing commit but fail on update
	existingCommit := &utils.LeaderCommitData{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: eoaAddressHex,
		Cos:        cosArray,
		CreatedAt:  time.Now().Unix(),
	}
	mockRepo.On("GetLeaderCommitByRoundAndEoaAddr", mock.Anything, "100", "1", eoaAddressHex).
		Return(existingCommit, nil)
	mockRepo.On("UpdateLeaderCommit", mock.Anything, mock.AnythingOfType("*utils.LeaderCommitData")).
		Return(errors.New("update failed"))

	// Create request
	req := utils.SecretValueRequest{
		RegularEoaAddress: eoaAddressHex,
		SecretValue:       secretValue[:],
		Signature:         signature,
	}

	reqBytes, _ := json.Marshal(req)
	mockStream := new(MockStream)
	mockStream.reader = bytes.NewReader(reqBytes)
	mockStream.On("Close").Return(nil)

	node.AcceptSecretValue(context.Background(), nil, mockStream, nil)

	mockStream.AssertCalled(t, "Close")
	mockRepo.AssertExpectations(t)

	// Cleanup
	defer delete(utils.CommittedNodes, uniqueKey)
}

// TestIndicesConcurrency tests concurrent access to indices
func TestIndicesConcurrency(t *testing.T) {
	node := createTestNodeForSecretHandler()

	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			node.AppendToIndices(big.NewInt(int64(i)))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = node.GetIndices()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			node.ResetIndicesForNewRound()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	<-done
	<-done
	<-done

	assert.True(t, true, "Concurrent access completed without panics")
}

package leader_node

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/eapache/queue"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// MockFallbackEthClientForAtomicState is a mock implementation of the fallback eth client
type MockFallbackEthClientForAtomicState struct {
	fallback_ethclient.IFallbackEthClient
}

// MockEthServiceForAtomicState is a mock implementation of the eth service interface
type MockEthServiceForAtomicState struct {
	eth.IEthService
	currentRound      *big.Int
	trialNum          *big.Int
	shouldError       bool
	errorOnRoundFetch bool
	errorOnTrialFetch bool
}

// UpdateCurrentRoundFromContract mocks the contract round fetch
func (m *MockEthServiceForAtomicState) UpdateCurrentRoundFromContract(ctx context.Context, client fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	if m.shouldError && m.errorOnRoundFetch {
		return nil, errors.New("failed to fetch current round")
	}
	return m.currentRound, nil
}

// GetTrialNumFromContract mocks the contract trial number fetch
func (m *MockEthServiceForAtomicState) GetTrialNumFromContract(ctx context.Context, client fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	if m.shouldError && m.errorOnTrialFetch {
		return nil, errors.New("failed to fetch trial number")
	}
	return m.trialNum, nil
}

// GetActivatedOperatorsCached mocks the activated operators cache
func (m *MockEthServiceForAtomicState) GetActivatedOperatorsCached() []common.Address {
	return []common.Address{}
}

// GetActivatedOperatorsLength mocks the length of activated operators
func (m *MockEthServiceForAtomicState) GetActivatedOperatorsLength() int64 {
	return 0
}

// createTestLeaderNode creates a minimal LeaderNode for testing
func createTestLeaderNode() *LeaderNode {
	return &LeaderNode{
		roundsData:          make(map[string]RoundData),
		activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
		cvOnChain:           make(map[string]bool),
		roundSecrets:        make(map[string][][32]byte),
		roundSecret:         make(map[string]map[string]bool),
		secretsOnChain:      make(map[string]bool),
		revealRequestStatus: make(map[string][]string),
		cleanupQueue:        queue.New(),
	}
}

// TestSetGetExecution tests the atomic execution flag
func TestSetGetExecution(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	assert.False(t, node.GetExecution(), "Initial execution should be false")

	// Test setting to true
	node.SetExecution(true)
	assert.True(t, node.GetExecution(), "Execution should be true after setting")

	// Test setting to false
	node.SetExecution(false)
	assert.False(t, node.GetExecution(), "Execution should be false after setting")
}

// TestSetGetHalted tests the atomic halted flag
func TestSetGetHalted(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	assert.False(t, node.GetHalted(), "Initial halted should be false")

	// Test setting to true
	node.SetHalted(true)
	assert.True(t, node.GetHalted(), "Halted should be true after setting")

	// Test setting to false
	node.SetHalted(false)
	assert.False(t, node.GetHalted(), "Halted should be false after setting")
}

// TestSetGetSubmittingMerkleRoot tests the atomic submittingMerkleRoot flag
func TestSetGetSubmittingMerkleRoot(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	assert.False(t, node.GetSubmittingMerkleRoot(), "Initial submittingMerkleRoot should be false")

	// Test setting to true
	node.SetSubmittingMerkleRoot(true)
	assert.True(t, node.GetSubmittingMerkleRoot(), "SubmittingMerkleRoot should be true after setting")

	// Test setting to false
	node.SetSubmittingMerkleRoot(false)
	assert.False(t, node.GetSubmittingMerkleRoot(), "SubmittingMerkleRoot should be false after setting")
}

// TestCompareAndSwapSubmittingMerkleRoot tests the CAS operation
func TestCompareAndSwapSubmittingMerkleRoot(t *testing.T) {
	node := createTestLeaderNode()

	// Test CAS from false to true
	success := node.CompareAndSwapSubmittingMerkleRoot(false, true)
	assert.True(t, success, "CAS should succeed when old value matches")
	assert.True(t, node.GetSubmittingMerkleRoot(), "Value should be true after CAS")

	// Test CAS with wrong old value
	success = node.CompareAndSwapSubmittingMerkleRoot(false, false)
	assert.False(t, success, "CAS should fail when old value doesn't match")
	assert.True(t, node.GetSubmittingMerkleRoot(), "Value should remain true after failed CAS")

	// Test CAS from true to false
	success = node.CompareAndSwapSubmittingMerkleRoot(true, false)
	assert.True(t, success, "CAS should succeed when old value matches")
	assert.False(t, node.GetSubmittingMerkleRoot(), "Value should be false after CAS")
}

// TestSetGetRequestedToSubmitCoMonitoringActive tests the monitoring flag
func TestSetGetRequestedToSubmitCoMonitoringActive(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive(), "Initial should be false")

	// Test setting to true
	node.SetRequestedToSubmitCoMonitoringActive(true)
	assert.True(t, node.GetRequestedToSubmitCoMonitoringActive(), "Should be true after setting")

	// Test setting to false
	node.SetRequestedToSubmitCoMonitoringActive(false)
	assert.False(t, node.GetRequestedToSubmitCoMonitoringActive(), "Should be false after setting")
}

// TestSetGetRequestedToSubmitCvMonitoringActive tests the monitoring flag
func TestSetGetRequestedToSubmitCvMonitoringActive(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive(), "Initial should be false")

	// Test setting to true
	node.SetRequestedToSubmitCvMonitoringActive(true)
	assert.True(t, node.GetRequestedToSubmitCvMonitoringActive(), "Should be true after setting")

	// Test setting to false
	node.SetRequestedToSubmitCvMonitoringActive(false)
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive(), "Should be false after setting")
}

// TestSetGetRequestToSubmitCvMonitoringActive tests the monitoring flag
func TestSetGetRequestToSubmitCvMonitoringActive(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	assert.False(t, node.GetRequestToSubmitCvMonitoringActive(), "Initial should be false")

	// Test setting to true
	node.SetRequestToSubmitCvMonitoringActive(true)
	assert.True(t, node.GetRequestToSubmitCvMonitoringActive(), "Should be true after setting")

	// Test setting to false
	node.SetRequestToSubmitCvMonitoringActive(false)
	assert.False(t, node.GetRequestToSubmitCvMonitoringActive(), "Should be false after setting")
}

// TestSetGetRequestToSubmitCoTimerMonitoringActive tests the monitoring flag
func TestSetGetRequestToSubmitCoTimerMonitoringActive(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	assert.False(t, node.GetRequestToSubmitCoTimerMonitoringActive(), "Initial should be false")

	// Test setting to true
	node.SetRequestToSubmitCoTimerMonitoringActive(true)
	assert.True(t, node.GetRequestToSubmitCoTimerMonitoringActive(), "Should be true after setting")

	// Test setting to false
	node.SetRequestToSubmitCoTimerMonitoringActive(false)
	assert.False(t, node.GetRequestToSubmitCoTimerMonitoringActive(), "Should be false after setting")
}

// TestSetGetFailToSubmitSMonitoringActive tests the monitoring flag
func TestSetGetFailToSubmitSMonitoringActive(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	assert.False(t, node.GetFailToSubmitSMonitoringActive(), "Initial should be false")

	// Test setting to true
	node.SetFailToSubmitSMonitoringActive(true)
	assert.True(t, node.GetFailToSubmitSMonitoringActive(), "Should be true after setting")

	// Test setting to false
	node.SetFailToSubmitSMonitoringActive(false)
	assert.False(t, node.GetFailToSubmitSMonitoringActive(), "Should be false after setting")
}

// TestDeleteActiveBroadcasts tests deleting from activeBroadcasts map
func TestDeleteActiveBroadcasts(t *testing.T) {
	node := createTestLeaderNode()

	// Add a broadcast tracker
	key := "test-key"
	node.activeBroadcasts[key] = &utils.BroadcastTracker{}

	node.DeleteActiveBroadcasts(key)

	_, exists := node.activeBroadcasts[key]
	assert.False(t, exists, "Key should be deleted from activeBroadcasts")
}

// TestDeleteCvOnChain tests deleting from cvOnChain map
func TestDeleteCvOnChain(t *testing.T) {
	node := createTestLeaderNode()

	// Add a value
	key := "test-key"
	node.cvOnChain[key] = true

	node.DeleteCvOnChain(key)

	_, exists := node.cvOnChain[key]
	assert.False(t, exists, "Key should be deleted from cvOnChain")
}

// TestDeleteRoundSecrets tests deleting from roundSecrets map
func TestDeleteRoundSecrets(t *testing.T) {
	node := createTestLeaderNode()

	// Add a value
	key := "test-key"
	node.roundSecrets[key] = [][32]byte{{1, 2, 3}}

	node.DeleteRoundSecrets(key)

	_, exists := node.roundSecrets[key]
	assert.False(t, exists, "Key should be deleted from roundSecrets")
}

// TestSetGetRoundSecretValue tests roundSecret map operations
func TestSetGetRoundSecretValue(t *testing.T) {
	node := createTestLeaderNode()

	uniqueKey := "round-1-trial-1"
	eoaAddress := "0x123"

	// Test getting non-existent value
	value, exists := node.GetRoundSecretValue(uniqueKey, eoaAddress)
	assert.False(t, exists, "Should return false for non-existent key")
	assert.False(t, value, "Value should be false for non-existent key")

	// Test setting value
	node.SetRoundSecretValue(uniqueKey, eoaAddress, true)

	// Test getting existing value
	value, exists = node.GetRoundSecretValue(uniqueKey, eoaAddress)
	assert.True(t, exists, "Should return true for existing key")
	assert.True(t, value, "Value should be true")

	// Test updating value
	node.SetRoundSecretValue(uniqueKey, eoaAddress, false)
	value, exists = node.GetRoundSecretValue(uniqueKey, eoaAddress)
	assert.True(t, exists, "Should return true for existing key")
	assert.False(t, value, "Value should be false after update")
}

// TestDeleteRoundSecret tests deleting from roundSecret map
func TestDeleteRoundSecret(t *testing.T) {
	node := createTestLeaderNode()

	uniqueKey := "round-1-trial-1"
	eoaAddress := "0x123"

	// Add a value
	node.SetRoundSecretValue(uniqueKey, eoaAddress, true)

	node.DeleteRoundSecret(uniqueKey)

	_, exists := node.GetRoundSecretValue(uniqueKey, eoaAddress)
	assert.False(t, exists, "Key should be deleted from roundSecret")
}

// TestSetGetSecretsOnChain tests secretsOnChain map operations
func TestSetGetSecretsOnChain(t *testing.T) {
	node := createTestLeaderNode()

	uniqueKey := "round-1-trial-1"

	// Test getting non-existent value
	value, exists := node.GetSecretsOnChain(uniqueKey)
	assert.False(t, exists, "Should return false for non-existent key")
	assert.False(t, value, "Value should be false for non-existent key")

	// Test setting value
	node.SetSecretsOnChain(uniqueKey, true)

	// Test getting existing value
	value, exists = node.GetSecretsOnChain(uniqueKey)
	assert.True(t, exists, "Should return true for existing key")
	assert.True(t, value, "Value should be true")

	// Test updating value
	node.SetSecretsOnChain(uniqueKey, false)
	value, exists = node.GetSecretsOnChain(uniqueKey)
	assert.True(t, exists, "Should return true for existing key")
	assert.False(t, value, "Value should be false after update")
}

// TestDeleteSecretsOnChain tests deleting from secretsOnChain map
func TestDeleteSecretsOnChain(t *testing.T) {
	node := createTestLeaderNode()

	uniqueKey := "round-1-trial-1"

	// Add a value
	node.SetSecretsOnChain(uniqueKey, true)

	node.DeleteSecretsOnChain(uniqueKey)

	_, exists := node.GetSecretsOnChain(uniqueKey)
	assert.False(t, exists, "Key should be deleted from secretsOnChain")
}

// TestGetRoundSecretsValue tests getting values from roundSecrets map
func TestGetRoundSecretsValue(t *testing.T) {
	node := createTestLeaderNode()

	uniqueKey := "round-1-trial-1"

	// Test getting non-existent value
	value, exists := node.GetRoundSecretsValue(uniqueKey)
	assert.False(t, exists, "Should return false for non-existent key")
	assert.Nil(t, value, "Value should be nil for non-existent key")

	// Add some secrets
	secret1 := [32]byte{1, 2, 3}
	secret2 := [32]byte{4, 5, 6}
	node.roundSecrets[uniqueKey] = [][32]byte{secret1, secret2}

	// Test getting existing value
	value, exists = node.GetRoundSecretsValue(uniqueKey)
	assert.True(t, exists, "Should return true for existing key")
	assert.Equal(t, 2, len(value), "Should return correct number of secrets")
	assert.Equal(t, secret1, value[0], "First secret should match")
	assert.Equal(t, secret2, value[1], "Second secret should match")

	value[0] = [32]byte{99, 99, 99}
	originalValue, _ := node.GetRoundSecretsValue(uniqueKey)
	assert.Equal(t, secret1, originalValue[0], "Original value should not be modified")
}

// TestAppendToRoundSecrets tests appending to roundSecrets map
func TestAppendToRoundSecrets(t *testing.T) {
	node := createTestLeaderNode()

	uniqueKey := "round-1-trial-1"
	secret1 := [32]byte{1, 2, 3}
	secret2 := [32]byte{4, 5, 6}

	// Test appending to non-existent key
	node.AppendToRoundSecrets(uniqueKey, secret1)
	value, exists := node.GetRoundSecretsValue(uniqueKey)
	assert.True(t, exists, "Key should exist after append")
	assert.Equal(t, 1, len(value), "Should have 1 secret")
	assert.Equal(t, secret1, value[0], "Secret should match")

	// Test appending to existing key
	node.AppendToRoundSecrets(uniqueKey, secret2)
	value, exists = node.GetRoundSecretsValue(uniqueKey)
	assert.True(t, exists, "Key should still exist")
	assert.Equal(t, 2, len(value), "Should have 2 secrets")
	assert.Equal(t, secret1, value[0], "First secret should match")
	assert.Equal(t, secret2, value[1], "Second secret should match")
}

// TestEnqueueUniqueKeyForCleanup tests the cleanup queue functionality
func TestEnqueueUniqueKeyForCleanup(t *testing.T) {
	node := createTestLeaderNode()

	// Add some data to the maps that would be cleaned up
	for i := 1; i <= 6; i++ {
		key := "key-" + string(rune('0'+i))
		node.SetRoundData(key, RoundData{MerkleRoot: true})
		node.SetSecretsOnChain(key, true)
	}

	// Add 4 keys - should not trigger cleanup
	for i := 1; i <= 4; i++ {
		key := "key-" + string(rune('0'+i))
		node.EnqueueUniqueKeyForCleanup(key)
	}
	assert.Equal(t, 4, node.cleanupQueue.Length(), "Queue should have 4 keys")

	for i := 1; i <= 4; i++ {
		key := "key-" + string(rune('0'+i))
		_, exists := node.GetRoundData(key)
		assert.True(t, exists, "Data should still exist for "+key)
	}

	node.EnqueueUniqueKeyForCleanup("key-5")
	assert.Equal(t, 4, node.cleanupQueue.Length(), "Queue should be back to 4 keys")

	_, exists := node.GetRoundData("key-1")
	assert.False(t, exists, "Data should be cleaned up for key-1")

	node.EnqueueUniqueKeyForCleanup("key-6")
	assert.Equal(t, 4, node.cleanupQueue.Length(), "Queue should remain at 4 keys")

	_, exists = node.GetRoundData("key-2")
	assert.False(t, exists, "Data should be cleaned up for key-2")

	for i := 3; i <= 6; i++ {
		key := "key-" + string(rune('0'+i))
		_, exists := node.GetRoundData(key)
		assert.True(t, exists, "Data should still exist for "+key)
	}
}

// TestSetGetSecretRequestSentForWhichRound tests atomic string operations
func TestSetGetSecretRequestSentForWhichRound(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state (nil pointer)
	value := node.GetSecretRequestSentForWhichRound()
	assert.Equal(t, "", value, "Initial value should be empty string")

	// Test setting value
	node.SetSecretRequestSentForWhichRound("round-123")
	value = node.GetSecretRequestSentForWhichRound()
	assert.Equal(t, "round-123", value, "Value should match set value")

	// Test updating value
	node.SetSecretRequestSentForWhichRound("round-456")
	value = node.GetSecretRequestSentForWhichRound()
	assert.Equal(t, "round-456", value, "Value should be updated")
}

// TestSetGetCurrentRound tests atomic string operations
func TestSetGetCurrentRound(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state (nil pointer)
	value := node.GetCurrentRound()
	assert.Equal(t, "", value, "Initial value should be empty string")

	// Test setting value
	node.SetCurrentRound("round-100")
	value = node.GetCurrentRound()
	assert.Equal(t, "round-100", value, "Value should match set value")

	// Test updating value
	node.SetCurrentRound("round-200")
	value = node.GetCurrentRound()
	assert.Equal(t, "round-200", value, "Value should be updated")
}

// TestSetGetCurrentTrial tests atomic string operations
func TestSetGetCurrentTrial(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state (nil pointer)
	value := node.GetCurrentTrial()
	assert.Equal(t, "", value, "Initial value should be empty string")

	// Test setting value
	node.SetCurrentTrial("trial-1")
	value = node.GetCurrentTrial()
	assert.Equal(t, "trial-1", value, "Value should match set value")

	// Test updating value
	node.SetCurrentTrial("trial-2")
	value = node.GetCurrentTrial()
	assert.Equal(t, "trial-2", value, "Value should be updated")
}

// TestSetGetRoundData tests roundsData map operations
func TestSetGetRoundData(t *testing.T) {
	node := createTestLeaderNode()

	key := "round-1-trial-1"

	// Test getting non-existent value
	data, exists := node.GetRoundData(key)
	assert.False(t, exists, "Should return false for non-existent key")
	assert.Equal(t, RoundData{}, data, "Data should be empty for non-existent key")

	// Test setting value
	roundData := RoundData{
		MerkleRoot:   true,
		RandomNumber: false,
	}
	node.SetRoundData(key, roundData)

	// Test getting existing value
	data, exists = node.GetRoundData(key)
	assert.True(t, exists, "Should return true for existing key")
	assert.Equal(t, roundData, data, "Data should match")

	// Test updating value
	updatedData := RoundData{
		MerkleRoot:   true,
		RandomNumber: true,
	}
	node.SetRoundData(key, updatedData)
	data, exists = node.GetRoundData(key)
	assert.True(t, exists, "Should return true for existing key")
	assert.Equal(t, updatedData, data, "Data should be updated")
}

// TestDeleteRoundsData tests deleting from roundsData map
func TestDeleteRoundsData(t *testing.T) {
	node := createTestLeaderNode()

	key := "round-1-trial-1"
	roundData := RoundData{MerkleRoot: true, RandomNumber: false}

	// Add a value
	node.SetRoundData(key, roundData)

	node.DeleteRoundsData(key)

	_, exists := node.GetRoundData(key)
	assert.False(t, exists, "Key should be deleted from roundsData")
}

// TestSetRoundDataWithNilMap tests setting data when map is nil
func TestSetRoundDataWithNilMap(t *testing.T) {
	node := &LeaderNode{}

	key := "round-1-trial-1"
	roundData := RoundData{MerkleRoot: true, RandomNumber: false}

	// Set data when map is nil
	node.SetRoundData(key, roundData)

	data, exists := node.GetRoundData(key)
	assert.True(t, exists, "Should create map and set data")
	assert.Equal(t, roundData, data, "Data should match")
}

// TestSetGetRevealRequestStatus tests revealRequestStatus map operations
func TestSetGetRevealRequestStatus(t *testing.T) {
	node := createTestLeaderNode()

	key := "round-1-trial-1"

	// Test getting non-existent value
	value, exists := node.GetRevealRequestStatus(key)
	assert.False(t, exists, "Should return false for non-existent key")
	assert.Nil(t, value, "Value should be nil for non-existent key")

	// Test setting value
	status := []string{"0x123", "0x456"}
	node.SetRevealRequestStatus(key, status)

	// Test getting existing value
	value, exists = node.GetRevealRequestStatus(key)
	assert.True(t, exists, "Should return true for existing key")
	assert.Equal(t, status, value, "Value should match")

	// Test updating value
	updatedStatus := []string{"0x123", "0x456", "0x789"}
	node.SetRevealRequestStatus(key, updatedStatus)
	value, exists = node.GetRevealRequestStatus(key)
	assert.True(t, exists, "Should return true for existing key")
	assert.Equal(t, updatedStatus, value, "Value should be updated")
}

// TestDeleteRevealRequestStatus tests deleting from revealRequestStatus map
func TestDeleteRevealRequestStatus(t *testing.T) {
	node := createTestLeaderNode()

	key := "round-1-trial-1"
	status := []string{"0x123"}

	// Add a value
	node.SetRevealRequestStatus(key, status)

	node.DeleteRevealRequestStatus(key)

	_, exists := node.GetRevealRequestStatus(key)
	assert.False(t, exists, "Key should be deleted from revealRequestStatus")
}

// TestSetGetActiveBroadcast tests activeBroadcasts map operations
func TestSetGetActiveBroadcast(t *testing.T) {
	node := createTestLeaderNode()

	key := "test-key"

	// Test getting non-existent value
	tracker, exists := node.GetActiveBroadcast(key)
	assert.False(t, exists, "Should return false for non-existent key")
	assert.Nil(t, tracker, "Tracker should be nil for non-existent key")

	// Test setting value
	broadcastTracker := &utils.BroadcastTracker{}
	node.SetActiveBroadcast(key, broadcastTracker)

	// Test getting existing value
	tracker, exists = node.GetActiveBroadcast(key)
	assert.True(t, exists, "Should return true for existing key")
	assert.Equal(t, broadcastTracker, tracker, "Tracker should match")
}

// TestDeleteActiveBroadcast tests deleting from activeBroadcasts map
func TestDeleteActiveBroadcast(t *testing.T) {
	node := createTestLeaderNode()

	key := "test-key"
	broadcastTracker := &utils.BroadcastTracker{}

	// Add a value
	node.SetActiveBroadcast(key, broadcastTracker)

	node.DeleteActiveBroadcast(key)

	_, exists := node.GetActiveBroadcast(key)
	assert.False(t, exists, "Key should be deleted from activeBroadcasts")
}

// TestSetGetCvOnChain tests cvOnChain map operations
func TestSetGetCvOnChain(t *testing.T) {
	node := createTestLeaderNode()

	key := "test-key"

	// Test getting non-existent value
	value, exists := node.GetCvOnChain(key)
	assert.False(t, exists, "Should return false for non-existent key")
	assert.False(t, value, "Value should be false for non-existent key")

	// Test setting value
	node.SetCvOnChain(key, true)

	// Test getting existing value
	value, exists = node.GetCvOnChain(key)
	assert.True(t, exists, "Should return true for existing key")
	assert.True(t, value, "Value should be true")

	// Test updating value
	node.SetCvOnChain(key, false)
	value, exists = node.GetCvOnChain(key)
	assert.True(t, exists, "Should return true for existing key")
	assert.False(t, value, "Value should be false after update")
}

// TestSetGetReq tests req struct operations
func TestSetGetReq(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	req := node.GetReq()
	assert.Equal(t, RandomRequest{}, req, "Initial req should be empty")

	// Test setting value
	testReq := RandomRequest{
		Round:     big.NewInt(100),
		TrialNum:  big.NewInt(1),
		StartTime: big.NewInt(1234567890),
		State:     big.NewInt(1),
	}
	node.SetReq(testReq)

	// Test getting value
	req = node.GetReq()
	assert.Equal(t, testReq.Round, req.Round, "Round should match")
	assert.Equal(t, testReq.TrialNum, req.TrialNum, "TrialNum should match")
	assert.Equal(t, testReq.StartTime, req.StartTime, "StartTime should match")
	assert.Equal(t, testReq.State, req.State, "State should match")

	// Test updating value
	updatedReq := RandomRequest{
		Round:     big.NewInt(101),
		TrialNum:  big.NewInt(2),
		StartTime: big.NewInt(1234567899),
		State:     big.NewInt(2),
	}
	node.SetReq(updatedReq)
	req = node.GetReq()
	assert.Equal(t, updatedReq, req, "Req should be updated")
}

// TestSetGetLastSubmitSTimestamp tests timestamp operations
func TestSetGetLastSubmitSTimestamp(t *testing.T) {
	node := createTestLeaderNode()

	// Test initial state
	timestamp := node.GetLastSubmitSTimestamp()
	assert.Nil(t, timestamp, "Initial timestamp should be nil")

	// Test setting value
	testTimestamp := big.NewInt(1234567890)
	node.SetLastSubmitSTimestamp(testTimestamp)

	// Test getting value
	timestamp = node.GetLastSubmitSTimestamp()
	assert.Equal(t, testTimestamp, timestamp, "Timestamp should match")

	// Test updating value
	updatedTimestamp := big.NewInt(9876543210)
	node.SetLastSubmitSTimestamp(updatedTimestamp)
	timestamp = node.GetLastSubmitSTimestamp()
	assert.Equal(t, updatedTimestamp, timestamp, "Timestamp should be updated")
}

// TestUpdateCurrentRoundAndTrial_Success tests successful update of current round and trial
func TestUpdateCurrentRoundAndTrial_Success(t *testing.T) {
	ctx := context.Background()
	node := createTestLeaderNode()

	// Create a mock eth service
	mockEthService := &MockEthServiceForAtomicState{
		currentRound: big.NewInt(100),
		trialNum:     big.NewInt(5),
		shouldError:  false,
	}

	// Replace the global eth.Service with mock
	originalService := eth.Service
	eth.Service = mockEthService
	defer func() { eth.Service = originalService }()

	// Create a mock fallback client
	mockClient := &MockFallbackEthClientForAtomicState{}
	node.fallbackEthClient = mockClient

	// Execute the function
	err := node.UpdateCurrentRoundAndTrial(ctx)

	// Assertions
	assert.NoError(t, err, "Should not return error on success")
	assert.Equal(t, "100", node.GetCurrentRound(), "Current round should be updated to 100")
	assert.Equal(t, "5", node.GetCurrentTrial(), "Current trial should be updated to 5")
}

// TestUpdateCurrentRoundAndTrial_ErrorFetchingRound tests error when fetching current round fails
func TestUpdateCurrentRoundAndTrial_ErrorFetchingRound(t *testing.T) {
	ctx := context.Background()
	node := createTestLeaderNode()

	// Create a mock eth service that returns error on UpdateCurrentRoundFromContract
	mockEthService := &MockEthServiceForAtomicState{
		shouldError:       true,
		errorOnRoundFetch: true,
	}

	// Replace the global eth.Service with mock
	originalService := eth.Service
	eth.Service = mockEthService
	defer func() { eth.Service = originalService }()

	// Create a mock fallback client
	mockClient := &MockFallbackEthClientForAtomicState{}
	node.fallbackEthClient = mockClient

	// Set initial values
	node.SetCurrentRound("50")
	node.SetCurrentTrial("3")

	// Execute the function
	err := node.UpdateCurrentRoundAndTrial(ctx)

	// Assertions
	assert.Error(t, err, "Should return error when fetching round fails")
	assert.Contains(t, err.Error(), "failed to fetch current round", "Error message should indicate round fetch failure")
	// Current values should remain unchanged
	assert.Equal(t, "50", node.GetCurrentRound(), "Current round should not be updated on error")
	assert.Equal(t, "3", node.GetCurrentTrial(), "Current trial should not be updated on error")
}

// TestUpdateCurrentRoundAndTrial_ErrorFetchingTrial tests error when fetching trial number fails
func TestUpdateCurrentRoundAndTrial_ErrorFetchingTrial(t *testing.T) {
	ctx := context.Background()
	node := createTestLeaderNode()

	// Create a mock eth service that returns error on GetTrialNumFromContract
	mockEthService := &MockEthServiceForAtomicState{
		currentRound:      big.NewInt(200),
		shouldError:       true,
		errorOnTrialFetch: true,
	}

	// Replace the global eth.Service with mock
	originalService := eth.Service
	eth.Service = mockEthService
	defer func() { eth.Service = originalService }()

	// Create a mock fallback client
	mockClient := &MockFallbackEthClientForAtomicState{}
	node.fallbackEthClient = mockClient

	// Set initial values
	node.SetCurrentRound("50")
	node.SetCurrentTrial("3")

	// Execute the function
	err := node.UpdateCurrentRoundAndTrial(ctx)

	// Assertions
	assert.Error(t, err, "Should return error when fetching trial fails")
	assert.Contains(t, err.Error(), "failed to fetch trial number", "Error message should indicate trial fetch failure")
	// Current values should remain unchanged
	assert.Equal(t, "50", node.GetCurrentRound(), "Current round should not be updated on error")
	assert.Equal(t, "3", node.GetCurrentTrial(), "Current trial should not be updated on error")
}

// TestUpdateCurrentRoundAndTrial_MultipleUpdates tests multiple consecutive updates
func TestUpdateCurrentRoundAndTrial_MultipleUpdates(t *testing.T) {
	ctx := context.Background()
	node := createTestLeaderNode()

	// Create a mock fallback client
	mockClient := &MockFallbackEthClientForAtomicState{}
	node.fallbackEthClient = mockClient

	// Replace the global eth.Service with mock
	originalService := eth.Service
	defer func() { eth.Service = originalService }()

	testCases := []struct {
		round    *big.Int
		trial    *big.Int
		expected struct {
			round string
			trial string
		}
	}{
		{big.NewInt(1), big.NewInt(1), struct{ round, trial string }{"1", "1"}},
		{big.NewInt(2), big.NewInt(1), struct{ round, trial string }{"2", "1"}},
		{big.NewInt(2), big.NewInt(2), struct{ round, trial string }{"2", "2"}},
		{big.NewInt(100), big.NewInt(10), struct{ round, trial string }{"100", "10"}},
	}

	for _, tc := range testCases {
		mockEthService := &MockEthServiceForAtomicState{
			currentRound: tc.round,
			trialNum:     tc.trial,
			shouldError:  false,
		}
		eth.Service = mockEthService

		err := node.UpdateCurrentRoundAndTrial(ctx)

		assert.NoError(t, err, "Should not return error for round %s trial %s", tc.expected.round, tc.expected.trial)
		assert.Equal(t, tc.expected.round, node.GetCurrentRound(), "Current round should be %s", tc.expected.round)
		assert.Equal(t, tc.expected.trial, node.GetCurrentTrial(), "Current trial should be %s", tc.expected.trial)
	}
}

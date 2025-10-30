package leader_node

import (
	"context"
	"log"
	"math/big"
	"sync/atomic"
	"unsafe"

	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

// ============================================================================
// ATOMIC BOOLEAN VARIABLES (using int32: 0 = false, 1 = true)
// ============================================================================

// Execution state management
func (n *LeaderNode) SetExecution(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.execution, val)
}

func (n *LeaderNode) GetExecution() bool {
	return atomic.LoadInt32(&n.execution) == 1
}

// Halted state management (already using atomic in original code)
func (n *LeaderNode) SetHalted(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.halted, val)
}

func (n *LeaderNode) GetHalted() bool {
	return atomic.LoadInt32(&n.halted) == 1
}

// Getter and setter functions for submittingMerkleRoot atomic variable
func (n *LeaderNode) SetSubmittingMerkleRoot(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.submittingMerkleRoot, val)
}

func (n *LeaderNode) GetSubmittingMerkleRoot() bool {
	return atomic.LoadInt32(&n.submittingMerkleRoot) == 1
}

// CompareAndSwapSubmittingMerkleRoot performs atomic compare-and-swap operation
func (n *LeaderNode) CompareAndSwapSubmittingMerkleRoot(old, new bool) bool {
	var oldVal, newVal int32
	if old {
		oldVal = 1
	}
	if new {
		newVal = 1
	}
	return atomic.CompareAndSwapInt32(&n.submittingMerkleRoot, oldVal, newVal)
}

// RequestedToSubmitCo monitoring management
func (n *LeaderNode) SetRequestedToSubmitCoMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.requestedToSubmitCoMonitoringActive, val)
}

func (n *LeaderNode) GetRequestedToSubmitCoMonitoringActive() bool {
	return atomic.LoadInt32(&n.requestedToSubmitCoMonitoringActive) == 1
}

// RequestedToSubmitCv monitoring management
func (n *LeaderNode) SetRequestedToSubmitCvMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.requestedToSubmitCvMonitoringActive, val)
}

func (n *LeaderNode) GetRequestedToSubmitCvMonitoringActive() bool {
	return atomic.LoadInt32(&n.requestedToSubmitCvMonitoringActive) == 1
}

// RequestToSubmitCv monitoring management
func (n *LeaderNode) SetRequestToSubmitCvMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.requestToSubmitCvMonitoringActive, val)
}

func (n *LeaderNode) GetRequestToSubmitCvMonitoringActive() bool {
	return atomic.LoadInt32(&n.requestToSubmitCvMonitoringActive) == 1
}

// RequestToSubmitCo timer monitoring management
func (n *LeaderNode) SetRequestToSubmitCoTimerMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.requestToSubmitCoTimerMonitoringActive, val)
}

func (n *LeaderNode) GetRequestToSubmitCoTimerMonitoringActive() bool {
	return atomic.LoadInt32(&n.requestToSubmitCoTimerMonitoringActive) == 1
}

// ============================================================================
// MAP CLEANUP FUNCTIONS
// ============================================================================

// DeleteActiveBroadcasts deletes activeBroadcasts entry for uniqueKey
func (n *LeaderNode) DeleteActiveBroadcasts(uniqueKey string) {
	n.activeBroadcastsMu.Lock()
	defer n.activeBroadcastsMu.Unlock()
	delete(n.activeBroadcasts, uniqueKey)
}

// DeleteCvOnChain deletes CvOnChain entry for uniqueKey
func (n *LeaderNode) DeleteCvOnChain(uniqueKey string) {
	n.cvOnChainMu.Lock()
	defer n.cvOnChainMu.Unlock()
	delete(n.cvOnChain, uniqueKey)
}

// DeleteRoundSecrets deletes RoundSecrets entry for uniqueKey
func (n *LeaderNode) DeleteRoundSecrets(uniqueKey string) {
	n.secretMapsMutex.Lock()
	defer n.secretMapsMutex.Unlock()
	delete(n.roundSecrets, uniqueKey)
}

// ============================================================================
// ROUNDSECRET MAP MANAGEMENT
// ============================================================================

// SetRoundSecretValue sets a value in the roundSecret map for a specific uniqueKey and EOA
func (n *LeaderNode) SetRoundSecretValue(uniqueKey, eoaAddress string, value bool) {
	n.secretMapsMutex.Lock()
	defer n.secretMapsMutex.Unlock()
	if n.roundSecret[uniqueKey] == nil {
		n.roundSecret[uniqueKey] = make(map[string]bool)
	}
	n.roundSecret[uniqueKey][eoaAddress] = value
}

// GetRoundSecretValue gets a value from the roundSecret map for a specific uniqueKey and EOA
func (n *LeaderNode) GetRoundSecretValue(uniqueKey, eoaAddress string) (bool, bool) {
	n.secretMapsMutex.RLock()
	defer n.secretMapsMutex.RUnlock()
	if n.roundSecret[uniqueKey] == nil {
		return false, false
	}
	value, exists := n.roundSecret[uniqueKey][eoaAddress]
	return value, exists
}

// DeleteRoundSecret deletes roundSecret entry for uniqueKey
func (n *LeaderNode) DeleteRoundSecret(uniqueKey string) {
	n.secretMapsMutex.Lock()
	defer n.secretMapsMutex.Unlock()
	delete(n.roundSecret, uniqueKey)
}

// ============================================================================
// SECRETSONCHAIN MAP MANAGEMENT
// ============================================================================

// SetSecretsOnChain sets a value in the secretsOnChain map for a specific uniqueKey
func (n *LeaderNode) SetSecretsOnChain(uniqueKey string, value bool) {
	n.secretsOnChainMu.Lock()
	defer n.secretsOnChainMu.Unlock()
	n.secretsOnChain[uniqueKey] = value
}

// GetSecretsOnChain gets a value from the secretsOnChain map for a specific uniqueKey
func (n *LeaderNode) GetSecretsOnChain(uniqueKey string) (bool, bool) {
	n.secretsOnChainMu.RLock()
	defer n.secretsOnChainMu.RUnlock()
	value, exists := n.secretsOnChain[uniqueKey]
	return value, exists
}

// DeleteSecretsOnChain deletes secretsOnChain entry for uniqueKey
func (n *LeaderNode) DeleteSecretsOnChain(uniqueKey string) {
	n.secretsOnChainMu.Lock()
	defer n.secretsOnChainMu.Unlock()
	delete(n.secretsOnChain, uniqueKey)
}

// ============================================================================
// ROUNDSECRETS MAP MANAGEMENT
// ============================================================================

// GetRoundSecretsValue gets a value from the RoundSecrets map for a specific uniqueKey
func (n *LeaderNode) GetRoundSecretsValue(uniqueKey string) ([][32]byte, bool) {
	n.secretMapsMutex.RLock()
	defer n.secretMapsMutex.RUnlock()
	value, exists := n.roundSecrets[uniqueKey]
	if !exists {
		return nil, false
	}
	// Return a copy to prevent external modifications
	result := make([][32]byte, len(value))
	copy(result, value)
	return result, true
}

// AppendToRoundSecrets appends a secret value to the RoundSecrets map for a specific uniqueKey
func (n *LeaderNode) AppendToRoundSecrets(uniqueKey string, secretValue [32]byte) {
	n.secretMapsMutex.Lock()
	defer n.secretMapsMutex.Unlock()
	if n.roundSecrets[uniqueKey] == nil {
		n.roundSecrets[uniqueKey] = make([][32]byte, 0)
	}
	n.roundSecrets[uniqueKey] = append(n.roundSecrets[uniqueKey], secretValue)
}

// EnqueueUniqueKeyForCleanup adds a uniqueKey to the cleanup queue. If the
// queue size reaches 5 or more, it pops the oldest uniqueKey and cleans it up.
func (n *LeaderNode) EnqueueUniqueKeyForCleanup(uniqueKey string) {
	n.cleanupQueueMu.Lock()

	// enqueue
	n.cleanupQueue.Add(uniqueKey)
	length := n.cleanupQueue.Length()
	// if size >= 5, pop oldest and cleanup until size reaches 4
	if length >= 5 {
		for n.cleanupQueue.Length() >= 5 {
			oldest := n.cleanupQueue.Remove().(string)
			// perform cleanup
			n.CleanupRoundDataByUniqueKey(oldest)
		}
		n.cleanupQueueMu.Unlock()
		return
	}
	n.cleanupQueueMu.Unlock()

}

// FailToSubmitS monitoring management
func (n *LeaderNode) SetFailToSubmitSMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.failToSubmitSMonitoringActive, val)
}

func (n *LeaderNode) GetFailToSubmitSMonitoringActive() bool {
	return atomic.LoadInt32(&n.failToSubmitSMonitoringActive) == 1
}

// ============================================================================
// ATOMIC STRING VARIABLES (using unsafe.Pointer)
// ============================================================================

// SecretRequestSentForWhichRound management
func (n *LeaderNode) SetSecretRequestSentForWhichRound(value string) {
	atomic.StorePointer(&n.secretRequestSentForWhichRound, unsafe.Pointer(&value))
}

func (n *LeaderNode) GetSecretRequestSentForWhichRound() string {
	ptr := atomic.LoadPointer(&n.secretRequestSentForWhichRound)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// CurrentRound management
func (n *LeaderNode) SetCurrentRound(value string) {
	atomic.StorePointer(&n.currentRound, unsafe.Pointer(&value))
}

func (n *LeaderNode) GetCurrentRound() string {
	ptr := atomic.LoadPointer(&n.currentRound)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// CurrentTrial management
func (n *LeaderNode) SetCurrentTrial(value string) {
	atomic.StorePointer(&n.currentTrial, unsafe.Pointer(&value))
}

func (n *LeaderNode) GetCurrentTrial() string {
	ptr := atomic.LoadPointer(&n.currentTrial)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// ============================================================================
// MUTEX-PROTECTED MAP VARIABLES
// ============================================================================

// RoundsData map management
func (n *LeaderNode) SetRoundData(key string, data RoundData) {
	n.roundsDataMu.Lock()
	defer n.roundsDataMu.Unlock()
	if n.roundsData == nil {
		n.roundsData = make(map[string]RoundData)
	}
	n.roundsData[key] = data
}

func (n *LeaderNode) GetRoundData(key string) (RoundData, bool) {
	n.roundsDataMu.RLock()
	defer n.roundsDataMu.RUnlock()
	data, exists := n.roundsData[key]
	return data, exists
}

func (n *LeaderNode) DeleteRoundsData(key string) {
	n.roundsDataMu.Lock()
	defer n.roundsDataMu.Unlock()
	delete(n.roundsData, key)
}

// RevealRequestStatus map management
func (n *LeaderNode) SetRevealRequestStatus(key string, value []string) {
	n.revealRequestStatusMu.Lock()
	defer n.revealRequestStatusMu.Unlock()
	n.revealRequestStatus[key] = value
}

func (n *LeaderNode) GetRevealRequestStatus(key string) ([]string, bool) {
	n.revealRequestStatusMu.RLock()
	defer n.revealRequestStatusMu.RUnlock()
	value, exists := n.revealRequestStatus[key]
	return value, exists
}

func (n *LeaderNode) DeleteRevealRequestStatus(key string) {
	n.revealRequestStatusMu.Lock()
	defer n.revealRequestStatusMu.Unlock()
	delete(n.revealRequestStatus, key)
}

// ActiveBroadcasts map management
func (n *LeaderNode) SetActiveBroadcast(key string, tracker *utils.BroadcastTracker) {
	n.activeBroadcastsMu.Lock()
	defer n.activeBroadcastsMu.Unlock()
	n.activeBroadcasts[key] = tracker
}

func (n *LeaderNode) GetActiveBroadcast(key string) (*utils.BroadcastTracker, bool) {
	n.activeBroadcastsMu.RLock()
	defer n.activeBroadcastsMu.RUnlock()
	tracker, exists := n.activeBroadcasts[key]
	return tracker, exists
}

func (n *LeaderNode) DeleteActiveBroadcast(key string) {
	n.activeBroadcastsMu.Lock()
	defer n.activeBroadcastsMu.Unlock()
	delete(n.activeBroadcasts, key)
}

// CvOnChain map management
func (n *LeaderNode) SetCvOnChain(key string, value bool) {
	n.cvOnChainMu.Lock()
	defer n.cvOnChainMu.Unlock()
	n.cvOnChain[key] = value
}

func (n *LeaderNode) GetCvOnChain(key string) (bool, bool) {
	n.cvOnChainMu.RLock()
	defer n.cvOnChainMu.RUnlock()
	value, exists := n.cvOnChain[key]
	return value, exists
}

// ============================================================================
// MUTEX-PROTECTED STRUCT VARIABLES
// ============================================================================

// Req struct management
func (n *LeaderNode) SetReq(req RandomRequest) {
	n.reqMu.Lock()
	defer n.reqMu.Unlock()
	n.req = req
}

func (n *LeaderNode) GetReq() RandomRequest {
	n.reqMu.RLock()
	defer n.reqMu.RUnlock()
	return n.req
}

// LastSubmitS timestamp management
func (n *LeaderNode) SetLastSubmitSTimestamp(timestamp *big.Int) {
	n.timestampMu.Lock()
	defer n.timestampMu.Unlock()
	n.lastSubmitSTimestamp = timestamp
}

func (n *LeaderNode) GetLastSubmitSTimestamp() *big.Int {
	n.timestampMu.RLock()
	defer n.timestampMu.RUnlock()
	return n.lastSubmitSTimestamp
}

// ============================================================================
// CONTRACT INTEGRATION FUNCTIONS
// ============================================================================

// UpdateCurrentRoundFromContract fetches the current round from the contract and updates local state
func (n *LeaderNode) UpdateCurrentRoundAndTrial(ctx context.Context) error {
	currentRound, err := eth.Service.UpdateCurrentRoundFromContract(ctx, n.fallbackEthClient)
	if err != nil {
		log.Printf("failed to fetch current round from contract: %v", err)
		return err
	}

	trialNumBig, err := eth.Service.GetTrialNumFromContract(ctx, n.fallbackEthClient, currentRound)
	if err != nil {
		log.Printf("failed to fetch trial number for round %s: %v", trialNumBig.String(), err)
		return err
	}

	// Update the local current round state
	n.SetCurrentRound(currentRound.String())
	n.SetCurrentTrial(trialNumBig.String())
	return nil
}

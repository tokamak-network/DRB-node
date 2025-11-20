package regular_node

import (
	"crypto/ecdsa"
	"math/big"
	"sync/atomic"
	"unsafe"
)

// ============================================================================
// ATOMIC BOOLEAN VARIABLES (using int32: 0 = false, 1 = true)
// ============================================================================

// Execution state management
func (n *RegularNode) SetExecution(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.execution, val)
}

func (n *RegularNode) GetExecution() bool {
	return atomic.LoadInt32(&n.execution) == 1
}

// Leader monitoring state management
func (n *RegularNode) SetLeaderMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.leaderMonitoringActive, val)
}

func (n *RegularNode) GetLeaderMonitoringActive() bool {
	return atomic.LoadInt32(&n.leaderMonitoringActive) == 1
}

// MerkleRootSubmitted event tracking
func (n *RegularNode) SetMerkleRootSubmittedEventEmitted(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.merkleRootSubmittedEventEmitted, val)
}

func (n *RegularNode) GetMerkleRootSubmittedEventEmitted() bool {
	return atomic.LoadInt32(&n.merkleRootSubmittedEventEmitted) == 1
}

// EnqueueUniqueKeyForCleanup adds a uniqueKey to the cleanup queue. If the
// queue size reaches 5 or more, it pops the oldest uniqueKey and cleans it up.
func (n *RegularNode) EnqueueUniqueKeyForCleanup(uniqueKey string) {
	n.cleanupQueueMu.Lock()

	// enqueue
	n.cleanupQueue.Add(uniqueKey)
	length := n.cleanupQueue.Length()
	// if size >= 5, pop oldest and cleanup until size reaches 4
	if length >= 5 {
		for n.cleanupQueue.Length() >= 5 {
			oldest := n.cleanupQueue.Remove().(string)
			// perform cleanup outside
			n.CleanupRoundDataByUniqueKey(oldest)
		}
		n.cleanupQueueMu.Unlock()
		return
	}
	n.cleanupQueueMu.Unlock()
}

// ============================================================================
// ATOMIC STRING VARIABLES (using unsafe.Pointer)
// ============================================================================

// CurrentRound management
func (n *RegularNode) SetCurrentRound(value string) {
	atomic.StorePointer(&n.currentRound, unsafe.Pointer(&value))
}

func (n *RegularNode) GetCurrentRound() string {
	ptr := atomic.LoadPointer(&n.currentRound)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// CurrentTrialNum management
func (n *RegularNode) SetCurrentTrialNum(value string) {
	atomic.StorePointer(&n.currentTrialNum, unsafe.Pointer(&value))
}

func (n *RegularNode) GetCurrentTrialNum() string {
	ptr := atomic.LoadPointer(&n.currentTrialNum)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// RegularNodeEOA management
func (n *RegularNode) SetRegularNodeEOA(value string) {
	atomic.StorePointer(&n.regularNodeEOA, unsafe.Pointer(&value))
}

func (n *RegularNode) GetRegularNodeEOA() string {
	ptr := atomic.LoadPointer(&n.regularNodeEOA)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// ============================================================================
// MUTEX-PROTECTED SLICE VARIABLES
// ============================================================================

// CvRequestIndices slice management
func (n *RegularNode) SetCvRequestIndices(indices []*big.Int) {
	n.cvRequestIndicesMu.Lock()
	defer n.cvRequestIndicesMu.Unlock()
	n.cvRequestIndices = indices
}

func (n *RegularNode) GetCvRequestIndices() []*big.Int {
	n.cvRequestIndicesMu.RLock()
	defer n.cvRequestIndicesMu.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]*big.Int, len(n.cvRequestIndices))
	copy(result, n.cvRequestIndices)
	return result
}

func (n *RegularNode) ClearCvRequestIndices() {
	n.cvRequestIndicesMu.Lock()
	defer n.cvRequestIndicesMu.Unlock()
	n.cvRequestIndices = []*big.Int{}
}

// ============================================================================
// MUTEX-PROTECTED MAP VARIABLES
// ============================================================================

// RoundsData map management
func (n *RegularNode) SetRoundData(key string, data RoundData) {
	n.roundsDataMu.Lock()
	defer n.roundsDataMu.Unlock()
	if n.roundsData == nil {
		n.roundsData = make(map[string]RoundData)
	}
	n.roundsData[key] = data
}

func (n *RegularNode) GetRoundData(key string) (RoundData, bool) {
	n.roundsDataMu.RLock()
	defer n.roundsDataMu.RUnlock()
	data, exists := n.roundsData[key]
	return data, exists
}

func (n *RegularNode) DeleteRoundsData(key string) {
	n.roundsDataMu.Lock()
	defer n.roundsDataMu.Unlock()
	delete(n.roundsData, key)
}

// StrictOrderWhileSecretRequest map management
func (n *RegularNode) SetStrictOrder(key string, value []string) {
	n.strictOrderMu.Lock()
	defer n.strictOrderMu.Unlock()
	n.strictOrderWhileSecretRequest[key] = value
}

func (n *RegularNode) GetStrictOrder(key string) ([]string, bool) {
	n.strictOrderMu.RLock()
	defer n.strictOrderMu.RUnlock()
	value, exists := n.strictOrderWhileSecretRequest[key]
	return value, exists
}

func (n *RegularNode) DeleteStrictOrder(key string) {
	n.strictOrderMu.Lock()
	defer n.strictOrderMu.Unlock()
	delete(n.strictOrderWhileSecretRequest, key)
}

func (n *RegularNode) GetHalted() bool {
	return atomic.LoadInt32(&n.halted) == 1
}

func (n *RegularNode) SetHalted(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&n.halted, val)
}

// ============================================================================
// SUBMITTEDCVINDICES MAP MANAGEMENT
// ============================================================================

// SetSubmittedCvIndicesValue sets a value in the submittedCvIndices map for a specific uniqueKey and index
func (n *RegularNode) SetSubmittedCvIndicesValue(uniqueKey, index string, value bool) {
	n.submittedCvIndicesMutex.Lock()
	defer n.submittedCvIndicesMutex.Unlock()
	if n.submittedCvIndices == nil {
		n.submittedCvIndices = make(map[string]map[string]bool)
	}
	if n.submittedCvIndices[uniqueKey] == nil {
		n.submittedCvIndices[uniqueKey] = make(map[string]bool)
	}
	n.submittedCvIndices[uniqueKey][index] = value
}

// GetSubmittedCvIndicesValue gets a value from the submittedCvIndices map for a specific uniqueKey and index
func (n *RegularNode) GetSubmittedCvIndicesValue(uniqueKey, index string) (bool, bool) {
	n.submittedCvIndicesMutex.RLock()
	defer n.submittedCvIndicesMutex.RUnlock()
	if n.submittedCvIndices == nil || n.submittedCvIndices[uniqueKey] == nil {
		return false, false
	}
	value, exists := n.submittedCvIndices[uniqueKey][index]
	return value, exists
}

// GetSubmittedCvIndicesMap gets the entire map for a specific uniqueKey
func (n *RegularNode) GetSubmittedCvIndicesMap(uniqueKey string) (map[string]bool, bool) {
	n.submittedCvIndicesMutex.RLock()
	defer n.submittedCvIndicesMutex.RUnlock()
	if n.submittedCvIndices == nil || n.submittedCvIndices[uniqueKey] == nil {
		return nil, false
	}
	// Return a copy to prevent external modifications
	result := make(map[string]bool)
	for k, v := range n.submittedCvIndices[uniqueKey] {
		result[k] = v
	}
	return result, true
}

// SetSubmittedCvIndicesMap sets the entire map for a specific uniqueKey
func (n *RegularNode) SetSubmittedCvIndicesMap(uniqueKey string, value map[string]bool) {
	n.submittedCvIndicesMutex.Lock()
	defer n.submittedCvIndicesMutex.Unlock()
	if n.submittedCvIndices == nil {
		n.submittedCvIndices = make(map[string]map[string]bool)
	}
	if n.submittedCvIndices[uniqueKey] == nil {
		n.submittedCvIndices[uniqueKey] = make(map[string]bool)
	}
	// Copy the input map to prevent external modifications
	for k, v := range value {
		n.submittedCvIndices[uniqueKey][k] = v
	}
}

// DeleteSubmittedCvIndices deletes submittedCvIndices entry for uniqueKey
func (n *RegularNode) DeleteSubmittedCvIndices(uniqueKey string) {
	n.submittedCvIndicesMutex.Lock()
	defer n.submittedCvIndicesMutex.Unlock()
	delete(n.submittedCvIndices, uniqueKey)
}

// DeleteCosReceived deletes cosRecevied entry for uniqueKey
func (n *RegularNode) DeleteCosReceived(uniqueKey string) {
	n.cosRecevied.Delete(uniqueKey)
}

// ============================================================================
// MUTEX-PROTECTED STRUCT VARIABLES
// ============================================================================

// RegularNodePrivateKey management
func (n *RegularNode) SetRegularNodePrivateKey(privateKey *ecdsa.PrivateKey) {
	n.privateKeyMu.Lock()
	defer n.privateKeyMu.Unlock()
	n.regularNodePrivateKey = privateKey
}

func (n *RegularNode) GetRegularNodePrivateKey() *ecdsa.PrivateKey {
	n.privateKeyMu.RLock()
	defer n.privateKeyMu.RUnlock()
	return n.regularNodePrivateKey
}

// ============================================================================
// MUTEX-PROTECTED BIG.INT POINTER VARIABLES
// ============================================================================

// MerkleRootSubmittedTime management
func (n *RegularNode) MerkleRootSubmittedTOrRequestedCvTime(timestamp *big.Int) {
	n.merkleRootTimeMu.Lock()
	defer n.merkleRootTimeMu.Unlock()
	n.merkleRootSubmittedTOrRequestedCvTime = timestamp
}

func (n *RegularNode) GetMerkleRootSubmittedTOrRequestedCvTime() *big.Int {
	n.merkleRootTimeMu.RLock()
	defer n.merkleRootTimeMu.RUnlock()
	return n.merkleRootSubmittedTOrRequestedCvTime
}

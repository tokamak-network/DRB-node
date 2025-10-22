package regular_node

import (
	"crypto/ecdsa"
	"math/big"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/eapache/queue"
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
	atomic.StoreInt32(&Execution, val)
}

func (n *RegularNode) GetExecution() bool {
	return atomic.LoadInt32(&Execution) == 1
}

// Leader monitoring state management
func (n *RegularNode) SetLeaderMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&leaderMonitoringActive, val)
}

func (n *RegularNode) GetLeaderMonitoringActive() bool {
	return atomic.LoadInt32(&leaderMonitoringActive) == 1
}

// MerkleRootSubmitted event tracking
func (n *RegularNode) SetMerkleRootSubmittedEventEmitted(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&merkleRootSubmittedEventEmitted, val)
}

func (n *RegularNode) GetMerkleRootSubmittedEventEmitted() bool {
	return atomic.LoadInt32(&merkleRootSubmittedEventEmitted) == 1
}

// =========================================================================
// Cleanup queue for deferred round data cleanup
// =========================================================================

var cleanupQueue *queue.Queue = queue.New()
var cleanupQueueMu sync.Mutex

// EnqueueUniqueKeyForCleanup adds a uniqueKey to the cleanup queue. If the
// queue size reaches 5 or more, it pops the oldest uniqueKey and cleans it up.
func (n *RegularNode) EnqueueUniqueKeyForCleanup(uniqueKey string) {
	cleanupQueueMu.Lock()

	// enqueue
	cleanupQueue.Add(uniqueKey)
	length := cleanupQueue.Length()
	// if size >= 5, pop oldest and cleanup until size reaches 4
	if length >= 5 {
		for cleanupQueue.Length() >= 5 {
			oldest := cleanupQueue.Remove().(string)
			// perform cleanup outside
			n.CleanupRoundDataByUniqueKey(oldest)
		}
		cleanupQueueMu.Unlock()
		return
	}
	cleanupQueueMu.Unlock()
}

// ============================================================================
// ATOMIC STRING VARIABLES (using unsafe.Pointer)
// ============================================================================

// CurrentRound management
func (n *RegularNode) SetCurrentRound(value string) {
	atomic.StorePointer(&CurrentRound, unsafe.Pointer(&value))
}

func (n *RegularNode) GetCurrentRound() string {
	ptr := atomic.LoadPointer(&CurrentRound)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// CurrentTrialNum management
func (n *RegularNode) SetCurrentTrialNum(value string) {
	atomic.StorePointer(&CurrentTrialNum, unsafe.Pointer(&value))
}

func (n *RegularNode) GetCurrentTrialNum() string {
	ptr := atomic.LoadPointer(&CurrentTrialNum)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// RegularNodeEOA management
func (n *RegularNode) SetRegularNodeEOA(value string) {
	atomic.StorePointer(&regularNodeEOA, unsafe.Pointer(&value))
}

func (n *RegularNode) GetRegularNodeEOA() string {
	ptr := atomic.LoadPointer(&regularNodeEOA)
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
	cvRequestIndicesMu.Lock()
	defer cvRequestIndicesMu.Unlock()
	cvRequestIndices = indices
}

func (n *RegularNode) GetCvRequestIndices() []*big.Int {
	cvRequestIndicesMu.RLock()
	defer cvRequestIndicesMu.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]*big.Int, len(cvRequestIndices))
	copy(result, cvRequestIndices)
	return result
}

func (n *RegularNode) ClearCvRequestIndices() {
	cvRequestIndicesMu.Lock()
	defer cvRequestIndicesMu.Unlock()
	cvRequestIndices = []*big.Int{}
}

// ============================================================================
// MUTEX-PROTECTED MAP VARIABLES
// ============================================================================

// RoundsData map management
func (n *RegularNode) SetRoundData(key string, data RoundData) {
	RoundsDataMu.Lock()
	defer RoundsDataMu.Unlock()
	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}
	RoundsData[key] = data
}

func (n *RegularNode) GetRoundData(key string) (RoundData, bool) {
	RoundsDataMu.RLock()
	defer RoundsDataMu.RUnlock()
	data, exists := RoundsData[key]
	return data, exists
}

func (n *RegularNode) DeleteRoundsData(key string) {
	RoundsDataMu.Lock()
	defer RoundsDataMu.Unlock()
	delete(RoundsData, key)
}

// StrictOrderWhileSecretRequest map management
func (n *RegularNode) SetStrictOrder(key string, value []string) {
	strictOrderMu.Lock()
	defer strictOrderMu.Unlock()
	strictOrderWhileSecretRequest[key] = value
}

func (n *RegularNode) GetStrictOrder(key string) ([]string, bool) {
	strictOrderMu.RLock()
	defer strictOrderMu.RUnlock()
	value, exists := strictOrderWhileSecretRequest[key]
	return value, exists
}

func (n *RegularNode) DeleteStrictOrder(key string) {
	strictOrderMu.Lock()
	defer strictOrderMu.Unlock()
	delete(strictOrderWhileSecretRequest, key)
}

func (n *RegularNode) GetHalted() bool {
	return atomic.LoadInt32(&Halted) == 1
}

func (n *RegularNode) SetHalted(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&Halted, val)
}

// ============================================================================
// SUBMITTEDCVINDICES MAP MANAGEMENT
// ============================================================================

// SetSubmittedCvIndicesValue sets a value in the submittedCvIndices map for a specific uniqueKey and index
func (n *RegularNode) SetSubmittedCvIndicesValue(uniqueKey, index string, value bool) {
	submittedCvIndicesMutex.Lock()
	defer submittedCvIndicesMutex.Unlock()
	if submittedCvIndices == nil {
		submittedCvIndices = make(map[string]map[string]bool)
	}
	if submittedCvIndices[uniqueKey] == nil {
		submittedCvIndices[uniqueKey] = make(map[string]bool)
	}
	submittedCvIndices[uniqueKey][index] = value
}

// GetSubmittedCvIndicesValue gets a value from the submittedCvIndices map for a specific uniqueKey and index
func (n *RegularNode) GetSubmittedCvIndicesValue(uniqueKey, index string) (bool, bool) {
	submittedCvIndicesMutex.RLock()
	defer submittedCvIndicesMutex.RUnlock()
	if submittedCvIndices == nil || submittedCvIndices[uniqueKey] == nil {
		return false, false
	}
	value, exists := submittedCvIndices[uniqueKey][index]
	return value, exists
}

// GetSubmittedCvIndicesMap gets the entire map for a specific uniqueKey
func (n *RegularNode) GetSubmittedCvIndicesMap(uniqueKey string) (map[string]bool, bool) {
	submittedCvIndicesMutex.RLock()
	defer submittedCvIndicesMutex.RUnlock()
	if submittedCvIndices == nil || submittedCvIndices[uniqueKey] == nil {
		return nil, false
	}
	// Return a copy to prevent external modifications
	result := make(map[string]bool)
	for k, v := range submittedCvIndices[uniqueKey] {
		result[k] = v
	}
	return result, true
}

// SetSubmittedCvIndicesMap sets the entire map for a specific uniqueKey
func (n *RegularNode) SetSubmittedCvIndicesMap(uniqueKey string, value map[string]bool) {
	submittedCvIndicesMutex.Lock()
	defer submittedCvIndicesMutex.Unlock()
	if submittedCvIndices == nil {
		submittedCvIndices = make(map[string]map[string]bool)
	}
	if submittedCvIndices[uniqueKey] == nil {
		submittedCvIndices[uniqueKey] = make(map[string]bool)
	}
	// Copy the input map to prevent external modifications
	for k, v := range value {
		submittedCvIndices[uniqueKey][k] = v
	}
}

// DeleteSubmittedCvIndices deletes submittedCvIndices entry for uniqueKey
func (n *RegularNode) DeleteSubmittedCvIndices(uniqueKey string) {
	submittedCvIndicesMutex.Lock()
	defer submittedCvIndicesMutex.Unlock()
	delete(submittedCvIndices, uniqueKey)
}

// ============================================================================
// MUTEX-PROTECTED STRUCT VARIABLES
// ============================================================================

// RegularNodePrivateKey management
func (n *RegularNode) SetRegularNodePrivateKey(privateKey *ecdsa.PrivateKey) {
	privateKeyMu.Lock()
	defer privateKeyMu.Unlock()
	regularNodePrivateKey = privateKey
}

func (n *RegularNode) GetRegularNodePrivateKey() *ecdsa.PrivateKey {
	privateKeyMu.RLock()
	defer privateKeyMu.RUnlock()
	return regularNodePrivateKey
}

// ============================================================================
// MUTEX-PROTECTED BIG.INT POINTER VARIABLES
// ============================================================================

// MerkleRootSubmittedTime management
func (n *RegularNode) MerkleRootSubmittedTOrRequestedCvTime(timestamp *big.Int) {
	merkleRootTimeMu.Lock()
	defer merkleRootTimeMu.Unlock()
	merkleRootSubmittedTOrRequestedCvTime = timestamp
}

func (n *RegularNode) GetMerkleRootSubmittedTOrRequestedCvTime() *big.Int {
	merkleRootTimeMu.RLock()
	defer merkleRootTimeMu.RUnlock()
	return merkleRootSubmittedTOrRequestedCvTime
}

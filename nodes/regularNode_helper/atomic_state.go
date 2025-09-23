package regularNode_helper

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
func SetExecution(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&Execution, val)
}

func GetExecution() bool {
	return atomic.LoadInt32(&Execution) == 1
}

// Leader monitoring state management
func SetLeaderMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&leaderMonitoringActive, val)
}

func GetLeaderMonitoringActive() bool {
	return atomic.LoadInt32(&leaderMonitoringActive) == 1
}

// MerkleRootSubmitted event tracking
func SetMerkleRootSubmittedEventEmitted(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&merkleRootSubmittedEventEmitted, val)
}

func GetMerkleRootSubmittedEventEmitted() bool {
	return atomic.LoadInt32(&merkleRootSubmittedEventEmitted) == 1
}

// =========================================================================
// Cleanup queue for deferred round data cleanup
// =========================================================================

var cleanupQueue *queue.Queue = queue.New()
var cleanupQueueMu sync.Mutex

// EnqueueUniqueKeyForCleanup adds a uniqueKey to the cleanup queue. If the
// queue size reaches 5 or more, it pops the oldest uniqueKey and cleans it up.
func EnqueueUniqueKeyForCleanup(uniqueKey string) {
	cleanupQueueMu.Lock()

	// enqueue
	cleanupQueue.Add(uniqueKey)
	length := cleanupQueue.Length()
	// if size >= 5, pop oldest and cleanup until size reaches 4
	if length >= 5 {
		for cleanupQueue.Length() >= 5 {
			oldest := cleanupQueue.Remove().(string)
			// perform cleanup outside
			CleanupRoundDataByUniqueKey(oldest)
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
func SetCurrentRound(value string) {
	atomic.StorePointer(&CurrentRound, unsafe.Pointer(&value))
}

func GetCurrentRound() string {
	ptr := atomic.LoadPointer(&CurrentRound)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// CurrentTrialNum management
func SetCurrentTrialNum(value string) {
	atomic.StorePointer(&CurrentTrialNum, unsafe.Pointer(&value))
}

func GetCurrentTrialNum() string {
	ptr := atomic.LoadPointer(&CurrentTrialNum)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// RegularNodeEOA management
func SetRegularNodeEOA(value string) {
	atomic.StorePointer(&regularNodeEOA, unsafe.Pointer(&value))
}

func GetRegularNodeEOA() string {
	ptr := atomic.LoadPointer(&regularNodeEOA)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

// ============================================================================
// MUTEX-PROTECTED SLICE VARIABLES
// ============================================================================

// ActivatedOperator slice management
func SetActivatedOperator(operators []string) {
	ActivatedOperatorMu.Lock()
	defer ActivatedOperatorMu.Unlock()
	ActivatedOperator = operators
}

func GetActivatedOperator() []string {
	ActivatedOperatorMu.RLock()
	defer ActivatedOperatorMu.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]string, len(ActivatedOperator))
	copy(result, ActivatedOperator)
	return result
}

// CvRequestIndices slice management
func SetCvRequestIndices(indices []*big.Int) {
	cvRequestIndicesMu.Lock()
	defer cvRequestIndicesMu.Unlock()
	cvRequestIndices = indices
}

func GetCvRequestIndices() []*big.Int {
	cvRequestIndicesMu.RLock()
	defer cvRequestIndicesMu.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]*big.Int, len(cvRequestIndices))
	copy(result, cvRequestIndices)
	return result
}

func ClearCvRequestIndices() {
	cvRequestIndicesMu.Lock()
	defer cvRequestIndicesMu.Unlock()
	cvRequestIndices = []*big.Int{}
}

// ============================================================================
// MUTEX-PROTECTED MAP VARIABLES
// ============================================================================

// RoundsData map management
func SetRoundData(key string, data RoundData) {
	RoundsDataMu.Lock()
	defer RoundsDataMu.Unlock()
	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}
	RoundsData[key] = data
}

func GetRoundData(key string) (RoundData, bool) {
	RoundsDataMu.RLock()
	defer RoundsDataMu.RUnlock()
	data, exists := RoundsData[key]
	return data, exists
}

func DeleteRoundsData(key string) {
	RoundsDataMu.Lock()
	defer RoundsDataMu.Unlock()
	delete(RoundsData, key)
}

// StrictOrderWhileSecretRequest map management
func SetStrictOrder(key string, value []string) {
	strictOrderMu.Lock()
	defer strictOrderMu.Unlock()
	strictOrderWhileSecretRequest[key] = value
}

func GetStrictOrder(key string) ([]string, bool) {
	strictOrderMu.RLock()
	defer strictOrderMu.RUnlock()
	value, exists := strictOrderWhileSecretRequest[key]
	return value, exists
}

func DeleteStrictOrder(key string) {
	strictOrderMu.Lock()
	defer strictOrderMu.Unlock()
	delete(strictOrderWhileSecretRequest, key)
}

// DeleteSubmittedCvIndices deletes submittedCvIndices entry for uniqueKey
func DeleteSubmittedCvIndices(uniqueKey string) {
	submittedCvIndicesMutex.Lock()
	defer submittedCvIndicesMutex.Unlock()
	delete(submittedCvIndices, uniqueKey)
}

// ============================================================================
// MUTEX-PROTECTED STRUCT VARIABLES
// ============================================================================

// RegularNodePrivateKey management
func SetRegularNodePrivateKey(privateKey *ecdsa.PrivateKey) {
	privateKeyMu.Lock()
	defer privateKeyMu.Unlock()
	regularNodePrivateKey = privateKey
}

func GetRegularNodePrivateKey() *ecdsa.PrivateKey {
	privateKeyMu.RLock()
	defer privateKeyMu.RUnlock()
	return regularNodePrivateKey
}

// ============================================================================
// MUTEX-PROTECTED BIG.INT POINTER VARIABLES
// ============================================================================

// MerkleRootSubmittedTime management
func MerkleRootSubmittedTOrRequestedCvTime(timestamp *big.Int) {
	merkleRootTimeMu.Lock()
	defer merkleRootTimeMu.Unlock()
	merkleRootSubmittedTOrRequestedCvTime = timestamp
}

func GetMerkleRootSubmittedTOrRequestedCvTime() *big.Int {
	merkleRootTimeMu.RLock()
	defer merkleRootTimeMu.RUnlock()
	return merkleRootSubmittedTOrRequestedCvTime
}

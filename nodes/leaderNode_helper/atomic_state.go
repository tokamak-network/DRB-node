package leaderNode_helper

import (
	"math/big"
	"sync/atomic"
	"unsafe"

	"github.com/tokamak-network/DRB-node/utils"
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

// Halted state management (already using atomic in original code)
func SetHalted(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&Halted, val)
}

func GetHalted() bool {
	return atomic.LoadInt32(&Halted) == 1
}

// Getter and setter functions for submittingMerkleRoot atomic variable
func SetSubmittingMerkleRoot(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&submittingMerkleRoot, val)
}

func GetSubmittingMerkleRoot() bool {
	return atomic.LoadInt32(&submittingMerkleRoot) == 1
}

// CompareAndSwapSubmittingMerkleRoot performs atomic compare-and-swap operation
func CompareAndSwapSubmittingMerkleRoot(old, new bool) bool {
	var oldVal, newVal int32
	if old {
		oldVal = 1
	}
	if new {
		newVal = 1
	}
	return atomic.CompareAndSwapInt32(&submittingMerkleRoot, oldVal, newVal)
}

// RequestedToSubmitCo monitoring management
func SetRequestedToSubmitCoMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&RequestedToSubmitCoMonitoringActive, val)
}

func GetRequestedToSubmitCoMonitoringActive() bool {
	return atomic.LoadInt32(&RequestedToSubmitCoMonitoringActive) == 1
}

// RequestedToSubmitCv monitoring management
func SetRequestedToSubmitCvMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&RequestedToSubmitCvMonitoringActive, val)
}

func GetRequestedToSubmitCvMonitoringActive() bool {
	return atomic.LoadInt32(&RequestedToSubmitCvMonitoringActive) == 1
}

// RequestToSubmitCv monitoring management
func SetRequestToSubmitCvMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&RequestToSubmitCvMonitoringActive, val)
}

func GetRequestToSubmitCvMonitoringActive() bool {
	return atomic.LoadInt32(&RequestToSubmitCvMonitoringActive) == 1
}

// RequestToSubmitCo timer monitoring management
func SetRequestToSubmitCoTimerMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&RequestToSubmitCoTimerMonitoringActive, val)
}

func GetRequestToSubmitCoTimerMonitoringActive() bool {
	return atomic.LoadInt32(&RequestToSubmitCoTimerMonitoringActive) == 1
}

func SetMerkleRootSubmittedTime(value *big.Int) {
	atomic.StorePointer(&merkleRootSubmittedTime, unsafe.Pointer(value))
}

func GetMerkleRootSubmittedTime() *big.Int {
	ptr := atomic.LoadPointer(&merkleRootSubmittedTime)
	if ptr == nil {
		return nil
	}
	return (*big.Int)(ptr)
}

// FailToSubmitS monitoring management
func SetFailToSubmitSMonitoringActive(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&failToSubmitSMonitoringActive, val)
}

func GetFailToSubmitSMonitoringActive() bool {
	return atomic.LoadInt32(&failToSubmitSMonitoringActive) == 1
}

// ============================================================================
// ATOMIC STRING VARIABLES (using unsafe.Pointer)
// ============================================================================

// SecretRequestSentForWhichRound management
func SetSecretRequestSentForWhichRound(value string) {
	atomic.StorePointer(&SecretRequestSentForWhichRound, unsafe.Pointer(&value))
}

func GetSecretRequestSentForWhichRound() string {
	ptr := atomic.LoadPointer(&SecretRequestSentForWhichRound)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
}

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

// CurrentTrial management
func SetCurrentTrial(value string) {
	atomic.StorePointer(&CurrentTrial, unsafe.Pointer(&value))
}

func GetCurrentTrial() string {
	ptr := atomic.LoadPointer(&CurrentTrial)
	if ptr == nil {
		return ""
	}
	return *(*string)(ptr)
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

// RevealRequestStatus map management
func SetRevealRequestStatus(key string, value []string) {
	revealRequestStatusMu.Lock()
	defer revealRequestStatusMu.Unlock()
	revealRequestStatus[key] = value
}

func GetRevealRequestStatus(key string) ([]string, bool) {
	revealRequestStatusMu.RLock()
	defer revealRequestStatusMu.RUnlock()
	value, exists := revealRequestStatus[key]
	return value, exists
}

func DeleteRevealRequestStatus(key string) {
	revealRequestStatusMu.Lock()
	defer revealRequestStatusMu.Unlock()
	delete(revealRequestStatus, key)
}

// ActiveBroadcasts map management
func SetActiveBroadcast(key string, tracker *utils.BroadcastTracker) {
	activeBroadcastsMu.Lock()
	defer activeBroadcastsMu.Unlock()
	activeBroadcasts[key] = tracker
}

func GetActiveBroadcast(key string) (*utils.BroadcastTracker, bool) {
	activeBroadcastsMu.RLock()
	defer activeBroadcastsMu.RUnlock()
	tracker, exists := activeBroadcasts[key]
	return tracker, exists
}

func DeleteActiveBroadcast(key string) {
	activeBroadcastsMu.Lock()
	defer activeBroadcastsMu.Unlock()
	delete(activeBroadcasts, key)
}

// CvOnChain map management
func SetCvOnChain(key string, value bool) {
	CvOnChainMu.Lock()
	defer CvOnChainMu.Unlock()
	CvOnChain[key] = value
}

func GetCvOnChain(key string) (bool, bool) {
	CvOnChainMu.RLock()
	defer CvOnChainMu.RUnlock()
	value, exists := CvOnChain[key]
	return value, exists
}

// ============================================================================
// MUTEX-PROTECTED STRUCT VARIABLES
// ============================================================================

// Req struct management
func SetReq(req RandomRequest) {
	ReqMu.Lock()
	defer ReqMu.Unlock()
	Req = req
}

func GetReq() RandomRequest {
	ReqMu.RLock()
	defer ReqMu.RUnlock()
	return Req
}

// LastSubmitS timestamp management
func SetLastSubmitSTimestamp(timestamp *big.Int) {
	timestampMu.Lock()
	defer timestampMu.Unlock()
	lastSubmitSTimestamp = timestamp
}

func GetLastSubmitSTimestamp() *big.Int {
	timestampMu.RLock()
	defer timestampMu.RUnlock()
	return lastSubmitSTimestamp
}

package leaderNode_helper

import (
	"log"
	"math/big"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/eapache/queue"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
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

// ============================================================================
// MAP CLEANUP FUNCTIONS
// ============================================================================

// DeleteActiveBroadcasts deletes activeBroadcasts entry for uniqueKey
func DeleteActiveBroadcasts(uniqueKey string) {
	activeBroadcastsMu.Lock()
	defer activeBroadcastsMu.Unlock()
	delete(activeBroadcasts, uniqueKey)
}

// DeleteCvOnChain deletes CvOnChain entry for uniqueKey
func DeleteCvOnChain(uniqueKey string) {
	CvOnChainMu.Lock()
	defer CvOnChainMu.Unlock()
	delete(CvOnChain, uniqueKey)
}

// DeleteRoundSecrets deletes RoundSecrets entry for uniqueKey
func DeleteRoundSecrets(uniqueKey string) {
	secretMapsMutex.Lock()
	defer secretMapsMutex.Unlock()
	delete(RoundSecrets, uniqueKey)
}

// ============================================================================
// ROUNDSECRET MAP MANAGEMENT
// ============================================================================

// SetRoundSecretValue sets a value in the roundSecret map for a specific uniqueKey and EOA
func SetRoundSecretValue(uniqueKey, eoaAddress string, value bool) {
	secretMapsMutex.Lock()
	defer secretMapsMutex.Unlock()
	if roundSecret[uniqueKey] == nil {
		roundSecret[uniqueKey] = make(map[string]bool)
	}
	roundSecret[uniqueKey][eoaAddress] = value
}

// GetRoundSecretValue gets a value from the roundSecret map for a specific uniqueKey and EOA
func GetRoundSecretValue(uniqueKey, eoaAddress string) (bool, bool) {
	secretMapsMutex.RLock()
	defer secretMapsMutex.RUnlock()
	if roundSecret[uniqueKey] == nil {
		return false, false
	}
	value, exists := roundSecret[uniqueKey][eoaAddress]
	return value, exists
}

// DeleteRoundSecret deletes roundSecret entry for uniqueKey
func DeleteRoundSecret(uniqueKey string) {
	secretMapsMutex.Lock()
	defer secretMapsMutex.Unlock()
	delete(roundSecret, uniqueKey)
}

// ============================================================================
// SECRETSONCHAIN MAP MANAGEMENT
// ============================================================================

// SetSecretsOnChain sets a value in the secretsOnChain map for a specific uniqueKey
func SetSecretsOnChain(uniqueKey string, value bool) {
	secretsOnChainMu.Lock()
	defer secretsOnChainMu.Unlock()
	secretsOnChain[uniqueKey] = value
}

// GetSecretsOnChain gets a value from the secretsOnChain map for a specific uniqueKey
func GetSecretsOnChain(uniqueKey string) (bool, bool) {
	secretsOnChainMu.RLock()
	defer secretsOnChainMu.RUnlock()
	value, exists := secretsOnChain[uniqueKey]
	return value, exists
}

// DeleteSecretsOnChain deletes secretsOnChain entry for uniqueKey
func DeleteSecretsOnChain(uniqueKey string) {
	secretsOnChainMu.Lock()
	defer secretsOnChainMu.Unlock()
	delete(secretsOnChain, uniqueKey)
}

// ============================================================================
// ROUNDSECRETS MAP MANAGEMENT
// ============================================================================

// GetRoundSecretsValue gets a value from the RoundSecrets map for a specific uniqueKey
func GetRoundSecretsValue(uniqueKey string) ([][32]byte, bool) {
	secretMapsMutex.RLock()
	defer secretMapsMutex.RUnlock()
	value, exists := RoundSecrets[uniqueKey]
	if !exists {
		return nil, false
	}
	// Return a copy to prevent external modifications
	result := make([][32]byte, len(value))
	copy(result, value)
	return result, true
}

// AppendToRoundSecrets appends a secret value to the RoundSecrets map for a specific uniqueKey
func AppendToRoundSecrets(uniqueKey string, secretValue [32]byte) {
	secretMapsMutex.Lock()
	defer secretMapsMutex.Unlock()
	if RoundSecrets[uniqueKey] == nil {
		RoundSecrets[uniqueKey] = make([][32]byte, 0)
	}
	RoundSecrets[uniqueKey] = append(RoundSecrets[uniqueKey], secretValue)
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
			// perform cleanup
			CleanupRoundDataByUniqueKey(oldest)
		}
		cleanupQueueMu.Unlock()
		return
	}
	cleanupQueueMu.Unlock()

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

func DeleteRoundsData(key string) {
	RoundsDataMu.Lock()
	defer RoundsDataMu.Unlock()
	delete(RoundsData, key)
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

// ============================================================================
// CONTRACT INTEGRATION FUNCTIONS
// ============================================================================

// UpdateCurrentRoundFromContract fetches the current round from the contract and updates local state
func UpdateCurrentRoundAndTrial(fallbackEthClient *fallback_ethclient.FallbackRPCClient) error {
	currentRound, err := eth.UpdateCurrentRoundFromContract(fallbackEthClient)
	if err != nil {
		log.Printf("failed to fetch current round from contract: %v", err)
		return err
	}

	trialNumBig, err := eth.GetTrialNumFromContract(fallbackEthClient, currentRound)
	if err != nil {
		log.Printf("failed to fetch trial number for round %s: %v", trialNumBig.String(), err)
		return err
	}

	// Update the local current round state
	SetCurrentRound(currentRound.String())
	SetCurrentTrial(trialNumBig.String())
	return nil
}

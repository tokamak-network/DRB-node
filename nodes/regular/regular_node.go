package regular_node

import (
	"crypto/ecdsa"
	"math/big"
	"sync"
	"time"
	"unsafe"

	"github.com/eapache/queue"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
)

type RegularNode struct {
	fallbackEthClient *fallback_ethclient.FallbackRPCClient

	// Add new variables for monitoring with atomic protection
	leaderMonitoringActive                                int32 // 0 = false, 1 = true
	monitoringTimer                                       *time.Timer
	merkleRootSubmittedEventEmitted                       int32 // 0 = false, 1 = true
	merkleRootMonitoringTimer                             *time.Timer
	requestToSubmitSOrGenerateRandomNumberMonitoringTimer *time.Timer
	merkleRootSubmittedTOrRequestedCvTime                 *big.Int
	merkleRootTimeMu                                      sync.RWMutex // Protect big.Int pointer

	cvRequestIndices   []*big.Int
	cvRequestIndicesMu sync.RWMutex

	submittedCvIndices      map[string]map[string]bool // Track which indices have submitted CV values
	submittedCvIndicesMutex sync.RWMutex               // Protect access to submittedCvIndices

	// =========================================================================
	// Cleanup queue for deferred round data cleanup
	// =========================================================================

	cleanupQueue   *queue.Queue
	cleanupQueueMu sync.Mutex

	// Global variables with mutex protection for thread safety
	strictOrderWhileSecretRequest map[string][]string
	strictOrderMu                 sync.RWMutex

	// Atomic variables for thread safety
	execution int32 // 0 = false, 1 = true
	halted    int32 // 0 = false, 1 = true

	// String variables - using atomic with unsafe.Pointer
	currentRound    unsafe.Pointer // *string
	currentTrialNum unsafe.Pointer // *string

	// Map variables with mutex protection
	roundsData   map[string]RoundData
	roundsDataMu sync.RWMutex

	startTime   *big.Int
	startTimeMu sync.RWMutex

	// Global variables with atomic/mutex protection for thread safety
	cosRecevied sync.Map // outer: string, inner: *sync.Map (string->bool) - sync.Map is already thread-safe

	// regularNodeEOA string with atomic protection
	regularNodeEOA unsafe.Pointer // *string

	// Private key with mutex protection
	regularNodePrivateKey *ecdsa.PrivateKey
	privateKeyMu          sync.RWMutex
}

func NewRegularNode(fallbackEthClient *fallback_ethclient.FallbackRPCClient) *RegularNode {
	return &RegularNode{
		fallbackEthClient:             fallbackEthClient,
		submittedCvIndices:            make(map[string]map[string]bool),
		cleanupQueue:                  queue.New(),
		strictOrderWhileSecretRequest: make(map[string][]string),
		roundsData:                    make(map[string]RoundData),
		cosRecevied:                   sync.Map{},
	}
}

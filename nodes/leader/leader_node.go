package leader_node

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"
	"unsafe"

	"github.com/eapache/queue"
	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

type LeaderNode struct {
	fallbackEthClient fallback_ethclient.IFallbackEthClient

	// Repositories for managing leader node data on database
	leaderCommitRepository     database.ILeaderCommitRepository
	batchRepository            database.IBatchRepository
	broadcastTrackerRepository database.IBroadcastTrackerRepository
	reavealOrderRepository     database.IRevealOrderRepository
	nodeInfoRepository         database.INodeInfoRepository

	// External services
	revealOrderService commitreveal2.IRevealOrderService // Interface for testability
	p2pClient          *libp2putils.P2PClient
	ethService         eth.IEthService // Injected eth service for testability

	// Cached client for transactions (uses LEADER_PRIVATE_KEY)
	client *utils.Client

	// Internal variables to manage leader node data on memory
	commitMu sync.Mutex

	// Atomic variables for thread safety
	execution int32 // 0 = false, 1 = true
	halted    int32 // 0 = false, 1 = true

	// Merkle root submission status flag (using atomic operations)
	submittingMerkleRoot int32 // 0 = false, 1 = true (atomic)

	// Monitoring active flags - using atomic for thread safety
	requestedToSubmitCoMonitoringActive    int32 // 0 = false, 1 = true
	requestedToSubmitCvMonitoringActive    int32 // 0 = false, 1 = true
	requestToSubmitCvMonitoringActive      int32 // 0 = false, 1 = true
	requestToSubmitCoTimerMonitoringActive int32 // 0 = false, 1 = true

	// Timer variables with mutex protection for race condition safety
	requestedToSubmitCoMonitoringTimer    *time.Timer
	requestedToSubmitCvMonitoringTimer    *time.Timer
	requestToSubmitCvMonitoringTimer      *time.Timer
	requestToSubmitCoTimerMonitoringTimer *time.Timer
	timerMutex                           sync.RWMutex

	// In case last SSubmitted event also get's emmitted with Status event and curState is IN_PROGRESS then CurrentRound vairable will not be consistent
	// Note: secretRequestSentForWhichRound, CurrentRound, CurrentTrial, and Req are now handled with atomic operations
	// String variables - using atomic with unsafe.Pointer
	secretRequestSentForWhichRound unsafe.Pointer // *string
	currentRound                   unsafe.Pointer // *string
	currentTrial                   unsafe.Pointer // *string

	// Map variables with mutex protection
	roundsData   map[string]RoundData
	roundsDataMu sync.RWMutex

	// req is a struct so it needs mutex protection
	req   RandomRequest
	reqMu sync.RWMutex

	// =========================================================================
	// Broadcast variables with mutex protection
	// =========================================================================
	broadcastMutex     sync.Mutex
	activeBroadcasts   map[string]*utils.BroadcastTracker
	activeBroadcastsMu sync.RWMutex

	// =========================================================================
	// cvOnChain variables with mutex protection
	// =========================================================================
	cvOnChain   map[string]bool
	cvOnChainMu sync.RWMutex

	// =========================================================================
	// roundSecrets variables with mutex protection
	// =========================================================================
	roundSecrets     map[string][][32]byte
	roundSecret      map[string]map[string]bool
	secretsOnChain   map[string]bool
	indices          []*big.Int
	indicesMutex     sync.RWMutex
	secretMapsMutex  sync.RWMutex
	secretsOnChainMu sync.RWMutex

	// Tracks EOAs that have been sent requests per round - protected with mutex
	revealRequestStatus   map[string][]string
	revealRequestStatusMu sync.RWMutex

	// Variables for monitoring failToSubmitS condition - using atomic for thread safety
	failToSubmitSMonitoringActive int32 // 0 = false, 1 = true
	failToSubmitSMonitoringTimer  *time.Timer
	lastSubmitSTimestamp          *big.Int
	timestampMu                   sync.RWMutex // Protect big.Int pointers

	lastProcessedBlock    uint64
	lastProcessedTxIndex  uint
	lastProcessedLogIndex uint
	lastProcessedCoordsMu sync.RWMutex

	// =========================================================================
	// Cleanup queue for deferred round data cleanup
	// =========================================================================
	cleanupQueue   *queue.Queue
	cleanupQueueMu sync.Mutex
}

func NewLeaderNode(
	fallbackEthClient *fallback_ethclient.FallbackRPCClient,
	revealOrderService commitreveal2.IRevealOrderService,
	p2pClient *libp2putils.P2PClient,
	leaderCommitRepository database.ILeaderCommitRepository,
	batchRepository *database.BatchRepository,
	broadcastTrackerRepository database.IBroadcastTrackerRepository,
	reavealOrderRepository database.IRevealOrderRepository,
	nodeInfoRepository database.INodeInfoRepository,
) (*LeaderNode, error) {
	if fallbackEthClient == nil {
		return nil, errors.New("fallbackEthClient cannot be nil")
	}
	if revealOrderService == nil {
		return nil, errors.New("revealOrderService cannot be nil")
	}
	if p2pClient == nil {
		return nil, errors.New("p2pClient cannot be nil")
	}
	if leaderCommitRepository == nil {
		return nil, errors.New("leaderCommitRepository cannot be nil")
	}
	if batchRepository == nil {
		return nil, errors.New("batchRepository cannot be nil")
	}
	if broadcastTrackerRepository == nil {
		return nil, errors.New("broadcastTrackerRepository cannot be nil")
	}
	if reavealOrderRepository == nil {
		return nil, errors.New("reavealOrderRepository cannot be nil")
	}
	if nodeInfoRepository == nil {
		return nil, errors.New("nodeInfoRepository cannot be nil")
	}

	// Create cached client for transactions
	client, err := utils.NewLeaderClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return nil, err
	}

	return &LeaderNode{
		client:                     client,
		fallbackEthClient:          fallbackEthClient,
		leaderCommitRepository:     leaderCommitRepository,
		revealOrderService:         revealOrderService,
		batchRepository:            batchRepository,
		broadcastTrackerRepository: broadcastTrackerRepository,
		reavealOrderRepository:     reavealOrderRepository,
		nodeInfoRepository:         nodeInfoRepository,
		p2pClient:                  p2pClient,
		ethService:                 eth.Service, // Use default eth service
		roundsData:                 make(map[string]RoundData),
		activeBroadcasts:           make(map[string]*utils.BroadcastTracker),
		cvOnChain:                  make(map[string]bool),
		roundSecrets:               make(map[string][][32]byte),
		roundSecret:                make(map[string]map[string]bool),
		secretsOnChain:             make(map[string]bool),
		indices:                    make([]*big.Int, 0),
		revealRequestStatus:        make(map[string][]string),
		cleanupQueue:               queue.New(),
	}, nil
}

func (n *LeaderNode) AddLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	return n.leaderCommitRepository.AddLeaderCommit(ctx, commitData)
}

func (n *LeaderNode) GetLeaderCommitByRoundAndEoaAddr(ctx context.Context, round, trialNum, eoaAddr string) (*utils.LeaderCommitData, error) {
	return n.leaderCommitRepository.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, eoaAddr)
}

func (n *LeaderNode) UpdateLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	return n.leaderCommitRepository.UpdateLeaderCommit(ctx, commitData)
}

func (n *LeaderNode) DetermineRevealOrder(ctx context.Context, round, trialNum string, activatedOps []common.Address) (bool, error) {
	return n.revealOrderService.DetermineRevealOrder(ctx, round, trialNum, activatedOps)
}

func (n *LeaderNode) DetermineRegularRevealOrder(ctx context.Context, round, trialNum string, activatedOps []common.Address) (bool, error) {
	return n.revealOrderService.DetermineRegularRevealOrder(ctx, round, trialNum, activatedOps)
}

func (n *LeaderNode) CreateHost(port string, nodeType string) (host.Host, peer.ID, error) {
	return n.p2pClient.CreateHost(port, nodeType)
}

func (n *LeaderNode) SetHost(h host.Host) {
	n.p2pClient.SetHost(h)
}

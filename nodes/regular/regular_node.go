package regular_node

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"sync"
	"time"
	"unsafe"

	"github.com/eapache/queue"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

type RegularNode struct {
	fallbackEthClient fallback_ethclient.IFallbackEthClient

	// Repositories for managing regular node data on database
	peerCommitDataRepository database.IPeerCommitRepository
	revealOrderRepository    database.IRevealOrderRepository
	regularCommitRepository  database.IRegularCommitRepository
	batchRepository          database.IBatchRepository
	nodeInfoRepository       database.INodeInfoRepository

	// External services
	revealOrderService *commitreveal2.RevealOrderService
	p2pClient          *libp2putils.P2PClient

	// Cached client for transactions (uses EOA_PRIVATE_KEY)
	client *utils.Client

	// Add new variables for monitoring with atomic protection
	leaderMonitoringActive                                int32 // 0 = false, 1 = true
	monitoringTimer                                       *time.Timer
	merkleRootSubmittedEventEmitted                       int32 // 0 = false, 1 = true
	merkleRootMonitoringTimer                             *time.Timer
	requestToSubmitSOrGenerateRandomNumberMonitoringTimer *time.Timer
	timerMutex                                            sync.RWMutex // Protect timer operations for race condition safety
	submitSMonitoringReferenceTime                        *big.Int
	submitSMonitoringReferenceTimeMu                      sync.RWMutex // Protect big.Int pointer

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

func NewRegularNode(
	fallbackEthClient fallback_ethclient.IFallbackEthClient,
	revealOrderService *commitreveal2.RevealOrderService,
	p2pClient *libp2putils.P2PClient,
	peerCommitDataRepository *database.PeerCommitRepository,
	revealOrderRepository *database.RevealOrderRepository,
	regularCommitRepository *database.RegularCommitRepository,
	batchRepository *database.BatchRepository,
	nodeInfoRepository *database.NodeInfoRepository,
) (*RegularNode, error) {
	if fallbackEthClient == nil {
		return nil, errors.New("fallbackEthClient cannot be nil")
	}
	if revealOrderService == nil {
		return nil, errors.New("revealOrderService cannot be nil")
	}
	if p2pClient == nil {
		return nil, errors.New("p2pClient cannot be nil")
	}
	if peerCommitDataRepository == nil {
		return nil, errors.New("peerCommitDataRepository cannot be nil")
	}
	if revealOrderRepository == nil {
		return nil, errors.New("revealOrderRepository cannot be nil")
	}
	if regularCommitRepository == nil {
		return nil, errors.New("regularCommitRepository cannot be nil")
	}
	if batchRepository == nil {
		return nil, errors.New("batchRepository cannot be nil")
	}
	if nodeInfoRepository == nil {
		return nil, errors.New("nodeInfoRepository cannot be nil")
	}

	// Create cached client for transactions
	client, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return nil, err
	}

	return &RegularNode{
		client:                        client,
		fallbackEthClient:             fallbackEthClient,
		revealOrderService:            revealOrderService,
		p2pClient:                     p2pClient,
		peerCommitDataRepository:      peerCommitDataRepository,
		revealOrderRepository:         revealOrderRepository,
		regularCommitRepository:       regularCommitRepository,
		batchRepository:               batchRepository,
		nodeInfoRepository:            nodeInfoRepository,
		submittedCvIndices:            make(map[string]map[string]bool),
		cleanupQueue:                  queue.New(),
		strictOrderWhileSecretRequest: make(map[string][]string),
		roundsData:                    make(map[string]RoundData),
		cosRecevied:                   sync.Map{},
	}, nil
}

func (n *RegularNode) GetCommitByRound(ctx context.Context, round, trialNum string) (*utils.CommitData, error) {
	return n.regularCommitRepository.GetCommitByRound(ctx, round, trialNum)
}

func (n *RegularNode) AddNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	return n.nodeInfoRepository.AddAndUpdateNodeInfo(ctx, nodeInfo)
}

func (n *RegularNode) AddCommit(ctx context.Context, commitData *utils.CommitData) error {
	return n.regularCommitRepository.AddCommit(ctx, commitData)
}

func (n *RegularNode) UpdateCommit(ctx context.Context, commitData *utils.CommitData) error {
	return n.regularCommitRepository.UpdateCommit(ctx, commitData)
}

func (n *RegularNode) CreateHost(port string, nodeType string) (host.Host, peer.ID, error) {
	return n.p2pClient.CreateHost(port, nodeType)
}

func (n *RegularNode) SetHost(h host.Host) {
	n.p2pClient.SetHost(h)
}

func (n *RegularNode) ConnectToLeader(ctx context.Context, leaderIP, leaderPort, leaderPeerID string) (*peer.AddrInfo, error) {
	return n.p2pClient.ConnectToPeer(ctx, leaderIP, leaderPort, leaderPeerID)
}

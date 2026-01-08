package leader_node

import (
	"context"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-pg/pg/v10"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// MockP2PClient is a mock for P2PClient
type MockP2PClient struct {
	mock.Mock
}

func (m *MockP2PClient) CreateHost(port string, nodeType string) (host.Host, peer.ID, error) {
	args := m.Called(port, nodeType)
	if args.Get(0) == nil {
		return nil, "", args.Error(2)
	}
	return args.Get(0).(host.Host), args.Get(1).(peer.ID), args.Error(2)
}

func (m *MockP2PClient) SetHost(h host.Host) {
	m.Called(h)
}

func (m *MockP2PClient) GetHostInstance() host.Host {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(host.Host)
}

func (m *MockP2PClient) GetConnectedPeers(ctx context.Context) map[string]*utils.NodeInfo {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(map[string]*utils.NodeInfo)
}

// MockLeaderCommitRepository is a mock for ILeaderCommitRepository (reusing existing interface pattern)
type MockLeaderCommitRepo struct {
	mock.Mock
}

func (m *MockLeaderCommitRepo) AddLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

func (m *MockLeaderCommitRepo) GetLeaderCommitByRoundAndEoaAddr(ctx context.Context, round, trialNum, eoaAddr string) (*utils.LeaderCommitData, error) {
	args := m.Called(ctx, round, trialNum, eoaAddr)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.LeaderCommitData), args.Error(1)
}

func (m *MockLeaderCommitRepo) UpdateLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

func (m *MockLeaderCommitRepo) GetLeaderCommitsByRoundAndTrialNum(ctx context.Context, round, trialNum string) ([]*utils.LeaderCommitData, error) {
	args := m.Called(ctx, round, trialNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.LeaderCommitData), args.Error(1)
}

func (m *MockLeaderCommitRepo) UpdateLeaderCommitRandomNumberGenerated(ctx context.Context, round, trialNum string) error {
	args := m.Called(ctx, round, trialNum)
	return args.Error(0)
}

// MockRevealOrderRepo is a mock for IRevealOrderRepository
type MockRevealOrderRepo struct {
	mock.Mock
}

func (m *MockRevealOrderRepo) GetRevealOrder(ctx context.Context, round, trialNum string) (*utils.RevealOrderData, error) {
	args := m.Called(ctx, round, trialNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.RevealOrderData), args.Error(1)
}

func (m *MockRevealOrderRepo) AddRevealOrder(ctx context.Context, data *utils.RevealOrderData) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

// MockNodeInfoRepo is a mock for INodeInfoRepository
type MockNodeInfoRepo struct {
	mock.Mock
}

func (m *MockNodeInfoRepo) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.NodeInfo), args.Error(1)
}

func (m *MockNodeInfoRepo) AddAndUpdateNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	args := m.Called(ctx, nodeInfo)
	return args.Error(0)
}

func (m *MockNodeInfoRepo) GetNodeInfoByEOA(ctx context.Context, eoaAddress string) (*utils.NodeInfo, error) {
	args := m.Called(ctx, eoaAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.NodeInfo), args.Error(1)
}

func (m *MockNodeInfoRepo) DeleteNodeInfoByEOA(ctx context.Context, eoaAddress string) error {
	args := m.Called(ctx, eoaAddress)
	return args.Error(0)
}

// MockBroadcastTrackerRepo is a mock for IBroadcastTrackerRepository
type MockBroadcastTrackerRepo struct {
	mock.Mock
}

func (m *MockBroadcastTrackerRepo) AddBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error {
	args := m.Called(ctx, tracker)
	return args.Error(0)
}

func (m *MockBroadcastTrackerRepo) UpdateBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error {
	args := m.Called(ctx, tracker)
	return args.Error(0)
}

// Helper function to create a LeaderNode with mocks
func setupLeaderNodeWithMocksForNodeTest() (*LeaderNode, *MockRevealOrderService, *MockLeaderCommitRepo) {
	mockRevealOrderService := new(MockRevealOrderService)
	mockLeaderCommitRepo := new(MockLeaderCommitRepo)
	mockBatchRepo := new(MockBatchRepository)
	mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
	mockRevealOrderRepo := new(MockRevealOrderRepo)
	mockNodeInfoRepo := new(MockNodeInfoRepo)

	// Create a real libp2putils.P2PClient for testing
	p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

	leaderNode := &LeaderNode{
		leaderCommitRepository:     mockLeaderCommitRepo,
		revealOrderService:         mockRevealOrderService,
		batchRepository:            mockBatchRepo,
		broadcastTrackerRepository: mockBroadcastTrackerRepo,
		reavealOrderRepository:     mockRevealOrderRepo,
		nodeInfoRepository:         mockNodeInfoRepo,
		p2pClient:                  p2pClient,
		ethService:                 eth.Service,
		roundsData:                 make(map[string]RoundData),
		activeBroadcasts:           make(map[string]*utils.BroadcastTracker),
		cvOnChain:                  make(map[string]bool),
		roundSecrets:               make(map[string][][32]byte),
		roundSecret:                make(map[string]map[string]bool),
		secretsOnChain:             make(map[string]bool),
		indices:                    make([]*big.Int, 0),
		revealRequestStatus:        make(map[string][]string),
	}

	return leaderNode, mockRevealOrderService, mockLeaderCommitRepo
}

// Test for DetermineRegularRevealOrder
func TestLeaderNode_DetermineRegularRevealOrder(t *testing.T) {
	ctx := context.Background()
	round := "1"
	trialNum := "0"
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0x2234567890123456789012345678901234567890"),
	}

	t.Run("Success", func(t *testing.T) {
		leaderNode, mockRevealOrderService, _ := setupLeaderNodeWithMocksForNodeTest()

		// Setup mock expectations
		mockRevealOrderService.On("DetermineRegularRevealOrder", ctx, round, trialNum, activatedOps).Return(true, nil)

		// Execute
		result, err := leaderNode.DetermineRegularRevealOrder(ctx, round, trialNum, activatedOps)

		// Assert
		assert.NoError(t, err)
		assert.True(t, result)
		mockRevealOrderService.AssertExpectations(t)
	})

	t.Run("Error from service", func(t *testing.T) {
		leaderNode, mockRevealOrderService, _ := setupLeaderNodeWithMocksForNodeTest()

		// Setup mock expectations
		expectedError := assert.AnError
		mockRevealOrderService.On("DetermineRegularRevealOrder", ctx, round, trialNum, activatedOps).Return(false, expectedError)

		// Execute
		result, err := leaderNode.DetermineRegularRevealOrder(ctx, round, trialNum, activatedOps)

		// Assert
		assert.Error(t, err)
		assert.False(t, result)
		assert.Equal(t, expectedError, err)
		mockRevealOrderService.AssertExpectations(t)
	})

	t.Run("Empty activated operators", func(t *testing.T) {
		leaderNode, mockRevealOrderService, _ := setupLeaderNodeWithMocksForNodeTest()

		emptyOps := []common.Address{}
		mockRevealOrderService.On("DetermineRegularRevealOrder", ctx, round, trialNum, emptyOps).Return(false, nil)

		result, err := leaderNode.DetermineRegularRevealOrder(ctx, round, trialNum, emptyOps)

		assert.NoError(t, err)
		assert.False(t, result)
		mockRevealOrderService.AssertExpectations(t)
	})
}

// Test for CreateHost and SetHost - these are simple wrapper functions
func TestLeaderNode_CreateHost_And_SetHost(t *testing.T) {

	t.Run("CreateHost - verify p2pClient is called", func(t *testing.T) {
		// Setup
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		host, peerID, err := leaderNode.CreateHost("4001", "leader")

		_ = host
		_ = peerID
		_ = err
	})

	t.Run("SetHost - verify p2pClient is called", func(t *testing.T) {
		// Setup
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		leaderNode.SetHost(nil)
		assert.NotNil(t, leaderNode.p2pClient)
	})
}

func createTestHostForSetHost(t *testing.T) host.Host {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	return h
}

func createTestLeaderNodeForSetHost() *LeaderNode {
	mockNodeInfoRepo := new(MockNodeInfoRepo)
	p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

	return &LeaderNode{
		p2pClient:           p2pClient,
		roundsData:          make(map[string]RoundData),
		activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
		cvOnChain:           make(map[string]bool),
		roundSecrets:        make(map[string][][32]byte),
		roundSecret:         make(map[string]map[string]bool),
		secretsOnChain:      make(map[string]bool),
		indices:             make([]*big.Int, 0),
		revealRequestStatus: make(map[string][]string),
	}
}

func TestLeaderNode_SetHost_InvalidHostScenarios(t *testing.T) {

	t.Run("SetHost with nil host", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		assert.NotPanics(t, func() {
			leaderNode.SetHost(nil)
		})
		assert.Nil(t, leaderNode.p2pClient.GetHostInstance())
	})

	t.Run("SetHost with nil host, verify GetHostInstance", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		leaderNode.SetHost(nil)

		retrievedHost := leaderNode.p2pClient.GetHostInstance()
		assert.Nil(t, retrievedHost, "GetHostInstance should return nil after SetHost(nil)")
	})

	t.Run("SetHost with nil host, then use it in ConnectToPeer", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		leaderNode.SetHost(nil)
		assert.Nil(t, leaderNode.p2pClient.GetHostInstance(), "hostInstance should be nil")

		testHost := createTestHostForSetHost(t)
		defer testHost.Close()
		validPeerID := testHost.ID().String()
		assert.Panics(t, func() {
			_, _ = leaderNode.p2pClient.ConnectToPeer(context.Background(), "127.0.0.1", "4001", validPeerID)
		}, "ConnectToPeer should panic when hostInstance is nil")
	})

	t.Run("SetHost when p2pClient is nil", func(t *testing.T) {
		leaderNode := &LeaderNode{
			p2pClient:           nil, // nil p2pClient
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		testHost := createTestHostForSetHost(t)
		defer testHost.Close()

		assert.Panics(t, func() {
			leaderNode.SetHost(testHost)
		}, "SetHost should panic when p2pClient is nil")
	})

	t.Run("SetHost when LeaderNode itself is nil", func(t *testing.T) {
		var leaderNode *LeaderNode = nil

		testHost := createTestHostForSetHost(t)
		defer testHost.Close()

		assert.Panics(t, func() {
			leaderNode.SetHost(testHost)
		}, "SetHost should panic when LeaderNode is nil")
	})

	t.Run("Replace existing host", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		host1 := createTestHostForSetHost(t)
		defer host1.Close()

		host2 := createTestHostForSetHost(t)
		defer host2.Close()

		leaderNode.SetHost(host1)
		assert.Equal(t, host1, leaderNode.p2pClient.GetHostInstance(), "First host should be set")

		leaderNode.SetHost(host2)
		assert.Equal(t, host2, leaderNode.p2pClient.GetHostInstance(), "Second host should replace first")
		assert.NotEqual(t, host1, leaderNode.p2pClient.GetHostInstance(), "First host should no longer be stored")
	})

	t.Run("Replace host with nil", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		testHost := createTestHostForSetHost(t)
		defer testHost.Close()

		leaderNode.SetHost(testHost)
		assert.Equal(t, testHost, leaderNode.p2pClient.GetHostInstance())

		leaderNode.SetHost(nil)
		assert.Nil(t, leaderNode.p2pClient.GetHostInstance(), "Host should be cleared (nil)")
	})

	t.Run("Replace nil host with valid host", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		leaderNode.SetHost(nil)
		assert.Nil(t, leaderNode.p2pClient.GetHostInstance())

		testHost := createTestHostForSetHost(t)
		defer testHost.Close()

		leaderNode.SetHost(testHost)
		assert.Equal(t, testHost, leaderNode.p2pClient.GetHostInstance(), "Valid host should be set correctly")
	})

	t.Run("SetHost and verify host is stored", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		testHost := createTestHostForSetHost(t)
		defer testHost.Close()

		leaderNode.SetHost(testHost)

		retrievedHost := leaderNode.p2pClient.GetHostInstance()
		assert.Equal(t, testHost, retrievedHost, "SetHost should store the host correctly")
		assert.NotNil(t, retrievedHost, "Retrieved host should not be nil")
	})

	t.Run("SetHost and verify host properties", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		testHost := createTestHostForSetHost(t)
		defer testHost.Close()

		leaderNode.SetHost(testHost)
		retrievedHost := leaderNode.p2pClient.GetHostInstance()
		assert.NotNil(t, retrievedHost, "Host should not be nil")
		assert.NotEmpty(t, retrievedHost.ID().String(), "Host should have a PeerID")
		assert.NotEmpty(t, retrievedHost.Addrs(), "Host should have addresses")
	})

	t.Run("SetHost with closed host", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		testHost := createTestHostForSetHost(t)
		testHost.Close()

		assert.NotPanics(t, func() {
			leaderNode.SetHost(testHost)
		})
		assert.Equal(t, testHost, leaderNode.p2pClient.GetHostInstance(), "Closed host should still be stored")
	})

	t.Run("SetHost with closed host, then use it", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		testHost := createTestHostForSetHost(t)
		testHost.Close()

		leaderNode.SetHost(testHost)

		retrievedHost := leaderNode.p2pClient.GetHostInstance()
		assert.Equal(t, testHost, retrievedHost, "Closed host should be stored")
	})

	t.Run("Concurrent SetHost calls", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		host1 := createTestHostForSetHost(t)
		defer host1.Close()

		host2 := createTestHostForSetHost(t)
		defer host2.Close()

		host3 := createTestHostForSetHost(t)
		defer host3.Close()

		var wg sync.WaitGroup
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				var h host.Host
				switch goroutineID % 3 {
				case 0:
					h = host1
				case 1:
					h = host2
				default:
					h = host3
				}

				assert.NotPanics(t, func() {
					leaderNode.SetHost(h)
				})
			}(i)
		}

		wg.Wait()

		retrievedHost := leaderNode.p2pClient.GetHostInstance()
		assert.NotNil(t, retrievedHost, "Host should be set after concurrent calls")
		assert.True(t,
			retrievedHost == host1 || retrievedHost == host2 || retrievedHost == host3,
			"Retrieved host should be one of the concurrently set hosts")
	})

	t.Run("SetHost while host is being used", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		host1 := createTestHostForSetHost(t)
		defer host1.Close()

		host2 := createTestHostForSetHost(t)
		defer host2.Close()

		leaderNode.SetHost(host1)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = host1.Peerstore()
		}()

		leaderNode.SetHost(host2)

		wg.Wait()

		assert.Equal(t, host2, leaderNode.p2pClient.GetHostInstance(), "Host should be replaced even during concurrent usage")
	})

	t.Run("Multiple SetHost calls with same host", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		testHost := createTestHostForSetHost(t)
		defer testHost.Close()

		leaderNode.SetHost(testHost)
		leaderNode.SetHost(testHost)
		leaderNode.SetHost(testHost)

		assert.Equal(t, testHost, leaderNode.p2pClient.GetHostInstance(), "Host should remain set after multiple calls")
	})

	t.Run("SetHost with nil, then nil again", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		leaderNode.SetHost(nil)
		leaderNode.SetHost(nil)
		leaderNode.SetHost(nil)

		assert.Nil(t, leaderNode.p2pClient.GetHostInstance(), "Host should remain nil after multiple nil SetHost calls")
	})

	t.Run("SetHost with hosts created on different ports", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()
		host1, err1 := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/4001"))
		require.NoError(t, err1)
		defer host1.Close()

		host2, err2 := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/4002"))
		require.NoError(t, err2)
		defer host2.Close()
		leaderNode.SetHost(host1)
		assert.Equal(t, host1, leaderNode.p2pClient.GetHostInstance())
		assert.Equal(t, host1.ID(), leaderNode.p2pClient.GetHostInstance().ID())
		leaderNode.SetHost(host2)
		assert.Equal(t, host2, leaderNode.p2pClient.GetHostInstance())
		assert.NotEqual(t, host1.ID(), leaderNode.p2pClient.GetHostInstance().ID())
		assert.NotEqual(t, host1.Addrs(), host2.Addrs())
	})

	t.Run("SetHost with hosts on different IP addresses", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()
		hostLocalhost, err1 := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err1)
		defer hostLocalhost.Close()
		hostAllInterfaces, err2 := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))
		require.NoError(t, err2)
		defer hostAllInterfaces.Close()

		leaderNode.SetHost(hostLocalhost)
		assert.Equal(t, hostLocalhost, leaderNode.p2pClient.GetHostInstance())

		leaderNode.SetHost(hostAllInterfaces)
		assert.Equal(t, hostAllInterfaces, leaderNode.p2pClient.GetHostInstance())
	})

	t.Run("SetHost with hosts having different PeerIDs", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		host1, err1 := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err1)
		defer host1.Close()

		host2, err2 := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err2)
		defer host2.Close()
		assert.NotEqual(t, host1.ID(), host2.ID())

		leaderNode.SetHost(host1)
		assert.Equal(t, host1.ID(), leaderNode.p2pClient.GetHostInstance().ID())

		leaderNode.SetHost(host2)
		assert.Equal(t, host2.ID(), leaderNode.p2pClient.GetHostInstance().ID())
		assert.NotEqual(t, host1.ID(), leaderNode.p2pClient.GetHostInstance().ID())
	})

	t.Run("SetHost with host listening on multiple addresses", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()
		multiAddrHost, err := libp2p.New(
			libp2p.ListenAddrStrings(
				"/ip4/127.0.0.1/tcp/0",
				"/ip4/0.0.0.0/tcp/0",
			),
		)
		require.NoError(t, err)
		defer multiAddrHost.Close()

		leaderNode.SetHost(multiAddrHost)
		retrievedHost := leaderNode.p2pClient.GetHostInstance()

		assert.Equal(t, multiAddrHost, retrievedHost)
		assert.GreaterOrEqual(t, len(retrievedHost.Addrs()), 2, "Host should have multiple addresses")
	})

	t.Run("SetHost with hosts using different network protocols", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()
		tcpHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err)
		defer tcpHost.Close()

		leaderNode.SetHost(tcpHost)
		assert.Equal(t, tcpHost, leaderNode.p2pClient.GetHostInstance())
	})

	t.Run("SetHost preserves host properties correctly", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		testHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err)
		defer testHost.Close()

		originalPeerID := testHost.ID()
		originalAddrs := testHost.Addrs()

		leaderNode.SetHost(testHost)
		retrievedHost := leaderNode.p2pClient.GetHostInstance()

		assert.Equal(t, originalPeerID, retrievedHost.ID())
		assert.Equal(t, originalAddrs, retrievedHost.Addrs())
		assert.Equal(t, testHost.Network(), retrievedHost.Network())
	})

	t.Run("SetHost with host using auto-assigned port ", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()
		autoPortHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err)
		defer autoPortHost.Close()

		leaderNode.SetHost(autoPortHost)
		retrievedHost := leaderNode.p2pClient.GetHostInstance()

		assert.Equal(t, autoPortHost, retrievedHost)
		assert.NotEmpty(t, retrievedHost.Addrs(), "Host should have assigned address")
	})

	t.Run("SetHost and verify host can be used for network operations", func(t *testing.T) {
		leaderNode := createTestLeaderNodeForSetHost()

		host1, err1 := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err1)
		defer host1.Close()

		host2, err2 := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err2)
		defer host2.Close()

		leaderNode.SetHost(host1)

		assert.NotNil(t, leaderNode.p2pClient.GetHostInstance().Peerstore())
		assert.NotEmpty(t, leaderNode.p2pClient.GetHostInstance().ID())

		leaderNode.SetHost(host2)

		assert.NotNil(t, leaderNode.p2pClient.GetHostInstance().Peerstore())
		assert.NotEqual(t, host1.ID(), leaderNode.p2pClient.GetHostInstance().ID())
	})
}
func TestLeaderNode_CreateHost_NetworkInterfaceFailures(t *testing.T) {
	t.Run("CreateHost with nil p2pClient", func(t *testing.T) {
		leaderNode := &LeaderNode{
			p2pClient:           nil, // nil p2pClient
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		assert.NotPanics(t, func() {
			_, _, err := leaderNode.CreateHost("4001", "leader")
			_ = err
		}, "CreateHost on nil p2pClient doesn't panic.")
	})

	t.Run("CreateHost with empty port string", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("", "leader")
		assert.Error(t, err, "Expected error with empty port string")
	})

	t.Run("CreateHost with non-numeric port", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("abc", "leader")
		assert.Error(t, err, "Expected error with non-numeric port")
	})

	t.Run("CreateHost with port out of range", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("99999", "leader")
		assert.Error(t, err, "Expected error with port out of range")
	})

	t.Run("CreateHost with negative port", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("-1", "leader")
		assert.Error(t, err, "Expected error with negative port")
	})

	t.Run("CreateHost with invalid nodeType", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("4001", "")
		_ = err
	})

	t.Run("CreateHost with very large port number", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}
		_, _, err := leaderNode.CreateHost("999999", "leader")
		assert.Error(t, err, "Expected error with very large port number")
	})

	t.Run("CreateHost with port containing leading whitespace", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost(" 4001", "leader")
		assert.Error(t, err, "Expected error with port containing leading whitespace")
	})

	t.Run("CreateHost with port containing trailing whitespace", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("4001 ", "leader")
		assert.Error(t, err, "Expected error with port containing trailing whitespace")
	})

	t.Run("CreateHost with port containing leading zeros", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("04001", "leader")

		if err != nil {
			assert.NotContains(t, err.Error(), "invalid port", "Error should not be port validation related")
			assert.NotContains(t, err.Error(), "failed to create libp2p host", "Error should not be libp2p host creation related")
		}
	})

	t.Run("CreateHost with decimal port", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("4001.5", "leader")
		assert.Error(t, err, "Expected error with decimal port")
	})

	t.Run("CreateHost with port containing special characters", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		testCases := []string{"4001@", "4001#", "40-01", "40_01", "4001!", "4001$"}
		for _, port := range testCases {
			_, _, err := leaderNode.CreateHost(port, "leader")
			assert.Error(t, err, "Expected error with port containing special characters: %s", port)
		}
	})

	t.Run("CreateHost with maximum valid port (65535)", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("65535", "leader")

		if err != nil {
			assert.NotContains(t, err.Error(), "invalid port", "Port 65535 should be valid")
			assert.NotContains(t, err.Error(), "port out of range", "Port 65535 should be in valid range")
		}
	})

	t.Run("CreateHost with minimum valid port (1)", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("1", "leader")

		if err != nil {
			assert.NotContains(t, err.Error(), "invalid port", "Port 1 should be valid")
			assert.NotContains(t, err.Error(), "port out of range", "Port 1 should be in valid range")
		}
	})

	t.Run("CreateHost with port just above maximum (65536)", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("65536", "leader")
		assert.Error(t, err, "Expected error with port just above maximum (65536)")
	})
	t.Run("CreateHost with very long port string", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		longPort := "4" + string(make([]byte, 1000))
		_, _, err := leaderNode.CreateHost(longPort, "leader")
		assert.Error(t, err, "Expected error with very long port string")
	})

	t.Run("CreateHost with port containing null character", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		portWithNull := "4001" + string([]byte{0})
		_, _, err := leaderNode.CreateHost(portWithNull, "leader")
		assert.Error(t, err, "Expected error with port containing null character")
	})

	t.Run("CreateHost with port containing unicode characters", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		testCases := []string{"4001测试", "4001🚀", "4001α", "4001ñ"}
		for _, port := range testCases {
			_, _, err := leaderNode.CreateHost(port, "leader")
			assert.Error(t, err, "Expected error with port containing unicode: %s", port)
		}
	})

	t.Run("CreateHost with different nodeTypes", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		nodeTypes := []string{"leader", "regular", "test", "node"}
		for _, nodeType := range nodeTypes {
			_, _, err := leaderNode.CreateHost("4001", nodeType)

			if err != nil {
				assert.NotContains(t, err.Error(), "invalid nodeType", "NodeType %s should be accepted", nodeType)
			}
		}
	})

	t.Run("CreateHost with nodeType containing special characters", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		testCases := []string{"leader@", "leader/", "leader node", "leader#", "leader$"}
		for _, nodeType := range testCases {
			_, _, err := leaderNode.CreateHost("4001", nodeType)
			if err != nil {
				assert.True(t,
					containsAny(err.Error(), []string{"failed to write", "failed to load", "no such file", "invalid", "permission"}),
					"Error for nodeType '%s' should be file-related, got: %v", nodeType, err)
			}
		}
	})

	t.Run("CreateHost with very long nodeType", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		longNodeType := "leader" + string(make([]byte, 1000))
		_, _, err := leaderNode.CreateHost("4001", longNodeType)
		if err != nil {
			assert.True(t,
				containsAny(err.Error(), []string{"failed to write", "failed to load", "no such file", "invalid", "too long", "file name"}),
				"Error for very long nodeType should be file-related, got: %v", err)
		}
	})

	t.Run("CreateHost with nodeType containing path traversal", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		testCases := []string{"../leader", "../../leader", "..\\leader", "/etc/passwd"}
		for _, nodeType := range testCases {
			_, _, err := leaderNode.CreateHost("4001", nodeType)
			if err != nil {

				assert.True(t,
					containsAny(err.Error(), []string{"failed to write", "failed to load", "permission", "invalid", "no such file"}),
					"Path traversal attempt '%s' should cause file-related error, got: %v", nodeType, err)
			} else {
				t.Logf(" Path traversal '%s' did not cause error", nodeType)
			}
		}
	})
	t.Run("CreateHost with network interface unavailable", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		leaderNode := &LeaderNode{
			p2pClient:           p2pClient,
			roundsData:          make(map[string]RoundData),
			activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
			cvOnChain:           make(map[string]bool),
			roundSecrets:        make(map[string][][32]byte),
			roundSecret:         make(map[string]map[string]bool),
			secretsOnChain:      make(map[string]bool),
			indices:             make([]*big.Int, 0),
			revealRequestStatus: make(map[string][]string),
		}

		_, _, err := leaderNode.CreateHost("999999", "leader")
		assert.Error(t, err, "Expected error when network interface binding fails")
	})
}

func containsAny(errMsg string, keywords []string) bool {
	errMsgLower := strings.ToLower(errMsg)
	for _, keyword := range keywords {
		if strings.Contains(errMsgLower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func TestLeaderNode_AddLeaderCommit(t *testing.T) {
	ctx := context.Background()
	commitData := &utils.LeaderCommitData{
		Round:      "1",
		TrialNum:   "0",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}

	t.Run("Success", func(t *testing.T) {
		leaderNode, _, mockLeaderCommitRepo := setupLeaderNodeWithMocksForNodeTest()

		mockLeaderCommitRepo.On("AddLeaderCommit", ctx, commitData).Return(nil)

		err := leaderNode.AddLeaderCommit(ctx, commitData)

		assert.NoError(t, err)
		mockLeaderCommitRepo.AssertExpectations(t)
	})

	t.Run("Error", func(t *testing.T) {
		leaderNode, _, mockLeaderCommitRepo := setupLeaderNodeWithMocksForNodeTest()

		expectedError := assert.AnError
		mockLeaderCommitRepo.On("AddLeaderCommit", ctx, commitData).Return(expectedError)

		err := leaderNode.AddLeaderCommit(ctx, commitData)

		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		mockLeaderCommitRepo.AssertExpectations(t)
	})
}

func TestLeaderNode_GetLeaderCommitByRoundAndEoaAddr(t *testing.T) {
	ctx := context.Background()
	round := "1"
	trialNum := "0"
	eoaAddr := "0x1234567890123456789012345678901234567890"

	t.Run("Success", func(t *testing.T) {
		leaderNode, _, mockLeaderCommitRepo := setupLeaderNodeWithMocksForNodeTest()

		expectedData := &utils.LeaderCommitData{
			Round:      round,
			TrialNum:   trialNum,
			EOAAddress: eoaAddr,
		}

		mockLeaderCommitRepo.On("GetLeaderCommitByRoundAndEoaAddr", ctx, round, trialNum, eoaAddr).Return(expectedData, nil)

		result, err := leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, eoaAddr)

		assert.NoError(t, err)
		assert.Equal(t, expectedData, result)
		mockLeaderCommitRepo.AssertExpectations(t)
	})

	t.Run("Not found", func(t *testing.T) {
		leaderNode, _, mockLeaderCommitRepo := setupLeaderNodeWithMocksForNodeTest()

		expectedError := assert.AnError
		mockLeaderCommitRepo.On("GetLeaderCommitByRoundAndEoaAddr", ctx, round, trialNum, eoaAddr).Return(nil, expectedError)

		result, err := leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, eoaAddr)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockLeaderCommitRepo.AssertExpectations(t)
	})
}

func TestLeaderNode_UpdateLeaderCommit(t *testing.T) {
	ctx := context.Background()
	commitData := &utils.LeaderCommitData{
		Round:      "1",
		TrialNum:   "0",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}

	t.Run("Success", func(t *testing.T) {
		leaderNode, _, mockLeaderCommitRepo := setupLeaderNodeWithMocksForNodeTest()

		mockLeaderCommitRepo.On("UpdateLeaderCommit", ctx, commitData).Return(nil)

		err := leaderNode.UpdateLeaderCommit(ctx, commitData)

		assert.NoError(t, err)
		mockLeaderCommitRepo.AssertExpectations(t)
	})

	t.Run("Error", func(t *testing.T) {
		leaderNode, _, mockLeaderCommitRepo := setupLeaderNodeWithMocksForNodeTest()

		expectedError := assert.AnError
		mockLeaderCommitRepo.On("UpdateLeaderCommit", ctx, commitData).Return(expectedError)

		err := leaderNode.UpdateLeaderCommit(ctx, commitData)

		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		mockLeaderCommitRepo.AssertExpectations(t)
	})
}

func TestLeaderNode_DetermineRevealOrder(t *testing.T) {
	ctx := context.Background()
	round := "1"
	trialNum := "0"
	activatedOps := []common.Address{
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}

	t.Run("Success", func(t *testing.T) {
		leaderNode, mockRevealOrderService, _ := setupLeaderNodeWithMocksForNodeTest()

		mockRevealOrderService.On("DetermineRevealOrder", ctx, round, trialNum, activatedOps).Return(true, nil)

		result, err := leaderNode.DetermineRevealOrder(ctx, round, trialNum, activatedOps)

		assert.NoError(t, err)
		assert.True(t, result)
		mockRevealOrderService.AssertExpectations(t)
	})

	t.Run("Error", func(t *testing.T) {
		leaderNode, mockRevealOrderService, _ := setupLeaderNodeWithMocksForNodeTest()

		expectedError := assert.AnError
		mockRevealOrderService.On("DetermineRevealOrder", ctx, round, trialNum, activatedOps).Return(false, expectedError)

		result, err := leaderNode.DetermineRevealOrder(ctx, round, trialNum, activatedOps)

		assert.Error(t, err)
		assert.False(t, result)
		assert.Equal(t, expectedError, err)
		mockRevealOrderService.AssertExpectations(t)
	})
}

func TestNewLeaderNode(t *testing.T) {
	// Create mock repositories
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	mockRevealOrderService := new(MockRevealOrderService)
	mockNodeInfoRepo := new(MockNodeInfoRepo)
	p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
	mockLeaderCommitRepo := new(MockLeaderCommitRepo)
	mockBatchRepo := &database.BatchRepository{}
	mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
	mockRevealOrderRepo := new(MockRevealOrderRepo)

	// Create LeaderNode
	leaderNode := NewLeaderNode(
		mockFallbackClient,
		mockRevealOrderService,
		p2pClient,
		mockLeaderCommitRepo,
		mockBatchRepo,
		mockBroadcastTrackerRepo,
		mockRevealOrderRepo,
		mockNodeInfoRepo,
	)

	// Assertions
	assert.NotNil(t, leaderNode)
	assert.NotNil(t, leaderNode.fallbackEthClient)
	assert.NotNil(t, leaderNode.leaderCommitRepository)
	assert.NotNil(t, leaderNode.revealOrderService)
	assert.NotNil(t, leaderNode.batchRepository)
	assert.NotNil(t, leaderNode.broadcastTrackerRepository)
	assert.NotNil(t, leaderNode.reavealOrderRepository)
	assert.NotNil(t, leaderNode.nodeInfoRepository)
	assert.NotNil(t, leaderNode.ethService)
	assert.Equal(t, eth.Service, leaderNode.ethService)
	assert.NotNil(t, leaderNode.roundsData)
	assert.NotNil(t, leaderNode.activeBroadcasts)
	assert.NotNil(t, leaderNode.cvOnChain)
	assert.NotNil(t, leaderNode.roundSecrets)
	assert.NotNil(t, leaderNode.roundSecret)
	assert.NotNil(t, leaderNode.secretsOnChain)
	assert.NotNil(t, leaderNode.indices)
	assert.NotNil(t, leaderNode.revealRequestStatus)
}

// Test cases for Database Connection Failures
func TestNewLeaderNode_DatabaseConnectionFailures(t *testing.T) {
	ctx := context.Background()
	commitData := &utils.LeaderCommitData{
		Round:      "1",
		TrialNum:   "0",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}

	t.Run("AddLeaderCommit with nil database in repository", func(t *testing.T) {
		// Create repository with nil database
		repo := database.NewLeaderCommitRepository(nil)

		mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
		mockRevealOrderService := new(MockRevealOrderService)
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
		mockBatchRepo := &database.BatchRepository{}
		mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
		mockRevealOrderRepo := new(MockRevealOrderRepo)

		leaderNode := NewLeaderNode(
			mockFallbackClient,
			mockRevealOrderService,
			p2pClient,
			repo,
			mockBatchRepo,
			mockBroadcastTrackerRepo,
			mockRevealOrderRepo,
			mockNodeInfoRepo,
		)

		assert.Panics(t, func() {
			_ = leaderNode.AddLeaderCommit(ctx, commitData)
		}, "Expected panic when database is nil")
	})

	t.Run("GetLeaderCommitByRoundAndEoaAddr with nil database", func(t *testing.T) {
		repo := database.NewLeaderCommitRepository(nil)

		mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
		mockRevealOrderService := new(MockRevealOrderService)
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
		mockBatchRepo := &database.BatchRepository{}
		mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
		mockRevealOrderRepo := new(MockRevealOrderRepo)

		leaderNode := NewLeaderNode(
			mockFallbackClient,
			mockRevealOrderService,
			p2pClient,
			repo,
			mockBatchRepo,
			mockBroadcastTrackerRepo,
			mockRevealOrderRepo,
			mockNodeInfoRepo,
		)

		assert.Panics(t, func() {
			_, _ = leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, "1", "0", "0x1234567890123456789012345678901234567890")
		}, "Expected panic when database is nil")
	})

	t.Run("UpdateLeaderCommit with nil database", func(t *testing.T) {
		// Create repository with nil database
		repo := database.NewLeaderCommitRepository(nil)

		mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
		mockRevealOrderService := new(MockRevealOrderService)
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
		mockBatchRepo := &database.BatchRepository{}
		mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
		mockRevealOrderRepo := new(MockRevealOrderRepo)

		leaderNode := NewLeaderNode(
			mockFallbackClient,
			mockRevealOrderService,
			p2pClient,
			repo,
			mockBatchRepo,
			mockBroadcastTrackerRepo,
			mockRevealOrderRepo,
			mockNodeInfoRepo,
		)

		// Attempt to update commit
		assert.Panics(t, func() {
			_ = leaderNode.UpdateLeaderCommit(ctx, commitData)
		}, "Expected panic when database is nil")
	})

	t.Run("AddLeaderCommit with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		repo := database.NewLeaderCommitRepository(invalidDB)

		mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
		mockRevealOrderService := new(MockRevealOrderService)
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
		mockBatchRepo := &database.BatchRepository{}
		mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
		mockRevealOrderRepo := new(MockRevealOrderRepo)

		leaderNode := NewLeaderNode(
			mockFallbackClient,
			mockRevealOrderService,
			p2pClient,
			repo,
			mockBatchRepo,
			mockBroadcastTrackerRepo,
			mockRevealOrderRepo,
			mockNodeInfoRepo,
		)

		err := leaderNode.AddLeaderCommit(ctx, commitData)
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("GetLeaderCommitByRoundAndEoaAddr with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "127.0.0.1:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "test",
		})
		defer invalidDB.Close()

		repo := database.NewLeaderCommitRepository(invalidDB)

		mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
		mockRevealOrderService := new(MockRevealOrderService)
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
		mockBatchRepo := &database.BatchRepository{}
		mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
		mockRevealOrderRepo := new(MockRevealOrderRepo)

		leaderNode := NewLeaderNode(
			mockFallbackClient,
			mockRevealOrderService,
			p2pClient,
			repo,
			mockBatchRepo,
			mockBroadcastTrackerRepo,
			mockRevealOrderRepo,
			mockNodeInfoRepo,
		)

		result, err := leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, "1", "0", "0x1234567890123456789012345678901234567890")
		assert.Error(t, err, "Expected error with invalid database connection")
		assert.Nil(t, result)
	})

	t.Run("UpdateLeaderCommit with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid:5432",
			User:     "postgres",
			Password: "wrong",
			Database: "test",
		})
		defer invalidDB.Close()

		repo := database.NewLeaderCommitRepository(invalidDB)

		mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
		mockRevealOrderService := new(MockRevealOrderService)
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
		mockBatchRepo := &database.BatchRepository{}
		mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
		mockRevealOrderRepo := new(MockRevealOrderRepo)

		leaderNode := NewLeaderNode(
			mockFallbackClient,
			mockRevealOrderService,
			p2pClient,
			repo,
			mockBatchRepo,
			mockBroadcastTrackerRepo,
			mockRevealOrderRepo,
			mockNodeInfoRepo,
		)

		err := leaderNode.UpdateLeaderCommit(ctx, commitData)
		assert.Error(t, err, "Expected error with invalid database connection")
	})
}
func TestNewLeaderNodeHandler_DatabaseConnectionFailures(t *testing.T) {
	t.Run("NewLeaderNodeHandler with nil database", func(t *testing.T) {
		handler := NewLeaderNodeHandler(nil, nil)

		assert.NotNil(t, handler, "Handler should be created even with nil database")
		assert.NotNil(t, handler.leaderNode, "LeaderNode should be created")
		ctx := context.Background()
		commitData := &utils.LeaderCommitData{
			Round:      "1",
			TrialNum:   "0",
			EOAAddress: "0x1234567890123456789012345678901234567890",
		}
		assert.Panics(t, func() {
			_ = handler.leaderNode.AddLeaderCommit(ctx, commitData)
		}, "Expected panic when database is nil")
	})

	t.Run("NewLeaderNodeHandler with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		handler := NewLeaderNodeHandler(nil, invalidDB)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler.leaderNode)

		ctx := context.Background()
		commitData := &utils.LeaderCommitData{
			Round:      "1",
			TrialNum:   "0",
			EOAAddress: "0x1234567890123456789012345678901234567890",
		}

		err := handler.leaderNode.AddLeaderCommit(ctx, commitData)
		assert.Error(t, err, "Expected error with invalid database connection")
	})
}

func TestNewLeaderNode_RuntimeDatabaseDisconnection(t *testing.T) {
	ctx := context.Background()
	commitData := &utils.LeaderCommitData{
		Round:      "1",
		TrialNum:   "0",
		EOAAddress: "0x1234567890123456789012345678901234567890",
	}

	const (
		postgresHost     = "localhost"
		postgresUser     = "postgres"
		postgresPassword = "123"
		postgresDB       = "testdb"
		postgresPort     = "5433"
	)

	t.Run("AddLeaderCommit with database disconnect during operation", func(t *testing.T) {
		testDB := pg.Connect(&pg.Options{
			Addr:     postgresHost + ":" + postgresPort,
			User:     postgresUser,
			Password: postgresPassword,
			Database: postgresDB,
		})

		if err := testDB.Ping(ctx); err != nil {
			testDB.Close()
			t.Skip("Skipping test: PostgreSQL database not available:", err)
			return
		}

		repo := database.NewLeaderCommitRepository(testDB)

		mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
		mockRevealOrderService := new(MockRevealOrderService)
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
		mockBatchRepo := &database.BatchRepository{}
		mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
		mockRevealOrderRepo := new(MockRevealOrderRepo)

		leaderNode := NewLeaderNode(
			mockFallbackClient,
			mockRevealOrderService,
			p2pClient,
			repo,
			mockBatchRepo,
			mockBroadcastTrackerRepo,
			mockRevealOrderRepo,
			mockNodeInfoRepo,
		)

		err := leaderNode.AddLeaderCommit(ctx, commitData)
		if err != nil {
			testDB.Close()
			t.Skip("Skipping test: Initial database operation failed:", err)
			return
		}

		testDB.Close()

		err = leaderNode.AddLeaderCommit(ctx, commitData)
		assert.Error(t, err, "Expected error when database is disconnected during operation")
	})

	t.Run("GetLeaderCommitByRoundAndEoaAddr with database disconnect during operation", func(t *testing.T) {
		testDB := pg.Connect(&pg.Options{
			Addr:     postgresHost + ":" + postgresPort,
			User:     postgresUser,
			Password: postgresPassword,
			Database: postgresDB,
		})

		if err := testDB.Ping(ctx); err != nil {
			testDB.Close()
			t.Skip("Skipping test: PostgreSQL database not available:", err)
			return
		}

		repo := database.NewLeaderCommitRepository(testDB)

		mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
		mockRevealOrderService := new(MockRevealOrderService)
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
		mockBatchRepo := &database.BatchRepository{}
		mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
		mockRevealOrderRepo := new(MockRevealOrderRepo)

		leaderNode := NewLeaderNode(
			mockFallbackClient,
			mockRevealOrderService,
			p2pClient,
			repo,
			mockBatchRepo,
			mockBroadcastTrackerRepo,
			mockRevealOrderRepo,
			mockNodeInfoRepo,
		)

		testDB.Close()
		result, err := leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, "1", "0", "0x1234567890123456789012345678901234567890")
		assert.Error(t, err, "Expected error when database is disconnected during read operation")
		assert.Nil(t, result, "Result should be nil when database is disconnected")
	})

	t.Run("UpdateLeaderCommit with database disconnect during operation", func(t *testing.T) {
		testDB := pg.Connect(&pg.Options{
			Addr:     postgresHost + ":" + postgresPort,
			User:     postgresUser,
			Password: postgresPassword,
			Database: postgresDB,
		})
		if err := testDB.Ping(ctx); err != nil {
			testDB.Close()
			t.Skip("Skipping test: PostgreSQL database not available:", err)
			return
		}

		repo := database.NewLeaderCommitRepository(testDB)

		mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
		mockRevealOrderService := new(MockRevealOrderService)
		mockNodeInfoRepo := new(MockNodeInfoRepo)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)
		mockBatchRepo := &database.BatchRepository{}
		mockBroadcastTrackerRepo := new(MockBroadcastTrackerRepo)
		mockRevealOrderRepo := new(MockRevealOrderRepo)

		leaderNode := NewLeaderNode(
			mockFallbackClient,
			mockRevealOrderService,
			p2pClient,
			repo,
			mockBatchRepo,
			mockBroadcastTrackerRepo,
			mockRevealOrderRepo,
			mockNodeInfoRepo,
		)
		testDB.Close()
		err := leaderNode.UpdateLeaderCommit(ctx, commitData)
		assert.Error(t, err, "Expected error when database is disconnected during update operation")
	})
}

func TestNewLeaderNode_BatchRepository_DatabaseConnectionFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteOldRoundDataForLeaderNode with nil database", func(t *testing.T) {
		batchRepo := database.NewBatchRepository(nil)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, batchRepo, nil, nil, nil)

		assert.Panics(t, func() {
			_ = leaderNode.batchRepository.DeleteOldRoundDataForLeaderNode(ctx, "1")
		}, "Expected panic when database is nil")
	})

	t.Run("DeleteOldRoundDataForLeaderNode with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		batchRepo := database.NewBatchRepository(invalidDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, batchRepo, nil, nil, nil)

		err := leaderNode.batchRepository.DeleteOldRoundDataForLeaderNode(ctx, "1")
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("DeleteRoundTrialDataForLeaderNode with nil database", func(t *testing.T) {
		batchRepo := database.NewBatchRepository(nil)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, batchRepo, nil, nil, nil)

		assert.Panics(t, func() {
			_ = leaderNode.batchRepository.DeleteRoundTrialDataForLeaderNode(ctx, "1", "0")
		}, "Expected panic when database is nil")
	})

	t.Run("DeleteRoundTrialDataForLeaderNode with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		batchRepo := database.NewBatchRepository(invalidDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, batchRepo, nil, nil, nil)

		err := leaderNode.batchRepository.DeleteRoundTrialDataForLeaderNode(ctx, "1", "0")
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("DeleteOldRoundDataForLeaderNode with database disconnect during operation", func(t *testing.T) {
		const (
			postgresHost     = "localhost"
			postgresUser     = "postgres"
			postgresPassword = "123"
			postgresDB       = "testdb"
			postgresPort     = "5433"
		)

		testDB := pg.Connect(&pg.Options{
			Addr:     postgresHost + ":" + postgresPort,
			User:     postgresUser,
			Password: postgresPassword,
			Database: postgresDB,
		})

		if err := testDB.Ping(ctx); err != nil {
			testDB.Close()
			t.Skip("Skipping test: PostgreSQL database not available:", err)
			return
		}

		batchRepo := database.NewBatchRepository(testDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, batchRepo, nil, nil, nil)

		testDB.Close()

		err := leaderNode.batchRepository.DeleteOldRoundDataForLeaderNode(ctx, "1")
		assert.Error(t, err, "Expected error when database is disconnected during operation")
	})
}

func TestNewLeaderNode_BroadcastTrackerRepository_DatabaseConnectionFailures(t *testing.T) {
	ctx := context.Background()
	tracker := &utils.BroadcastTracker{
		Round:        "1",
		TrialNum:     "0",
		EOAAddress:   "0x1234567890123456789012345678901234567890",
		Type:         "cvs",
		MessageID:    "msg1",
		Data:         [32]byte{},
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     0,
		Timeout:      60,
	}

	t.Run("AddBroadcastTracker with nil database", func(t *testing.T) {
		broadcastRepo := database.NewBroadcastTrackerRepository(nil)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, broadcastRepo, nil, nil)

		assert.Panics(t, func() {
			_ = leaderNode.broadcastTrackerRepository.AddBroadcastTracker(ctx, tracker)
		}, "Expected panic when database is nil")
	})

	t.Run("AddBroadcastTracker with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		broadcastRepo := database.NewBroadcastTrackerRepository(invalidDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, broadcastRepo, nil, nil)

		err := leaderNode.broadcastTrackerRepository.AddBroadcastTracker(ctx, tracker)
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("UpdateBroadcastTracker with nil database", func(t *testing.T) {
		broadcastRepo := database.NewBroadcastTrackerRepository(nil)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, broadcastRepo, nil, nil)

		assert.Panics(t, func() {
			_ = leaderNode.broadcastTrackerRepository.UpdateBroadcastTracker(ctx, tracker)
		}, "Expected panic when database is nil")
	})

	t.Run("UpdateBroadcastTracker with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		broadcastRepo := database.NewBroadcastTrackerRepository(invalidDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, broadcastRepo, nil, nil)

		err := leaderNode.broadcastTrackerRepository.UpdateBroadcastTracker(ctx, tracker)
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("AddBroadcastTracker with database disconnect during operation", func(t *testing.T) {
		const (
			postgresHost     = "localhost"
			postgresUser     = "postgres"
			postgresPassword = "123"
			postgresDB       = "testdb"
			postgresPort     = "5433"
		)

		testDB := pg.Connect(&pg.Options{
			Addr:     postgresHost + ":" + postgresPort,
			User:     postgresUser,
			Password: postgresPassword,
			Database: postgresDB,
		})

		if err := testDB.Ping(ctx); err != nil {
			testDB.Close()
			t.Skip("Skipping test: PostgreSQL database not available:", err)
			return
		}

		broadcastRepo := database.NewBroadcastTrackerRepository(testDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, broadcastRepo, nil, nil)

		testDB.Close()

		err := leaderNode.broadcastTrackerRepository.AddBroadcastTracker(ctx, tracker)
		assert.Error(t, err, "Expected error when database is disconnected during operation")
	})
}
func TestNewLeaderNode_NodeInfoRepository_DatabaseConnectionFailures(t *testing.T) {
	ctx := context.Background()
	nodeInfo := &utils.NodeInfo{
		IP:         "127.0.0.1",
		Port:       "4001",
		PeerID:     "peer123",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		PrivateKey: []byte{},
	}

	t.Run("AddAndUpdateNodeInfo with nil database", func(t *testing.T) {
		nodeInfoRepo := database.NewNodeInfoRepository(nil)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, nil, nodeInfoRepo)

		assert.Panics(t, func() {
			_ = leaderNode.nodeInfoRepository.AddAndUpdateNodeInfo(ctx, nodeInfo)
		}, "Expected panic when database is nil")
	})

	t.Run("AddAndUpdateNodeInfo with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		nodeInfoRepo := database.NewNodeInfoRepository(invalidDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, nil, nodeInfoRepo)

		err := leaderNode.nodeInfoRepository.AddAndUpdateNodeInfo(ctx, nodeInfo)
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("GetNodeInfos with nil database", func(t *testing.T) {
		nodeInfoRepo := database.NewNodeInfoRepository(nil)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, nil, nodeInfoRepo)

		assert.Panics(t, func() {
			_, _ = leaderNode.nodeInfoRepository.GetNodeInfos(ctx)
		}, "Expected panic when database is nil")
	})

	t.Run("GetNodeInfos with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		nodeInfoRepo := database.NewNodeInfoRepository(invalidDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, nil, nodeInfoRepo)

		result, err := leaderNode.nodeInfoRepository.GetNodeInfos(ctx)
		assert.Error(t, err, "Expected error with invalid database connection")
		assert.Nil(t, result)
	})

	t.Run("DeleteNodeInfoByEOA with nil database", func(t *testing.T) {
		nodeInfoRepo := database.NewNodeInfoRepository(nil)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, nil, nodeInfoRepo)

		assert.Panics(t, func() {
			_ = leaderNode.nodeInfoRepository.DeleteNodeInfoByEOA(ctx, "0x1234567890123456789012345678901234567890")
		}, "Expected panic when database is nil")
	})

	t.Run("DeleteNodeInfoByEOA with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		nodeInfoRepo := database.NewNodeInfoRepository(invalidDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, nil, nodeInfoRepo)

		err := leaderNode.nodeInfoRepository.DeleteNodeInfoByEOA(ctx, "0x1234567890123456789012345678901234567890")
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("GetNodeInfos with database disconnect during operation", func(t *testing.T) {
		const (
			postgresHost     = "localhost"
			postgresUser     = "postgres"
			postgresPassword = "123"
			postgresDB       = "testdb"
			postgresPort     = "5433"
		)

		testDB := pg.Connect(&pg.Options{
			Addr:     postgresHost + ":" + postgresPort,
			User:     postgresUser,
			Password: postgresPassword,
			Database: postgresDB,
		})

		if err := testDB.Ping(ctx); err != nil {
			testDB.Close()
			t.Skip("Skipping test: PostgreSQL database not available:", err)
			return
		}

		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, nil, nodeInfoRepo)

		testDB.Close()

		result, err := leaderNode.nodeInfoRepository.GetNodeInfos(ctx)
		assert.Error(t, err, "Expected error when database is disconnected during operation")
		assert.Nil(t, result)
	})
}
func TestNewLeaderNode_RevealOrderRepository_DatabaseConnectionFailures(t *testing.T) {
	ctx := context.Background()
	revealOrder := &utils.RevealOrderData{
		Round:        "1",
		TrialNum:     "0",
		OrderedNodes: []string{"node1", "node2"},
		RevealOrder:  []int{0, 1},
		RV:           "rv123",
	}

	t.Run("GetRevealOrder with nil database", func(t *testing.T) {
		revealOrderRepo := database.NewRevealOrderRepository(nil)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, revealOrderRepo, nil)

		assert.Panics(t, func() {
			_, _ = leaderNode.reavealOrderRepository.GetRevealOrder(ctx, "1", "0")
		}, "Expected panic when database is nil")
	})

	t.Run("GetRevealOrder with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		revealOrderRepo := database.NewRevealOrderRepository(invalidDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, revealOrderRepo, nil)

		result, err := leaderNode.reavealOrderRepository.GetRevealOrder(ctx, "1", "0")
		assert.Error(t, err, "Expected error with invalid database connection")
		assert.Nil(t, result)
	})

	t.Run("AddRevealOrder with nil database", func(t *testing.T) {
		revealOrderRepo := database.NewRevealOrderRepository(nil)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, revealOrderRepo, nil)

		assert.Panics(t, func() {
			_ = leaderNode.reavealOrderRepository.AddRevealOrder(ctx, revealOrder)
		}, "Expected panic when database is nil")
	})

	t.Run("AddRevealOrder with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		revealOrderRepo := database.NewRevealOrderRepository(invalidDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, revealOrderRepo, nil)

		err := leaderNode.reavealOrderRepository.AddRevealOrder(ctx, revealOrder)
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("GetRevealOrder with database disconnect during operation", func(t *testing.T) {
		const (
			postgresHost     = "localhost"
			postgresUser     = "postgres"
			postgresPassword = "123"
			postgresDB       = "testdb"
			postgresPort     = "5433"
		)

		testDB := pg.Connect(&pg.Options{
			Addr:     postgresHost + ":" + postgresPort,
			User:     postgresUser,
			Password: postgresPassword,
			Database: postgresDB,
		})

		if err := testDB.Ping(ctx); err != nil {
			testDB.Close()
			t.Skip("Skipping test: PostgreSQL database not available:", err)
			return
		}

		revealOrderRepo := database.NewRevealOrderRepository(testDB)
		leaderNode := NewLeaderNode(nil, nil, nil, nil, nil, nil, revealOrderRepo, nil)

		testDB.Close()

		result, err := leaderNode.reavealOrderRepository.GetRevealOrder(ctx, "1", "0")
		assert.Error(t, err, "Expected error when database is disconnected during operation")
		assert.Nil(t, result)
	})
}
func TestNewLeaderNode_ConstructorNilValidation(t *testing.T) {
	t.Run("NewLeaderNode with nil repositories", func(t *testing.T) {
		leaderNode := NewLeaderNode(
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		)
		assert.NotNil(t, leaderNode, "LeaderNode should be created even with nil repositories")
		assert.Nil(t, leaderNode.leaderCommitRepository, "Repository should be nil")
		assert.Nil(t, leaderNode.batchRepository, "BatchRepository should be nil")
		assert.Nil(t, leaderNode.broadcastTrackerRepository, "BroadcastTrackerRepository should be nil")
		assert.Nil(t, leaderNode.reavealOrderRepository, "RevealOrderRepository should be nil")
		assert.Nil(t, leaderNode.nodeInfoRepository, "NodeInfoRepository should be nil")
	})

	t.Run("NewLeaderNode with partial nil repositories", func(t *testing.T) {
		mockLeaderCommitRepo := new(MockLeaderCommitRepo)
		mockBatchRepo := &database.BatchRepository{}

		leaderNode := NewLeaderNode(
			nil,
			nil,
			nil,
			mockLeaderCommitRepo,
			mockBatchRepo,
			nil,
			nil,
			nil,
		)

		assert.NotNil(t, leaderNode)
		assert.NotNil(t, leaderNode.leaderCommitRepository)
		assert.NotNil(t, leaderNode.batchRepository)
		assert.Nil(t, leaderNode.broadcastTrackerRepository)
		assert.Nil(t, leaderNode.reavealOrderRepository)
		assert.Nil(t, leaderNode.nodeInfoRepository)
	})
}

func TestNewLeaderNode_P2PClientCreationWithInvalidDatabase(t *testing.T) {
	t.Run("P2PClient creation with nil nodeInfoRepository database", func(t *testing.T) {
		nodeInfoRepo := database.NewNodeInfoRepository(nil)

		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)
		assert.NotNil(t, p2pClient, "P2PClient should be created even with nil DB repository")

		ctx := context.Background()
		assert.Panics(t, func() {
			_, _ = nodeInfoRepo.GetNodeInfos(ctx)
		}, "NodeInfoRepository operations should panic with nil database")
	})

	t.Run("P2PClient creation with invalid nodeInfoRepository database", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		nodeInfoRepo := database.NewNodeInfoRepository(invalidDB)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		assert.NotNil(t, p2pClient, "P2PClient should be created even with invalid DB")

		ctx := context.Background()
		result, err := nodeInfoRepo.GetNodeInfos(ctx)
		assert.Error(t, err, "Expected error with invalid database connection")
		assert.Nil(t, result)
	})
}

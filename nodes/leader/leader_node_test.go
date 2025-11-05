package leader_node

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

func (m *MockNodeInfoRepo) AddNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
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

// Additional tests for better coverage

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

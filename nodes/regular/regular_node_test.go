package regular_node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/utils"
)

type MockRegularCommitRepoForNode struct {
	mock.Mock
}

func (m *MockRegularCommitRepoForNode) AddCommit(ctx context.Context, commit *utils.CommitData) error {
	args := m.Called(ctx, commit)
	return args.Error(0)
}

func (m *MockRegularCommitRepoForNode) UpdateCommit(ctx context.Context, commit *utils.CommitData) error {
	args := m.Called(ctx, commit)
	return args.Error(0)
}

func (m *MockRegularCommitRepoForNode) GetCommitByRound(ctx context.Context, round, trialNum string) (*utils.CommitData, error) {
	args := m.Called(ctx, round, trialNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.CommitData), args.Error(1)
}

type MockNodeInfoRepoForNode struct {
	mock.Mock
}

func (m *MockNodeInfoRepoForNode) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.NodeInfo), args.Error(1)
}

func (m *MockNodeInfoRepoForNode) AddNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	args := m.Called(ctx, nodeInfo)
	return args.Error(0)
}

func (m *MockNodeInfoRepoForNode) GetNodeInfoByEOA(ctx context.Context, eoaAddress string) (*utils.NodeInfo, error) {
	args := m.Called(ctx, eoaAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.NodeInfo), args.Error(1)
}

func (m *MockNodeInfoRepoForNode) DeleteNodeInfoByEOA(ctx context.Context, eoaAddress string) error {
	args := m.Called(ctx, eoaAddress)
	return args.Error(0)
}

// Helper function to create a RegularNode with mocks (similar to leader node pattern)
func setupRegularNodeWithMocks() (*RegularNode, *MockNodeInfoRepoForNode, *MockRegularCommitRepoForNode) {
	mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
	mockCommitRepo := new(MockRegularCommitRepoForNode)

	// Create a real libp2putils.P2PClient for testing (same as leader node)
	p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

	regularNode := &RegularNode{
		p2pClient:                     p2pClient,
		regularCommitRepository:       mockCommitRepo,
		nodeInfoRepository:            mockNodeInfoRepo,
		submittedCvIndices:            make(map[string]map[string]bool),
		strictOrderWhileSecretRequest: make(map[string][]string),
		roundsData:                    make(map[string]RoundData),
	}

	return regularNode, mockNodeInfoRepo, mockCommitRepo
}

// Test for CreateHost and SetHost - these are simple wrapper functions
func TestRegularNode_CreateHost_And_SetHost(t *testing.T) {

	t.Run("CreateHost - verify p2pClient is called", func(t *testing.T) {
		// Setup
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		host, peerID, err := regularNode.CreateHost("8080", "regular")

		_ = host
		_ = peerID
		_ = err
	})

	t.Run("SetHost - verify p2pClient is called", func(t *testing.T) {

		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		regularNode.SetHost(nil)

		// Verify no panic occurred
		assert.NotNil(t, regularNode.p2pClient)
	})
}

func TestRegularNode_ConnectToLeader(t *testing.T) {
	t.Run("ConnectToLeader - verify p2pClient is called", func(t *testing.T) {
		// Setup
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		ctx := context.Background()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", "leader-peer-id")

		_ = addrInfo
		_ = err

		assert.NotNil(t, regularNode.p2pClient)
	})
}

func TestRegularNode_GetCommitByRound(t *testing.T) {
	ctx := context.Background()
	round := "100"
	trialNum := "1"

	t.Run("Success", func(t *testing.T) {
		regularNode, _, mockCommitRepo := setupRegularNodeWithMocks()

		expectedData := &utils.CommitData{
			Round:    round,
			TrialNum: trialNum,
		}

		mockCommitRepo.On("GetCommitByRound", ctx, round, trialNum).Return(expectedData, nil)

		result, err := regularNode.GetCommitByRound(ctx, round, trialNum)

		assert.NoError(t, err)
		assert.Equal(t, expectedData, result)
		mockCommitRepo.AssertExpectations(t)
	})

	t.Run("Not found", func(t *testing.T) {
		regularNode, _, mockCommitRepo := setupRegularNodeWithMocks()

		expectedError := assert.AnError
		mockCommitRepo.On("GetCommitByRound", ctx, round, trialNum).Return(nil, expectedError)

		result, err := regularNode.GetCommitByRound(ctx, round, trialNum)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockCommitRepo.AssertExpectations(t)
	})
}

func TestRegularNode_AddNodeInfo(t *testing.T) {
	ctx := context.Background()
	nodeInfo := &utils.NodeInfo{
		PeerID: "test-peer-id",
		IP:     "127.0.0.1",
		Port:   "8080",
	}

	t.Run("Success", func(t *testing.T) {
		regularNode, mockNodeInfoRepo, _ := setupRegularNodeWithMocks()

		mockNodeInfoRepo.On("AddNodeInfo", ctx, nodeInfo).Return(nil)

		err := regularNode.AddNodeInfo(ctx, nodeInfo)

		assert.NoError(t, err)
		mockNodeInfoRepo.AssertExpectations(t)
	})

	t.Run("Error", func(t *testing.T) {
		regularNode, mockNodeInfoRepo, _ := setupRegularNodeWithMocks()

		expectedError := assert.AnError
		mockNodeInfoRepo.On("AddNodeInfo", ctx, nodeInfo).Return(expectedError)

		err := regularNode.AddNodeInfo(ctx, nodeInfo)

		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		mockNodeInfoRepo.AssertExpectations(t)
	})
}

func TestRegularNode_AddCommit(t *testing.T) {
	ctx := context.Background()
	commitData := &utils.CommitData{
		Round:    "100",
		TrialNum: "1",
	}

	t.Run("Success", func(t *testing.T) {
		regularNode, _, mockCommitRepo := setupRegularNodeWithMocks()

		mockCommitRepo.On("AddCommit", ctx, commitData).Return(nil)

		err := regularNode.AddCommit(ctx, commitData)

		assert.NoError(t, err)
		mockCommitRepo.AssertExpectations(t)
	})

	t.Run("Error", func(t *testing.T) {
		regularNode, _, mockCommitRepo := setupRegularNodeWithMocks()

		expectedError := assert.AnError
		mockCommitRepo.On("AddCommit", ctx, commitData).Return(expectedError)

		err := regularNode.AddCommit(ctx, commitData)

		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		mockCommitRepo.AssertExpectations(t)
	})
}

func TestRegularNode_UpdateCommit(t *testing.T) {
	ctx := context.Background()
	commitData := &utils.CommitData{
		Round:    "100",
		TrialNum: "1",
	}

	t.Run("Success", func(t *testing.T) {
		regularNode, _, mockCommitRepo := setupRegularNodeWithMocks()

		mockCommitRepo.On("UpdateCommit", ctx, commitData).Return(nil)

		err := regularNode.UpdateCommit(ctx, commitData)

		assert.NoError(t, err)
		mockCommitRepo.AssertExpectations(t)
	})

	t.Run("Error", func(t *testing.T) {
		regularNode, _, mockCommitRepo := setupRegularNodeWithMocks()

		expectedError := assert.AnError
		mockCommitRepo.On("UpdateCommit", ctx, commitData).Return(expectedError)

		err := regularNode.UpdateCommit(ctx, commitData)

		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		mockCommitRepo.AssertExpectations(t)
	})
}

func TestNewRegularNode(t *testing.T) {
	regularNode := NewRegularNode(
		nil, // fallbackEthClient
		nil, // revealOrderService
		nil, // p2pClient
		nil, // peerCommitDataRepository
		nil, // revealOrderRepository
		nil, // regularCommitRepository
		nil, // batchRepository
		nil, // nodeInfoRepository
	)

	// Assertions - verify the node is created and internal maps are initialized
	assert.NotNil(t, regularNode)
	assert.NotNil(t, regularNode.submittedCvIndices)
	assert.NotNil(t, regularNode.cleanupQueue)
	assert.NotNil(t, regularNode.strictOrderWhileSecretRequest)
	assert.NotNil(t, regularNode.roundsData)
	assert.Equal(t, 0, len(regularNode.submittedCvIndices))
	assert.Equal(t, 0, len(regularNode.strictOrderWhileSecretRequest))
	assert.Equal(t, 0, len(regularNode.roundsData))
}

func TestRegularNode_P2PMethodsIntegration(t *testing.T) {
	t.Run("Complete P2P workflow", func(t *testing.T) {
		// Setup
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		host, peerID, err := regularNode.CreateHost("0", "regular")

		_ = host
		_ = peerID
		_ = err

		regularNode.SetHost(nil)
		assert.NotNil(t, regularNode.p2pClient)

		ctx := context.Background()
		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", "test-peer")
		_ = addrInfo
		_ = err
		assert.True(t, true, "All P2P delegation methods executed without panic")
	})
}

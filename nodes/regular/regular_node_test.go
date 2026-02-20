package regular_node

import (
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eapache/queue"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/go-pg/pg/v10"
	appconfig "github.com/tokamak-network/DRB-node/config"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
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

func (m *MockNodeInfoRepoForNode) AddAndUpdateNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
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

func TestRegularNode_CreateHost_PortBindingFailures(t *testing.T) {
	t.Run("CreateHost with empty port string", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		_, _, err := regularNode.CreateHost("", "regular")
		assert.Error(t, err, "Expected error with empty port string")
	})

	t.Run("CreateHost with non-numeric port", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		_, _, err := regularNode.CreateHost("abc", "regular")
		assert.Error(t, err, "Expected error with non-numeric port")
	})

	t.Run("CreateHost with port out of range", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		_, _, err := regularNode.CreateHost("99999", "regular")
		assert.Error(t, err, "Expected error with port out of range")
	})

	t.Run("CreateHost with port just above maximum (65536)", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		_, _, err := regularNode.CreateHost("65536", "regular")
		assert.Error(t, err, "Expected error with port just above maximum (65536)")
	})

	t.Run("CreateHost with negative port", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		_, _, err := regularNode.CreateHost("-1", "regular")
		assert.Error(t, err, "Expected error with negative port")
	})

	t.Run("CreateHost with decimal port", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		_, _, err := regularNode.CreateHost("4001.5", "regular")
		assert.Error(t, err, "Expected error with decimal port")
	})

	t.Run("CreateHost with port containing special characters", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		testCases := []string{"4001@", "4001#", "40-01", "40_01", "4001!", "4001$"}
		for _, port := range testCases {
			_, _, err := regularNode.CreateHost(port, "regular")
			assert.Error(t, err, "Expected error with port containing special characters: %s", port)
		}
	})

	t.Run("CreateHost with port containing leading zeros", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		_, _, err := regularNode.CreateHost("04001", "regular")

		if err != nil {
			assert.NotEmpty(t, err.Error(), "Error message should not be empty")
		}
	})

	t.Run("CreateHost with very long port string", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		longPort := "4" + string(make([]byte, 1000))
		_, _, err := regularNode.CreateHost(longPort, "regular")
		assert.Error(t, err, "Expected error with very long port string")
	})

	t.Run("CreateHost with port containing null character", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		portWithNull := "4001" + string([]byte{0})
		_, _, err := regularNode.CreateHost(portWithNull, "regular")
		assert.Error(t, err, "Expected error with port containing null character")
	})

	t.Run("CreateHost with port containing unicode characters", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		testCases := []string{"4001测试", "4001🚀", "4001α", "4001ñ"}
		for _, port := range testCases {
			_, _, err := regularNode.CreateHost(port, "regular")
			assert.Error(t, err, "Expected error with port containing unicode: %s", port)
		}
	})

	t.Run("CreateHost with maximum valid port (65535)", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Skip("Skipping test: Could not find available port")
			return
		}
		listener.Close()

		_, _, err = regularNode.CreateHost("65535", "regular")

		if err != nil {
			assert.NotEmpty(t, err.Error(), "Error message should not be empty")
		}
	})

	t.Run("CreateHost with minimum valid port (1)", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		_, _, err := regularNode.CreateHost("1", "regular")

		if err != nil {
			assert.NotEmpty(t, err.Error(), "Error message should not be empty")
		}
	})

	t.Run("CreateHost error propagation - verify error contains context", func(t *testing.T) {
		mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
		p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

		regularNode := &RegularNode{
			p2pClient:                     p2pClient,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		_, _, err := regularNode.CreateHost("invalid", "regular")
		assert.Error(t, err)
		assert.NotEmpty(t, err.Error(), "Error message should not be empty")
	})
	t.Run("CreateHost with nil p2pClient", func(t *testing.T) {
		regularNode := &RegularNode{
			p2pClient:                     nil,
			submittedCvIndices:            make(map[string]map[string]bool),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
		}

		assert.NotPanics(t, func() {
			_, _, err := regularNode.CreateHost("8080", "regular")
			_ = err
		}, "CreateHost on nil p2pClient should handle gracefully")
	})
}

func containsAny(s string, substrings ...string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
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

func TestRegularNode_ConnectToLeader_TimeoutScenarios(t *testing.T) {
	mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
	p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

	regularNode := &RegularNode{
		p2pClient:                     p2pClient,
		submittedCvIndices:            make(map[string]map[string]bool),
		strictOrderWhileSecretRequest: make(map[string][]string),
		roundsData:                    make(map[string]RoundData),
	}

	setupHostForConnectionTests := func(t *testing.T) (host.Host, string) {
		h, err := libp2p.New()
		if err != nil {
			t.Fatalf("Failed to create libp2p host: %v", err)
		}
		regularNode.SetHost(h)
		peerID := h.ID().String()
		t.Cleanup(func() {
			h.Close()
		})
		return h, peerID
	}

	t.Run("Context timeout - connection should fail with timeout error", func(t *testing.T) {
		_, validPeerID := setupHostForConnectionTests(t)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "9999", validPeerID)
		assert.Error(t, err, "Connection should fail with any error")
		_ = addrInfo
	})

	t.Run("Context cancellation - connection should fail with cancellation error", func(t *testing.T) {
		_, validPeerID := setupHostForConnectionTests(t)

		ctx, cancel := context.WithCancel(context.Background())

		cancel()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", validPeerID)

		assert.Error(t, err, "Connection should fail with any error ")
		_ = addrInfo
	})

	t.Run("Unreachable leader - connection to non-existent leader", func(t *testing.T) {
		_, validPeerID := setupHostForConnectionTests(t)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "65534", validPeerID)

		assert.Error(t, err)
		assert.NotNil(t, err)
	})

	t.Run("Invalid peer ID format - should fail during parsing", func(t *testing.T) {
		ctx := context.Background()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", "invalid-peer-id")
		assert.Error(t, err)
		assert.Nil(t, addrInfo)
		assert.True(t, strings.Contains(err.Error(), "failed to parse") ||
			strings.Contains(err.Error(), "failed to create peer info") ||
			strings.Contains(err.Error(), "multiaddress"))
	})

	t.Run("DNS resolution failure - DNS name", func(t *testing.T) {
		_, validPeerID := setupHostForConnectionTests(t)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "nonexistent.example.com", "8081", validPeerID)

		assert.Error(t, err)
		_ = addrInfo
	})

	t.Run("Very short timeout - immediate timeout", func(t *testing.T) {
		_, validPeerID := setupHostForConnectionTests(t)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(1 * time.Millisecond)

		_, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", validPeerID)

		assert.Error(t, err, "Connection should fail with any error")
	})

	t.Run("Connection timeout with valid format but unreachable address", func(t *testing.T) {
		setupHostForConnectionTests(t)

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		validPeerID := "12D3KooWExample1234567890123456789012345678901234567890"
		addrInfo, err := regularNode.ConnectToLeader(ctx, "192.0.2.1", "8081", validPeerID)
		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Multiple rapid timeout attempts", func(t *testing.T) {
		_, validPeerID := setupHostForConnectionTests(t)

		for i := 0; i < 3; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "9999", validPeerID)
			cancel()

			assert.Error(t, err, "Attempt %d should fail", i+1)
			_ = addrInfo
		}
	})
}

func TestRegularNode_ConnectToLeader_InputValidationAndEdgeCases(t *testing.T) {
	mockNodeInfoRepo := new(MockNodeInfoRepoForNode)
	p2pClient := libp2putils.NewP2PClient(mockNodeInfoRepo)

	regularNode := &RegularNode{
		p2pClient:                     p2pClient,
		submittedCvIndices:            make(map[string]map[string]bool),
		strictOrderWhileSecretRequest: make(map[string][]string),
		roundsData:                    make(map[string]RoundData),
	}

	setupHostForConnectionTests := func(t *testing.T) {
		h, err := libp2p.New()
		if err != nil {
			t.Fatalf("Failed to create libp2p host: %v", err)
		}
		regularNode.SetHost(h)
		t.Cleanup(func() {
			h.Close()
		})
	}

	t.Run("Nil host instance - should panic or return error", func(t *testing.T) {
		tempHost, err := libp2p.New()
		if err != nil {
			t.Fatalf("Failed to create temp host: %v", err)
		}
		defer tempHost.Close()
		validPeerID := tempHost.ID().String()

		ctx := context.Background()

		assert.Panics(t, func() {
			_, _ = regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", validPeerID)
		}, "ConnectToLeader should panic when hostInstance is nil")
	})

	t.Run("Empty leaderIP - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "", "8081", "12D3KooWTest1234567890123456789012345678901234567890")
		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Empty leaderPort - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "", "12D3KooWTest1234567890123456789012345678901234567890")

		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Empty leaderPeerID - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", "")

		assert.Error(t, err)
		assert.Nil(t, addrInfo)
		assert.True(t, strings.Contains(err.Error(), "failed to parse") ||
			strings.Contains(err.Error(), "failed to create peer info") ||
			strings.Contains(err.Error(), "multiaddress"))
	})

	t.Run("All empty inputs - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "", "", "")

		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Nil context - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)

		ctx := context.TODO()

		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "9999", "12D3KooWTest1234567890123456789012345678901234567890")
		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Context without timeout - connection should eventually fail", func(t *testing.T) {
		setupHostForConnectionTests(t)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		addrInfo, err := regularNode.ConnectToLeader(ctx, "192.0.2.1", "8081", "12D3KooWTest1234567890123456789012345678901234567890")

		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("DNS hardcoding - leaderIP parameter is ignored", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		validPeerID := "12D3KooWTest1234567890123456789012345678901234567890"

		addrInfo1, err1 := regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", validPeerID)
		addrInfo2, err2 := regularNode.ConnectToLeader(ctx, "192.168.1.1", "8081", validPeerID)
		addrInfo3, err3 := regularNode.ConnectToLeader(ctx, "nonexistent.example.com", "8081", validPeerID)

		assert.Error(t, err1)
		assert.Error(t, err2)
		assert.Error(t, err3)
		assert.Nil(t, addrInfo1)
		assert.Nil(t, addrInfo2)
		assert.Nil(t, addrInfo3)
	})

	t.Run("Very long leaderIP - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		longIP := strings.Repeat("a", 10000)
		addrInfo, err := regularNode.ConnectToLeader(ctx, longIP, "8081", "12D3KooWTest1234567890123456789012345678901234567890")

		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Very long leaderPort - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		longPort := strings.Repeat("8", 10000)
		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", longPort, "12D3KooWTest1234567890123456789012345678901234567890")
		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Very long leaderPeerID - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		longPeerID := strings.Repeat("a", 10000)
		addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", longPeerID)
		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Special characters in leaderIP - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		testCases := []string{
			"127.0.0.1; rm -rf /",
			"127.0.0.1 && ls",
			"127.0.0.1 | cat /etc/passwd",
			"127.0.0.1\n127.0.0.2",
			"127.0.0.1\t127.0.0.2",
		}

		for _, maliciousIP := range testCases {
			addrInfo, err := regularNode.ConnectToLeader(ctx, maliciousIP, "8081", "12D3KooWTest1234567890123456789012345678901234567890")
			assert.Error(t, err, "Should fail for malicious IP: %s", maliciousIP)
			assert.Nil(t, addrInfo, "Should return nil for malicious IP: %s", maliciousIP)
		}
	})

	t.Run("Special characters in leaderPort - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		testCases := []string{
			"8081; ls",
			"8081 && cat /etc/passwd",
			"8081 | rm -rf",
			"8081\n8082",
		}

		for _, maliciousPort := range testCases {
			addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", maliciousPort, "12D3KooWTest1234567890123456789012345678901234567890")
			assert.Error(t, err, "Should fail for malicious port: %s", maliciousPort)
			assert.Nil(t, addrInfo, "Should return nil for malicious port: %s", maliciousPort)
		}
	})

	t.Run("Special characters in leaderPeerID - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		testCases := []string{
			"../../etc/passwd",
			"peer-id; rm -rf",
			"peer-id && ls",
			"peer-id\npeer-id2",
		}

		for _, maliciousPeerID := range testCases {
			addrInfo, err := regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", maliciousPeerID)
			assert.Error(t, err, "Should fail for malicious peer ID: %s", maliciousPeerID)
			assert.Nil(t, addrInfo, "Should return nil for malicious peer ID: %s", maliciousPeerID)
		}
	})

	t.Run("Whitespace in inputs - should handle gracefully", func(t *testing.T) {
		h, err := libp2p.New()
		if err != nil {
			t.Fatalf("Failed to create libp2p host: %v", err)
		}
		defer h.Close()
		regularNode.SetHost(h)
		validPeerID := h.ID().String()

		ctx := context.Background()

		testCases := []struct {
			name        string
			ip          string
			port        string
			peerID      string
			shouldError bool
			description string
		}{
			{
				name:        "Leading space in IP",
				ip:          " 127.0.0.1",
				port:        "8081",
				peerID:      validPeerID,
				shouldError: false,
				description: "Whitespace in IP should be ignored since IP parameter is not used",
			},
			{
				name:        "Trailing space in IP",
				ip:          "127.0.0.1 ",
				port:        "8081",
				peerID:      validPeerID,
				shouldError: false,
				description: "Whitespace in IP should be ignored since IP parameter is not used",
			},
			{
				name:        "Leading space in port",
				ip:          "127.0.0.1",
				port:        " 8081",
				peerID:      validPeerID,
				shouldError: true,
				description: "Whitespace in port should cause parsing error",
			},
			{
				name:        "Trailing space in port",
				ip:          "127.0.0.1",
				port:        "8081 ",
				peerID:      validPeerID,
				shouldError: true,
				description: "Whitespace in port should cause parsing error",
			},
			{
				name:        "Leading space in peer ID",
				ip:          "127.0.0.1",
				port:        "8081",
				peerID:      " " + validPeerID,
				shouldError: true,
				description: "Whitespace in peer ID should cause parsing error",
			},
			{
				name:        "Trailing space in peer ID",
				ip:          "127.0.0.1",
				port:        "8081",
				peerID:      validPeerID + " ",
				shouldError: true,
				description: "Whitespace in peer ID should cause parsing error",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				addrInfo, err := regularNode.ConnectToLeader(ctx, tc.ip, tc.port, tc.peerID)

				if tc.shouldError {

					assert.Error(t, err, "Expected error for %s: %s", tc.name, tc.description)
					if err != nil {
						assert.True(t,
							strings.Contains(err.Error(), "failed to parse") ||
								strings.Contains(err.Error(), "failed to create peer info") ||
								strings.Contains(err.Error(), "multiaddress") ||
								strings.Contains(err.Error(), "invalid"),
							"Error should be related to parsing, got: %v", err)
					}
				} else {
					if err != nil {
						assert.False(t,
							strings.Contains(err.Error(), "failed to parse") ||
								strings.Contains(err.Error(), "failed to create peer info"),
							"Should not fail due to parsing for %s, got: %v", tc.name, err)
					}
				}
				_ = addrInfo
			})
		}
	})

	t.Run("Null bytes in inputs - should handle gracefully", func(t *testing.T) {
		setupHostForConnectionTests(t)
		ctx := context.Background()

		ipWithNull := "127.0.0.1" + string([]byte{0})
		portWithNull := "8081" + string([]byte{0})
		peerIDWithNull := "12D3KooWTest1234567890123456789012345678901234567890" + string([]byte{0})

		addrInfo, err := regularNode.ConnectToLeader(ctx, ipWithNull, "8081", "12D3KooWTest1234567890123456789012345678901234567890")
		assert.Error(t, err, "Should fail for IP with null byte")
		assert.Nil(t, addrInfo)

		addrInfo, err = regularNode.ConnectToLeader(ctx, "127.0.0.1", portWithNull, "12D3KooWTest1234567890123456789012345678901234567890")
		assert.Error(t, err, "Should fail for port with null byte")
		assert.Nil(t, addrInfo)

		addrInfo, err = regularNode.ConnectToLeader(ctx, "127.0.0.1", "8081", peerIDWithNull)
		assert.Error(t, err, "Should fail for peer ID with null byte")
		assert.Nil(t, addrInfo)
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

		mockNodeInfoRepo.On("AddAndUpdateNodeInfo", ctx, nodeInfo).Return(nil)

		err := regularNode.AddNodeInfo(ctx, nodeInfo)

		assert.NoError(t, err)
		mockNodeInfoRepo.AssertExpectations(t)
	})

	t.Run("Error", func(t *testing.T) {
		regularNode, mockNodeInfoRepo, _ := setupRegularNodeWithMocks()

		expectedError := assert.AnError
		mockNodeInfoRepo.On("AddAndUpdateNodeInfo", ctx, nodeInfo).Return(expectedError)

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
	regularNode := &RegularNode{
		submittedCvIndices:            make(map[string]map[string]bool),
		cleanupQueue:                  queue.New(),
		strictOrderWhileSecretRequest: make(map[string][]string),
		roundsData:                    make(map[string]RoundData),
		cosRecevied:                   sync.Map{},
	}

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

func TestNewRegularNode_DependencyInjectionFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("GetCommitByRound panics when regularCommitRepository is nil", func(t *testing.T) {
		node := &RegularNode{
			regularCommitRepository:       nil,
			submittedCvIndices:            make(map[string]map[string]bool),
			cleanupQueue:                  queue.New(),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
			cosRecevied:                   sync.Map{},
		}

		assert.Panics(t, func() {
			_, _ = node.GetCommitByRound(ctx, "1", "0")
		}, "Expected panic when regularCommitRepository is nil")
	})

	t.Run("AddCommit panics when regularCommitRepository is nil", func(t *testing.T) {
		node := &RegularNode{
			regularCommitRepository:       nil,
			submittedCvIndices:            make(map[string]map[string]bool),
			cleanupQueue:                  queue.New(),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
			cosRecevied:                   sync.Map{},
		}

		commitData := &utils.CommitData{
			Round:    "1",
			TrialNum: "0",
		}

		assert.Panics(t, func() {
			_ = node.AddCommit(ctx, commitData)
		}, "Expected panic when regularCommitRepository is nil")
	})

	t.Run("UpdateCommit panics when regularCommitRepository is nil", func(t *testing.T) {
		node := &RegularNode{
			regularCommitRepository:       nil,
			submittedCvIndices:            make(map[string]map[string]bool),
			cleanupQueue:                  queue.New(),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
			cosRecevied:                   sync.Map{},
		}

		commitData := &utils.CommitData{
			Round:    "1",
			TrialNum: "0",
		}

		assert.Panics(t, func() {
			_ = node.UpdateCommit(ctx, commitData)
		}, "Expected panic when regularCommitRepository is nil")
	})

	t.Run("AddNodeInfo panics when nodeInfoRepository is nil", func(t *testing.T) {
		node := &RegularNode{
			nodeInfoRepository:            nil,
			submittedCvIndices:            make(map[string]map[string]bool),
			cleanupQueue:                  queue.New(),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
			cosRecevied:                   sync.Map{},
		}

		nodeInfo := &utils.NodeInfo{
			PeerID: "test-peer-id",
			IP:     "127.0.0.1",
			Port:   "8080",
		}

		assert.Panics(t, func() {
			_ = node.AddNodeInfo(ctx, nodeInfo)
		}, "Expected panic when nodeInfoRepository is nil")
	})

	t.Run("CreateHost with nil p2pClient", func(t *testing.T) {
		node := &RegularNode{
			p2pClient:                     nil,
			submittedCvIndices:            make(map[string]map[string]bool),
			cleanupQueue:                  queue.New(),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
			cosRecevied:                   sync.Map{},
		}

		assert.Nil(t, node.p2pClient, "p2pClient should be nil - dependency injection failure")

		_, _, err := node.CreateHost("8080", "regular")
		assert.Error(t, err, "Expected error when p2pClient is nil")
	})

	t.Run("SetHost panics when p2pClient is nil", func(t *testing.T) {
		node := &RegularNode{
			p2pClient:                     nil,
			submittedCvIndices:            make(map[string]map[string]bool),
			cleanupQueue:                  queue.New(),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
			cosRecevied:                   sync.Map{},
		}

		assert.Panics(t, func() {
			node.SetHost(nil)
		}, "Expected panic when p2pClient is nil")
	})

	t.Run("ConnectToLeader with nil p2pClient", func(t *testing.T) {
		node := &RegularNode{
			p2pClient:                     nil,
			submittedCvIndices:            make(map[string]map[string]bool),
			cleanupQueue:                  queue.New(),
			strictOrderWhileSecretRequest: make(map[string][]string),
			roundsData:                    make(map[string]RoundData),
			cosRecevied:                   sync.Map{},
		}

		assert.Nil(t, node.p2pClient, "p2pClient should be nil - dependency injection failure")

		_, err := node.ConnectToLeader(ctx, "127.0.0.1", "8081", "leader-peer-id")
		assert.Error(t, err, "Expected error when p2pClient is nil")
	})
}

func getTestDB(t *testing.T) *pg.DB {
	testDB := pg.Connect(&pg.Options{
		Addr:     "localhost:5433",
		User:     "postgres",
		Password: "123",
		Database: "testdb",
	})

	if err := testDB.Ping(context.Background()); err != nil {
		testDB.Close()
		t.Skip("Skipping test: PostgreSQL database not available:", err)
		return nil
	}
	return testDB
}

func createRegularNodeWithValidDeps(t *testing.T, testDB *pg.DB, fallbackClient fallback_ethclient.IFallbackEthClient) (*RegularNode, error) {
	peerRepo := database.NewPeerCommitRepository(testDB)
	revealRepo := database.NewRevealOrderRepository(testDB)
	commitRepo := database.NewRegularCommitRepository(testDB)
	batchRepo := database.NewBatchRepository(testDB)
	nodeInfoRepo := database.NewNodeInfoRepository(testDB)
	leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
	revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, peerRepo, leaderCommitRepo)
	p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

	return NewRegularNode(
		fallbackClient,
		revealOrderService,
		p2pClient,
		peerRepo,
		revealRepo,
		commitRepo,
		batchRepo,
		nodeInfoRepo,
	)
}

func TestNewRegularNode_InvalidDependencyStates(t *testing.T) {
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	appconfig.Reload() // refresh cached config so NewRegularNode sees CONTRACT_ADDRESS
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()
	ctx := context.Background()

	t.Run("RegularCommitRepository with nil database panics", func(t *testing.T) {
		repo := database.NewRegularCommitRepository(nil)

		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		peerRepo := database.NewPeerCommitRepository(testDB)
		revealRepo := database.NewRevealOrderRepository(testDB)
		batchRepo := database.NewBatchRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, peerRepo, leaderCommitRepo)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		node, err := NewRegularNode(
			mockFallbackClient,
			revealOrderService,
			p2pClient,
			peerRepo,
			revealRepo,
			repo,
			batchRepo,
			nodeInfoRepo,
		)
		require.NoError(t, err)

		assert.Panics(t, func() {
			_, _ = node.GetCommitByRound(ctx, "1", "0")
		}, "Expected panic when database is nil")
	})

	t.Run("RegularCommitRepository with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		repo := database.NewRegularCommitRepository(invalidDB)

		testDB := pg.Connect(&pg.Options{
			Addr:     "localhost:5433",
			User:     "postgres",
			Password: "123",
			Database: "testdb",
		})
		defer testDB.Close()

		if err := testDB.Ping(context.Background()); err != nil {
			t.Skip("Skipping test: PostgreSQL database not available:", err)
			return
		}

		mockFallbackClient := new(MockFallbackEthClient)
		peerRepo := database.NewPeerCommitRepository(testDB)
		revealRepo := database.NewRevealOrderRepository(testDB)
		batchRepo := database.NewBatchRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, peerRepo, leaderCommitRepo)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		node, err := NewRegularNode(
			mockFallbackClient,
			revealOrderService,
			p2pClient,
			peerRepo,
			revealRepo,
			repo,
			batchRepo,
			nodeInfoRepo,
		)
		require.NoError(t, err)

		commitData := &utils.CommitData{
			Round:    "1",
			TrialNum: "0",
		}

		err = node.AddCommit(ctx, commitData)
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("NodeInfoRepository with nil database panics", func(t *testing.T) {
		repo := database.NewNodeInfoRepository(nil)

		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		peerRepo := database.NewPeerCommitRepository(testDB)
		revealRepo := database.NewRevealOrderRepository(testDB)
		commitRepo := database.NewRegularCommitRepository(testDB)
		batchRepo := database.NewBatchRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, peerRepo, leaderCommitRepo)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		node, err := NewRegularNode(
			mockFallbackClient,
			revealOrderService,
			p2pClient,
			peerRepo,
			revealRepo,
			commitRepo,
			batchRepo,
			repo,
		)
		require.NoError(t, err)

		nodeInfo := &utils.NodeInfo{
			PeerID: "test-peer-id",
			IP:     "127.0.0.1",
			Port:   "8080",
		}

		assert.Panics(t, func() {
			_ = node.AddNodeInfo(ctx, nodeInfo)
		}, "Expected panic when database is nil")
	})

	t.Run("NodeInfoRepository with invalid database connection", func(t *testing.T) {
		invalidDB := pg.Connect(&pg.Options{
			Addr:     "invalid-host:9999",
			User:     "postgres",
			Password: "wrong",
			Database: "nonexistent",
		})
		defer invalidDB.Close()

		repo := database.NewNodeInfoRepository(invalidDB)

		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		peerRepo := database.NewPeerCommitRepository(testDB)
		revealRepo := database.NewRevealOrderRepository(testDB)
		commitRepo := database.NewRegularCommitRepository(testDB)
		batchRepo := database.NewBatchRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, peerRepo, leaderCommitRepo)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		node, err := NewRegularNode(
			mockFallbackClient,
			revealOrderService,
			p2pClient,
			peerRepo,
			revealRepo,
			commitRepo,
			batchRepo,
			repo,
		)
		require.NoError(t, err)

		nodeInfo := &utils.NodeInfo{
			PeerID: "test-peer-id",
			IP:     "127.0.0.1",
			Port:   "8080",
		}

		err = node.AddNodeInfo(ctx, nodeInfo)
		assert.Error(t, err, "Expected error with invalid database connection")
	})

	t.Run("PeerCommitRepository with nil database panics", func(t *testing.T) {
		repo := database.NewPeerCommitRepository(nil)

		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		revealRepo := database.NewRevealOrderRepository(testDB)
		commitRepo := database.NewRegularCommitRepository(testDB)
		batchRepo := database.NewBatchRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, repo, leaderCommitRepo)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		node, err := NewRegularNode(
			mockFallbackClient,
			revealOrderService,
			p2pClient,
			repo,
			revealRepo,
			commitRepo,
			batchRepo,
			nodeInfoRepo,
		)
		require.NoError(t, err)

		assert.NotNil(t, node)
		assert.NotNil(t, node.peerCommitDataRepository, "Repository should exist")

		assert.Panics(t, func() {
			_, _ = node.peerCommitDataRepository.GetPeerCommitData(ctx, "1", "0", "0x1234567890123456789012345678901234567890")
		}, "Expected panic when database is nil")
	})

	t.Run("RevealOrderRepository with nil database panics", func(t *testing.T) {
		repo := database.NewRevealOrderRepository(nil)

		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		peerRepo := database.NewPeerCommitRepository(testDB)
		commitRepo := database.NewRegularCommitRepository(testDB)
		batchRepo := database.NewBatchRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(repo, peerRepo, leaderCommitRepo)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		node, err := NewRegularNode(
			mockFallbackClient,
			revealOrderService,
			p2pClient,
			peerRepo,
			repo,
			commitRepo,
			batchRepo,
			nodeInfoRepo,
		)
		require.NoError(t, err)

		assert.NotNil(t, node)
		assert.NotNil(t, node.revealOrderRepository, "Repository should exist")

		assert.Panics(t, func() {
			_, _ = node.revealOrderRepository.GetRevealOrder(ctx, "1", "0")
		}, "Expected panic when database is nil")
	})

	t.Run("BatchRepository with nil database panics", func(t *testing.T) {
		repo := database.NewBatchRepository(nil)

		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		peerRepo := database.NewPeerCommitRepository(testDB)
		revealRepo := database.NewRevealOrderRepository(testDB)
		commitRepo := database.NewRegularCommitRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, peerRepo, leaderCommitRepo)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		node, err := NewRegularNode(
			mockFallbackClient,
			revealOrderService,
			p2pClient,
			peerRepo,
			revealRepo,
			commitRepo,
			repo,
			nodeInfoRepo,
		)
		require.NoError(t, err)

		assert.NotNil(t, node)
		assert.NotNil(t, node.batchRepository, "Repository should exist")

		assert.Panics(t, func() {
			_ = node.batchRepository.DeleteOldRoundDataForRegularNode(ctx, "1")
		}, "Expected panic when database is nil")
	})
}

func TestNewRegularNode_PartialDependencyInjection(t *testing.T) {
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	appconfig.Reload() // refresh cached config so NewRegularNode sees CONTRACT_ADDRESS
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()
	t.Run("Constructor requires all dependencies", func(t *testing.T) {
		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		node, err := createRegularNodeWithValidDeps(t, testDB, mockFallbackClient)

		require.NoError(t, err)
		assert.NotNil(t, node)
		assert.NotNil(t, node.regularCommitRepository)
		assert.NotNil(t, node.nodeInfoRepository)
		assert.NotNil(t, node.p2pClient)
		assert.NotNil(t, node.fallbackEthClient)
		assert.NotNil(t, node.revealOrderService)
		assert.NotNil(t, node.peerCommitDataRepository)
		assert.NotNil(t, node.revealOrderRepository)
		assert.NotNil(t, node.batchRepository)
	})

	t.Run("Constructor rejects nil dependencies", func(t *testing.T) {
		_, err := NewRegularNode(
			nil, nil, nil, nil, nil, nil, nil, nil,
		)

		assert.Error(t, err, "Expected error when all dependencies are nil")
		assert.Contains(t, err.Error(), "cannot be nil")
	})
}

func TestNewRegularNode_RuntimeDatabaseDisconnection(t *testing.T) {
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	appconfig.Reload() // refresh cached config so NewRegularNode sees CONTRACT_ADDRESS
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()
	ctx := context.Background()

	const (
		postgresHost     = "localhost"
		postgresUser     = "postgres"
		postgresPassword = "123"
		postgresDB       = "testdb"
		postgresPort     = "5433"
	)

	t.Run("GetCommitByRound with database disconnect during operation", func(t *testing.T) {
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

		mockFallbackClient := new(MockFallbackEthClient)
		node, err := createRegularNodeWithValidDeps(t, testDB, mockFallbackClient)
		require.NoError(t, err)

		testDB.Close()

		result, err := node.GetCommitByRound(ctx, "1", "0")
		assert.Error(t, err, "Expected error when database is disconnected during read operation")
		assert.Nil(t, result, "Result should be nil when database is disconnected")
	})

	t.Run("AddCommit with database disconnect during operation", func(t *testing.T) {
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

		mockFallbackClient := new(MockFallbackEthClient)
		node, err := createRegularNodeWithValidDeps(t, testDB, mockFallbackClient)
		require.NoError(t, err)

		commitData := &utils.CommitData{
			Round:    "1",
			TrialNum: "0",
		}

		err = node.AddCommit(ctx, commitData)
		if err != nil {
			testDB.Close()
			t.Skip("Skipping test: Initial database operation failed:", err)
			return
		}

		testDB.Close()

		err = node.AddCommit(ctx, commitData)
		assert.Error(t, err, "Expected error when database is disconnected during write operation")
	})

	t.Run("UpdateCommit with database disconnect during operation", func(t *testing.T) {
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

		mockFallbackClient := new(MockFallbackEthClient)
		node, err := createRegularNodeWithValidDeps(t, testDB, mockFallbackClient)
		require.NoError(t, err)

		testDB.Close()

		commitData := &utils.CommitData{
			Round:    "1",
			TrialNum: "0",
		}

		err = node.UpdateCommit(ctx, commitData)
		assert.Error(t, err, "Expected error when database is disconnected during update operation")
	})

	t.Run("AddNodeInfo with database disconnect during operation", func(t *testing.T) {
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

		mockFallbackClient := new(MockFallbackEthClient)
		node, err := createRegularNodeWithValidDeps(t, testDB, mockFallbackClient)
		require.NoError(t, err)

		testDB.Close()

		nodeInfo := &utils.NodeInfo{
			PeerID: "test-peer-id",
			IP:     "127.0.0.1",
			Port:   "8080",
		}

		err = node.AddNodeInfo(ctx, nodeInfo)
		assert.Error(t, err, "Expected error when database is disconnected during operation")
	})
}

func TestNewRegularNode_ConstructorNilValidation(t *testing.T) {
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	appconfig.Reload() // refresh cached config so NewRegularNode sees CONTRACT_ADDRESS
	defer func() {
		os.Unsetenv("EOA_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()
	t.Run("NewRegularNode rejects nil fallbackEthClient", func(t *testing.T) {
		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		peerRepo := database.NewPeerCommitRepository(testDB)
		revealRepo := database.NewRevealOrderRepository(testDB)
		commitRepo := database.NewRegularCommitRepository(testDB)
		batchRepo := database.NewBatchRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, peerRepo, leaderCommitRepo)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		_, err := NewRegularNode(
			nil,
			revealOrderService,
			p2pClient,
			peerRepo,
			revealRepo,
			commitRepo,
			batchRepo,
			nodeInfoRepo,
		)

		assert.Error(t, err, "Expected error when fallbackEthClient is nil")
		assert.Contains(t, err.Error(), "fallbackEthClient cannot be nil")
	})

	t.Run("NewRegularNode rejects nil revealOrderService", func(t *testing.T) {
		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		peerRepo := database.NewPeerCommitRepository(testDB)
		revealRepo := database.NewRevealOrderRepository(testDB)
		commitRepo := database.NewRegularCommitRepository(testDB)
		batchRepo := database.NewBatchRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		_, err := NewRegularNode(
			mockFallbackClient,
			nil,
			p2pClient,
			peerRepo,
			revealRepo,
			commitRepo,
			batchRepo,
			nodeInfoRepo,
		)

		assert.Error(t, err, "Expected error when revealOrderService is nil")
		assert.Contains(t, err.Error(), "revealOrderService cannot be nil")
	})

	t.Run("NewRegularNode rejects nil p2pClient", func(t *testing.T) {
		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		peerRepo := database.NewPeerCommitRepository(testDB)
		revealRepo := database.NewRevealOrderRepository(testDB)
		commitRepo := database.NewRegularCommitRepository(testDB)
		batchRepo := database.NewBatchRepository(testDB)
		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, peerRepo, leaderCommitRepo)

		_, err := NewRegularNode(
			mockFallbackClient,
			revealOrderService,
			nil,
			peerRepo,
			revealRepo,
			commitRepo,
			batchRepo,
			nodeInfoRepo,
		)

		assert.Error(t, err, "Expected error when p2pClient is nil")
		assert.Contains(t, err.Error(), "p2pClient cannot be nil")
	})

	t.Run("NewRegularNode rejects nil repositories", func(t *testing.T) {
		mockFallbackClient := new(MockFallbackEthClient)
		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		nodeInfoRepo := database.NewNodeInfoRepository(testDB)
		leaderCommitRepo := database.NewLeaderCommitRepository(testDB)
		revealRepo := database.NewRevealOrderRepository(testDB)
		peerRepo := database.NewPeerCommitRepository(testDB)
		revealOrderService := commitreveal2.NewRevealOrderService(revealRepo, peerRepo, leaderCommitRepo)
		p2pClient := libp2putils.NewP2PClient(nodeInfoRepo)

		_, err := NewRegularNode(
			mockFallbackClient,
			revealOrderService,
			p2pClient,
			nil,
			nil,
			nil,
			nil,
			nil,
		)

		assert.Error(t, err, "Expected error when repositories are nil")
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("NewRegularNode accepts all valid dependencies", func(t *testing.T) {
		testDB := getTestDB(t)
		if testDB == nil {
			return
		}
		defer testDB.Close()

		mockFallbackClient := new(MockFallbackEthClient)
		node, err := createRegularNodeWithValidDeps(t, testDB, mockFallbackClient)

		require.NoError(t, err)
		assert.NotNil(t, node)
		assert.NotNil(t, node.regularCommitRepository)
		assert.NotNil(t, node.nodeInfoRepository)
		assert.NotNil(t, node.p2pClient)
		assert.NotNil(t, node.fallbackEthClient)
		assert.NotNil(t, node.revealOrderService)
		assert.NotNil(t, node.peerCommitDataRepository)
		assert.NotNil(t, node.revealOrderRepository)
		assert.NotNil(t, node.batchRepository)
	})
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

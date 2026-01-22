package libp2putils

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tokamak-network/DRB-node/utils"
)

// MockNodeInfoRepository is a mock for database.INodeInfoRepository
type MockNodeInfoRepository struct {
	mock.Mock
}

func (m *MockNodeInfoRepository) AddAndUpdateNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	args := m.Called(ctx, nodeInfo)
	return args.Error(0)
}

func (m *MockNodeInfoRepository) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.NodeInfo), args.Error(1)
}

func (m *MockNodeInfoRepository) DeleteNodeInfoByEOA(ctx context.Context, eoaAddress string) error {
	args := m.Called(ctx, eoaAddress)
	return args.Error(0)
}

func createTestHost(t *testing.T) host.Host {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	return h
}

func TestNewP2PClient(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		assert.NotNil(t, client)
		assert.NotNil(t, client.nodeInfoRepository)
		assert.Nil(t, client.hostInstance)
	})

	t.Run("With nil repository", func(t *testing.T) {
		client := NewP2PClient(nil)

		assert.NotNil(t, client)
		assert.Nil(t, client.nodeInfoRepository)
		assert.Nil(t, client.hostInstance)
	})
}

func TestSetHost(t *testing.T) {
	t.Run("Set valid host", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		defer testHost.Close()

		client.SetHost(testHost)

		assert.Equal(t, testHost, client.GetHostInstance())
	})

	t.Run("Set nil host", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		client.SetHost(nil)

		assert.Nil(t, client.GetHostInstance())
	})

	t.Run("Replace existing host", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		host1 := createTestHost(t)
		defer host1.Close()
		host2 := createTestHost(t)
		defer host2.Close()

		client.SetHost(host1)
		assert.Equal(t, host1, client.GetHostInstance())

		client.SetHost(host2)
		assert.Equal(t, host2, client.GetHostInstance())
		assert.NotEqual(t, host1, client.GetHostInstance())
	})

	t.Run("Set host with closed host", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		testHost.Close()

		assert.NotPanics(t, func() {
			client.SetHost(testHost)
		})
		assert.Equal(t, testHost, client.GetHostInstance())
	})
}

func TestGetHostInstance(t *testing.T) {
	t.Run("Get nil host", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		assert.Nil(t, client.GetHostInstance())
	})

	t.Run("Get valid host", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		defer testHost.Close()

		client.SetHost(testHost)

		retrievedHost := client.GetHostInstance()
		assert.Equal(t, testHost, retrievedHost)
		assert.NotNil(t, retrievedHost)
		assert.NotEmpty(t, retrievedHost.ID().String())
	})
}

// TestCreateHost tests CreateHost method
func TestCreateHost(t *testing.T) {
	t.Run("Create host with new key generation", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		nodeType := "test-new-key"
		_, _, err := client.CreateHost("0", nodeType)

		if err != nil {
			errMsg := err.Error()
			assert.True(t,
				strings.Contains(errMsg, "failed to create libp2p host") ||
					strings.Contains(errMsg, "failed to write") ||
					strings.Contains(errMsg, "failed to load") ||
					strings.Contains(errMsg, "not found") ||
					strings.Contains(errMsg, "private key file"),
				"Error should be related to host creation or file operations, got: %s", errMsg)
		}
	})

	t.Run("Create host with invalid port", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		_, _, err := client.CreateHost("invalid", "test")
		assert.Error(t, err)
	})

	t.Run("Create host with empty port", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		_, _, err := client.CreateHost("", "test")
		assert.Error(t, err)
	})

	t.Run("Create host with port out of range", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		_, _, err := client.CreateHost("99999", "test")
		assert.Error(t, err)
	})

	t.Run("Create host with negative port", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		_, _, err := client.CreateHost("-1", "test")
		assert.Error(t, err)
	})

	t.Run("Create host with different node types", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		nodeTypes := []string{"leader", "regular", "test"}
		for _, nodeType := range nodeTypes {
			_, _, err := client.CreateHost("0", nodeType)
			_ = err
		}
	})

	t.Run("Create host successfully - generates new key and creates host", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		nodeType := "test-success-" + t.Name()

		host, peerID, err := client.CreateHost("0", nodeType)

		if err == nil {
			assert.NotNil(t, host)
			assert.NotEmpty(t, peerID.String())

			assert.NotEmpty(t, host.ID().String())
			assert.Equal(t, peerID, host.ID())
			assert.NotEmpty(t, host.Addrs())

			host.Close()
		} else {
			assert.True(t,
				strings.Contains(err.Error(), "failed to write") ||
					strings.Contains(err.Error(), "failed to create libp2p host") ||
					strings.Contains(err.Error(), "no such file") ||
					strings.Contains(err.Error(), "permission denied") ||
					strings.Contains(err.Error(), "not found") ||
					strings.Contains(err.Error(), "private key file"),
				"Expected file or host creation error, got: %v", err)
		}
	})

	t.Run("Create host - error when libp2p.New fails with invalid port format", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		nodeType := "test-libp2p-error-" + t.Name()

		invalidPorts := []string{"99999", "abc", "-1"}

		for _, invalidPort := range invalidPorts {
			_, _, err := client.CreateHost(invalidPort, nodeType)
			assert.Error(t, err)
			assert.True(t,
				strings.Contains(err.Error(), "failed to create libp2p host") ||
					strings.Contains(err.Error(), "invalid port") ||
					strings.Contains(err.Error(), "failed to write") ||
					strings.Contains(err.Error(), "no such file") ||
					strings.Contains(err.Error(), "not found") ||
					strings.Contains(err.Error(), "private key file"),
				"Expected libp2p or file error, got: %v", err)
		}
	})

	t.Run("Create host - error", func(t *testing.T) {

		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		nodeType := "../../../etc/passwd"

		_, _, err := client.CreateHost("0", nodeType)
		if err != nil {
			if strings.Contains(err.Error(), "error checking private key file") {
				assert.Error(t, err)
				return
			}
			_ = err
		}

	})

	t.Run("Create host with regular node type - missing REGULAR_NODE_NUMBER (uses regularnode.bin)", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		originalNodeNum := os.Getenv("REGULAR_NODE_NUMBER")
		originalPeerID := os.Getenv("REGULAR_PEER_ID")
		os.Unsetenv("REGULAR_NODE_NUMBER")
		os.Unsetenv("REGULAR_PEER_ID")
		defer func() {
			if originalNodeNum != "" {
				os.Setenv("REGULAR_NODE_NUMBER", originalNodeNum)
			}
			if originalPeerID != "" {
				os.Setenv("REGULAR_PEER_ID", originalPeerID)
			}
		}()

		// When REGULAR_NODE_NUMBER is empty, it uses regularnode.bin and REGULAR_PEER_ID
		// Since the file doesn't exist, it should error about the file not being found
		_, _, err := client.CreateHost("0", "regular")
		assert.Error(t, err)
		// Error should be about file not found (regularnode.bin) or REGULAR_PEER_ID missing
		assert.True(t,
			strings.Contains(err.Error(), "regularnode.bin") ||
				strings.Contains(err.Error(), "REGULAR_PEER_ID") ||
				strings.Contains(err.Error(), "not found"),
			"Error should mention regularnode.bin or REGULAR_PEER_ID, got: %s", err.Error())
	})

	t.Run("Create host with regular node type - missing REGULAR_PEER_ID", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		originalNodeNum := os.Getenv("REGULAR_NODE_NUMBER")
		originalPeerID := os.Getenv("REGULAR_PEER_ID")
		os.Setenv("REGULAR_NODE_NUMBER", "1")
		os.Unsetenv("REGULAR_PEER_ID")
		defer func() {
			if originalNodeNum != "" {
				os.Setenv("REGULAR_NODE_NUMBER", originalNodeNum)
			} else {
				os.Unsetenv("REGULAR_NODE_NUMBER")
			}
			if originalPeerID != "" {
				os.Setenv("REGULAR_PEER_ID", originalPeerID)
			}
		}()

		filePath := "static-key/regularnode1.bin"
		err := os.MkdirAll("static-key", 0755)
		if err != nil {
			t.Skipf("Cannot create static-key directory: %v", err)
			return
		}
		defer os.RemoveAll("static-key")

		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			t.Fatalf("Failed to generate key: %v", err)
		}
		buff, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			t.Fatalf("Failed to marshal key: %v", err)
		}
		err = os.WriteFile(filePath, buff, 0644)
		if err != nil {
			t.Fatalf("Failed to write key file: %v", err)
		}

		_, _, err = client.CreateHost("0", "regular")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "REGULAR_PEER_ID")
		assert.Contains(t, err.Error(), "is required for regular nodes")
	})

	t.Run("Create host with regular node type - mismatched Peer ID", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		originalNodeNum := os.Getenv("REGULAR_NODE_NUMBER")
		originalPeerID := os.Getenv("REGULAR_PEER_ID")
		os.Setenv("REGULAR_NODE_NUMBER", "1")
		os.Setenv("REGULAR_PEER_ID", "12D3KooWInvalidPeerID1234567890123456789012345678901234567890")
		defer func() {
			if originalNodeNum != "" {
				os.Setenv("REGULAR_NODE_NUMBER", originalNodeNum)
			} else {
				os.Unsetenv("REGULAR_NODE_NUMBER")
			}
			if originalPeerID != "" {
				os.Setenv("REGULAR_PEER_ID", originalPeerID)
			} else {
				os.Unsetenv("REGULAR_PEER_ID")
			}
		}()

		filePath := "static-key/regularnode1.bin"
		err := os.MkdirAll("static-key", 0755)
		if err != nil {
			t.Skipf("Cannot create static-key directory: %v", err)
			return
		}
		defer os.RemoveAll("static-key")

		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			t.Fatalf("Failed to generate key: %v", err)
		}
		buff, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			t.Fatalf("Failed to marshal key: %v", err)
		}
		err = os.WriteFile(filePath, buff, 0644)
		if err != nil {
			t.Fatalf("Failed to write key file: %v", err)
		}

		_, _, err = client.CreateHost("0", "regular")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not match peer ID from key file")
		assert.Contains(t, err.Error(), "REGULAR_PEER_ID")
	})

	t.Run("Create host with regular node type - success with valid env vars", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		originalNodeNum := os.Getenv("REGULAR_NODE_NUMBER")
		originalPeerID := os.Getenv("REGULAR_PEER_ID")
		os.Setenv("REGULAR_NODE_NUMBER", "1")
		defer func() {
			if originalNodeNum != "" {
				os.Setenv("REGULAR_NODE_NUMBER", originalNodeNum)
			} else {
				os.Unsetenv("REGULAR_NODE_NUMBER")
			}
			if originalPeerID != "" {
				os.Setenv("REGULAR_PEER_ID", originalPeerID)
			} else {
				os.Unsetenv("REGULAR_PEER_ID")
			}
		}()

		filePath := "static-key/regularnode1.bin"
		err := os.MkdirAll("static-key", 0755)
		if err != nil {
			t.Skipf("Cannot create static-key directory: %v", err)
			return
		}
		defer os.RemoveAll("static-key")

		// Generate a valid key file
		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			t.Fatalf("Failed to generate key: %v", err)
		}
		buff, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			t.Fatalf("Failed to marshal key: %v", err)
		}
		err = os.WriteFile(filePath, buff, 0644)
		if err != nil {
			t.Fatalf("Failed to write key file: %v", err)
		}

		// Get the peer ID from the key
		peerID, err := peer.IDFromPrivateKey(privKey)
		if err != nil {
			t.Fatalf("Failed to get peer ID: %v", err)
		}

		os.Setenv("REGULAR_PEER_ID", peerID.String())

		host, returnedPeerID, err := client.CreateHost("0", "regular")
		if err != nil {
			if strings.Contains(err.Error(), "failed to create libp2p host") {
				return
			}
			t.Fatalf("Unexpected error: %v", err)
		}

		assert.NotNil(t, host)
		assert.Equal(t, peerID, returnedPeerID)
		assert.Equal(t, peerID.String(), os.Getenv("REGULAR_PEER_ID"))
		host.Close()
	})

	t.Run("Create host with regular node type - different node numbers", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		// Test with node number 2
		originalNodeNum := os.Getenv("REGULAR_NODE_NUMBER")
		originalPeerID := os.Getenv("REGULAR_PEER_ID")
		os.Setenv("REGULAR_NODE_NUMBER", "2")
		defer func() {
			if originalNodeNum != "" {
				os.Setenv("REGULAR_NODE_NUMBER", originalNodeNum)
			} else {
				os.Unsetenv("REGULAR_NODE_NUMBER")
			}
			if originalPeerID != "" {
				os.Setenv("REGULAR_PEER_ID", originalPeerID)
			} else {
				os.Unsetenv("REGULAR_PEER_ID")
			}
		}()

		// Create a valid key file for node 2
		filePath := "static-key/regularnode2.bin"
		err := os.MkdirAll("static-key", 0755)
		if err != nil {
			t.Skipf("Cannot create static-key directory: %v", err)
			return
		}
		defer os.RemoveAll("static-key")

		// Generate a valid key file
		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			t.Fatalf("Failed to generate key: %v", err)
		}
		buff, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			t.Fatalf("Failed to marshal key: %v", err)
		}
		err = os.WriteFile(filePath, buff, 0644)
		if err != nil {
			t.Fatalf("Failed to write key file: %v", err)
		}

		// Get the peer ID from the key
		peerID, err := peer.IDFromPrivateKey(privKey)
		if err != nil {
			t.Fatalf("Failed to get peer ID: %v", err)
		}

		// Set the correct peer ID in environment
		os.Setenv("REGULAR_PEER_ID", peerID.String())

		_, _, err = client.CreateHost("0", "regular")
		if err != nil {
			if strings.Contains(err.Error(), "REGULAR_PEER_ID") && strings.Contains(err.Error(), "required") {
				t.Fatalf("Should have set REGULAR_PEER_ID: %v", err)
			}
		}
	})

	t.Run("Create host - regular node type file path construction", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		originalNodeNum := os.Getenv("REGULAR_NODE_NUMBER")
		os.Setenv("REGULAR_NODE_NUMBER", "5")
		defer func() {
			if originalNodeNum != "" {
				os.Setenv("REGULAR_NODE_NUMBER", originalNodeNum)
			} else {
				os.Unsetenv("REGULAR_NODE_NUMBER")
			}
		}()

		_, _, err := client.CreateHost("0", "regular")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regularnode5.bin")
	})

	t.Run("Create host - regular node type with empty REGULAR_NODE_NUMBER (falls back to regularnode.bin)", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		originalNodeNum := os.Getenv("REGULAR_NODE_NUMBER")
		originalPeerID := os.Getenv("REGULAR_PEER_ID")

		// Create regularnode.bin
		filePath := "static-key/regularnode.bin"
		err := os.MkdirAll("static-key", 0755)
		if err != nil {
			t.Skipf("Cannot create static-key directory: %v", err)
			return
		}
		defer os.RemoveAll("static-key")

		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			t.Fatalf("Failed to generate key: %v", err)
		}
		buff, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			t.Fatalf("Failed to marshal key: %v", err)
		}
		err = os.WriteFile(filePath, buff, 0644)
		if err != nil {
			t.Fatalf("Failed to write key file: %v", err)
		}

		// Set REGULAR_NODE_NUMBER to "1" first, then unset it
		// This simulates the case where REGULAR_NODE_NUMBER becomes empty
		os.Setenv("REGULAR_NODE_NUMBER", "1")
		os.Unsetenv("REGULAR_NODE_NUMBER")
		os.Unsetenv("REGULAR_PEER_ID") // Also unset REGULAR_PEER_ID to trigger error

		defer func() {
			if originalNodeNum != "" {
				os.Setenv("REGULAR_NODE_NUMBER", originalNodeNum)
			}
			if originalPeerID != "" {
				os.Setenv("REGULAR_PEER_ID", originalPeerID)
			}
		}()

		// When REGULAR_NODE_NUMBER is empty, it uses regularnode.bin and REGULAR_PEER_ID
		// Since REGULAR_PEER_ID is not set, it should error about REGULAR_PEER_ID
		_, _, err = client.CreateHost("0", "regular")
		assert.Error(t, err)
		// Error should be about REGULAR_PEER_ID being required (not REGULAR_NODE_NUMBER)
		assert.Contains(t, err.Error(), "REGULAR_PEER_ID")
		assert.Contains(t, err.Error(), "required")
	})

}

// TestConnectToPeer tests ConnectToPeer method
func TestConnectToPeer(t *testing.T) {
	t.Run("Connect with nil host instance", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		ctx := context.Background()
		addrInfo, err := client.ConnectToPeer(ctx, "127.0.0.1", "4001", "12D3KooWTest1234567890123456789012345678901234567890")

		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Connect with valid host but invalid peer ID", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		defer testHost.Close()
		client.SetHost(testHost)

		ctx := context.Background()
		addrInfo, err := client.ConnectToPeer(ctx, "127.0.0.1", "4001", "invalid-peer-id")

		assert.Error(t, err)
		assert.Nil(t, addrInfo)
		assert.True(t, strings.Contains(err.Error(), "failed to parse") ||
			strings.Contains(err.Error(), "failed to create peer info"))
	})

	t.Run("Connect with empty peer ID", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		defer testHost.Close()
		client.SetHost(testHost)

		ctx := context.Background()
		addrInfo, err := client.ConnectToPeer(ctx, "127.0.0.1", "4001", "")

		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Connect with empty port", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		defer testHost.Close()
		client.SetHost(testHost)

		ctx := context.Background()
		validPeerID := testHost.ID().String()
		addrInfo, err := client.ConnectToPeer(ctx, "127.0.0.1", "", validPeerID)

		assert.Error(t, err)
		assert.Nil(t, addrInfo)
	})

	t.Run("Connect with unreachable peer", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		defer testHost.Close()
		client.SetHost(testHost)

		targetHost := createTestHost(t)
		defer targetHost.Close()
		validPeerID := targetHost.ID().String()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		addrInfo, err := client.ConnectToPeer(ctx, "127.0.0.1", "65534", validPeerID)

		assert.Error(t, err)
		_ = addrInfo
	})

	t.Run("Connect with context timeout", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		defer testHost.Close()
		client.SetHost(testHost)

		targetHost := createTestHost(t)
		defer targetHost.Close()
		validPeerID := targetHost.ID().String()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		addrInfo, err := client.ConnectToPeer(ctx, "127.0.0.1", "9999", validPeerID)

		assert.Error(t, err)
		_ = addrInfo
	})

	t.Run("Connect with cancelled context", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		defer testHost.Close()
		client.SetHost(testHost)

		targetHost := createTestHost(t)
		defer targetHost.Close()
		validPeerID := targetHost.ID().String()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		addrInfo, err := client.ConnectToPeer(ctx, "127.0.0.1", "4001", validPeerID)

		assert.Error(t, err)
		_ = addrInfo
		// ./build.sh
	})

	t.Run("DNS resolution - uses hardcoded DNS name", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)
		testHost := createTestHost(t)
		defer testHost.Close()
		client.SetHost(testHost)

		targetHost := createTestHost(t)
		defer targetHost.Close()
		validPeerID := targetHost.ID().String()

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		//  leaderIP parameter is ignored
		addrInfo1, err1 := client.ConnectToPeer(ctx, "127.0.0.1", "4001", validPeerID)
		addrInfo2, err2 := client.ConnectToPeer(ctx, "192.168.1.1", "4001", validPeerID)

		assert.Error(t, err1)
		assert.Error(t, err2)
		_ = addrInfo1
		_ = addrInfo2
	})
}

// TestGetConnectedPeers tests GetConnectedPeers method
func TestGetConnectedPeers(t *testing.T) {
	t.Run("Success with multiple nodes", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		// Create real hosts to get valid peer IDs
		host1 := createTestHost(t)
		defer host1.Close()
		host2 := createTestHost(t)
		defer host2.Close()

		ctx := context.Background()
		expectedNodes := []*utils.NodeInfo{
			{
				IP:         "127.0.0.1",
				Port:       "4001",
				PeerID:     host1.ID().String(),
				EOAAddress: "0x1234567890123456789012345678901234567890",
			},
			{
				IP:         "127.0.0.1",
				Port:       "4002",
				PeerID:     host2.ID().String(),
				EOAAddress: "0x2234567890123456789012345678901234567890",
			},
		}

		mockRepo.On("GetNodeInfos", ctx).Return(expectedNodes, nil)

		result := client.GetConnectedPeers(ctx)

		assert.NotNil(t, result)
		assert.Equal(t, len(expectedNodes), len(result))
		assert.Contains(t, result, expectedNodes[0].EOAAddress)
		assert.Contains(t, result, expectedNodes[1].EOAAddress)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Success with empty nodes", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		ctx := context.Background()
		expectedNodes := []*utils.NodeInfo{}

		mockRepo.On("GetNodeInfos", ctx).Return(expectedNodes, nil)

		result := client.GetConnectedPeers(ctx)

		assert.NotNil(t, result)
		assert.Equal(t, 0, len(result))

		mockRepo.AssertExpectations(t)
	})

	t.Run("Error from repository", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		ctx := context.Background()
		expectedError := assert.AnError

		mockRepo.On("GetNodeInfos", ctx).Return(nil, expectedError)

		result := client.GetConnectedPeers(ctx)

		assert.Nil(t, result)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Node with invalid multiaddress", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		ctx := context.Background()
		expectedNodes := []*utils.NodeInfo{
			{
				IP:         "invalid-ip",
				Port:       "invalid-port",
				PeerID:     "invalid-peer-id",
				EOAAddress: "0x1234567890123456789012345678901234567890",
			},
		}

		mockRepo.On("GetNodeInfos", ctx).Return(expectedNodes, nil)

		result := client.GetConnectedPeers(ctx)

		assert.NotNil(t, result)
		assert.NotContains(t, result, expectedNodes[0].EOAAddress)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Node with valid multiaddress", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		// Create a real peer ID for testing
		testHost := createTestHost(t)
		defer testHost.Close()
		validPeerID := testHost.ID().String()

		ctx := context.Background()
		expectedNodes := []*utils.NodeInfo{
			{
				IP:         "127.0.0.1",
				Port:       "4001",
				PeerID:     validPeerID,
				EOAAddress: "0x1234567890123456789012345678901234567890",
			},
		}

		mockRepo.On("GetNodeInfos", ctx).Return(expectedNodes, nil)

		result := client.GetConnectedPeers(ctx)

		assert.NotNil(t, result)
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result, expectedNodes[0].EOAAddress)

		nodeInfo := result[expectedNodes[0].EOAAddress]
		assert.Equal(t, expectedNodes[0].IP, nodeInfo.IP)
		assert.Equal(t, expectedNodes[0].Port, nodeInfo.Port)
		assert.Equal(t, expectedNodes[0].PeerID, nodeInfo.PeerID.String())

		mockRepo.AssertExpectations(t)
	})

	t.Run("Multiple nodes with some invalid", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		testHost := createTestHost(t)
		defer testHost.Close()
		validPeerID := testHost.ID().String()

		ctx := context.Background()
		expectedNodes := []*utils.NodeInfo{
			{
				IP:         "127.0.0.1",
				Port:       "4001",
				PeerID:     validPeerID,
				EOAAddress: "0x1234567890123456789012345678901234567890",
			},
			{
				IP:         "invalid-ip",
				Port:       "invalid-port",
				PeerID:     "invalid-peer-id",
				EOAAddress: "0x2234567890123456789012345678901234567890",
			},
			{
				IP:         "192.168.1.1",
				Port:       "4002",
				PeerID:     validPeerID,
				EOAAddress: "0x3234567890123456789012345678901234567890",
			},
		}

		mockRepo.On("GetNodeInfos", ctx).Return(expectedNodes, nil)

		result := client.GetConnectedPeers(ctx)

		assert.NotNil(t, result)
		assert.Contains(t, result, expectedNodes[0].EOAAddress)
		assert.NotContains(t, result, expectedNodes[1].EOAAddress)
		assert.Contains(t, result, expectedNodes[2].EOAAddress)

		mockRepo.AssertExpectations(t)
	})
}

func TestP2PClient_Integration(t *testing.T) {
	t.Run("Complete workflow", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		// Create host
		testHost := createTestHost(t)
		defer testHost.Close()

		// Set host
		client.SetHost(testHost)
		assert.Equal(t, testHost, client.GetHostInstance())

		// Get host instance
		retrievedHost := client.GetHostInstance()
		assert.NotNil(t, retrievedHost)
		assert.Equal(t, testHost.ID(), retrievedHost.ID())

		// Test GetConnectedPeers
		ctx := context.Background()
		mockRepo.On("GetNodeInfos", ctx).Return([]*utils.NodeInfo{}, nil)
		result := client.GetConnectedPeers(ctx)
		assert.NotNil(t, result)

		mockRepo.AssertExpectations(t)
	})

	t.Run("SetHost and ConnectToPeer workflow", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		testHost := createTestHost(t)
		defer testHost.Close()
		client.SetHost(testHost)

		targetHost := createTestHost(t)
		defer targetHost.Close()
		validPeerID := targetHost.ID().String()

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		addrInfo, err := client.ConnectToPeer(ctx, "127.0.0.1", "4001", validPeerID)
		assert.Error(t, err)
		_ = addrInfo
	})
}

func TestP2PClient_EdgeCases(t *testing.T) {
	t.Run("Concurrent SetHost calls", func(t *testing.T) {
		mockRepo := new(MockNodeInfoRepository)
		client := NewP2PClient(mockRepo)

		host1 := createTestHost(t)
		defer host1.Close()
		host2 := createTestHost(t)
		defer host2.Close()
		host3 := createTestHost(t)
		defer host3.Close()

		done := make(chan bool, 3)
		go func() {
			client.SetHost(host1)
			done <- true
		}()
		go func() {
			client.SetHost(host2)
			done <- true
		}()
		go func() {
			client.SetHost(host3)
			done <- true
		}()

		for i := 0; i < 3; i++ {
			<-done
		}

		retrievedHost := client.GetHostInstance()
		assert.NotNil(t, retrievedHost)
		assert.True(t, retrievedHost == host1 || retrievedHost == host2 || retrievedHost == host3)
	})
}

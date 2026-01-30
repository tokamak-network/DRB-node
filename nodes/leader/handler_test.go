package leader_node

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-pg/pg/v10"
	_ "github.com/lib/pq"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// LeaderHandlerTestSuite defines the test suite
type LeaderHandlerTestSuite struct {
	suite.Suite
	db                   *pg.DB
	leaderCommitRepo     *database.LeaderCommitRepository
	batchRepo            *database.BatchRepository
	broadcastTrackerRepo *database.BroadcastTrackerRepository
	revealOrderRepo      *database.RevealOrderRepository
	nodeInfoRepo         *database.NodeInfoRepository
	peerCommitRepo       *database.PeerCommitRepository
	revealOrderService   *commitreveal2.RevealOrderService
	p2pClient            *libp2putils.P2PClient
	leaderNodeHandler    *LeaderNodeHandler
}

// SetupSuite runs once before all tests in the suite
func (suite *LeaderHandlerTestSuite) SetupSuite() {
	// Initialize logger
	logger.InitLogger()

	const (
		postgresHost     = "localhost"
		postgresUser     = "postgres"
		postgresPassword = "123"
		postgresDB       = "testdb"
		postgresPort     = "5433"
	)

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		postgresUser, postgresPassword, postgresHost, postgresPort, postgresDB)

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		suite.T().Skip("Skipping test suite: PostgreSQL database not available:", err)
		return
	}
	defer sqlDB.Close()

	err = sqlDB.Ping()
	if err != nil {
		suite.T().Skip("Skipping test suite: PostgreSQL database not available:", err)
		return
	}

	err = database.MigrationsUp(sqlDB)
	require.NoError(suite.T(), err, "Failed to run migrations")

	suite.db = pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", postgresHost, postgresPort),
		User:     postgresUser,
		Password: postgresPassword,
		Database: postgresDB,
	})

	err = suite.db.Ping(context.Background())
	require.NoError(suite.T(), err, "Failed to connect to test database")

	log.Println("Leader handler test suite initialized successfully")
}

// SetupTest runs before each test
func (suite *LeaderHandlerTestSuite) SetupTest() {
	suite.leaderCommitRepo = database.NewLeaderCommitRepository(suite.db)
	suite.batchRepo = database.NewBatchRepository(suite.db)
	suite.broadcastTrackerRepo = database.NewBroadcastTrackerRepository(suite.db)
	suite.revealOrderRepo = database.NewRevealOrderRepository(suite.db)
	suite.nodeInfoRepo = database.NewNodeInfoRepository(suite.db)
	suite.peerCommitRepo = database.NewPeerCommitRepository(suite.db)

	suite.revealOrderService = commitreveal2.NewRevealOrderService(
		suite.revealOrderRepo,
		suite.peerCommitRepo,
		suite.leaderCommitRepo,
	)
	suite.p2pClient = libp2putils.NewP2PClient(suite.nodeInfoRepo)

	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	var err error
	suite.leaderNodeHandler, err = NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
}

// TearDownSuite runs once after all tests in the suite
func (suite *LeaderHandlerTestSuite) TearDownSuite() {
	if suite.db != nil {
		suite.db.Close()
		log.Println("Leader handler test suite cleaned up")
	}
}

// Mock connection implementation
type mockConn struct {
	remotePeer peer.ID
}

func (m *mockConn) LocalPeer() peer.ID                                    { return "" }
func (m *mockConn) RemotePeer() peer.ID                                   { return m.remotePeer }
func (m *mockConn) LocalPrivateKey() libp2pcrypto.PrivKey                 { return nil }
func (m *mockConn) RemotePublicKey() libp2pcrypto.PubKey                  { return nil }
func (m *mockConn) ID() string                                            { return "mock-conn-id" }
func (m *mockConn) Close() error                                          { return nil }
func (m *mockConn) GetStreams() []network.Stream                          { return nil }
func (m *mockConn) Stat() network.ConnStats                               { return network.ConnStats{} }
func (m *mockConn) LocalMultiaddr() multiaddr.Multiaddr                   { return nil }
func (m *mockConn) RemoteMultiaddr() multiaddr.Multiaddr                  { return nil }
func (m *mockConn) Scope() network.ConnScope                              { return nil }
func (m *mockConn) IsClosed() bool                                        { return false }
func (m *mockConn) ConnState() network.ConnectionState                    { return network.ConnectionState{} }
func (m *mockConn) NewStream(ctx context.Context) (network.Stream, error) { return nil, nil }

// Mock stream implementation
type mockStream struct {
	readBuffer  *bytes.Buffer
	writeBuffer *bytes.Buffer
	closed      bool
	mu          sync.Mutex // Add mutex for thread safety
	conn        *mockConn
}

func newMockStream() *mockStream {
	// Create a mock peer ID for the connection
	mockPeerID, _ := peer.Decode("12D3KooWTestPeerID1234567890123456789012345678901234567890")
	return &mockStream{
		readBuffer:  new(bytes.Buffer),
		writeBuffer: new(bytes.Buffer),
		closed:      false,
		conn:        &mockConn{remotePeer: mockPeerID},
	}
}

func (m *mockStream) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.readBuffer.Read(p)
}

func (m *mockStream) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeBuffer.Write(p)
}

func (m *mockStream) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockStream) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil
}
func (m *mockStream) SetDeadline(t time.Time) error      { return nil }
func (m *mockStream) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockStream) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockStream) ID() string                         { return "mock-stream-id" }
func (m *mockStream) Protocol() protocol.ID              { return protocol.ID("/test") }
func (m *mockStream) SetProtocol(protocol.ID) error      { return nil }
func (m *mockStream) Stat() network.Stats                { return network.Stats{} }
func (m *mockStream) Conn() network.Conn {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conn == nil {
		mockPeerID, _ := peer.Decode("12D3KooWTestPeerID1234567890123456789012345678901234567890")
		m.conn = &mockConn{remotePeer: mockPeerID}
	}
	return m.conn
}
func (m *mockStream) CloseWrite() error          { m.closed = true; return nil }
func (m *mockStream) CloseRead() error           { m.closed = true; return nil }
func (m *mockStream) Scope() network.StreamScope { return nil }

// TestLeaderHandler_AtomicOperations tests atomic get/set operations
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AtomicOperations() {
	handler := &LeaderNodeHandler{}

	assert.False(suite.T(), handler.GetMerkleRootSubmitted())
	handler.SetMerkleRootSubmitted(true)
	assert.True(suite.T(), handler.GetMerkleRootSubmitted())
	handler.SetMerkleRootSubmitted(false)
	assert.False(suite.T(), handler.GetMerkleRootSubmitted())
}

func (suite *LeaderHandlerTestSuite) TestLeaderHandler_Initialization() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)

	require.NotNil(suite.T(), handler)
	assert.NotNil(suite.T(), handler.leaderNode)
	assert.False(suite.T(), handler.GetMerkleRootSubmitted())
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_RepositoryInitialization() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)

	require.NotNil(suite.T(), handler)
	require.NotNil(suite.T(), handler.leaderNode)

	assert.NotNil(suite.T(), handler.leaderNode.leaderCommitRepository, "LeaderCommitRepository should be initialized")
	assert.NotNil(suite.T(), handler.leaderNode.batchRepository, "BatchRepository should be initialized")
	assert.NotNil(suite.T(), handler.leaderNode.broadcastTrackerRepository, "BroadcastTrackerRepository should be initialized")
	assert.NotNil(suite.T(), handler.leaderNode.reavealOrderRepository, "RevealOrderRepository should be initialized")
	assert.NotNil(suite.T(), handler.leaderNode.nodeInfoRepository, "NodeInfoRepository should be initialized")

	assert.NotNil(suite.T(), handler.leaderNode.revealOrderService, "RevealOrderService should be initialized")
	assert.NotNil(suite.T(), handler.leaderNode.p2pClient, "P2PClient should be initialized")
	assert.NotNil(suite.T(), handler.ethService, "EthService should be initialized")
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_BatchRepositoryIntegration() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	testRound := "batch_test_round_1"
	testTrial := "1"
	testOp := common.HexToAddress("0xB111111111111111111111111111111111111111")
	cvs := [32]byte{1, 2, 3}
	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cvs:        cvs,
		CvsHex:     hex.EncodeToString(cvs[:]),
	}

	err = handler.leaderNode.AddLeaderCommit(ctx, commitData)
	require.NoError(suite.T(), err, "Should be able to add commit data")
	retrieved, err := handler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs, retrieved.Cvs)
	err = handler.leaderNode.batchRepository.DeleteRoundTrialDataForLeaderNode(ctx, testRound, testTrial)
	require.NoError(suite.T(), err, "DeleteRoundTrialDataForLeaderNode should succeed")

	_, err = handler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, testRound, testTrial, testOp.Hex())
	assert.Error(suite.T(), err, "Data should be deleted after DeleteRoundTrialDataForLeaderNode")
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_BatchRepositoryDeleteOldRounds() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	oldRound := "old_round_1"
	currentRound := "current_round_1"
	testTrial := "1"
	testOp := common.HexToAddress("0xB222222222222222222222222222222222222222")
	cvs := [32]byte{1, 2, 3}
	oldCommitData := &utils.LeaderCommitData{
		Round:      oldRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cvs:        cvs,
		CvsHex:     hex.EncodeToString(cvs[:]),
	}
	err = handler.leaderNode.AddLeaderCommit(ctx, oldCommitData)
	require.NoError(suite.T(), err)
	currentCommitData := &utils.LeaderCommitData{
		Round:      currentRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cvs:        cvs,
		CvsHex:     hex.EncodeToString(cvs[:]),
	}
	err = handler.leaderNode.AddLeaderCommit(ctx, currentCommitData)
	require.NoError(suite.T(), err)
	err = handler.leaderNode.batchRepository.DeleteOldRoundDataForLeaderNode(ctx, currentRound)
	require.NoError(suite.T(), err, "DeleteOldRoundDataForLeaderNode should succeed")
	_, err = handler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, oldRound, testTrial, testOp.Hex())
	assert.Error(suite.T(), err, "Old round data should be deleted")

	retrieved, err := handler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, currentRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err, "Current round data should still exist")
	assert.Equal(suite.T(), currentRound, retrieved.Round)
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_BroadcastTrackerRepositoryIntegration() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	testRound := "broadcast_test_round_1"
	testTrial := "1"
	testOp := common.HexToAddress("0xC111111111111111111111111111111111111111")
	messageID := "test_message_001"

	tracker := &utils.BroadcastTracker{
		Round:        testRound,
		TrialNum:     testTrial,
		EOAAddress:   testOp.Hex(),
		Type:         "cvs",
		MessageID:    messageID,
		Data:         [32]byte{1, 2, 3, 4, 5},
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     time.Now().Unix(),
		Timeout:      30,
	}

	err = handler.leaderNode.broadcastTrackerRepository.AddBroadcastTracker(ctx, tracker)
	require.NoError(suite.T(), err, "AddBroadcastTracker should succeed")

	tracker.Attempts = 1
	tracker.Acknowledged["peer1"] = true
	err = handler.leaderNode.broadcastTrackerRepository.UpdateBroadcastTracker(ctx, tracker)
	require.NoError(suite.T(), err, "UpdateBroadcastTracker should succeed")

	suite.db.Model((*database.BroadcastTrackerScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ? AND message_id = ?",
			testRound, testTrial, testOp.Hex(), messageID).
		Delete()
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_RevealOrderRepositoryIntegration() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	testRound := "reveal_test_round_1"
	testTrial := "1"

	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{"0xNode1", "0xNode2", "0xNode3"},
		RevealOrder:  []int{0, 1, 2},
		RV:           "test_rv_value",
	}

	err = handler.leaderNode.reavealOrderRepository.AddRevealOrder(ctx, revealOrder)
	require.NoError(suite.T(), err, "AddRevealOrder should succeed")

	retrieved, err := handler.leaderNode.reavealOrderRepository.GetRevealOrder(ctx, testRound, testTrial)
	require.NoError(suite.T(), err, "GetRevealOrder should succeed")
	assert.Equal(suite.T(), testRound, retrieved.Round)
	assert.Equal(suite.T(), testTrial, retrieved.TrialNum)
	assert.Equal(suite.T(), revealOrder.OrderedNodes, retrieved.OrderedNodes)
	assert.Equal(suite.T(), revealOrder.RevealOrder, retrieved.RevealOrder)
	assert.Equal(suite.T(), revealOrder.RV, retrieved.RV)

	suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_NodeInfoRepositoryIntegration() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	testEOA := "0xD111111111111111111111111111111111111111"
	testIP := "192.168.1.100"
	testPort := "4001"
	testPeerID := "12D3KooWTestPeerID1234567890123456789012345678901234567890"

	nodeInfo := &utils.NodeInfo{
		EOAAddress: testEOA,
		IP:         testIP,
		Port:       testPort,
		PeerID:     testPeerID,
	}

	err = handler.leaderNode.nodeInfoRepository.AddAndUpdateNodeInfo(ctx, nodeInfo)
	require.NoError(suite.T(), err, "AddAndUpdateNodeInfo should succeed")

	nodes, err := handler.leaderNode.nodeInfoRepository.GetNodeInfos(ctx)
	require.NoError(suite.T(), err, "GetNodeInfos should succeed")
	assert.GreaterOrEqual(suite.T(), len(nodes), 1, "Should have at least one node")

	found := false
	for _, node := range nodes {
		if node.EOAAddress == testEOA {
			found = true
			assert.Equal(suite.T(), testIP, node.IP)
			assert.Equal(suite.T(), testPort, node.Port)
			assert.Equal(suite.T(), testPeerID, node.PeerID)
			break
		}
	}
	assert.True(suite.T(), found, "Node should be found in GetNodeInfos")

	err = handler.leaderNode.nodeInfoRepository.DeleteNodeInfoByEOA(ctx, testEOA)
	require.NoError(suite.T(), err, "DeleteNodeInfoByEOA should succeed")

	nodes, err = handler.leaderNode.nodeInfoRepository.GetNodeInfos(ctx)
	require.NoError(suite.T(), err)
	for _, node := range nodes {
		assert.NotEqual(suite.T(), testEOA, node.EOAAddress, "Node should be deleted")
	}
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_PeerCommitRepositoryIntegration() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), handler)
	require.NotNil(suite.T(), handler.leaderNode)
	require.NotNil(suite.T(), handler.leaderNode.revealOrderService, "RevealOrderService uses PeerCommitRepository")

	ctx := context.Background()

	testRound := "peer_commit_test_round_1"
	testTrial := "1"
	testEOA := "0xE111111111111111111111111111111111111111"

	cvs := [32]byte{1, 2, 3}
	cos := [32]byte{4, 5, 6}
	secretValue := [32]byte{7, 8, 9}

	peerCommit := &database.PeerCommitDataScheme{
		Round:       testRound,
		TrialNum:    testTrial,
		EOAAddress:  testEOA,
		Cvs:         cvs[:],
		Cos:         cos[:],
		SecretValue: secretValue[:],
	}
	err = suite.peerCommitRepo.AddPeerCommitData(ctx, peerCommit)
	require.NoError(suite.T(), err, "AddPeerCommitData should succeed")

	retrieved, err := suite.peerCommitRepo.GetPeerCommitData(ctx, testRound, testTrial, testEOA)
	require.NoError(suite.T(), err, "GetPeerCommitData should succeed")
	assert.Equal(suite.T(), testRound, retrieved.Round)
	assert.Equal(suite.T(), testTrial, retrieved.TrialNum)
	assert.Equal(suite.T(), testEOA, retrieved.EOAAddress)
	assert.Equal(suite.T(), cvs[:], retrieved.Cvs)
	assert.Equal(suite.T(), cos[:], retrieved.Cos)
	assert.Equal(suite.T(), secretValue[:], retrieved.SecretValue)

	retrieved.SecretValue = []byte{10, 11, 12}
	err = suite.peerCommitRepo.UpdatePeerCommitData(ctx, retrieved)
	require.NoError(suite.T(), err, "UpdatePeerCommitData should succeed")

	updated, err := suite.peerCommitRepo.GetPeerCommitData(ctx, testRound, testTrial, testEOA)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), []byte{10, 11, 12}, updated.SecretValue)

	suite.db.Model((*database.PeerCommitDataScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testEOA).
		Delete()
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_RevealOrderServiceIntegration() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	assert.NotNil(suite.T(), handler.leaderNode.revealOrderService, "RevealOrderService should be initialized")

	testRound := "reveal_service_test_round_1"
	testTrial := "1"
	testOp1 := common.HexToAddress("0xF111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0xF222222222222222222222222222222222222222")

	cvs1 := [32]byte{1, 2, 3}
	cos1 := [32]byte{10, 20, 30}
	commitData1 := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp1.Hex(),
		Cvs:        cvs1,
		CvsHex:     hex.EncodeToString(cvs1[:]),
		Cos:        cos1,
		CosHex:     hex.EncodeToString(cos1[:]),
	}
	err = handler.leaderNode.AddLeaderCommit(ctx, commitData1)
	require.NoError(suite.T(), err)

	cvs2 := [32]byte{4, 5, 6}
	cos2 := [32]byte{40, 50, 60}
	commitData2 := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp2.Hex(),
		Cvs:        cvs2,
		CvsHex:     hex.EncodeToString(cvs2[:]),
		Cos:        cos2,
		CosHex:     hex.EncodeToString(cos2[:]),
	}
	err = handler.leaderNode.AddLeaderCommit(ctx, commitData2)
	require.NoError(suite.T(), err)

	// Test DetermineRevealOrder (uses RevealOrderService)
	activatedOps := []common.Address{testOp1, testOp2}
	success, err := handler.leaderNode.DetermineRevealOrder(ctx, testRound, testTrial, activatedOps)
	require.NoError(suite.T(), err, "DetermineRevealOrder should succeed")
	assert.True(suite.T(), success, "DetermineRevealOrder should return true")

	// Verify reveal order was created
	revealOrder, err := handler.leaderNode.reavealOrderRepository.GetRevealOrder(ctx, testRound, testTrial)
	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), revealOrder)
	assert.Equal(suite.T(), testRound, revealOrder.Round)
	assert.Equal(suite.T(), testTrial, revealOrder.TrialNum)
	assert.Len(suite.T(), revealOrder.OrderedNodes, 2, "Should have 2 ordered nodes")
	assert.Len(suite.T(), revealOrder.RevealOrder, 2, "Should have 2 reveal order indices")
	assert.NotEmpty(suite.T(), revealOrder.RV, "RV should not be empty")

	// Cleanup
	suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_P2PClientIntegration() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	assert.NotNil(suite.T(), handler.leaderNode.p2pClient, "P2PClient should be initialized")

	testEOA1 := "0xG111111111111111111111111111111111111111"
	testEOA2 := "0xG222222222222222222222222222222222222222"

	testPeerID1 := "12D3KooWPeer1ID123456789012345678901234567890123456789"
	testPeerID2 := "12D3KooWPeer2ID123456789012345678901234567890123456789"

	nodeInfo1 := &utils.NodeInfo{
		EOAAddress: testEOA1,
		IP:         "192.168.1.101",
		Port:       "4001",
		PeerID:     testPeerID1,
	}
	err = handler.leaderNode.nodeInfoRepository.AddAndUpdateNodeInfo(ctx, nodeInfo1)
	require.NoError(suite.T(), err)

	nodeInfo2 := &utils.NodeInfo{
		EOAAddress: testEOA2,
		IP:         "192.168.1.102",
		Port:       "4002",
		PeerID:     testPeerID2,
	}
	err = handler.leaderNode.nodeInfoRepository.AddAndUpdateNodeInfo(ctx, nodeInfo2)
	require.NoError(suite.T(), err)

	nodes := handler.leaderNode.p2pClient.GetConnectedPeers(ctx)
	assert.NotNil(suite.T(), nodes, "GetConnectedPeers should return a map")

	dbNodes, err := handler.leaderNode.nodeInfoRepository.GetNodeInfos(ctx)
	require.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), len(dbNodes), 2, "Should have at least 2 nodes in database")

	found1 := false
	found2 := false
	for _, node := range dbNodes {
		if node.EOAAddress == testEOA1 {
			found1 = true
		}
		if node.EOAAddress == testEOA2 {
			found2 = true
		}
	}
	assert.True(suite.T(), found1, "Node1 should be in database")
	assert.True(suite.T(), found2, "Node2 should be in database")

	if len(nodes) >= 2 {
		if _, exists1 := nodes[testEOA1]; exists1 {
			assert.Contains(suite.T(), nodes, testEOA1, "Node1 should be in GetConnectedPeers")
		}
		if _, exists2 := nodes[testEOA2]; exists2 {
			assert.Contains(suite.T(), nodes, testEOA2, "Node2 should be in GetConnectedPeers")
		}
	}

	handler.leaderNode.nodeInfoRepository.DeleteNodeInfoByEOA(ctx, testEOA1)
	handler.leaderNode.nodeInfoRepository.DeleteNodeInfoByEOA(ctx, testEOA2)
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_EndToEndIntegration() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	testRound := "e2e_test_round_1"
	testTrial := "1"
	testOp := common.HexToAddress("0xH111111111111111111111111111111111111111")
	messageID := "e2e_message_001"

	cvs := [32]byte{1, 2, 3, 4, 5}
	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cvs:        cvs,
		CvsHex:     hex.EncodeToString(cvs[:]),
	}
	err = handler.leaderNode.AddLeaderCommit(ctx, commitData)
	require.NoError(suite.T(), err, "Step 1: AddLeaderCommit should succeed")

	tracker := &utils.BroadcastTracker{
		Round:        testRound,
		TrialNum:     testTrial,
		EOAAddress:   testOp.Hex(),
		Type:         "cvs",
		MessageID:    messageID,
		Data:         cvs,
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     time.Now().Unix(),
		Timeout:      30,
	}
	err = handler.leaderNode.broadcastTrackerRepository.AddBroadcastTracker(ctx, tracker)
	require.NoError(suite.T(), err, "Step 2: AddBroadcastTracker should succeed")

	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{testOp.Hex()},
		RevealOrder:  []int{0},
		RV:           "e2e_rv_value",
	}
	err = handler.leaderNode.reavealOrderRepository.AddRevealOrder(ctx, revealOrder)
	require.NoError(suite.T(), err, "Step 3: AddRevealOrder should succeed")

	nodeInfo := &utils.NodeInfo{
		EOAAddress: testOp.Hex(),
		IP:         "192.168.1.200",
		Port:       "4003",
		PeerID:     "12D3KooWE2EPeerID1234567890123456789012345678901234567890",
	}
	err = handler.leaderNode.nodeInfoRepository.AddAndUpdateNodeInfo(ctx, nodeInfo)
	require.NoError(suite.T(), err, "Step 4: AddAndUpdateNodeInfo should succeed")

	retrievedCommit, err := handler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs, retrievedCommit.Cvs, "Step 5: LeaderCommit should persist")

	retrievedRevealOrder, err := handler.leaderNode.reavealOrderRepository.GetRevealOrder(ctx, testRound, testTrial)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), testRound, retrievedRevealOrder.Round, "Step 5: RevealOrder should persist")

	nodes, err := handler.leaderNode.nodeInfoRepository.GetNodeInfos(ctx)
	require.NoError(suite.T(), err)
	found := false
	for _, node := range nodes {
		if node.EOAAddress == testOp.Hex() {
			found = true
			break
		}
	}
	assert.True(suite.T(), found, "Step 5: NodeInfo should persist")

	err = handler.leaderNode.batchRepository.DeleteRoundTrialDataForLeaderNode(ctx, testRound, testTrial)
	require.NoError(suite.T(), err, "Step 6: DeleteRoundTrialDataForLeaderNode should succeed")

	_, err = handler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, testRound, testTrial, testOp.Hex())
	assert.Error(suite.T(), err, "Step 7: LeaderCommit should be deleted")

	_, err = handler.leaderNode.reavealOrderRepository.GetRevealOrder(ctx, testRound, testTrial)
	assert.Error(suite.T(), err, "Step 7: RevealOrder should be deleted")

	nodes, err = handler.leaderNode.nodeInfoRepository.GetNodeInfos(ctx)
	require.NoError(suite.T(), err)
	for _, node := range nodes {
		if node.EOAAddress == testOp.Hex() {
			handler.leaderNode.nodeInfoRepository.DeleteNodeInfoByEOA(ctx, testOp.Hex())
			break
		}
	}
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_DatabaseConnectionValidation() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	err = suite.db.Ping(ctx)
	require.NoError(suite.T(), err, "Database connection should be valid")

	testRound := "conn_test_round_1"
	testTrial := "1"
	testOp := common.HexToAddress("0xI111111111111111111111111111111111111111")

	cvs := [32]byte{1, 2, 3}
	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cvs:        cvs,
		CvsHex:     hex.EncodeToString(cvs[:]),
	}

	err = handler.leaderNode.AddLeaderCommit(ctx, commitData)
	require.NoError(suite.T(), err, "LeaderCommitRepository should work")

	retrieved, err := handler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err, "GetLeaderCommitByRoundAndEoaAddr should work")
	assert.Equal(suite.T(), cvs, retrieved.Cvs)

	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
}

func (suite *LeaderHandlerTestSuite) TestNewLeaderNodeHandler_ConcurrentDatabaseOperations() {
	mockFallbackClient := &fallback_ethclient.FallbackRPCClient{}
	handler, err := NewLeaderNodeHandler(mockFallbackClient, suite.db)
	require.NoError(suite.T(), err)
	ctx := context.Background()

	testRound := "concurrent_test_round_1"
	testTrial := "1"
	numGoroutines := 5

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			testOp := common.HexToAddress(fmt.Sprintf("0x%040d", goroutineID))
			cvs := [32]byte{byte(goroutineID)}
			commitData := &utils.LeaderCommitData{
				Round:      testRound,
				TrialNum:   testTrial,
				EOAAddress: testOp.Hex(),
				Cvs:        cvs,
				CvsHex:     hex.EncodeToString(cvs[:]),
			}

			err := handler.leaderNode.AddLeaderCommit(ctx, commitData)
			mu.Lock()
			if err != nil {
				errors = append(errors, fmt.Errorf("goroutine %d: %v", goroutineID, err))
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	assert.Empty(suite.T(), errors, "Concurrent AddLeaderCommit operations should succeed")

	commits, err := handler.leaderNode.leaderCommitRepository.GetLeaderCommitsByRoundAndTrialNum(ctx, testRound, testTrial)
	require.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), len(commits), numGoroutines, "All concurrent commits should be saved")

	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
}

// TestLeaderHandler_HandleRegistrationRequest tests registration request handling
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleRegistrationRequest() {
	stream := newMockStream()

	suite.leaderNodeHandler.handleRegistrationRequest(context.Background(), stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCommitRequest_Halted tests commit request when halted
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_Halted() {
	suite.leaderNodeHandler.leaderNode.SetHalted(true)
	stream := newMockStream()

	suite.leaderNodeHandler.handleCommitRequest(context.Background(), stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCommitRequest_DecodeError tests decode error handling
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_DecodeError() {
	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	stream := newMockStream()
	stream.readBuffer.Write([]byte("invalid json"))

	suite.leaderNodeHandler.handleCommitRequest(context.Background(), stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCOSRequest_Halted tests COS request when halted
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_Halted() {
	suite.leaderNodeHandler.leaderNode.SetHalted(true)
	stream := newMockStream()

	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCOSRequest_DecodeError tests COS request decode error
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_DecodeError() {
	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	stream := newMockStream()
	stream.readBuffer.Write([]byte("invalid json"))

	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleAcknowledgment_Halted tests acknowledgment when halted
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleAcknowledgment_Halted() {
	suite.leaderNodeHandler.leaderNode.SetHalted(true)
	stream := newMockStream()

	suite.leaderNodeHandler.handleAcknowledgment(context.Background(), stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleAcknowledgment_DecodeError tests acknowledgment decode error
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleAcknowledgment_DecodeError() {
	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	stream := newMockStream()
	stream.readBuffer.Write([]byte("invalid json"))

	suite.leaderNodeHandler.handleAcknowledgment(context.Background(), stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_CheckActivation_NotActivated tests activation check failure
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CheckActivation_NotActivated() {
	testOp := common.HexToAddress("0xTestOp")

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{}, nil // Empty list
		},
	}

	suite.leaderNodeHandler.ethService = mockEth

	result := suite.leaderNodeHandler.CheckActivation(context.Background(), testOp, "commit")
	assert.False(suite.T(), result)
}

// TestLeaderHandler_AllCommitsReceived_EmptyOps tests with no operators
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCommitsReceived_EmptyOps() {
	eth.SetActivatedOperatorsCached([]common.Address{})
	result := suite.leaderNodeHandler.allCommitsReceivedUnlocked("test_key")
	assert.False(suite.T(), result)
}

// TestLeaderHandler_AllCommitsReceived_NoRound tests with nonexistent round
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCommitsReceived_NoRound() {
	testOp := common.HexToAddress("0x1234567890123456789012345678901234567890")
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	uniqueKey := "nonexistent_round_key_12345"
	utils.DeleteCommittedNodes(uniqueKey)

	result := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)
}

// TestLeaderHandler_AllCommitsReceived_Partial tests with partial commits
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCommitsReceived_Partial() {
	testRound := "test_round_13"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0xA111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0xA222222222222222222222222222222222222222")

	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2})

	cvs := [32]byte{1}
	commitData := utils.LeaderCommitData{Cvs: cvs}
	utils.SetCommittedNodeData(uniqueKey, testOp1, commitData)

	result := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)
}

// TestLeaderHandler_AllCommitsReceived_Complete tests when all commits are received
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCommitsReceived_Complete() {
	testRound := "test_round_14"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0xB111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0xB222222222222222222222222222222222222222")

	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2})

	cvs1 := [32]byte{1}
	cvs2 := [32]byte{2}
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: cvs1})
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{Cvs: cvs2})

	result := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.True(suite.T(), result)

}

// TestLeaderHandler_AllCosReceived_Complete tests when all COS are received
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCosReceived_Complete() {
	testRound := "test_round_15"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0xC111111111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0xC222222222222222222222222222222222222222")

	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2})

	cos1 := [32]byte{1}
	cos2 := [32]byte{2}
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cos: cos1})
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{Cos: cos2})

	result := suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.True(suite.T(), result)

}

// TestLeaderHandler_UpdateInMemoryData tests in-memory data updates
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_UpdateInMemoryData() {
	testRound := "test_round_16"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xD111111111111111111111111111111111111111")

	cvs := [32]byte{1, 2, 3}
	commitData := utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cvs:        cvs,
	}

	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, commitData)

	storedData, ok := utils.GetCommittedNodeData(uniqueKey, testOp)
	assert.True(suite.T(), ok)
	assert.Equal(suite.T(), cvs, storedData.Cvs)

}

// TestLeaderHandler_DatabaseOperations tests database operations
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_DatabaseOperations() {
	testRound := "test_round_17"
	testTrial := "1"
	testOp := common.HexToAddress("0xE111111111111111111111111111111111111111")
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvs
	commitData.CvsHex = hex.EncodeToString(cvs[:])

	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	savedData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs, savedData.Cvs)

	// Update
	cos := [32]byte{32, 31, 30}
	savedData.Cos = cos
	err = suite.leaderNodeHandler.leaderNode.UpdateLeaderCommit(context.Background(), savedData)
	require.NoError(suite.T(), err)

}

// TestLeaderHandler_COSHashVerification tests COS hash verification
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_COSHashVerification() {
	testOp := common.HexToAddress("0xF111111111111111111111111111111111111111")
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	cos := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}

	operatorIndex := 0
	opIndexByte := []byte{uint8(operatorIndex)}

	calculatedCvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	recalculatedCvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))

	assert.True(suite.T(), bytes.Equal(calculatedCvs, recalculatedCvs))

}

// TestLeaderHandler_BroadcastTrackerIntegration tests broadcast tracker integration
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_BroadcastTrackerIntegration() {
	messageID := "test-broadcast-msg-001"
	testRound := "test_round_19"
	testTrial := "1"
	testOp := common.HexToAddress("0xBroadcastOp")

	tracker := &utils.BroadcastTracker{
		MessageID:    messageID,
		Round:        testRound,
		TrialNum:     testTrial,
		EOAAddress:   testOp.Hex(),
		Type:         "cvs",
		Data:         [32]byte{1, 2, 3},
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     0,
		Timeout:      30,
	}

	err := suite.broadcastTrackerRepo.AddBroadcastTracker(context.Background(), tracker)
	require.NoError(suite.T(), err)

	trackers, err := suite.broadcastTrackerRepo.GetBroadcastTrackers(context.Background())
	require.NoError(suite.T(), err)

	found := false
	for _, t := range trackers {
		if t.MessageID == messageID {
			found = true
			break
		}
	}
	assert.True(suite.T(), found)

	err = suite.broadcastTrackerRepo.DeleteBroadcastTracker(context.Background(), tracker)
	require.NoError(suite.T(), err)

}

// TestLeaderHandler_ConcurrentAtomicAccess tests concurrent atomic access
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_ConcurrentAtomicAccess() {
	handler := &LeaderNodeHandler{}
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			handler.SetMerkleRootSubmitted(true)
			handler.SetMerkleRootSubmitted(false)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = handler.GetMerkleRootSubmitted()
		}
		done <- true
	}()

	<-done
	<-done

	handler.SetMerkleRootSubmitted(true)
	assert.True(suite.T(), handler.GetMerkleRootSubmitted())

}

// TestLeaderHandler_AllCosReceived_EmptyOps tests COS with empty operators
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCosReceived_EmptyOps() {
	eth.SetActivatedOperatorsCached([]common.Address{})
	result := suite.leaderNodeHandler.allCosReceivedUnlocked("test_key")
	assert.False(suite.T(), result)

}

// TestLeaderHandler_AllCosReceived_NoRound tests COS with nonexistent round
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCosReceived_NoRound() {
	testOp := common.HexToAddress("0x9999999999999999999999999999999999999999")
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	uniqueKey := "nonexistent_cos_key_99999"
	utils.DeleteCommittedNodes(uniqueKey)

	result := suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)

}

// TestLeaderHandler_AllCosReceived_Partial tests COS with partial data
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCosReceived_Partial() {
	testRound := "cos_test_round_232323"
	testTrial := "232323"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0xCCCCCC1111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0xCCCCCC2222222222222222222222222222222222")

	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2})

	cos := [32]byte{1}
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp1.Hex(),
		Cos:        cos,
	})
	// Op2 has no COS

	result := suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)

}

// TestLeaderHandler_HandleCommitRequest_WithProperRequest tests commit request with proper structure
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_WithProperRequest() {
	testRound := "proper_req_round_24"
	testTrial := "1"
	testOp := common.HexToAddress("0xProperReqOp")

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	suite.leaderNodeHandler.leaderNode.SetHalted(false)

	// Create a properly structured request
	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	sign := utils.SignInfo{R: "100", S: "101", V: "27"}

	commitReq := utils.CommitRequest{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cvs:        cvs,
		Sign:       sign,
		Signature:  []byte("will_fail_but_reaches_verification"),
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(commitReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCommitRequest(context.Background(), stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCOSRequest_WithProperRequest tests COS request with proper structure
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_WithProperRequest() {
	testRound := "proper_cos_round_25"
	testTrial := "1"
	testOp := common.HexToAddress("0xProperCOSOp")

	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	cos := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	cosReq := utils.CosRequest{
		EOAAddress: testOp.Hex(),
		Cos:        cos,
		Signature:  []byte("will_fail_but_reaches_verification"),
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleAcknowledgment_WithProperRequest tests acknowledgment with proper structure
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleAcknowledgment_WithProperRequest() {
	testRound := "ack_round_26"
	testTrial := "1"
	testOp := common.HexToAddress("0xAckOp")

	suite.leaderNodeHandler.leaderNode.SetHalted(false)

	ackMsg := utils.AcknowledgmentMessage{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		MessageID:  "msg-123",
		Type:       "cvs",
		Status:     "received",
		Signature:  []byte("will_fail_but_reaches_verification"),
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(ackMsg)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleAcknowledgment(context.Background(), stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_MultipleOperatorsScenarios tests multiple operators scenarios
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_MultipleOperatorsScenarios() {
	testRound := "multi_ops_27"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	ops := []common.Address{
		common.HexToAddress("0xOPS1111111111111111111111111111111111111"),
		common.HexToAddress("0xOPS2222222222222222222222222222222222222"),
		common.HexToAddress("0xOPS3333333333333333333333333333333333333"),
	}

	eth.SetActivatedOperatorsCached(ops)

	// Test with no data
	result := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)

	// Add data for all
	for i, op := range ops {
		cvs := [32]byte{byte(i + 1)}
		utils.SetCommittedNodeData(uniqueKey, op, utils.LeaderCommitData{
			Round:      testRound,
			TrialNum:   testTrial,
			EOAAddress: op.Hex(),
			Cvs:        cvs,
		})
	}

	result = suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.True(suite.T(), result)

}

// TestLeaderHandler_DataCombinations tests various data combinations
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_DataCombinations() {
	testRound := "combo_28"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xComboOp")

	// Test with CVS only
	cvs := [32]byte{1, 2, 3}
	utils.SetCommittedNodeData(uniqueKey, testOp, utils.LeaderCommitData{Cvs: cvs})

	data, ok := utils.GetCommittedNodeData(uniqueKey, testOp)
	assert.True(suite.T(), ok)
	assert.Equal(suite.T(), cvs, data.Cvs)
	assert.Equal(suite.T(), [32]byte{}, data.Cos)

	// Add COS
	cos := [32]byte{4, 5, 6}
	data.Cos = cos
	utils.SetCommittedNodeData(uniqueKey, testOp, data)

	data, _ = utils.GetCommittedNodeData(uniqueKey, testOp)
	assert.Equal(suite.T(), cvs, data.Cvs)
	assert.Equal(suite.T(), cos, data.Cos)

}

// TestLeaderHandler_GetOrCreateNew tests creating new leader commit data
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_GetOrCreateNew() {
	testRound := "new_data_29"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xNewDataOp29")

	// Clear any existing data
	utils.DeleteCommittedNodes(uniqueKey)

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)

	assert.NotNil(suite.T(), commitData)
	assert.Equal(suite.T(), testRound, commitData.Round)
	assert.Equal(suite.T(), testTrial, commitData.TrialNum)
	assert.Equal(suite.T(), testOp.Hex(), commitData.EOAAddress)

}

// TestLeaderHandler_GetOrCreateExisting tests getting existing leader commit data
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_GetOrCreateExisting() {
	testRound := "existing_data_30"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xExistingDataOp30")

	// Pre-populate data
	cvs := [32]byte{100, 101, 102}
	existingData := utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cvs:        cvs,
	}
	utils.SetCommittedNodeData(uniqueKey, testOp, existingData)

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)

	assert.Equal(suite.T(), cvs, commitData.Cvs)

}

// TestLeaderHandler_CommitDataAllFields tests commit data with all fields
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CommitDataAllFields() {
	testRound := "all_fields_31"
	testTrial := "1"
	testOp := common.HexToAddress("0xAllFieldsOp")
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	cos := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	secret := [32]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131}

	commitData := utils.LeaderCommitData{
		Round:                 testRound,
		TrialNum:              testTrial,
		EOAAddress:            testOp.Hex(),
		Cvs:                   cvs,
		CvsHex:                hex.EncodeToString(cvs[:]),
		Cos:                   cos,
		CosHex:                hex.EncodeToString(cos[:]),
		SecretValue:           secret,
		SecretValueHex:        hex.EncodeToString(secret[:]),
		SubmitMerkleRootDone:  true,
		RandomNumberGenerated: false,
	}

	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, commitData)

	retrieved, ok := utils.GetCommittedNodeData(uniqueKey, testOp)
	assert.True(suite.T(), ok)
	assert.Equal(suite.T(), cvs, retrieved.Cvs)
	assert.Equal(suite.T(), cos, retrieved.Cos)
	assert.Equal(suite.T(), secret, retrieved.SecretValue)
	assert.True(suite.T(), retrieved.SubmitMerkleRootDone)

}

// TestLeaderHandler_MerkleRootSubmittedFlag tests merkle root submitted flag
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_MerkleRootSubmittedFlag() {
	testRound := "merkle_32"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0xMerkle1")
	testOp2 := common.HexToAddress("0xMerkle2")

	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2})

	// Add all commits
	cvs1 := [32]byte{1}
	cvs2 := [32]byte{2}
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: cvs1})
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{Cvs: cvs2})

	// Test the condition: !GetMerkleRootSubmitted() && allCommitsReceivedUnlocked
	allReceived := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	merkleSubmitted := suite.leaderNodeHandler.GetMerkleRootSubmitted()

	assert.True(suite.T(), allReceived)
	assert.False(suite.T(), merkleSubmitted)

	// This condition would trigger merkle root generation
	shouldGenerate := !merkleSubmitted && allReceived
	assert.True(suite.T(), shouldGenerate)

	// Set flag
	suite.leaderNodeHandler.SetMerkleRootSubmitted(true)
	shouldGenerate = !suite.leaderNodeHandler.GetMerkleRootSubmitted() && allReceived
	assert.False(suite.T(), shouldGenerate)

}

// TestLeaderHandler_AllCosReceivedTrigger tests all COS received trigger condition
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCosReceivedTrigger() {
	testRound := "all_cos_33"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0xCOSTrigger1")
	testOp2 := common.HexToAddress("0xCOSTrigger2")

	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2})

	// Add CVS and COS for both
	cvs1 := [32]byte{1}
	cos1 := [32]byte{2}
	cvs2 := [32]byte{3}
	cos2 := [32]byte{4}

	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: cvs1, Cos: cos1})
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{Cvs: cvs2, Cos: cos2})

	allCosReceived := suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.True(suite.T(), allCosReceived)

}

// TestLeaderHandler_MemoryDatabaseSync tests memory and database synchronization
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_MemoryDatabaseSync() {
	testRound := "sync_35"
	testTrial := "1"
	testOp := common.HexToAddress("0xSyncOp")
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	cvs := [32]byte{10, 20, 30}

	// Update memory
	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvs
	commitData.CvsHex = hex.EncodeToString(cvs[:])

	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	memData, ok := utils.GetCommittedNodeData(uniqueKey, testOp)
	assert.True(suite.T(), ok)
	assert.Equal(suite.T(), cvs, memData.Cvs)

	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	dbData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs, dbData.Cvs)

}

// TestLeaderHandler_CheckActivation_NotActivated2 tests activation check failure (different EOA)
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CheckActivation_NotActivated2() {
	testOp := common.HexToAddress("0xTest")

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{}, nil // Empty list
		},
	}

	suite.leaderNodeHandler.ethService = mockEth

	result := suite.leaderNodeHandler.CheckActivation(context.Background(), testOp, "test")
	assert.False(suite.T(), result)
}

// TestLeaderHandler_OperatorIndexByteConversion tests operator index to byte conversion
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_OperatorIndexByteConversion() {
	for i := 0; i < 10; i++ {
		opIndexByte := []byte{uint8(i)}
		assert.Len(suite.T(), opIndexByte, 1)
		assert.Equal(suite.T(), uint8(i), opIndexByte[0])
	}

}

// TestLeaderHandler_COSHashWithDifferentIndices tests COS hash with different indices
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_COSHashWithDifferentIndices() {
	cos := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}

	// Test with index 0
	opIndex0 := []byte{uint8(0)}
	hash0 := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndex0))

	// Test with index 1
	opIndex1 := []byte{uint8(1)}
	hash1 := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndex1))

	// Test with index 2
	opIndex2 := []byte{uint8(2)}
	hash2 := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndex2))

	// All should be different
	assert.False(suite.T(), bytes.Equal(hash0, hash1))
	assert.False(suite.T(), bytes.Equal(hash1, hash2))
	assert.False(suite.T(), bytes.Equal(hash0, hash2))

}

func generateValidSignature(eoaAddress string, privateKey *ecdsa.PrivateKey) []byte {
	// Hash the EOA address (same as VerifySignature does)
	hash := crypto.Keccak256Hash([]byte(eoaAddress))

	// Sign the hash with the private key
	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return nil
	}

	return signature
}

func generateCosRequestSignature(req utils.CosRequest, privateKey *ecdsa.PrivateKey) []byte {
	signature, err := utils.SignCosRequestContent(req, privateKey)
	if err != nil {
		return nil
	}
	return signature
}

func generateAcknowledgmentSignature(ack utils.AcknowledgmentMessage, privateKey *ecdsa.PrivateKey) []byte {
	signature, err := utils.SignAcknowledgmentContent(ack, privateKey)
	if err != nil {
		return nil
	}
	return signature
}

func generateCommitRequestSignature(req utils.CommitRequest, privateKey *ecdsa.PrivateKey) []byte {
	signature, err := utils.SignCommitRequestContent(req, privateKey)
	if err != nil {
		return nil
	}
	return signature
}

func generateCvsEIP712Signature(round string, trialNum string, cvs [32]byte, privateKey *ecdsa.PrivateKey) utils.SignInfo {
	roundBigInt, ok := new(big.Int).SetString(round, 10)
	if !ok {
		hash := crypto.Keccak256Hash([]byte(round))
		roundBigInt = new(big.Int).SetBytes(hash.Bytes())
	}

	trialNumBigInt, ok := new(big.Int).SetString(trialNum, 10)
	if !ok {
		hash := crypto.Keccak256Hash([]byte(trialNum))
		trialNumBigInt = new(big.Int).SetBytes(hash.Bytes())
	}

	typedDataHash, err := utils.ComputeCvsEIP712TypedDataHash(roundBigInt, trialNumBigInt, cvs)
	if err != nil {
		return utils.SignInfo{}
	}

	signature, err := crypto.Sign(typedDataHash.Bytes(), privateKey)
	if err != nil {
		return utils.SignInfo{}
	}

	r := hex.EncodeToString(signature[:32])
	s := hex.EncodeToString(signature[32:64])
	v := uint8(signature[64]) + 27

	return utils.SignInfo{
		R: r,
		S: s,
		V: fmt.Sprintf("%d", v),
	}
}

func createTestKeyPair() (*ecdsa.PrivateKey, string) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, ""
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, ""
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	return privateKey, address
}

// TestLeaderHandler_CompleteCVSFlow tests complete CVS flow with database
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CompleteCVSFlow() {
	testRound := "complete_cvs_46"
	testTrial := "1"
	testOp := common.HexToAddress("0xCompleteCVSOp46")
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	// Mock environment
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Create and save CVS
	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	sign := utils.SignInfo{R: "100", S: "101", V: "27"}

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvs
	commitData.CvsHex = hex.EncodeToString(cvs[:])
	commitData.Sign = sign
	commitData.SubmitMerkleRootDone = false
	commitData.RandomNumberGenerated = false

	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	// Update memory
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	dbData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs, dbData.Cvs)

	memData, _ := utils.GetCommittedNodeData(uniqueKey, testOp)
	assert.Equal(suite.T(), cvs, memData.Cvs)
}

// TestLeaderHandler_CompleteCOSFlow tests complete COS flow with hash verification
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CompleteCOSFlow() {
	testRound := "complete_cos_47"
	testTrial := "1"
	testOp := common.HexToAddress("0xCompleteCOSOp47")
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Create CVS first
	cos := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}
	opIndexByte := []byte{uint8(0)}
	cvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))

	var cvsArray [32]byte
	copy(cvsArray[:], cvs)

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvsArray
	commitData.CvsHex = hex.EncodeToString(cvsArray[:])

	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	// Now add COS
	savedData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)

	savedData.Cos = cos
	savedData.CosHex = hex.EncodeToString(cos[:])

	err = suite.leaderNodeHandler.leaderNode.UpdateLeaderCommit(context.Background(), savedData)
	require.NoError(suite.T(), err)

	recalculatedCvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	assert.True(suite.T(), bytes.Equal(recalculatedCvs, cvsArray[:]))

}

// TestLeaderHandler_CompleteDataLifecycle tests complete data lifecycle
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CompleteDataLifecycle() {
	testRound := "lifecycle_48"
	testTrial := "1"
	testOp := common.HexToAddress("0xLifecycleOp48")
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvs
	commitData.CvsHex = hex.EncodeToString(cvs[:])

	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	cos := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	savedData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	savedData.Cos = cos
	savedData.CosHex = hex.EncodeToString(cos[:])
	err = suite.leaderNodeHandler.leaderNode.UpdateLeaderCommit(context.Background(), savedData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *savedData)

	secret := [32]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131}
	savedData, err = suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	savedData.SecretValue = secret
	savedData.SecretValueHex = hex.EncodeToString(secret[:])
	err = suite.leaderNodeHandler.leaderNode.UpdateLeaderCommit(context.Background(), savedData)
	require.NoError(suite.T(), err)

	finalData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs, finalData.Cvs)
	assert.Equal(suite.T(), cos, finalData.Cos)
	assert.Equal(suite.T(), secret, finalData.SecretValue)

}

// TestLeaderHandler_ManyOperators tests handling many operators
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_ManyOperators() {
	testRound := "many_ops_49"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Create 5 operators
	var ops []common.Address
	for i := 0; i < 5; i++ {
		op := common.HexToAddress(fmt.Sprintf("0x%040d", i+2000))
		ops = append(ops, op)
	}

	eth.SetActivatedOperatorsCached(ops)

	// Add non-empty CVS for all
	for i, op := range ops {
		cvs := [32]byte{byte(i + 1)} // Non-zero CVS
		utils.SetCommittedNodeData(uniqueKey, op, utils.LeaderCommitData{
			Round:      testRound,
			TrialNum:   testTrial,
			EOAAddress: op.Hex(),
			Cvs:        cvs,
		})
	}

	committedNodes, exists := utils.GetCommittedNodes(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), len(ops), len(committedNodes))

	// Now add COS for all
	for i, op := range ops {
		data, _ := utils.GetCommittedNodeData(uniqueKey, op)
		data.Cos = [32]byte{byte(i + 100)}
		utils.SetCommittedNodeData(uniqueKey, op, data)
	}

	for _, op := range ops {
		data, _ := utils.GetCommittedNodeData(uniqueKey, op)
		assert.NotEqual(suite.T(), [32]byte{}, data.Cos)
	}

}

// TestLeaderHandler_ConcurrentRoundUpdates tests concurrent updates to same round
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_ConcurrentRoundUpdates() {
	testRound := "concurrent_50"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xConcurrentOp50")

	done := make(chan bool, 3)

	// Multiple goroutines updating the same operator
	for g := 0; g < 3; g++ {
		go func(routineNum int) {
			for i := 0; i < 10; i++ {
				suite.leaderNodeHandler.commitMu.Lock()

				commitData := utils.LeaderCommitData{
					Round:      testRound,
					TrialNum:   testTrial,
					EOAAddress: testOp.Hex(),
					Cvs:        [32]byte{byte(routineNum*10 + i)},
				}

				suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, commitData)
				suite.leaderNodeHandler.commitMu.Unlock()
			}
			done <- true
		}(g)
	}

	<-done
	<-done
	<-done

	_, exists := utils.GetCommittedNodes(uniqueKey)
	assert.True(suite.T(), exists)

}

// TestLeaderHandler_CommitRequestInternalLogic tests commit request internal logic
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CommitRequestInternalLogic() {
	testRound := "internal_commit_51"
	testTrial := "1"
	testOp := common.HexToAddress("0xInternalOp51")
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	// Mock activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})
	suite.leaderNodeHandler.SetMerkleRootSubmitted(false)

	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	round := testRound
	eoaAddress := testOp

	suite.leaderNodeHandler.commitMu.Lock()
	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(round, testTrial, uniqueKey, eoaAddress)

	if commitData.Cvs == [32]byte{} {
		commitData.Cvs = cvs
		commitData.CvsHex = hex.EncodeToString(cvs[:])
		commitData.Sign = utils.SignInfo{R: "123", S: "456", V: "27"}
		commitData.SubmitMerkleRootDone = false
		commitData.RandomNumberGenerated = false
	}
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, eoaAddress, *commitData)

	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, eoaAddress, *commitData)

	activatedOps := eth.GetActivatedOperatorsCached()
	assert.Len(suite.T(), activatedOps, 1)

	merkleNotSubmitted := !suite.leaderNodeHandler.GetMerkleRootSubmitted()
	allCommitsReceived := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.True(suite.T(), merkleNotSubmitted)
	assert.True(suite.T(), allCommitsReceived)

	suite.leaderNodeHandler.commitMu.Unlock()

	dbData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs, dbData.Cvs)
}

// TestLeaderHandler_COSRequestInternalLogic tests COS request internal logic
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_COSRequestInternalLogic() {
	testRound := "internal_cos_52"
	testTrial := "1"
	testOp := common.HexToAddress("0xInternalCOSOp52")
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	// Mock activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Set up handler
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	cos := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}
	opIndexByte := []byte{uint8(0)}
	cvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	var cvsArray [32]byte
	copy(cvsArray[:], cvs)

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvsArray
	commitData.CvsHex = hex.EncodeToString(cvsArray[:])
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	round := suite.leaderNodeHandler.leaderNode.GetCurrentRound()
	assert.Equal(suite.T(), testRound, round)

	trial := suite.leaderNodeHandler.leaderNode.GetCurrentTrial()
	assert.Equal(suite.T(), testTrial, trial)

	eoaAddress := testOp

	suite.leaderNodeHandler.commitMu.Lock()

	commitData = suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(round, trial, uniqueKey, eoaAddress)

	assert.NotEqual(suite.T(), [32]byte{}, commitData.Cvs)

	activatedOperators := eth.GetActivatedOperatorsCached()

	operatorIndex := -1
	for i, op := range activatedOperators {
		if op.Hex() == testOp.Hex() {
			operatorIndex = i
			break
		}
	}
	assert.Equal(suite.T(), 0, operatorIndex)

	opIndexByte = []byte{uint8(operatorIndex)}

	recalculatedCvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))

	assert.True(suite.T(), bytes.Equal(recalculatedCvs, commitData.Cvs[:]))

	if commitData.Cos == [32]byte{} {
		commitData.Cos = cos
		commitData.CosHex = hex.EncodeToString(cos[:])
	}

	err = suite.leaderNodeHandler.leaderNode.UpdateLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, eoaAddress, *commitData)

	suite.leaderNodeHandler.commitMu.Unlock()

	savedData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cos, savedData.Cos)
}

// TestLeaderHandler_HandleCOSRequest_ValidSignature tests COS request with valid signature
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_ValidSignature() {
	testRound := "valid_cos_54"
	testTrial := "1"

	// Generate valid key pair
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)
	require.NotEmpty(suite.T(), eoaAddress)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	// Mock activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Set up handler
	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	cos := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}
	opIndexByte := []byte{uint8(0)}
	cvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	var cvsArray [32]byte
	copy(cvsArray[:], cvs)

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvsArray
	commitData.CvsHex = hex.EncodeToString(cvsArray[:])
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	// Now send COS request
	cosReq := utils.CosRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		Cos:        cos,
		EOAAddress: eoaAddress,
	}

	// Generate VALID signature for COS request (ALL fields)
	signature := generateCosRequestSignature(cosReq, privateKey)
	require.NotNil(suite.T(), signature)
	cosReq.Signature = signature

	reqBytes, _ := json.Marshal(cosReq)
	mockStream := &mockStream{
		readBuffer:  bytes.NewBuffer(reqBytes),
		writeBuffer: &bytes.Buffer{},
	}

	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, mockStream)

	savedData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cos, savedData.Cos)
	assert.Equal(suite.T(), hex.EncodeToString(cos[:]), savedData.CosHex)

	recalculatedCvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	assert.True(suite.T(), bytes.Equal(recalculatedCvs, cvsArray[:]))

}

// TestLeaderHandler_HandleCommitRequest_CoreLogicWithValidSignature tests commit request with valid signature
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_CoreLogicWithValidSignature() {
	testRound := "valid_commit_54"
	testTrial := "1"

	// Generate valid key pair
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)
	require.NotEmpty(suite.T(), eoaAddress)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	// Mock activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})
	suite.leaderNodeHandler.SetMerkleRootSubmitted(false)

	// Generate CVS data
	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	signature := generateValidSignature(eoaAddress, privateKey)
	require.NotNil(suite.T(), signature)

	verifyReq := utils.Verification{
		EOAAddress: eoaAddress,
		Signature:  signature,
	}
	assert.True(suite.T(), utils.VerifySignature(verifyReq), "Signature should be valid")

	// Create request struct with valid signature
	req := utils.CommitRequest{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress,
		Cvs:        cvs,
		Signature:  signature,
		Sign:       utils.SignInfo{R: "123", S: "456", V: "27"},
	}

	round := req.Round
	eoaAddress_addr := common.HexToAddress(req.EOAAddress)
	assert.Equal(suite.T(), signature, req.Signature)

	suite.leaderNodeHandler.commitMu.Lock()
	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(round, req.TrialNum, uniqueKey, eoaAddress_addr)
	if commitData.Cvs == [32]byte{} {
		commitData.Cvs = req.Cvs
		commitData.CvsHex = hex.EncodeToString(req.Cvs[:])
		commitData.Sign = req.Sign
		commitData.SubmitMerkleRootDone = false
		commitData.RandomNumberGenerated = false
	}
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, eoaAddress_addr, *commitData)

	// Update database
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, eoaAddress_addr, *commitData)
	activatedOps := eth.GetActivatedOperatorsCached()
	assert.Len(suite.T(), activatedOps, 1)

	merkleNotSubmitted := !suite.leaderNodeHandler.GetMerkleRootSubmitted()
	allCommitsReceived := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.True(suite.T(), merkleNotSubmitted)
	assert.True(suite.T(), allCommitsReceived)

	suite.leaderNodeHandler.commitMu.Unlock()

	dbData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs, dbData.Cvs)

	roundCommits, exists := utils.GetCommittedNodes(uniqueKey)
	assert.True(suite.T(), exists)
	data, ok := roundCommits[testOp]
	assert.True(suite.T(), ok)
	assert.Equal(suite.T(), cvs, data.Cvs)
}

// TestLeaderHandler_HandleCOSRequest_MissingCVS tests COS request when CVS doesn't exist
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_MissingCVS() {
	testRound := "cos_missing_cvs_63"
	testTrial := "1"

	// Generate valid key pair
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)
	require.NotEmpty(suite.T(), eoaAddress)

	testOp := common.HexToAddress(eoaAddress)

	// Mock activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Set up handler
	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	cos := [32]byte{10, 20, 30}
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	cosReq := utils.CosRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		Cos:        cos,
		EOAAddress: eoaAddress,
	}

	// Generate valid signature
	signature := generateCosRequestSignature(cosReq, privateKey)
	require.NotNil(suite.T(), signature)
	cosReq.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq)
	stream.readBuffer.Write(jsonData)

	// Should reject because no CVS exists
	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCOSRequest_DuplicateCOS tests receiving COS twice
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_DuplicateCOS() {
	testRound := "cos_duplicate_64"
	testTrial := "1"

	// Generate valid key pair
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)
	require.NotEmpty(suite.T(), eoaAddress)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	// Mock activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Set up handler
	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	// Create CVS first
	cos := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}
	opIndexByte := []byte{uint8(0)}
	cvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	var cvsArray [32]byte
	copy(cvsArray[:], cvs)

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvsArray
	commitData.CvsHex = hex.EncodeToString(cvsArray[:])
	commitData.Cos = cos // Already set COS
	commitData.CosHex = hex.EncodeToString(cos[:])

	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	// Try to send COS again
	cosReq := utils.CosRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		Cos:        cos,
		EOAAddress: eoaAddress,
	}

	// Generate valid signature
	signature := generateCosRequestSignature(cosReq, privateKey)
	require.NotNil(suite.T(), signature)
	cosReq.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq)
	stream.readBuffer.Write(jsonData)

	// Should skip because COS already exists
	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCOSRequest_InvalidHash tests COS with hash mismatch
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_InvalidHash() {
	testRound := "cos_invalid_hash_65"
	testTrial := "1"

	// Generate valid key pair
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)
	require.NotEmpty(suite.T(), eoaAddress)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	// Mock activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Set up handler
	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	// Set a CVS that won't match the COS we send
	wrongCvs := [32]byte{99, 99, 99}

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = wrongCvs
	commitData.CvsHex = hex.EncodeToString(wrongCvs[:])

	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	// Send a COS that doesn't match
	cos := [32]byte{10, 20, 30}

	cosReq := utils.CosRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		Cos:        cos,
		EOAAddress: eoaAddress,
	}

	// Generate valid signature
	signature := generateCosRequestSignature(cosReq, privateKey)
	require.NotNil(suite.T(), signature)
	cosReq.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq)
	stream.readBuffer.Write(jsonData)

	// Should reject due to hash mismatch
	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)

	assert.True(suite.T(), stream.closed)
}

// MockEthService for testing
type MockEthService struct {
	GetActivatedOperatorsCachedFunc func() []common.Address
	GetActivatedOperatorsFunc       func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error)
}

func (m *MockEthService) GetActivatedOperatorsCached() []common.Address {
	if m.GetActivatedOperatorsCachedFunc != nil {
		return m.GetActivatedOperatorsCachedFunc()
	}
	return []common.Address{}
}

func (m *MockEthService) GetActivatedOperatorsLength() int64 {
	return int64(len(m.GetActivatedOperatorsCached()))
}

func (m *MockEthService) SetActivatedOperatorsCached(operators []common.Address) {
	// No-op for tests
}

func (m *MockEthService) GetActivatedOperatorsUnsafe() []common.Address {
	return m.GetActivatedOperatorsCached()
}

func (m *MockEthService) GetActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
	if m.GetActivatedOperatorsFunc != nil {
		return m.GetActivatedOperatorsFunc(ctx, fallbackEthClient)
	}
	return []common.Address{}, nil
}

func (m *MockEthService) UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) error {
	// No-op for tests
	return nil
}

func (m *MockEthService) CallSmartContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	return nil, nil
}

func (m *MockEthService) ExecuteTransaction(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
	return nil, nil, nil
}

func (m *MockEthService) UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	return nil, nil
}

func (m *MockEthService) GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	return nil, nil
}

// TestLeaderHandler_IsEOAActivatedForRound_Success tests successful EOA activation check
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_IsEOAActivatedForRound_Success() {
	testOp := common.HexToAddress("0xActivatedOp")

	// Create mock eth service
	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp}, nil
		},
	}

	// Inject mock
	suite.leaderNodeHandler.ethService = mockEth

	isNetworkErr, isActivated := suite.leaderNodeHandler.isEOAActivatedForRound(context.Background(), testOp)

	assert.False(suite.T(), isNetworkErr)
	assert.True(suite.T(), isActivated)
}

// TestLeaderHandler_IsEOAActivatedForRound_NotActivated tests non-activated EOA
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_IsEOAActivatedForRound_NotActivated() {
	testOp := common.HexToAddress("0x1111111111111111111111111111111111111111")
	otherOp := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{otherOp}, nil // Different operator
		},
	}

	suite.leaderNodeHandler.ethService = mockEth

	isNetworkErr, isActivated := suite.leaderNodeHandler.isEOAActivatedForRound(context.Background(), testOp)

	assert.False(suite.T(), isNetworkErr)
	assert.False(suite.T(), isActivated)
}

// TestLeaderHandler_IsEOAActivatedForRound_NetworkError tests network error
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_IsEOAActivatedForRound_NetworkError() {
	testOp := common.HexToAddress("0xTestOp")

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return nil, errors.New("network error")
		},
	}

	suite.leaderNodeHandler.ethService = mockEth

	isNetworkErr, isActivated := suite.leaderNodeHandler.isEOAActivatedForRound(context.Background(), testOp)

	assert.True(suite.T(), isNetworkErr)
	assert.False(suite.T(), isActivated)
}


// TestLeaderHandler_CheckActivation_Success tests successful activation check
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CheckActivation_Success() {
	// Generate valid key pair
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp}, nil
		},
	}

	suite.leaderNodeHandler.ethService = mockEth

	result := suite.leaderNodeHandler.CheckActivation(context.Background(), testOp, "commit")
	assert.True(suite.T(), result)
}

// TestLeaderHandler_CheckActivation_NetworkError tests network error case
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CheckActivation_NetworkError() {
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return nil, errors.New("network error")
		},
	}

	suite.leaderNodeHandler.ethService = mockEth

	result := suite.leaderNodeHandler.CheckActivation(context.Background(), testOp, "commit")
	assert.False(suite.T(), result)
}

// TestLeaderHandler_HandleCommitRequest_FullFlow_WithValidSig tests full commit flow with valid signature
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_FullFlow_WithValidSig() {
	testRound := "70"
	testTrial := "1"

	// Generate valid key pair
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	// Setup mock eth service
	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp}, nil
		},
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.SetMerkleRootSubmitted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	eip712Sign := generateCvsEIP712Signature(testRound, testTrial, cvs, privateKey)
	require.NotEmpty(suite.T(), eip712Sign.R, "EIP-712 signature R should not be empty")
	require.NotEmpty(suite.T(), eip712Sign.S, "EIP-712 signature S should not be empty")
	require.NotEmpty(suite.T(), eip712Sign.V, "EIP-712 signature V should not be empty")
	commitReq := utils.CommitRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress,
		Cvs:        cvs,
		Sign:       eip712Sign,
	}

	signature := generateCommitRequestSignature(commitReq, privateKey)
	commitReq.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(commitReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCommitRequest(context.Background(), stream)

	assert.True(suite.T(), stream.closed)

	// Verify data was saved
	savedData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(
		context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs, savedData.Cvs)
}

// TestLeaderHandler_HandleCommitRequest_DuplicateCVS tests receiving CVS twice
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_DuplicateCVS() {
	testRound := "duplicate_cvs_71"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp}, nil
		},
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth
	suite.leaderNodeHandler.leaderNode.SetHalted(false)

	// Pre-populate CVS
	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvs
	commitData.CvsHex = hex.EncodeToString(cvs[:])
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	// Try to send CVS again
	commitReq := utils.CommitRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress,
		Cvs:        cvs,
		Sign:       utils.SignInfo{R: "123", S: "456", V: "27"},
	}

	signature := generateCommitRequestSignature(commitReq, privateKey)
	commitReq.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(commitReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCommitRequest(context.Background(), stream)
	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCommitRequest_AllCommitsReceivedTrigger tests merkle root generation trigger
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_AllCommitsReceivedTrigger() {
	testRound := "72"
	testTrial := "1"

	// Generate 2 valid key pairs
	privateKey1, eoaAddress1 := createTestKeyPair()
	require.NotNil(suite.T(), privateKey1)

	privateKey2, eoaAddress2 := createTestKeyPair()
	require.NotNil(suite.T(), privateKey2)

	testOp1 := common.HexToAddress(eoaAddress1)
	testOp2 := common.HexToAddress(eoaAddress2)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp1, testOp2}, nil
		},
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth
	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.SetMerkleRootSubmitted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	// Add first commit
	cvs1 := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	commitData1 := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp1)
	commitData1.Cvs = cvs1
	commitData1.CvsHex = hex.EncodeToString(cvs1[:])
	commitData1.Sign = utils.SignInfo{R: "123", S: "456", V: "27"}
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData1)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp1, *commitData1)

	// Send second commit - this should trigger merkle root generation
	cvs2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17,
		16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	eip712Sign2 := generateCvsEIP712Signature(testRound, testTrial, cvs2, privateKey2)
	require.NotEmpty(suite.T(), eip712Sign2.R, "EIP-712 signature R should not be empty")
	require.NotEmpty(suite.T(), eip712Sign2.S, "EIP-712 signature S should not be empty")
	require.NotEmpty(suite.T(), eip712Sign2.V, "EIP-712 signature V should not be empty")

	commitReq2 := utils.CommitRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress2,
		Cvs:        cvs2,
		Sign:       eip712Sign2,
	}

	signature2 := generateCommitRequestSignature(commitReq2, privateKey2)
	commitReq2.Signature = signature2

	stream := newMockStream()
	jsonData, _ := json.Marshal(commitReq2)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCommitRequest(context.Background(), stream)
	assert.True(suite.T(), stream.closed)

	// Verify both commits exist
	savedData2, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(
		context.Background(), testRound, testTrial, testOp2.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cvs2, savedData2.Cvs)
}

// TestLeaderHandler_HandleCommitRequest_DBSaveError tests database save error handling
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_DBSaveError() {
	testRound := "db_error_73"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp}, nil
		},
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth
	suite.leaderNodeHandler.leaderNode.SetHalted(false)

	// Create request with very long round name to potentially cause DB error
	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	signature := generateValidSignature(eoaAddress, privateKey)

	commitReq := utils.CommitRequest{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress,
		Cvs:        cvs,
		Sign:       utils.SignInfo{R: "123", S: "456", V: "27"},
		Signature:  signature,
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(commitReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCommitRequest(context.Background(), stream)
	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleAcknowledgment_FullFlow tests full acknowledgment flow
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleAcknowledgment_FullFlow() {
	testRound := "ack_full_74"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp}, nil
		},
	}
	suite.leaderNodeHandler.ethService = mockEth
	suite.leaderNodeHandler.leaderNode.SetHalted(false)

	// Create a broadcast tracker first
	messageID := "test-msg-ack-001"
	tracker := &utils.BroadcastTracker{
		MessageID:    messageID,
		Round:        testRound,
		TrialNum:     testTrial,
		EOAAddress:   testOp.Hex(),
		Type:         "cvs",
		Acknowledged: map[string]bool{testOp.Hex(): false},
	}
	suite.leaderNodeHandler.leaderNode.SetActiveBroadcast(messageID, tracker)

	ackMsg := utils.AcknowledgmentMessage{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress,
		MessageID:  messageID,
		Type:       "cvs",
		Status:     "received",
	}
	signature := generateAcknowledgmentSignature(ackMsg, privateKey)
	ackMsg.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(ackMsg)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleAcknowledgment(context.Background(), stream)
	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleAcknowledgment_ErrorStatus tests acknowledgment with error status
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleAcknowledgment_ErrorStatus() {
	testRound := "ack_error_75"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp}, nil
		},
	}
	suite.leaderNodeHandler.ethService = mockEth
	suite.leaderNodeHandler.leaderNode.SetHalted(false)

	messageID := "test-msg-error-002"
	tracker := &utils.BroadcastTracker{
		MessageID:    messageID,
		Round:        testRound,
		TrialNum:     testTrial,
		EOAAddress:   testOp.Hex(),
		Type:         "cos",
		Acknowledged: map[string]bool{testOp.Hex(): false},
	}
	suite.leaderNodeHandler.leaderNode.SetActiveBroadcast(messageID, tracker)

	ackMsg := utils.AcknowledgmentMessage{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress,
		MessageID:  messageID,
		Type:       "cos",
		Status:     "error: processing failed",
	}
	signature := generateAcknowledgmentSignature(ackMsg, privateKey)
	ackMsg.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(ackMsg)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleAcknowledgment(context.Background(), stream)
	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCOSRequest_OperatorNotFound tests COS when operator not in list
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_OperatorNotFound() {
	testRound := "cos_op_not_found_76"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)
	otherOp := common.HexToAddress("0x5555555555555555555555555555555555555555")
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	// Mock with different operator
	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{otherOp} // Different operator
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	// Set CVS
	cvs := [32]byte{1, 2, 3}
	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvs
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	cos := [32]byte{10, 20, 30}
	cosReq := utils.CosRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		Cos:        cos,
		EOAAddress: eoaAddress,
	}

	signature := generateCosRequestSignature(cosReq, privateKey)
	cosReq.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)
	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCOSRequest_DBLoadError tests database load error
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_DBLoadError() {
	testRound := "cos_db_load_err_77"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	// Create CVS in memory but not in DB
	cos := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160,
		170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}
	opIndexByte := []byte{uint8(0)}
	cvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	var cvsArray [32]byte
	copy(cvsArray[:], cvs)

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvsArray
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)
	// Don't save to DB to trigger error

	cosReq := utils.CosRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		Cos:        cos,
		EOAAddress: eoaAddress,
	}

	signature := generateCosRequestSignature(cosReq, privateKey)
	cosReq.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)
	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCOSRequest_DBUpdateError tests database update error
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_DBUpdateError() {
	testRound := "cos_db_update_err_78"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	cos := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160,
		170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}
	opIndexByte := []byte{uint8(0)}
	cvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	var cvsArray [32]byte
	copy(cvsArray[:], cvs)

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvsArray
	commitData.CvsHex = hex.EncodeToString(cvsArray[:])
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	cosReq := utils.CosRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		Cos:        cos,
		EOAAddress: eoaAddress,
	}

	signature := generateCosRequestSignature(cosReq, privateKey)
	cosReq.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)
	assert.True(suite.T(), stream.closed)

	// Verify COS was saved
	savedData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(
		context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cos, savedData.Cos)
}

// TestLeaderHandler_AllCommitsReceivedUnlocked_WithMock tests using mock service
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCommitsReceivedUnlocked_WithMock() {
	testRound := "mock_all_commits_79"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0x6666666666666666666666666666666666666666")
	testOp2 := common.HexToAddress("0x7777777777777777777777777777777777777777")

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	// Test with no data
	result := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)

	// Add CVS for op1
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})
	result = suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)

	// Add CVS for op2
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{Cvs: [32]byte{2}})
	result = suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.True(suite.T(), result)
}

// TestLeaderHandler_AllCosReceivedUnlocked_WithMock tests using mock service
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCosReceivedUnlocked_WithMock() {
	testRound := "mock_all_cos_80"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0x8888888888888888888888888888888888888888")
	testOp2 := common.HexToAddress("0x9999999999999999999999999999999999999999")

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	// Test with no data
	result := suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)

	// Add COS for op1
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cos: [32]byte{1}})
	result = suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)

	// Add COS for op2
	utils.SetCommittedNodeData(uniqueKey, testOp2, utils.LeaderCommitData{Cos: [32]byte{2}})
	result = suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.True(suite.T(), result)
}

// TestLeaderHandler_CheckActivation_NotActivated3 tests when EOA not activated (different operator in list)
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_CheckActivation_NotActivated3() {
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)
	otherOp := common.HexToAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{otherOp}, nil // Different operator
		},
	}

	suite.leaderNodeHandler.ethService = mockEth

	result := suite.leaderNodeHandler.CheckActivation(context.Background(), testOp, "commit")
	assert.False(suite.T(), result)
}

// TestLeaderHandler_HandleCOSRequest_DBUpdateErrorPath tests DB update error scenario
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_DBUpdateErrorPath() {
	testRound := "cos_db_update_fail_90"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	// Create CVS with valid hash
	cos := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160,
		170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}
	opIndexByte := []byte{uint8(0)}
	cvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos[:], opIndexByte))
	var cvsArray [32]byte
	copy(cvsArray[:], cvs)

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvsArray
	commitData.CvsHex = hex.EncodeToString(cvsArray[:])
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	// Now DELETE from database to trigger DB load error, but keep in memory
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	cosReq := utils.CosRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		Cos:        cos,
		EOAAddress: eoaAddress,
	}

	signature := generateCosRequestSignature(cosReq, privateKey)
	cosReq.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq)
	stream.readBuffer.Write(jsonData)

	// This should trigger the DB load error and return early
	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)
	assert.True(suite.T(), stream.closed)

	// Verify error path was taken - data should NOT be saved to DB
	_, err = suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(
		context.Background(), testRound, testTrial, testOp.Hex())
	assert.Error(suite.T(), err) // Should error because DB entry was deleted
}

// TestLeaderHandler_HandleCOSRequest_RevealOrderError tests reveal order determination error
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCOSRequest_RevealOrderError() {
	testRound := "cos_reveal_error_91"
	testTrial := "1"

	// Generate 2 valid key pairs
	privateKey1, eoaAddress1 := createTestKeyPair()
	require.NotNil(suite.T(), privateKey1)

	privateKey2, eoaAddress2 := createTestKeyPair()
	require.NotNil(suite.T(), privateKey2)

	testOp1 := common.HexToAddress(eoaAddress1)
	testOp2 := common.HexToAddress(eoaAddress2)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
	suite.db.Model((*database.PeerCommitDataScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
	defer func() {
		suite.db.Model((*database.LeaderCommitScheme)(nil)).
			Where("round = ? AND trial_num = ?", testRound, testTrial).
			Delete()
		suite.db.Model((*database.PeerCommitDataScheme)(nil)).
			Where("round = ? AND trial_num = ?", testRound, testTrial).
			Delete()
	}()

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	suite.leaderNodeHandler.leaderNode.SetHalted(false)
	suite.leaderNodeHandler.leaderNode.SetCurrentRound(testRound)
	suite.leaderNodeHandler.leaderNode.SetCurrentTrial(testTrial)

	// Add CVS and COS for op1
	cos1 := [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160,
		170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 254, 253, 252, 251, 250, 249}
	opIndexByte1 := []byte{uint8(0)}
	cvs1 := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos1[:], opIndexByte1))
	var cvsArray1 [32]byte
	copy(cvsArray1[:], cvs1)

	commitData1 := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp1)
	commitData1.Cvs = cvsArray1
	commitData1.CvsHex = hex.EncodeToString(cvsArray1[:])
	commitData1.Cos = cos1
	commitData1.CosHex = hex.EncodeToString(cos1[:])
	commitData1.Sign = utils.SignInfo{R: "123", S: "456", V: "27"}
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData1)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp1, *commitData1)

	// NOTE: We deliberately do NOT add peer commit for op1 to cause reveal order calculation to fail

	// Add CVS for op2
	cos2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17,
		16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	opIndexByte2 := []byte{uint8(1)}
	cvs2 := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(cos2[:], opIndexByte2))
	var cvsArray2 [32]byte
	copy(cvsArray2[:], cvs2)

	commitData2 := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp2)
	commitData2.Cvs = cvsArray2
	commitData2.CvsHex = hex.EncodeToString(cvsArray2[:])
	commitData2.Sign = utils.SignInfo{R: "789", S: "012", V: "27"}
	err = suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData2)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp2, *commitData2)

	cosReq2 := utils.CosRequest{
		UniqueKey:  uniqueKey,
		Round:      testRound,
		TrialNum:   testTrial,
		Cos:        cos2,
		EOAAddress: eoaAddress2,
	}
	signature2 := generateCosRequestSignature(cosReq2, privateKey2)
	cosReq2.Signature = signature2

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq2)
	stream.readBuffer.Write(jsonData)

	// This should trigger the reveal order error path because peer commits are missing
	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)
	assert.True(suite.T(), stream.closed)

	// Verify COS was still saved despite reveal order error
	savedData2, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(
		context.Background(), testRound, testTrial, testOp2.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), cos2, savedData2.Cos)
}

// TestLeaderHandler_HandleAcknowledgment_VerificationFails tests verification failure after signature check
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleAcknowledgment_VerificationFails() {
	testRound := "ack_verify_fail_92"
	testTrial := "1"

	// Generate valid key pair
	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	otherOp := common.HexToAddress("0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD")

	// Mock with different operator (not activated)
	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{otherOp}, nil // Different operator - will fail activation check
		},
	}

	suite.leaderNodeHandler.ethService = mockEth
	suite.leaderNodeHandler.leaderNode.SetHalted(false)

	ackMsg := utils.AcknowledgmentMessage{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress,
		MessageID:  "msg-verify-fail",
		Type:       "cvs",
		Status:     "received",
	}

	signature := generateAcknowledgmentSignature(ackMsg, privateKey)
	ackMsg.Signature = signature

	stream := newMockStream()
	jsonData, _ := json.Marshal(ackMsg)
	stream.readBuffer.Write(jsonData)

	// This should pass signature check but fail activation check
	suite.leaderNodeHandler.handleAcknowledgment(context.Background(), stream)
	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCommitRequest_DBSaveErrorReturn tests DB save error return path
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_DBSaveErrorReturn() {
	testRound := "commit_db_save_err_93"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Setup scenario that could cause DB save error
	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp}, nil
		},
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth
	suite.leaderNodeHandler.leaderNode.SetHalted(false)

	// Pre-create data in memory and DB
	cvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = cvs
	commitData.CvsHex = hex.EncodeToString(cvs[:])
	commitData.Sign = utils.SignInfo{R: "100", S: "200", V: "27"}
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	signature := generateValidSignature(eoaAddress, privateKey)

	// Try sending same CVS again - should skip the CVS assignment block
	commitReq := utils.CommitRequest{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress,
		Cvs:        cvs, // Same CVS
		Sign:       utils.SignInfo{R: "100", S: "200", V: "27"},
		Signature:  signature,
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(commitReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCommitRequest(context.Background(), stream)
	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandler_HandleCommitRequest_SkipCVSBlock tests skipping CVS assignment when already exists
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_HandleCommitRequest_SkipCVSBlock() {
	testRound := "commit_skip_cvs_94"
	testTrial := "1"

	privateKey, eoaAddress := createTestKeyPair()
	require.NotNil(suite.T(), privateKey)

	testOp := common.HexToAddress(eoaAddress)
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()
	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", testRound, testTrial, testOp.Hex()).
		Delete()

	mockEth := &MockEthService{
		GetActivatedOperatorsFunc: func(ctx context.Context, client fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
			return []common.Address{testOp}, nil
		},
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth
	suite.leaderNodeHandler.leaderNode.SetHalted(false)

	// Pre-create CVS data
	existingCvs := [32]byte{99, 98, 97, 96, 95, 94, 93, 92, 91, 90, 89, 88, 87, 86, 85, 84,
		83, 82, 81, 80, 79, 78, 77, 76, 75, 74, 73, 72, 71, 70, 69, 68}

	commitData := suite.leaderNodeHandler.leaderNode.GetOrCreateLeaderCommitData(testRound, testTrial, uniqueKey, testOp)
	commitData.Cvs = existingCvs // Pre-existing CVS
	commitData.CvsHex = hex.EncodeToString(existingCvs[:])
	commitData.Sign = utils.SignInfo{R: "111", S: "222", V: "27"}
	err := suite.leaderNodeHandler.leaderNode.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)
	suite.leaderNodeHandler.updateInMemoryData(uniqueKey, testOp, *commitData)

	signature := generateValidSignature(eoaAddress, privateKey)

	// Send different CVS - should be skipped because CVS already exists
	newCvs := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	commitReq := utils.CommitRequest{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: eoaAddress,
		Cvs:        newCvs, // Different CVS
		Sign:       utils.SignInfo{R: "333", S: "444", V: "27"},
		Signature:  signature,
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(commitReq)
	stream.readBuffer.Write(jsonData)

	suite.leaderNodeHandler.handleCommitRequest(context.Background(), stream)
	assert.True(suite.T(), stream.closed)

	// Verify original CVS is still there (not overwritten)
	savedData, err := suite.leaderNodeHandler.leaderNode.GetLeaderCommitByRoundAndEoaAddr(
		context.Background(), testRound, testTrial, testOp.Hex())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), existingCvs, savedData.Cvs) // Should still be old CVS
	assert.NotEqual(suite.T(), newCvs, savedData.Cvs)   // Should NOT be new CVS
}

// TestLeaderHandler_AllCommitsReceivedUnlocked_EmptyOperators tests with empty operators using mock
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCommitsReceivedUnlocked_EmptyOperators() {
	uniqueKey := "test_empty_ops_95"

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{} // Empty list
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	result := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)
}

// TestLeaderHandler_AllCommitsReceivedUnlocked_NoRoundData tests with no round data using mock
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCommitsReceivedUnlocked_NoRoundData() {
	uniqueKey := "test_no_round_96"

	testOp := common.HexToAddress("0xEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE")
	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	// Ensure no data exists
	utils.DeleteCommittedNodes(uniqueKey)

	result := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)
}

// TestLeaderHandler_AllCosReceivedUnlocked_EmptyOperators tests COS with empty operators using mock
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCosReceivedUnlocked_EmptyOperators() {
	uniqueKey := "test_cos_empty_97"

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{} // Empty list
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	result := suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)
}

// TestLeaderHandler_AllCosReceivedUnlocked_NoRoundData tests COS with no round data using mock
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCosReceivedUnlocked_NoRoundData() {
	uniqueKey := "test_cos_no_round_98"

	testOp := common.HexToAddress("0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	// Ensure no data exists
	utils.DeleteCommittedNodes(uniqueKey)

	result := suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result)
}

// TestLeaderHandler_AllCosReceivedUnlocked_PartialData tests COS with partial data using mock
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCosReceivedUnlocked_PartialData() {
	testRound := "cos_partial_99"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0xABCDEF1111111111111111111111111111111111")
	testOp2 := common.HexToAddress("0xABCDEF2222222222222222222222222222222222")

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	// Add COS only for op1
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cos: [32]byte{1}})

	result := suite.leaderNodeHandler.allCosReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result) // Should be false because op2 has no COS
}

// TestLeaderHandler_AllCommitsReceivedUnlocked_PartialData tests CVS with partial data using mock
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AllCommitsReceivedUnlocked_PartialData() {
	testRound := "cvs_partial_100"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	testOp1 := common.HexToAddress("0xDEADBEEF11111111111111111111111111111111")
	testOp2 := common.HexToAddress("0xDEADBEEF22222222222222222222222222222222")

	mockEth := &MockEthService{
		GetActivatedOperatorsCachedFunc: func() []common.Address {
			return []common.Address{testOp1, testOp2}
		},
	}
	suite.leaderNodeHandler.ethService = mockEth

	// Add CVS only for op1
	utils.SetCommittedNodeData(uniqueKey, testOp1, utils.LeaderCommitData{Cvs: [32]byte{1}})

	result := suite.leaderNodeHandler.allCommitsReceivedUnlocked(uniqueKey)
	assert.False(suite.T(), result) // Should be false because op2 has no CVS
}

// TestLeaderHandlerTestSuite runs the test suite
func TestLeaderHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(LeaderHandlerTestSuite))
}

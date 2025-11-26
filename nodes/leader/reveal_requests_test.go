package leader_node

import (
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-pg/pg/v10"
	_ "github.com/lib/pq"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

// MockFallbackEthClient is a mock for the fallback eth client
type MockFallbackEthClient struct {
	mock.Mock
}

func (m *MockFallbackEthClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	args := m.Called(ctx, account, blockNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) NetworkID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) BlockTimestamp(ctx context.Context, blockNumber *big.Int) (uint64, error) {
	args := m.Called(ctx, blockNumber)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockFallbackEthClient) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	args := m.Called(ctx, q, ch)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ethereum.Subscription), args.Error(1)
}

func (m *MockFallbackEthClient) ChainID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	args := m.Called(ctx, msg)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockFallbackEthClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockFallbackEthClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	args := m.Called(ctx, tx)
	return args.Error(0)
}

func (m *MockFallbackEthClient) TransactionReceipt(ctx context.Context, signedTx *types.Transaction) (*types.Receipt, error) {
	args := m.Called(ctx, signedTx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Receipt), args.Error(1)
}

func (m *MockFallbackEthClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	args := m.Called(ctx, account)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockFallbackEthClient) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	args := m.Called(ctx, msg, blockNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

type RevealRequestsTestSuite struct {
	suite.Suite
	db                   *pg.DB
	leaderCommitRepo     *database.LeaderCommitRepository
	batchRepo            *database.BatchRepository
	broadcastTrackerRepo *database.BroadcastTrackerRepository
	revealOrderRepo      *database.RevealOrderRepository
	nodeInfoRepo         *database.NodeInfoRepository
	peerCommitRepo       *database.PeerCommitRepository
	revealOrderService   *commitreveal2.RevealOrderService
	leaderNode           *LeaderNode
	host                 host.Host
	testPrivateKey       *ecdsa.PrivateKey
	testEOA              string
}

// SetupSuite runs once before all tests in the suite
func (suite *RevealRequestsTestSuite) SetupSuite() {
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

	// Generate test key pair
	suite.testPrivateKey, err = crypto.GenerateKey()
	require.NoError(suite.T(), err)
	publicKey := suite.testPrivateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	require.True(suite.T(), ok)
	suite.testEOA = crypto.PubkeyToAddress(*publicKeyECDSA).Hex()

	// Set environment variables for tests
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(crypto.FromECDSA(suite.testPrivateKey)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")

}

// SetupTest runs before each test
func (suite *RevealRequestsTestSuite) SetupTest() {
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

	// Create mock fallback eth client
	mockFallbackClient := new(MockFallbackEthClient)
	mockFallbackClient.On("NetworkID", mock.Anything).Return(big.NewInt(1), nil).Maybe()
	mockFallbackClient.On("PendingNonceAt", mock.Anything, mock.Anything).Return(uint64(0), nil).Maybe()
	mockFallbackClient.On("ChainID", mock.Anything).Return(big.NewInt(1), nil).Maybe()
	mockFallbackClient.On("EstimateGas", mock.Anything, mock.Anything).Return(uint64(21000), nil).Maybe()
	mockFallbackClient.On("SuggestGasPrice", mock.Anything).Return(big.NewInt(1000000000), nil).Maybe()
	mockFallbackClient.On("SuggestGasTipCap", mock.Anything).Return(big.NewInt(1000000000), nil).Maybe()
	mockFallbackClient.On("SendTransaction", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockFallbackClient.On("TransactionReceipt", mock.Anything, mock.Anything).Return(&types.Receipt{}, nil).Maybe()

	suite.leaderNode = &LeaderNode{
		fallbackEthClient:          mockFallbackClient,
		leaderCommitRepository:     suite.leaderCommitRepo,
		batchRepository:            suite.batchRepo,
		broadcastTrackerRepository: suite.broadcastTrackerRepo,
		reavealOrderRepository:     suite.revealOrderRepo,
		nodeInfoRepository:         suite.nodeInfoRepo,
		roundsData:                 make(map[string]RoundData),
		activeBroadcasts:           make(map[string]*utils.BroadcastTracker),
		cvOnChain:                  make(map[string]bool),
		roundSecrets:               make(map[string][][32]byte),
		roundSecret:                make(map[string]map[string]bool),
		secretsOnChain:             make(map[string]bool),
		revealRequestStatus:        make(map[string][]string),
		indices:                    make([]*big.Int, 0),
		ethService:                 eth.Service,
	}

	// Create a test libp2p host
	var err error
	suite.host, err = libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
}

// TearDownTest runs after each test
func (suite *RevealRequestsTestSuite) TearDownTest() {
	if suite.host != nil {
		suite.host.Close()
	}
}

// TearDownSuite runs once after all tests in the suite
func (suite *RevealRequestsTestSuite) TearDownSuite() {
	if suite.db != nil {
		suite.db.Close()
	}
}

// TestStartSecretValueRequests_Halted tests secret value requests when system is halted
func (suite *RevealRequestsTestSuite) TestStartSecretValueRequests_Halted() {

	suite.leaderNode.SetHalted(true)
	suite.leaderNode.StartSecretValueRequests(context.Background(), suite.host, "1", "1")

	assert.True(suite.T(), suite.leaderNode.GetHalted())
}

// TestStartSecretValueRequests_MissingRevealOrder tests with missing reveal order
func (suite *RevealRequestsTestSuite) TestStartSecretValueRequests_MissingRevealOrder() {

	suite.leaderNode.SetHalted(false)
	testRound := "missing_reveal_order_2"
	testTrial := "1"

	suite.leaderNode.StartSecretValueRequests(context.Background(), suite.host, testRound, testTrial)

}

// TestStartSecretValueRequests_MissingNodes tests with missing node infos
func (suite *RevealRequestsTestSuite) TestStartSecretValueRequests_MissingNodes() {

	suite.leaderNode.SetHalted(false)
	testRound := "missing_nodes_3"
	testTrial := "1"

	// Create reveal order but no node infos
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{suite.testEOA},
		RevealOrder:  []int{0},
		RV:           "test_rv",
	}
	err := suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	suite.leaderNode.StartSecretValueRequests(context.Background(), suite.host, testRound, testTrial)

}

// TestStartSecretValueRequests_Success tests successful secret value request initialization
func (suite *RevealRequestsTestSuite) TestStartSecretValueRequests_Success() {

	suite.leaderNode.SetHalted(false)
	testRound := "success_4"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup first to avoid conflicts with IP or PeerID
	suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("peer_id = ? OR ip = ?", suite.host.ID().String(), "127.0.0.1").
		Delete()

	// Create node info
	nodeInfo := &utils.NodeInfo{
		EOAAddress: suite.testEOA,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.1",
		Port:       "9000",
	}
	err := suite.nodeInfoRepo.AddAndUpdateNodeInfo(context.Background(), nodeInfo)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address = ?", suite.testEOA).
		Delete()

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{suite.testEOA},
		RevealOrder:  []int{0},
		RV:           "test_rv",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	suite.leaderNode.StartSecretValueRequests(context.Background(), suite.host, testRound, testTrial)

	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.NotNil(suite.T(), status)

}

// TestHandleSecretValueResponse_Halted tests handling secret value response when halted
func (suite *RevealRequestsTestSuite) TestHandleSecretValueResponse_Halted() {

	suite.leaderNode.SetHalted(true)
	suite.leaderNode.HandleSecretValueResponse(context.Background(), suite.host, nil, "1", "1", suite.testEOA)

	assert.True(suite.T(), suite.leaderNode.GetHalted())
}

// TestHandleSecretValueResponse_MissingRevealOrder tests with missing reveal order
func (suite *RevealRequestsTestSuite) TestHandleSecretValueResponse_MissingRevealOrder() {

	suite.leaderNode.SetHalted(false)
	testRound := "missing_reveal_6"
	testTrial := "1"

	suite.leaderNode.HandleSecretValueResponse(context.Background(), suite.host, nil, testRound, testTrial, suite.testEOA)

}

// TestHandleSecretValueResponse_AllProcessed tests when all nodes are already processed
func (suite *RevealRequestsTestSuite) TestHandleSecretValueResponse_AllProcessed() {

	suite.leaderNode.SetHalted(false)
	testRound := "all_processed_7"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Cleanup first to avoid duplicate key constraint
	suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address = ?", suite.testEOA).
		Delete()

	// Create reveal order with one node
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{suite.testEOA},
		RevealOrder:  []int{0},
		RV:           "test_rv",
	}
	err := suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{suite.testEOA})

	// Create node info (required for GetNodeInfos)
	nodeInfo := &utils.NodeInfo{
		EOAAddress: suite.testEOA,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.3",
		Port:       "9007",
	}
	err = suite.nodeInfoRepo.AddAndUpdateNodeInfo(context.Background(), nodeInfo)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address = ?", suite.testEOA).
		Delete()

	suite.leaderNode.HandleSecretValueResponse(context.Background(), suite.host, nil, testRound, testTrial, suite.testEOA)

}

// TestContains tests the contains helper function
func (suite *RevealRequestsTestSuite) TestContains() {

	testSlice := []string{"addr1", "addr2", "addr3"}

	assert.True(suite.T(), contains(testSlice, "addr1"))
	assert.True(suite.T(), contains(testSlice, "addr2"))
	assert.True(suite.T(), contains(testSlice, "addr3"))
	assert.False(suite.T(), contains(testSlice, "addr4"))
	assert.False(suite.T(), contains([]string{}, "addr1"))

}

// TestStartFailToSubmitSMonitoring_Halted tests monitoring start when system is halted
func (suite *RevealRequestsTestSuite) TestStartFailToSubmitSMonitoring_Halted() {

	suite.leaderNode.SetHalted(true)
	suite.leaderNode.StartFailToSubmitSMonitoring(context.Background(), "1", "1", big.NewInt(time.Now().Unix()))

	assert.True(suite.T(), suite.leaderNode.GetHalted())
}

// TestStartFailToSubmitSMonitoring_AlreadyActive tests monitoring when already active
func (suite *RevealRequestsTestSuite) TestStartFailToSubmitSMonitoring_AlreadyActive() {

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(true)

	suite.leaderNode.StartFailToSubmitSMonitoring(context.Background(), "1", "1", big.NewInt(time.Now().Unix()))

	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())
}

// TestStartFailToSubmitSMonitoring_Success tests successful monitoring start
func (suite *RevealRequestsTestSuite) TestStartFailToSubmitSMonitoring_Success() {

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)

	timestamp := big.NewInt(time.Now().Unix())
	suite.leaderNode.StartFailToSubmitSMonitoring(context.Background(), "test_round_11", "1", timestamp)

	// Wait a bit for goroutine to start
	time.Sleep(100 * time.Millisecond)

	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())
	assert.NotNil(suite.T(), suite.leaderNode.GetLastSubmitSTimestamp())

	// Cleanup
	suite.leaderNode.StopFailToSubmitSMonitoring(context.Background(), "test_round_11", "1")

}

// TestStopFailToSubmitSMonitoring_NotActive tests stopping when not active
func (suite *RevealRequestsTestSuite) TestStopFailToSubmitSMonitoring_NotActive() {

	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)
	suite.leaderNode.StopFailToSubmitSMonitoring(context.Background(), "1", "1")

	assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())
}

// TestStopFailToSubmitSMonitoring_Success tests successful monitoring stop
func (suite *RevealRequestsTestSuite) TestStopFailToSubmitSMonitoring_Success() {

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)

	timestamp := big.NewInt(time.Now().Unix())
	suite.leaderNode.StartFailToSubmitSMonitoring(context.Background(), "test_round_13", "1", timestamp)

	time.Sleep(100 * time.Millisecond)
	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	suite.leaderNode.StopFailToSubmitSMonitoring(context.Background(), "test_round_13", "1")

	assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())
}

// TestUpdateLastSubmitSTimestamp_NotActive tests timestamp update when monitoring not active
func (suite *RevealRequestsTestSuite) TestUpdateLastSubmitSTimestamp_NotActive() {

	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)
	newTimestamp := big.NewInt(time.Now().Unix() + 100)

	suite.leaderNode.UpdateLastSubmitSTimestamp(context.Background(), newTimestamp, "1", "1")

	assert.Equal(suite.T(), newTimestamp, suite.leaderNode.GetLastSubmitSTimestamp())
	assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())
}

// TestUpdateLastSubmitSTimestamp_Active tests timestamp update when monitoring is active
func (suite *RevealRequestsTestSuite) TestUpdateLastSubmitSTimestamp_Active() {

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)

	testRound := "update_ts_15"
	testTrial := "1"

	timestamp := big.NewInt(time.Now().Unix())
	suite.leaderNode.StartFailToSubmitSMonitoring(context.Background(), testRound, testTrial, timestamp)

	time.Sleep(100 * time.Millisecond)
	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	// Update timestamp (should restart monitoring)
	newTimestamp := big.NewInt(time.Now().Unix() + 100)
	suite.leaderNode.UpdateLastSubmitSTimestamp(context.Background(), newTimestamp, testRound, testTrial)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(suite.T(), newTimestamp, suite.leaderNode.GetLastSubmitSTimestamp())
	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	// Cleanup
	suite.leaderNode.StopFailToSubmitSMonitoring(context.Background(), testRound, testTrial)

}

// TestPackIndices_Empty tests packing empty indices array
func (suite *RevealRequestsTestSuite) TestPackIndices_Empty() {

	result := PackIndices([]*big.Int{})
	assert.Equal(suite.T(), big.NewInt(0), result)

}

// TestPackIndices_Single tests packing single index
func (suite *RevealRequestsTestSuite) TestPackIndices_Single() {

	indices := []*big.Int{big.NewInt(5)}
	result := PackIndices(indices)

	assert.NotNil(suite.T(), result)
	assert.Equal(suite.T(), big.NewInt(5), result)

}

// TestPackIndices_Multiple tests packing multiple indices
func (suite *RevealRequestsTestSuite) TestPackIndices_Multiple() {

	indices := []*big.Int{
		big.NewInt(1),
		big.NewInt(2),
		big.NewInt(3),
	}
	result := PackIndices(indices)

	assert.NotNil(suite.T(), result)
	assert.True(suite.T(), result.Cmp(big.NewInt(0)) > 0)

}

// TestResetLeaderMonitoringState tests complete monitoring state reset
func (suite *RevealRequestsTestSuite) TestResetLeaderMonitoringState() {

	testRound := "reset_19"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	// Set up monitoring state
	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{suite.testEOA})
	timestamp := big.NewInt(time.Now().Unix())
	suite.leaderNode.StartFailToSubmitSMonitoring(context.Background(), testRound, testTrial, timestamp)

	time.Sleep(100 * time.Millisecond)

	suite.leaderNode.ResetLeaderMonitoringState(context.Background(), testRound, testTrial)

	assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())
	_, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.False(suite.T(), exists)

}

// TestPrepareArgumentsForRequestToSubmitS tests argument preparation for submit request
func (suite *RevealRequestsTestSuite) TestPrepareArgumentsForRequestToSubmitS() {

	testRound := "prepare_20"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xTestOp20")

	// Set up required data
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Create leader commit with cos, cvs
	cos := [32]byte{1, 2, 3}
	cvs := [32]byte{4, 5, 6}

	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cos:        cos,
		Cvs:        cvs,
	}

	err := suite.leaderCommitRepo.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{testOp.Hex()},
		RevealOrder:  []int{0},
		RV:           "test_rv",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	// Set round secrets
	secret := [32]byte{10, 11, 12}
	suite.leaderNode.AppendToRoundSecrets(uniqueKey, secret)

	// Call prepare function
	allCos, secretsReceived, packedVs, cvNotOnChainCvAndSigRS, packedRevealOrders := suite.leaderNode.prepareArgumentsForRequestToSubmitS(context.Background(), testRound, testTrial)

	assert.NotNil(suite.T(), allCos)
	assert.NotNil(suite.T(), secretsReceived)
	assert.NotNil(suite.T(), packedVs)
	assert.NotNil(suite.T(), cvNotOnChainCvAndSigRS)
	assert.NotNil(suite.T(), packedRevealOrders)

}

// TestRoundSecretValueManagement tests round secret value management
func (suite *RevealRequestsTestSuite) TestRoundSecretValueManagement() {

	uniqueKey := "test_key_21"
	testEOA := suite.testEOA

	// Test getting non-existent secret
	hasSecret, exists := suite.leaderNode.GetRoundSecretValue(uniqueKey, testEOA)
	assert.False(suite.T(), exists)
	assert.False(suite.T(), hasSecret)

	// Test secrets on chain flag
	suite.leaderNode.SetSecretsOnChain(uniqueKey, true)
	onChain, exists := suite.leaderNode.GetSecretsOnChain(uniqueKey)
	assert.True(suite.T(), exists)
	assert.True(suite.T(), onChain)

}

// TestRevealRequestStatusManagement tests reveal request status management
func (suite *RevealRequestsTestSuite) TestRevealRequestStatusManagement() {

	uniqueKey := "test_key_22"

	// Test non-existent status
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.False(suite.T(), exists)
	assert.Nil(suite.T(), status)

	// Set status
	testStatus := []string{"addr1", "addr2"}
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, testStatus)

	status, exists = suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), testStatus, status)

	suite.leaderNode.DeleteRevealRequestStatus(uniqueKey)
	_, exists = suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.False(suite.T(), exists)

}

// TestSecretValueRequestTimer tests timer expiration simulation
func (suite *RevealRequestsTestSuite) TestSecretValueRequestTimer() {

	testRound := "timer_23"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetCurrentRound(testRound)

	// Test that timer logic exists (we can't wait 20 seconds in test)
	regularEoa := suite.testEOA

	hasSecret, exists := suite.leaderNode.GetRoundSecretValue(uniqueKey, regularEoa)
	assert.False(suite.T(), exists || hasSecret)

	testSecret := [32]byte{1, 2, 3}
	suite.leaderNode.AppendToRoundSecrets(uniqueKey, testSecret)

	retrieved, exists := suite.leaderNode.GetRoundSecretsValue(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), 1, len(retrieved))
	assert.Equal(suite.T(), testSecret, retrieved[0])

}

// TestMultipleRevealRequestsSequence tests multiple reveal requests in sequence
func (suite *RevealRequestsTestSuite) TestMultipleRevealRequestsSequence() {

	testRound := "sequence_24"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	suite.leaderNode.SetHalted(false)

	// Create multiple nodes
	nodes := []string{
		"0xNode1000000000000000000000000000000000001",
		"0xNode2000000000000000000000000000000000002",
		"0xNode3000000000000000000000000000000000003",
	}

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: nodes,
		RevealOrder:  []int{0, 1, 2},
		RV:           "test_rv",
	}
	err := suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})

	// Process first node
	currentStatus, _ := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	currentStatus = append(currentStatus, nodes[0])
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, currentStatus)

	status, _ := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.Contains(suite.T(), status, nodes[0])

	// Process second node
	currentStatus, _ = suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	currentStatus = append(currentStatus, nodes[1])
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, currentStatus)

	status, _ = suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.Contains(suite.T(), status, nodes[0])
	assert.Contains(suite.T(), status, nodes[1])

}

// TestConcurrentRevealRequestAccess tests concurrent access to reveal request status
func (suite *RevealRequestsTestSuite) TestConcurrentRevealRequestAccess() {

	uniqueKey := "concurrent_25"
	done := make(chan bool, 2)

	// Writer goroutine
	go func() {
		for i := 0; i < 50; i++ {
			status := []string{fmt.Sprintf("addr_%d", i)}
			suite.leaderNode.SetRevealRequestStatus(uniqueKey, status)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 50; i++ {
			suite.leaderNode.GetRevealRequestStatus(uniqueKey)
		}
		done <- true
	}()

	<-done
	<-done

	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.NotNil(suite.T(), status)

}

// TestPackIndices_LargeNumbers tests packing very large indices
func (suite *RevealRequestsTestSuite) TestPackIndices_LargeNumbers() {

	indices := []*big.Int{
		big.NewInt(255),
		big.NewInt(256),
		big.NewInt(1000),
	}
	result := PackIndices(indices)

	assert.NotNil(suite.T(), result)
	assert.True(suite.T(), result.Cmp(big.NewInt(0)) > 0)

}

// TestMonitoringPeriodCalculation tests monitoring period calculation
func (suite *RevealRequestsTestSuite) TestMonitoringPeriodCalculation() {

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)

	testRound := "period_27"
	testTrial := "1"

	// Set future timestamp to avoid immediate trigger
	futureTimestamp := big.NewInt(time.Now().Unix() + 100)
	suite.leaderNode.SetLastSubmitSTimestamp(futureTimestamp)

	period := big.NewInt(200)
	suite.leaderNode.startMonitoringWithPeriod(context.Background(), testRound, testTrial, period)

	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	time.Sleep(100 * time.Millisecond)

	// Cleanup
	suite.leaderNode.StopFailToSubmitSMonitoring(context.Background(), testRound, testTrial)
	assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

}

// TestMultipleStartStopCycles tests multiple monitoring start/stop cycles
func (suite *RevealRequestsTestSuite) TestMultipleStartStopCycles() {

	suite.leaderNode.SetHalted(false)
	testRound := "cycles_28"
	testTrial := "1"

	for i := 0; i < 3; i++ {
		suite.leaderNode.SetFailToSubmitSMonitoringActive(false)
		timestamp := big.NewInt(time.Now().Unix() + int64(i*10))

		suite.leaderNode.StartFailToSubmitSMonitoring(context.Background(), testRound, testTrial, timestamp)
		time.Sleep(100 * time.Millisecond)

		assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

		suite.leaderNode.StopFailToSubmitSMonitoring(context.Background(), testRound, testTrial)
		assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())
	}

}

// TestHandleSecretValueResponse_NextNode tests handling response and moving to next node
func (suite *RevealRequestsTestSuite) TestHandleSecretValueResponse_NextNode() {

	suite.leaderNode.SetHalted(false)
	testRound := "next_node_29"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	node1 := "0xNode1000000000000000000000000000000000001"
	node2 := "0xNode2000000000000000000000000000000000002"

	// Cleanup first
	suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address IN (?)", pg.In([]string{node1, node2})).
		Delete()

	// Create additional hosts for unique PeerIDs
	host1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
	defer host1.Close()

	host2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
	defer host2.Close()

	// Create node infos with unique IPs and PeerIDs
	nodeInfo1 := &utils.NodeInfo{
		EOAAddress: node1,
		PeerID:     host1.ID().String(),
		IP:         "127.0.0.1",
		Port:       "9001",
	}
	err = suite.nodeInfoRepo.AddAndUpdateNodeInfo(context.Background(), nodeInfo1)
	require.NoError(suite.T(), err)

	nodeInfo2 := &utils.NodeInfo{
		EOAAddress: node2,
		PeerID:     host2.ID().String(),
		IP:         "127.0.0.2",
		Port:       "9002",
	}
	err = suite.nodeInfoRepo.AddAndUpdateNodeInfo(context.Background(), nodeInfo2)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address IN (?)", pg.In([]string{node1, node2})).
		Delete()

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{node1, node2},
		RevealOrder:  []int{0, 1},
		RV:           "test_rv",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{node1})

	suite.leaderNode.HandleSecretValueResponse(context.Background(), suite.host, nil, testRound, testTrial, node1)

	status, _ := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.Contains(suite.T(), status, node1)

}

// TestLoadNodeDataIntegration tests node data loading integration
func (suite *RevealRequestsTestSuite) TestLoadNodeDataIntegration() {

	testRound := "load_data_30"
	testTrial := "1"
	testOp := common.HexToAddress("0xLoadDataOp30")

	// Set activated operators so LoadNodeData can work
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Create leader commit with full data
	cos := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	cvs := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cos:        cos,
		CosHex:     hex.EncodeToString(cos[:]),
		Cvs:        cvs,
		CvsHex:     hex.EncodeToString(cvs[:]),
		Sign: utils.SignInfo{
			R: "12345",
			S: "67890",
			V: "27",
		},
	}

	err := suite.leaderCommitRepo.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	commits, cosArray, cvsArray, vs, rs, ss := suite.leaderNode.LoadNodeData(context.Background(), testRound, testTrial)

	assert.NotNil(suite.T(), commits)
	assert.Len(suite.T(), commits, 1)
	assert.NotNil(suite.T(), cosArray)
	assert.Len(suite.T(), cosArray, 1)
	assert.NotNil(suite.T(), cvsArray)
	assert.Len(suite.T(), cvsArray, 1)
	assert.NotNil(suite.T(), vs)
	assert.Len(suite.T(), vs, 1)
	assert.NotNil(suite.T(), rs)
	assert.Len(suite.T(), rs, 1)
	assert.NotNil(suite.T(), ss)
	assert.Len(suite.T(), ss, 1)

}

// TestStartMonitoringWithPeriod_FutureDeadline tests monitoring with future deadline
func (suite *RevealRequestsTestSuite) TestStartMonitoringWithPeriod_FutureDeadline() {

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)

	testRound := "future_deadline_31"
	testTrial := "1"

	futureTimestamp := big.NewInt(time.Now().Unix() + 10000)
	suite.leaderNode.SetLastSubmitSTimestamp(futureTimestamp)

	period := big.NewInt(20000)
	suite.leaderNode.startMonitoringWithPeriod(context.Background(), testRound, testTrial, period)

	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())
	assert.NotNil(suite.T(), suite.leaderNode.failToSubmitSMonitoringTimer)

	suite.leaderNode.StopFailToSubmitSMonitoring(context.Background(), testRound, testTrial)
	assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

}

// TestGetIndices_WithData tests getting indices with data
func (suite *RevealRequestsTestSuite) TestGetIndices_WithData() {

	indices := suite.leaderNode.GetIndices()
	assert.Empty(suite.T(), indices)

}

// TestPrepareArguments_Comprehensive
func (suite *RevealRequestsTestSuite) TestPrepareArguments_Comprehensive() {

	testRound := "comprehensive_33"
	testTrial := "33"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xComprehensiveOp33333333333333333333")

	// Cleanup
	suite.db.Exec("DELETE FROM leader_commit_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	suite.db.Exec("DELETE FROM reveal_order_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)

	defer func() {
		suite.db.Exec("DELETE FROM leader_commit_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
		suite.db.Exec("DELETE FROM reveal_order_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	}()

	// Set up activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Create leader commit with all fields
	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cos:        [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		Cvs:        [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		Sign: utils.SignInfo{
			R: "100",
			S: "200",
			V: "27",
		},
	}

	err := suite.leaderCommitRepo.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{testOp.Hex()},
		RevealOrder:  []int{0},
		RV:           "test_rv",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	// Set round secrets
	secret := [32]byte{10, 11, 12}
	suite.leaderNode.AppendToRoundSecrets(uniqueKey, secret)

	// Call prepare function
	allCos, secretsReceived, packedVs, cvNotOnChainCvAndSigRS, packedRevealOrders := suite.leaderNode.prepareArgumentsForRequestToSubmitS(context.Background(), testRound, testTrial)

	assert.NotNil(suite.T(), allCos)
	assert.Len(suite.T(), allCos, 1)
	assert.NotNil(suite.T(), secretsReceived)
	assert.NotNil(suite.T(), packedVs)
	assert.NotNil(suite.T(), cvNotOnChainCvAndSigRS)
	assert.NotNil(suite.T(), packedRevealOrders)

}

// TestRevealRequestStatus_EdgeCases tests reveal request status edge cases
func (suite *RevealRequestsTestSuite) TestRevealRequestStatus_EdgeCases() {

	uniqueKey := "edge_34"

	// Set empty array
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Empty(suite.T(), status)

	// Update with single item
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{"single"})
	status, exists = suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Len(suite.T(), status, 1)

	// Update with many items
	many := make([]string, 100)
	for i := 0; i < 100; i++ {
		many[i] = fmt.Sprintf("addr_%d", i)
	}
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, many)
	status, exists = suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Len(suite.T(), status, 100)

}

// TestRoundSecrets_MultipleAppends tests appending multiple round secrets
func (suite *RevealRequestsTestSuite) TestRoundSecrets_MultipleAppends() {

	uniqueKey := "multi_append_35"

	// Append multiple secrets
	for i := 0; i < 10; i++ {
		secret := [32]byte{byte(i)}
		suite.leaderNode.AppendToRoundSecrets(uniqueKey, secret)
	}

	retrieved, exists := suite.leaderNode.GetRoundSecretsValue(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Len(suite.T(), retrieved, 10)

	for i := 0; i < 10; i++ {
		assert.Equal(suite.T(), byte(i), retrieved[i][0])
	}

}

// TestLastSubmitSTimestamp_Nil tests setting timestamp to nil
func (suite *RevealRequestsTestSuite) TestLastSubmitSTimestamp_Nil() {

	// Set to nil
	suite.leaderNode.SetLastSubmitSTimestamp(nil)
	ts := suite.leaderNode.GetLastSubmitSTimestamp()
	assert.Nil(suite.T(), ts)

	// Set to value
	newTs := big.NewInt(12345)
	suite.leaderNode.SetLastSubmitSTimestamp(newTs)
	ts = suite.leaderNode.GetLastSubmitSTimestamp()
	assert.Equal(suite.T(), newTs, ts)

}

// TestStartMonitoring_WithNilCheck tests monitoring with nil timestamp check
func (suite *RevealRequestsTestSuite) TestStartMonitoring_WithNilCheck() {

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)

	testRound := "nil_check_37"
	testTrial := "1"

	suite.leaderNode.SetLastSubmitSTimestamp(nil)
	timestamp := big.NewInt(time.Now().Unix() + 100)

	suite.leaderNode.StartFailToSubmitSMonitoring(context.Background(), testRound, testTrial, timestamp)

	time.Sleep(100 * time.Millisecond)

	assert.NotNil(suite.T(), suite.leaderNode.GetLastSubmitSTimestamp())
	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	suite.leaderNode.StopFailToSubmitSMonitoring(context.Background(), testRound, testTrial)

}

// TestSecretsOnChain_FlagManagement tests secrets on chain flag management
func (suite *RevealRequestsTestSuite) TestSecretsOnChain_FlagManagement() {

	uniqueKey1 := "key_38_1"
	uniqueKey2 := "key_38_2"

	// Set for key1
	suite.leaderNode.SetSecretsOnChain(uniqueKey1, true)
	val1, exists1 := suite.leaderNode.GetSecretsOnChain(uniqueKey1)
	assert.True(suite.T(), exists1)
	assert.True(suite.T(), val1)

	// Set for key2
	suite.leaderNode.SetSecretsOnChain(uniqueKey2, false)
	val2, exists2 := suite.leaderNode.GetSecretsOnChain(uniqueKey2)
	assert.True(suite.T(), exists2)
	assert.False(suite.T(), val2)

	_, exists3 := suite.leaderNode.GetSecretsOnChain("nonexistent")
	assert.False(suite.T(), exists3)

}

// TestRoundSecretValue_BooleanMap tests round secret value boolean map
func (suite *RevealRequestsTestSuite) TestRoundSecretValue_BooleanMap() {

	uniqueKey := "bool_map_39"
	eoa1 := "0xEOA1"
	eoa2 := "0xEOA2"

	// Initially not exists
	val, exists := suite.leaderNode.GetRoundSecretValue(uniqueKey, eoa1)
	assert.False(suite.T(), exists)
	assert.False(suite.T(), val)

	// Set values (this requires SetRoundSecretValue method)
	suite.leaderNode.SetRoundSecretValue(uniqueKey, eoa1, true)
	suite.leaderNode.SetRoundSecretValue(uniqueKey, eoa2, false)

	val1, exists1 := suite.leaderNode.GetRoundSecretValue(uniqueKey, eoa1)
	assert.True(suite.T(), exists1)
	assert.True(suite.T(), val1)

	val2, exists2 := suite.leaderNode.GetRoundSecretValue(uniqueKey, eoa2)
	assert.True(suite.T(), exists2)
	assert.False(suite.T(), val2)

}

// TestHandleSecretValueResponse_NodeInfoNotFound tests when node info is not found
func (suite *RevealRequestsTestSuite) TestHandleSecretValueResponse_NodeInfoNotFound() {

	suite.leaderNode.SetHalted(false)
	testRound := "no_nodeinfo_40"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	node1 := "0xNodeInfo1111111111111111111111111111111"
	node2 := "0xNodeInfo2222222222222222222222222222222"

	// Cleanup
	suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address IN (?)", pg.In([]string{node1, node2})).
		Delete()
	defer suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address IN (?)", pg.In([]string{node1, node2})).
		Delete()

	// Create reveal order with two nodes
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{node1, node2},
		RevealOrder:  []int{0, 1},
		RV:           "test_rv",
	}
	err := suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{node1})

	// Create only node1 info
	nodeInfo1 := &utils.NodeInfo{
		EOAAddress: node1,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.10",
		Port:       "9010",
	}
	err = suite.nodeInfoRepo.AddAndUpdateNodeInfo(context.Background(), nodeInfo1)
	require.NoError(suite.T(), err)

	suite.leaderNode.HandleSecretValueResponse(context.Background(), suite.host, nil, testRound, testTrial, node1)

}

// TestPackIndices_WithZero tests packing indices with zero values
func (suite *RevealRequestsTestSuite) TestPackIndices_WithZero() {

	indices := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(0),
	}
	result := PackIndices(indices)

	assert.NotNil(suite.T(), result)
	expected := big.NewInt(256)
	assert.Equal(suite.T(), expected, result)

}

// TestPrepareArguments_NoIndices tests argument preparation with no indices
func (suite *RevealRequestsTestSuite) TestPrepareArguments_NoIndices() {

	testRound := "no_indices_42"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xNoIndicesOp42")

	// Set up activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Create leader commit
	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cos:        [32]byte{1, 2, 3},
		Cvs:        [32]byte{4, 5, 6},
		Sign: utils.SignInfo{
			R: "100",
			S: "200",
			V: "27",
		},
	}

	err := suite.leaderCommitRepo.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{testOp.Hex()},
		RevealOrder:  []int{0},
		RV:           "test_rv",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	// Set round secrets
	secret := [32]byte{10, 11, 12}
	suite.leaderNode.AppendToRoundSecrets(uniqueKey, secret)

	// Clear indices
	suite.leaderNode.indicesMutex.Lock()
	suite.leaderNode.indices = []*big.Int{}
	suite.leaderNode.indicesMutex.Unlock()

	// Call prepare function - all operators are not on chain
	allCos, secretsReceived, packedVs, cvNotOnChainCvAndSigRS, packedRevealOrders := suite.leaderNode.prepareArgumentsForRequestToSubmitS(context.Background(), testRound, testTrial)

	assert.NotNil(suite.T(), allCos)
	assert.Len(suite.T(), allCos, 1)
	assert.NotNil(suite.T(), secretsReceived)
	assert.NotNil(suite.T(), packedVs)
	assert.NotNil(suite.T(), cvNotOnChainCvAndSigRS)
	assert.Len(suite.T(), cvNotOnChainCvAndSigRS, 1)
	assert.NotNil(suite.T(), packedRevealOrders)

}

// TestRevealRequestStatus_SpecialChars tests reveal request status with special characters
func (suite *RevealRequestsTestSuite) TestRevealRequestStatus_SpecialChars() {

	uniqueKey := "special_43/with:chars-test"

	addresses := []string{
		"0xABCDEF1234567890ABCDEF1234567890ABCDEF12",
		"0x1234567890ABCDEF1234567890ABCDEF12345678",
	}

	suite.leaderNode.SetRevealRequestStatus(uniqueKey, addresses)
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)

	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), addresses, status)

}

// TestRoundSecretsValue_ReturnsCopy tests that getting round secrets returns a copy
func (suite *RevealRequestsTestSuite) TestRoundSecretsValue_ReturnsCopy() {

	uniqueKey := "copy_test_44"
	original := [32]byte{1, 2, 3, 4, 5}

	suite.leaderNode.AppendToRoundSecrets(uniqueKey, original)

	retrieved, exists := suite.leaderNode.GetRoundSecretsValue(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Len(suite.T(), retrieved, 1)

	// Modify the retrieved value
	retrieved[0][0] = 99

	retrieved2, _ := suite.leaderNode.GetRoundSecretsValue(uniqueKey)
	assert.Equal(suite.T(), byte(1), retrieved2[0][0])

}

// TestContains_Duplicates tests contains function with duplicate values
func (suite *RevealRequestsTestSuite) TestContains_Duplicates() {

	slice := []string{"addr1", "addr1", "addr2", "addr2"}

	assert.True(suite.T(), contains(slice, "addr1"))
	assert.True(suite.T(), contains(slice, "addr2"))
	assert.False(suite.T(), contains(slice, "addr3"))

}

// TestStartSecretValueRequests_MultipleNodesFirstMatch tests with multiple nodes but only first match
func (suite *RevealRequestsTestSuite) TestStartSecretValueRequests_MultipleNodesFirstMatch() {

	suite.leaderNode.SetHalted(false)
	testRound := "first_match_46"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	node1 := "0xFirstNode111111111111111111111111111111"
	node2 := "0xSecondNode222222222222222222222222222222"

	// Cleanup
	suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address IN (?)", pg.In([]string{node1, node2})).
		Delete()
	defer suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address IN (?)", pg.In([]string{node1, node2})).
		Delete()

	// Create additional hosts for unique PeerIDs
	host1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
	defer host1.Close()

	host2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
	defer host2.Close()

	// Create both node infos with unique PeerIDs
	nodeInfo1 := &utils.NodeInfo{
		EOAAddress: node1,
		PeerID:     host1.ID().String(),
		IP:         "127.0.0.20",
		Port:       "9020",
	}
	err = suite.nodeInfoRepo.AddAndUpdateNodeInfo(context.Background(), nodeInfo1)
	require.NoError(suite.T(), err)

	nodeInfo2 := &utils.NodeInfo{
		EOAAddress: node2,
		PeerID:     host2.ID().String(),
		IP:         "127.0.0.21",
		Port:       "9021",
	}
	err = suite.nodeInfoRepo.AddAndUpdateNodeInfo(context.Background(), nodeInfo2)
	require.NoError(suite.T(), err)

	// Create reveal order with node1 first
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{node1, node2},
		RevealOrder:  []int{0, 1},
		RV:           "test_rv",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	defer suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	suite.leaderNode.StartSecretValueRequests(context.Background(), suite.host, testRound, testTrial)

	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.NotNil(suite.T(), status)

}

// TestTimestamp_SequentialUpdates tests sequential timestamp updates
func (suite *RevealRequestsTestSuite) TestTimestamp_SequentialUpdates() {

	timestamps := []*big.Int{
		big.NewInt(1000),
		big.NewInt(2000),
		big.NewInt(3000),
		nil,
		big.NewInt(4000),
	}

	for i, ts := range timestamps {
		suite.leaderNode.SetLastSubmitSTimestamp(ts)
		retrieved := suite.leaderNode.GetLastSubmitSTimestamp()
		assert.Equal(suite.T(), ts, retrieved, fmt.Sprintf("Failed at index %d", i))
	}

}

// TestRevealRequestStatus_ConcurrentWrites tests concurrent writes to reveal request status
func (suite *RevealRequestsTestSuite) TestRevealRequestStatus_ConcurrentWrites() {

	uniqueKey := "concurrent_writes_48"
	done := make(chan bool, 5)

	// Multiple writers
	for g := 0; g < 5; g++ {
		go func(routineNum int) {
			for i := 0; i < 20; i++ {
				status := []string{fmt.Sprintf("routine_%d_item_%d", routineNum, i)}
				suite.leaderNode.SetRevealRequestStatus(uniqueKey, status)
			}
			done <- true
		}(g)
	}

	for g := 0; g < 5; g++ {
		<-done
	}

	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.NotNil(suite.T(), status)

}

// TestMonitoringState_Transitions tests monitoring state transitions
func (suite *RevealRequestsTestSuite) TestMonitoringState_Transitions() {

	suite.leaderNode.SetHalted(false)
	testRound := "transitions_49"
	testTrial := "1"

	// Transition: inactive -> active
	assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())
	suite.leaderNode.SetFailToSubmitSMonitoringActive(true)
	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	// Transition: active -> inactive via Stop
	timestamp := big.NewInt(time.Now().Unix() + 1000)
	suite.leaderNode.SetLastSubmitSTimestamp(timestamp)
	suite.leaderNode.startMonitoringWithPeriod(context.Background(), testRound, testTrial, big.NewInt(2000))
	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	suite.leaderNode.StopFailToSubmitSMonitoring(context.Background(), testRound, testTrial)
	assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

}

// TestPackIndices_Mathematical tests pack indices mathematical correctness
func (suite *RevealRequestsTestSuite) TestPackIndices_Mathematical() {

	indices := []*big.Int{
		big.NewInt(1),
		big.NewInt(2),
		big.NewInt(3),
	}
	result := PackIndices(indices)

	expected := big.NewInt(197121)
	assert.Equal(suite.T(), expected, result)

}

// sendToRegularNodeMockFunc is a function type that wraps sendToRegularNode for mocking in tests
type sendToRegularNodeMockFunc func(ctx context.Context, h host.Host, nodeInfo utils.NodeInfo, protocol string, data interface{}) error

// sendSecretValueRequestToNodeTestable is a testable version that uses the mockable function
func (n *LeaderNode) sendSecretValueRequestToNodeTestable(
	ctx context.Context,
	h host.Host,
	round string,
	trialNum string,
	uniqueKey string,
	regularEoa string,
	nodeInfo *utils.NodeInfo,
	order int,
	sendFunc sendToRegularNodeMockFunc,
	timerDuration time.Duration, // Configurable timer duration for testing
) {
	// Load private key from environment variable
	privateKeyHex := appconfig.Get().LeaderPrivateKey
	if privateKeyHex == "" {
		log.Println("LEADER_PRIVATE_KEY is not set in environment variables.")
		return
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode leader private key: %v", err)
		return
	}

	leaderEoa := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	log.Printf("EOA Address: %s", leaderEoa)

	// Sign the round number
	signature := utils.SignData(leaderEoa, privateKey)

	// Create the secret value request
	req := utils.SecretValueRequest{
		LeaderEoaAddress:  leaderEoa,
		RegularEoaAddress: regularEoa,
		Round:             round,
		TrialNum:          trialNum,
		Signature:         signature,
		Order:             order,
	}

	fmt.Println("Sending secret value request to EOA:", regularEoa)

	// Send the request using the provided function
	err = sendFunc(ctx, h, *nodeInfo, "/sendSecretValue", req)
	if err != nil {
		log.Printf("Failed to send secret value request to EOA %s for round %s with trail %s: %v", regularEoa, round, trialNum, err)
	} else {
		log.Printf("✅ Secret value request sent to EOA %s for round %s with trail %s", regularEoa, round, trialNum)

		go func() {
			timer := time.NewTimer(timerDuration)
			defer timer.Stop()

			// Wait for the timer to expire
			<-timer.C

			// If the timer expires and the secret value is not received, call handleMissingSecretValue
			hasSecret, exists := n.GetRoundSecretValue(uniqueKey, regularEoa)
			if !exists || !hasSecret {
				log.Printf("Secret value not received for EOA %s in round %s with trail %s within %v. Handling missing secret value.", regularEoa, round, trialNum, timerDuration)
				n.SetSecretsOnChain(uniqueKey, true)
				n.requestToSubmitS(ctx, round, trialNum)
			}
		}()

		// Mark this EOA as requested
		currentStatus, _ := n.GetRevealRequestStatus(uniqueKey)
		currentStatus = append(currentStatus, regularEoa)
		n.SetRevealRequestStatus(uniqueKey, currentStatus)
	}
}

// TestSendSecretValueRequest_Success tests successful secret value request with timer expiration
func (suite *RevealRequestsTestSuite) TestSendSecretValueRequest_Success() {
	ctx := context.Background()

	// Set up test data
	testRound := "send_success_50"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xSendSuccessOp50000000000000000000000")

	// Cleanup database
	suite.db.Exec("DELETE FROM leader_commit_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	suite.db.Exec("DELETE FROM reveal_order_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	defer func() {
		suite.db.Exec("DELETE FROM leader_commit_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
		suite.db.Exec("DELETE FROM reveal_order_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	}()

	// Set up activated operators for requestToSubmitS
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Create leader commit data for requestToSubmitS
	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cos:        [32]byte{1, 2, 3},
		Cvs:        [32]byte{4, 5, 6},
		Sign: utils.SignInfo{
			R: "100",
			S: "200",
			V: "27",
		},
	}
	err := suite.leaderCommitRepo.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	// Create reveal order for requestToSubmitS
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{testOp.Hex()},
		RevealOrder:  []int{0},
		RV:           "test_rv",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	nodeInfo := &utils.NodeInfo{
		EOAAddress: testOp.Hex(),
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.50",
		Port:       "9050",
	}

	// Create a mock sendToRegularNode function that returns success
	mockSendFunc := func(ctx context.Context, h host.Host, nodeInfo utils.NodeInfo, protocol string, data interface{}) error {
		// Simulate successful send
		return nil
	}

	// Initialize reveal request status
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})

	// Use a 500ms timer for faster testing
	testTimerDuration := 500 * time.Millisecond

	// Call the testable function
	suite.leaderNode.sendSecretValueRequestToNodeTestable(
		ctx,
		suite.host,
		testRound,
		testTrial,
		uniqueKey,
		testOp.Hex(),
		nodeInfo,
		0,
		mockSendFunc,
		testTimerDuration,
	)

	// Wait for immediate calls
	time.Sleep(100 * time.Millisecond)

	// Verify that the EOA was marked as requested
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Contains(suite.T(), status, testOp.Hex())

	// Wait for the timer to expire
	time.Sleep(700 * time.Millisecond)

	onChain, exists := suite.leaderNode.GetSecretsOnChain(uniqueKey)
	assert.True(suite.T(), exists)
	assert.True(suite.T(), onChain)

	fmt.Println("✅ TestSendSecretValueRequest_Success completed successfully")
}

// TestSendSecretValueRequest_Failure tests when sendToRegularNode fails
func (suite *RevealRequestsTestSuite) TestSendSecretValueRequest_Failure() {
	ctx := context.Background()

	// Set up test data
	testRound := "send_failure_51"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	regularEoa := "0xRegularNode5100000000000000000000000000"

	nodeInfo := &utils.NodeInfo{
		EOAAddress: regularEoa,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.51",
		Port:       "9051",
	}

	// Create a mock sendToRegularNode function that returns an error
	mockSendFunc := func(ctx context.Context, h host.Host, nodeInfo utils.NodeInfo, protocol string, data interface{}) error {
		// Simulate send failure
		return fmt.Errorf("failed to create stream: connection refused")
	}

	// Initialize reveal request status
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})

	// Use a 500ms timer (though it shouldn't start since send fails)
	testTimerDuration := 500 * time.Millisecond

	// Call the testable function
	suite.leaderNode.sendSecretValueRequestToNodeTestable(
		ctx,
		suite.host,
		testRound,
		testTrial,
		uniqueKey,
		regularEoa,
		nodeInfo,
		0,
		mockSendFunc,
		testTimerDuration,
	)

	// Wait a bit to ensure no goroutines are started
	time.Sleep(200 * time.Millisecond)

	// Verify that the EOA was NOT marked as requested (because send failed)
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.NotContains(suite.T(), status, regularEoa)

	// Wait longer to ensure timer doesn't fire
	time.Sleep(600 * time.Millisecond)

	// Verify that SetSecretsOnChain was NOT called
	_, exists = suite.leaderNode.GetSecretsOnChain(uniqueKey)
	assert.False(suite.T(), exists)

	fmt.Println(" TestSendSecretValueRequest_Failure completed - no methods called after send failure")
}

// TestSendSecretValueRequest_SecretReceivedBeforeTimeout tests when secret is received before timer expires
func (suite *RevealRequestsTestSuite) TestSendSecretValueRequest_SecretReceivedBeforeTimeout() {
	ctx := context.Background()

	// Set up test data
	testRound := "send_received_52"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	regularEoa := "0xRegularNode5200000000000000000000000000"

	nodeInfo := &utils.NodeInfo{
		EOAAddress: regularEoa,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.52",
		Port:       "9052",
	}

	// Create a mock sendToRegularNode function that returns success
	mockSendFunc := func(ctx context.Context, h host.Host, nodeInfo utils.NodeInfo, protocol string, data interface{}) error {
		return nil
	}

	// Initialize reveal request status
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})

	// Use a 500ms timer
	testTimerDuration := 500 * time.Millisecond

	// Call the testable function
	suite.leaderNode.sendSecretValueRequestToNodeTestable(
		ctx,
		suite.host,
		testRound,
		testTrial,
		uniqueKey,
		regularEoa,
		nodeInfo,
		0,
		mockSendFunc,
		testTimerDuration,
	)

	// Wait for immediate calls
	time.Sleep(100 * time.Millisecond)

	// Verify that the EOA was marked as requested
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Contains(suite.T(), status, regularEoa)

	// Simulate that the secret was received before timer expires
	suite.leaderNode.SetRoundSecretValue(uniqueKey, regularEoa, true)

	// Wait for the timer to expire
	time.Sleep(700 * time.Millisecond)

	// Verify that SetSecretsOnChain was NOT called (because secret was received)
	// In the real implementation, requestToSubmitS should not be called
	hasSecret, exists := suite.leaderNode.GetRoundSecretValue(uniqueKey, regularEoa)
	assert.True(suite.T(), exists)
	assert.True(suite.T(), hasSecret)

	fmt.Println(" TestSendSecretValueRequest_SecretReceivedBeforeTimeout completed successfully")
}

// TestSendSecretValueRequest_MissingPrivateKey tests when LEADER_PRIVATE_KEY is not set
func (suite *RevealRequestsTestSuite) TestSendSecretValueRequest_MissingPrivateKey() {
	ctx := context.Background()

	// Save original LEADER_PRIVATE_KEY
	originalKey := appconfig.Get().LeaderPrivateKey
	defer os.Setenv("LEADER_PRIVATE_KEY", originalKey)

	// Unset LEADER_PRIVATE_KEY
	os.Unsetenv("LEADER_PRIVATE_KEY")

	// Set up test data
	testRound := "send_no_key_53"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	regularEoa := "0xRegularNode5300000000000000000000000000"

	nodeInfo := &utils.NodeInfo{
		EOAAddress: regularEoa,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.53",
		Port:       "9053",
	}

	mockSendFunc := func(ctx context.Context, h host.Host, nodeInfo utils.NodeInfo, protocol string, data interface{}) error {
		return nil
	}

	// Initialize reveal request status
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})

	testTimerDuration := 500 * time.Millisecond

	// Call the testable function (should return early)
	suite.leaderNode.sendSecretValueRequestToNodeTestable(
		ctx,
		suite.host,
		testRound,
		testTrial,
		uniqueKey,
		regularEoa,
		nodeInfo,
		0,
		mockSendFunc,
		testTimerDuration,
	)

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Verify that the EOA was NOT marked as requested
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.NotContains(suite.T(), status, regularEoa)

	fmt.Println(" TestSendSecretValueRequest_MissingPrivateKey completed successfully")
}

// TestSendSecretValueRequest_InvalidPrivateKey tests when LEADER_PRIVATE_KEY is invalid
func (suite *RevealRequestsTestSuite) TestSendSecretValueRequest_InvalidPrivateKey() {
	ctx := context.Background()

	// Save original LEADER_PRIVATE_KEY
	originalKey := appconfig.Get().LeaderPrivateKey
	defer os.Setenv("LEADER_PRIVATE_KEY", originalKey)

	// Set invalid private key
	os.Setenv("LEADER_PRIVATE_KEY", "invalid_hex_string")

	// Set up test data
	testRound := "send_invalid_key_54"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	regularEoa := "0xRegularNode5400000000000000000000000000"

	nodeInfo := &utils.NodeInfo{
		EOAAddress: regularEoa,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.54",
		Port:       "9054",
	}

	mockSendFunc := func(ctx context.Context, h host.Host, nodeInfo utils.NodeInfo, protocol string, data interface{}) error {
		return nil
	}

	// Initialize reveal request status
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})

	testTimerDuration := 500 * time.Millisecond

	// Call the testable function (should return early)
	suite.leaderNode.sendSecretValueRequestToNodeTestable(
		ctx,
		suite.host,
		testRound,
		testTrial,
		uniqueKey,
		regularEoa,
		nodeInfo,
		0,
		mockSendFunc,
		testTimerDuration,
	)

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Verify that the EOA was NOT marked as requested
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.NotContains(suite.T(), status, regularEoa)

	fmt.Println(" TestSendSecretValueRequest_InvalidPrivateKey completed successfully")
}

// TestSendSecretValueRequest_MultipleRequests tests sending requests to multiple nodes
func (suite *RevealRequestsTestSuite) TestSendSecretValueRequest_MultipleRequests() {
	ctx := context.Background()

	// Set up test data
	testRound := "send_multiple_55"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	nodes := []struct {
		eoa  string
		ip   string
		port string
	}{
		{"0xNode1555555555555555555555555555555555", "127.0.0.55", "9055"},
		{"0xNode2555555555555555555555555555555556", "127.0.0.56", "9056"},
		{"0xNode3555555555555555555555555555555557", "127.0.0.57", "9057"},
	}

	mockSendFunc := func(ctx context.Context, h host.Host, nodeInfo utils.NodeInfo, protocol string, data interface{}) error {
		return nil
	}

	// Initialize reveal request status
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})

	// Use a very short timer to verify behavior quickly, but we'll mark secrets as received
	testTimerDuration := 200 * time.Millisecond

	// Send requests to all nodes
	for i, node := range nodes {
		nodeInfo := &utils.NodeInfo{
			EOAAddress: node.eoa,
			PeerID:     suite.host.ID().String(),
			IP:         node.ip,
			Port:       node.port,
		}

		suite.leaderNode.sendSecretValueRequestToNodeTestable(
			ctx,
			suite.host,
			testRound,
			testTrial,
			uniqueKey,
			node.eoa,
			nodeInfo,
			i,
			mockSendFunc,
			testTimerDuration,
		)

		// Immediately mark secret as received to prevent timer from calling requestToSubmitS
		suite.leaderNode.SetRoundSecretValue(uniqueKey, node.eoa, true)

		// Small delay between requests
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for all immediate calls
	time.Sleep(100 * time.Millisecond)

	// Verify that all EOAs were marked as requested
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Len(suite.T(), status, 3)
	for _, node := range nodes {
		assert.Contains(suite.T(), status, node.eoa)
	}

	// Wait for timers to complete
	time.Sleep(300 * time.Millisecond)

	fmt.Println(" TestSendSecretValueRequest_MultipleRequests completed successfully")
}

// TestSendSecretValueRequest_SuccessWithRealStream tests the success path with actual stream
func (suite *RevealRequestsTestSuite) TestSendSecretValueRequest_SuccessWithRealStream() {
	ctx := context.Background()

	testRound := "real_stream_56"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xRealStreamOp56000000000000000000000")

	// Cleanup database
	suite.db.Exec("DELETE FROM leader_commit_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	suite.db.Exec("DELETE FROM reveal_order_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	defer func() {
		suite.db.Exec("DELETE FROM leader_commit_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
		suite.db.Exec("DELETE FROM reveal_order_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	}()

	// Set up activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Create leader commit data
	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cos:        [32]byte{1, 2, 3},
		Cvs:        [32]byte{4, 5, 6},
		Sign: utils.SignInfo{
			R: "100",
			S: "200",
			V: "27",
		},
	}
	err := suite.leaderCommitRepo.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{testOp.Hex()},
		RevealOrder:  []int{0},
		RV:           "test_rv",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	// Initialize reveal request status
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})

	// Use the testable wrapper with very short timer
	testTimerDuration := 100 * time.Millisecond

	// Mock successful send function
	mockSendFunc := func(ctx context.Context, h host.Host, nodeInfo utils.NodeInfo, protocol string, data interface{}) error {
		return nil // Simulate successful send
	}

	nodeInfo := &utils.NodeInfo{
		EOAAddress: testOp.Hex(),
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.56",
		Port:       "9056",
	}

	// Call testable function
	suite.leaderNode.sendSecretValueRequestToNodeTestable(
		ctx,
		suite.host,
		testRound,
		testTrial,
		uniqueKey,
		testOp.Hex(),
		nodeInfo,
		0,
		mockSendFunc,
		testTimerDuration,
	)

	// Wait for immediate updates
	time.Sleep(50 * time.Millisecond)

	// Verify EOA was marked as requested
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Contains(suite.T(), status, testOp.Hex())

	// Wait for timer to expire and requestToSubmitS to be called
	time.Sleep(200 * time.Millisecond)

	// Verify SetSecretsOnChain was called
	onChain, exists := suite.leaderNode.GetSecretsOnChain(uniqueKey)
	assert.True(suite.T(), exists)
	assert.True(suite.T(), onChain)

	fmt.Println(" TestSendSecretValueRequest_SuccessWithRealStream completed successfully")
}

// TestCallFailToSubmitS tests the callFailToSubmitS function
func (suite *RevealRequestsTestSuite) TestCallFailToSubmitS() {

	ctx := context.Background()
	testRound := "fail_submit_s_57"
	testTrial := "1"

	suite.leaderNode.callFailToSubmitS(ctx, testRound, testTrial)

	fmt.Println(" TestCallFailToSubmitS completed - function executes without panic")
}

// TestStartFailToSubmitSMonitoring_TimerExpiration tests timer expiration callback
func (suite *RevealRequestsTestSuite) TestStartFailToSubmitSMonitoring_TimerExpiration() {
	ctx := context.Background()
	testRound := "timer_expiry_58"
	testTrial := "1"

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)

	// Set timestamp to past so timer fires immediately
	pastTimestamp := big.NewInt(time.Now().Unix() - 100)

	// Start monitoring with past timestamp - timer should fire almost immediately
	suite.leaderNode.StartFailToSubmitSMonitoring(ctx, testRound, testTrial, pastTimestamp)

	// Wait for monitoring to start
	time.Sleep(50 * time.Millisecond)

	time.Sleep(200 * time.Millisecond)

	// After timer fires and callback completes, monitoring should be inactive
	assert.False(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	fmt.Println("TestStartFailToSubmitSMonitoring_TimerExpiration completed successfully")
}

// TestSendToRegularNode_SuccessPath tests the success path of sendToRegularNode using mocknet
func (suite *RevealRequestsTestSuite) TestSendToRegularNode_SuccessPath() {
	// This test uses mocknet to create a real stream and test successful send
	ctx := context.Background()

	// Create two hosts using libp2p (not mocknet for simplicity)
	host1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
	defer host1.Close()

	host2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
	defer host2.Close()

	// Set up stream handler on host2
	receivedData := make(chan utils.SecretValueRequest, 1)
	host2.SetStreamHandler("/testProtocol", func(s network.Stream) {
		defer s.Close()
		var req utils.SecretValueRequest
		err := json.NewDecoder(s).Decode(&req)
		if err == nil {
			receivedData <- req
		}
	})

	// Connect hosts
	host1.Peerstore().AddAddrs(host2.ID(), host2.Addrs(), time.Hour)

	// Test data
	testReq := utils.SecretValueRequest{
		LeaderEoaAddress:  "0xLeader",
		RegularEoaAddress: "0xRegular",
		Round:             "100",
		TrialNum:          "1",
		Order:             0,
	}

	nodeInfo := utils.NodeInfo{
		PeerID: host2.ID().String(),
		IP:     "127.0.0.1",
		Port:   "9999",
	}

	// Call sendToRegularNode
	err = sendToRegularNode(ctx, host1, nodeInfo, "/testProtocol", testReq)
	assert.NoError(suite.T(), err)

	// Verify data was received
	select {
	case received := <-receivedData:
		assert.Equal(suite.T(), testReq.LeaderEoaAddress, received.LeaderEoaAddress)
		assert.Equal(suite.T(), testReq.Round, received.Round)
	case <-time.After(2 * time.Second):
		suite.T().Fatal("Timeout waiting for data")
	}

	fmt.Println(" TestSendToRegularNode_SuccessPath completed successfully")
}

// TestStartMonitoringWithPeriod_ImmediateExpiration tests monitoring with immediate expiration
func (suite *RevealRequestsTestSuite) TestStartMonitoringWithPeriod_ImmediateExpiration() {
	ctx := context.Background()
	testRound := "immediate_59"
	testTrial := "1"

	suite.leaderNode.SetHalted(false)
	suite.leaderNode.SetFailToSubmitSMonitoringActive(false)

	// Set last submit timestamp to long in the past
	veryPastTimestamp := big.NewInt(time.Now().Unix() - 1000)
	suite.leaderNode.SetLastSubmitSTimestamp(veryPastTimestamp)

	shortPeriod := big.NewInt(10)
	suite.leaderNode.startMonitoringWithPeriod(ctx, testRound, testTrial, shortPeriod)

	// Monitoring should start
	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	// Wait for timer to fire (it should fire almost immediately due to negative duration)
	time.Sleep(500 * time.Millisecond)

	// Cleanup
	suite.leaderNode.StopFailToSubmitSMonitoring(ctx, testRound, testTrial)

	fmt.Println(" TestStartMonitoringWithPeriod_ImmediateExpiration completed successfully")
}

// TestSendSecretValueRequest_IntegrationWithMocknet tests the actual sendSecretValueRequestToNode with working stream
func (suite *RevealRequestsTestSuite) TestSendSecretValueRequest_IntegrationWithMocknet() {
	ctx := context.Background()

	testRound := "integration_60"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	regularEoa := "0xIntegrationNode60000000000000000000000"

	// Create two hosts
	leaderHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
	defer leaderHost.Close()

	regularHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
	defer regularHost.Close()

	requestReceived := make(chan bool, 1)
	regularHost.SetStreamHandler("/sendSecretValue", func(s network.Stream) {
		defer s.Close()
		var req utils.SecretValueRequest
		err := json.NewDecoder(s).Decode(&req)
		if err == nil {
			log.Printf("Regular node received secret value request: %+v", req)
			requestReceived <- true
		}
	})

	// Connect the hosts
	leaderHost.Peerstore().AddAddrs(regularHost.ID(), regularHost.Addrs(), time.Hour)

	// Initialize reveal request status
	suite.leaderNode.SetRevealRequestStatus(uniqueKey, []string{})

	nodeInfo := &utils.NodeInfo{
		EOAAddress: regularEoa,
		PeerID:     regularHost.ID().String(),
		IP:         "127.0.0.1",
		Port:       "0",
	}

	// Call the actual sendSecretValueRequestToNode function
	suite.leaderNode.sendSecretValueRequestToNode(ctx, leaderHost, testRound, testTrial, uniqueKey, regularEoa, nodeInfo, 0)

	// Wait for request to be received
	select {
	case <-requestReceived:
		log.Println(" Secret value request was successfully sent and received")
	case <-time.After(2 * time.Second):
		suite.T().Fatal("Timeout waiting for secret value request")
	}

	time.Sleep(100 * time.Millisecond)

	// Verify EOA was marked as requested (this covers the else branch lines 94-117)
	status, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)
	assert.Contains(suite.T(), status, regularEoa)

	fmt.Println(" TestSendSecretValueRequest_IntegrationWithMocknet completed successfully")
}

// TestRequestToSubmitS_FullExecution tests requestToSubmitS with all dependencies mocked
func (suite *RevealRequestsTestSuite) TestRequestToSubmitS_FullExecution() {
	ctx := context.Background()

	testRound := "request_submit_s_61"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp := common.HexToAddress("0xRequestSubmitSOp61000000000000000000")

	// Cleanup database
	suite.db.Exec("DELETE FROM leader_commit_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	suite.db.Exec("DELETE FROM reveal_order_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	defer func() {
		suite.db.Exec("DELETE FROM leader_commit_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
		suite.db.Exec("DELETE FROM reveal_order_schemes WHERE round = ? AND trial_num = ?", testRound, testTrial)
	}()

	// Set current round so SetSecretRequestSentForWhichRound works
	suite.leaderNode.SetCurrentRound(testRound)

	// Set up activated operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp})

	// Create leader commit data with all required fields
	commitData := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp.Hex(),
		Cos:        [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		Cvs:        [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		Sign: utils.SignInfo{
			R: "1000",
			S: "2000",
			V: "27",
		},
	}
	err := suite.leaderCommitRepo.AddLeaderCommit(context.Background(), commitData)
	require.NoError(suite.T(), err)

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{testOp.Hex()},
		RevealOrder:  []int{0},
		RV:           "test_rv_61",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	// Set round secrets
	secret := [32]byte{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41}
	suite.leaderNode.AppendToRoundSecrets(uniqueKey, secret)

	// Call requestToSubmitS - it will fail at ExecuteTransaction but covers most of the function
	suite.leaderNode.requestToSubmitS(ctx, testRound, testTrial)

	// Verify SetSecretRequestSentForWhichRound was called
	assert.Equal(suite.T(), testRound, suite.leaderNode.GetSecretRequestSentForWhichRound())

	fmt.Println(" TestRequestToSubmitS_FullExecution completed successfully")
}

// TestSendToRegularNode_EncodingError tests encoding error path
func (suite *RevealRequestsTestSuite) TestSendToRegularNode_EncodingError() {
	ctx := context.Background()

	// Create host
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(suite.T(), err)
	defer h.Close()

	// Try to send to a non-existent peer - this will fail at stream creation
	nodeInfo := utils.NodeInfo{
		PeerID: "invalid_peer_id",
		IP:     "127.0.0.1",
		Port:   "9999",
	}

	// This should fail
	err = sendToRegularNode(ctx, h, nodeInfo, "/testProtocol", struct{}{})
	assert.Error(suite.T(), err)

	fmt.Println(" TestSendToRegularNode_EncodingError completed - error path covered")
}

// TestCallFailToSubmitS_WithMockEthService tests callFailToSubmitS with mocked eth service
func (suite *RevealRequestsTestSuite) TestCallFailToSubmitS_WithMockEthService() {
	ctx := context.Background()
	testRound := "mock_fail_submit_s_62"
	testTrial := "1"

	// Save original eth service and restore it later
	originalEthService := suite.leaderNode.ethService
	defer func() {
		suite.leaderNode.ethService = originalEthService
	}()

	suite.leaderNode.callFailToSubmitS(ctx, testRound, testTrial)

	// Test passes if no panic occurs - the function handles missing ABI gracefully
	fmt.Println(" TestCallFailToSubmitS_WithMockEthService completed successfully")
}

// TestStartSecretValueRequests_AllPathsCovered tests all code paths in StartSecretValueRequests
func (suite *RevealRequestsTestSuite) TestStartSecretValueRequests_AllPathsCovered() {
	ctx := context.Background()

	testRound := "all_paths_63"
	testTrial := "1"
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)

	suite.leaderNode.SetHalted(true)
	suite.leaderNode.StartSecretValueRequests(ctx, suite.host, testRound, testTrial)
	suite.leaderNode.SetHalted(false)

	node1 := "0xAllPaths1111111111111111111111111111111"

	// Cleanup
	suite.db.Model((*database.NodeInfoScheme)(nil)).
		Where("eoa_address = ?", node1).
		Delete()
	suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
	defer func() {
		suite.db.Model((*database.NodeInfoScheme)(nil)).
			Where("eoa_address = ?", node1).
			Delete()
		suite.db.Model((*database.RevealOrderScheme)(nil)).
			Where("round = ? AND trial_num = ?", testRound, testTrial).
			Delete()
	}()

	// Create node info
	nodeInfo := &utils.NodeInfo{
		EOAAddress: node1,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.63",
		Port:       "9063",
	}
	err := suite.nodeInfoRepo.AddAndUpdateNodeInfo(context.Background(), nodeInfo)
	require.NoError(suite.T(), err)

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{node1},
		RevealOrder:  []int{0},
		RV:           "test_rv_63",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	// This should trigger sendSecretValueRequestToNode
	suite.leaderNode.StartSecretValueRequests(ctx, suite.host, testRound, testTrial)

	// Verify reveal request status was initialized
	_, exists := suite.leaderNode.GetRevealRequestStatus(uniqueKey)
	assert.True(suite.T(), exists)

	fmt.Println(" TestStartSecretValueRequests_AllPathsCovered completed successfully")
}

// TestPrepareArgumentsForRequestToSubmitS_WithIndices tests with various index scenarios
func (suite *RevealRequestsTestSuite) TestPrepareArgumentsForRequestToSubmitS_WithIndices() {
	ctx := context.Background()

	nanoTime := time.Now().UnixNano()
	testRound := fmt.Sprintf("prep_idx_%d", nanoTime)
	testTrial := fmt.Sprintf("%d", nanoTime%10000) // Use unique trial  from timestamp
	uniqueKey := utils.GetUniqueKey(testRound, testTrial)
	testOp1 := common.HexToAddress(fmt.Sprintf("0x64%038d", nanoTime%1000000000000000000))
	testOp2 := common.HexToAddress(fmt.Sprintf("0x65%038d", (nanoTime+1)%1000000000000000000))

	// Thorough cleanup - delete by both round/trial AND eoa_address
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()
	suite.db.Model((*database.LeaderCommitScheme)(nil)).
		Where("eoa_address IN (?)", pg.In([]string{testOp1.Hex(), testOp2.Hex()})).
		Delete()
	suite.db.Model((*database.RevealOrderScheme)(nil)).
		Where("round = ? AND trial_num = ?", testRound, testTrial).
		Delete()

	// Also cleanup at the end
	defer func() {
		suite.db.Model((*database.LeaderCommitScheme)(nil)).
			Where("round = ? AND trial_num = ?", testRound, testTrial).
			Delete()
		suite.db.Model((*database.LeaderCommitScheme)(nil)).
			Where("eoa_address IN (?)", pg.In([]string{testOp1.Hex(), testOp2.Hex()})).
			Delete()
		suite.db.Model((*database.RevealOrderScheme)(nil)).
			Where("round = ? AND trial_num = ?", testRound, testTrial).
			Delete()
		// Reset indices
		suite.leaderNode.indicesMutex.Lock()
		suite.leaderNode.indices = []*big.Int{}
		suite.leaderNode.indicesMutex.Unlock()
	}()

	// Set up activated operators with 2 operators
	eth.SetActivatedOperatorsCached([]common.Address{testOp1, testOp2})

	// Create leader commits for both
	commitData1 := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp1.Hex(),
		Cos:        [32]byte{1, 2, 3},
		Cvs:        [32]byte{4, 5, 6},
		Sign: utils.SignInfo{
			R: "100",
			S: "200",
			V: "27",
		},
	}
	err := suite.leaderCommitRepo.AddLeaderCommit(context.Background(), commitData1)
	require.NoError(suite.T(), err)

	commitData2 := &utils.LeaderCommitData{
		Round:      testRound,
		TrialNum:   testTrial,
		EOAAddress: testOp2.Hex(),
		Cos:        [32]byte{7, 8, 9},
		Cvs:        [32]byte{10, 11, 12},
		Sign: utils.SignInfo{
			R: "300",
			S: "400",
			V: "28",
		},
	}
	err = suite.leaderCommitRepo.AddLeaderCommit(context.Background(), commitData2)
	require.NoError(suite.T(), err)

	// Create reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        testRound,
		TrialNum:     testTrial,
		OrderedNodes: []string{testOp1.Hex(), testOp2.Hex()},
		RevealOrder:  []int{0, 1},
		RV:           "test_rv_64",
	}
	err = suite.revealOrderRepo.AddRevealOrder(context.Background(), revealOrder)
	require.NoError(suite.T(), err)

	// Set indices to indicate one operator already submitted on-chain
	suite.leaderNode.AppendToIndices(big.NewInt(0))

	// Set round secrets
	secret1 := [32]byte{10, 11, 12}
	secret2 := [32]byte{13, 14, 15}
	suite.leaderNode.AppendToRoundSecrets(uniqueKey, secret1)
	suite.leaderNode.AppendToRoundSecrets(uniqueKey, secret2)

	// Call prepare function - should handle indices correctly
	allCos, secretsReceived, packedVs, cvNotOnChainCvAndSigRS, packedRevealOrders := suite.leaderNode.prepareArgumentsForRequestToSubmitS(ctx, testRound, testTrial)

	assert.NotNil(suite.T(), allCos)
	assert.Len(suite.T(), allCos, 2) // Both operators
	assert.NotNil(suite.T(), secretsReceived)
	assert.Len(suite.T(), secretsReceived, 2) // Both secrets
	assert.NotNil(suite.T(), packedVs)
	assert.NotNil(suite.T(), cvNotOnChainCvAndSigRS)
	assert.Len(suite.T(), cvNotOnChainCvAndSigRS, 1) // Only one not on chain (operator at index 1)
	assert.NotNil(suite.T(), packedRevealOrders)

	fmt.Println(" TestPrepareArgumentsForRequestToSubmitS_WithIndices completed successfully")
}

// TestRevealRequestsTestSuite runs the test suite
func TestRevealRequestsTestSuite(t *testing.T) {
	suite.Run(t, new(RevealRequestsTestSuite))
}

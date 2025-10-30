package leader_node

import (
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-pg/pg/v10"
	_ "github.com/lib/pq"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

// RevealRequestsTestSuite defines the test suite for reveal_requests.go
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
	require.NoError(suite.T(), err, "Failed to open SQL DB")
	defer sqlDB.Close()

	err = sqlDB.Ping()
	require.NoError(suite.T(), err, "Failed to ping SQL DB")

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

	// Set environment variable for tests
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(crypto.FromECDSA(suite.testPrivateKey)))

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

	suite.leaderNode = &LeaderNode{
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

	// Create node info
	nodeInfo := &utils.NodeInfo{
		EOAAddress: suite.testEOA,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.1",
		Port:       "9000",
	}
	err := suite.nodeInfoRepo.AddNodeInfo(context.Background(), nodeInfo)
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
	err = suite.nodeInfoRepo.AddNodeInfo(context.Background(), nodeInfo)
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

	period := big.NewInt(200) // 200 seconds in future
	suite.leaderNode.startMonitoringWithPeriod(context.Background(), testRound, testTrial, period)

	assert.True(suite.T(), suite.leaderNode.GetFailToSubmitSMonitoringActive())

	// Wait briefly to ensure timer is set
	time.Sleep(100 * time.Millisecond)

	// Cleanup - stop before timer expires to avoid contract call
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

	// Create node infos with unique IPs
	nodeInfo1 := &utils.NodeInfo{
		EOAAddress: node1,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.1",
		Port:       "9001",
	}
	err := suite.nodeInfoRepo.AddNodeInfo(context.Background(), nodeInfo1)
	require.NoError(suite.T(), err)

	nodeInfo2 := &utils.NodeInfo{
		EOAAddress: node2,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.2",
		Port:       "9002",
	}
	err = suite.nodeInfoRepo.AddNodeInfo(context.Background(), nodeInfo2)
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

// TestPrepareArguments_Comprehensive tests comprehensive argument preparation
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
	err = suite.nodeInfoRepo.AddNodeInfo(context.Background(), nodeInfo1)
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

	// Create both node infos
	nodeInfo1 := &utils.NodeInfo{
		EOAAddress: node1,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.20",
		Port:       "9020",
	}
	err := suite.nodeInfoRepo.AddNodeInfo(context.Background(), nodeInfo1)
	require.NoError(suite.T(), err)

	nodeInfo2 := &utils.NodeInfo{
		EOAAddress: node2,
		PeerID:     suite.host.ID().String(),
		IP:         "127.0.0.21",
		Port:       "9021",
	}
	err = suite.nodeInfoRepo.AddNodeInfo(context.Background(), nodeInfo2)
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

// TestRevealRequestsTestSuite runs the test suite
func TestRevealRequestsTestSuite(t *testing.T) {
	suite.Run(t, new(RevealRequestsTestSuite))
}

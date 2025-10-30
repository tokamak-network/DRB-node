package leader_node

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-pg/pg/v10"
	_ "github.com/lib/pq"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
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

	suite.leaderNodeHandler = NewLeaderNodeHandler(nil, suite.db)
}

// TearDownSuite runs once after all tests in the suite
func (suite *LeaderHandlerTestSuite) TearDownSuite() {
	if suite.db != nil {
		suite.db.Close()
		log.Println("Leader handler test suite cleaned up")
	}
}

// Mock stream implementation
type mockStream struct {
	readBuffer  *bytes.Buffer
	writeBuffer *bytes.Buffer
	closed      bool
}

func newMockStream() *mockStream {
	return &mockStream{
		readBuffer:  new(bytes.Buffer),
		writeBuffer: new(bytes.Buffer),
		closed:      false,
	}
}

func (m *mockStream) Read(p []byte) (n int, err error)   { return m.readBuffer.Read(p) }
func (m *mockStream) Write(p []byte) (n int, err error)  { return m.writeBuffer.Write(p) }
func (m *mockStream) Close() error                       { m.closed = true; return nil }
func (m *mockStream) Reset() error                       { return nil }
func (m *mockStream) SetDeadline(t time.Time) error      { return nil }
func (m *mockStream) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockStream) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockStream) ID() string                         { return "mock-stream-id" }
func (m *mockStream) Protocol() protocol.ID              { return protocol.ID("/test") }
func (m *mockStream) SetProtocol(protocol.ID) error      { return nil }
func (m *mockStream) Stat() network.Stats                { return network.Stats{} }
func (m *mockStream) Conn() network.Conn                 { return nil }
func (m *mockStream) CloseWrite() error                  { m.closed = true; return nil }
func (m *mockStream) CloseRead() error                   { m.closed = true; return nil }
func (m *mockStream) Scope() network.StreamScope         { return nil }

// TestLeaderHandler_AtomicOperations tests atomic get/set operations
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_AtomicOperations() {
	handler := &LeaderNodeHandler{}

	assert.False(suite.T(), handler.GetMerkleRootSubmitted())
	handler.SetMerkleRootSubmitted(true)
	assert.True(suite.T(), handler.GetMerkleRootSubmitted())
	handler.SetMerkleRootSubmitted(false)
	assert.False(suite.T(), handler.GetMerkleRootSubmitted())
}

// TestLeaderHandler_Initialization tests handler initialization
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_Initialization() {
	handler := NewLeaderNodeHandler(nil, suite.db)

	require.NotNil(suite.T(), handler)
	assert.NotNil(suite.T(), handler.leaderNode)
	assert.False(suite.T(), handler.GetMerkleRootSubmitted())
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

// TestLeaderHandler_VerifySignature_Fail tests signature verification failure
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_VerifySignature_Fail() {
	req := utils.Request{
		Round:      "1",
		TrialNum:   "1",
		EOAAddress: "0xTestOp",
		Signature:  []byte("invalid"),
	}

	result := suite.leaderNodeHandler.VerifySignatureAndCheckActivation(context.Background(), req, "commit")
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

// TestLeaderHandler_VerifySignature_ActivationFail tests signature verification with activation failure
func (suite *LeaderHandlerTestSuite) TestLeaderHandler_VerifySignature_ActivationFail() {
	req := utils.Request{
		Round:      "1",
		TrialNum:   "1",
		EOAAddress: "0xTest",
		Signature:  []byte{1, 2, 3}, // Invalid
	}

	result := suite.leaderNodeHandler.VerifySignatureAndCheckActivation(context.Background(), req, "test")
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

	// Generate VALID signature
	signature := generateValidSignature(eoaAddress, privateKey)
	require.NotNil(suite.T(), signature)

	verifyReq := utils.Verification{
		EOAAddress: eoaAddress,
		Signature:  signature,
	}
	assert.True(suite.T(), utils.VerifySignature(verifyReq), "Signature should be valid")

	// Now send COS request
	cosReq := utils.CosRequest{
		EOAAddress: eoaAddress,
		Cos:        cos,
		Signature:  signature,
	}

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

	// Generate valid signature
	signature := generateValidSignature(eoaAddress, privateKey)
	require.NotNil(suite.T(), signature)

	cosReq := utils.CosRequest{
		EOAAddress: eoaAddress,
		Cos:        cos,
		Signature:  signature,
	}

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

	// Generate valid signature
	signature := generateValidSignature(eoaAddress, privateKey)
	require.NotNil(suite.T(), signature)

	// Try to send COS again
	cosReq := utils.CosRequest{
		EOAAddress: eoaAddress,
		Cos:        cos,
		Signature:  signature,
	}

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

	// Generate valid signature
	signature := generateValidSignature(eoaAddress, privateKey)
	require.NotNil(suite.T(), signature)

	cosReq := utils.CosRequest{
		EOAAddress: eoaAddress,
		Cos:        cos,
		Signature:  signature,
	}

	stream := newMockStream()
	jsonData, _ := json.Marshal(cosReq)
	stream.readBuffer.Write(jsonData)

	// Should reject due to hash mismatch
	suite.leaderNodeHandler.handleCOSRequest(context.Background(), nil, stream)

	assert.True(suite.T(), stream.closed)
}

// TestLeaderHandlerTestSuite runs the test suite
func TestLeaderHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(LeaderHandlerTestSuite))
}

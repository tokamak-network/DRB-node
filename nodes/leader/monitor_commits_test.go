package leader_node

import (
	"context"
	"encoding/hex"
	"errors"
	"log"
	"math/big"
	"os"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

const testABIContent = `{
    "abi": [
        {
            "inputs": [],
            "name": "deposit",
            "outputs": [],
            "stateMutability": "payable",
            "type": "function"
        },
        {
            "inputs": [{"name": "operator", "type": "address"}],
            "name": "s_depositAmount",
            "outputs": [{"name": "", "type": "uint256"}],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "s_activationThreshold",
            "outputs": [{"name": "", "type": "uint256"}],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "activate",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "getActivatedOperators",
            "outputs": [{"name": "", "type": "address[]"}],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [{"name": "cv", "type": "bytes32"}],
            "name": "submitCv",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [{"name": "co", "type": "bytes32"}],
            "name": "submitCo",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [{"name": "s", "type": "bytes32"}],
            "name": "submitS",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "generateRandomNumber",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "generateRandomNumberWhenSomeCvsAreOnChain",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "packedIndicesAscendingFromLSB", "type": "uint256"}
            ],
            "name": "RequestedToSubmitCv",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "cv", "type": "bytes32"},
                {"indexed": false, "name": "index", "type": "uint256"}
            ],
            "name": "CvSubmitted",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "curRound", "type": "uint256"},
                {"indexed": false, "name": "curTrialNum", "type": "uint256"},
                {"indexed": false, "name": "curState", "type": "uint256"}
            ],
            "name": "Status",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "merkleRoot", "type": "bytes32"}
            ],
            "name": "MerkleRootSubmitted",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "indicesLength", "type": "uint256"},
                {"indexed": false, "name": "packedIndices", "type": "uint256"}
            ],
            "name": "RequestedToSubmitCo",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "indexK", "type": "uint256"}
            ],
            "name": "RequestedToSubmitSFromIndexK",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "s", "type": "bytes32"},
                {"indexed": false, "name": "index", "type": "uint256"}
            ],
            "name": "SSubmitted",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "round", "type": "uint256"},
                {"indexed": false, "name": "trialNum", "type": "uint256"},
                {"indexed": false, "name": "co", "type": "bytes32"},
                {"indexed": false, "name": "index", "type": "uint256"}
            ],
            "name": "CoSubmitted",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {"indexed": false, "name": "operator", "type": "address"}
            ],
            "name": "DeActivated",
            "type": "event"
        },
        {
            "inputs": [],
            "name": "failToRequestSubmitCvOrSubmitMerkleRoot",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "failToSubmitMerkleRootAfterDispute",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "failToRequestSorGenerateRandomNumber",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        }
    ]
}`

type mockLeaderCommitRepo struct {
	byRoundTrial    []*utils.LeaderCommitData
	getErr          error
	getByEOAData    map[string]*utils.LeaderCommitData
	updateRandomHit int
}

var errNotFound = errors.New("not found")

func (m *mockLeaderCommitRepo) GetLeaderCommitByRoundAndEoaAddr(ctx context.Context, round, trialNum, eoaAddress string) (*utils.LeaderCommitData, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.getByEOAData != nil {
		if v, ok := m.getByEOAData[eoaAddress]; ok {
			return v, nil
		}
	}
	return nil, errNotFound
}

func (m *mockLeaderCommitRepo) UpdateLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	return nil
}
func (m *mockLeaderCommitRepo) AddLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	return nil
}
func (m *mockLeaderCommitRepo) GetLeaderCommitsByRoundAndTrialNum(ctx context.Context, round, trialNum string) ([]*utils.LeaderCommitData, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.byRoundTrial, nil
}
func (m *mockLeaderCommitRepo) UpdateLeaderCommitRandomNumberGenerated(ctx context.Context, round, trialNum string) error {
	m.updateRandomHit++
	return nil
}

var _ database.ILeaderCommitRepository = (*mockLeaderCommitRepo)(nil)

type mockEthServiceWithError struct{}

func (m *mockEthServiceWithError) GetActivatedOperatorsCached() []common.Address {
	return nil
}
func (m *mockEthServiceWithError) GetActivatedOperatorsLength() int64 { return 0 }
func (m *mockEthServiceWithError) SetActivatedOperatorsCached(operators []common.Address) {
}
func (m *mockEthServiceWithError) GetActivatedOperatorsUnsafe() []common.Address { return nil }
func (m *mockEthServiceWithError) GetActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
	return nil, errors.New("network error")
}
func (m *mockEthServiceWithError) UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) {
}
func (m *mockEthServiceWithError) CallSmartContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	return nil, nil
}
func (m *mockEthServiceWithError) ExecuteTransaction(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
	return nil, nil, errors.New("tx error")
}
func (m *mockEthServiceWithError) UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	return nil, errors.New("error")
}
func (m *mockEthServiceWithError) GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	return nil, errors.New("error")
}

var _ eth.IEthService = (*mockEthServiceWithError)(nil)

type mockRevealOrderRepo struct {
	order *utils.RevealOrderData
	err   error
}

func (m *mockRevealOrderRepo) GetRevealOrder(ctx context.Context, round, trialNum string) (*utils.RevealOrderData, error) {
	return m.order, m.err
}
func (m *mockRevealOrderRepo) AddRevealOrder(ctx context.Context, order *utils.RevealOrderData) error {
	return nil
}

var _ database.IRevealOrderRepository = (*mockRevealOrderRepo)(nil)

type noopBatchRepo struct{}

func (n *noopBatchRepo) DeleteOldRoundDataForLeaderNode(ctx context.Context, currentRound string) error {
	return nil
}
func (n *noopBatchRepo) DeleteRoundTrialDataForLeaderNode(ctx context.Context, round, trialNum string) error {
	return nil
}
func (n *noopBatchRepo) DeleteOldRoundDataForRegularNode(ctx context.Context, currentRound string) error {
	return nil
}
func (n *noopBatchRepo) DeleteRoundTrialDataForRegularNode(ctx context.Context, round, trialNum string) error {
	return nil
}

var _ database.IBatchRepository = (*noopBatchRepo)(nil)

type noopBroadcastTrackerRepo struct{}

func (n *noopBroadcastTrackerRepo) AddBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error {
	return nil
}
func (n *noopBroadcastTrackerRepo) UpdateBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error {
	return nil
}

var _ database.IBroadcastTrackerRepository = (*noopBroadcastTrackerRepo)(nil)

type noopNodeInfoRepo struct{}

func (n *noopNodeInfoRepo) AddAndUpdateNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	return nil
}
func (n *noopNodeInfoRepo) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	return nil, nil
}
func (n *noopNodeInfoRepo) DeleteNodeInfoByEOA(ctx context.Context, eoaAddress string) error {
	return nil
}

var _ database.INodeInfoRepository = (*noopNodeInfoRepo)(nil)

type mockEthService struct {
	operators      []common.Address
	execCalled     int
	execLastMethod string
}

func (m *mockEthService) GetActivatedOperatorsCached() []common.Address {
	return append([]common.Address{}, m.operators...)
}
func (m *mockEthService) GetActivatedOperatorsLength() int64 { return int64(len(m.operators)) }
func (m *mockEthService) SetActivatedOperatorsCached(operators []common.Address) {
	m.operators = append([]common.Address{}, operators...)
}
func (m *mockEthService) GetActivatedOperatorsUnsafe() []common.Address { return m.operators }
func (m *mockEthService) GetActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
	return append([]common.Address{}, m.operators...), nil
}
func (m *mockEthService) UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) {
}
func (m *mockEthService) CallSmartContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	return nil, nil
}
func (m *mockEthService) ExecuteTransaction(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
	m.execCalled++
	m.execLastMethod = method
	// Return a properly initialized transaction to avoid nil pointer errors
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    0,
		GasPrice: big.NewInt(0),
		Gas:      0,
		To:       &common.Address{},
		Value:    big.NewInt(0),
		Data:     nil,
	})
	return tx, nil, nil
}
func (m *mockEthService) UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	return big.NewInt(1), nil
}
func (m *mockEthService) GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	return big.NewInt(1), nil
}

// Ensure mock satisfies the interface
var _ eth.IEthService = (*mockEthService)(nil)

type MonitorCommitsSuite struct {
	suite.Suite
	ctx     context.Context
	ln      *LeaderNode
	ethMock *mockEthService
	lcRepo  *mockLeaderCommitRepo
	roRepo  *mockRevealOrderRepo
}

func (s *MonitorCommitsSuite) SetupTest() {
	// Initialize logger
	logger.InitLogger()

	s.ctx = context.Background()
	// Mock eth service
	s.ethMock = &mockEthService{}
	eth.Service = s.ethMock

	// Minimal repos
	s.lcRepo = &mockLeaderCommitRepo{}
	s.roRepo = &mockRevealOrderRepo{}

	s.ln = &LeaderNode{
		leaderCommitRepository:     s.lcRepo,
		batchRepository:            &noopBatchRepo{},
		broadcastTrackerRepository: &noopBroadcastTrackerRepo{},
		reavealOrderRepository:     s.roRepo,
		nodeInfoRepository:         &noopNodeInfoRepo{},
		cvOnChain:                  make(map[string]bool),
		roundSecrets:               make(map[string][][32]byte),
		roundSecret:                make(map[string]map[string]bool),
		secretsOnChain:             make(map[string]bool),
	}

	// Set round/trial
	s.ln.SetCurrentRound("1")
	s.ln.SetCurrentTrial("1")
	// Default: not halted, execution true
	s.ln.SetHalted(false)
	s.ln.SetExecution(true)

	// Set envs needed by tx paths
	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", common.HexToAddress("0x0000000000000000000000000000000000000001").Hex())
}

func (s *MonitorCommitsSuite) TearDownTest() {
	// Restore default service to avoid leaking across other tests
	eth.Service = eth.NewDefaultEthService()
}

func makeLeaderCommit(addr common.Address, withSecret bool, v string, r common.Hash, sh common.Hash) *utils.LeaderCommitData {
	var secret [32]byte
	if withSecret {
		copy(secret[:], []byte("secret-" + addr.Hex())[:])
	}
	return &utils.LeaderCommitData{
		EOAAddress:  addr.Hex(),
		SecretValue: secret,
		Cvs:         [32]byte{1},
		Cos:         [32]byte{2},
		Sign: utils.SignInfo{
			V: v,
			R: r.Hex(),
			S: sh.Hex(),
		},
	}
}

func (s *MonitorCommitsSuite) Test_packRevealOrder() {
	bi := packRevealOrder([]int{1, 2, 3})
	s.Equal("0x030201", "0x"+hex.EncodeToString(bi.Bytes()))
}

func (s *MonitorCommitsSuite) Test_packVsValues() {
	bi := packVsValues([]uint8{27, 28})
	s.Equal("0x1c1b", "0x"+hex.EncodeToString(bi.Bytes()))
}

func (s *MonitorCommitsSuite) Test_sortLeaderCommitsByActivatedOperators() {
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	s.ethMock.SetActivatedOperatorsCached([]common.Address{addr2, addr1})
	lc1 := makeLeaderCommit(addr1, true, "27", common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2)))
	lc2 := makeLeaderCommit(addr2, true, "28", common.BigToHash(big.NewInt(3)), common.BigToHash(big.NewInt(4)))
	res := sortLeaderCommitsByActivatedOperators([]*utils.LeaderCommitData{lc1, lc2})
	s.Require().Len(res, 2)
	s.Equal(addr2.Hex(), res[0].EOAAddress)
	s.Equal(addr1.Hex(), res[1].EOAAddress)
}

func (s *MonitorCommitsSuite) Test_LoadNodeData_ParsesAndDefaults() {
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	s.ethMock.SetActivatedOperatorsCached([]common.Address{addr1, addr2})
	lc1 := makeLeaderCommit(addr1, true, "27", common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2)))
	lc2 := makeLeaderCommit(addr2, true, "", common.Hash{}, common.Hash{})
	s.lcRepo.byRoundTrial = []*utils.LeaderCommitData{lc2, lc1}

	cvs, cos, secrets, vs, rs, ss := s.ln.LoadNodeData(s.ctx, "1", "1")
	s.Require().Len(secrets, 2)
	s.Equal(byte(0), vs[1]) // second commit had empty v
	s.Equal(common.Hash{}, rs[1])
	s.Equal(common.Hash{}, ss[1])
	s.Equal([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, cvs[0])
	s.Equal([]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, cos[0])
}

func (s *MonitorCommitsSuite) Test_checkRoundsForCompletion_SkipsWhenMissingSignature() {
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	s.ethMock.SetActivatedOperatorsCached([]common.Address{addr1, addr2})
	lc1 := makeLeaderCommit(addr1, true, "27", common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2)))
	lc2 := makeLeaderCommit(addr2, true, "", common.Hash{}, common.Hash{})
	s.lcRepo.getByEOAData = map[string]*utils.LeaderCommitData{
		addr1.Hex(): lc1,
		addr2.Hex(): lc2,
	}

	s.ln.checkRoundsForCompletion(s.ctx)
	s.Equal(0, s.ethMock.execCalled)
	s.Equal(0, s.lcRepo.updateRandomHit)
}

func (s *MonitorCommitsSuite) Test_checkRoundsForCompletion_SkipsWhenNotEnoughOperators() {
	s.ethMock.SetActivatedOperatorsCached([]common.Address{common.HexToAddress("0x1")})
	s.ln.checkRoundsForCompletion(s.ctx)
	s.Equal(0, s.ethMock.execCalled)
}

func TestMonitorCommitsSuite(t *testing.T) {
	suite.Run(t, new(MonitorCommitsSuite))
}

func TestLeaderNode_checkRoundsForCompletion_Halted(t *testing.T) {
	logger.InitLogger()

	mockEth := &mockEthService{operators: []common.Address{}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ln := &LeaderNode{
		leaderCommitRepository: &mockLeaderCommitRepo{},
		reavealOrderRepository: &mockRevealOrderRepo{},
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(true)

	// Should return early when halted
	ln.checkRoundsForCompletion(context.Background())

	// No error expected, just early return
}

func TestLeaderNode_checkRoundsForCompletion_AllRandomNumberGenerated(t *testing.T) {
	logger.InitLogger()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthService{operators: []common.Address{addr1, addr2}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		byRoundTrial: []*utils.LeaderCommitData{
			{
				EOAAddress:            addr1.Hex(),
				RandomNumberGenerated: true,
			},
			{
				EOAAddress:            addr2.Hex(),
				RandomNumberGenerated: true,
			},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		reavealOrderRepository: &mockRevealOrderRepo{},
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Should return early when all random numbers are already generated
	ln.checkRoundsForCompletion(context.Background())
}

func TestLeaderNode_checkRoundsForCompletion_InsufficientOperators(t *testing.T) {
	logger.InitLogger()

	// Only 1 operator (need at least 2)
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")

	mockEth := &mockEthService{operators: []common.Address{addr1}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ln := &LeaderNode{
		leaderCommitRepository: &mockLeaderCommitRepo{},
		reavealOrderRepository: &mockRevealOrderRepo{},
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Should return early when operators < 2
	ln.checkRoundsForCompletion(context.Background())
}

func TestLeaderNode_checkRoundsForCompletion_MissingCommitData(t *testing.T) {
	logger.InitLogger()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthService{operators: []common.Address{addr1, addr2}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		getErr: errors.New("commit data not found"),
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		reavealOrderRepository: &mockRevealOrderRepo{},
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Should return early when commit data not found
	ln.checkRoundsForCompletion(context.Background())
}

func TestLeaderNode_checkRoundsForCompletion_MissingSecretValue(t *testing.T) {
	logger.InitLogger()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthService{operators: []common.Address{addr1, addr2}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		getByEOAData: map[string]*utils.LeaderCommitData{
			addr1.Hex(): {
				EOAAddress:  addr1.Hex(),
				SecretValue: [32]byte{}, // Empty secret
				Cos:         [32]byte{1},
			},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		reavealOrderRepository: &mockRevealOrderRepo{},
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Should return early when secret value is missing
	ln.checkRoundsForCompletion(context.Background())
}

func TestLeaderNode_checkRoundsForCompletion_EmptySignatureV(t *testing.T) {
	logger.InitLogger()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthService{operators: []common.Address{addr1, addr2}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		getByEOAData: map[string]*utils.LeaderCommitData{
			addr1.Hex(): {
				EOAAddress:  addr1.Hex(),
				SecretValue: [32]byte{1, 2, 3},
				Sign: utils.SignInfo{
					V: "", // Empty V
					R: "0x123",
					S: "0x456",
				},
			},
			addr2.Hex(): {
				EOAAddress:  addr2.Hex(),
				SecretValue: [32]byte{4, 5, 6},
				Sign: utils.SignInfo{
					V: "27",
					R: "0x789",
					S: "0xabc",
				},
			},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		reavealOrderRepository: &mockRevealOrderRepo{},
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Should not submit when signature is incomplete
	ln.checkRoundsForCompletion(context.Background())
}

func TestLeaderNode_checkRoundsForCompletion_InvalidSignatureV(t *testing.T) {
	logger.InitLogger()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthService{operators: []common.Address{addr1, addr2}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		getByEOAData: map[string]*utils.LeaderCommitData{
			addr1.Hex(): {
				EOAAddress:  addr1.Hex(),
				SecretValue: [32]byte{1, 2, 3},
				Sign: utils.SignInfo{
					V: "invalid", // Invalid V value
					R: "0x123",
					S: "0x456",
				},
			},
			addr2.Hex(): {
				EOAAddress:  addr2.Hex(),
				SecretValue: [32]byte{4, 5, 6},
				Sign: utils.SignInfo{
					V: "27",
					R: "0x789",
					S: "0xabc",
				},
			},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		reavealOrderRepository: &mockRevealOrderRepo{},
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Should not submit when signature parsing fails
	ln.checkRoundsForCompletion(context.Background())
}

func TestLeaderNode_checkRoundsForCompletion_AllSubmitted_NoCvOnChain(t *testing.T) {
	logger.InitLogger()

	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthService{operators: []common.Address{addr1, addr2}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		getByEOAData: map[string]*utils.LeaderCommitData{
			addr1.Hex(): {
				EOAAddress:  addr1.Hex(),
				SecretValue: [32]byte{1, 2, 3},
				Sign: utils.SignInfo{
					V: "27",
					R: common.BigToHash(big.NewInt(1)).Hex(),
					S: common.BigToHash(big.NewInt(2)).Hex(),
				},
			},
			addr2.Hex(): {
				EOAAddress:  addr2.Hex(),
				SecretValue: [32]byte{4, 5, 6},
				Sign: utils.SignInfo{
					V: "28",
					R: common.BigToHash(big.NewInt(3)).Hex(),
					S: common.BigToHash(big.NewInt(4)).Hex(),
				},
			},
		},
	}

	mockRevealRepo := &mockRevealOrderRepo{
		order: &utils.RevealOrderData{
			Round:       "1",
			TrialNum:    "1",
			RevealOrder: []int{0, 1},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		reavealOrderRepository: mockRevealRepo,
		secretsOnChain:         make(map[string]bool),
		cvOnChain:              make(map[string]bool),
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Should trigger transaction
	ln.checkRoundsForCompletion(context.Background())

	// Verify transaction was called
	assert.Equal(t, 1, mockEth.execCalled)
	assert.Equal(t, "generateRandomNumber", mockEth.execLastMethod)
	assert.Equal(t, 1, mockRepo.updateRandomHit)
}

func TestLeaderNode_checkRoundsForCompletion_AllSubmitted_CvOnChain(t *testing.T) {
	logger.InitLogger()

	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthService{operators: []common.Address{addr1, addr2}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		getByEOAData: map[string]*utils.LeaderCommitData{
			addr1.Hex(): {
				EOAAddress:  addr1.Hex(),
				SecretValue: [32]byte{1, 2, 3},
				Sign: utils.SignInfo{
					V: "27",
					R: common.BigToHash(big.NewInt(1)).Hex(),
					S: common.BigToHash(big.NewInt(2)).Hex(),
				},
			},
			addr2.Hex(): {
				EOAAddress:  addr2.Hex(),
				SecretValue: [32]byte{4, 5, 6},
				Sign: utils.SignInfo{
					V: "28",
					R: common.BigToHash(big.NewInt(3)).Hex(),
					S: common.BigToHash(big.NewInt(4)).Hex(),
				},
			},
		},
	}

	mockRevealRepo := &mockRevealOrderRepo{
		order: &utils.RevealOrderData{
			Round:       "1",
			TrialNum:    "1",
			RevealOrder: []int{0, 1},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		reavealOrderRepository: mockRevealRepo,
		secretsOnChain:         make(map[string]bool),
		cvOnChain:              make(map[string]bool),
		indicesMutex:           sync.RWMutex{},
		indices:                make([]*big.Int, 0),
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Set cvOnChain to true
	uniqueKey := utils.GetUniqueKey("1", "1")
	ln.SetCvOnChain(uniqueKey, true)

	// Should trigger transaction with some CVs on chain
	ln.checkRoundsForCompletion(context.Background())

	// Verify transaction was called with the alternate method
	assert.Equal(t, 1, mockEth.execCalled)
	assert.Equal(t, "generateRandomNumberWhenSomeCvsAreOnChain", mockEth.execLastMethod)
	assert.Equal(t, 1, mockRepo.updateRandomHit)
}

func TestLeaderNode_checkRoundsForCompletion_SecretsAlreadyOnChain(t *testing.T) {
	logger.InitLogger()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthService{operators: []common.Address{addr1, addr2}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		getByEOAData: map[string]*utils.LeaderCommitData{
			addr1.Hex(): {
				EOAAddress:  addr1.Hex(),
				SecretValue: [32]byte{1, 2, 3},
				Sign: utils.SignInfo{
					V: "27",
					R: common.BigToHash(big.NewInt(1)).Hex(),
					S: common.BigToHash(big.NewInt(2)).Hex(),
				},
			},
			addr2.Hex(): {
				EOAAddress:  addr2.Hex(),
				SecretValue: [32]byte{4, 5, 6},
				Sign: utils.SignInfo{
					V: "28",
					R: common.BigToHash(big.NewInt(3)).Hex(),
					S: common.BigToHash(big.NewInt(4)).Hex(),
				},
			},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		secretsOnChain:         make(map[string]bool),
		cvOnChain:              make(map[string]bool),
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Set secretsOnChain to true
	uniqueKey := utils.GetUniqueKey("1", "1")
	ln.SetSecretsOnChain(uniqueKey, true)

	// Should not call transaction because secrets are already on chain
	ln.checkRoundsForCompletion(context.Background())

	// Verify no transaction was called
	assert.Equal(t, 0, mockEth.execCalled)
}

func TestLeaderNode_checkRoundsForCompletion_TransactionError(t *testing.T) {
	logger.InitLogger()

	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthServiceWithError{} // Returns errors
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		getByEOAData: map[string]*utils.LeaderCommitData{
			addr1.Hex(): {
				EOAAddress:  addr1.Hex(),
				SecretValue: [32]byte{1, 2, 3},
				Sign: utils.SignInfo{
					V: "27",
					R: common.BigToHash(big.NewInt(1)).Hex(),
					S: common.BigToHash(big.NewInt(2)).Hex(),
				},
			},
			addr2.Hex(): {
				EOAAddress:  addr2.Hex(),
				SecretValue: [32]byte{4, 5, 6},
				Sign: utils.SignInfo{
					V: "28",
					R: common.BigToHash(big.NewInt(3)).Hex(),
					S: common.BigToHash(big.NewInt(4)).Hex(),
				},
			},
		},
	}

	mockRevealRepo := &mockRevealOrderRepo{
		order: &utils.RevealOrderData{
			Round:       "1",
			TrialNum:    "1",
			RevealOrder: []int{0, 1},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		reavealOrderRepository: mockRevealRepo,
		secretsOnChain:         make(map[string]bool),
		cvOnChain:              make(map[string]bool),
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Should attempt transaction but fail
	ln.checkRoundsForCompletion(context.Background())

	// Transaction was attempted but updateRandomHit should be 0 due to error
	assert.Equal(t, 0, mockRepo.updateRandomHit)
}

func TestLeaderNode_checkRoundsForCompletion_CvOnChain_WithIndices(t *testing.T) {
	logger.InitLogger()

	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	addr3 := common.HexToAddress("0x3333333333333333333333333333333333333333")

	mockEth := &mockEthService{operators: []common.Address{addr1, addr2, addr3}}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		getByEOAData: map[string]*utils.LeaderCommitData{
			addr1.Hex(): {
				EOAAddress:  addr1.Hex(),
				SecretValue: [32]byte{1, 2, 3},
				Sign: utils.SignInfo{
					V: "27",
					R: common.BigToHash(big.NewInt(1)).Hex(),
					S: common.BigToHash(big.NewInt(2)).Hex(),
				},
			},
			addr2.Hex(): {
				EOAAddress:  addr2.Hex(),
				SecretValue: [32]byte{4, 5, 6},
				Sign: utils.SignInfo{
					V: "28",
					R: common.BigToHash(big.NewInt(3)).Hex(),
					S: common.BigToHash(big.NewInt(4)).Hex(),
				},
			},
			addr3.Hex(): {
				EOAAddress:  addr3.Hex(),
				SecretValue: [32]byte{7, 8, 9},
				Sign: utils.SignInfo{
					V: "27",
					R: common.BigToHash(big.NewInt(5)).Hex(),
					S: common.BigToHash(big.NewInt(6)).Hex(),
				},
			},
		},
	}

	mockRevealRepo := &mockRevealOrderRepo{
		order: &utils.RevealOrderData{
			Round:       "1",
			TrialNum:    "1",
			RevealOrder: []int{0, 1, 2},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		reavealOrderRepository: mockRevealRepo,
		secretsOnChain:         make(map[string]bool),
		cvOnChain:              make(map[string]bool),
		indices:                make([]*big.Int, 0),
		indicesMutex:           sync.RWMutex{},
	}

	ln.SetCurrentRound("1")
	ln.SetCurrentTrial("1")
	ln.SetHalted(false)

	// Set cvOnChain to true and add some indices
	uniqueKey := utils.GetUniqueKey("1", "1")
	ln.SetCvOnChain(uniqueKey, true)
	ln.AppendToIndices(big.NewInt(0)) // addr1's CV is on chain

	// Should skip addr1 when building signatures (covers lines 89-96)
	ln.checkRoundsForCompletion(context.Background())

	// Verify transaction was called
	assert.Equal(t, 1, mockEth.execCalled)
	assert.Equal(t, "generateRandomNumberWhenSomeCvsAreOnChain", mockEth.execLastMethod)
}

func TestLeaderNode_FetchActivatedOperators_Success(t *testing.T) {
	logger.InitLogger()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	mockEth := &mockEthService{
		operators: []common.Address{addr1, addr2},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ln := &LeaderNode{}

	result, err := ln.FetchActivatedOperators(context.Background(), nil, "1")

	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, addr1.Hex(), result[0])
	assert.Equal(t, addr2.Hex(), result[1])
}

func TestLeaderNode_FetchActivatedOperators_Error(t *testing.T) {
	logger.InitLogger()

	mockEth := &mockEthServiceWithError{}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ln := &LeaderNode{}

	result, err := ln.FetchActivatedOperators(context.Background(), nil, "1")

	// Should return error and empty result
	assert.Error(t, err)
	assert.Empty(t, result)
}

func TestLeaderNode_generateRandomNumberTransaction_Success(t *testing.T) {
	logger.InitLogger()

	// Set up environment
	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	mockEth := &mockEthService{}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRevealRepo := &mockRevealOrderRepo{
		order: &utils.RevealOrderData{
			Round:       "1",
			TrialNum:    "1",
			RevealOrder: []int{0, 1},
		},
	}

	ln := &LeaderNode{
		reavealOrderRepository: mockRevealRepo,
	}

	secrets := [][]byte{{1, 2, 3}, {4, 5, 6}}
	vs := []uint8{27, 28}
	rs := []common.Hash{common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2))}
	ss := []common.Hash{common.BigToHash(big.NewInt(3)), common.BigToHash(big.NewInt(4))}

	err := ln.generateRandomNumberTransaction(context.Background(), "1", "1", secrets, vs, rs, ss)

	assert.NoError(t, err)
	assert.Equal(t, 1, mockEth.execCalled)
	assert.Equal(t, "generateRandomNumber", mockEth.execLastMethod)
}

func TestLeaderNode_generateRandomNumberTransaction_InvalidPrivateKey(t *testing.T) {
	logger.InitLogger()

	os.Setenv("LEADER_PRIVATE_KEY", "invalid-key")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	ln := &LeaderNode{}

	secrets := [][]byte{{1, 2, 3}}
	vs := []uint8{27}
	rs := []common.Hash{common.BigToHash(big.NewInt(1))}
	ss := []common.Hash{common.BigToHash(big.NewInt(2))}

	err := ln.generateRandomNumberTransaction(context.Background(), "1", "1", secrets, vs, rs, ss)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create leader client")
}

func TestLeaderNode_generateRandomNumberTransaction_RevealOrderError(t *testing.T) {
	logger.InitLogger()

	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	mockRevealRepo := &mockRevealOrderRepo{
		err: errors.New("reveal order not found"),
	}

	ln := &LeaderNode{
		reavealOrderRepository: mockRevealRepo,
	}

	secrets := [][]byte{{1, 2, 3}}
	vs := []uint8{27}
	rs := []common.Hash{common.BigToHash(big.NewInt(1))}
	ss := []common.Hash{common.BigToHash(big.NewInt(2))}

	err := ln.generateRandomNumberTransaction(context.Background(), "1", "1", secrets, vs, rs, ss)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reveal order not found")
}

func TestLeaderNode_generateRandomNumberTransaction_ExecuteTransactionError(t *testing.T) {
	logger.InitLogger()

	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	// Use the mockEthServiceWithError which returns errors
	mockEth := &mockEthServiceWithError{}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRevealRepo := &mockRevealOrderRepo{
		order: &utils.RevealOrderData{
			Round:       "1",
			TrialNum:    "1",
			RevealOrder: []int{0, 1},
		},
	}

	ln := &LeaderNode{
		reavealOrderRepository: mockRevealRepo,
	}

	secrets := [][]byte{{1, 2, 3}}
	vs := []uint8{27}
	rs := []common.Hash{common.BigToHash(big.NewInt(1))}
	ss := []common.Hash{common.BigToHash(big.NewInt(2))}

	err := ln.generateRandomNumberTransaction(context.Background(), "1", "1", secrets, vs, rs, ss)

	// Should get error from ExecuteTransaction
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tx error")
}

func TestLeaderNode_generateRandomNumberTransactionSomeCvOnChain_Success(t *testing.T) {
	logger.InitLogger()

	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	mockEth := &mockEthService{}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRevealRepo := &mockRevealOrderRepo{
		order: &utils.RevealOrderData{
			Round:       "1",
			TrialNum:    "1",
			RevealOrder: []int{0, 1},
		},
	}

	ln := &LeaderNode{
		reavealOrderRepository: mockRevealRepo,
	}

	secrets := [][]byte{{1, 2, 3}, {4, 5, 6}}
	vs := []uint8{27, 28}
	rs := []common.Hash{common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2))}
	ss := []common.Hash{common.BigToHash(big.NewInt(3)), common.BigToHash(big.NewInt(4))}

	err := ln.generateRandomNumberTransactionSomeCvOnChain(context.Background(), "1", "1", secrets, vs, rs, ss)

	assert.NoError(t, err)
	assert.Equal(t, 1, mockEth.execCalled)
	assert.Equal(t, "generateRandomNumberWhenSomeCvsAreOnChain", mockEth.execLastMethod)
}

func TestLeaderNode_generateRandomNumberTransactionSomeCvOnChain_InvalidPrivateKey(t *testing.T) {
	logger.InitLogger()

	os.Setenv("LEADER_PRIVATE_KEY", "invalid-key")
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	ln := &LeaderNode{}

	secrets := [][]byte{{1, 2, 3}}
	vs := []uint8{27}
	rs := []common.Hash{common.BigToHash(big.NewInt(1))}
	ss := []common.Hash{common.BigToHash(big.NewInt(2))}

	err := ln.generateRandomNumberTransactionSomeCvOnChain(context.Background(), "1", "1", secrets, vs, rs, ss)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create leader client")
}

func TestLeaderNode_generateRandomNumberTransactionSomeCvOnChain_RevealOrderError(t *testing.T) {
	logger.InitLogger()

	pk, _ := ethcrypto.GenerateKey()
	os.Setenv("LEADER_PRIVATE_KEY", hex.EncodeToString(ethcrypto.FromECDSA(pk)))
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("LEADER_PRIVATE_KEY")
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	mockRevealRepo := &mockRevealOrderRepo{
		err: errors.New("reveal order not found"),
	}

	ln := &LeaderNode{
		reavealOrderRepository: mockRevealRepo,
	}

	secrets := [][]byte{{1, 2, 3}}
	vs := []uint8{27}
	rs := []common.Hash{common.BigToHash(big.NewInt(1))}
	ss := []common.Hash{common.BigToHash(big.NewInt(2))}

	err := ln.generateRandomNumberTransactionSomeCvOnChain(context.Background(), "1", "1", secrets, vs, rs, ss)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reveal order not found")
}

func TestLeaderNode_completeRound_Success(t *testing.T) {
	logger.InitLogger()

	mockRepo := &mockLeaderCommitRepo{}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		roundsData:             make(map[string]RoundData),
	}

	err := ln.completeRound(context.Background(), "1", "1")

	assert.NoError(t, err)
	assert.Equal(t, 1, mockRepo.updateRandomHit)

	// Verify round data was set
	data, exists := ln.GetRoundData("1")
	assert.True(t, exists)
	assert.True(t, data.RandomNumber)
}

func TestLeaderNode_completeRound_UpdateError(t *testing.T) {
	logger.InitLogger()

	mockRepo := &mockLeaderCommitRepo{}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
	}

	err := ln.completeRound(context.Background(), "1", "1")

	// Should not error with current mock
	assert.NoError(t, err)
}

func TestLeaderNode_completeRound_ExistingRoundData(t *testing.T) {
	logger.InitLogger()

	mockRepo := &mockLeaderCommitRepo{}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
		roundsData:             make(map[string]RoundData),
	}

	// Set existing round data
	ln.SetRoundData("1", RoundData{
		MerkleRoot:   true,
		RandomNumber: false,
	})

	err := ln.completeRound(context.Background(), "1", "1")

	assert.NoError(t, err)

	// Verify round data was updated
	data, exists := ln.GetRoundData("1")
	assert.True(t, exists)
	assert.True(t, data.RandomNumber)
	assert.True(t, data.MerkleRoot) // Should preserve existing value
}

func TestLeaderNode_LoadNodeData_LoadError(t *testing.T) {
	logger.InitLogger()

	mockRepo := &mockLeaderCommitRepo{
		getErr: errors.New("database error"),
	}

	mockEth := &mockEthService{}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
	}

	// Should handle error gracefully and return empty slices
	cvs, cos, secrets, vs, rs, ss := ln.LoadNodeData(context.Background(), "1", "1")

	assert.Empty(t, cvs)
	assert.Empty(t, cos)
	assert.Empty(t, secrets)
	assert.Empty(t, vs)
	assert.Empty(t, rs)
	assert.Empty(t, ss)
}

func TestLeaderNode_LoadNodeData_ParseError(t *testing.T) {
	logger.InitLogger()

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")

	mockEth := &mockEthService{
		operators: []common.Address{addr1},
	}
	originalService := eth.Service
	eth.Service = mockEth
	defer func() { eth.Service = originalService }()

	mockRepo := &mockLeaderCommitRepo{
		byRoundTrial: []*utils.LeaderCommitData{
			{
				EOAAddress:  addr1.Hex(),
				SecretValue: [32]byte{1, 2, 3},
				Cvs:         [32]byte{4, 5, 6},
				Cos:         [32]byte{7, 8, 9},
				Sign: utils.SignInfo{
					V: "invalid", // Will cause parse error
					R: "0x123",
					S: "0x456",
				},
			},
		},
	}

	ln := &LeaderNode{
		leaderCommitRepository: mockRepo,
	}

	cvs, cos, secrets, vs, rs, ss := ln.LoadNodeData(context.Background(), "1", "1")

	// Should still process but with v = 0 for invalid parse
	assert.Len(t, cvs, 1)
	assert.Len(t, cos, 1)
	assert.Len(t, secrets, 1)
	assert.Len(t, vs, 1)
	assert.Equal(t, uint8(0), vs[0]) // Default value for parse error
	assert.Len(t, rs, 1)
	assert.Len(t, ss, 1)
}

func TestMain(m *testing.M) {
	setupTestEnvironment()

	// Run tests
	code := m.Run()

	// Cleanup
	cleanupTestEnvironment()

	os.Exit(code)
}

func setupTestEnvironment() {
	err := os.MkdirAll("contract/abi", 0755)
	if err != nil {
		log.Fatal("Failed to create test directory:", err)
	}

	err = os.WriteFile("contract/abi/Commit2RevealDRB.json", []byte(testABIContent), 0644)
	if err != nil {
		log.Fatal("Failed to create test ABI file:", err)
	}

	log.Println("✓ Test ABI file created for leader tests")
}

func cleanupTestEnvironment() {
	// Remove test ABI file
	os.Remove("contract/abi/Commit2RevealDRB.json")
	log.Println("✓ Test environment cleaned up for leader tests")
}

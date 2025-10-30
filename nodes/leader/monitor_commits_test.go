package leader_node

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// ===================== Mocks =====================

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

func (n *noopNodeInfoRepo) AddNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	return nil
}
func (n *noopNodeInfoRepo) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	return nil, nil
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
	return new(types.Transaction), nil, nil
}
func (m *mockEthService) UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	return big.NewInt(1), nil
}
func (m *mockEthService) GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	return big.NewInt(1), nil
}

// Ensure mock satisfies the interface
var _ eth.IEthService = (*mockEthService)(nil)

// ===================== Suite =====================

type MonitorCommitsSuite struct {
	suite.Suite
	ctx     context.Context
	ln      *LeaderNode
	ethMock *mockEthService
	lcRepo  *mockLeaderCommitRepo
	roRepo  *mockRevealOrderRepo
}

func (s *MonitorCommitsSuite) SetupTest() {
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

// ---------------- Helper construction ----------------

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

// ===================== Tests =====================

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
	s.Equal([32]byte{1}, cvs[0])
	s.Equal([32]byte{2}, cos[0])
}

func (s *MonitorCommitsSuite) Test_checkRoundsForCompletion_AllEOAsSubmitted_TriggersTxAndCompletion() {
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	s.ethMock.SetActivatedOperatorsCached([]common.Address{addr1, addr2})
	// Both with secrets and full signatures
	lc1 := makeLeaderCommit(addr1, true, "27", common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2)))
	lc2 := makeLeaderCommit(addr2, true, "28", common.BigToHash(big.NewInt(3)), common.BigToHash(big.NewInt(4)))
	s.lcRepo.getByEOAData = map[string]*utils.LeaderCommitData{
		addr1.Hex(): lc1,
		addr2.Hex(): lc2,
	}
	s.roRepo.order = &utils.RevealOrderData{RevealOrder: []int{0, 1}}

	// Ensure neither secrets nor CVs considered on-chain
	uniqueKey := utils.GetUniqueKey("1", "1")
	_, _ = s.ln.GetCvOnChain(uniqueKey)
	_, _ = s.ln.GetSecretsOnChain(uniqueKey)

	s.ln.checkRoundsForCompletion(s.ctx)

	s.Equal(1, s.ethMock.execCalled)
	s.True(s.lcRepo.updateRandomHit > 0)
	data, ok := s.ln.GetRoundData("1")
	s.True(ok)
	s.True(data.RandomNumber)
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

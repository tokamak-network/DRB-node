package leader_node

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/suite"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

// mock repository for node info
type mockNodeInfoRepo struct {
	addCalled int
	addArg    *utils.NodeInfo
	addErr    error
}

func (m *mockNodeInfoRepo) AddAndUpdateNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	m.addCalled++
	m.addArg = nodeInfo
	return m.addErr
}

func (m *mockNodeInfoRepo) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	return nil, nil
}

func (m *mockNodeInfoRepo) DeleteNodeInfoByEOA(ctx context.Context, eoa string) error { return nil }

// Suite
type RegistrationHelperSuite struct {
	suite.Suite
	repo *mockNodeInfoRepo
	ln   *LeaderNode
	ctx  context.Context
}

func (s *RegistrationHelperSuite) SetupTest() {
	// Initialize logger
	logger.InitLogger()

	s.repo = &mockNodeInfoRepo{}
	s.ln = &LeaderNode{nodeInfoRepository: s.repo}
	s.ctx = context.Background()
	// reset activated operators cache before each test
	eth.Service.SetActivatedOperatorsCached(nil)
}

func (s *RegistrationHelperSuite) Test_RegisterNode_Success() {
	// generate EOA and signature
	pk, _ := ethcrypto.GenerateKey()
	addr := ethcrypto.PubkeyToAddress(pk.PublicKey).Hex()
	sig := utils.SignData(addr, pk)

	// mark as activated
	eth.Service.SetActivatedOperatorsCached([]common.Address{common.HexToAddress(addr)})

	req := utils.RegistrationRequest{EOAAddress: addr, Signature: sig, PeerID: "peer-1"}
	err := s.ln.registerNodeInternal(s.ctx, req, "/ip4/10.0.0.1/tcp/7000")
	s.Require().NoError(err)
	s.Equal(1, s.repo.addCalled)
	s.Equal("10.0.0.1", s.repo.addArg.IP)
	s.Equal("7000", s.repo.addArg.Port)
	s.Equal(addr, s.repo.addArg.EOAAddress)
	s.Equal("peer-1", s.repo.addArg.PeerID)
}

func (s *RegistrationHelperSuite) Test_RegisterNode_InvalidJSON() {
	// stream with invalid json
	err := s.ln.registerNodeInternal(s.ctx, utils.RegistrationRequest{}, "/ip4/127.0.0.1/tcp/9000")
	s.Error(err)
	s.Contains(err.Error(), "failed to verify signature")
}

func (s *RegistrationHelperSuite) Test_RegisterNode_InvalidSignature() {
	pk, _ := ethcrypto.GenerateKey()
	addr := ethcrypto.PubkeyToAddress(pk.PublicKey).Hex()
	// invalid signature: random bytes
	sig := []byte("invalid")
	eth.Service.SetActivatedOperatorsCached([]common.Address{common.HexToAddress(addr)})
	req := utils.RegistrationRequest{EOAAddress: addr, Signature: sig, PeerID: "p"}
	err := s.ln.registerNodeInternal(s.ctx, req, "/ip4/1.2.3.4/tcp/1234")
	s.Error(err)
	s.Contains(err.Error(), "failed to verify signature for PeerID: p")
}

func (s *RegistrationHelperSuite) Test_RegisterNode_EOA_NotActivated() {
	pk, _ := ethcrypto.GenerateKey()
	addr := ethcrypto.PubkeyToAddress(pk.PublicKey).Hex()
	sig := utils.SignData(addr, pk)
	// do not set activated operators (empty)
	req := utils.RegistrationRequest{EOAAddress: addr, Signature: sig, PeerID: "p"}
	err := s.ln.registerNodeInternal(s.ctx, req, "/ip4/1.1.1.1/tcp/3030")
	s.Error(err)
	s.Contains(err.Error(), "is not activated, registration denied")
}

func (s *RegistrationHelperSuite) Test_RegisterNode_InvalidRemoteAddr() {
	pk, _ := ethcrypto.GenerateKey()
	addr := ethcrypto.PubkeyToAddress(pk.PublicKey).Hex()
	sig := utils.SignData(addr, pk)
	eth.Service.SetActivatedOperatorsCached([]common.Address{common.HexToAddress(addr)})

	req := utils.RegistrationRequest{EOAAddress: addr, Signature: sig, PeerID: "p"}
	// invalid multiaddr (too short path parts)
	err := s.ln.registerNodeInternal(s.ctx, req, "/ip4/10.0.0.1")
	s.Error(err)
	s.Contains(err.Error(), "invalid remote address format")
}

var _ database.INodeInfoRepository = (*mockNodeInfoRepo)(nil)

func TestRegistrationHelperSuite(t *testing.T) {
	suite.Run(t, new(RegistrationHelperSuite))
}

func (s *RegistrationHelperSuite) Test_RegisterNode_Function_Success() {
	pk, _ := ethcrypto.GenerateKey()
	addr := ethcrypto.PubkeyToAddress(pk.PublicKey).Hex()
	sig := utils.SignData(addr, pk)

	eth.Service.SetActivatedOperatorsCached([]common.Address{common.HexToAddress(addr)})
	req := utils.RegistrationRequest{EOAAddress: addr, Signature: sig, PeerID: "peer-X"}
	payload, _ := json.Marshal(req)

	// mocknet setup
	mn := mocknet.New()
	h1, _ := mn.GenPeer()
	h2, _ := mn.GenPeer()
	_ = mn.LinkAll()
	_ = mn.ConnectAllButSelf()

	done := make(chan error, 1)
	proto := protocol.ID("/reg/1.0.0")
	// handler on receiver calls RegisterNode
	h2.SetStreamHandler(proto, func(st network.Stream) {
		// ensure leader has no on-chain side-effects
		s.ln.fallbackEthClient = nil
		err := s.ln.RegisterNode(s.ctx, st, "")
		done <- err
	})

	// open stream and write request
	st, err := h1.NewStream(s.ctx, h2.ID(), proto)
	s.Require().NoError(err)
	_, _ = st.Write(payload)
	_ = st.CloseWrite()

	// wait for handler
	select {
	case err := <-done:
		s.Require().NoError(err)
	case <-time.After(2 * time.Second):
		s.Fail("timeout waiting for RegisterNode handler")
	}

	// repository assertions
	s.Equal(1, s.repo.addCalled)
	s.NotEmpty(s.repo.addArg.IP)
	s.NotEmpty(s.repo.addArg.Port)
	s.Equal(addr, s.repo.addArg.EOAAddress)
	s.Equal("peer-X", s.repo.addArg.PeerID)
}

func (s *RegistrationHelperSuite) Test_RegisterNode_Function_InvalidJSON() {
	mn := mocknet.New()
	h1, _ := mn.GenPeer()
	h2, _ := mn.GenPeer()
	_ = mn.LinkAll()
	_ = mn.ConnectAllButSelf()

	done := make(chan error, 1)
	proto := protocol.ID("/reg/1.0.0")
	h2.SetStreamHandler(proto, func(st network.Stream) {
		s.ln.fallbackEthClient = nil
		err := s.ln.RegisterNode(s.ctx, st, "")
		done <- err
	})

	st, err := h1.NewStream(s.ctx, h2.ID(), proto)
	s.Require().NoError(err)
	_, _ = st.Write([]byte("{")) // invalid JSON
	_ = st.CloseWrite()

	select {
	case err := <-done:
		s.Error(err)
		s.Contains(err.Error(), "failed to decode registration request")
	case <-time.After(2 * time.Second):
		s.Fail("timeout waiting for RegisterNode handler")
	}
}

func (s *RegistrationHelperSuite) Test_RegisterNode_Function_InvalidSignature() {
	pk, _ := ethcrypto.GenerateKey()
	addr := ethcrypto.PubkeyToAddress(pk.PublicKey).Hex()
	req := utils.RegistrationRequest{EOAAddress: addr, Signature: []byte("bad"), PeerID: "p"}
	payload, _ := json.Marshal(req)

	mn := mocknet.New()
	h1, _ := mn.GenPeer()
	h2, _ := mn.GenPeer()
	_ = mn.LinkAll()
	_ = mn.ConnectAllButSelf()

	done := make(chan error, 1)
	proto := protocol.ID("/reg/1.0.0")
	h2.SetStreamHandler(proto, func(st network.Stream) {
		s.ln.fallbackEthClient = nil
		err := s.ln.RegisterNode(s.ctx, st, "")
		done <- err
	})

	st, err := h1.NewStream(s.ctx, h2.ID(), proto)
	s.Require().NoError(err)
	_, _ = st.Write(payload)
	_ = st.CloseWrite()

	select {
	case err := <-done:
		s.Error(err)
		s.Contains(err.Error(), "failed to verify signature for PeerID: p")
	case <-time.After(2 * time.Second):
		s.Fail("timeout waiting for RegisterNode handler")
	}
}

func (s *RegistrationHelperSuite) Test_RegisterNode_Function_EOA_NotActivated() {
	pk, _ := ethcrypto.GenerateKey()
	addr := ethcrypto.PubkeyToAddress(pk.PublicKey).Hex()
	sig := utils.SignData(addr, pk)
	req := utils.RegistrationRequest{EOAAddress: addr, Signature: sig, PeerID: "p"}
	payload, _ := json.Marshal(req)

	mn := mocknet.New()
	h1, _ := mn.GenPeer()
	h2, _ := mn.GenPeer()
	_ = mn.LinkAll()
	_ = mn.ConnectAllButSelf()

	done := make(chan error, 1)
	proto := protocol.ID("/reg/1.0.0")
	h2.SetStreamHandler(proto, func(st network.Stream) {
		s.ln.fallbackEthClient = nil
		err := s.ln.RegisterNode(s.ctx, st, "")
		done <- err
	})

	st, err := h1.NewStream(s.ctx, h2.ID(), proto)
	s.Require().NoError(err)
	_, _ = st.Write(payload)
	_ = st.CloseWrite()

	select {
	case err := <-done:
		s.Error(err)
		s.Contains(err.Error(), "is not activated, registration denied")
	case <-time.After(2 * time.Second):
		s.Fail("timeout waiting for RegisterNode handler")
	}
}

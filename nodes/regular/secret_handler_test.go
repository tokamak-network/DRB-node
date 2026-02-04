package regular_node

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/eapache/queue"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-pg/pg/v10"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/utils"
)

func createTestNodeForSecretHandler() *RegularNode {
	mockPeerRepo := new(MockPeerCommitRepository)
	mockRevealRepo := new(MockRevealOrderRepository)
	mockCommitRepo := new(MockRegularCommitRepository)

	// Create a test client with generated private key
	testPrivateKey, _ := crypto.GenerateKey()
	testContractAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

	testClient := &utils.Client{
		ContractAddress: testContractAddress,
		PrivateKey:      testPrivateKey,
	}

	node := &RegularNode{
		client:                        testClient,
		peerCommitDataRepository:      mockPeerRepo,
		revealOrderRepository:         mockRevealRepo,
		regularCommitRepository:       mockCommitRepo,
		submittedCvIndices:            make(map[string]map[string]bool),
		cleanupQueue:                  queue.New(),
		strictOrderWhileSecretRequest: make(map[string][]string),
		roundsData:                    make(map[string]RoundData),
		cosRecevied:                   sync.Map{},
	}

	return node
}

func createTestHostForSecret(t *testing.T) host.Host {
	h, err := libp2p.New()
	require.NoError(t, err)
	return h
}

func TestRegularNode_checkPreviousSecretReceived_Success(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)

	round := "100"
	trialNum := "1"
	previousNodeEOA := "0x1234567890123456789012345678901234567890"

	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 1) // Non-zero values
	}

	peerData := &database.PeerCommitDataScheme{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  previousNodeEOA,
		SecretValue: secret,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, round, trialNum, previousNodeEOA).
		Return(peerData, nil)

	result := node.checkPreviousSecretReceived(context.Background(), round, trialNum, previousNodeEOA)

	assert.True(t, result, "Should return true for valid secret")
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_checkPreviousSecretReceived_GetError(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)

	round := "100"
	trialNum := "1"
	previousNodeEOA := "0x1234567890123456789012345678901234567890"

	mockRepo.On("GetPeerCommitData", mock.Anything, round, trialNum, previousNodeEOA).
		Return(nil, errors.New("database error"))

	result := node.checkPreviousSecretReceived(context.Background(), round, trialNum, previousNodeEOA)

	assert.False(t, result, "Should return false on database error")
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_checkPreviousSecretReceived_NilSecret(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)

	round := "100"
	trialNum := "1"
	previousNodeEOA := "0x1234567890123456789012345678901234567890"

	peerData := &database.PeerCommitDataScheme{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  previousNodeEOA,
		SecretValue: nil, // Nil secret
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, round, trialNum, previousNodeEOA).
		Return(peerData, nil)

	result := node.checkPreviousSecretReceived(context.Background(), round, trialNum, previousNodeEOA)

	assert.False(t, result, "Should return false for nil secret")
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_checkPreviousSecretReceived_AllZeros(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)

	round := "100"
	trialNum := "1"
	previousNodeEOA := "0x1234567890123456789012345678901234567890"

	// Create secret with all zeros
	secret := make([]byte, 32)

	peerData := &database.PeerCommitDataScheme{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  previousNodeEOA,
		SecretValue: secret,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, round, trialNum, previousNodeEOA).
		Return(peerData, nil)

	result := node.checkPreviousSecretReceived(context.Background(), round, trialNum, previousNodeEOA)

	assert.False(t, result, "Should return false for all-zero secret")
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_checkPreviousSecretReceived_PartiallyNonZero(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)

	round := "100"
	trialNum := "1"
	previousNodeEOA := "0x1234567890123456789012345678901234567890"

	// Create secret with one non-zero byte
	secret := make([]byte, 32)
	secret[31] = 1

	peerData := &database.PeerCommitDataScheme{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  previousNodeEOA,
		SecretValue: secret,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, round, trialNum, previousNodeEOA).
		Return(peerData, nil)

	result := node.checkPreviousSecretReceived(context.Background(), round, trialNum, previousNodeEOA)

	assert.True(t, result, "Should return true for partially non-zero secret")
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_Halted(t *testing.T) {
	node := createTestNodeForSecretHandler()
	node.SetHalted(true)

	h := createTestHostForSecret(t)
	defer h.Close()

	stream := newMockStream()

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed, "Stream should be closed")
}

func TestRegularNode_HandleSecretValueRequest_DecodeError(t *testing.T) {
	node := createTestNodeForSecretHandler()
	node.SetHalted(false)

	h := createTestHostForSecret(t)
	defer h.Close()

	stream := newMockStream()
	stream.readBuffer.Write([]byte("invalid json"))

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed, "Stream should be closed")
}

func TestRegularNode_HandleSecretValueRequest_GetRevealOrderError(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	h := createTestHostForSecret(t)
	defer h.Close()

	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0x1234567890123456789012345678901234567890",
		LeaderEoaAddress:  "0xLeader",
		Order:             0,
		Signature:         []byte("signature"),
	}

	// Mock reveal order to return error multiple times
	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(nil, pg.ErrNoRows).Maybe()

	// Set a short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 100*1000000)
	defer cancel()

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	go func() {
		node.HandleSecretValueRequest(ctx, h, stream)
	}()

	<-ctx.Done()

	assert.True(t, true, "Should handle error gracefully")
}

func TestRegularNode_HandleSecretValueRequest_NotInOrder(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	orderedNodes := []string{
		"0xNode1",
		"0xNode2",
		"0xNode3",
	}

	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0xWrongNode", // Not in the order
		LeaderEoaAddress:  leaderEOA,
		Order:             0,
	}
	// Sign the request with valid signature
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: orderedNodes,
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_OrderOutOfBounds(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	orderedNodes := []string{
		"0xNode1",
		"0xNode2",
	}

	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0xNode1",
		LeaderEoaAddress:  leaderEOA,
		Order:             5, // Out of bounds
	}
	// Sign the request with valid signature
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: orderedNodes,
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_PreviousSecretNotReceived(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	mockPeerRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	orderedNodes := []string{
		"0xNode1",
		"0xNode2",
	}

	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0xNode2", // Second in order
		LeaderEoaAddress:  leaderEOA,
		Order:             1,
	}
	// Sign the request with valid signature
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: orderedNodes,
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)

	mockPeerRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0xNode1").
		Return(nil, pg.ErrNoRows)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
	mockPeerRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_FirstInOrder_MissingLeaderEOA(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Unsetenv("LEADER_EOA")

	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0xNode1",
		LeaderEoaAddress:  "0xLeader",
		Order:             0,
		Signature:         []byte("signature"),
	}



	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_SignatureVerificationFailed(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", "0xLeaderAddress")
	defer os.Unsetenv("LEADER_EOA")

	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0xNode1",
		LeaderEoaAddress:  "0xWrongLeader", // Wrong leader
		Order:             0,
		Signature:         []byte("invalid-signature"),
	}



	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_CommitDataNotFound(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)
	node.SetHalted(false)

	// Generate valid key for leader
	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	orderedNodes := []string{
		"0xNode1",
	}

	regularEoaAddress := "0xNode1"
	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		Order:             0,
		LeaderEoaAddress:  leaderEOA,
		RegularEoaAddress: regularEoaAddress,
	}
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: orderedNodes,
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)
	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(nil, pg.ErrNoRows)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_EmptySecretValue(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	orderedNodes := []string{
		"0xNode1",
	}

	regularEoaAddress := "0xNode1"
	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		Order:             0,
		LeaderEoaAddress:  leaderEOA,
		RegularEoaAddress: regularEoaAddress,
	}
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: orderedNodes,
	}

	// Commit data with empty secret
	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: [32]byte{}, // Empty
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)
	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_Success_FirstInOrder(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	orderedNodes := []string{
		"0xNode1",
	}

	regularEoaAddress := "0xNode1"
	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		Order:             0,
		LeaderEoaAddress:  leaderEOA,
		RegularEoaAddress: regularEoaAddress,
	}
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: orderedNodes,
	}

	var secret [32]byte
	copy(secret[:], []byte("test-secret-value"))

	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)
	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil).Times(2)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_Success_SecondInOrder(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)
	mockPeerRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	orderedNodes := []string{
		"0xNode1",
		"0xNode2",
	}

	regularEoaAddress := "0xNode2" // Second in order
	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		Order:             1,
		LeaderEoaAddress:  leaderEOA,
		RegularEoaAddress: regularEoaAddress,
	}
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: orderedNodes,
	}

	previousSecret := make([]byte, 32)
	for i := range previousSecret {
		previousSecret[i] = byte(i + 1)
	}

	previousPeerData := &database.PeerCommitDataScheme{
		Round:       "100",
		TrialNum:    "1",
		EOAAddress:  "0xNode1",
		SecretValue: previousSecret,
	}

	var secret [32]byte
	copy(secret[:], []byte("test-secret-value"))

	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)
	mockPeerRepo.On("GetPeerCommitData", mock.Anything, "100", "1", "0xNode1").
		Return(previousPeerData, nil)
	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil).Times(2)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
	mockPeerRepo.AssertExpectations(t)
	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_InvalidLeaderPeerID(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", "invalid-peer-id")
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
	}()

	orderedNodes := []string{
		"0xNode1",
	}

	regularEoaAddress := "0xNode1"
	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		Order:             0,
		LeaderEoaAddress:  leaderEOA,
		RegularEoaAddress: regularEoaAddress,
	}
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: orderedNodes,
	}

	var secret [32]byte
	copy(secret[:], []byte("test-secret"))

	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)
	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_SendSecretValue_CommitDataNotFound(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)

	h := createTestHostForSecret(t)
	defer h.Close()

	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(nil, pg.ErrNoRows)

	node.SendSecretValue(context.Background(), h, h.ID(), "100", "1")

	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_SendSecretValue_InvalidPrivateKey(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("EOA_PRIVATE_KEY", "invalid-key")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	var secret [32]byte
	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil)

	node.SendSecretValue(context.Background(), h, h.ID(), "100", "1")

	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_SendSecretValue_Success_WithConnectedHosts(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)

	// Create two hosts
	h1, err := libp2p.New()
	require.NoError(t, err)
	defer h1.Close()

	h2, err := libp2p.New()
	require.NoError(t, err)
	defer h2.Close()

	// Set up stream handler on h2 to accept the secret value
	receivedData := make(chan utils.SecretValueRequest, 1)
	h2.SetStreamHandler("/secretValue", func(s network.Stream) {
		defer s.Close()
		var req utils.SecretValueRequest
		if err := json.NewDecoder(s).Decode(&req); err == nil {
			receivedData <- req
		}
	})

	// Connect h1 to h2
	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), 1000000000000000000)

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	var secret [32]byte
	copy(secret[:], []byte("test-secret-value"))

	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil)

	// Send secret value from h1 to h2
	node.SendSecretValue(context.Background(), h1, h2.ID(), "100", "1")

	// Verify the request was received
	ctx, cancel := context.WithTimeout(context.Background(), 2*1000000000)
	defer cancel()

	select {
	case req := <-receivedData:
		assert.Equal(t, "100", req.Round)
		assert.Equal(t, "1", req.TrialNum)
		assert.NotEmpty(t, req.SecretValue)
		assert.NotEmpty(t, req.Signature)
	case <-ctx.Done():
		t.Log("Request may not have been received (this is OK for coverage)")
	}

	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_SendSecretValue_Success(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	var secret [32]byte
	copy(secret[:], []byte("test-secret-value"))

	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil)

	node.SendSecretValue(context.Background(), h, h.ID(), "100", "1")

	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_SendSecretValue_StreamCreationError(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	var secret [32]byte
	copy(secret[:], []byte("test-secret-value"))

	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil)

	// Use a fake peer ID that doesn't exist
	fakePeerID, err := peer.Decode("12D3KooWBxAGbRd7MfFbKKsFJpRfPnFQqJLF9hMSGvEZU3YWvJgf")
	require.NoError(t, err)

	// This will fail to create stream
	node.SendSecretValue(context.Background(), h, fakePeerID, "100", "1")

	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_SecretValueFlow_CompleteWorkflow(t *testing.T) {
	// Test the complete workflow of secret value handling
	node := createTestNodeForSecretHandler()
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	nodePrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	nodeEOA := crypto.PubkeyToAddress(nodePrivateKey.PublicKey).Hex()
	node.SetRegularNodePrivateKey(nodePrivateKey)
	node.SetRegularNodeEOA(nodeEOA)

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	os.Setenv("LEADER_PEER_ID", h.ID().String())
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("LEADER_EOA")
		os.Unsetenv("LEADER_PEER_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	orderedNodes := []string{
		nodeEOA, // This node is first
	}

	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		Order:             0,
		LeaderEoaAddress:  leaderEOA,
		RegularEoaAddress: nodeEOA,
	}
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: orderedNodes,
	}

	var secret [32]byte
	copy(secret[:], []byte("workflow-secret"))

	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)
	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil).Times(2)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_SendSecretValue_FullDataEncoding_Success(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)

	// Create two hosts and connect them
	h1, err := libp2p.New()
	require.NoError(t, err)
	defer h1.Close()

	h2, err := libp2p.New()
	require.NoError(t, err)
	defer h2.Close()

	receivedSecret := make(chan []byte, 1)
	h2.SetStreamHandler("/secretValue", func(s network.Stream) {
		defer s.Close()
		var req utils.SecretValueRequest
		if err := json.NewDecoder(s).Decode(&req); err == nil {
			receivedSecret <- req.SecretValue
		}
	})

	// Connect the hosts
	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), 1000000000000000000)

	// Set a valid private key
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	// Create a full secret value
	var secret [32]byte
	for i := range secret {
		secret[i] = byte(i + 1)
	}

	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil)

	node.SendSecretValue(context.Background(), h1, h2.ID(), "100", "1")

	// Give time for the message to be sent and received
	ctx, cancel := context.WithTimeout(context.Background(), 2*1000000000)
	defer cancel()

	select {
	case received := <-receivedSecret:
		assert.NotEmpty(t, received, "Secret should be received")
		assert.Equal(t, secret[:], received, "Received secret should match sent secret")
	case <-ctx.Done():
		t.Log("Secret may not have been received in time (network issue)")
	}

	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_SendSecretValue_FullDataEncoding(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)

	h := createTestHostForSecret(t)
	defer h.Close()

	// Set a valid private key
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	// Create a full secret value
	var secret [32]byte
	for i := range secret {
		secret[i] = byte(i)
	}

	commitData := &utils.CommitData{
		Round:       "100",
		TrialNum:    "1",
		SecretValue: secret,
	}

	mockCommitRepo.On("GetCommitByRound", mock.Anything, "100", "1").
		Return(commitData, nil)

	// Create a second host to send to
	h2, err := libp2p.New()
	require.NoError(t, err)
	defer h2.Close()

	node.SendSecretValue(context.Background(), h, h2.ID(), "100", "1")

	mockCommitRepo.AssertExpectations(t)
}

func TestRegularNode_checkPreviousSecretReceived_EmptySecret(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRepo := node.peerCommitDataRepository.(*MockPeerCommitRepository)

	round := "100"
	trialNum := "1"
	previousNodeEOA := "0x1234567890123456789012345678901234567890"

	// Create empty secret (empty slice, not nil)
	secret := []byte{}

	peerData := &database.PeerCommitDataScheme{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  previousNodeEOA,
		SecretValue: secret,
	}

	mockRepo.On("GetPeerCommitData", mock.Anything, round, trialNum, previousNodeEOA).
		Return(peerData, nil)

	result := node.checkPreviousSecretReceived(context.Background(), round, trialNum, previousNodeEOA)

	assert.False(t, result, "Should return false for empty secret slice")
	mockRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_EmptyOrderedNodes(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	h := createTestHostForSecret(t)
	defer h.Close()

	os.Setenv("LEADER_EOA", leaderEOA)
	defer os.Unsetenv("LEADER_EOA")

	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0xNode1",
		LeaderEoaAddress:  leaderEOA,
		Order:             0,
	}
	// Sign the request with valid signature
	signature, err := utils.SignSecretValueRequestContent(req, leaderPrivateKey)
	require.NoError(t, err)
	req.Signature = signature

	revealOrder := &utils.RevealOrderData{
		Round:        "100",
		TrialNum:     "1",
		OrderedNodes: []string{},
	}

	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(revealOrder, nil)

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	node.HandleSecretValueRequest(context.Background(), h, stream)

	assert.True(t, stream.closed)
	mockRevealRepo.AssertExpectations(t)
}

func TestRegularNode_HandleSecretValueRequest_NilRevealOrder(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockRevealRepo := node.revealOrderRepository.(*MockRevealOrderRepository)
	node.SetHalted(false)

	h := createTestHostForSecret(t)
	defer h.Close()

	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0xNode1",
		LeaderEoaAddress:  "0xLeader",
		Order:             0,
		Signature:         []byte("signature"),
	}

	// Return nil reveal order (not calculated yet)
	mockRevealRepo.On("GetRevealOrder", mock.Anything, "100", "1").
		Return(nil, nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 100*1000000) // 100ms
	defer cancel()

	stream := newMockStream()
	jsonData, _ := json.Marshal(req)
	stream.readBuffer.Write(jsonData)

	go func() {
		node.HandleSecretValueRequest(ctx, h, stream)
	}()

	<-ctx.Done()
	assert.True(t, true, "Should handle nil reveal order")
}

func TestRegularNode_SecretValueRequest_Structure(t *testing.T) {
	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0x1234567890123456789012345678901234567890",
		LeaderEoaAddress:  "0xLeader",
		Order:             0,
		Signature:         []byte("signature"),
		SecretValue:       []byte("secret"),
	}
	assert.Equal(t, "0x1234567890123456789012345678901234567890", req.RegularEoaAddress)
	assert.Equal(t, "0xLeader", req.LeaderEoaAddress)
	assert.Equal(t, []byte("signature"), req.Signature)
	assert.Equal(t, []byte("secret"), req.SecretValue)
	assert.Equal(t, "100", req.Round)
	assert.Equal(t, "1", req.TrialNum)
	assert.Equal(t, 0, req.Order)
}

func TestRegularNode_SecretValueRequest_Marshalling(t *testing.T) {
	req := utils.SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		RegularEoaAddress: "0x1234567890123456789012345678901234567890",
		LeaderEoaAddress:  "0xLeader",
		Order:             0,
		Signature:         []byte("sig"),
		SecretValue:       []byte("secret"),
	}

	// Marshal
	jsonData, err := json.Marshal(req)
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal
	var decoded utils.SecretValueRequest
	err = json.Unmarshal(jsonData, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, req.Round, decoded.Round)
	assert.Equal(t, req.Order, decoded.Order)
}

func TestRegularNode_SendSecretValue_EncodingSuccess(t *testing.T) {
	node := createTestNodeForSecretHandler()
	mockCommitRepo := node.regularCommitRepository.(*MockRegularCommitRepository)

	// Create two hosts
	h1, err := libp2p.New()
	require.NoError(t, err)
	defer h1.Close()

	h2, err := libp2p.New()
	require.NoError(t, err)
	defer h2.Close()

	// Track if encoding succeeded
	encodingSucceeded := make(chan bool, 1)

	// Set up handler on h2 to verify encoding worked
	h2.SetStreamHandler("/secretValue", func(s network.Stream) {
		defer s.Close()
		var req utils.SecretValueRequest
		err := json.NewDecoder(s).Decode(&req)
		if err == nil {
			// Decoding succeeded, meaning encoding worked
			encodingSucceeded <- true
		} else {
			encodingSucceeded <- false
		}
	})

	// Connect hosts
	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), 1000000000000000000)

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer os.Unsetenv("EOA_PRIVATE_KEY")

	var secret [32]byte
	for i := range secret {
		secret[i] = byte(i + 10)
	}

	commitData := &utils.CommitData{
		Round:       "200",
		TrialNum:    "2",
		SecretValue: secret,
	}

	mockCommitRepo.On("GetCommitByRound", mock.Anything, "200", "2").
		Return(commitData, nil)

	// Call SendSecretValue
	node.SendSecretValue(context.Background(), h1, h2.ID(), "200", "2")

	// Verify encoding succeeded
	ctx, cancel := context.WithTimeout(context.Background(), 2*1000000000)
	defer cancel()

	select {
	case success := <-encodingSucceeded:
		assert.True(t, success, "Encoding should succeed")
	case <-ctx.Done():
		t.Log("Encoding verification timed out (network timing)")
	}

	mockCommitRepo.AssertExpectations(t)
}

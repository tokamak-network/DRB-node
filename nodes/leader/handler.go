package leader_node

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-pg/pg/v10"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// Helper functions for merkleRootSubmitted atomic variable
func (lh *LeaderNodeHandler) SetMerkleRootSubmitted(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&lh.merkleRootSubmitted, val)
}

func (lh *LeaderNodeHandler) GetMerkleRootSubmitted() bool {
	return atomic.LoadInt32(&lh.merkleRootSubmitted) == 1
}

type LeaderNodeHandler struct {
	fallbackEthClient   *fallback_ethclient.FallbackRPCClient
	merkleRootSubmitted int32 // 0 = false, 1 = true (atomic)
	commitMu            sync.Mutex
	leaderNode          *LeaderNode
	ethService          eth.IEthService // Injected eth service for testability
}

func NewLeaderNodeHandler(fallbackEthClient *fallback_ethclient.FallbackRPCClient, db *pg.DB) *LeaderNodeHandler {
	leaderCommitRepository := database.NewLeaderCommitRepository(db)
	batchRepository := database.NewBatchRepository(db)
	broadcastTrackerRepository := database.NewBroadcastTrackerRepository(db)
	reavealOrderRepository := database.NewRevealOrderRepository(db)
	nodeInfoRepository := database.NewNodeInfoRepository(db)
	peerCommitRepository := database.NewPeerCommitRepository(db)
	revealOrderService := commitreveal2.NewRevealOrderService(
		reavealOrderRepository,
		peerCommitRepository,
		leaderCommitRepository,
	)
	p2pClient := libp2putils.NewP2PClient(nodeInfoRepository)
	leaderNode := NewLeaderNode(
		fallbackEthClient,
		revealOrderService,
		p2pClient,
		leaderCommitRepository,
		batchRepository,
		broadcastTrackerRepository,
		reavealOrderRepository,
		nodeInfoRepository,
	)

	return &LeaderNodeHandler{
		fallbackEthClient:   fallbackEthClient,
		leaderNode:          leaderNode,
		merkleRootSubmitted: 0,
		commitMu:            sync.Mutex{},
		ethService:          eth.Service, // Use default eth service
	}
}

func (lh *LeaderNodeHandler) Run(ctx context.Context) {
	envCfg := appconfig.Get()

	port := envCfg.LeaderPort
	if port == "" {
		log.Fatal("LEADER_PORT is not set in environment variables.")
	}

	nodeType := envCfg.NodeType
	if nodeType == "" {
		log.Fatal("NODE_TYPE is not set in environment variables.")
	}

	h, peerID, err := lh.leaderNode.CreateHost(port, nodeType)
	if err != nil {
		log.Fatalf("Error creating host: %v", err)
	}

	defer h.Close()
	lh.leaderNode.SetHost(h)
	h.SetStreamHandler("/register", func(s network.Stream) {
		lh.handleRegistrationRequest(ctx, s)
	})
	h.SetStreamHandler("/cvs", func(s network.Stream) {
		lh.handleCommitRequest(ctx, s)
	})
	h.SetStreamHandler("/cos", func(s network.Stream) {
		lh.handleCOSRequest(ctx, h, s)
	})
	h.SetStreamHandler("/secretValue", func(s network.Stream) {
		lh.leaderNode.AcceptSecretValue(ctx, h, s, lh.fallbackEthClient)
	})
	h.SetStreamHandler("/acknowledgment", func(s network.Stream) {
		lh.handleAcknowledgment(ctx, s)
	})

	log.Printf("Leader node running on: %s", h.Addrs())
	log.Printf("Leader node PeerID: %s", peerID.String())

	lh.ethService.UpdateActivatedOperators(ctx, lh.fallbackEthClient)

	err = lh.leaderNode.UpdateCurrentRoundAndTrial(ctx)
	if err != nil {
		log.Fatalf("Failed to update current round and trial from contract: %v", err)
	}

	go lh.leaderNode.CheckHaltedState(ctx)
	go lh.leaderNode.MonitorCommits(ctx)
	go lh.leaderNode.ReceiveCommit(ctx)

	// Block until shutdown signal is received
	<-ctx.Done()
	log.Println("Leader node received shutdown signal, cleaning up...")

	// Give goroutines a moment to finish
	time.Sleep(2 * time.Second)

	log.Println("Leader node shutdown complete")
}

func (lh *LeaderNodeHandler) handleRegistrationRequest(ctx context.Context, s network.Stream) {
	defer s.Close()

	if err := lh.leaderNode.RegisterNode(ctx, s, "contract/abi/Commit2RevealDRB.json"); err != nil {
		log.Printf("Failed to handle registration request: %v", err)
		return
	}
	log.Println("Node registration completed.")
}

func (lh *LeaderNodeHandler) handleCommitRequest(ctx context.Context, s network.Stream) {
	defer s.Close()
	if lh.leaderNode.GetHalted() {
		log.Println("System is halted. Skipping handleCommitRequest.")
		return
	}

	var req utils.CommitRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode commit request: %v", err)
		return
	}

	// Verify signature for ALL fields (Round, TrialNum, Cvs, EOAAddress, UniqueKey)
	if !utils.VerifyCommitRequestContentSignature(req, req.EOAAddress) {
		log.Printf("Signature verification failed for commit request from EOA: %s. Round, TrialNum, Cvs, EOAAddress, or UniqueKey may have been tampered.", req.EOAAddress)
		return
	}

	// EIP-712 signature verification (content-based - Round, TrialNum, CVS)
	if !lh.VerifyCvsEIP712Signature(req.Round, req.TrialNum, req.Cvs, req.Sign, req.EOAAddress) {
		log.Printf("EIP-712 signature verification failed for CVS from EOA %s, round %s, trial %s. Rejecting CVS.", req.EOAAddress, req.Round, req.TrialNum)
		return
	}

	eoaAddress := common.HexToAddress(req.EOAAddress)
	if !lh.CheckActivation(ctx, eoaAddress, "commit") {
		return
	}

	// Use leader's current round and trial
	round := lh.leaderNode.GetCurrentRound()
	trial := lh.leaderNode.GetCurrentTrial()


	lh.commitMu.Lock()
	defer lh.commitMu.Unlock()
	uniqueKey := utils.GetUniqueKey(round, trial)
	commitData := lh.leaderNode.GetOrCreateLeaderCommitData(round, trial, uniqueKey, eoaAddress)
	if commitData.Cvs == [32]byte{} {
		commitData.Cvs = req.Cvs
		commitData.CvsHex = hex.EncodeToString(req.Cvs[:])
		commitData.Sign = req.Sign
		commitData.SubmitMerkleRootDone = false
		commitData.RandomNumberGenerated = false
	}
	lh.updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	log.Printf("Commit data saved and updated in-memory for round %s with trail %s EOA %s", round, trial, commitData.EOAAddress)

	// Update database for commit data from regular node
	if err := lh.leaderNode.AddLeaderCommit(ctx, commitData); err != nil {
		log.Printf("Error saving commit data for round %s EOA %s: %v", round, commitData.EOAAddress, err)
		return
	}
	lh.updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	activatedOps := lh.ethService.GetActivatedOperatorsCached()
	lh.leaderNode.ReliableBroadCastCVS(ctx, round, trial, eoaAddress, commitData.Cvs, activatedOps)
	// Check if all commits are ready after this update
	if !lh.GetMerkleRootSubmitted() && lh.allCommitsReceivedUnlocked(uniqueKey) {
		log.Printf("All CVS received for round %s with trail %s. Generating Merkle root...", round, trial)
		lh.commitMu.Unlock() // Unlock before calling GenerateMerkleRoot
		lh.leaderNode.GenerateMerkleRoot(ctx, round, trial)
		lh.commitMu.Lock() // Re-lock if needed
	}
}

func (lh *LeaderNodeHandler) handleCOSRequest(ctx context.Context, h host.Host, s network.Stream) {
	defer s.Close()
	if lh.leaderNode.GetHalted() {
		log.Println("System is halted. Skipping handleCOSRequest.")
		return
	}
	var req utils.CosRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode COS request: %v", err)
		return
	}

	// Verify signature for ALL fields
	if !utils.VerifyCosRequestContentSignature(req, req.EOAAddress) {
		log.Printf("Signature verification failed for COS value request from EOA: %s. Round, TrialNum, Cos, EOAAddress, or UniqueKey may have been tampered.", req.EOAAddress)
		return
	}

	round := lh.leaderNode.GetCurrentRound()
	trial := lh.leaderNode.GetCurrentTrial()
	eoaAddress := common.HexToAddress(req.EOAAddress)

	lh.commitMu.Lock()
	defer lh.commitMu.Unlock()
	uniqueKey := utils.GetUniqueKey(round, trial)
	// Update in-memory data for leaderCommit's COS
	commitData := lh.leaderNode.GetOrCreateLeaderCommitData(round, trial, uniqueKey, eoaAddress)
	if commitData.Cvs == [32]byte{} {
		log.Printf("No CVS found for round %s with trail %s EOA %s, rejecting COS.", round, trial, eoaAddress.Hex())
		return
	}

	activatedOperators := lh.ethService.GetActivatedOperatorsCached()
	operatorIndex := -1
	for i, op := range activatedOperators {
		if op.Hex() == req.EOAAddress {
			operatorIndex = i
			break
		}
	}

	if operatorIndex == -1 {
		log.Printf("Operator %s not found in activated operators", eoaAddress.Hex())
		return
	}
	// Convert operatorIndex to a single byte
	opIndexByte := []byte{uint8(operatorIndex)}
	// Calculate CVS using abi.encodePacked(CO, uint8(operatorIndex))
	recalculatedCvs := commitreveal2.Keccak256(commitreveal2.AbiEncodePacked(req.Cos[:], opIndexByte))

	if !bytes.Equal(recalculatedCvs, commitData.Cvs[:]) {
		log.Printf("COS hash mismatch for round %s with trail %s EOA %s. Rejecting COS.", round, trial, eoaAddress.Hex())
		return
	}

	if commitData.Cos != [32]byte{} {
		log.Printf("COS already received for round %s with trail %s EOA %s. Skipping.", round, trial, eoaAddress.Hex())
		return
	}

	commitData.Cos = req.Cos
	commitData.CosHex = hex.EncodeToString(req.Cos[:])

	lh.updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	log.Printf("COS data saved and updated in-memory for round %s with trail %s EOA %s", round, trial, eoaAddress.Hex())

	// Update database for leaderCommit's COS
	leaderCommitDBData, err := lh.leaderNode.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trial, eoaAddress.Hex())
	if err != nil {
		log.Printf("Error loading leaderCommit data from database for round: %s, trail: %s, and eoaAddress: %s, error: %v", round, trial, eoaAddress.Hex(), err)
		return
	}

	leaderCommitDBData.Cos = req.Cos
	leaderCommitDBData.CosHex = hex.EncodeToString(req.Cos[:])

	if err := lh.leaderNode.UpdateLeaderCommit(ctx, leaderCommitDBData); err != nil {
		log.Printf("Error saving COS data for round %s with trail %s EOA %s: %v", round, trial, eoaAddress.Hex(), err)
		return
	}
	lh.updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	activatedOps := lh.ethService.GetActivatedOperatorsCached()
	lh.leaderNode.ReliableBroadCastCOS(ctx, round, trial, eoaAddress, commitData.Cos, activatedOps)

	// Also, if all COS are received (if that matters), we determine reveal order as existing code:
	if lh.allCosReceivedUnlocked(uniqueKey) {
		log.Printf("All COS received for round %s with trail %s.", round, trial)
		_, err := lh.leaderNode.DetermineRevealOrder(ctx, round, trial, activatedOps)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s with trail %s: %v", round, trial, err)
			return
		}
		lh.leaderNode.StartSecretValueRequests(ctx, h, round, trial)
	}
}

func (lh *LeaderNodeHandler) CheckActivation(ctx context.Context, eoaAddress common.Address, reqType string) bool {
	IsNetworkError, isEOAActivated := lh.isEOAActivatedForRound(ctx, eoaAddress)
	if IsNetworkError {
		log.Printf("Network error. Skipping activation check.")
		return false
	}
	if !isEOAActivated {
		log.Printf("EOA %s not activated, skipping %v.", eoaAddress.Hex(), reqType)
		return false
	}
	return true
}

// allCommitsReceivedUnlocked checks if all operators have CVS in-memory.
// Called with commitMu locked.
func (lh *LeaderNodeHandler) allCommitsReceivedUnlocked(uniqueKey string) bool {
	ops := lh.ethService.GetActivatedOperatorsCached()
	if len(ops) == 0 {
		return false
	}

	roundCommits, roundExists := utils.GetCommittedNodes(uniqueKey)
	if !roundExists || len(roundCommits) == 0 {
		return false
	}

	for _, op := range ops {
		data, ok := roundCommits[op]
		if !ok || data.Cvs == [32]byte{} {
			return false
		}
	}
	return true
}
func (lh *LeaderNodeHandler) allCosReceivedUnlocked(uniqueKey string) bool {
	ops := lh.ethService.GetActivatedOperatorsCached()
	if len(ops) == 0 {
		return false
	}

	roundCommits, roundExists := utils.GetCommittedNodes(uniqueKey)
	if !roundExists || len(roundCommits) == 0 {
		return false
	}

	for _, op := range ops {
		data, ok := roundCommits[op]
		if !ok || data.Cos == [32]byte{} {
			return false
		}
	}
	return true
}

func (lh *LeaderNodeHandler) UpdatedallCommitsReceivedUnlocked(ctx context.Context, fallbackEthClient *fallback_ethclient.FallbackRPCClient, uniqueKey string) map[string]bool {
	result := make(map[string]bool)
	ops, _ := lh.ethService.GetActivatedOperators(ctx, fallbackEthClient)

	roundCommits, roundExists := utils.GetCommittedNodes(uniqueKey)
	if !roundExists || len(roundCommits) == 0 {
		for _, op := range ops {
			result[op.Hex()] = false
		}
	}

	for _, op := range ops {
		data, ok := roundCommits[op]
		if ok {
			if data.Cvs != [32]byte{} {
				result[op.Hex()] = true
			} else {
				result[op.Hex()] = false
			}
		} else {
			result[op.Hex()] = false
		}
	}
	return result
}

// updateInMemoryData updates committedNodes with the latest commitData.
// Called with commitMu locked.
func (lh *LeaderNodeHandler) updateInMemoryData(uniqueKey string, eoaAddress common.Address, commitData utils.LeaderCommitData) {
	utils.SetCommittedNodeData(uniqueKey, eoaAddress, commitData)
}

func (lh *LeaderNodeHandler) isEOAActivatedForRound(ctx context.Context, eoaAddress common.Address) (bool, bool) {
	activatedOperators, err := lh.ethService.GetActivatedOperators(ctx, lh.fallbackEthClient)
	if err != nil {
		log.Printf("Error fetching the activated operators %v", err)
		// Return true for network error flag, false for activation status
		return true, false
	}

	for _, operator := range activatedOperators {
		if operator == eoaAddress {
			log.Printf("EOA address %s is activated", eoaAddress.Hex())
			return false, true
		}
	}

	log.Printf("EOA address %s is NOT activated", eoaAddress.Hex())
	return false, false
}

func (lh *LeaderNodeHandler) handleAcknowledgment(ctx context.Context, s network.Stream) {
	defer s.Close()
	if lh.leaderNode.GetHalted() {
		log.Println("System is halted. Skipping handleAcknowledgment.")
		return
	}
	var ack utils.AcknowledgmentMessage
	if err := json.NewDecoder(s).Decode(&ack); err != nil {
		log.Printf("Failed to decode acknowledgment message: %v", err)
		return
	}

	// Verify signature for ALL fields
	if !utils.VerifyAcknowledgmentContentSignature(ack, ack.EOAAddress) {
		log.Printf("Signature verification failed for acknowledgment from EOA: %s (message ID: %s). Round, TrialNum, EOAAddress, MessageID, Type, or Status may have been tampered.", ack.EOAAddress, ack.MessageID)
		return
	}

	// Check if EOA is activated
	eoaAddress := common.HexToAddress(ack.EOAAddress)
	if !lh.CheckActivation(ctx, eoaAddress, "acknowledgment") {
		return
	}

	log.Printf("Received acknowledgment from %s for %s broadcast (message ID: %s, status: %s)",
		ack.EOAAddress, ack.Type, ack.MessageID, ack.Status)

	// Process the acknowledgment
	lh.leaderNode.HandleAcknowledgment(ctx, ack)
}

func (lh *LeaderNodeHandler) VerifyCvsEIP712Signature(round string, trialNum string, cvs [32]byte, signInfo utils.SignInfo, expectedEOA string) bool {
	vStr := signInfo.V
	rStr := signInfo.R
	sStr := signInfo.S

	if vStr == "" || rStr == "" || sStr == "" {
		log.Printf("EIP-712 signature verification failed: v, r, or s is empty for EOA %s", expectedEOA)
		return false
	}

	v, err := strconv.ParseUint(vStr, 10, 8)
	if err != nil {
		log.Printf("EIP-712 signature verification failed: invalid v value '%s' for EOA %s: %v", vStr, expectedEOA, err)
		return false
	}

	if v < 27 {
		v += 27
	}

	rBytes, err := hex.DecodeString(strings.TrimPrefix(rStr, "0x"))
	if err != nil {
		log.Printf("EIP-712 signature verification failed: invalid r value '%s' for EOA %s: %v", rStr, expectedEOA, err)
		return false
	}

	sBytes, err := hex.DecodeString(strings.TrimPrefix(sStr, "0x"))
	if err != nil {
		log.Printf("EIP-712 signature verification failed: invalid s value '%s' for EOA %s: %v", sStr, expectedEOA, err)
		return false
	}

	if len(rBytes) != 32 || len(sBytes) != 32 {
		log.Printf("EIP-712 signature verification failed: r or s is not 32 bytes for EOA %s", expectedEOA)
		return false
	}

	roundBigInt, ok := new(big.Int).SetString(round, 10)
	if !ok {
		log.Printf("EIP-712 signature verification failed: invalid round '%s' for EOA %s", round, expectedEOA)
		return false
	}

	trialNumBigInt, ok := new(big.Int).SetString(trialNum, 10)
	if !ok {
		log.Printf("EIP-712 signature verification failed: invalid trialNum '%s' for EOA %s", trialNum, expectedEOA)
		return false
	}

	typedDataHash, err := utils.ComputeCvsEIP712TypedDataHash(roundBigInt, trialNumBigInt, cvs)
	if err != nil {
		log.Printf("EIP-712 signature verification failed: error computing typed data hash for EOA %s: %v", expectedEOA, err)
		return false
	}

	signature := make([]byte, 65)
	copy(signature[:32], rBytes)
	copy(signature[32:64], sBytes)
	signature[64] = byte(v - 27)

	pubKey, err := crypto.SigToPub(typedDataHash.Bytes(), signature)
	if err != nil {
		log.Printf("EIP-712 signature verification failed: error recovering public key for EOA %s: %v", expectedEOA, err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey)
	expectedAddress := common.HexToAddress(expectedEOA)

	isValid := recoveredAddress == expectedAddress
	if isValid {
		log.Printf("EIP-712 signature verification successful for EOA %s (recovered: %s)", expectedEOA, recoveredAddress.Hex())
	} else {
		log.Printf("EIP-712 signature verification failed for EOA %s (recovered: %s)", expectedEOA, recoveredAddress.Hex())
	}

	return isValid
}

package nodes

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
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
	leaderNode          *leader_node.LeaderNode
}

func NewLeaderNodeHandler(fallbackEthClient *fallback_ethclient.FallbackRPCClient) *LeaderNodeHandler {
	return &LeaderNodeHandler{
		fallbackEthClient:   fallbackEthClient,
		leaderNode:          leader_node.NewLeaderNode(fallbackEthClient),
		merkleRootSubmitted: 0,
		commitMu:            sync.Mutex{},
	}
}

func (lh *LeaderNodeHandler) Run() {
	port := os.Getenv("LEADER_PORT")
	if port == "" {
		log.Fatal("LEADER_PORT is not set in environment variables.")
	}

	nodeType := os.Getenv("NODE_TYPE")
	if nodeType == "" {
		log.Fatal("NODE_TYPE is not set in environment variables.")
	}

	h, peerID, err := libp2putils.CreateHost(port, nodeType)
	if err != nil {
		log.Fatalf("Error creating host: %v", err)
	}

	// Populate peerstore from DB
	// libp2putils.PopulatePeerstoreFromDB(h)

	defer h.Close()
	libp2putils.SetHost(h)
	h.SetStreamHandler("/register", lh.handleRegistrationRequest)
	h.SetStreamHandler("/cvs", lh.handleCommitRequest)
	h.SetStreamHandler("/cos", func(s network.Stream) {
		lh.handleCOSRequest(h, s)
	})
	h.SetStreamHandler("/secretValue", func(s network.Stream) {
		lh.leaderNode.AcceptSecretValue(h, s, lh.fallbackEthClient)
	})
	h.SetStreamHandler("/acknowledgment", func(s network.Stream) {
		lh.handleAcknowledgment(s)
	})

	log.Printf("Leader node running on: %s", h.Addrs())
	log.Printf("Leader node PeerID: %s", peerID.String())

	eth.UpdateActivatedOperators(lh.fallbackEthClient)
	lh.leaderNode.UpdateCurrentRoundAndTrial()
	go lh.leaderNode.CheckHaltedState()
	go lh.leaderNode.MonitorCommits()
	go lh.leaderNode.ReceiveCommit()
	// leaderNode_helper.StartBroadcastCleanup()
	// leaderNode_helper.StartLeaderCommitCleanup()
	for {
		if !lh.leaderNode.GetExecution() {
			time.Sleep(10 * time.Second)
			continue
		}
		// firstRequest := leaderNode_helper.GetReq()
		// processRounds(fallbackEthClient, leaderNode_helper.GetReq())
		time.Sleep(30 * time.Second)
	}
}

func (lh *LeaderNodeHandler) handleRegistrationRequest(s network.Stream) {
	defer s.Close()

	if err := lh.leaderNode.RegisterNode(s, "contract/abi/Commit2RevealDRB.json"); err != nil {
		log.Printf("Failed to handle registration request: %v", err)
		return
	}
	log.Println("Node registration completed.")
}

func (lh *LeaderNodeHandler) handleCommitRequest(s network.Stream) {
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

	commitVerificationRequest := utils.Request{
		Round:      req.Round,
		TrialNum:   req.TrialNum,
		EOAAddress: req.EOAAddress,
		Signature:  req.Signature,
	}

	if !lh.VerifySignatureAndCheckActivation(commitVerificationRequest, "commit") {
		return
	}

	round := req.Round
	eoaAddress := common.HexToAddress(req.EOAAddress)

	lh.commitMu.Lock()
	defer lh.commitMu.Unlock()
	uniqueKey := utils.GetUniqueKey(round, req.TrialNum)
	commitData := lh.leaderNode.GetOrCreateLeaderCommitData(round, req.TrialNum, uniqueKey, eoaAddress)
	if commitData.Cvs == [32]byte{} {
		commitData.Cvs = req.Cvs
		commitData.CvsHex = hex.EncodeToString(req.Cvs[:])
		commitData.Sign = req.Sign
		commitData.SubmitMerkleRootDone = false
		commitData.RandomNumberGenerated = false
	}
	lh.updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	log.Printf("Commit data saved and updated in-memory for round %s with trail %s EOA %s", round, req.TrialNum, commitData.EOAAddress)

	// Update database for commit data from regular node
	if err := database.AddLeaderCommit(commitData); err != nil {
		log.Printf("Error saving commit data for round %s EOA %s: %v", round, commitData.EOAAddress, err)
		return
	}
	lh.updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	activatedOps := eth.GetActivatedOperatorsCached()
	lh.leaderNode.ReliableBroadCastCVS(libp2putils.HostInstance, round, req.TrialNum, eoaAddress, commitData.Cvs, activatedOps)
	// Check if all commits are ready after this update
	if !lh.GetMerkleRootSubmitted() && lh.allCommitsReceivedUnlocked(uniqueKey) {
		log.Printf("All CVS received for round %s with trail %s. Generating Merkle root...", round, req.TrialNum)
		lh.commitMu.Unlock() // Unlock before calling GenerateMerkleRoot
		lh.leaderNode.GenerateMerkleRoot(round, req.TrialNum)
		lh.commitMu.Lock() // Re-lock if needed
	}
}

func (lh *LeaderNodeHandler) handleCOSRequest(h host.Host, s network.Stream) {
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

	// Verify the EOA signature
	verifyReq := utils.Verification{
		EOAAddress: req.EOAAddress,
		Signature:  req.Signature,
	}

	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for COS value request from EOA: %s", req.EOAAddress)
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

	activatedOperators := eth.GetActivatedOperatorsCached()
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
	leaderCommitDBData, err := database.GetLeaderCommitByRoundAndEoaAddr(round, trial, eoaAddress.Hex())
	if err != nil {
		log.Printf("Error loading leaderCommit data from database for round: %s, trail: %s, and eoaAddress: %s, error: %v", round, trial, eoaAddress.Hex(), err)
		return
	}

	leaderCommitDBData.Cos = req.Cos
	leaderCommitDBData.CosHex = hex.EncodeToString(req.Cos[:])

	if err := database.UpdateLeaderCommit(leaderCommitDBData); err != nil {
		log.Printf("Error saving COS data for round %s with trail %s EOA %s: %v", round, trial, eoaAddress.Hex(), err)
		return
	}
	lh.updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	activatedOps := eth.GetActivatedOperatorsCached()
	lh.leaderNode.ReliableBroadCastCOS(libp2putils.HostInstance, round, trial, eoaAddress, commitData.Cos, activatedOps)

	// Also, if all COS are received (if that matters), we determine reveal order as existing code:
	if lh.allCosReceivedUnlocked(uniqueKey) {
		log.Printf("All COS received for round %s with trail %s.", round, trial)
		_, err := commitreveal2.DetermineRevealOrder(round, trial, activatedOps)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s with trail %s: %v", round, trial, err)
			return
		}
		lh.leaderNode.StartSecretValueRequests(h, round, trial)
	}
}

func (lh *LeaderNodeHandler) VerifySignatureAndCheckActivation(req utils.Request, reqType string) bool {
	verifyReq := utils.Verification{EOAAddress: req.EOAAddress, Signature: req.Signature}
	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for round %s EOA %s", req.Round, req.EOAAddress)
		return false
	}

	eoaAddress := common.HexToAddress(req.EOAAddress)

	IsNetworkError, isEOAActivated := lh.isEOAActivatedForRound(eoaAddress)
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
	ops := eth.GetActivatedOperatorsCached()
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
	ops := eth.GetActivatedOperatorsCached()
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

func (lh *LeaderNodeHandler) UpdatedallCommitsReceivedUnlocked(fallbackEthClient *fallback_ethclient.FallbackRPCClient, uniqueKey string) map[string]bool {
	result := make(map[string]bool)
	ops, _ := eth.GetActivatedOperators(fallbackEthClient)

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

func (lh *LeaderNodeHandler) isEOAActivatedForRound(eoaAddress common.Address) (bool, bool) {
	activatedOperators, err := eth.GetActivatedOperators(lh.fallbackEthClient)
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

func (lh *LeaderNodeHandler) handleAcknowledgment(s network.Stream) {
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

	// Verify EOA signature for acknowledgment
	verifyReq := utils.Verification{
		EOAAddress: ack.EOAAddress,
		Signature:  ack.Signature,
	}

	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for acknowledgment from EOA: %s (message ID: %s)", ack.EOAAddress, ack.MessageID)
		return
	}

	commitVerificationRequest := utils.Request{
		Round:      ack.Round,
		TrialNum:   ack.TrialNum,
		EOAAddress: ack.EOAAddress,
		Signature:  ack.Signature,
	}

	if !lh.VerifySignatureAndCheckActivation(commitVerificationRequest, "commit") {
		return
	}

	log.Printf("Received acknowledgment from %s for %s broadcast (message ID: %s, status: %s)",
		ack.EOAAddress, ack.Type, ack.MessageID, ack.Status)

	// Process the acknowledgment
	lh.leaderNode.HandleAcknowledgment(ack)
}

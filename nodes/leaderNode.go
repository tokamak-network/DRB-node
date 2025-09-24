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
	leaderNode_helper "github.com/tokamak-network/DRB-node/nodes/leaderNode_helper"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// Helper functions for merkleRootSubmitted atomic variable
func SetMerkleRootSubmitted(value bool) {
	var val int32
	if value {
		val = 1
	}
	atomic.StoreInt32(&merkleRootSubmitted, val)
}

func GetMerkleRootSubmitted() bool {
	return atomic.LoadInt32(&merkleRootSubmitted) == 1
}

var merkleRootSubmitted int32 // 0 = false, 1 = true (atomic)
var commitMu sync.Mutex

type Handler struct {
	fallbackEthClient *fallback_ethclient.FallbackRPCClient
}

func RunLeaderNode(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
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

	handler := &Handler{
		fallbackEthClient: fallbackEthClient,
	}

	defer h.Close()
	libp2putils.SetHost(h)
	h.SetStreamHandler("/register", handler.handleRegistrationRequest)
	h.SetStreamHandler("/cvs", handler.handleCommitRequest)
	h.SetStreamHandler("/cos", func(s network.Stream) {
		handleCOSRequest(fallbackEthClient, h, s)
	})
	h.SetStreamHandler("/secretValue", func(s network.Stream) {
		leaderNode_helper.AcceptSecretValue(h, s, fallbackEthClient)
	})
	h.SetStreamHandler("/acknowledgment", func(s network.Stream) {
		handleAcknowledgment(fallbackEthClient, s)
	})

	log.Printf("Leader node running on: %s", h.Addrs())
	log.Printf("Leader node PeerID: %s", peerID.String())

	eth.UpdateActivatedOperators(fallbackEthClient)
	leaderNode_helper.UpdateCurrentRoundAndTrial(fallbackEthClient)
	go leaderNode_helper.CheckHaltedState(fallbackEthClient)
	go leaderNode_helper.MonitorCommits(fallbackEthClient)
	go leaderNode_helper.ReceiveCommit(fallbackEthClient)
	// leaderNode_helper.StartBroadcastCleanup()
	// leaderNode_helper.StartLeaderCommitCleanup()
	for {
		if !leaderNode_helper.GetExecution() {
			time.Sleep(10 * time.Second)
			continue
		}
		// firstRequest := leaderNode_helper.GetReq()
		// processRounds(fallbackEthClient, leaderNode_helper.GetReq())
		time.Sleep(30 * time.Second)
	}
}

func (h *Handler) handleRegistrationRequest(s network.Stream) {
	defer s.Close()

	if err := leaderNode_helper.RegisterNode(s, "contract/abi/Commit2RevealDRB.json", h.fallbackEthClient); err != nil {
		log.Printf("Failed to handle registration request: %v", err)
		return
	}
	log.Println("Node registration completed.")
}

func (h *Handler) handleCommitRequest(s network.Stream) {
	defer s.Close()
	if atomic.LoadInt32(&leaderNode_helper.Halted) == 1 {
		log.Println("System is halted. Skipping handleCommitRequest.")
		return
	}
	fallbackEthClient := h.fallbackEthClient

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

	if !VerifySignatureAndCheckActivation(fallbackEthClient, commitVerificationRequest, "commit") {
		return
	}

	round := req.Round
	eoaAddress := common.HexToAddress(req.EOAAddress)

	commitMu.Lock()
	defer commitMu.Unlock()
	uniqueKey := utils.GetUniqueKey(round, req.TrialNum)
	commitData := leaderNode_helper.GetOrCreateLeaderCommitData(round, req.TrialNum, uniqueKey, eoaAddress)
	if commitData.Cvs == [32]byte{} {
		commitData.Cvs = req.Cvs
		commitData.CvsHex = hex.EncodeToString(req.Cvs[:])
		commitData.Sign = req.Sign
		commitData.SubmitMerkleRootDone = false
		commitData.RandomNumberGenerated = false
	}
	updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	log.Printf("Commit data saved and updated in-memory for round %s with trail %s EOA %s", round, req.TrialNum, commitData.EOAAddress)

	// Update database for commit data from regular node
	if err := database.AddLeaderCommit(commitData); err != nil {
		log.Printf("Error saving commit data for round %s EOA %s: %v", round, commitData.EOAAddress, err)
		return
	}
	updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	leaderNode_helper.ReliableBroadCastCVS(libp2putils.HostInstance, round, req.TrialNum, eoaAddress, commitData.Cvs)
	// Check if all commits are ready after this update
	if !GetMerkleRootSubmitted() && allCommitsReceivedUnlocked(uniqueKey) {
		log.Printf("All CVS received for round %s with trail %s. Generating Merkle root...", round, req.TrialNum)
		commitMu.Unlock() // Unlock before calling GenerateMerkleRoot
		leaderNode_helper.GenerateMerkleRoot(fallbackEthClient, round, req.TrialNum)
		commitMu.Lock() // Re-lock if needed
	}
}

func handleCOSRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, h host.Host, s network.Stream) {
	defer s.Close()
	if atomic.LoadInt32(&leaderNode_helper.Halted) == 1 {
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

	round := leaderNode_helper.GetCurrentRound()
	trial := leaderNode_helper.GetCurrentTrial()
	eoaAddress := common.HexToAddress(req.EOAAddress)

	commitMu.Lock()
	defer commitMu.Unlock()
	uniqueKey := utils.GetUniqueKey(round, trial)
	// Update in-memory data for leaderCommit's COS
	commitData := leaderNode_helper.GetOrCreateLeaderCommitData(round, trial, uniqueKey, eoaAddress)
	if commitData.Cvs == [32]byte{} {
		log.Printf("No CVS found for round %s with trail %s EOA %s, rejecting COS.", round, trial, eoaAddress.Hex())
		return
	}

	recalculatedCvs := commitreveal2.Keccak256(req.Cos[:])
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

	updateInMemoryData(uniqueKey, eoaAddress, *commitData)
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
	updateInMemoryData(uniqueKey, eoaAddress, *commitData)
	leaderNode_helper.ReliableBroadCastCOS(libp2putils.HostInstance, round, trial, eoaAddress, commitData.Cos)
	// Check if all commits are ready after this COS
	// if !GetMerkleRootSubmitted() && allCommitsReceivedUnlocked(uniqueKey) {
	// 	log.Printf("All CVS received for round %s with trail %s after COS, generating Merkle root...", round, trial)
	// 	commitMu.Unlock()
	// 	generateMerkleRoot(fallbackEthClient, round, trial)
	// 	commitMu.Lock()
	// }

	// Also, if all COS are received (if that matters), we determine reveal order as existing code:
	if allCosReceivedUnlocked(uniqueKey) {
		log.Printf("All COS received for round %s with trail %s.", round, trial)
		_, err := commitreveal2.DetermineRevealOrder(round, trial, eth.ActivatedOperators)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s with trail %s: %v", round, trial, err)
			return
		}
		leaderNode_helper.StartSecretValueRequests(h, fallbackEthClient, round, trial)
	}
}

func VerifySignatureAndCheckActivation(fallbackEthClient *fallback_ethclient.FallbackRPCClient, req utils.Request, reqType string) bool {
	verifyReq := utils.Verification{EOAAddress: req.EOAAddress, Signature: req.Signature}
	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for round %s EOA %s", req.Round, req.EOAAddress)
		return false
	}

	eoaAddress := common.HexToAddress(req.EOAAddress)

	IsNetworkError, isEOAActivated := isEOAActivatedForRound(fallbackEthClient, eoaAddress)
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
func allCommitsReceivedUnlocked(uniqueKey string) bool {
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
func allCosReceivedUnlocked(uniqueKey string) bool {
	ops := eth.ActivatedOperators
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

func UpdatedallCommitsReceivedUnlocked(fallbackEthClient *fallback_ethclient.FallbackRPCClient, uniqueKey string) map[string]bool {
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
func updateInMemoryData(uniqueKey string, eoaAddress common.Address, commitData utils.LeaderCommitData) {
	utils.SetCommittedNodeData(uniqueKey, eoaAddress, commitData)
}

// func updateCommitDataAfterSubmit(roundNum string, trialNum string, uniqueKey string) {
// 	commitMu.Lock()
// 	defer commitMu.Unlock()

// 	roundMap, exists := utils.GetCommittedNodes(uniqueKey)
// 	if !exists {
// 		return
// 	}

// 	for eoaAddress, data := range roundMap {
// 		log.Printf("Setting submit_merkle_root_done = true for key: %s+%s", uniqueKey, eoaAddress.Hex())

// 		// Update database with marked submitmerkleroot as done
// 		leaderCommitDBData, err := database.GetLeaderCommitByRoundAndEoaAddr(roundNum, trialNum, eoaAddress.Hex())
// 		if err != nil {
// 			log.Printf("Error loading leaderCommit data from database for round: %s, and eoaAddress: %s, error: %v", roundNum, eoaAddress.Hex(), err)
// 			return
// 		}

// 		leaderCommitDBData.SubmitMerkleRootDone = true

// 		if err := database.UpdateLeaderCommit(leaderCommitDBData); err != nil {
// 			log.Printf("Failed to save updated commit data for %s in round %s: %v", eoaAddress.Hex(), uniqueKey, err)
// 			return
// 		} else {
// 			data.SubmitMerkleRootDone = true
// 			roundMap[eoaAddress] = data
// 		}
// 	}
// }

func isEOAActivatedForRound(fallbackEthClient *fallback_ethclient.FallbackRPCClient, eoaAddress common.Address) (bool, bool) {
	activatedOperators, err := eth.GetActivatedOperators(fallbackEthClient)
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

func handleAcknowledgment(fallbackEthClient *fallback_ethclient.FallbackRPCClient, s network.Stream) {
	defer s.Close()
	if atomic.LoadInt32(&leaderNode_helper.Halted) == 1 {
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

	if !VerifySignatureAndCheckActivation(fallbackEthClient, commitVerificationRequest, "commit") {
		return
	}

	log.Printf("Received acknowledgment from %s for %s broadcast (message ID: %s, status: %s)",
		ack.EOAAddress, ack.Type, ack.MessageID, ack.Status)

	// Process the acknowledgment
	leaderNode_helper.HandleAcknowledgment(ack)
}

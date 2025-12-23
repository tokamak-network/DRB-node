package leader_node

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// var SecretValue [][32]byte

// ResetIndicesForNewRound resets the Indices array for a new round
func (n *LeaderNode) ResetIndicesForNewRound() {
	n.indicesMutex.Lock()
	defer n.indicesMutex.Unlock()
	n.indices = make([]*big.Int, 0)
}

// GetIndices returns a copy of the current Indices array
func (n *LeaderNode) GetIndices() []*big.Int {
	n.indicesMutex.RLock()
	defer n.indicesMutex.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]*big.Int, len(n.indices))
	for i, idx := range n.indices {
		result[i] = new(big.Int).Set(idx)
	}
	return result
}

// AppendToIndices safely appends a new index to the Indices array
func (n *LeaderNode) AppendToIndices(index *big.Int) {
	n.indicesMutex.Lock()
	defer n.indicesMutex.Unlock()
	n.indices = append(n.indices, new(big.Int).Set(index))
}

// SetIndices safely sets the entire Indices array
func (n *LeaderNode) SetIndices(indices []*big.Int) {
	n.indicesMutex.Lock()
	defer n.indicesMutex.Unlock()

	// Create a copy of the input slice
	n.indices = make([]*big.Int, len(indices))
	for i, idx := range indices {
		n.indices[i] = new(big.Int).Set(idx)
	}
}

// AcceptSecretValue processes and stores secret values sent by regular nodes.
func (n *LeaderNode) AcceptSecretValue(ctx context.Context, h host.Host, s network.Stream, fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	defer s.Close()
	if n.GetHalted() {
		log.Println("System is halted. Skipping AcceptSecretValue.")
		return
	}
	// Decode the incoming request
	var req utils.SecretValueRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode secret value request: %v", err)
		return
	}

	// Verify signature for ALL fields
	if !utils.VerifySecretValueContentSignature(req, req.RegularEoaAddress) {
		log.Printf("Signature verification failed for secret value from EOA: %s. Round, TrialNum, SecretValue, or RegularEoaAddress may have been tampered.", req.RegularEoaAddress)
		return
	}

	// log.Printf("Successfully verified signature for EOA: %s", req.RegularEoaAddress)

	round := n.GetCurrentRound()
	trial := n.GetCurrentTrial()
	// log.Printf("Successfully verified signature for EOA: %s", req.RegularEoaAddress)
	uniqueKey := utils.GetUniqueKey(round, trial)
	eoaAddress := common.HexToAddress(req.RegularEoaAddress)
	commitData := n.GetOrCreateLeaderCommitData(round, trial, uniqueKey, eoaAddress)
	if commitData.Cos == [32]byte{} {
		log.Printf("No COS found for round %s with trail %s EOA %s, rejecting secret value.", round, trial, eoaAddress.Hex())
		return
	}

	recalculatedCos := commitreveal2.Keccak256(req.SecretValue[:])
	if !bytes.Equal(recalculatedCos, commitData.Cos[:]) {
		log.Printf("Secret value hash mismatch for round %s with trail %s EOA %s. Rejecting secret value.", round, trial, eoaAddress.Hex())
		return
	}

	log.Printf("Secret value hash matches for round %s with trail %s EOA %s.", round, trial, eoaAddress.Hex())
	// Fetch or initialize the leader commit data for the given round and EOA
	leaderCommitData, err := n.leaderCommitRepository.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trial, req.RegularEoaAddress)
	if err != nil {
		log.Printf("Commit data not found  for round %s with trail %s EOA %s.", round, trial, eoaAddress.Hex())
		return
	}

	var secretValueArray [32]byte
	if len(req.SecretValue) != 32 {
		log.Printf("Invalid secret value length: expected 32 bytes, got %d", len(req.SecretValue))
		return
	}
	copy(secretValueArray[:], req.SecretValue[:])

	// Use the new atomic setter function
	n.AppendToRoundSecrets(uniqueKey, secretValueArray)
	// Store the secret value in both byte array and hex string formats
	copy(leaderCommitData.SecretValue[:], req.SecretValue[:])
	leaderCommitData.SecretValueHex = hex.EncodeToString(req.SecretValue[:])

	log.Printf("Received secret value for round %s with trail %s and EOA %s: byte=%x, hex=%s",
		round, trial, req.RegularEoaAddress, leaderCommitData.SecretValue, leaderCommitData.SecretValueHex)

	if err := n.leaderCommitRepository.UpdateLeaderCommit(ctx, leaderCommitData); err != nil {
		log.Printf("Failed to save updated commit data for %s in round %s with trail %s: %v", req.RegularEoaAddress, round, trial, err)
		return
	}

	log.Printf("Successfully saved secret value for round %s with trail %s and EOA %s", round, trial, req.RegularEoaAddress)

	// Use the new atomic setter function
	n.SetRoundSecretValue(uniqueKey, req.RegularEoaAddress, true)

	// 🔄 Wait for broadcast to complete before proceeding to next node
	log.Printf("🔄 Broadcasting secret from %s for round %s with trail %s...", req.RegularEoaAddress, round, trial)
	activatedOps := eth.Service.GetActivatedOperatorsCached()
	broadcastCompleted := n.ReliableBroadCastSSync(ctx, h, round, trial, req.RegularEoaAddress, leaderCommitData.SecretValue, activatedOps)

	if broadcastCompleted {
		log.Printf("✅ Broadcast completed for %s. Proceeding to next node in reveal order.", req.RegularEoaAddress)
		// Continue requesting secret values from remaining nodes in the reveal order
		n.HandleSecretValueResponse(ctx, h, fallbackEthClient, round, trial, req.RegularEoaAddress)
	} else {
		log.Printf("⚠️ Broadcast incomplete for %s. Proceeding anyway to next node.", req.RegularEoaAddress)
		// Still continue even if broadcast incomplete (leader's decision)
		n.HandleSecretValueResponse(ctx, h, fallbackEthClient, round, trial, req.RegularEoaAddress)
	}
}

// getOrCreateLeaderCommitData returns commitData from in-memory map or creates a new one.
// Called with commitMu locked.
func (n *LeaderNode) GetOrCreateLeaderCommitData(roundNum string, trialNum string, uniqueKey string, eoaAddress common.Address) *utils.LeaderCommitData {
	utils.EnsureCommittedNodesRoundExists(uniqueKey)

	data, existsData := utils.GetCommittedNodeData(uniqueKey, eoaAddress)
	if !existsData {
		data = utils.LeaderCommitData{
			UniqueKey:  uniqueKey,
			Round:      roundNum,
			TrialNum:   trialNum,
			EOAAddress: eoaAddress.Hex(),
			CreatedAt:  time.Now().Unix(),
		}
		utils.SetCommittedNodeData(uniqueKey, eoaAddress, data)
	}
	return &data
}

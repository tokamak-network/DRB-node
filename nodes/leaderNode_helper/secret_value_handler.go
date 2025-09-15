package leaderNode_helper

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// var SecretValue [][32]byte
var RoundSecrets = make(map[string][][32]byte)
var roundSecret = make(map[string]map[string]bool)
var secretsOnChain = make(map[string]bool)
var Indices []*big.Int
var indicesMutex sync.RWMutex
var secretMapsMutex sync.Mutex
var secretsOnChainMu sync.RWMutex

// ResetIndicesForNewRound resets the Indices array for a new round
func ResetIndicesForNewRound() {
	indicesMutex.Lock()
	defer indicesMutex.Unlock()
	Indices = make([]*big.Int, 0)
}

// GetIndices returns a copy of the current Indices array
func GetIndices() []*big.Int {
	indicesMutex.RLock()
	defer indicesMutex.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]*big.Int, len(Indices))
	for i, idx := range Indices {
		result[i] = new(big.Int).Set(idx)
	}
	return result
}

// AppendToIndices safely appends a new index to the Indices array
func AppendToIndices(index *big.Int) {
	indicesMutex.Lock()
	defer indicesMutex.Unlock()
	Indices = append(Indices, new(big.Int).Set(index))
}

// SetIndices safely sets the entire Indices array
func SetIndices(indices []*big.Int) {
	indicesMutex.Lock()
	defer indicesMutex.Unlock()

	// Create a copy of the input slice
	Indices = make([]*big.Int, len(indices))
	for i, idx := range indices {
		Indices[i] = new(big.Int).Set(idx)
	}
}

// AcceptSecretValue processes and stores secret values sent by regular nodes.
func AcceptSecretValue(h host.Host, s network.Stream, fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	defer s.Close()
	if GetHalted() {
		log.Println("System is halted. Skipping AcceptSecretValue.")
		return
	}
	// Decode the incoming request
	var req utils.SecretValueRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode secret value request: %v", err)
		return
	}

	// Verify the EOA signature
	verifyReq := utils.Verification{
		EOAAddress: req.RegularEoaAddress,
		Signature:  req.Signature,
	}

	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for secret value request from EOA: %s", req.RegularEoaAddress)
		return
	}

	// log.Printf("Successfully verified signature for EOA: %s", req.RegularEoaAddress)

	round := GetCurrentRound()
	trial := GetCurrentTrial()
	// log.Printf("Successfully verified signature for EOA: %s", req.RegularEoaAddress)
	uniqueKey := utils.GetUniqueKey(round, trial)
	eoaAddress := common.HexToAddress(req.RegularEoaAddress)
	commitData := GetOrCreateLeaderCommitData(round, trial, uniqueKey, eoaAddress)
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
	leaderCommitData, err := database.GetLeaderCommitByRoundAndEoaAddr(round, trial, req.RegularEoaAddress)
	if err != nil {
		log.Printf("Commit data not found, initializing new entry for round %s and EOA %s", round, req.RegularEoaAddress)
		leaderCommitData = &utils.LeaderCommitData{
			Round:      round,
			TrialNum:   trial,
			EOAAddress: req.RegularEoaAddress,
			CreatedAt:  time.Now().Unix(),
		}
	}
	var secretValueArray [32]byte
	copy(secretValueArray[:], req.SecretValue[:]) // Convert req.SecretValue to [32]byte
	secretMapsMutex.Lock()
	if _, exists := RoundSecrets[uniqueKey]; !exists {
		RoundSecrets[uniqueKey] = make([][32]byte, 0)
	}
	RoundSecrets[uniqueKey] = append(RoundSecrets[uniqueKey], secretValueArray)
	secretMapsMutex.Unlock()
	// Store the secret value in both byte array and hex string formats
	copy(leaderCommitData.SecretValue[:], req.SecretValue[:])
	leaderCommitData.SecretValueHex = hex.EncodeToString(req.SecretValue[:])

	log.Printf("Received secret value for round %s with trail %s and EOA %s: byte=%x, hex=%s",
		round, trial, req.RegularEoaAddress, leaderCommitData.SecretValue, leaderCommitData.SecretValueHex)

	if err := database.UpdateLeaderCommit(leaderCommitData); err != nil {
		log.Printf("Failed to save updated commit data for %s in round %s with trail %s: %v", req.RegularEoaAddress, round, trial, err)
		return
	}

	log.Printf("Successfully saved secret value for round %s with trail %s and EOA %s", round, trial, req.RegularEoaAddress)
	secretMapsMutex.Lock()
	if _, exists := roundSecret[uniqueKey]; !exists {
		roundSecret[uniqueKey] = make(map[string]bool)
	}
	roundSecret[uniqueKey][req.RegularEoaAddress] = true
	secretMapsMutex.Unlock()

	// 🔄 Wait for broadcast to complete before proceeding to next node
	log.Printf("🔄 Broadcasting secret from %s for round %s with trail %s...", req.RegularEoaAddress, round, trial)
	broadcastCompleted := ReliableBroadCastSSync(h, round, trial, req.RegularEoaAddress, leaderCommitData.SecretValue)

	if broadcastCompleted {
		log.Printf("✅ Broadcast completed for %s. Proceeding to next node in reveal order.", req.RegularEoaAddress)
		// Continue requesting secret values from remaining nodes in the reveal order
		HandleSecretValueResponse(h, fallbackEthClient, round, trial, req.RegularEoaAddress)
	} else {
		log.Printf("⚠️ Broadcast incomplete for %s. Proceeding anyway to next node.", req.RegularEoaAddress)
		// Still continue even if broadcast incomplete (leader's decision)
		HandleSecretValueResponse(h, fallbackEthClient, round, trial, req.RegularEoaAddress)
	}
}

// getOrCreateLeaderCommitData returns commitData from in-memory map or creates a new one.
// Called with commitMu locked.
func GetOrCreateLeaderCommitData(roundNum string, trialNum string, uniqueKey string, eoaAddress common.Address) *utils.LeaderCommitData {
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

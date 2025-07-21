package leaderNode_helper

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
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

	// Decode the incoming request
	var req utils.SecretValueRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode secret value request: %v", err)
		return
	}

	// Verify the EOA signature
	verifyReq := utils.RegistrationRequest{
		EOAAddress: req.RegularEoaAddress,
		Signature:  req.Signature,
	}

	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for secret value request from EOA: %s", req.RegularEoaAddress)
		return
	}

	log.Printf("Successfully verified signature for EOA: %s", req.RegularEoaAddress)

	// Get the unique key for the round and trial number
	uniqueKey := utils.GetUniqueKey(req.Round, req.TrialNum)
	// Fetch or initialize the leader commit data for the given round and EOA
	leaderCommitData, err := database.GetLeaderCommitByRoundAndEoaAddr(req.Round, req.TrialNum, uniqueKey, req.RegularEoaAddress)
	if err != nil {
		log.Printf("Commit data not found, initializing new entry for round %s and EOA %s", req.Round, req.RegularEoaAddress)
		leaderCommitData = &utils.LeaderCommitData{
			Round:      req.Round,
			TrialNum:   req.TrialNum,
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
		req.Round, req.TrialNum, req.RegularEoaAddress, leaderCommitData.SecretValue, leaderCommitData.SecretValueHex)

	if err := database.UpdateLeaderCommit(leaderCommitData); err != nil {
		log.Printf("Failed to save updated commit data for %s in round %s with trail %s: %v", req.RegularEoaAddress, req.Round, req.TrialNum, err)
		return
	}

	log.Printf("Successfully saved secret value for round %s with trail %s and EOA %s", req.Round, req.TrialNum, req.RegularEoaAddress)
	secretMapsMutex.Lock()
	if _, exists := roundSecret[uniqueKey]; !exists {
		roundSecret[uniqueKey] = make(map[string]bool)
	}
	roundSecret[uniqueKey][req.RegularEoaAddress] = true
	secretMapsMutex.Unlock()
	ReliableBroadCastS(h, req.Round, req.TrialNum, req.RegularEoaAddress, leaderCommitData.SecretValue)
	// Continue requesting secret values from remaining nodes in the reveal order
	HandleSecretValueResponse(h, fallbackEthClient, req.Round, req.TrialNum, req.RegularEoaAddress)
}

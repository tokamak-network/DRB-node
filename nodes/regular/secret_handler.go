package regular_node

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/utils"
)

// checkPreviousSecretReceived checks if the previous node's secret was received via broadcast
func (n *RegularNode) checkPreviousSecretReceived(ctx context.Context, round, trialNum, previousNodeEOA string) bool {
	// Check if we have the peer commit data (from broadcast) for the previous node
	peerCommitData, err := n.peerCommitDataRepository.GetPeerCommitData(ctx, round, trialNum, previousNodeEOA)
	if err != nil {
		log.Printf("Failed to get peer commit data for previous node %s: %v", previousNodeEOA, err)
		return false
	}

	// Check if secret value exists and is not empty
	if peerCommitData.SecretValue == nil {
		log.Printf("Previous node %s secret value is empty or nil", previousNodeEOA)
		return false
	}

	// Additional check: make sure it's not all zeros (empty array)
	allZeros := true
	for _, b := range peerCommitData.SecretValue {
		if b != 0 {
			allZeros = false
			break
		}
	}

	if allZeros {
		log.Printf("Previous node %s secret value is all zeros", previousNodeEOA)
		return false
	}

	log.Printf("✅ Previous node %s secret value confirmed: %x", previousNodeEOA, peerCommitData.SecretValue[:8]) // Show first 8 bytes for verification
	return true
}

// HandleSecretValueRequest processes secret value requests from the leader node
func (n *RegularNode) HandleSecretValueRequest(ctx context.Context, h host.Host, s network.Stream) {
	defer s.Close()

	if n.GetHalted() {
		log.Println("System is halted. Skipping HandleSecretValueRequest.")
		return
	}
	// Decode the request
	var req utils.SecretValueRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode secret value request: %v", err)
		return
	}
	uniqueKey := utils.GetUniqueKey(req.Round, req.TrialNum)
	for {
		roundData, err := n.revealOrderRepository.GetRevealOrder(ctx, req.Round, req.TrialNum)
		if err != nil {
			log.Printf("Failed to load reveal order with trail %s for round %s: %v", req.TrialNum, req.Round, err)
		} else if roundData != nil {
			n.SetStrictOrder(uniqueKey, roundData.OrderedNodes)
			break
		} else {
			log.Printf("Reveal order not yet calculated for round %s with trail %s. Waiting...", req.Round, req.TrialNum)
		}
		time.Sleep(5 * time.Second)
	}
	strictOrder, exists := n.GetStrictOrder(uniqueKey)
	if !exists || len(strictOrder) == 0 || req.Order >= len(strictOrder) || strictOrder[req.Order] != req.RegularEoaAddress {
		log.Printf("EOA %s is not next in the reveal order %v for round %s with trail %s", req.RegularEoaAddress, req.Order, req.Round, req.TrialNum)
		return
	}

	// 🔍 Check if previous node's secret was received (if not first in order)
	if req.Order > 0 {
		previousNodeEOA := strictOrder[req.Order-1]
		log.Printf("🔍 Checking if previous node's secret was received from EOA %s", previousNodeEOA)

		// Check if we received the previous node's secret via broadcast
		hasPreviousSecret := n.checkPreviousSecretReceived(ctx, req.Round, req.TrialNum, previousNodeEOA)
		if !hasPreviousSecret {
			log.Printf("❌ Previous node's secret from %s not yet received. Cannot send secret yet.", previousNodeEOA)
			return
		}
		log.Printf("✅ Previous node's secret from %s confirmed received. Proceeding to send own secret.", previousNodeEOA)
	} else {
		log.Printf("🎯 First node in reveal order. No need to check previous secrets.")
	}

	// Fetch the leader's EOA address from the environment variables
	leaderEOA := appconfig.Get().LeaderEOA
	if leaderEOA == "" {
		log.Println("LEADER_EOA is not set in the environment variables")
		return
	}

	// Use the existing signature verification mechanism
	verifyReq := utils.Verification{
		EOAAddress: req.LeaderEoaAddress, // Sender's address
		Signature:  req.Signature,        // Signature
	}

	// Verify the signature
	if !utils.VerifySignatureForRegularNode(verifyReq, leaderEOA) {
		log.Printf("Signature verification failed for secret value request: expected %s, got %s", leaderEOA, req.LeaderEoaAddress)
		return
	}

	// Log the request details
	log.Printf("Verified secret value request for round %s with trail %s from leader %s", req.Round, req.TrialNum, req.LeaderEoaAddress)

	// Fetch the secret value for the specified round
	commitData, err := n.regularCommitRepository.GetCommitByRound(ctx, req.Round, req.TrialNum)
	if err != nil {
		log.Printf("Failed to load commit data for round %s with trail %s: %v", req.Round, req.TrialNum, err)
		return
	}

	// Check if the secret value exists
	if commitData.SecretValue == [32]byte{} {
		log.Printf("No secret value found for round %s with Trail %s", req.Round, req.TrialNum)
		return
	}

	// Send the secret value back to the leader
	leaderPeerIDStr := appconfig.Get().LeaderPeerID
	if leaderPeerIDStr == "" {
		log.Fatal("LEADER_PEER_ID is not set in environment variables.")
	}
	leaderPeerID, err := peer.Decode(leaderPeerIDStr)
	if err != nil {
		log.Printf("Failed to decode leader peer ID: %v", err)
		return
	}

	n.SendSecretValue(ctx, h, leaderPeerID, req.Round, req.TrialNum)
}

// SendSecretValue sends the secret value for a round to the leader node
func (n *RegularNode) SendSecretValue(ctx context.Context, h host.Host, leaderPeerID peer.ID, roundNum string, trialNum string) {
	// Check if mocking is enabled - skip P2P send, will submit on-chain
	if appconfig.Get().MockSendSecretToLeader {
		log.Printf("MOCK MODE: Skipping P2P send secret to leader for round %s. Secret will be submitted on-chain when RequestedToSubmitSFromIndexK event is received.", roundNum)
		return
	}

	// Load the commit data for the specified round
	commitData, err := n.regularCommitRepository.GetCommitByRound(ctx, roundNum, trialNum)
	if err != nil {
		log.Printf("Failed to load commit data for round %s with trial %s: %v", roundNum, trialNum, err)
		return
	}

	// Fetch the regular node's private key
	privateKeyHex := appconfig.Get().EOAPrivateKey
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode regular node private key: %v", err)
		return
	}

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	log.Printf("EOA Address: %s", eoaAddress)

	// Sign the round number using the regular node's private key
	signature := utils.SignData(eoaAddress, privateKey)
	// Create the secret value request
	req := utils.SecretValueRequest{
		RegularEoaAddress: eoaAddress, // Regular node's Ethereum address
		Signature:         signature,
		SecretValue:       commitData.SecretValue[:],
		Round:             roundNum,
		TrialNum:          trialNum,
	}

	// Open a stream to the leader node
	stream, err := h.NewStream(ctx, leaderPeerID, "/secretValue")
	if err != nil {
		log.Printf("Failed to create stream to leader node: %v", err)
		return
	}
	defer stream.Close()

	// Send the request
	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(req); err != nil {
		log.Printf("Failed to send secret value request: %v", err)
		return
	}

	log.Printf("Secret value sent for round %s to leader node", roundNum)
}

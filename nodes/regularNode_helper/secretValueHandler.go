package regularNode_helper

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/utils"
)

var strictOrderWhileSecretRequest = make(map[string][]string)

// HandleSecretValueRequest processes secret value requests from the leader node
func HandleSecretValueRequest(h host.Host, s network.Stream) {
	defer s.Close()

	if atomic.LoadInt32(&Halted) == 1 {
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
		roundData, err := database.GetRevealOrder(req.Round, req.TrialNum)
		if err != nil {
			log.Printf("Failed to load reveal order with trial %s for round %s: %v", req.TrialNum, req.Round, err)
		} else if roundData != nil {
			if strictOrderWhileSecretRequest[uniqueKey] == nil {
				strictOrderWhileSecretRequest[uniqueKey] = roundData.OrderedNodes
			}
			break
		} else {
			log.Printf("Reveal order not yet calculated for round %s with trial %s. Waiting...", req.Round, req.TrialNum)
		}
		time.Sleep(5 * time.Second)
	}
	// if req.Order == int(2) {
	// 	return
	// }
	if len(strictOrderWhileSecretRequest[uniqueKey]) == 0 || strictOrderWhileSecretRequest[uniqueKey][req.Order] != req.RegularEoaAddress {
		log.Printf("EOA %s is not next in the reveal order %v for round %s with trial %s", req.RegularEoaAddress, req.Order, req.Round, req.TrialNum)
		return
	}

	// Fetch the leader's EOA address from the environment variables
	leaderEOA := os.Getenv("LEADER_EOA")
	if leaderEOA == "" {
		log.Println("LEADER_EOA is not set in the environment variables")
		return
	}

	// Use the existing signature verification mechanism
	verifyReq := utils.RegistrationRequest{
		EOAAddress: req.LeaderEoaAddress, // Sender's address
		Signature:  req.Signature,        // Signature
	}

	// Verify the signature
	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for secret value request: expected %s, got %s", leaderEOA, req.LeaderEoaAddress)
		return
	}

	// Log the request details
	// log.Printf("Verified secret value request for round %s with trial %s from leader %s", req.Round, req.TrialNum, req.LeaderEoaAddress)

	// Fetch the secret value for the specified round
	commitData, err := database.GetCommitByRound(req.Round, req.TrialNum)
	if err != nil {
		log.Printf("Failed to load commit data for round %s with trial %s: %v", req.Round, req.TrialNum, err)
		return
	}

	// Check if the secret value exists
	if commitData.SecretValue == [32]byte{} {
		log.Printf("No secret value found for round %s with trial %s", req.Round, req.TrialNum)
		return
	}

	// Send the secret value back to the leader
	leaderPeerIDStr := os.Getenv("LEADER_PEER_ID")
	if leaderPeerIDStr == "" {
		log.Fatal("LEADER_PEER_ID is not set in environment variables.")
	}
	leaderPeerID, err := peer.Decode(leaderPeerIDStr)
	if err != nil {
		log.Printf("Failed to decode leader peer ID: %v", err)
		return
	}

	SendSecretValue(h, leaderPeerID, req.Round, req.TrialNum)
}

// SendSecretValue sends the secret value for a round to the leader node
func SendSecretValue(h host.Host, leaderPeerID peer.ID, roundNum string, trialNum string) {
	// Load the commit data for the specified round
	commitData, err := database.GetCommitByRound(roundNum, trialNum)
	if err != nil {
		log.Printf("Failed to load commit data for round %s with trial %s: %v", roundNum, trialNum, err)
		return
	}

	// Fetch the regular node's private key
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode regular node private key: %v", err)
		return
	}

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	// log.Printf("EOA Address: %s", eoaAddress)

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
	stream, err := h.NewStream(context.Background(), leaderPeerID, "/secretValue")
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

	log.Printf("\033[32mSecret value sent for round %s to leader node\033[0m", roundNum)
}

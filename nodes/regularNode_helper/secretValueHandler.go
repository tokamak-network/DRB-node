package regularNode_helper

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/tokamak-network/DRB-node/utils"
)

// HandleSecretValueRequest processes secret value requests from the leader node
func HandleSecretValueRequest(h host.Host, s network.Stream) {
	defer s.Close()

	// Decode the request
	var req utils.SecretValueRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode secret value request: %v", err)
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
		EOAAddress: req.EOAAddress, // Sender's address
		Signature:  req.Signature,  // Signature
	}

	// Verify the signature
	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for secret value request: expected %s, got %s", leaderEOA, req.EOAAddress)
		return
	}

	// Log the request details
	log.Printf("Verified secret value request for round %s from leader %s", req.Round, req.EOAAddress)

	// Fetch the secret value for the specified round
	commitData, err := utils.LoadCommitData(req.Round)
	if err != nil {
		log.Printf("Failed to load commit data for round %s: %v", req.Round, err)
		return
	}

	// Check if the secret value exists
	if commitData.SecretValue == [32]byte{} {
		log.Printf("No secret value found for round %s", req.Round)
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

	SendSecretValue(h, leaderPeerID, req.Round)
}

// SendSecretValue sends the secret value for a round to the leader node
func SendSecretValue(h host.Host, leaderPeerID peer.ID, roundNum string) {
	// Load the commit data for the specified round
	commitData, err := utils.LoadCommitData(roundNum)
	if err != nil {
		log.Printf("Failed to load commit data for round %s: %v", roundNum, err)
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
	log.Printf("EOA Address: %s", eoaAddress)

	// Sign the round number using the regular node's private key
	signature := utils.SignData(eoaAddress, privateKey)

	// Create the secret value request
	req := utils.SecretValueRequest{
		EOAAddress:  eoaAddress, // Regular node's Ethereum address
		Signature:   signature,
		SecretValue: commitData.SecretValue[:],
		Round:       roundNum,
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

	log.Printf("Secret value sent for round %s to leader node", roundNum)
}

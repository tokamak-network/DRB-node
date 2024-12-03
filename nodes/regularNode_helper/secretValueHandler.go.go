package regularNode_helper

import (
	"encoding/json"
	"log"
	"os"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/utils"
)

// HandleSecretValueRequest processes secret value requests from the leader node
func HandleSecretValueRequest(s network.Stream) {
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
	commitData, err := utils.LOAD_COMMIT_DATA(req.Round)
	if err != nil {
		log.Printf("Failed to load commit data for round %s: %v", req.Round, err)
		return
	}

	// Log the secret value if it exists
	if commitData.SecretValue == [32]byte{} {
		log.Printf("No secret value found for round %s", req.Round)
	} else {
		log.Printf("Secret value for round %s: 0x%x", req.Round, commitData.SecretValue)
	}
}


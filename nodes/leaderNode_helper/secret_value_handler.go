package leaderNode_helper

import (
	"encoding/hex"
	"encoding/json"
	"log"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/utils"
)

// AcceptSecretValue processes and stores secret values sent by regular nodes.
func AcceptSecretValue(h host.Host, s network.Stream) {
	defer s.Close()

	// Decode the incoming request
	var req utils.SecretValueRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode secret value request: %v", err)
		return
	}

	// Verify the EOA signature
	verifyReq := utils.RegistrationRequest{
		EOAAddress: req.EOAAddress,
		Signature:  req.Signature,
	}

	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for secret value request from EOA: %s", req.EOAAddress)
		return
	}

	log.Printf("Successfully verified signature for EOA: %s", req.EOAAddress)

	// Fetch or initialize the leader commit data for the given round and EOA
	leaderCommitData, err := database.GetLeaderCommitByRoundAndEoaAddr(req.Round, req.EOAAddress)
	if err != nil {
		log.Printf("Commit data not found, initializing new entry for round %s and EOA %s", req.Round, req.EOAAddress)
		leaderCommitData = &utils.LeaderCommitData{
			Round:      req.Round,
			EOAAddress: req.EOAAddress,
		}
	}

	// Store the secret value in both byte array and hex string formats
	copy(leaderCommitData.SecretValue[:], req.SecretValue[:])
	leaderCommitData.SecretValueHex = hex.EncodeToString(req.SecretValue[:])

	log.Printf("Received secret value for round %s and EOA %s: byte=%x, hex=%s",
		req.Round, req.EOAAddress, leaderCommitData.SecretValue, leaderCommitData.SecretValueHex)

	if err := database.UpdateLeaderCommit(leaderCommitData); err != nil {
		log.Printf("Failed to save updated commit data for %s in round %s: %v", req.EOAAddress, req.Round, err)
		return
	}

	log.Printf("Successfully saved secret value for round %s and EOA %s", req.Round, req.EOAAddress)

	// Continue requesting secret values from remaining nodes in the reveal order
	HandleSecretValueResponse(h, req.Round, req.EOAAddress)
}

package regularNode_helper

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

var strictOrderWhileSecretRequest = make(map[string][]string)

// HandleSecretValueRequest processes secret value requests from the leader node
func HandleSecretValueRequest(h host.Host, s network.Stream) {
	defer s.Close()

	// Decode the request
	var req utils.SecretValueRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		logger.Infof("Failed to decode secret value request: %v", err)
		return
	}
	filePath := "regular_reveal_order.json"
	for {
		data, err := commitreveal2.LoadRevealOrders(filePath)
		if err != nil {
			logger.Infof("Failed to load reveal order: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		roundData, exists := data[CurrentRound]
		if exists {
			if strictOrderWhileSecretRequest[CurrentRound] == nil {
				rawOrderedNodes := roundData.(map[string]interface{})["ordered_nodes"].([]interface{})
				orderedNodes := make([]string, len(rawOrderedNodes))
				for i, v := range rawOrderedNodes {
					orderedNodes[i] = v.(string)
				}
				strictOrderWhileSecretRequest[CurrentRound] = orderedNodes
			}
			break
		}

		logger.Infof("Reveal order not yet calculated for round %s. Waiting...", CurrentRound)
		time.Sleep(2 * time.Second)
	}
	if len(strictOrderWhileSecretRequest[CurrentRound]) == 0 || strictOrderWhileSecretRequest[CurrentRound][req.Order] != req.RegularEoaAddress {
		logger.Infof("EOA %s is not next in the reveal order %v for round %s", req.RegularEoaAddress, req.Order, CurrentRound)
		return
	}

	// Fetch the leader's EOA address from the environment variables
	leaderEOA := os.Getenv("LEADER_EOA")
	if leaderEOA == "" {
		logger.Info("LEADER_EOA is not set in the environment variables")
		return
	}

	// Use the existing signature verification mechanism
	verifyReq := utils.RegistrationRequest{
		EOAAddress: req.LeaderEoaAddress, // Sender's address
		Signature:  req.Signature,        // Signature
	}

	// Verify the signature
	if !utils.VerifySignature(verifyReq) {
		logger.Infof("Signature verification failed for secret value request: expected %s, got %s", leaderEOA, req.LeaderEoaAddress)
		return
	}

	// Log the request details
	logger.Infof("Verified secret value request for round %s from leader %s", CurrentRound, req.LeaderEoaAddress)

	// Fetch the secret value for the specified round
	commitData, err := utils.LoadCommitData(CurrentRound)
	if err != nil {
		logger.Infof("Failed to load commit data for round %s: %v", CurrentRound, err)
		return
	}

	// Check if the secret value exists
	if commitData.SecretValue == [32]byte{} {
		logger.Infof("No secret value found for round %s", CurrentRound)
		return
	}

	// Send the secret value back to the leader
	leaderPeerIDStr := os.Getenv("LEADER_PEER_ID")
	if leaderPeerIDStr == "" {
		logger.Fatal("LEADER_PEER_ID is not set in environment variables.")
	}
	leaderPeerID, err := peer.Decode(leaderPeerIDStr)
	if err != nil {
		logger.Infof("Failed to decode leader peer ID: %v", err)
		return
	}

	SendSecretValue(h, leaderPeerID, CurrentRound)
}

// SendSecretValue sends the secret value for a round to the leader node
func SendSecretValue(h host.Host, leaderPeerID peer.ID, roundNum string) {
	// Load the commit data for the specified round
	commitData, err := utils.LoadCommitData(roundNum)
	if err != nil {
		logger.Infof("Failed to load commit data for round %s: %v", roundNum, err)
		return
	}

	// Fetch the regular node's private key
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		logger.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		logger.Infof("Failed to decode regular node private key: %v", err)
		return
	}

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	logger.Infof("EOA Address: %s", eoaAddress)

	// Sign the round number using the regular node's private key
	signature := utils.SignData(eoaAddress, privateKey)

	// Create the secret value request
	req := utils.SecretValueRequest{
		RegularEoaAddress: eoaAddress, // Regular node's Ethereum address
		Signature:         signature,
		SecretValue:       commitData.SecretValue[:],
		Round:             roundNum,
	}

	// Open a stream to the leader node
	stream, err := h.NewStream(context.Background(), leaderPeerID, "/secretValue")
	if err != nil {
		logger.Infof("Failed to create stream to leader node: %v", err)
		return
	}
	defer stream.Close()

	// Send the request
	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(req); err != nil {
		logger.Infof("Failed to send secret value request: %v", err)
		return
	}

	logger.Infof("Secret value sent for round %s to leader node", roundNum)
}

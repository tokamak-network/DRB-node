package leaderNode_helper

import (
	"encoding/hex"
	"encoding/json"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// var SecretValue [][32]byte
var RoundSecrets = make(map[string][][32]byte)

// AcceptSecretValue processes and stores secret values sent by regular nodes.
func AcceptSecretValue(h host.Host, s network.Stream, fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	defer s.Close()

	// Decode the incoming request
	var req utils.SecretValueRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		logger.Infof("Failed to decode secret value request: %v", err)
		return
	}

	// Verify the EOA signature
	verifyReq := utils.RegistrationRequest{
		EOAAddress: req.RegularEoaAddress,
		Signature:  req.Signature,
	}

	if !utils.VerifySignature(verifyReq) {
		logger.Infof("Signature verification failed for secret value request from EOA: %s", req.RegularEoaAddress)
		return
	}

	logger.Infof("Successfully verified signature for EOA: %s", req.RegularEoaAddress)

	// Fetch or initialize the leader commit data for the given round and EOA
	commitData, err := utils.LoadLeaderCommitData(req.Round, req.RegularEoaAddress)
	if err != nil {
		logger.Infof("Commit data not found, initializing new entry for round %s and EOA %s", req.Round, req.RegularEoaAddress)
		commitData = &utils.LeaderCommitData{
			Round:      req.Round,
			EOAAddress: req.RegularEoaAddress,
		}
	}
	var secretValueArray [32]byte
	copy(secretValueArray[:], req.SecretValue[:]) // Convert req.SecretValue to [32]byte
	if _, exists := RoundSecrets[req.Round]; !exists {
		RoundSecrets[req.Round] = make([][32]byte, 0)
	}
	RoundSecrets[req.Round] = append(RoundSecrets[req.Round], secretValueArray)
	// Store the secret value in both byte array and hex string formats
	copy(commitData.SecretValue[:], req.SecretValue[:])
	commitData.SecretValueHex = hex.EncodeToString(req.SecretValue[:])

	logger.Infof("Received and stored secret value for round %s and EOA %s: byte=%x, hex=%s",
		req.Round, req.RegularEoaAddress, commitData.SecretValue, commitData.SecretValueHex)

	// Save the updated commit data
	if err := utils.SaveLeaderCommitData(*commitData); err != nil {
		logger.Infof("Failed to save leader commit data for round %s and EOA %s: %v", req.Round, req.RegularEoaAddress, err)
		return
	}

	logger.Infof("Successfully saved secret value for round %s and EOA %s", req.Round, req.RegularEoaAddress)
	broadCastS(h, req.Round, req.RegularEoaAddress, commitData.SecretValue)
	// Continue requesting secret values from remaining nodes in the reveal order
	HandleSecretValueResponse(h, fallbackEthClient, req.Round, req.RegularEoaAddress)
}

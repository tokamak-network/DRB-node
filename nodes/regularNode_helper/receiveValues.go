package regularNode_helper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/tokamak-network/DRB-node/utils"
)

var peerNodeInfo map[string]utils.PeerCommitData
var CosRecevied = make(map[string]map[string]bool)

// Global variable to store the regular node's EOA address
var regularNodeEOA string

// SetRegularNodeEOA sets the regular node's EOA address
func SetRegularNodeEOA(eoa string) {
	regularNodeEOA = eoa
}

// getRegularNodeEOA returns the regular node's own EOA address
func getRegularNodeEOA() string {
	if regularNodeEOA == "" {
		log.Printf("Regular node EOA address not set")
		return ""
	}
	return regularNodeEOA
}

// sendAcknowledgment sends an acknowledgment back to the leader
func sendAcknowledgment(h host.Host, leaderPeerID peer.ID, ack utils.AcknowledgmentMessage) {
	stream, err := h.NewStream(context.Background(), leaderPeerID, protocol.ID("/acknowledgment"))
	if err != nil {
		log.Printf("Failed to create acknowledgment stream: %v", err)
		return
	}
	defer stream.Close()

	if err := json.NewEncoder(stream).Encode(ack); err != nil {
		log.Printf("Failed to send acknowledgment: %v", err)
	} else {
		log.Printf("Acknowledgment sent for %s broadcast (message ID: %s)", ack.Type, ack.MessageID)
	}
}

// HandleCvs processes incoming CVS values and sends acknowledgment
func HandleCvs(h host.Host, s network.Stream) {
	defer s.Close()

	var message utils.BroadcastMessage
	if err := json.NewDecoder(s).Decode(&message); err != nil {
		log.Printf("Failed to decode CVS broadcast message: %v", err)
		return
	}

	log.Printf("Received CVS broadcast for round %s from EOA %s (message ID: %s)",
		message.Round, message.EOAAddress, message.MessageID)

	// Process the CVS data
	peerCommitData := utils.PeerCommitData{
		Round:      message.Round,
		EOAAddress: message.EOAAddress,
		Cvs:        message.Data,
	}

	// Save to file
	filePath := "peerNodeInfo.json"
	if _, err := os.Stat(filePath); err == nil {
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Failed to open peerNodeInfo.json: %v", err)
			return
		}
		defer file.Close()

		if err := json.NewDecoder(file).Decode(&peerNodeInfo); err != nil {
			log.Printf("Failed to decode peerNodeInfo.json: %v", err)
			return
		}
	} else if os.IsNotExist(err) {
		log.Printf("peerNodeInfo.json does not exist. Creating a new file.")
		peerNodeInfo = make(map[string]utils.PeerCommitData)
	} else {
		log.Printf("Error checking peerNodeInfo.json: %v", err)
		return
	}

	key := fmt.Sprintf("%s+%s", message.Round, message.EOAAddress)
	peerNodeInfo[key] = peerCommitData

	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create peerNodeInfo.json: %v", err)
		return
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(peerNodeInfo); err != nil {
		log.Printf("Failed to write to peerNodeInfo.json: %v", err)
		return
	}

	log.Printf("Successfully saved CVS data for round %s and EOA %s into peerNodeInfo.json", message.Round, message.EOAAddress)

	// Send acknowledgment
	ack := utils.AcknowledgmentMessage{
		Round:      message.Round,
		EOAAddress: getRegularNodeEOA(),
		MessageID:  message.MessageID,
		Type:       message.Type,
		Status:     "received",
	}

	// Get leader peer ID from environment or connection
	leaderPeerIDStr := os.Getenv("LEADER_PEER_ID")
	if leaderPeerIDStr == "" {
		log.Printf("LEADER_PEER_ID not set, cannot send acknowledgment")
		return
	}

	leaderPeerID, err := peer.Decode(leaderPeerIDStr)
	if err != nil {
		log.Printf("Failed to decode leader peer ID: %v", err)
		return
	}

	sendAcknowledgment(h, leaderPeerID, ack)
}

// HandleCos processes incoming COS values and sends acknowledgment
func HandleCos(h host.Host, s network.Stream) {
	defer s.Close()

	var message utils.BroadcastMessage
	if err := json.NewDecoder(s).Decode(&message); err != nil {
		log.Printf("Failed to decode COS broadcast message: %v", err)
		return
	}

	log.Printf("Received COS broadcast for round %s from EOA %s (message ID: %s)",
		message.Round, message.EOAAddress, message.MessageID)

	// Process the COS data
	if CosRecevied[message.Round] == nil {
		CosRecevied[message.Round] = make(map[string]bool)
	}
	CosRecevied[message.Round][message.EOAAddress] = true

	filePath := "peerNodeInfo.json"
	if _, err := os.Stat(filePath); err == nil {
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Failed to open peerNodeInfo.json: %v", err)
			return
		}
		defer file.Close()

		if err := json.NewDecoder(file).Decode(&peerNodeInfo); err != nil {
			log.Printf("Failed to decode peerNodeInfo.json: %v", err)
			return
		}
	} else if os.IsNotExist(err) {
		log.Printf("peerNodeInfo.json does not exist. Creating a new file.")
		peerNodeInfo = make(map[string]utils.PeerCommitData)
	} else {
		log.Printf("Error checking peerNodeInfo.json: %v", err)
		return
	}

	key := fmt.Sprintf("%s+%s", message.Round, message.EOAAddress)
	data := peerNodeInfo[key]
	data.Cos = message.Data
	peerNodeInfo[key] = data

	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create peerNodeInfo.json: %v", err)
		return
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(peerNodeInfo); err != nil {
		log.Printf("Failed to write to peerNodeInfo.json: %v", err)
		return
	}

	log.Printf("Successfully saved COS data for round %s and EOA %s into peerNodeInfo.json", message.Round, message.EOAAddress)

	// Send acknowledgment
	ack := utils.AcknowledgmentMessage{
		Round:      message.Round,
		EOAAddress: getRegularNodeEOA(),
		MessageID:  message.MessageID,
		Type:       message.Type,
		Status:     "received",
	}

	// Get leader peer ID from environment or connection
	leaderPeerIDStr := os.Getenv("LEADER_PEER_ID")
	if leaderPeerIDStr == "" {
		log.Printf("LEADER_PEER_ID not set, cannot send acknowledgment")
		return
	}

	leaderPeerID, err := peer.Decode(leaderPeerIDStr)
	if err != nil {
		log.Printf("Failed to decode leader peer ID: %v", err)
		return
	}

	sendAcknowledgment(h, leaderPeerID, ack)
}

// HandleSecret processes incoming secret values and sends acknowledgment
func HandleSecret(h host.Host, s network.Stream) {
	defer s.Close()

	var message utils.BroadcastMessage
	if err := json.NewDecoder(s).Decode(&message); err != nil {
		log.Printf("Failed to decode secret broadcast message: %v", err)
		return
	}

	log.Printf("Received secret broadcast for round %s from EOA %s (message ID: %s)",
		message.Round, message.EOAAddress, message.MessageID)

	// Process the secret data
	filePath := "peerNodeInfo.json"
	if _, err := os.Stat(filePath); err == nil {
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Failed to open peerNodeInfo.json: %v", err)
			return
		}
		defer file.Close()

		if err := json.NewDecoder(file).Decode(&peerNodeInfo); err != nil {
			log.Printf("Failed to decode peerNodeInfo.json: %v", err)
			return
		}
	} else if os.IsNotExist(err) {
		log.Printf("peerNodeInfo.json does not exist. Creating a new file.")
		peerNodeInfo = make(map[string]utils.PeerCommitData)
	} else {
		log.Printf("Error checking peerNodeInfo.json: %v", err)
		return
	}

	key := fmt.Sprintf("%s+%s", message.Round, message.EOAAddress)
	data := peerNodeInfo[key]
	data.SecretValue = message.Data
	peerNodeInfo[key] = data

	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create peerNodeInfo.json: %v", err)
		return
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(peerNodeInfo); err != nil {
		log.Printf("Failed to write to peerNodeInfo.json: %v", err)
		return
	}

	log.Printf("Successfully saved secret data for round %s and EOA %s into peerNodeInfo.json", message.Round, message.EOAAddress)

	// Send acknowledgment
	ack := utils.AcknowledgmentMessage{
		Round:      message.Round,
		EOAAddress: getRegularNodeEOA(),
		MessageID:  message.MessageID,
		Type:       message.Type,
		Status:     "received",
	}

	// Get leader peer ID from environment or connection
	leaderPeerIDStr := os.Getenv("LEADER_PEER_ID")
	if leaderPeerIDStr == "" {
		log.Printf("LEADER_PEER_ID not set, cannot send acknowledgment")
		return
	}

	leaderPeerID, err := peer.Decode(leaderPeerIDStr)
	if err != nil {
		log.Printf("Failed to decode leader peer ID: %v", err)
		return
	}

	sendAcknowledgment(h, leaderPeerID, ack)
}

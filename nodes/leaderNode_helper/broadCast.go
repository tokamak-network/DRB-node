package leaderNode_helper

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/utils"
)

var broadcastMutex sync.Mutex
var activeBroadcasts = make(map[string]*utils.BroadcastTracker)

// ReliableBroadCastS broadcasts secret values with acknowledgment tracking
func ReliableBroadCastS(h host.Host, roundNum string, eoaAddress string, secret [32]byte) {
	messageID := utils.GenerateMessageID(roundNum, eoaAddress, "secret")

	tracker := &utils.BroadcastTracker{
		Round:        roundNum,
		EOAAddress:   eoaAddress,
		Type:         "secret",
		MessageID:    messageID,
		Data:         secret,
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     time.Now().Unix(),
		Timeout:      30, // 30 seconds timeout
	}

	for _, op := range eth.ActivatedOperators {
		tracker.Acknowledged[op.Hex()] = false
	}

	if err := utils.SaveBroadcastTracker(*tracker); err != nil {
		log.Printf("Failed to save broadcast tracker: %v", err)
		return
	}

	broadcastMutex.Lock()
	activeBroadcasts[messageID] = tracker
	broadcastMutex.Unlock()

	// Start the broadcast process
	go performReliableBroadcast(h, tracker, "secret")
}

// ReliableBroadCastCOS broadcasts COS values with acknowledgment tracking
func ReliableBroadCastCOS(h host.Host, roundNum string, eoaAddress common.Address, cos [32]byte) {
	messageID := utils.GenerateMessageID(roundNum, eoaAddress.Hex(), "cos")

	tracker := &utils.BroadcastTracker{
		Round:        roundNum,
		EOAAddress:   eoaAddress.Hex(),
		Type:         "cos",
		MessageID:    messageID,
		Data:         cos,
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     time.Now().Unix(),
		Timeout:      30, // 30 seconds timeout
	}

	for _, op := range eth.ActivatedOperators {
		tracker.Acknowledged[op.Hex()] = false
	}

	if err := utils.SaveBroadcastTracker(*tracker); err != nil {
		log.Printf("Failed to save broadcast tracker: %v", err)
		return
	}

	broadcastMutex.Lock()
	activeBroadcasts[messageID] = tracker
	broadcastMutex.Unlock()

	// Start the broadcast process
	go performReliableBroadcast(h, tracker, "cos")
}

// ReliableBroadCastCVS broadcasts CVS values with acknowledgment tracking
func ReliableBroadCastCVS(h host.Host, roundNum string, eoaAddress common.Address, cvs [32]byte) {
	messageID := utils.GenerateMessageID(roundNum, eoaAddress.Hex(), "cvs")

	tracker := &utils.BroadcastTracker{
		Round:        roundNum,
		EOAAddress:   eoaAddress.Hex(),
		Type:         "cvs",
		MessageID:    messageID,
		Data:         cvs,
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     time.Now().Unix(),
		Timeout:      30, // 30 seconds timeout
	}

	for _, op := range eth.ActivatedOperators {
		tracker.Acknowledged[op.Hex()] = false
	}

	if err := utils.SaveBroadcastTracker(*tracker); err != nil {
		log.Printf("Failed to save broadcast tracker: %v", err)
		return
	}

	broadcastMutex.Lock()
	activeBroadcasts[messageID] = tracker
	broadcastMutex.Unlock()

	// Start the broadcast process
	go performReliableBroadcast(h, tracker, "cvs")
}

// performReliableBroadcast handles the actual broadcasting with retry logic
func performReliableBroadcast(h host.Host, tracker *utils.BroadcastTracker, broadcastType string) {
	nodeInfo := libp2putils.GetConnectedPeers()

	for tracker.Attempts < tracker.MaxAttempts {
		tracker.Attempts++
		tracker.LastSent = time.Now().Unix()

		log.Printf("Broadcasting %s (attempt %d/%d) for round %s, EOA %s",
			broadcastType, tracker.Attempts, tracker.MaxAttempts, tracker.Round, tracker.EOAAddress)

		// Create broadcast message
		message := utils.BroadcastMessage{
			Round:      tracker.Round,
			EOAAddress: tracker.EOAAddress,
			MessageID:  tracker.MessageID,
			Type:       broadcastType,
			Data:       tracker.Data,
		}

		// Determine stream protocol based on type
		var streamProtocol protocol.ID
		switch broadcastType {
		case "cvs":
			streamProtocol = protocol.ID("/cvsBroadcast")
		case "cos":
			streamProtocol = protocol.ID("/cosBroadcast")
		case "secret":
			streamProtocol = protocol.ID("/secretBroadcast")
		default:
			log.Printf("Unknown broadcast type: %s", broadcastType)
			return
		}

		// Send to all activated operators
		for _, op := range eth.ActivatedOperators {
			if tracker.Acknowledged[op.Hex()] {
				continue // Skip already acknowledged nodes
			}

			stream, err := h.NewStream(context.Background(), nodeInfo[op.Hex()].PeerID, streamProtocol)
			if err != nil {
				log.Printf("Failed to create stream to peer %s: %v", nodeInfo[op.Hex()].PeerID, err)
				continue
			}

			if err := json.NewEncoder(stream).Encode(message); err != nil {
				log.Printf("Failed to send %s to regular node %s: %v", broadcastType, op.Hex(), err)
			} else {
				log.Printf("%s sent to regular node %s for round %s (attempt %d)",
					broadcastType, op.Hex(), tracker.Round, tracker.Attempts)
			}
			stream.Close()
		}

		// Update tracker
		if err := utils.UpdateBroadcastTracker(*tracker); err != nil {
			log.Printf("Failed to update broadcast tracker: %v", err)
		}

		// Wait for acknowledgments or timeout
		time.Sleep(time.Duration(tracker.Timeout) * time.Second)

		// Check if all nodes have acknowledged
		allAcknowledged := true
		for _, acknowledged := range tracker.Acknowledged {
			if !acknowledged {
				allAcknowledged = false
				break
			}
		}

		if allAcknowledged {
			log.Printf("All nodes acknowledged %s for round %s, EOA %s",
				broadcastType, tracker.Round, tracker.EOAAddress)
			break
		}

		// If this was the last attempt, log unacknowledged nodes
		if tracker.Attempts >= tracker.MaxAttempts {
			log.Printf("Max attempts reached for %s broadcast. Unacknowledged nodes:", broadcastType)
			for eoa, acknowledged := range tracker.Acknowledged {
				if !acknowledged {
					log.Printf("  - %s", eoa)
				}
			}
		}
	}

	// Clean up from memory
	broadcastMutex.Lock()
	delete(activeBroadcasts, tracker.MessageID)
	broadcastMutex.Unlock()
}

// HandleAcknowledgment processes acknowledgments from regular nodes
func HandleAcknowledgment(ack utils.AcknowledgmentMessage) {
	broadcastMutex.Lock()
	defer broadcastMutex.Unlock()

	tracker, exists := activeBroadcasts[ack.MessageID]
	if !exists {
		log.Printf("Received acknowledgment for unknown message ID: %s", ack.MessageID)
		return
	}

	if ack.Status == "received" {
		// Mark acknowledgment from the sender (regular node that sent the ack)
		tracker.Acknowledged[ack.EOAAddress] = true
		log.Printf("Received acknowledgment from %s for %s broadcast (message ID: %s)",
			ack.EOAAddress, ack.Type, ack.MessageID)
	} else {
		log.Printf("Received error acknowledgment from %s for %s broadcast (message ID: %s): %s",
			ack.EOAAddress, ack.Type, ack.MessageID, ack.Status)
	}

	// Update tracker
	if err := utils.UpdateBroadcastTracker(*tracker); err != nil {
		log.Printf("Failed to update broadcast tracker: %v", err)
	}
}

// StartBroadcastCleanup starts a goroutine to clean up old broadcast trackers
func StartBroadcastCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute) // Clean up every 5 minutes
		defer ticker.Stop()

		for range ticker.C {
			cleanupOldBroadcasts()
		}
	}()
}

// cleanupOldBroadcasts removes broadcast trackers older than 1 hour
func cleanupOldBroadcasts() {
	broadcastMutex.Lock()
	defer broadcastMutex.Unlock()

	currentTime := time.Now().Unix()
	cutoffTime := currentTime - 3600 // 1 hour ago

	// Clean up in-memory trackers
	for messageID, tracker := range activeBroadcasts {
		if tracker.LastSent < cutoffTime {
			delete(activeBroadcasts, messageID)
			log.Printf("Cleaned up old broadcast tracker: %s", messageID)
		}
	}

	// Clean up file-based trackers
	trackers, err := utils.LoadBroadcastTrackers()
	if err != nil {
		log.Printf("Failed to load broadcast trackers for cleanup: %v", err)
		return
	}

	cleaned := false
	for key, tracker := range trackers {
		if tracker.LastSent < cutoffTime {
			delete(trackers, key)
			cleaned = true
		}
	}

	if cleaned {
		// Save cleaned trackers back to file
		file, err := os.Create("broadcast_trackers.json")
		if err != nil {
			log.Printf("Failed to create broadcast tracker file for cleanup: %v", err)
			return
		}
		defer file.Close()

		encoder := json.NewEncoder(file)
		if err := encoder.Encode(trackers); err != nil {
			log.Printf("Failed to encode cleaned broadcast trackers: %v", err)
		}
	}
}

package leaderNode_helper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/utils"
)

var broadcastMutex sync.Mutex
var activeBroadcasts = make(map[string]*utils.BroadcastTracker)

// ReliableBroadCastS broadcasts secret values with acknowledgment tracking
func ReliableBroadCastS(h host.Host, roundNum string, trialNum string, eoaAddress string, secret [32]byte) {
	messageID := generateMessageID(roundNum, trialNum, eoaAddress, "secret")

	tracker := &utils.BroadcastTracker{
		Round:        roundNum,
		TrialNum:     trialNum,
		EOAAddress:   eoaAddress,
		Type:         "secret",
		MessageID:    messageID,
		Data:         secret,
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     time.Now().Unix(),
		Timeout:      3, // 30 seconds timeout
	}

	for _, op := range eth.ActivatedOperators {
		tracker.Acknowledged[op.Hex()] = false
	}

	if err := database.AddBroadcastTracker(tracker); err != nil {
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
func ReliableBroadCastCOS(h host.Host, roundNum string, trialNum string, eoaAddress common.Address, cos [32]byte) {
	messageID := generateMessageID(roundNum, trialNum, eoaAddress.Hex(), "cos")

	tracker := &utils.BroadcastTracker{
		Round:        roundNum,
		TrialNum:     trialNum,
		EOAAddress:   eoaAddress.Hex(),
		Type:         "cos",
		MessageID:    messageID,
		Data:         cos,
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     time.Now().Unix(),
		Timeout:      3, // 30 seconds timeout
	}

	for _, op := range eth.ActivatedOperators {
		tracker.Acknowledged[op.Hex()] = false
	}

	if err := database.AddBroadcastTracker(tracker); err != nil {
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
func ReliableBroadCastCVS(h host.Host, roundNum string, trialNum string, eoaAddress common.Address, cvs [32]byte) {
	messageID := generateMessageID(roundNum, trialNum, eoaAddress.Hex(), "cvs")

	tracker := &utils.BroadcastTracker{
		Round:        roundNum,
		TrialNum:     trialNum,
		EOAAddress:   eoaAddress.Hex(),
		Type:         "cvs",
		MessageID:    messageID,
		Data:         cvs,
		Attempts:     0,
		MaxAttempts:  3,
		Acknowledged: make(map[string]bool),
		LastSent:     time.Now().Unix(),
		Timeout:      3, // 3 seconds timeout
	}

	for _, op := range eth.ActivatedOperators {
		tracker.Acknowledged[op.Hex()] = false
	}

	if err := database.AddBroadcastTracker(tracker); err != nil {
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
	if atomic.LoadInt32(&Halted) == 1 {
		broadcastMutex.Lock()
		delete(activeBroadcasts, tracker.MessageID)
		broadcastMutex.Unlock()
		log.Println("System is halted. Skipping processCVS.")
		return
	}
	nodeInfo := libp2putils.GetConnectedPeers()

	for tracker.Attempts < tracker.MaxAttempts {
		tracker.Attempts++
		tracker.LastSent = time.Now().Unix()

		log.Printf("Broadcasting %s (attempt %d/%d) for round %s, trail %s, EOA %s",
			broadcastType, tracker.Attempts, tracker.MaxAttempts, tracker.Round, tracker.TrialNum, tracker.EOAAddress)

		// Create broadcast message
		message := utils.BroadcastMessage{
			Round:      tracker.Round,
			TrialNum:   tracker.TrialNum,
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
		broadcastMutex.Lock()
		operatorsToSend := make([]common.Address, 0)
		for _, op := range eth.ActivatedOperators {
			if tracker.Acknowledged[op.Hex()] {
				continue // Skip already acknowledged nodes
			}
			operatorsToSend = append(operatorsToSend, op)
		}
		broadcastMutex.Unlock()

		for _, op := range operatorsToSend {
			// Add peer info into peer store
			peerID := nodeInfo[op.Hex()].PeerID
			peerAddrStr := fmt.Sprintf("/ip4/%s/tcp/%s", nodeInfo[op.Hex()].IP, nodeInfo[op.Hex()].Port)
			peerAddr, _ := multiaddr.NewMultiaddr(peerAddrStr)
			h.Peerstore().AddAddr(peerID, peerAddr, peerstore.PermanentAddrTTL)

			stream, err := h.NewStream(context.Background(), nodeInfo[op.Hex()].PeerID, streamProtocol)
			if err != nil {
				log.Printf("Failed to create stream to peer %s: %v", nodeInfo[op.Hex()].PeerID, err)
				continue
			}

			if err := json.NewEncoder(stream).Encode(message); err != nil {
				log.Printf("Failed to send %s to regular node %s: %v", broadcastType, op.Hex(), err)
			} else {
				log.Printf("%s sent to regular node %s for round %s with trail %s (attempt %d)",
					broadcastType, op.Hex(), tracker.Round, tracker.TrialNum, tracker.Attempts)
			}
			stream.Close()
		}

		// Update tracker
		if err := database.UpdateBroadcastTracker(tracker); err != nil {
			log.Printf("Failed to update broadcast tracker: %v", err)
		}

		// Wait for acknowledgments or timeout
		time.Sleep(time.Duration(tracker.Timeout) * time.Second)

		// Check if all nodes have acknowledged
		broadcastMutex.Lock()
		allAcknowledged := true
		for _, acknowledged := range tracker.Acknowledged {
			if !acknowledged {
				allAcknowledged = false
				break
			}
		}

		if allAcknowledged {
			broadcastMutex.Unlock()
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
		broadcastMutex.Unlock()
	}

	// Clean up from memory
	broadcastMutex.Lock()
	delete(activeBroadcasts, tracker.MessageID)
	broadcastMutex.Unlock()
}

// HandleAcknowledgment processes acknowledgments from regular nodes
func HandleAcknowledgment(ack utils.AcknowledgmentMessage) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping HandleAcknowledgment.")
		return
	}
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

		// Check if all nodes have acknowledged
		allAcknowledged := true
		for _, acknowledged := range tracker.Acknowledged {
			if !acknowledged {
				allAcknowledged = false
				break
			}
		}

		// Log immediately when all nodes have acknowledged
		if allAcknowledged {
			log.Printf("All nodes acknowledged %s for round %s, EOA %s",
				ack.Type, tracker.Round, tracker.EOAAddress)
		}
	} else {
		log.Printf("Received error acknowledgment from %s for %s broadcast (message ID: %s): %s",
			ack.EOAAddress, ack.Type, ack.MessageID, ack.Status)
	}

	// Update tracker
	if err := database.UpdateBroadcastTracker(tracker); err != nil {
		log.Printf("Failed to update broadcast tracker: %v", err)
	}
}

// // StartBroadcastCleanup starts a goroutine to clean up old broadcast trackers
// func StartBroadcastCleanup() {
// 	go func() {
// 		ticker := time.NewTicker(5 * time.Minute) // Clean up every 5 minutes
// 		defer ticker.Stop()

// 		for range ticker.C {
// 			cleanupOldBroadcasts()
// 		}
// 	}()
// }
// func StartLeaderCommitCleanup() {
// 	go func() {
// 		ticker := time.NewTicker(5 * time.Minute) // Clean up every 5 minutes
// 		defer ticker.Stop()

// 		for range ticker.C {
// 			cleanupOldLeaderCommits()
// 		}
// 	}()
// }

// // cleanupOldLeaderCommits removes leader commit data older than 1 hour
// func cleanupOldLeaderCommits() {
// 	CommitMu.Lock()
// 	defer CommitMu.Unlock()

// 	currentTime := time.Now().Unix()
// 	cutoffTime := currentTime - 3600*12 // 12 hours ago

// 	// Clean up in-memory leader commits
// 	for roundNum, roundMap := range utils.CommittedNodes {
// 		for eoaAddress, commitData := range roundMap {
// 			if commitData.CreatedAt < cutoffTime {
// 				delete(roundMap, eoaAddress)
// 				log.Printf("Cleaned up old leader commit data: round %s, EOA %s", roundNum, eoaAddress.Hex())
// 			}
// 		}
// 		// Remove empty round maps
// 		if len(roundMap) == 0 {
// 			delete(utils.CommittedNodes, roundNum)
// 			log.Printf("Removed empty round map for round %s", roundNum)
// 		}
// 	}

// 	// Clean up file-based leader commits
// 	commits, err := utils.LoadAllLeaderCommitData()
// 	if err != nil {
// 		log.Printf("Failed to load leader commit data for cleanup: %v", err)
// 		return
// 	}

// 	cleaned := false
// 	for key, commitData := range commits {
// 		if commitData.CreatedAt < cutoffTime {
// 			delete(commits, key)
// 			cleaned = true
// 			log.Printf("Cleaned up old leader commit data from file: %s", key)
// 		}
// 	}

// 	if cleaned {
// 		// Save cleaned commits back to file
// 		if err := utils.SaveAllLeaderCommitData(commits); err != nil {
// 			log.Printf("Failed to save cleaned leader commit data: %v", err)
// 		}
// 	}
// }

// // cleanupOldBroadcasts removes broadcast trackers older than 1 hour
// func cleanupOldBroadcasts() {
// 	broadcastMutex.Lock()
// 	defer broadcastMutex.Unlock()

// 	currentTime := time.Now().Unix()
// 	cutoffTime := currentTime - 3600 // 1 hour ago

// 	// Clean up in-memory trackers
// 	for messageID, tracker := range activeBroadcasts {
// 		if tracker.LastSent < cutoffTime {
// 			delete(activeBroadcasts, messageID)
// 			log.Printf("Cleaned up old broadcast tracker: %s", messageID)
// 		}
// 	}

// 	// Clean up file-based trackers
// 	trackers, err := database.GetBroadcastTrackers()
// 	if err != nil {
// 		log.Printf("Failed to load broadcast trackers for cleanup: %v", err)
// 		return
// 	}

// 	cleaned := false
// 	for _, tracker := range trackers {
// 		if tracker.LastSent < cutoffTime {
// 			err := database.DeleteBroadcastTracker(tracker)
// 			if err != nil {
// 				log.Printf("Failed to delete broadcast tracker for %s_%s_%s_%s", tracker.Round, tracker.EOAAddress, tracker.Type, tracker.MessageID)
// 				return
// 			}
// 			cleaned = true
// 		}
// 	}

// 	if cleaned {
// 		// Save cleaned trackers back to file using helper function
// 		if err := database.AddAllBroadcastTrackers(trackers); err != nil {
// 			log.Printf("Failed to save cleaned broadcast trackers: %v", err)
// 		}
// 	}
// }

// generateMessageID creates a unique message ID for broadcasts
func generateMessageID(round, eoaAddress, trailNum, messageType string) string {
	return fmt.Sprintf("%s_%s_%s_%s_%d", round, trailNum, eoaAddress, messageType, time.Now().UnixNano())
}

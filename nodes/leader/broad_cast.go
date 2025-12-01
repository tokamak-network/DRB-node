package leader_node

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/utils"
)

// getLeaderPrivateKey retrieves the leader's private key from environment
func getLeaderPrivateKey() (*ecdsa.PrivateKey, string, error) {
	privateKeyHex := appconfig.Get().LeaderPrivateKey
	if privateKeyHex == "" {
		return nil, "", fmt.Errorf("LEADER_PRIVATE_KEY is not set in environment variables")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode leader private key: %v", err)
	}

	leaderEOA := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	return privateKey, leaderEOA, nil
}

// ReliableBroadCastSSync broadcasts secret values with acknowledgment tracking and waits for completion
func (n *LeaderNode) ReliableBroadCastSSync(ctx context.Context, h host.Host, roundNum string, trialNum string, eoaAddress string, secret [32]byte, activatedOps []common.Address) bool {
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
		Timeout:      3, // 3 seconds timeout
	}

	for _, op := range activatedOps {
		tracker.Acknowledged[op.Hex()] = false
	}

	if err := n.broadcastTrackerRepository.AddBroadcastTracker(ctx, tracker); err != nil {
		log.Printf("Failed to save broadcast tracker: %v", err)
		return false
	}

	n.SetActiveBroadcast(messageID, tracker)

	// 🔄 Perform synchronous broadcast (wait for completion)
	completed := n.performReliableBroadcastSync(ctx, h, tracker, "secret", activatedOps)

	// Clean up from memory
	n.DeleteActiveBroadcast(messageID)

	return completed
}

// ReliableBroadCastCOS broadcasts COS values with acknowledgment tracking
func (n *LeaderNode) ReliableBroadCastCOS(ctx context.Context, roundNum string, trialNum string, eoaAddress common.Address, cos [32]byte, activatedOps []common.Address) {
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

	for _, op := range activatedOps {
		tracker.Acknowledged[op.Hex()] = false
	}

	if err := n.broadcastTrackerRepository.AddBroadcastTracker(ctx, tracker); err != nil {
		log.Printf("Failed to save broadcast tracker: %v", err)
		return
	}

	n.SetActiveBroadcast(messageID, tracker)

	// Start the broadcast process
	go n.performReliableBroadcast(ctx, n.p2pClient.GetHostInstance(), tracker, "cos", activatedOps)
}

// ReliableBroadCastCVS broadcasts CVS values with acknowledgment tracking
func (n *LeaderNode) ReliableBroadCastCVS(ctx context.Context, roundNum string, trialNum string, eoaAddress common.Address, cvs [32]byte, activatedOps []common.Address) {
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

	for _, op := range activatedOps {
		tracker.Acknowledged[op.Hex()] = false
	}

	if err := n.broadcastTrackerRepository.AddBroadcastTracker(ctx, tracker); err != nil {
		log.Printf("Failed to save broadcast tracker: %v", err)
		return
	}

	n.SetActiveBroadcast(messageID, tracker)

	// Start the broadcast process
	go n.performReliableBroadcast(ctx, n.p2pClient.GetHostInstance(), tracker, "cvs", activatedOps)
}

// performReliableBroadcast handles the actual broadcasting with retry logic
func (n *LeaderNode) performReliableBroadcast(ctx context.Context, h host.Host, tracker *utils.BroadcastTracker, broadcastType string, activatedOps []common.Address) {
	if n.GetHalted() {
		n.DeleteActiveBroadcast(tracker.MessageID)
		log.Println("System is halted. Skipping processCVS.")
		return
	}
	nodeInfo := n.p2pClient.GetConnectedPeers(ctx)

	for tracker.Attempts < tracker.MaxAttempts {
		tracker.Attempts++
		tracker.LastSent = time.Now().Unix()

		log.Printf("Broadcasting %s (attempt %d/%d) for round %s, trail %s, EOA %s",
			broadcastType, tracker.Attempts, tracker.MaxAttempts, tracker.Round, tracker.TrialNum, tracker.EOAAddress)

		// Get leader's private key and EOA for signing
		privateKey, leaderEOA, err := getLeaderPrivateKey()
		if err != nil {
			log.Printf("Failed to get leader private key: %v", err)
			return
		}

		// Generate signature for the broadcast
		signature := utils.SignData(leaderEOA, privateKey)

		// Create broadcast message
		message := utils.BroadcastMessage{
			Round:      tracker.Round,
			TrialNum:   tracker.TrialNum,
			EOAAddress: tracker.EOAAddress,
			MessageID:  tracker.MessageID,
			Type:       broadcastType,
			Data:       tracker.Data,
			SignerEOA:  leaderEOA,
			Signature:  signature,
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
		n.broadcastMutex.Lock()
		operatorsToSend := make([]common.Address, 0)
		for _, op := range activatedOps {
			if tracker.Acknowledged[op.Hex()] {
				continue // Skip already acknowledged nodes
			}
			operatorsToSend = append(operatorsToSend, op)
		}
		n.broadcastMutex.Unlock()

		for _, op := range operatorsToSend {
			// Add peer info into peer store
			peerID := nodeInfo[op.Hex()].PeerID
			peerAddrStr := fmt.Sprintf("/ip4/%s/tcp/%s", nodeInfo[op.Hex()].IP, nodeInfo[op.Hex()].Port)
			peerAddr, _ := multiaddr.NewMultiaddr(peerAddrStr)
			h.Peerstore().AddAddr(peerID, peerAddr, peerstore.PermanentAddrTTL)

			stream, err := h.NewStream(ctx, nodeInfo[op.Hex()].PeerID, streamProtocol)
			if err != nil {
				log.Printf("Failed to create stream to peer %s: %v", nodeInfo[op.Hex()].PeerID, err)
				continue
			}
			defer stream.Close()

			if err := json.NewEncoder(stream).Encode(message); err != nil {
				log.Printf("Failed to send %s to regular node %s: %v", broadcastType, op.Hex(), err)
			} else {
				log.Printf("%s sent to regular node %s for round %s with trail %s (attempt %d)",
					broadcastType, op.Hex(), tracker.Round, tracker.TrialNum, tracker.Attempts)
			}
		}

		// Update tracker
		if err := n.broadcastTrackerRepository.UpdateBroadcastTracker(ctx, tracker); err != nil {
			log.Printf("Failed to update broadcast tracker: %v", err)
		}

		// Wait for acknowledgments or timeout
		time.Sleep(time.Duration(tracker.Timeout) * time.Second)

		// Check if all nodes have acknowledged
		n.broadcastMutex.Lock()
		allAcknowledged := true
		for _, acknowledged := range tracker.Acknowledged {
			if !acknowledged {
				allAcknowledged = false
				break
			}
		}

		if allAcknowledged {
			n.broadcastMutex.Unlock()
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
		n.broadcastMutex.Unlock()
	}

	// Clean up from memory
	n.DeleteActiveBroadcast(tracker.MessageID)
}

// performReliableBroadcastSync handles broadcasting synchronously and returns completion status
func (n *LeaderNode) performReliableBroadcastSync(ctx context.Context, h host.Host, tracker *utils.BroadcastTracker, broadcastType string, activatedOps []common.Address) bool {
	if n.GetHalted() {
		log.Println("System is halted. Skipping broadcast.")
		return false
	}

	// Get leader private key and EOA for signing
	privateKey, leaderEOA, err := getLeaderPrivateKey()
	if err != nil {
		log.Printf("Failed to get leader private key: %v", err)
		return false
	}

	// Get connected peer information
	nodeInfo := n.p2pClient.GetConnectedPeers(ctx)

	// Create message
	message := utils.BroadcastMessage{
		MessageID:  tracker.MessageID,
		Round:      tracker.Round,
		TrialNum:   tracker.TrialNum,
		EOAAddress: tracker.EOAAddress,
		Data:       tracker.Data,
		Type:       broadcastType,
		SignerEOA:  leaderEOA,
	}

	// Sign the message
	signature := utils.SignData(leaderEOA, privateKey)
	message.Signature = signature

	for tracker.Attempts < tracker.MaxAttempts {
		tracker.Attempts++
		tracker.LastSent = time.Now().Unix()

		log.Printf("🔄 Starting %s broadcast attempt %d/%d for round %s, EOA %s",
			broadcastType, tracker.Attempts, tracker.MaxAttempts, tracker.Round, tracker.EOAAddress)

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
			return false
		}

		// Send to all activated operators
		n.broadcastMutex.Lock()
		operatorsToSend := make([]common.Address, 0)
		for _, op := range activatedOps {
			if tracker.Acknowledged[op.Hex()] {
				continue // Skip already acknowledged nodes
			}
			operatorsToSend = append(operatorsToSend, op)
		}
		n.broadcastMutex.Unlock()

		for _, op := range operatorsToSend {
			// Add peer info into peer store
			peerID := nodeInfo[op.Hex()].PeerID
			peerAddrStr := fmt.Sprintf("/ip4/%s/tcp/%s", nodeInfo[op.Hex()].IP, nodeInfo[op.Hex()].Port)
			peerAddr, _ := multiaddr.NewMultiaddr(peerAddrStr)
			h.Peerstore().AddAddr(peerID, peerAddr, peerstore.PermanentAddrTTL)

			stream, err := h.NewStream(ctx, nodeInfo[op.Hex()].PeerID, streamProtocol)
			if err != nil {
				log.Printf("Failed to create stream to peer %s: %v", nodeInfo[op.Hex()].PeerID, err)
				continue
			}
			defer stream.Close()

			if err := json.NewEncoder(stream).Encode(message); err != nil {
				log.Printf("Failed to send %s to regular node %s: %v", broadcastType, op.Hex(), err)
			} else {
				log.Printf("%s sent to regular node %s for round %s with trail %s (attempt %d)",
					broadcastType, op.Hex(), tracker.Round, tracker.TrialNum, tracker.Attempts)
			}
		}

		// Update tracker
		if err := n.broadcastTrackerRepository.UpdateBroadcastTracker(ctx, tracker); err != nil {
			log.Printf("Failed to update broadcast tracker: %v", err)
		}

		// Wait for acknowledgments or timeout
		time.Sleep(time.Duration(tracker.Timeout) * time.Second)

		// Check if all nodes have acknowledged
		n.broadcastMutex.Lock()
		allAcknowledged := true
		for _, acknowledged := range tracker.Acknowledged {
			if !acknowledged {
				allAcknowledged = false
				break
			}
		}

		if allAcknowledged {
			log.Printf("✅ All nodes acknowledged %s broadcast for round %s, EOA %s",
				broadcastType, tracker.Round, tracker.EOAAddress)
			n.broadcastMutex.Unlock()
			return true
		}

		// If this was the last attempt, log unacknowledged nodes
		if tracker.Attempts >= tracker.MaxAttempts {
			log.Printf("⚠️ Max attempts reached for %s broadcast. Unacknowledged nodes:", broadcastType)
			for eoa, acknowledged := range tracker.Acknowledged {
				if !acknowledged {
					log.Printf("  - %s", eoa)
				}
			}
			n.broadcastMutex.Unlock()
			return false
		}
		n.broadcastMutex.Unlock()
	}

	return false
}

// HandleAcknowledgment processes acknowledgments from regular nodes
func (n *LeaderNode) HandleAcknowledgment(ctx context.Context, ack utils.AcknowledgmentMessage) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping HandleAcknowledgment.")
		return
	}
	n.broadcastMutex.Lock()
	defer n.broadcastMutex.Unlock()

	tracker, exists := n.GetActiveBroadcast(ack.MessageID)
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
	if err := n.broadcastTrackerRepository.UpdateBroadcastTracker(ctx, tracker); err != nil {
		log.Printf("Failed to update broadcast tracker: %v", err)
	}
}

// generateMessageID creates a unique message ID for broadcasts
func generateMessageID(round, eoaAddress, trailNum, messageType string) string {
	return fmt.Sprintf("%s_%s_%s_%s_%d", round, trailNum, eoaAddress, messageType, time.Now().UnixNano())
}

package regularNode_helper

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"sync/atomic"

	"github.com/go-pg/pg/v10"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/utils"
)

var CosRecevied sync.Map // outer: string, inner: *sync.Map (string->bool)

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
	}
}

// HandleCvs processes incoming CVS values and sends acknowledgment
func HandleCvs(h host.Host, s network.Stream) {
	defer s.Close()

	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping HandleCvs.")
		return
	}

	var message utils.BroadcastMessage
	if err := json.NewDecoder(s).Decode(&message); err != nil {
		log.Printf("Failed to decode CVS broadcast message: %v", err)
		return
	}

	peerCommitData, err := database.GetPeerCommitData(message.Round, message.TrialNum, message.EOAAddress)
	if err != nil {
		if err == pg.ErrNoRows {
			// No existing record, create new and insert
			peerCommitData = &database.PeerCommitDataScheme{
				Round:      message.Round,
				TrialNum:   message.TrialNum,
				EOAAddress: message.EOAAddress,
				Cvs:        message.Data[:],
			}
			if err = database.AddPeerCommitData(peerCommitData); err != nil {
				log.Printf("Failed to add new peer commit data: %v", err)
				return
			}
		} else {
			log.Printf("Failed to get peer commit data: %v", err)
			return
		}
	} else {
		// Existing record found, update fields
		peerCommitData.Cvs = message.Data[:]

		if err = database.UpdatePeerCommitData(peerCommitData); err != nil {
			log.Printf("Failed to update peer commit data: %v", err)
			return
		}
	}

	// Send acknowledgment
	ack := utils.AcknowledgmentMessage{
		Round:      message.Round,
		TrialNum:   message.TrialNum,
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

	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping HandleCos.")
		return
	}

	var message utils.BroadcastMessage
	if err := json.NewDecoder(s).Decode(&message); err != nil {
		log.Printf("Failed to decode COS broadcast message: %v", err)
		return
	}

	// Process the COS data
	uniqueKey := utils.GetUniqueKey(message.Round, message.TrialNum)
	SetCosReceived(uniqueKey, message.EOAAddress, true)

	peerCommitData, err := database.GetPeerCommitData(message.Round, message.TrialNum, message.EOAAddress)
	if err != nil {
		if err == pg.ErrNoRows {
			// No existing record, create new and insert
			peerCommitData = &database.PeerCommitDataScheme{
				Round:      message.Round,
				TrialNum:   message.TrialNum,
				EOAAddress: message.EOAAddress,
				Cos:        message.Data[:],
			}
			if err = database.AddPeerCommitData(peerCommitData); err != nil {
				log.Printf("Failed to add new peer commit data: %v", err)
				return
			}
		} else {
			log.Printf("Failed to get peer commit data: %v", err)
			return
		}
	} else {
		// Existing record found, update fields
		peerCommitData.Cos = message.Data[:]

		if err = database.UpdatePeerCommitData(peerCommitData); err != nil {
			log.Printf("Failed to update peer commit data: %v", err)
			return
		}
	}

	// Send acknowledgment
	ack := utils.AcknowledgmentMessage{
		Round:      message.Round,
		TrialNum:   message.TrialNum,
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

	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping HandleSecret.")
		return
	}

	var message utils.BroadcastMessage
	if err := json.NewDecoder(s).Decode(&message); err != nil {
		log.Printf("Failed to decode secret broadcast message: %v", err)
		return
	}

	peerCommitData, err := database.GetPeerCommitData(message.Round, message.TrialNum, message.EOAAddress)
	if err != nil {
		if err == pg.ErrNoRows {
			// No existing record, create new and insert
			peerCommitData = &database.PeerCommitDataScheme{
				Round:       message.Round,
				TrialNum:    message.TrialNum,
				EOAAddress:  message.EOAAddress,
				SecretValue: message.Data[:],
			}
			if err = database.AddPeerCommitData(peerCommitData); err != nil {
				log.Printf("Failed to add new peer commit data: %v", err)
				return
			}
		} else {
			log.Printf("Failed to get peer commit data: %v", err)
			return
		}
	} else {
		// Existing record found, update fields
		peerCommitData.SecretValue = message.Data[:]

		if err = database.UpdatePeerCommitData(peerCommitData); err != nil {
			log.Printf("Failed to update peer commit data: %v", err)
			return
		}
	}

	log.Printf("Successfully saved secret value for round %s and EOA %s", message.Round, message.EOAAddress)

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

func SetCosReceived(outer, inner string, value bool) {
	actual, _ := CosRecevied.LoadOrStore(outer, &sync.Map{})
	innerMap := actual.(*sync.Map)
	innerMap.Store(inner, value)
}

func GetCosReceived(outer, inner string) (bool, bool) {
	actual, ok := CosRecevied.Load(outer)
	if !ok {
		return false, false
	}
	innerMap := actual.(*sync.Map)
	v, ok := innerMap.Load(inner)
	if !ok {
		return false, false
	}
	return v.(bool), true
}

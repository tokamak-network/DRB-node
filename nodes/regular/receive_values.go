package regular_node

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/go-pg/pg/v10"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/utils"
)

// generates a signature for the acknowledgment
func (n *RegularNode) generateAcknowledgmentSignature(ack utils.AcknowledgmentMessage) []byte {
	if n.GetRegularNodePrivateKey() == nil {
		log.Printf("Regular node private key not set, cannot sign acknowledgment")
		return nil
	}
	signature, err := utils.SignAcknowledgmentContent(ack, n.GetRegularNodePrivateKey())
	if err != nil {
		log.Printf("Failed to sign acknowledgment content: %v", err)
		return nil
	}
	return signature
}

// sendAcknowledgment sends an acknowledgment back to the leader
func (n *RegularNode) sendAcknowledgment(ctx context.Context, h host.Host, leaderPeerID peer.ID, ack utils.AcknowledgmentMessage) {
	stream, err := h.NewStream(ctx, leaderPeerID, protocol.ID("/acknowledgment"))
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
func (n *RegularNode) HandleCvs(ctx context.Context, h host.Host, s network.Stream) {
	defer s.Close()

	if n.GetHalted() {
		log.Println("System is halted. Skipping HandleCvs.")
		return
	}

	decodeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var message utils.BroadcastMessage
	if err := utils.DecodeJSONWithContext(decodeCtx, s, &message); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("CVS broadcast message decode timeout after 10s from peer: %s", s.Conn().RemotePeer())
		} else {
			log.Printf("Failed to decode CVS broadcast message: %v", err)
		}
		return
	}

	leaderEOA := appconfig.Get().LeaderEOA
	if leaderEOA == "" {
		log.Println("LEADER_EOA is not set in the environment variables")
		return
	}

	if !utils.VerifyBroadcastMessageContentSignature(message, leaderEOA) {
		log.Printf("Signature verification failed for CVS broadcast from signer: %s (message ID: %s). Message content may have been tampered.", message.SignerEOA, message.MessageID)
		return
	}

	log.Printf("Signature verified for CVS broadcast from %s", message.SignerEOA)
	log.Printf("Received CVS broadcast for round %s with trail %s from EOA %s (message ID: %s)",
		message.Round, message.TrialNum, message.EOAAddress, message.MessageID)

	peerCommitData, err := n.peerCommitDataRepository.GetPeerCommitData(ctx, message.Round, message.TrialNum, message.EOAAddress)
	if err != nil {
		if err == pg.ErrNoRows {
			// No existing record, create new and insert
			peerCommitData = &database.PeerCommitDataScheme{
				Round:      message.Round,
				TrialNum:   message.TrialNum,
				EOAAddress: message.EOAAddress,
				Cvs:        message.Data[:],
			}
			if err = n.peerCommitDataRepository.AddPeerCommitData(ctx, peerCommitData); err != nil {
				log.Printf("Failed to add new peer commit data: %v", err)
				return
			}
		} else {
			log.Printf("Database connection error while getting peer commit data for round %s, trial %s, EOA %s: %v", message.Round, message.TrialNum, message.EOAAddress, err)
			return
		}
	} else {
		// Existing record found, update fields
		peerCommitData.Cvs = message.Data[:]

		if err = n.peerCommitDataRepository.UpdatePeerCommitData(ctx, peerCommitData); err != nil {
			log.Printf("Failed to update peer commit data: %v", err)
			return
		}
	}

	log.Printf("Successfully saved CVS data for round %s and EOA %s", message.Round, message.EOAAddress)

	// Send acknowledgment
	eoaAddress := n.GetRegularNodeEOA()
	ack := utils.AcknowledgmentMessage{
		Round:      message.Round,
		TrialNum:   message.TrialNum,
		EOAAddress: eoaAddress,
		MessageID:  message.MessageID,
		Type:       message.Type,
		Status:     "received",
	}
	signature := n.generateAcknowledgmentSignature(ack)
	ack.Signature = signature

	// Get leader peer ID from environment or connection
	leaderPeerIDStr := appconfig.Get().LeaderPeerID
	if leaderPeerIDStr == "" {
		log.Printf("LEADER_PEER_ID not set, cannot send acknowledgment")
		return
	}

	leaderPeerID, err := peer.Decode(leaderPeerIDStr)
	if err != nil {
		log.Printf("Failed to decode leader peer ID: %v", err)
		return
	}

	n.sendAcknowledgment(ctx, h, leaderPeerID, ack)
}

// HandleCos processes incoming COS values and sends acknowledgment
func (n *RegularNode) HandleCos(ctx context.Context, h host.Host, s network.Stream) {
	defer s.Close()

	if n.GetHalted() {
		log.Println("System is halted. Skipping HandleCos.")
		return
	}

	decodeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var message utils.BroadcastMessage
	if err := utils.DecodeJSONWithContext(decodeCtx, s, &message); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("COS broadcast message decode timeout after 10s from peer: %s", s.Conn().RemotePeer())
		} else {
			log.Printf("Failed to decode COS broadcast message: %v", err)
		}
		return
	}

	leaderEOA := appconfig.Get().LeaderEOA
	if leaderEOA == "" {
		log.Println("LEADER_EOA is not set in the environment variables")
		return
	}

	// Verify leader signature for broadcast message
	if !utils.VerifyBroadcastMessageContentSignature(message, leaderEOA) {
		log.Printf("Signature verification failed for COS broadcast from signer: %s (message ID: %s). Message content may have been tampered.", message.SignerEOA, message.MessageID)
		return
	}

	log.Printf("Signature verified for COS broadcast from %s", message.SignerEOA)
	log.Printf("Received COS broadcast for round %s with trail %s from EOA %s (message ID: %s)",
		message.Round, message.TrialNum, message.EOAAddress, message.MessageID)

	// Process the COS data
	uniqueKey := utils.GetUniqueKey(message.Round, message.TrialNum)
	n.SetCosReceived(uniqueKey, message.EOAAddress, true)

	peerCommitData, err := n.peerCommitDataRepository.GetPeerCommitData(ctx, message.Round, message.TrialNum, message.EOAAddress)
	if err != nil {
		if err == pg.ErrNoRows {
			// No existing record, create new and insert
			peerCommitData = &database.PeerCommitDataScheme{
				Round:      message.Round,
				TrialNum:   message.TrialNum,
				EOAAddress: message.EOAAddress,
				Cos:        message.Data[:],
			}
			if err = n.peerCommitDataRepository.AddPeerCommitData(ctx, peerCommitData); err != nil {
				log.Printf("Failed to add new peer commit data: %v", err)
				return
			}
		} else {
			log.Printf("Database connection error while getting peer commit data for round %s, trial %s, EOA %s: %v", message.Round, message.TrialNum, message.EOAAddress, err)
			return
		}
	} else {
		// Existing record found, update fields
		peerCommitData.Cos = message.Data[:]

		if err = n.peerCommitDataRepository.UpdatePeerCommitData(ctx, peerCommitData); err != nil {
			log.Printf("Failed to update peer commit data: %v", err)
			return
		}
	}

	log.Printf("Successfully saved COS data for round %s with trail %s and EOA %s", message.Round, message.TrialNum, message.EOAAddress)

	// Send acknowledgment
	eoaAddress := n.GetRegularNodeEOA()
	ack := utils.AcknowledgmentMessage{
		Round:      message.Round,
		TrialNum:   message.TrialNum,
		EOAAddress: eoaAddress,
		MessageID:  message.MessageID,
		Type:       message.Type,
		Status:     "received",
	}
	signature := n.generateAcknowledgmentSignature(ack)
	ack.Signature = signature

	// Get leader peer ID from environment or connection
	leaderPeerIDStr := appconfig.Get().LeaderPeerID
	if leaderPeerIDStr == "" {
		log.Printf("LEADER_PEER_ID not set, cannot send acknowledgment")
		return
	}

	leaderPeerID, err := peer.Decode(leaderPeerIDStr)
	if err != nil {
		log.Printf("Failed to decode leader peer ID: %v", err)
		return
	}

	n.sendAcknowledgment(ctx, h, leaderPeerID, ack)
}

// HandleSecret processes incoming secret values and sends acknowledgment
func (n *RegularNode) HandleSecret(ctx context.Context, h host.Host, s network.Stream) {
	defer s.Close()

	if n.GetHalted() {
		log.Println("System is halted. Skipping HandleSecret.")
		return
	}

	decodeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var message utils.BroadcastMessage
	if err := utils.DecodeJSONWithContext(decodeCtx, s, &message); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("Secret broadcast message decode timeout after 10s from peer: %s", s.Conn().RemotePeer())
		} else {
			log.Printf("Failed to decode secret broadcast message: %v", err)
		}
		return
	}

	leaderEOA := appconfig.Get().LeaderEOA
	if leaderEOA == "" {
		log.Println("LEADER_EOA is not set in the environment variables")
		return
	}

	// Verify leader signature for broadcast message
	if !utils.VerifyBroadcastMessageContentSignature(message, leaderEOA) {
		log.Printf("Signature verification failed for secret broadcast from signer: %s (message ID: %s). Message content may have been tampered.", message.SignerEOA, message.MessageID)
		return
	}

	log.Printf("Signature verified for secret broadcast from %s", message.SignerEOA)
	log.Printf("Received secret broadcast for round %s with trail %s from EOA %s (message ID: %s)",
		message.Round, message.TrialNum, message.EOAAddress, message.MessageID)

	peerCommitData, err := n.peerCommitDataRepository.GetPeerCommitData(ctx, message.Round, message.TrialNum, message.EOAAddress)
	if err != nil {
		if err == pg.ErrNoRows {
			// No existing record, create new and insert
			peerCommitData = &database.PeerCommitDataScheme{
				Round:       message.Round,
				TrialNum:    message.TrialNum,
				EOAAddress:  message.EOAAddress,
				SecretValue: message.Data[:],
			}
			if err = n.peerCommitDataRepository.AddPeerCommitData(ctx, peerCommitData); err != nil {
				log.Printf("Failed to add new peer commit data: %v", err)
				return
			}
		} else {
			log.Printf("Database connection error while getting peer commit data for round %s, trial %s, EOA %s: %v", message.Round, message.TrialNum, message.EOAAddress, err)
			return
		}
	} else {
		// Existing record found, update fields
		peerCommitData.SecretValue = message.Data[:]

		if err = n.peerCommitDataRepository.UpdatePeerCommitData(ctx, peerCommitData); err != nil {
			log.Printf("Failed to update peer commit data: %v", err)
			return
		}
	}

	log.Printf("Successfully saved secret value for round %s and EOA %s", message.Round, message.EOAAddress)

	// Send acknowledgment
	eoaAddress := n.GetRegularNodeEOA()
	ack := utils.AcknowledgmentMessage{
		Round:      message.Round,
		TrialNum:   message.TrialNum,
		EOAAddress: eoaAddress,
		MessageID:  message.MessageID,
		Type:       message.Type,
		Status:     "received",
	}
	signature := n.generateAcknowledgmentSignature(ack)
	ack.Signature = signature

	// Get leader peer ID from environment or connection
	leaderPeerIDStr := appconfig.Get().LeaderPeerID
	if leaderPeerIDStr == "" {
		log.Printf("LEADER_PEER_ID not set, cannot send acknowledgment")
		return
	}

	leaderPeerID, err := peer.Decode(leaderPeerIDStr)
	if err != nil {
		log.Printf("Failed to decode leader peer ID: %v", err)
		return
	}

	n.sendAcknowledgment(ctx, h, leaderPeerID, ack)
}

func (n *RegularNode) SetCosReceived(outer, inner string, value bool) {
	actual, _ := n.cosRecevied.LoadOrStore(outer, &sync.Map{})
	innerMap := actual.(*sync.Map)
	innerMap.Store(inner, value)
}

func (n *RegularNode) GetCosReceived(outer, inner string) (bool, bool) {
	actual, ok := n.cosRecevied.Load(outer)
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

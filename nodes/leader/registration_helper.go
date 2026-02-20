package leader_node

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

// RegisterNode handles both saving node information and activating the node on-chain.
func (n *LeaderNode) RegisterNode(ctx context.Context, s network.Stream, abiFilePath string) error {
	decodeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var req utils.RegistrationRequest
	if err := utils.DecodeJSONWithContext(decodeCtx, s, &req); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("registration request decode timeout after 10s from peer: %s", s.Conn().RemotePeer())
		}
		return fmt.Errorf("failed to decode registration request: %v", err)
	}
	remoteAddr := s.Conn().RemoteMultiaddr().String()
	return n.registerNodeInternal(ctx, req, remoteAddr)
}

// registerNodeInternal contains the core registration logic to ease unit testing.
func (n *LeaderNode) registerNodeInternal(ctx context.Context, req utils.RegistrationRequest, remoteAddr string) error {
	if !utils.VerifyRegistrationRequestContentSignature(req, req.EOAAddress) {
		return fmt.Errorf("signature verification failed for registration request from EOA: %s. EOAAddress, PeerID, IP, or Port may have been tampered", req.EOAAddress)
	}

	log.Printf("Verified registration for PeerID: %s", req.PeerID)
	if n.fallbackEthClient != nil {
		if err := eth.Service.UpdateActivatedOperators(ctx, n.fallbackEthClient); err != nil {
			log.Printf("Failed to update activated operators during registration: %v", err)
		}
	}
	operators := eth.Service.GetActivatedOperatorsCached()

	// Check if the EOA is in the activated operators list
	isActivated := false
	for _, operator := range operators {
		if operator.Hex() == req.EOAAddress {
			isActivated = true
			break
		}
	}

	// Only register if the EOA is activated
	if !isActivated {
		return fmt.Errorf("EOA %s is not activated, registration denied", req.EOAAddress)
	}

	log.Printf("EOA %s is activated, proceeding with registration", req.EOAAddress)

	// Use IP and Port from registration request (public IP if prod, local IP otherwise)
	if req.IP == "" || req.Port == "" {
		return fmt.Errorf("IP or Port not provided in registration request")
	}

	// Update or add the node information
	nodeInfo := utils.NodeInfo{
		IP:         req.IP,
		Port:       req.Port,
		PeerID:     req.PeerID,
		EOAAddress: req.EOAAddress,
	}

	// Save updated nodes
	log.Printf("Attempting to register node: EOA=%s, IP=%s, Port=%s, PeerID=%s",
		req.EOAAddress, req.IP, req.Port, req.PeerID)

	err := n.nodeInfoRepository.AddAndUpdateNodeInfo(ctx, &nodeInfo)
	if err != nil {
		log.Printf("Registration failed for EOA %s: %v", req.EOAAddress, err)
		log.Printf("Registration details: IP=%s, Port=%s, PeerID=%s", req.IP, req.Port, req.PeerID)
		return fmt.Errorf("failed to save registered nodes: %v", err)
	}

	log.Printf("Successfully registered or updated EOA %s with NodeInfo: IP=%s, Port=%s, PeerID=%s.", req.EOAAddress, req.IP, req.Port, req.PeerID)
	return nil
}

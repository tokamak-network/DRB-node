package leader_node

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

// RegisterNode handles both saving node information and activating the node on-chain.
func (n *LeaderNode) RegisterNode(ctx context.Context, s network.Stream, abiFilePath string) error {
	var req utils.RegistrationRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		return fmt.Errorf("failed to decode registration request: %v", err)
	}
	remoteAddr := s.Conn().RemoteMultiaddr().String()
	return n.registerNodeInternal(ctx, req, remoteAddr)
}

// registerNodeInternal contains the core registration logic to ease unit testing.
func (n *LeaderNode) registerNodeInternal(ctx context.Context, req utils.RegistrationRequest, remoteAddr string) error {
	verifyReq := utils.Verification{
		EOAAddress: req.EOAAddress,
		Signature:  req.Signature,
	}
	if !utils.VerifySignature(verifyReq) {
		return fmt.Errorf("failed to verify signature for PeerID: %s", req.PeerID)
	}

	log.Printf("Verified registration for PeerID: %s", req.PeerID)
	if n.fallbackEthClient != nil {
		eth.Service.UpdateActivatedOperators(ctx, n.fallbackEthClient)
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

	parts := strings.Split(remoteAddr, "/")
	if len(parts) < 5 {
		return fmt.Errorf("invalid remote address format: %s", remoteAddr)
	}

	ip := parts[2]   // Extract IP
	port := parts[4] // Extract port

	// Update or add the node information
	nodeInfo := utils.NodeInfo{
		IP:         ip,
		Port:       port,
		PeerID:     req.PeerID,
		EOAAddress: req.EOAAddress,
	}

	// Save updated nodes
	err := n.nodeInfoRepository.AddAndUpdateNodeInfo(ctx, &nodeInfo)
	if err != nil {
		return fmt.Errorf("failed to save registered nodes: %v", err)
	}

	log.Printf("Successfully registered or updated EOA %s with NodeInfo: IP=%s, Port=%s, PeerID=%s.", req.EOAAddress, ip, port, req.PeerID)
	return nil
}

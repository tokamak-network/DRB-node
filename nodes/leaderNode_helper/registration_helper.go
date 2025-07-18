package leaderNode_helper

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/utils"
)

// RegisterNode handles both saving node information and activating the node on-chain.
func RegisterNode(s network.Stream, abiFilePath string) error {
	var req utils.RegistrationRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		return fmt.Errorf("failed to decode registration request: %v", err)
	}

	if !utils.VerifySignature(req) {
		return fmt.Errorf("failed to verify signature for PeerID: %s", req.PeerID)
	}

	log.Printf("Verified registration for PeerID: %s", req.PeerID)

	// Get the remote IP and port
	remoteAddr := s.Conn().RemoteMultiaddr().String()
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
	err := database.AddNodeInfo(&nodeInfo)
	if err != nil {
		return fmt.Errorf("failed to save registered nodes: %v", err)
	}

	log.Printf("Successfully registered or updated EOA %s with NodeInfo: IP=%s, Port=%s, PeerID=%s.", req.EOAAddress, ip, port, req.PeerID)

	return nil
}

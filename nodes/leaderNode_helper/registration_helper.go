package leaderNode_helper

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/utils"
)

// NodeInfo stores the information for a registered node
type NodeInfo struct {
	IP     string `json:"ip"`
	Port   string `json:"port"`
	PeerID string `json:"peer_id"`
}

// LoadRegisteredNodes loads the registered nodes from a JSON file.
func LoadRegisteredNodes(filePath string) (map[string]NodeInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return an empty map if the file doesn't exist
			return make(map[string]NodeInfo), nil
		}
		return nil, fmt.Errorf("failed to open registered nodes file: %v", err)
	}
	defer file.Close()

	var data map[string]NodeInfo
	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode registered nodes file: %v", err)
	}

	return data, nil
}

// SaveRegisteredNodes saves the registered nodes to a JSON file.
func SaveRegisteredNodes(filePath string, data map[string]NodeInfo) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create registered nodes file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(data)
	if err != nil {
		return fmt.Errorf("failed to write registered nodes to file: %v", err)
	}

	return nil
}

// RegisterNode handles both saving node information and activating the node on-chain.
func RegisterNode(s network.Stream, filePath, abiFilePath string) error {
	var req utils.RegistrationRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		return fmt.Errorf("failed to decode registration request: %v", err)
	}

	if !utils.VerifySignature(req) {
		return fmt.Errorf("failed to verify signature for PeerID: %s", req.PeerID)
	}
	log.Printf("Registring node with EOA %v", req.EOAAddress)

	// Get the remote IP and port
	remoteAddr := s.Conn().RemoteMultiaddr().String()
	parts := strings.Split(remoteAddr, "/")
	if len(parts) < 5 {
		return fmt.Errorf("invalid remote address format: %s", remoteAddr)
	}

	ip := parts[2]   // Extract IP
	port := parts[4] // Extract port

	// Load existing nodes
	nodes, err := LoadRegisteredNodes(filePath)
	if err != nil {
		return fmt.Errorf("failed to load registered nodes: %v", err)
	}

	// Update or add the node information
	nodes[req.EOAAddress] = NodeInfo{
		IP:     ip,
		Port:   port,
		PeerID: req.PeerID,
	}

	// Save updated nodes
	err = SaveRegisteredNodes(filePath, nodes)
	if err != nil {
		return fmt.Errorf("failed to save registered nodes: %v", err)
	}

	return nil
}

package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const leaderNodeFile = "static-key/leadernode.bin"

func GeneratePeerID() (string, error) {
	var privKey crypto.PrivKey
	var peerID peer.ID

	// Check if the key file exists
	if _, err := os.Stat(leaderNodeFile); os.IsNotExist(err) {
		// File does not exist, generate a new private key
		log.Printf("Generating new private key...")
		privKey, _, err = crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			return "", fmt.Errorf("failed to generate private key: %v", err)
		}

		buff, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			return "", fmt.Errorf("failed to marshal private key: %v", err)
		}

		dir := filepath.Dir(leaderNodeFile)

		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %v", err)
		}

		err = os.WriteFile(leaderNodeFile, buff, 0644)
		if err != nil {
			return "", fmt.Errorf("failed to write private key to file: %v", err)
		}

		// Generate the PeerID from the private key
		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			return "", fmt.Errorf("failed to generate PeerID from private key: %v", err)
		}

		log.Printf("New PeerID Generated: %s", peerID)
		return peerID.String(), nil
	} else if err != nil {
		// Handle other potential errors when checking for the file.
		return "", fmt.Errorf("error checking for file '%s': %v", leaderNodeFile, err)
	} else {
		// File exists, load existing peer ID
		log.Printf("File '%s' already exists. Loading existing peer ID...", leaderNodeFile)
		buff, err := os.ReadFile(leaderNodeFile)
		if err != nil {
			return "", fmt.Errorf("failed to read existing key file '%s': %v", leaderNodeFile, err)
		}

		privKey, err = crypto.UnmarshalPrivateKey(buff)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal private key from '%s': %v", leaderNodeFile, err)
		}

		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			return "", fmt.Errorf("failed to generate PeerID from private key in '%s': %v", leaderNodeFile, err)
		}

		log.Printf("Loaded existing PeerID: %s", peerID)
		return peerID.String(), nil
	}
}

func GenerateRegularPeerIDs(count int) ([]string, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be greater than 0")
	}

	peerIDs := make([]string, 0, count)

	for i := 1; i <= count; i++ {
		peerID, err := GenerateRegularPeerIDForNode(i)
		if err != nil {
			return nil, fmt.Errorf("failed to generate peer ID for regular node %d: %w", i, err)
		}
		peerIDs = append(peerIDs, peerID)
	}

	return peerIDs, nil
}

func GenerateRegularPeerIDForNode(nodeNumber int) (string, error) {
	if nodeNumber < 0 {
		return "", fmt.Errorf("nodeNumber must be greater than or equal to 0")
	}

	var fileName string
	if nodeNumber == 0 {
		fileName = "static-key/regularnode.bin"
	} else {
		fileName = fmt.Sprintf("static-key/regularnode%d.bin", nodeNumber)
	}
	dir := filepath.Dir(leaderNodeFile)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	var privKey crypto.PrivKey
	var peerID peer.ID

	// Check if the key file exists
	if _, err := os.Stat(fileName); err == nil {
		log.Printf("File '%s' already exists. Loading existing peer ID...", fileName)
		buff, err := os.ReadFile(fileName)
		if err != nil {
			return "", fmt.Errorf("failed to read existing key file '%s': %v", fileName, err)
		}

		privKey, err = crypto.UnmarshalPrivateKey(buff)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal private key from '%s': %v", fileName, err)
		}

		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			return "", fmt.Errorf("failed to generate PeerID from private key in '%s': %v", fileName, err)
		}

		if nodeNumber == 0 {
			log.Printf("Loaded existing PeerID for Regular Node: %s", peerID)
		} else {
			log.Printf("Loaded existing PeerID for Regular Node %d: %s", nodeNumber, peerID)
		}
		return peerID.String(), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("error checking for file '%s': %v", fileName, err)
	}

	if nodeNumber == 0 {
		log.Printf("Generating new private key for Regular Node...")
	} else {
		log.Printf("Generating new private key for Regular Node %d...", nodeNumber)
	}
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
	if err != nil {
		if nodeNumber == 0 {
			return "", fmt.Errorf("failed to generate private key for Regular Node: %v", err)
		}
		return "", fmt.Errorf("failed to generate private key for Regular Node %d: %v", nodeNumber, err)
	}

	buff, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		if nodeNumber == 0 {
			return "", fmt.Errorf("failed to marshal private key for Regular Node: %v", err)
		}
		return "", fmt.Errorf("failed to marshal private key for Regular Node %d: %v", nodeNumber, err)
	}

	err = os.WriteFile(fileName, buff, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write private key to file '%s': %v", fileName, err)
	}

	peerID, err = peer.IDFromPrivateKey(privKey)
	if err != nil {
		if nodeNumber == 0 {
			return "", fmt.Errorf("failed to generate PeerID from private key for Regular Node: %v", err)
		}
		return "", fmt.Errorf("failed to generate PeerID from private key for Regular Node %d: %v", nodeNumber, err)
	}

	if nodeNumber == 0 {
		log.Printf("New PeerID Generated for Regular Node: %s", peerID)
	} else {
		log.Printf("New PeerID Generated for Regular Node %d: %s", nodeNumber, peerID)
	}
	return peerID.String(), nil
}

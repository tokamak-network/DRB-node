package utils

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerIDStorage structure to store PeerID's private key as bytes
type PeerIDStorage struct {
	PrivateKeyBytes []byte `json:"private_key_bytes"`
}

// GetPeerIDFileName returns the appropriate file name based on the node type
func GetPeerIDFileName() string {
	// Read NODE_TYPE from the environment variable
	nodeType := os.Getenv("NODE_TYPE")

	// Default to "regular" if NODE_TYPE is not set
	if nodeType == "" {
		nodeType = "regular"
	}

	// Return the corresponding file name based on the node type
	if nodeType == "leader" {
		return "leader_peer_id.json"
	}
	return "regular_peer_id.json"
}

// SavePeerID saves the libp2p PeerID's private key to a file as bytes
func SavePeerID(privKey crypto.PrivKey) error {
	// Convert the private key to bytes
	privKeyBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		log.Printf("Failed to marshal private key: %v", err)
		return err
	}

	// Create the storage object
	peerIDStorage := PeerIDStorage{PrivateKeyBytes: privKeyBytes}

	// Marshal and save to the correct file based on the node type
	fileName := GetPeerIDFileName()
	data, err := json.MarshalIndent(peerIDStorage, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal private key bytes: %v", err)
		return err
	}

	err = ioutil.WriteFile(fileName, data, 0644)
	if err != nil {
		log.Printf("Failed to write private key bytes to %s: %v", fileName, err)
		return err
	}

	log.Printf("Private key saved to %s", fileName)
	return nil
}

// LoadPeerID loads the libp2p PeerID's private key from a file
func LoadPeerID() (crypto.PrivKey, peer.ID, error) {
	// Get the correct file name based on the node type
	fileName := GetPeerIDFileName()

	// Attempt to read the file containing the private key bytes
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		// If the file does not exist, return an error
		log.Printf("Failed to read %s: %v", fileName, err)
		return nil, "", err
	}

	// Unmarshal the storage data
	var peerIDStorage PeerIDStorage
	err = json.Unmarshal(data, &peerIDStorage)
	if err != nil {
		log.Printf("Failed to unmarshal private key bytes from %s: %v", fileName, err)
		return nil, "", err
	}

	// Ensure the private key bytes exist
	if peerIDStorage.PrivateKeyBytes == nil {
		log.Printf("Private key bytes are missing in the file.")
		return nil, "", errors.New("private key bytes are empty in storage")
	}

	// Recreate the private key from the bytes
	privKey, err := crypto.UnmarshalPrivateKey(peerIDStorage.PrivateKeyBytes)
	if err != nil {
		log.Printf("Failed to unmarshal private key from bytes: %v", err)
		return nil, "", err
	}

	// Generate the PeerID from the private key
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		log.Printf("Failed to generate PeerID from private key: %v", err)
		return nil, "", err
	}

	log.Printf("Loaded private key and PeerID successfully from %s", fileName)
	return privKey, peerID, nil
}

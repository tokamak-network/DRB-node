package utils

import (
	"log"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const leaderNodeFile = "static-key/leadernode.bin"

func GeneratePeerID() {
	var privKey crypto.PrivKey

	// Check if the key file exists
	if _, err := os.Stat(leaderNodeFile); os.IsNotExist(err) {
		// File does not exist, generate a new private key
		log.Printf("Generating new private key...")
		privKey, _, err = crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			log.Println("failed to generate private key:", err)
			return
		}

		buff, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			log.Println("failed to marshal private key:", err)
			return
		}

		dir := filepath.Dir(leaderNodeFile)

		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Println("failed to create directory:", err)
			return
		}

		err = os.WriteFile(leaderNodeFile, buff, 0644)
		if err != nil {
			log.Println("failed to write private key to file:", err)
			return
		}
	} else if err != nil {
		// Handle other potential errors when checking for the file.
		log.Printf("Error checking for file '%s': %v\n", leaderNodeFile, err)
		os.Exit(1)
	} else {
		log.Printf("File '%s' already exists. No new peer ID generated.\n", leaderNodeFile)
		return
	}

	// Generate the PeerID from the private key
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		log.Printf("Failed to generate PeerID from private key: %v", err)
		return
	}

	log.Println("New PeerID Generated:", peerID)
}

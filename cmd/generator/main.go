package main

import (
	"fmt"
	"os"

	"github.com/tokamak-network/DRB-node/utils"
)

func main() {
	peerID, err := utils.GeneratePeerID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating leader peer ID: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nPlease add the following to your .env file:")
	fmt.Printf("LEADER_PEER_ID=%s\n", peerID)
}

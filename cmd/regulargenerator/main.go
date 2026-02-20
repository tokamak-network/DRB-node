package main

import (
	"fmt"
	"os"

	"github.com/tokamak-network/DRB-node/utils"
)

func main() {
	if len(os.Args) > 1 {
		peerID, err := utils.GenerateRegularPeerIDForNode(0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating regular node peer ID: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\nPlease add the following to your .env file:")
		fmt.Printf("REGULAR_PEER_ID=%s\n", peerID)
		return
	}

	peerIDs, err := utils.GenerateRegularPeerIDs(3)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating regular node peer IDs: %v\n", err)
		os.Exit(1)
	}

	if len(peerIDs) != 3 {
		fmt.Fprintf(os.Stderr, "Expected 3 peer IDs, got %d\n", len(peerIDs))
		os.Exit(1)
	}

	fmt.Println("\nPlease add the following to your .env file:")
	fmt.Printf("REGULAR1_PEER_ID=%s\n", peerIDs[0])
	fmt.Printf("REGULAR2_PEER_ID=%s\n", peerIDs[1])
	fmt.Printf("REGULAR3_PEER_ID=%s\n", peerIDs[2])
}

package libp2putils

import (
	"context"
	"fmt"
	"log"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
	"github.com/tokamak-network/DRB-node/database"
)

var (
	privKey crypto.PrivKey
	peerID  peer.ID
)

// CreateHost creates a new libp2p host with a given port and private key.
func CreateHost(port string) (host.Host, peer.ID, error) {
	nodeInfo, err := database.GetNodeInfo()
	if nodeInfo.PeerID == "" {
		log.Println("PeerID not found, generating a new one.")
		privKey, _, err = crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate private key: %v", err)
		}

		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get PeerID from private key: %v", err)
		}

		// Convert the private key to bytes
		privKeyBytes, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			log.Printf("Failed to marshal private key: %v", err)
			return nil, "", fmt.Errorf("Failed to marshal private key: %v", err)
		}

		nodeInfo.PrivateKey = privKeyBytes
		nodeInfo.PeerID = string(peerID)

		err = database.UpdateNodeInfo(nodeInfo)
		if err != nil {
			return nil, "", fmt.Errorf("failed to save nodeInfo: %v", err)
		}
	}

	// Recreate the private key from the bytes
	privKey, err = crypto.UnmarshalPrivateKey(nodeInfo.PrivateKey)
	if err != nil {
		log.Printf("Failed to unmarshal private key from bytes: %v", err)
		return nil, "", err
	}

	// Generate the PeerID from the private key
	peerID, err = peer.IDFromPrivateKey(privKey)
	if err != nil {
		log.Printf("Failed to generate PeerID from private key: %v", err)
		return nil, "", err
	}

	h, err := libp2p.New(libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)), libp2p.Identity(privKey))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create libp2p host: %v", err)
	}

	log.Printf("Host created with PeerID: %s", peerID.String())
	return h, peerID, nil
}

// ConnectToPeer connects to a specified peer using its multiaddress.
func ConnectToPeer(h host.Host, leaderIP, leaderPort, leaderPeerID string) (*peer.AddrInfo, error) {
	leaderAddrString := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", leaderIP, leaderPort, leaderPeerID)
	log.Printf("Leader multiaddress: %s", leaderAddrString)

	leaderAddr, err := multiaddr.NewMultiaddr(leaderAddrString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse leader multiaddress: %v", err)
	}

	leaderInfo, err := peer.AddrInfoFromP2pAddr(leaderAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer info from leader multiaddress: %v", err)
	}

	h.Peerstore().AddAddrs(leaderInfo.ID, leaderInfo.Addrs, peerstore.PermanentAddrTTL)
	return leaderInfo, h.Connect(context.Background(), *leaderInfo)
}

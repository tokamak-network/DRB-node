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
	"github.com/tokamak-network/DRB-node/utils"
)

// CreateHost creates a new libp2p host with a given port and private key.
func CreateHost(port string) (host.Host, peer.ID, error) {
	privKey, peerID, err := utils.LoadPeerID()
	if err != nil {
		log.Println("PeerID not found, generating a new one.")
		privKey, _, err = crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate private key: %v", err)
		}

		err = utils.SavePeerID(privKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to save PeerID: %v", err)
		}

		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get PeerID from private key: %v", err)
		}
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

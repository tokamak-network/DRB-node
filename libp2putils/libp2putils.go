package libp2putils

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
	"github.com/tokamak-network/DRB-node/database"
)

var (
	HostInstance host.Host
)

type NodeInfo struct {
	IP     string  `json:"ip"`
	Port   string  `json:"port"`
	PeerID peer.ID `json:"peer_id"`
}

func SetHost(h host.Host) {
	HostInstance = h
}

// CreateHost creates a new libp2p host with a given port and private key.
func CreateHost(port string, nodeType string) (host.Host, peer.ID, error) {
	// Define the file path based on nodeType to separate keys for leader and regular nodes
	filePath := fmt.Sprintf("/app/static-key/%snode.bin", nodeType)

	var privKey crypto.PrivKey

	// Check if the key file exists
	if _, err := os.Stat(filePath); err == nil {
		// File exists, load the private key
		log.Printf("Loading private key for %s from file: %s", nodeType, filePath)
		buff, err := os.ReadFile(filePath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to load private key: %v", err)
		}

		privKey, err = crypto.UnmarshalPrivateKey(buff)
		if err != nil {
			return nil, "", fmt.Errorf("failed to unmarshal private key: %v", err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// File does not exist, generate a new private key
		log.Printf("Generating new private key for %s", nodeType)
		privKey, _, err = crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate private key: %v", err)
		}

		buff, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal private key: %v", err)
		}

		err = os.WriteFile(filePath, buff, 0644)
		if err != nil {
			return nil, "", fmt.Errorf("failed to write private key to file: %v", err)
		}
	} else {
		return nil, "", fmt.Errorf("error checking private key file: %v", err)
	}

	// Generate the PeerID from the private key
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		log.Printf("Failed to generate PeerID from private key: %v", err)
		return nil, "", err
	}

	// Create the libp2p host with the private key identity
	h, err := libp2p.New(
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)),
		libp2p.Identity(privKey),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create libp2p host: %v", err)
	}

	log.Printf("%s host created with PeerID: %s", nodeType, peerID.String())
	return h, peerID, nil
}

// ConnectToPeer connects to a specified peer using its multiaddress.
func ConnectToPeer(h host.Host, leaderIP, leaderPort, leaderPeerID string) (*peer.AddrInfo, error) {
	// leaderAddrString := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", leaderIP, leaderPort, leaderPeerID)
	leaderAddrString := fmt.Sprintf("/dns/leadernode/tcp/%s/p2p/%s", leaderPort, leaderPeerID)
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

func GetConnectedPeers() map[string]NodeInfo {
	nodes, err := database.GetNodeInfos()
	if err != nil {
		log.Printf("Failed to get node infos: %v", err)
		return nil
	}

	finalNodes := make(map[string]NodeInfo)

	for _, node := range nodes {
		multiAddrStr := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", node.IP, node.Port, node.PeerID)
		multiAddr, err := multiaddr.NewMultiaddr(multiAddrStr)
		if err != nil {
			log.Printf("Failed to create multiaddress for EOA %s: %v", node.EOAAddress, err)
			continue
		}

		addrInfo, err := peer.AddrInfoFromP2pAddr(multiAddr)
		if err != nil {
			log.Printf("Failed to create AddrInfo for EOA %s: %v", node.EOAAddress, err)
			continue
		}

		finalNodes[node.EOAAddress] = NodeInfo{
			IP:     node.IP,
			Port:   node.Port,
			PeerID: addrInfo.ID,
		}
	}

	return finalNodes
}

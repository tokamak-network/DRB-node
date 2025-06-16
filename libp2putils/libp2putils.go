package libp2putils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

var HostInstance host.Host
var RegisteredNodes = make(map[string]peer.AddrInfo)
var mu sync.Mutex

func SetHost(h host.Host) {
	HostInstance = h
}

// CreateHost creates a new libp2p host with a given port and private key.
func CreateHost(port string) (host.Host, peer.ID, error) {
	if port == "" {
		return nil, "", fmt.Errorf("missing port")
	}

	privKey, peerID, err := utils.LoadPeerID()
	if err != nil {
		logger.Info("PeerID not found, generating a new one.")
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

	logger.Infof("Host created with PeerID: %s", peerID.String())
	return h, peerID, nil
}

// ConnectToPeer connects to a specified peer using its multiaddress.
func ConnectToPeer(h host.Host, leaderIP, leaderPort, leaderPeerID string) (*peer.AddrInfo, error) {
	leaderAddrString := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", leaderIP, leaderPort, leaderPeerID)
	logger.Infof("Leader multiaddress: %s", leaderAddrString)

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

func GetConnectedPeers() map[string]struct {
	IP     string  `json:"ip"`
	Port   string  `json:"port"`
	PeerID peer.ID `json:"peer_id"`
} {
	mu.Lock()
	defer mu.Unlock()

	filePath := "registered_nodes.json"
	file, err := os.Open(filePath)
	if err != nil {
		logger.Infof("Failed to open registered_nodes.json: %v", err)
		return nil
	}
	defer file.Close()

	var nodes map[string]struct {
		IP     string `json:"ip"`
		Port   string `json:"port"`
		PeerID string `json:"peer_id"`
	}
	data, err := io.ReadAll(file)
	if err != nil {
		logger.Infof("Failed to read registered_nodes.json: %v", err)
		return nil
	}

	err = json.Unmarshal(data, &nodes)
	if err != nil {
		logger.Infof("Failed to parse registered_nodes.json: %v", err)
		return nil
	}

	finalNodes := make(map[string]struct {
		IP     string  `json:"ip"`
		Port   string  `json:"port"`
		PeerID peer.ID `json:"peer_id"`
	})

	for eoa, node := range nodes {
		multiAddrStr := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", node.IP, node.Port, node.PeerID)
		multiAddr, err := multiaddr.NewMultiaddr(multiAddrStr)
		if err != nil {
			logger.Infof("Failed to create multiaddress for EOA %s: %v", eoa, err)
			continue
		}

		addrInfo, err := peer.AddrInfoFromP2pAddr(multiAddr)
		if err != nil {
			logger.Infof("Failed to create AddrInfo for EOA %s: %v", eoa, err)
			continue
		}

		finalNodes[eoa] = struct {
			IP     string  `json:"ip"`
			Port   string  `json:"port"`
			PeerID peer.ID `json:"peer_id"`
		}{
			IP:     node.IP,
			Port:   node.Port,
			PeerID: addrInfo.ID,
		}
	}

	return finalNodes
}

package libp2putils

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/multiformats/go-multiaddr"
	"github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/database"
)

type P2PClient struct {
	hostInstance       host.Host
	nodeInfoRepository database.INodeInfoRepository
}

func NewP2PClient(nodeInfoRepository database.INodeInfoRepository) *P2PClient {
	return &P2PClient{
		nodeInfoRepository: nodeInfoRepository,
	}
}

type NodeInfo struct {
	IP     string  `json:"ip"`
	Port   string  `json:"port"`
	PeerID peer.ID `json:"peer_id"`
}

func (p *P2PClient) SetHost(h host.Host) {
	p.hostInstance = h
}

func (p *P2PClient) GetHostInstance() host.Host {
	return p.hostInstance
}

// CreateHost creates a new libp2p host with a given port and private key.
func (p *P2PClient) CreateHost(port string, nodeType string) (host.Host, peer.ID, error) {
	// Load configuration once
	cfg := config.Get()

	var keyFileName string
	if nodeType == "regular" {
		if cfg.RegularNodeNumber == "" {
			keyFileName = "regularnode.bin"
		} else {
			keyFileName = fmt.Sprintf("regularnode%s.bin", cfg.RegularNodeNumber)
		}
	} else if nodeType == "leader" {
		keyFileName = "leadernode.bin"
	} else {
		return nil, "", fmt.Errorf("invalid nodeType: %s. nodeType must be 'leader' or 'regular'", nodeType)
	}

	filePath := fmt.Sprintf("static-key/%s", keyFileName)

	var privKey crypto.PrivKey

	// Check if the key file exists
	if _, err := os.Stat(filePath); err == nil {
		log.Printf("Loading private key for %s from: %s", nodeType, filePath)
	} else {
		return nil, "", fmt.Errorf("private key file '%s' not found in 'static-key/' directory. Please generate peer ID using run_generator.sh (for leader) or run_regulargenerator.sh (for regular nodes)", keyFileName)
	}

	buff, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load private key from '%s': %v", filePath, err)
	}

	privKey, err = crypto.UnmarshalPrivateKey(buff)
	if err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal private key from '%s': %v", filePath, err)
	}

	// Generate the PeerID from the private key
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		log.Printf("Failed to generate PeerID from private key: %v", err)
		return nil, "", err
	}

	if nodeType == "regular" {
		envVarName := "REGULAR_PEER_ID"
		regularPeerIDFromEnv := cfg.RegularPeerID

		if regularPeerIDFromEnv == "" {
			return nil, "", fmt.Errorf("%s is required for regular nodes. Please generate peer ID and add %s=%s to your .env file", envVarName, envVarName, peerID.String())
		}
		if regularPeerIDFromEnv != peerID.String() {
			return nil, "", fmt.Errorf("%s from environment (%s) does not match peer ID from key file (%s). Please ensure %s matches the generated peer ID", envVarName, regularPeerIDFromEnv, peerID.String(), envVarName)
		}
		log.Printf("%s validated successfully: %s", envVarName, regularPeerIDFromEnv)
	}

	limits := rcmgr.DefaultLimits
	limits.SystemBaseLimit.StreamsInbound = 512
	limits.SystemBaseLimit.StreamsOutbound = 512
	limits.SystemBaseLimit.ConnsInbound = 256
	limits.SystemBaseLimit.ConnsOutbound = 256
	limits.SystemBaseLimit.FD = 512

	// per connection per peer limit
	limits.ConnBaseLimit.StreamsInbound = 64
	limits.ConnBaseLimit.StreamsOutbound = 64

	limits.SystemBaseLimit.Memory = 1 << 30

	scaledLimits := limits.Scale(256, 512)

	rm, err := rcmgr.NewResourceManager(
		rcmgr.NewFixedLimiter(scaledLimits),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create resource manager: %v", err)
	}

	h, err := libp2p.New(
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)),
		libp2p.Identity(privKey),
		libp2p.ResourceManager(rm),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create libp2p host: %v", err)
	}

	log.Printf("%s host created with PeerID: %s (Resource limits: %d streams, %d conns)",
		nodeType, peerID.String(), 512, 256)
	return h, peerID, nil
}

// ConnectToPeer connects to a specified peer using its multiaddress.
func (p *P2PClient) ConnectToPeer(ctx context.Context, leaderIP, leaderPort, leaderPeerID string) (*peer.AddrInfo, error) {
	var leaderAddrString string
	if net.ParseIP(leaderIP) != nil {
		leaderAddrString = fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", leaderIP, leaderPort, leaderPeerID)
	} else {
		leaderAddrString = fmt.Sprintf("/dns/%s/tcp/%s/p2p/%s", leaderIP, leaderPort, leaderPeerID)
	}
	log.Printf("Leader multiaddress: %s", leaderAddrString)

	leaderAddr, err := multiaddr.NewMultiaddr(leaderAddrString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse leader multiaddress: %v", err)
	}

	leaderInfo, err := peer.AddrInfoFromP2pAddr(leaderAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer info from leader multiaddress: %v", err)
	}

	p.hostInstance.Peerstore().AddAddrs(leaderInfo.ID, leaderInfo.Addrs, peerstore.PermanentAddrTTL)
	return leaderInfo, p.hostInstance.Connect(ctx, *leaderInfo)
}

func (p *P2PClient) GetConnectedPeers(ctx context.Context) map[string]NodeInfo {
	nodes, err := p.nodeInfoRepository.GetNodeInfos(ctx)
	if err != nil {
		log.Printf("Database connection error while getting node infos: %v", err)
		return nil
	}
	if len(nodes) == 0 {
		log.Printf("No node infos found.")
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

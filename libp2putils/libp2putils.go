package libp2putils // Renamed the package to avoid conflict with imported libp2p package

import (
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host" // Correctly import the Host type
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
)

// CreateLibp2pNode creates and returns a new libp2p Host
func CreateLibp2pNode(port string) (*host.Host, error) {
	h, err := libp2p.New(
		libp2p.DefaultTransports,
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)),
	)
	if err != nil {
		return nil, fmt.Errorf("Failed to create libp2p host: %v", err)
	}

	return &h, nil  // Return pointer to host
}

// SetupStreamHandler sets up a stream handler for the provided host
func SetupStreamHandler(h *host.Host, handlerFunc func(network.Stream)) {
	(*h).SetStreamHandler("/register", handlerFunc)
}

// AddPeerToPeerstore adds a peer to the peerstore of the provided host
func AddPeerToPeerstore(h *host.Host, leaderAddr string) error {
	leaderAddrParsed, err := multiaddr.NewMultiaddr(leaderAddr)
	if err != nil {
		return fmt.Errorf("Failed to parse leader multiaddress: %v", err)
	}

	leaderInfo, err := peer.AddrInfoFromP2pAddr(leaderAddrParsed)
	if err != nil {
		return fmt.Errorf("Failed to create peer info from leader multiaddress: %v", err)
	}

	(*h).Peerstore().AddAddrs(leaderInfo.ID, leaderInfo.Addrs, peerstore.PermanentAddrTTL)
	return nil
}

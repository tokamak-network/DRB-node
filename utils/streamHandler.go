package utils

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

// CreateStream establishes a stream to a regular node for a given protocol
func CreateStream(h host.Host, nodeInfo NodeInfo, protocolStr string) (network.Stream, error) {
	// Format the peer address
	peerAddr := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", nodeInfo.IP, nodeInfo.Port, nodeInfo.PeerID)
	maddr, err := multiaddr.NewMultiaddr(peerAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse multiaddr: %v", err)
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer info from multiaddr: %v", err)
	}

	// Add the peer address to the peerstore
	h.Peerstore().AddAddrs(peerInfo.ID, peerInfo.Addrs, peerstore.PermanentAddrTTL)

	// Convert the protocol string to protocol.ID
	protoID := protocol.ID(protocolStr)

	// Open a stream to the peer using the specified protocol
	stream, err := h.NewStream(context.Background(), peerInfo.ID, protoID)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %v", err)
	}

	return stream, nil
}

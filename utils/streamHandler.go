package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

// CreateStream establishes a stream to a regular node for a given protocol
func CreateStream(ctx context.Context, h host.Host, nodeInfo NodeInfo, protocolStr string) (network.Stream, error) {
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
	stream, err := h.NewStream(ctx, peerInfo.ID, protoID)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %v", err)
	}

	return stream, nil
}

func DecodeJSONWithContext(ctx context.Context, s network.Stream, v interface{}) error {
	done := make(chan error, 1)
	go func() {
		err := json.NewDecoder(s).Decode(v)
		done <- err
	}()
	select {
	case err := <-done:
		return err

	case <-ctx.Done():
		s.Reset()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("JSON decode timeout: %w", ctx.Err())
		}
		return fmt.Errorf("JSON decode cancelled: %w", ctx.Err())
	}
}

package libp2putils

import (
	"context"
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

var topic *pubsub.Topic
var topicNameFlag = "renode"

func initDHT(ctx context.Context, h host.Host, leadernode []multiaddr.Multiaddr) (*dht.IpfsDHT, error) {
	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	kademliaDHT, err := dht.New(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("failed to create DHT: %w", err)
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("failed to bootstrap DHT: %w", err)
	}
	var wg sync.WaitGroup
	for _, peerAddr := range leadernode {
		peerinfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create peer info: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Connect(ctx, *peerinfo); err != nil {
				fmt.Println("Bootstrap warning:", err)
			}
		}()
	}
	wg.Wait()

	return kademliaDHT, nil
}

func discoverPeers(h host.Host, leadernode []multiaddr.Multiaddr) error {
	ctx := context.Background()
	kademliaDHT, err := initDHT(ctx, h, leadernode)
	if err != nil {
		return fmt.Errorf("failed to initialize DHT: %w", err)
	}
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, topicNameFlag)

	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		peerChan, err := routingDiscovery.FindPeers(ctx, topicNameFlag)
		if err != nil {
			return fmt.Errorf("failed to find peers: %w", err)
		}
		for peer := range peerChan {
			if peer.ID == h.ID() {
				continue // No self connection
			}
			err := h.Connect(ctx, peer)
			if err != nil {
				// fmt.Println("Failed connecting to ", peer.ID.Pretty(), ", error:", err)
			} else {
				fmt.Println("Connected to:", peer.ID)
				anyConnected = true
			}
		}
	}
	fmt.Println("Peer discovery complete")
	return nil
}

func subscribe(h host.Host) error {
	ctx := context.Background()

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return fmt.Errorf("failed to create gossipsub: %w", err)
	}

	topic, err = ps.Join(topicNameFlag)
	if err != nil {
		return fmt.Errorf("failed to join topic: %w", err)
	}

	sub, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	handleMessageFromSub(ctx, sub)

	return nil
}

func handleMessageFromSub(ctx context.Context, sub *pubsub.Subscription) error {
	for {
		m, err := sub.Next(ctx)
		if err != nil {
			return fmt.Errorf("failed to get next message: %w", err)
		}

		go handleIncomingMessage(m.Message.Data) // handle incoming message
	}
}

func createHost(listenHost string, port int, leadernodeURL string) (host.Host, multiaddr.Multiaddr, error) {
	h, err := libp2p.New(libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d", listenHost, port)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create host: %w", err)
	}

	addr, err := multiaddr.NewMultiaddr(leadernodeURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create multiaddr: %w", err)
	}

	return h, addr, nil
}

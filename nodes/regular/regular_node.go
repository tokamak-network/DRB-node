package regular_node

import "github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"

type RegularNode struct {
	fallbackEthClient *fallback_ethclient.FallbackRPCClient
}

func NewRegularNode(fallbackEthClient *fallback_ethclient.FallbackRPCClient) *RegularNode {
	return &RegularNode{
		fallbackEthClient: fallbackEthClient,
	}
}

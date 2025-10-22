package leader_node

import "github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"

type LeaderNode struct {
	fallbackEthClient *fallback_ethclient.FallbackRPCClient
}

func NewLeaderNode(fallbackEthClient *fallback_ethclient.FallbackRPCClient) *LeaderNode {
	return &LeaderNode{
		fallbackEthClient: fallbackEthClient,
	}
}

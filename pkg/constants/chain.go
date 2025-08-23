package constants

import (
	"time"

	"github.com/tokamak-network/DRB-node/pkg/types"
)

var Chains = map[uint64]types.NetworkInfo{
	1:        {Name: "Ethereum Mainnet", BlockTime: 12 * time.Second},
	11155111: {Name: "Ethereum Sepolia", BlockTime: 12 * time.Second},
	10:       {Name: "Optimism Mainnet", BlockTime: 2 * time.Second},
	11155420: {Name: "Optimism Sepolia", BlockTime: 2 * time.Second},
}

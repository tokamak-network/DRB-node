package constants

import (
	"time"

	"github.com/tokamak-network/DRB-node/pkg/types"
)

var Chains = map[uint64]types.NetworkInfo{
	1:        {Name: "Ethereum Mainnet", BlockTime: 12 * time.Second, EstimatedGasFactorPercent: 150},
	11155111: {Name: "Ethereum Sepolia", BlockTime: 12 * time.Second, EstimatedGasFactorPercent: 150},
	10:       {Name: "Optimism Mainnet", BlockTime: 2 * time.Second, EstimatedGasFactorPercent: 200},
	11155420: {Name: "Optimism Sepolia", BlockTime: 2 * time.Second, EstimatedGasFactorPercent: 200},
	1337:     {Name: "Geth Dev", BlockTime: 1 * time.Second, EstimatedGasFactorPercent: 150}, // Geth dev mode for testing
}

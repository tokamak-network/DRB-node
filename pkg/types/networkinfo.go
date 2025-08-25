package types

import "time"

type NetworkInfo struct {
	Name                      string
	BlockTime                 time.Duration // average time between blocks
	EstimatedGasFactorPercent uint64        // factor to multiply the estimated gas by
}

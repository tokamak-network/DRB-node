package types

import "time"

type NetworkInfo struct {
	Name      string
	BlockTime time.Duration // average time between blocks
}

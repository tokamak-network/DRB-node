package leader

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/common"
)

type Config struct {
	Port             uint64
	ContractAddress  common.Address
	LeaderPrivateKey *ecdsa.PrivateKey
}

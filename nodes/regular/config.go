package regular

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/common"
)

type Config struct {
	Port            string
	LeaderIP        string
	LeaderPort      string
	LeaderPeerID    string
	EOAPrivateKey   *ecdsa.PrivateKey
	ContractAddress common.Address
}

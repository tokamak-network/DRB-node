package utils

import (
	"crypto/ecdsa"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Constants defining durations and timeouts
const (
	CommitDuration  = 70  // Commit duration in seconds
	DisputeDuration = 130 // Dispute duration in seconds
	ContextTimeout  = 600000
)

// BigNumber represents a big number with value and bit length.
type BigNumber struct {
	Val    []byte   `json:"val"`
	Bitlen *big.Int `json:"bitlen"`
}

// PoFClient contains essential data for interacting with the Ethereum client and smart contracts.
type PoFClient struct {
	Client          *ethclient.Client
	ContractAddress common.Address
	ContractABI     abi.ABI
	PrivateKey      *ecdsa.PrivateKey
	Mutex           sync.Mutex
	WaitGroup       sync.WaitGroup
	LeaderRounds    map[*big.Int]common.Address
	MyAddress       common.Address
}

// RecoveredData represents data for a recovered round.
type RecoveredData struct {
	Round          string `json:"round"`
	BlockTimestamp string `json:"blockTimestamp"`
	ID             string `json:"id"`
	MsgSender      string `json:"msgSender"`
	Omega          string `json:"omega"`
	IsRecovered    bool   `json:"isRecovered"`
}

// FulfillRandomnessData represents the data structure for fulfilling randomness.
type FulfillRandomnessData struct {
	MsgSender      string
	BlockTimestamp string
	Success        bool
}

// RandomWordRequestedStruct represents the structure for a requested random word.
type RandomWordRequestedStruct struct {
	BlockTimestamp string `json:"blockTimestamp"`
	RoundInfo      struct {
		CommitCount       string `json:"commitCount"`
		ValidCommitCount  string `json:"validCommitCount"`
		IsRecovered       bool   `json:"isRecovered"`
		IsFulfillExecuted bool   `json:"isFulfillExecuted"`
	} `json:"roundInfo"`
	Round string `json:"round"`
}

// CommitData represents the data structure for a commit.
type CommitData struct {
	Round          string `json:"round"`
	MsgSender      string `json:"msgSender"`
	BlockTimestamp string `json:"blockTimestamp"`
	CommitIndex    string `json:"commitIndex"`
	CommitVal      string `json:"commitVal"`
	ID             string `json:"id"`
}

// RoundResults contains various categories of rounds based on their status.
type RoundResults struct {
	RecoverableRounds           []string
	CommittableRounds           []string
	FulfillableRounds           []string
	ReRequestableRounds         []string
	RecoverDisputeableRounds    []string
	LeadershipDisputeableRounds []string
	CompleteRounds              []string
	RecoveryData                []RecoveryResult
}

// SetupValues holds values used during setup, such as big integers and their respective lengths.
type SetupValues struct {
	T       *big.Int
	NBitLen *big.Int
	GBitLen *big.Int
	HBitLen *big.Int
	NVal    []byte `json:"nVal"`
	GVal    []byte `json:"gVal"`
	HVal    []byte `json:"hVal"`
}

// OperatorNumberChanged represents the structure for operator number change.
type OperatorNumberChanged struct {
	IsOperator bool `json:"isOperator"`
}

// RecoveryResult contains results related to the recovery process.
type RecoveryResult struct {
	OmegaRecov *big.Int
	X          BigNumber
	Y          BigNumber
	V          []BigNumber
}

// RoundItemType represents a round and its associated data.
type RoundItemType struct {
	Round string `json:"round"`
	Data  struct {
		CommitCount       string `json:"commitCount"`
		ValidCommitCount  string `json:"validCommitCount"`
		IsRecovered       bool   `json:"isRecovered"`
		IsFulfillExecuted bool   `json:"isFulfillExecuted"`
	} `json:"data"`
}

// CommitCsData represents the structure of commitCs data.
type CommitCsData struct {
	Round          string `json:"round"`
	CommitIndex    string `json:"commitIndex"`
	CommitVal      string `json:"commitVal"`
	ID             string `json:"id"`
	BlockTimestamp string `json:"blockTimestamp"`
	MsgSender      string `json:"msgSender"`
}

// CommitCsResponse represents the response structure for a commitCs query.
type CommitCsResponse struct {
	Data struct {
		CommitCs []CommitCsData `json:"commitCs"`
	} `json:"data"`
}

// GetConfig returns the loaded configuration.
func GetConfig() Config {
	return LoadConfig()
}

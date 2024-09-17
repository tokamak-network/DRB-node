package utils

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Client contains essential data for interacting with the Ethereum client and smart contracts.
type Client struct {
	Client          *ethclient.Client
	ContractAddress common.Address
	ContractABI     abi.ABI
	PrivateKey      *ecdsa.PrivateKey
	MyAddress       common.Address
}

// RandomWordRequestedStruct represents the structure for a requested random word.
type RandomWordRequestedStruct struct {
	RequestedTimestamp string `json:"requestedTimestamp"`
	CommitCount        string `json:"commitCount"`
	RevealCount  	   string `json:"revealCount"`
	IsRefunded         bool   `json:"isRefunded"`
	Round 			   string `json:"round"`
}

// RoundResults contains various categories of rounds based on their status.
type RoundResults struct {
	CommitRounds           []string
	RevealRounds           []string
}

type Config struct {
	RpcURL             string `json:"RpcURL"`
	HttpURL            string `json:"HttpURL"`
	ContractAddress    string `json:"ContractAddress"`
	SubgraphURL        string `json:"SubgraphURL"`
	OperatorDepositFee string `json:"OperatorDepositFee"`
}

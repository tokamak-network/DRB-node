// file: client/client.go

package client

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tokamak-network/DRB-Node/utils"
	// Import the service package
)

// NewPoFClient initializes and returns a new PoFClient instance.
func NewPoFClient(config utils.Config) (*utils.PoFClient, error) {
	client, err := ethclient.Dial(config.HttpURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the Ethereum client: %v", err)
	}

	privateKey, err := crypto.HexToECDSA(config.PrivateKey[2:])
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	contractABI, err := LoadContractABI("../contract/abi/CRRNGCoordinatorPoF.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load contract ABI: %v", err)
	}

	contractAddress := common.HexToAddress(config.ContractAddress)
	myAddress := common.HexToAddress(config.WalletAddress)

	return &utils.PoFClient{
		Client:          client,
		ContractAddress: contractAddress,
		ContractABI:     contractABI,
		PrivateKey:      privateKey,
		LeaderRounds:    make(map[*big.Int]common.Address),
		MyAddress:       myAddress,
	}, nil
}

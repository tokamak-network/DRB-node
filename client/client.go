// file: client/client.go

package client

import (
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tokamak-network/DRB-Node/logger"
	"github.com/tokamak-network/DRB-Node/utils"
	// Import the service package
)

// NewPoFClient initializes and returns a new PoFClient instance.
func NewPoFClient(config utils.Config) (*utils.PoFClient, error) {
	client, err := ethclient.Dial(config.HttpURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the Ethereum client: %v", err)
	}

	privateKeyHex := os.Getenv("PRIVATE_KEY")
	if privateKeyHex == "" {
		logger.Log.Error("PRIVATE_KEY environment variable is not set")
		return nil, fmt.Errorf("PRIVATE_KEY environment variable is not set")
	} else {
		logger.Log.Info("PRIVATE_KEY loaded: %s") // Debug log
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex[2:])
	if err != nil {
		logger.Log.Error("Failed to parse private key: %v", err)
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	contractABI, err := LoadContractABI("./contract/abi/CRRNGCoordinatorPoF.json")
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

// file: client/client.go

package client

import (
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

// NewClient initializes and returns a new Client instance.
func NewClient(config utils.Config) (*utils.Client, error) {
	client, err := ethclient.Dial(config.HttpURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the Ethereum client: %v", err)
	}

	privateKeyHex := os.Getenv("PRIVATE_KEY")
	if privateKeyHex == "" {
		logger.Log.Error("PRIVATE_KEY environment variable is not set")
		return nil, fmt.Errorf("PRIVATE_KEY environment variable is not set")
	} else {
		logger.Log.Info("PRIVATE_KEY loaded")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex[2:])
	if err != nil {
		logger.Log.Errorf("Failed to parse private key: %v", err)
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Fetch WalletAddress from .env file
	walletAddressHex := os.Getenv("WALLET_ADDRESS")
	if walletAddressHex == "" {
		logger.Log.Error("WALLET_ADDRESS environment variable is not set")
		return nil, fmt.Errorf("WALLET_ADDRESS environment variable is not set")
	}

	contractABI, err := LoadContractABI("./contract/abi/DRBCoordinator.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load contract ABI: %v", err)
	}

	contractAddress := common.HexToAddress(config.ContractAddress)
	myAddress := common.HexToAddress(walletAddressHex)

	return &utils.Client{
		Client:          client,
		ContractAddress: contractAddress,
		ContractABI:     contractABI,
		PrivateKey:      privateKey,
		LeaderRounds:    make(map[*big.Int]common.Address),
		MyAddress:       myAddress,
	}, nil
}

// ConnectToEthereumClient establishes a connection to an Ethereum client using the provided URL.
//func ConnectToEthereumClient(url string) (*ethclient.Client, error) {
//	client, err := ethclient.Dial(url)
//	if err != nil {
//		logger.Log.Errorf("Failed to connect to Ethereum client at %s: %v", url, err)
//		return nil, err
//	}
//	logger.Log.Infof("Connected to Ethereum client at %s", url)
//	return client, nil
//}

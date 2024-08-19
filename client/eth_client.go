package client

import (
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tokamak-network/DRB-Node/logger"
)

// ConnectToEthereumClient establishes a connection to an Ethereum client using the provided URL.
func ConnectToEthereumClient(url string) (*ethclient.Client, error) {
	client, err := ethclient.Dial(url)
	if err != nil {
		logger.Log.Error("Failed to connect to Ethereum client at %s: %v", url, err)
		return nil, err
	}
	logger.Log.Info("Connected to Ethereum client at %s", url)
	return client, nil
}

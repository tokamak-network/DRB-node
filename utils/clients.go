package utils

import (
	"context"
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Client struct {
	Client          *ethclient.Client
	ContractABI     abi.ABI
	ContractAddress common.Address
	PrivateKey      *ecdsa.PrivateKey // Explicitly use *ecdsa.PrivateKey
}

// CallReadOnlyFunction executes a read-only call on the smart contract.
func (c *Client) CallReadOnlyFunction(ctx context.Context, functionName string, params ...interface{}) ([]byte, error) {
	callData, err := c.ContractABI.Pack(functionName, params...)
	if err != nil {
		return nil, err
	}

	msg := ethereum.CallMsg{
		To:   &c.ContractAddress,
		Data: callData,
	}

	result, err := c.Client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

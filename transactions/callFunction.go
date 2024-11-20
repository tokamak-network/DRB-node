package transactions

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Smart contract call helper function
func CallSmartContract(client *ethclient.Client, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	data, err := parsedABI.Pack(method, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack data for %s: %v", method, err)
	}

	result, err := client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &contractAddress,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract method %s: %v", method, err)
	}

	var unpackedResult interface{}
	err = parsedABI.UnpackIntoInterface(&unpackedResult, method, result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result for %s: %v", method, err)
	}

	return unpackedResult, nil
}
package eth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const abiFilePath = "contracts/commit2reveal_abi.json"

// LoadContractABI loads the ABI of the smart contract
func LoadContractABI(filename string) (abi.ABI, error) {
	abiBytes, err := os.ReadFile(filename)
	if err != nil {
		return abi.ABI{}, fmt.Errorf("failed to read ABI file: %v", err)
	}

	var abiObject struct {
		ABI json.RawMessage `json:"abi"`
	}

	err = json.Unmarshal(abiBytes, &abiObject)
	if err != nil {
		return abi.ABI{}, fmt.Errorf("failed to unmarshal ABI JSON: %v", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(string(abiObject.ABI)))
	if err != nil {
		return abi.ABI{}, fmt.Errorf("failed to parse contract ABI: %v", err)
	}

	return parsedABI, nil
}

// CallSmartContract calls a method on the smart contract
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

// GetEthereumClient connects to Ethereum RPC
func GetEthereumClient() (*ethclient.Client, error) {
	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to Ethereum client: %v", err)
	}
	return client, nil
}

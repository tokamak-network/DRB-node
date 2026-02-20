package utils

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	appconfig "github.com/tokamak-network/DRB-node/config"
)

type Client struct {
	ContractABI     abi.ABI
	ContractAddress common.Address
	PrivateKey      *ecdsa.PrivateKey // Explicitly use *ecdsa.PrivateKey
}

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

// NewClient creates a new Client with the given ABI path and private key.
func NewClient(abiPath string, privateKeyHex string) (*Client, error) {
	contractAddressStr := appconfig.Get().ContractAddress
	if contractAddressStr == "" {
		return nil, errors.New("CONTRACT_ADDRESS is not set in environment variables")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := LoadContractABI(abiPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load contract ABI: %v", err)
	}

	if privateKeyHex == "" {
		return nil, errors.New("private key is not provided")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %v", err)
	}

	return &Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}, nil
}

// NewLeaderClient creates a new Client using the leader's private key.
func NewLeaderClient(abiPath string) (*Client, error) {
	privateKeyHex := appconfig.Get().LeaderPrivateKey
	if privateKeyHex == "" {
		return nil, errors.New("LEADER_PRIVATE_KEY is not set in environment variables")
	}
	return NewClient(abiPath, privateKeyHex)
}

// NewEOAClient creates a new Client using the EOA's private key.
func NewEOAClient(abiPath string) (*Client, error) {
	privateKeyHex := appconfig.Get().EOAPrivateKey
	if privateKeyHex == "" {
		return nil, errors.New("EOA_PRIVATE_KEY is not set in environment variables")
	}
	return NewClient(abiPath, privateKeyHex)
}

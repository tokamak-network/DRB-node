package utils

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
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

func NewLeaderClient(abiPath string) (*Client, error) {
	contractAddressStr := appconfig.Get().ContractAddress
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := LoadContractABI(abiPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load contract ABI: %v", err)
	}

	privateKeyHex := appconfig.Get().LeaderPrivateKey
	if privateKeyHex == "" {
		log.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode leader private key: %v", err)
	}

	return &Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}, nil
}

func NewEOAClient(abiPath string) (*Client, error) {
	contractAddressStr := appconfig.Get().ContractAddress
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := LoadContractABI(abiPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load contract ABI: %v", err)
	}

	privateKeyHex := appconfig.Get().EOAPrivateKey
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode leader private key: %v", err)
	}

	return &Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}, nil
}

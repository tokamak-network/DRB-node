package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"log"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// LoadContractABI loads and parses the contract ABI from a JSON file.
func LoadContractABI(filename string) (abi.ABI, error) {
	fileContent, err := readFile(filename)
	if err != nil {
		return abi.ABI{}, err
	}

	abiObject, err := parseABIJSON(fileContent)
	if err != nil {
		return abi.ABI{}, err
	}

	contractAbi, err := convertToABI(abiObject)
	if err != nil {
		return abi.ABI{}, err
	}

	log.Printf("Successfully loaded contract ABI from %s", filename)
	return contractAbi, nil
}

// readFile reads the contents of a file.
func readFile(filename string) ([]byte, error) {
	fileContent, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read ABI file: %v", err)
	}
	return fileContent, nil
}

// parseABIJSON unmarshals the ABI JSON from the file content.
func parseABIJSON(fileContent []byte) (struct {
	Abi []interface{} `json:"abi"`
}, error) {
	var abiObject struct {
		Abi []interface{} `json:"abi"`
	}
	if err := json.Unmarshal(fileContent, &abiObject); err != nil {
		return abiObject, fmt.Errorf("failed to parse ABI JSON: %v", err)
	}
	return abiObject, nil
}

// convertToABI marshals the ABI object and converts it to an abi.ABI structure.
func convertToABI(abiObject struct {
	Abi []interface{} `json:"abi"`
}) (abi.ABI, error) {
	abiBytes, err := json.Marshal(abiObject.Abi)
	if err != nil {
		return abi.ABI{}, fmt.Errorf("failed to re-marshal ABI: %v", err)
	}

	contractAbi, err := abi.JSON(bytes.NewReader(abiBytes))
	if err != nil {
		return abi.ABI{}, fmt.Errorf("failed to parse contract ABI: %v", err)
	}
	return contractAbi, nil
}

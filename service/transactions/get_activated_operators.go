package transactions

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

// GetActivatedOperatorsAtRound fetches the activated operators for a specific round from the smart contract.
func GetActivatedOperatorsAtRound(ctx context.Context, round *big.Int, client *utils.Client) ([]common.Address, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"round": round,
	})

	// Pack the data for calling the getActivatedOperatorsAtRound function from the contract ABI
	data, err := client.ContractABI.Pack("getActivatedOperatorsAtRound", round)
	if err != nil {
		log.Errorf("Failed to pack data for getActivatedOperatorsAtRound: %v", err)
		return nil, fmt.Errorf("failed to pack data for getActivatedOperatorsAtRound: %v", err)
	}

	// Prepare the CallMsg for calling the contract
	callMsg := ethereum.CallMsg{
		To:   &client.ContractAddress, // Address of the smart contract
		Data: data,                    // ABI-packed data
	}

	// Call the contract and get the result
	result, err := client.Client.CallContract(ctx, callMsg, nil) // Use nil to indicate the latest block
	if err != nil {
		log.Errorf("Failed to call getActivatedOperatorsAtRound: %v", err)
		return nil, fmt.Errorf("failed to call getActivatedOperatorsAtRound: %v", err)
	}

	// Unpack the result into the expected output type
	var activatedOperators []common.Address
	err = client.ContractABI.UnpackIntoInterface(&activatedOperators, "getActivatedOperatorsAtRound", result)
	if err != nil {
		log.Errorf("Failed to unpack result: %v", err)
		return nil, fmt.Errorf("failed to unpack result: %v", err)
	}

	return activatedOperators, nil
}

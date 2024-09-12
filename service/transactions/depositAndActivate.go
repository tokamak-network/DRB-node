package transactions

import (
	"context"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/sirupsen/logrus"
	"github.com/tokamak-network/DRB-node/contract/DRBCoordinator"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
	"math/big"
)

// DepositAndActivate initializes and executes a transaction to deposit ETH and activate a contract.
func DepositAndActivate(ctx context.Context, ethClient *ethclient.Client, contractAddress common.Address, auth *bind.TransactOpts) (common.Address, *types.Transaction, error) {
	log := logger.Log.WithFields(logrus.Fields{})

	config := utils.GetConfig()
	amount := new(big.Int)
	amount.SetString(config.OperatorDespositFee, 10)
	auth.Value = amount

	// Create a new instance of the DRBCoordinator transactor
	coordinator, err := DRBCoordinator.NewDRBCoordinatorTransactor(contractAddress, ethClient)
	if err != nil {
		log.Errorf("Failed to create DRBCoordinator transactor: %v", err)
		return common.Address{}, nil, err
	}

	// Call the DepositAndActivate function on the contract
	tx, err := coordinator.DepositAndActivate(auth)
	if err != nil {
		log.Errorf("DepositAndActivate transaction failed: %v", err)
		return common.Address{}, nil, err
	}

	log.Infof("DepositAndActivate transaction sent. TX Hash: %s", tx.Hash().Hex())
	return auth.From, tx, nil
}

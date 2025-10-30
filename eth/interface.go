package eth

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// IEthService defines the interface for Ethereum-related operations.
// This makes package-level functions mockable in tests by depending on the interface.
type IEthService interface {
	// Activated operators management
	GetActivatedOperatorsCached() []common.Address
	GetActivatedOperatorsLength() int64
	SetActivatedOperatorsCached(operators []common.Address)
	GetActivatedOperatorsUnsafe() []common.Address
	GetActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) ([]common.Address, error)
	UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient)

	// Contract interactions
	CallSmartContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error)
	ExecuteTransaction(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error)

	// Contract state queries
	UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error)
	GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error)
}

// DefaultEthService implements IEthService using the existing package-level functions.
type DefaultEthService struct{}

func NewDefaultEthService() IEthService { return &DefaultEthService{} }

func (s *DefaultEthService) GetActivatedOperatorsCached() []common.Address {
	return GetActivatedOperatorsCached()
}

func (s *DefaultEthService) GetActivatedOperatorsLength() int64 {
	return GetActivatedOperatorsLength()
}

func (s *DefaultEthService) SetActivatedOperatorsCached(operators []common.Address) {
	SetActivatedOperatorsCached(operators)
}

func (s *DefaultEthService) GetActivatedOperatorsUnsafe() []common.Address {
	return GetActivatedOperatorsUnsafe()
}

func (s *DefaultEthService) GetActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
	return GetActivatedOperators(ctx, fallbackEthClient)
}

func (s *DefaultEthService) UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) {
	UpdateActivatedOperators(ctx, fallbackEthClient)
}

func (s *DefaultEthService) CallSmartContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	return CallSmartContract(ctx, fallbackEthClient, parsedABI, method, contractAddress, params...)
}

func (s *DefaultEthService) ExecuteTransaction(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
	return ExecuteTransaction(ctx, clientUtils, fallbackEthClient, method, value, args...)
}

func (s *DefaultEthService) UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	return UpdateCurrentRoundFromContract(ctx, fallbackEthClient)
}

func (s *DefaultEthService) GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	return GetTrialNumFromContract(ctx, fallbackEthClient, round)
}

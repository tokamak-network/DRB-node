// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package DRBCoordinator

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// DRBCoordinatorStorageRequestInfo is an auto generated low-level Go binding around an user-defined struct.
type DRBCoordinatorStorageRequestInfo struct {
	Consumer              common.Address
	RequestedTime         *big.Int
	Cost                  *big.Int
	CallbackGasLimit      *big.Int
	MinDepositForOperator *big.Int
}

// DRBCoordinatorStorageRoundInfo is an auto generated low-level Go binding around an user-defined struct.
type DRBCoordinatorStorageRoundInfo struct {
	CommitEndTime    *big.Int
	RandomNumber     *big.Int
	FulfillSucceeded bool
}

// DRBCoordinatorMetaData contains all meta data concerning the DRBCoordinator contract.
var DRBCoordinatorMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"minDeposit\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"compensations\",\"type\":\"uint256[3]\",\"internalType\":\"uint256[3]\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"activate\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"calculateRequestPrice\",\"inputs\":[{\"name\":\"callbackGasLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"commit\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"a\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deactivate\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deposit\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"depositAndActivate\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"estimateRequestPrice\",\"inputs\":[{\"name\":\"callbackGasLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"gasPrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getActivatedOperatorIndex\",\"inputs\":[{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getActivatedOperators\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getActivatedOperatorsAtRound\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getActivatedOperatorsLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getActivatedOperatorsLengthAtRound\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCommitOrder\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCommits\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCommitsLength\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCompensations\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256[3]\",\"internalType\":\"uint256[3]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDepositAmount\",\"inputs\":[{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDurations\",\"inputs\":[],\"outputs\":[{\"name\":\"maxWait\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"commitDuration\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"revealDuration\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"getMinDeposit\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getRefund\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getRequestInfo\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structDRBCoordinatorStorage.RequestInfo\",\"components\":[{\"name\":\"consumer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"requestedTime\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cost\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"callbackGasLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minDepositForOperator\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getRevealOrder\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getReveals\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getRevealsLength\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getRoundInfo\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structDRBCoordinatorStorage.RoundInfo\",\"components\":[{\"name\":\"commitEndTime\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"randomNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"fulfillSucceeded\",\"type\":\"bool\",\"internalType\":\"bool\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"requestRandomNumber\",\"inputs\":[{\"name\":\"callbackGasLimit\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"outputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"reveal\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"s\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"s_l1FeeCalculationMode\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"uint8\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"s_l1FeeCoefficient\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"uint8\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"setCompensations\",\"inputs\":[{\"name\":\"compensations\",\"type\":\"uint256[3]\",\"internalType\":\"uint256[3]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setFlatFee\",\"inputs\":[{\"name\":\"flatFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setL1FeeCalculation\",\"inputs\":[{\"name\":\"mode\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"coefficient\",\"type\":\"uint8\",\"internalType\":\"uint8\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMinDeposit\",\"inputs\":[{\"name\":\"minDeposit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setPremiumPercentage\",\"inputs\":[{\"name\":\"premiumPercentage\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"withdraw\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"Activated\",\"inputs\":[{\"name\":\"operator\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Commit\",\"inputs\":[{\"name\":\"operator\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"round\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DeActivated\",\"inputs\":[{\"name\":\"operator\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"L1FeeCalculationSet\",\"inputs\":[{\"name\":\"mode\",\"type\":\"uint8\",\"indexed\":false,\"internalType\":\"uint8\"},{\"name\":\"coefficient\",\"type\":\"uint8\",\"indexed\":false,\"internalType\":\"uint8\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RandomNumberRequested\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"activatedOperators\",\"type\":\"address[]\",\"indexed\":false,\"internalType\":\"address[]\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Refund\",\"inputs\":[{\"name\":\"round\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Reveal\",\"inputs\":[{\"name\":\"operator\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"round\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyActivated\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyCommitted\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyDeactivated\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyRevealed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"CommitPhaseOver\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientAmount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientDeposit\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidL1FeeCalculationMode\",\"inputs\":[{\"name\":\"mode\",\"type\":\"uint8\",\"internalType\":\"uint8\"}]},{\"type\":\"error\",\"name\":\"InvalidL1FeeCoefficient\",\"inputs\":[{\"name\":\"coefficient\",\"type\":\"uint8\",\"internalType\":\"uint8\"}]},{\"type\":\"error\",\"name\":\"NotActivatedOperator\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotCommitted\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotConsumer\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotEnoughActivatedOperators\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotRefundable\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotRevealPhase\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotSlashingCondition\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"RevealValueMismatch\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WasNotActivated\",\"inputs\":[]}]",
}

// DRBCoordinatorABI is the input ABI used to generate the binding from.
// Deprecated: Use DRBCoordinatorMetaData.ABI instead.
var DRBCoordinatorABI = DRBCoordinatorMetaData.ABI

// DRBCoordinator is an auto generated Go binding around an Ethereum contract.
type DRBCoordinator struct {
	DRBCoordinatorCaller     // Read-only binding to the contract
	DRBCoordinatorTransactor // Write-only binding to the contract
	DRBCoordinatorFilterer   // Log filterer for contract events
}

// DRBCoordinatorCaller is an auto generated read-only Go binding around an Ethereum contract.
type DRBCoordinatorCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DRBCoordinatorTransactor is an auto generated write-only Go binding around an Ethereum contract.
type DRBCoordinatorTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DRBCoordinatorFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type DRBCoordinatorFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DRBCoordinatorSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type DRBCoordinatorSession struct {
	Contract     *DRBCoordinator   // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// DRBCoordinatorCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type DRBCoordinatorCallerSession struct {
	Contract *DRBCoordinatorCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts         // Call options to use throughout this session
}

// DRBCoordinatorTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type DRBCoordinatorTransactorSession struct {
	Contract     *DRBCoordinatorTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// DRBCoordinatorRaw is an auto generated low-level Go binding around an Ethereum contract.
type DRBCoordinatorRaw struct {
	Contract *DRBCoordinator // Generic contract binding to access the raw methods on
}

// DRBCoordinatorCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type DRBCoordinatorCallerRaw struct {
	Contract *DRBCoordinatorCaller // Generic read-only contract binding to access the raw methods on
}

// DRBCoordinatorTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type DRBCoordinatorTransactorRaw struct {
	Contract *DRBCoordinatorTransactor // Generic write-only contract binding to access the raw methods on
}

// NewDRBCoordinator creates a new instance of DRBCoordinator, bound to a specific deployed contract.
func NewDRBCoordinator(address common.Address, backend bind.ContractBackend) (*DRBCoordinator, error) {
	contract, err := bindDRBCoordinator(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &DRBCoordinator{DRBCoordinatorCaller: DRBCoordinatorCaller{contract: contract}, DRBCoordinatorTransactor: DRBCoordinatorTransactor{contract: contract}, DRBCoordinatorFilterer: DRBCoordinatorFilterer{contract: contract}}, nil
}

// NewDRBCoordinatorCaller creates a new read-only instance of DRBCoordinator, bound to a specific deployed contract.
func NewDRBCoordinatorCaller(address common.Address, caller bind.ContractCaller) (*DRBCoordinatorCaller, error) {
	contract, err := bindDRBCoordinator(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorCaller{contract: contract}, nil
}

// NewDRBCoordinatorTransactor creates a new write-only instance of DRBCoordinator, bound to a specific deployed contract.
func NewDRBCoordinatorTransactor(address common.Address, transactor bind.ContractTransactor) (*DRBCoordinatorTransactor, error) {
	contract, err := bindDRBCoordinator(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorTransactor{contract: contract}, nil
}

// NewDRBCoordinatorFilterer creates a new log filterer instance of DRBCoordinator, bound to a specific deployed contract.
func NewDRBCoordinatorFilterer(address common.Address, filterer bind.ContractFilterer) (*DRBCoordinatorFilterer, error) {
	contract, err := bindDRBCoordinator(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorFilterer{contract: contract}, nil
}

// bindDRBCoordinator binds a generic wrapper to an already deployed contract.
func bindDRBCoordinator(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := DRBCoordinatorMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DRBCoordinator *DRBCoordinatorRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DRBCoordinator.Contract.DRBCoordinatorCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DRBCoordinator *DRBCoordinatorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.DRBCoordinatorTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DRBCoordinator *DRBCoordinatorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.DRBCoordinatorTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DRBCoordinator *DRBCoordinatorCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DRBCoordinator.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DRBCoordinator *DRBCoordinatorTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DRBCoordinator *DRBCoordinatorTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.contract.Transact(opts, method, params...)
}

// CalculateRequestPrice is a free data retrieval call binding the contract method 0x640d3892.
//
// Solidity: function calculateRequestPrice(uint256 callbackGasLimit) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) CalculateRequestPrice(opts *bind.CallOpts, callbackGasLimit *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "calculateRequestPrice", callbackGasLimit)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// CalculateRequestPrice is a free data retrieval call binding the contract method 0x640d3892.
//
// Solidity: function calculateRequestPrice(uint256 callbackGasLimit) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) CalculateRequestPrice(callbackGasLimit *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.CalculateRequestPrice(&_DRBCoordinator.CallOpts, callbackGasLimit)
}

// CalculateRequestPrice is a free data retrieval call binding the contract method 0x640d3892.
//
// Solidity: function calculateRequestPrice(uint256 callbackGasLimit) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) CalculateRequestPrice(callbackGasLimit *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.CalculateRequestPrice(&_DRBCoordinator.CallOpts, callbackGasLimit)
}

// EstimateRequestPrice is a free data retrieval call binding the contract method 0xa9f664be.
//
// Solidity: function estimateRequestPrice(uint256 callbackGasLimit, uint256 gasPrice) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) EstimateRequestPrice(opts *bind.CallOpts, callbackGasLimit *big.Int, gasPrice *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "estimateRequestPrice", callbackGasLimit, gasPrice)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// EstimateRequestPrice is a free data retrieval call binding the contract method 0xa9f664be.
//
// Solidity: function estimateRequestPrice(uint256 callbackGasLimit, uint256 gasPrice) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) EstimateRequestPrice(callbackGasLimit *big.Int, gasPrice *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.EstimateRequestPrice(&_DRBCoordinator.CallOpts, callbackGasLimit, gasPrice)
}

// EstimateRequestPrice is a free data retrieval call binding the contract method 0xa9f664be.
//
// Solidity: function estimateRequestPrice(uint256 callbackGasLimit, uint256 gasPrice) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) EstimateRequestPrice(callbackGasLimit *big.Int, gasPrice *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.EstimateRequestPrice(&_DRBCoordinator.CallOpts, callbackGasLimit, gasPrice)
}

// GetActivatedOperatorIndex is a free data retrieval call binding the contract method 0x7e156fe4.
//
// Solidity: function getActivatedOperatorIndex(address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) GetActivatedOperatorIndex(opts *bind.CallOpts, operator common.Address) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getActivatedOperatorIndex", operator)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetActivatedOperatorIndex is a free data retrieval call binding the contract method 0x7e156fe4.
//
// Solidity: function getActivatedOperatorIndex(address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) GetActivatedOperatorIndex(operator common.Address) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetActivatedOperatorIndex(&_DRBCoordinator.CallOpts, operator)
}

// GetActivatedOperatorIndex is a free data retrieval call binding the contract method 0x7e156fe4.
//
// Solidity: function getActivatedOperatorIndex(address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetActivatedOperatorIndex(operator common.Address) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetActivatedOperatorIndex(&_DRBCoordinator.CallOpts, operator)
}

// GetActivatedOperators is a free data retrieval call binding the contract method 0xecd21a7e.
//
// Solidity: function getActivatedOperators() view returns(address[])
func (_DRBCoordinator *DRBCoordinatorCaller) GetActivatedOperators(opts *bind.CallOpts) ([]common.Address, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getActivatedOperators")

	if err != nil {
		return *new([]common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)

	return out0, err

}

// GetActivatedOperators is a free data retrieval call binding the contract method 0xecd21a7e.
//
// Solidity: function getActivatedOperators() view returns(address[])
func (_DRBCoordinator *DRBCoordinatorSession) GetActivatedOperators() ([]common.Address, error) {
	return _DRBCoordinator.Contract.GetActivatedOperators(&_DRBCoordinator.CallOpts)
}

// GetActivatedOperators is a free data retrieval call binding the contract method 0xecd21a7e.
//
// Solidity: function getActivatedOperators() view returns(address[])
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetActivatedOperators() ([]common.Address, error) {
	return _DRBCoordinator.Contract.GetActivatedOperators(&_DRBCoordinator.CallOpts)
}

// GetActivatedOperatorsAtRound is a free data retrieval call binding the contract method 0x1b6a72db.
//
// Solidity: function getActivatedOperatorsAtRound(uint256 round) view returns(address[])
func (_DRBCoordinator *DRBCoordinatorCaller) GetActivatedOperatorsAtRound(opts *bind.CallOpts, round *big.Int) ([]common.Address, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getActivatedOperatorsAtRound", round)

	if err != nil {
		return *new([]common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)

	return out0, err

}

// GetActivatedOperatorsAtRound is a free data retrieval call binding the contract method 0x1b6a72db.
//
// Solidity: function getActivatedOperatorsAtRound(uint256 round) view returns(address[])
func (_DRBCoordinator *DRBCoordinatorSession) GetActivatedOperatorsAtRound(round *big.Int) ([]common.Address, error) {
	return _DRBCoordinator.Contract.GetActivatedOperatorsAtRound(&_DRBCoordinator.CallOpts, round)
}

// GetActivatedOperatorsAtRound is a free data retrieval call binding the contract method 0x1b6a72db.
//
// Solidity: function getActivatedOperatorsAtRound(uint256 round) view returns(address[])
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetActivatedOperatorsAtRound(round *big.Int) ([]common.Address, error) {
	return _DRBCoordinator.Contract.GetActivatedOperatorsAtRound(&_DRBCoordinator.CallOpts, round)
}

// GetActivatedOperatorsLength is a free data retrieval call binding the contract method 0x36088f52.
//
// Solidity: function getActivatedOperatorsLength() view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) GetActivatedOperatorsLength(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getActivatedOperatorsLength")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetActivatedOperatorsLength is a free data retrieval call binding the contract method 0x36088f52.
//
// Solidity: function getActivatedOperatorsLength() view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) GetActivatedOperatorsLength() (*big.Int, error) {
	return _DRBCoordinator.Contract.GetActivatedOperatorsLength(&_DRBCoordinator.CallOpts)
}

// GetActivatedOperatorsLength is a free data retrieval call binding the contract method 0x36088f52.
//
// Solidity: function getActivatedOperatorsLength() view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetActivatedOperatorsLength() (*big.Int, error) {
	return _DRBCoordinator.Contract.GetActivatedOperatorsLength(&_DRBCoordinator.CallOpts)
}

// GetActivatedOperatorsLengthAtRound is a free data retrieval call binding the contract method 0x9eba1076.
//
// Solidity: function getActivatedOperatorsLengthAtRound(uint256 round) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) GetActivatedOperatorsLengthAtRound(opts *bind.CallOpts, round *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getActivatedOperatorsLengthAtRound", round)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetActivatedOperatorsLengthAtRound is a free data retrieval call binding the contract method 0x9eba1076.
//
// Solidity: function getActivatedOperatorsLengthAtRound(uint256 round) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) GetActivatedOperatorsLengthAtRound(round *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetActivatedOperatorsLengthAtRound(&_DRBCoordinator.CallOpts, round)
}

// GetActivatedOperatorsLengthAtRound is a free data retrieval call binding the contract method 0x9eba1076.
//
// Solidity: function getActivatedOperatorsLengthAtRound(uint256 round) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetActivatedOperatorsLengthAtRound(round *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetActivatedOperatorsLengthAtRound(&_DRBCoordinator.CallOpts, round)
}

// GetCommitOrder is a free data retrieval call binding the contract method 0x9eeac9e8.
//
// Solidity: function getCommitOrder(uint256 round, address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) GetCommitOrder(opts *bind.CallOpts, round *big.Int, operator common.Address) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getCommitOrder", round, operator)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCommitOrder is a free data retrieval call binding the contract method 0x9eeac9e8.
//
// Solidity: function getCommitOrder(uint256 round, address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) GetCommitOrder(round *big.Int, operator common.Address) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetCommitOrder(&_DRBCoordinator.CallOpts, round, operator)
}

// GetCommitOrder is a free data retrieval call binding the contract method 0x9eeac9e8.
//
// Solidity: function getCommitOrder(uint256 round, address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetCommitOrder(round *big.Int, operator common.Address) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetCommitOrder(&_DRBCoordinator.CallOpts, round, operator)
}

// GetCommits is a free data retrieval call binding the contract method 0xa9b6dc22.
//
// Solidity: function getCommits(uint256 round) view returns(bytes32[])
func (_DRBCoordinator *DRBCoordinatorCaller) GetCommits(opts *bind.CallOpts, round *big.Int) ([][32]byte, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getCommits", round)

	if err != nil {
		return *new([][32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([][32]byte)).(*[][32]byte)

	return out0, err

}

// GetCommits is a free data retrieval call binding the contract method 0xa9b6dc22.
//
// Solidity: function getCommits(uint256 round) view returns(bytes32[])
func (_DRBCoordinator *DRBCoordinatorSession) GetCommits(round *big.Int) ([][32]byte, error) {
	return _DRBCoordinator.Contract.GetCommits(&_DRBCoordinator.CallOpts, round)
}

// GetCommits is a free data retrieval call binding the contract method 0xa9b6dc22.
//
// Solidity: function getCommits(uint256 round) view returns(bytes32[])
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetCommits(round *big.Int) ([][32]byte, error) {
	return _DRBCoordinator.Contract.GetCommits(&_DRBCoordinator.CallOpts, round)
}

// GetCommitsLength is a free data retrieval call binding the contract method 0xb77308dd.
//
// Solidity: function getCommitsLength(uint256 round) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) GetCommitsLength(opts *bind.CallOpts, round *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getCommitsLength", round)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCommitsLength is a free data retrieval call binding the contract method 0xb77308dd.
//
// Solidity: function getCommitsLength(uint256 round) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) GetCommitsLength(round *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetCommitsLength(&_DRBCoordinator.CallOpts, round)
}

// GetCommitsLength is a free data retrieval call binding the contract method 0xb77308dd.
//
// Solidity: function getCommitsLength(uint256 round) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetCommitsLength(round *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetCommitsLength(&_DRBCoordinator.CallOpts, round)
}

// GetCompensations is a free data retrieval call binding the contract method 0x7b58b4e7.
//
// Solidity: function getCompensations() view returns(uint256[3])
func (_DRBCoordinator *DRBCoordinatorCaller) GetCompensations(opts *bind.CallOpts) ([3]*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getCompensations")

	if err != nil {
		return *new([3]*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new([3]*big.Int)).(*[3]*big.Int)

	return out0, err

}

// GetCompensations is a free data retrieval call binding the contract method 0x7b58b4e7.
//
// Solidity: function getCompensations() view returns(uint256[3])
func (_DRBCoordinator *DRBCoordinatorSession) GetCompensations() ([3]*big.Int, error) {
	return _DRBCoordinator.Contract.GetCompensations(&_DRBCoordinator.CallOpts)
}

// GetCompensations is a free data retrieval call binding the contract method 0x7b58b4e7.
//
// Solidity: function getCompensations() view returns(uint256[3])
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetCompensations() ([3]*big.Int, error) {
	return _DRBCoordinator.Contract.GetCompensations(&_DRBCoordinator.CallOpts)
}

// GetDepositAmount is a free data retrieval call binding the contract method 0xb8ba16fd.
//
// Solidity: function getDepositAmount(address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) GetDepositAmount(opts *bind.CallOpts, operator common.Address) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getDepositAmount", operator)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetDepositAmount is a free data retrieval call binding the contract method 0xb8ba16fd.
//
// Solidity: function getDepositAmount(address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) GetDepositAmount(operator common.Address) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetDepositAmount(&_DRBCoordinator.CallOpts, operator)
}

// GetDepositAmount is a free data retrieval call binding the contract method 0xb8ba16fd.
//
// Solidity: function getDepositAmount(address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetDepositAmount(operator common.Address) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetDepositAmount(&_DRBCoordinator.CallOpts, operator)
}

// GetDurations is a free data retrieval call binding the contract method 0xc4d5b37d.
//
// Solidity: function getDurations() pure returns(uint256 maxWait, uint256 commitDuration, uint256 revealDuration)
func (_DRBCoordinator *DRBCoordinatorCaller) GetDurations(opts *bind.CallOpts) (struct {
	MaxWait        *big.Int
	CommitDuration *big.Int
	RevealDuration *big.Int
}, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getDurations")

	outstruct := new(struct {
		MaxWait        *big.Int
		CommitDuration *big.Int
		RevealDuration *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.MaxWait = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.CommitDuration = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	outstruct.RevealDuration = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// GetDurations is a free data retrieval call binding the contract method 0xc4d5b37d.
//
// Solidity: function getDurations() pure returns(uint256 maxWait, uint256 commitDuration, uint256 revealDuration)
func (_DRBCoordinator *DRBCoordinatorSession) GetDurations() (struct {
	MaxWait        *big.Int
	CommitDuration *big.Int
	RevealDuration *big.Int
}, error) {
	return _DRBCoordinator.Contract.GetDurations(&_DRBCoordinator.CallOpts)
}

// GetDurations is a free data retrieval call binding the contract method 0xc4d5b37d.
//
// Solidity: function getDurations() pure returns(uint256 maxWait, uint256 commitDuration, uint256 revealDuration)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetDurations() (struct {
	MaxWait        *big.Int
	CommitDuration *big.Int
	RevealDuration *big.Int
}, error) {
	return _DRBCoordinator.Contract.GetDurations(&_DRBCoordinator.CallOpts)
}

// GetMinDeposit is a free data retrieval call binding the contract method 0x0eaad3f1.
//
// Solidity: function getMinDeposit() view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) GetMinDeposit(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getMinDeposit")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMinDeposit is a free data retrieval call binding the contract method 0x0eaad3f1.
//
// Solidity: function getMinDeposit() view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) GetMinDeposit() (*big.Int, error) {
	return _DRBCoordinator.Contract.GetMinDeposit(&_DRBCoordinator.CallOpts)
}

// GetMinDeposit is a free data retrieval call binding the contract method 0x0eaad3f1.
//
// Solidity: function getMinDeposit() view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetMinDeposit() (*big.Int, error) {
	return _DRBCoordinator.Contract.GetMinDeposit(&_DRBCoordinator.CallOpts)
}

// GetRequestInfo is a free data retrieval call binding the contract method 0x0b816045.
//
// Solidity: function getRequestInfo(uint256 round) view returns((address,uint256,uint256,uint256,uint256))
func (_DRBCoordinator *DRBCoordinatorCaller) GetRequestInfo(opts *bind.CallOpts, round *big.Int) (DRBCoordinatorStorageRequestInfo, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getRequestInfo", round)

	if err != nil {
		return *new(DRBCoordinatorStorageRequestInfo), err
	}

	out0 := *abi.ConvertType(out[0], new(DRBCoordinatorStorageRequestInfo)).(*DRBCoordinatorStorageRequestInfo)

	return out0, err

}

// GetRequestInfo is a free data retrieval call binding the contract method 0x0b816045.
//
// Solidity: function getRequestInfo(uint256 round) view returns((address,uint256,uint256,uint256,uint256))
func (_DRBCoordinator *DRBCoordinatorSession) GetRequestInfo(round *big.Int) (DRBCoordinatorStorageRequestInfo, error) {
	return _DRBCoordinator.Contract.GetRequestInfo(&_DRBCoordinator.CallOpts, round)
}

// GetRequestInfo is a free data retrieval call binding the contract method 0x0b816045.
//
// Solidity: function getRequestInfo(uint256 round) view returns((address,uint256,uint256,uint256,uint256))
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetRequestInfo(round *big.Int) (DRBCoordinatorStorageRequestInfo, error) {
	return _DRBCoordinator.Contract.GetRequestInfo(&_DRBCoordinator.CallOpts, round)
}

// GetRevealOrder is a free data retrieval call binding the contract method 0xb3fcaf64.
//
// Solidity: function getRevealOrder(uint256 round, address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) GetRevealOrder(opts *bind.CallOpts, round *big.Int, operator common.Address) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getRevealOrder", round, operator)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetRevealOrder is a free data retrieval call binding the contract method 0xb3fcaf64.
//
// Solidity: function getRevealOrder(uint256 round, address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) GetRevealOrder(round *big.Int, operator common.Address) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetRevealOrder(&_DRBCoordinator.CallOpts, round, operator)
}

// GetRevealOrder is a free data retrieval call binding the contract method 0xb3fcaf64.
//
// Solidity: function getRevealOrder(uint256 round, address operator) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetRevealOrder(round *big.Int, operator common.Address) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetRevealOrder(&_DRBCoordinator.CallOpts, round, operator)
}

// GetReveals is a free data retrieval call binding the contract method 0x97d9ee52.
//
// Solidity: function getReveals(uint256 round) view returns(bytes32[])
func (_DRBCoordinator *DRBCoordinatorCaller) GetReveals(opts *bind.CallOpts, round *big.Int) ([][32]byte, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getReveals", round)

	if err != nil {
		return *new([][32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([][32]byte)).(*[][32]byte)

	return out0, err

}

// GetReveals is a free data retrieval call binding the contract method 0x97d9ee52.
//
// Solidity: function getReveals(uint256 round) view returns(bytes32[])
func (_DRBCoordinator *DRBCoordinatorSession) GetReveals(round *big.Int) ([][32]byte, error) {
	return _DRBCoordinator.Contract.GetReveals(&_DRBCoordinator.CallOpts, round)
}

// GetReveals is a free data retrieval call binding the contract method 0x97d9ee52.
//
// Solidity: function getReveals(uint256 round) view returns(bytes32[])
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetReveals(round *big.Int) ([][32]byte, error) {
	return _DRBCoordinator.Contract.GetReveals(&_DRBCoordinator.CallOpts, round)
}

// GetRevealsLength is a free data retrieval call binding the contract method 0x35f67468.
//
// Solidity: function getRevealsLength(uint256 round) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCaller) GetRevealsLength(opts *bind.CallOpts, round *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getRevealsLength", round)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetRevealsLength is a free data retrieval call binding the contract method 0x35f67468.
//
// Solidity: function getRevealsLength(uint256 round) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorSession) GetRevealsLength(round *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetRevealsLength(&_DRBCoordinator.CallOpts, round)
}

// GetRevealsLength is a free data retrieval call binding the contract method 0x35f67468.
//
// Solidity: function getRevealsLength(uint256 round) view returns(uint256)
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetRevealsLength(round *big.Int) (*big.Int, error) {
	return _DRBCoordinator.Contract.GetRevealsLength(&_DRBCoordinator.CallOpts, round)
}

// GetRoundInfo is a free data retrieval call binding the contract method 0x88c3ffb0.
//
// Solidity: function getRoundInfo(uint256 round) view returns((uint256,uint256,bool))
func (_DRBCoordinator *DRBCoordinatorCaller) GetRoundInfo(opts *bind.CallOpts, round *big.Int) (DRBCoordinatorStorageRoundInfo, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "getRoundInfo", round)

	if err != nil {
		return *new(DRBCoordinatorStorageRoundInfo), err
	}

	out0 := *abi.ConvertType(out[0], new(DRBCoordinatorStorageRoundInfo)).(*DRBCoordinatorStorageRoundInfo)

	return out0, err

}

// GetRoundInfo is a free data retrieval call binding the contract method 0x88c3ffb0.
//
// Solidity: function getRoundInfo(uint256 round) view returns((uint256,uint256,bool))
func (_DRBCoordinator *DRBCoordinatorSession) GetRoundInfo(round *big.Int) (DRBCoordinatorStorageRoundInfo, error) {
	return _DRBCoordinator.Contract.GetRoundInfo(&_DRBCoordinator.CallOpts, round)
}

// GetRoundInfo is a free data retrieval call binding the contract method 0x88c3ffb0.
//
// Solidity: function getRoundInfo(uint256 round) view returns((uint256,uint256,bool))
func (_DRBCoordinator *DRBCoordinatorCallerSession) GetRoundInfo(round *big.Int) (DRBCoordinatorStorageRoundInfo, error) {
	return _DRBCoordinator.Contract.GetRoundInfo(&_DRBCoordinator.CallOpts, round)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_DRBCoordinator *DRBCoordinatorCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_DRBCoordinator *DRBCoordinatorSession) Owner() (common.Address, error) {
	return _DRBCoordinator.Contract.Owner(&_DRBCoordinator.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_DRBCoordinator *DRBCoordinatorCallerSession) Owner() (common.Address, error) {
	return _DRBCoordinator.Contract.Owner(&_DRBCoordinator.CallOpts)
}

// SL1FeeCalculationMode is a free data retrieval call binding the contract method 0x40e3290f.
//
// Solidity: function s_l1FeeCalculationMode() view returns(uint8)
func (_DRBCoordinator *DRBCoordinatorCaller) SL1FeeCalculationMode(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "s_l1FeeCalculationMode")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// SL1FeeCalculationMode is a free data retrieval call binding the contract method 0x40e3290f.
//
// Solidity: function s_l1FeeCalculationMode() view returns(uint8)
func (_DRBCoordinator *DRBCoordinatorSession) SL1FeeCalculationMode() (uint8, error) {
	return _DRBCoordinator.Contract.SL1FeeCalculationMode(&_DRBCoordinator.CallOpts)
}

// SL1FeeCalculationMode is a free data retrieval call binding the contract method 0x40e3290f.
//
// Solidity: function s_l1FeeCalculationMode() view returns(uint8)
func (_DRBCoordinator *DRBCoordinatorCallerSession) SL1FeeCalculationMode() (uint8, error) {
	return _DRBCoordinator.Contract.SL1FeeCalculationMode(&_DRBCoordinator.CallOpts)
}

// SL1FeeCoefficient is a free data retrieval call binding the contract method 0x90bd5c74.
//
// Solidity: function s_l1FeeCoefficient() view returns(uint8)
func (_DRBCoordinator *DRBCoordinatorCaller) SL1FeeCoefficient(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _DRBCoordinator.contract.Call(opts, &out, "s_l1FeeCoefficient")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// SL1FeeCoefficient is a free data retrieval call binding the contract method 0x90bd5c74.
//
// Solidity: function s_l1FeeCoefficient() view returns(uint8)
func (_DRBCoordinator *DRBCoordinatorSession) SL1FeeCoefficient() (uint8, error) {
	return _DRBCoordinator.Contract.SL1FeeCoefficient(&_DRBCoordinator.CallOpts)
}

// SL1FeeCoefficient is a free data retrieval call binding the contract method 0x90bd5c74.
//
// Solidity: function s_l1FeeCoefficient() view returns(uint8)
func (_DRBCoordinator *DRBCoordinatorCallerSession) SL1FeeCoefficient() (uint8, error) {
	return _DRBCoordinator.Contract.SL1FeeCoefficient(&_DRBCoordinator.CallOpts)
}

// Activate is a paid mutator transaction binding the contract method 0x0f15f4c0.
//
// Solidity: function activate() returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) Activate(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "activate")
}

// Activate is a paid mutator transaction binding the contract method 0x0f15f4c0.
//
// Solidity: function activate() returns()
func (_DRBCoordinator *DRBCoordinatorSession) Activate() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Activate(&_DRBCoordinator.TransactOpts)
}

// Activate is a paid mutator transaction binding the contract method 0x0f15f4c0.
//
// Solidity: function activate() returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) Activate() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Activate(&_DRBCoordinator.TransactOpts)
}

// Commit is a paid mutator transaction binding the contract method 0xf2f03877.
//
// Solidity: function commit(uint256 round, bytes32 a) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) Commit(opts *bind.TransactOpts, round *big.Int, a [32]byte) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "commit", round, a)
}

// Commit is a paid mutator transaction binding the contract method 0xf2f03877.
//
// Solidity: function commit(uint256 round, bytes32 a) returns()
func (_DRBCoordinator *DRBCoordinatorSession) Commit(round *big.Int, a [32]byte) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Commit(&_DRBCoordinator.TransactOpts, round, a)
}

// Commit is a paid mutator transaction binding the contract method 0xf2f03877.
//
// Solidity: function commit(uint256 round, bytes32 a) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) Commit(round *big.Int, a [32]byte) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Commit(&_DRBCoordinator.TransactOpts, round, a)
}

// Deactivate is a paid mutator transaction binding the contract method 0x51b42b00.
//
// Solidity: function deactivate() returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) Deactivate(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "deactivate")
}

// Deactivate is a paid mutator transaction binding the contract method 0x51b42b00.
//
// Solidity: function deactivate() returns()
func (_DRBCoordinator *DRBCoordinatorSession) Deactivate() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Deactivate(&_DRBCoordinator.TransactOpts)
}

// Deactivate is a paid mutator transaction binding the contract method 0x51b42b00.
//
// Solidity: function deactivate() returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) Deactivate() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Deactivate(&_DRBCoordinator.TransactOpts)
}

// Deposit is a paid mutator transaction binding the contract method 0xd0e30db0.
//
// Solidity: function deposit() payable returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) Deposit(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "deposit")
}

// Deposit is a paid mutator transaction binding the contract method 0xd0e30db0.
//
// Solidity: function deposit() payable returns()
func (_DRBCoordinator *DRBCoordinatorSession) Deposit() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Deposit(&_DRBCoordinator.TransactOpts)
}

// Deposit is a paid mutator transaction binding the contract method 0xd0e30db0.
//
// Solidity: function deposit() payable returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) Deposit() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Deposit(&_DRBCoordinator.TransactOpts)
}

// DepositAndActivate is a paid mutator transaction binding the contract method 0x77343032.
//
// Solidity: function depositAndActivate() payable returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) DepositAndActivate(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "depositAndActivate")
}

// DepositAndActivate is a paid mutator transaction binding the contract method 0x77343032.
//
// Solidity: function depositAndActivate() payable returns()
func (_DRBCoordinator *DRBCoordinatorSession) DepositAndActivate() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.DepositAndActivate(&_DRBCoordinator.TransactOpts)
}

// DepositAndActivate is a paid mutator transaction binding the contract method 0x77343032.
//
// Solidity: function depositAndActivate() payable returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) DepositAndActivate() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.DepositAndActivate(&_DRBCoordinator.TransactOpts)
}

// GetRefund is a paid mutator transaction binding the contract method 0xd2f0be99.
//
// Solidity: function getRefund(uint256 round) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) GetRefund(opts *bind.TransactOpts, round *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "getRefund", round)
}

// GetRefund is a paid mutator transaction binding the contract method 0xd2f0be99.
//
// Solidity: function getRefund(uint256 round) returns()
func (_DRBCoordinator *DRBCoordinatorSession) GetRefund(round *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.GetRefund(&_DRBCoordinator.TransactOpts, round)
}

// GetRefund is a paid mutator transaction binding the contract method 0xd2f0be99.
//
// Solidity: function getRefund(uint256 round) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) GetRefund(round *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.GetRefund(&_DRBCoordinator.TransactOpts, round)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_DRBCoordinator *DRBCoordinatorSession) RenounceOwnership() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.RenounceOwnership(&_DRBCoordinator.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _DRBCoordinator.Contract.RenounceOwnership(&_DRBCoordinator.TransactOpts)
}

// RequestRandomNumber is a paid mutator transaction binding the contract method 0xb5f3abb0.
//
// Solidity: function requestRandomNumber(uint32 callbackGasLimit) payable returns(uint256 round)
func (_DRBCoordinator *DRBCoordinatorTransactor) RequestRandomNumber(opts *bind.TransactOpts, callbackGasLimit uint32) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "requestRandomNumber", callbackGasLimit)
}

// RequestRandomNumber is a paid mutator transaction binding the contract method 0xb5f3abb0.
//
// Solidity: function requestRandomNumber(uint32 callbackGasLimit) payable returns(uint256 round)
func (_DRBCoordinator *DRBCoordinatorSession) RequestRandomNumber(callbackGasLimit uint32) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.RequestRandomNumber(&_DRBCoordinator.TransactOpts, callbackGasLimit)
}

// RequestRandomNumber is a paid mutator transaction binding the contract method 0xb5f3abb0.
//
// Solidity: function requestRandomNumber(uint32 callbackGasLimit) payable returns(uint256 round)
func (_DRBCoordinator *DRBCoordinatorTransactorSession) RequestRandomNumber(callbackGasLimit uint32) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.RequestRandomNumber(&_DRBCoordinator.TransactOpts, callbackGasLimit)
}

// Reveal is a paid mutator transaction binding the contract method 0x4036778f.
//
// Solidity: function reveal(uint256 round, bytes32 s) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) Reveal(opts *bind.TransactOpts, round *big.Int, s [32]byte) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "reveal", round, s)
}

// Reveal is a paid mutator transaction binding the contract method 0x4036778f.
//
// Solidity: function reveal(uint256 round, bytes32 s) returns()
func (_DRBCoordinator *DRBCoordinatorSession) Reveal(round *big.Int, s [32]byte) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Reveal(&_DRBCoordinator.TransactOpts, round, s)
}

// Reveal is a paid mutator transaction binding the contract method 0x4036778f.
//
// Solidity: function reveal(uint256 round, bytes32 s) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) Reveal(round *big.Int, s [32]byte) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Reveal(&_DRBCoordinator.TransactOpts, round, s)
}

// SetCompensations is a paid mutator transaction binding the contract method 0x7943188b.
//
// Solidity: function setCompensations(uint256[3] compensations) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) SetCompensations(opts *bind.TransactOpts, compensations [3]*big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "setCompensations", compensations)
}

// SetCompensations is a paid mutator transaction binding the contract method 0x7943188b.
//
// Solidity: function setCompensations(uint256[3] compensations) returns()
func (_DRBCoordinator *DRBCoordinatorSession) SetCompensations(compensations [3]*big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetCompensations(&_DRBCoordinator.TransactOpts, compensations)
}

// SetCompensations is a paid mutator transaction binding the contract method 0x7943188b.
//
// Solidity: function setCompensations(uint256[3] compensations) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) SetCompensations(compensations [3]*big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetCompensations(&_DRBCoordinator.TransactOpts, compensations)
}

// SetFlatFee is a paid mutator transaction binding the contract method 0x23fa495a.
//
// Solidity: function setFlatFee(uint256 flatFee) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) SetFlatFee(opts *bind.TransactOpts, flatFee *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "setFlatFee", flatFee)
}

// SetFlatFee is a paid mutator transaction binding the contract method 0x23fa495a.
//
// Solidity: function setFlatFee(uint256 flatFee) returns()
func (_DRBCoordinator *DRBCoordinatorSession) SetFlatFee(flatFee *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetFlatFee(&_DRBCoordinator.TransactOpts, flatFee)
}

// SetFlatFee is a paid mutator transaction binding the contract method 0x23fa495a.
//
// Solidity: function setFlatFee(uint256 flatFee) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) SetFlatFee(flatFee *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetFlatFee(&_DRBCoordinator.TransactOpts, flatFee)
}

// SetL1FeeCalculation is a paid mutator transaction binding the contract method 0x14530741.
//
// Solidity: function setL1FeeCalculation(uint8 mode, uint8 coefficient) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) SetL1FeeCalculation(opts *bind.TransactOpts, mode uint8, coefficient uint8) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "setL1FeeCalculation", mode, coefficient)
}

// SetL1FeeCalculation is a paid mutator transaction binding the contract method 0x14530741.
//
// Solidity: function setL1FeeCalculation(uint8 mode, uint8 coefficient) returns()
func (_DRBCoordinator *DRBCoordinatorSession) SetL1FeeCalculation(mode uint8, coefficient uint8) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetL1FeeCalculation(&_DRBCoordinator.TransactOpts, mode, coefficient)
}

// SetL1FeeCalculation is a paid mutator transaction binding the contract method 0x14530741.
//
// Solidity: function setL1FeeCalculation(uint8 mode, uint8 coefficient) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) SetL1FeeCalculation(mode uint8, coefficient uint8) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetL1FeeCalculation(&_DRBCoordinator.TransactOpts, mode, coefficient)
}

// SetMinDeposit is a paid mutator transaction binding the contract method 0x8fcc9cfb.
//
// Solidity: function setMinDeposit(uint256 minDeposit) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) SetMinDeposit(opts *bind.TransactOpts, minDeposit *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "setMinDeposit", minDeposit)
}

// SetMinDeposit is a paid mutator transaction binding the contract method 0x8fcc9cfb.
//
// Solidity: function setMinDeposit(uint256 minDeposit) returns()
func (_DRBCoordinator *DRBCoordinatorSession) SetMinDeposit(minDeposit *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetMinDeposit(&_DRBCoordinator.TransactOpts, minDeposit)
}

// SetMinDeposit is a paid mutator transaction binding the contract method 0x8fcc9cfb.
//
// Solidity: function setMinDeposit(uint256 minDeposit) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) SetMinDeposit(minDeposit *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetMinDeposit(&_DRBCoordinator.TransactOpts, minDeposit)
}

// SetPremiumPercentage is a paid mutator transaction binding the contract method 0xe76dd11e.
//
// Solidity: function setPremiumPercentage(uint256 premiumPercentage) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) SetPremiumPercentage(opts *bind.TransactOpts, premiumPercentage *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "setPremiumPercentage", premiumPercentage)
}

// SetPremiumPercentage is a paid mutator transaction binding the contract method 0xe76dd11e.
//
// Solidity: function setPremiumPercentage(uint256 premiumPercentage) returns()
func (_DRBCoordinator *DRBCoordinatorSession) SetPremiumPercentage(premiumPercentage *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetPremiumPercentage(&_DRBCoordinator.TransactOpts, premiumPercentage)
}

// SetPremiumPercentage is a paid mutator transaction binding the contract method 0xe76dd11e.
//
// Solidity: function setPremiumPercentage(uint256 premiumPercentage) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) SetPremiumPercentage(premiumPercentage *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.SetPremiumPercentage(&_DRBCoordinator.TransactOpts, premiumPercentage)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_DRBCoordinator *DRBCoordinatorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.TransferOwnership(&_DRBCoordinator.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.TransferOwnership(&_DRBCoordinator.TransactOpts, newOwner)
}

// Withdraw is a paid mutator transaction binding the contract method 0x2e1a7d4d.
//
// Solidity: function withdraw(uint256 amount) returns()
func (_DRBCoordinator *DRBCoordinatorTransactor) Withdraw(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.contract.Transact(opts, "withdraw", amount)
}

// Withdraw is a paid mutator transaction binding the contract method 0x2e1a7d4d.
//
// Solidity: function withdraw(uint256 amount) returns()
func (_DRBCoordinator *DRBCoordinatorSession) Withdraw(amount *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Withdraw(&_DRBCoordinator.TransactOpts, amount)
}

// Withdraw is a paid mutator transaction binding the contract method 0x2e1a7d4d.
//
// Solidity: function withdraw(uint256 amount) returns()
func (_DRBCoordinator *DRBCoordinatorTransactorSession) Withdraw(amount *big.Int) (*types.Transaction, error) {
	return _DRBCoordinator.Contract.Withdraw(&_DRBCoordinator.TransactOpts, amount)
}

// DRBCoordinatorActivatedIterator is returned from FilterActivated and is used to iterate over the raw logs and unpacked data for Activated events raised by the DRBCoordinator contract.
type DRBCoordinatorActivatedIterator struct {
	Event *DRBCoordinatorActivated // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *DRBCoordinatorActivatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DRBCoordinatorActivated)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(DRBCoordinatorActivated)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *DRBCoordinatorActivatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DRBCoordinatorActivatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DRBCoordinatorActivated represents a Activated event raised by the DRBCoordinator contract.
type DRBCoordinatorActivated struct {
	Operator common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterActivated is a free log retrieval operation binding the contract event 0x0cc43938d137e7efade6a531f663e78c1fc75257b0d65ffda2fdaf70cb49cdf9.
//
// Solidity: event Activated(address operator)
func (_DRBCoordinator *DRBCoordinatorFilterer) FilterActivated(opts *bind.FilterOpts) (*DRBCoordinatorActivatedIterator, error) {

	logs, sub, err := _DRBCoordinator.contract.FilterLogs(opts, "Activated")
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorActivatedIterator{contract: _DRBCoordinator.contract, event: "Activated", logs: logs, sub: sub}, nil
}

// WatchActivated is a free log subscription operation binding the contract event 0x0cc43938d137e7efade6a531f663e78c1fc75257b0d65ffda2fdaf70cb49cdf9.
//
// Solidity: event Activated(address operator)
func (_DRBCoordinator *DRBCoordinatorFilterer) WatchActivated(opts *bind.WatchOpts, sink chan<- *DRBCoordinatorActivated) (event.Subscription, error) {

	logs, sub, err := _DRBCoordinator.contract.WatchLogs(opts, "Activated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DRBCoordinatorActivated)
				if err := _DRBCoordinator.contract.UnpackLog(event, "Activated", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseActivated is a log parse operation binding the contract event 0x0cc43938d137e7efade6a531f663e78c1fc75257b0d65ffda2fdaf70cb49cdf9.
//
// Solidity: event Activated(address operator)
func (_DRBCoordinator *DRBCoordinatorFilterer) ParseActivated(log types.Log) (*DRBCoordinatorActivated, error) {
	event := new(DRBCoordinatorActivated)
	if err := _DRBCoordinator.contract.UnpackLog(event, "Activated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// DRBCoordinatorCommitIterator is returned from FilterCommit and is used to iterate over the raw logs and unpacked data for Commit events raised by the DRBCoordinator contract.
type DRBCoordinatorCommitIterator struct {
	Event *DRBCoordinatorCommit // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *DRBCoordinatorCommitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DRBCoordinatorCommit)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(DRBCoordinatorCommit)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *DRBCoordinatorCommitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DRBCoordinatorCommitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DRBCoordinatorCommit represents a Commit event raised by the DRBCoordinator contract.
type DRBCoordinatorCommit struct {
	Operator common.Address
	Round    *big.Int
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterCommit is a free log retrieval operation binding the contract event 0x5e1dd8c4451717d5ca4ffbefdada35e22e0871220b9ed9dd03a351f0938c5ed7.
//
// Solidity: event Commit(address operator, uint256 round)
func (_DRBCoordinator *DRBCoordinatorFilterer) FilterCommit(opts *bind.FilterOpts) (*DRBCoordinatorCommitIterator, error) {

	logs, sub, err := _DRBCoordinator.contract.FilterLogs(opts, "Commit")
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorCommitIterator{contract: _DRBCoordinator.contract, event: "Commit", logs: logs, sub: sub}, nil
}

// WatchCommit is a free log subscription operation binding the contract event 0x5e1dd8c4451717d5ca4ffbefdada35e22e0871220b9ed9dd03a351f0938c5ed7.
//
// Solidity: event Commit(address operator, uint256 round)
func (_DRBCoordinator *DRBCoordinatorFilterer) WatchCommit(opts *bind.WatchOpts, sink chan<- *DRBCoordinatorCommit) (event.Subscription, error) {

	logs, sub, err := _DRBCoordinator.contract.WatchLogs(opts, "Commit")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DRBCoordinatorCommit)
				if err := _DRBCoordinator.contract.UnpackLog(event, "Commit", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseCommit is a log parse operation binding the contract event 0x5e1dd8c4451717d5ca4ffbefdada35e22e0871220b9ed9dd03a351f0938c5ed7.
//
// Solidity: event Commit(address operator, uint256 round)
func (_DRBCoordinator *DRBCoordinatorFilterer) ParseCommit(log types.Log) (*DRBCoordinatorCommit, error) {
	event := new(DRBCoordinatorCommit)
	if err := _DRBCoordinator.contract.UnpackLog(event, "Commit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// DRBCoordinatorDeActivatedIterator is returned from FilterDeActivated and is used to iterate over the raw logs and unpacked data for DeActivated events raised by the DRBCoordinator contract.
type DRBCoordinatorDeActivatedIterator struct {
	Event *DRBCoordinatorDeActivated // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *DRBCoordinatorDeActivatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DRBCoordinatorDeActivated)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(DRBCoordinatorDeActivated)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *DRBCoordinatorDeActivatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DRBCoordinatorDeActivatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DRBCoordinatorDeActivated represents a DeActivated event raised by the DRBCoordinator contract.
type DRBCoordinatorDeActivated struct {
	Operator common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterDeActivated is a free log retrieval operation binding the contract event 0x5d10eb48d8c00fb4cc9120533a99e2eac5eb9d0f8ec06216b2e4d5b1ff175a4d.
//
// Solidity: event DeActivated(address operator)
func (_DRBCoordinator *DRBCoordinatorFilterer) FilterDeActivated(opts *bind.FilterOpts) (*DRBCoordinatorDeActivatedIterator, error) {

	logs, sub, err := _DRBCoordinator.contract.FilterLogs(opts, "DeActivated")
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorDeActivatedIterator{contract: _DRBCoordinator.contract, event: "DeActivated", logs: logs, sub: sub}, nil
}

// WatchDeActivated is a free log subscription operation binding the contract event 0x5d10eb48d8c00fb4cc9120533a99e2eac5eb9d0f8ec06216b2e4d5b1ff175a4d.
//
// Solidity: event DeActivated(address operator)
func (_DRBCoordinator *DRBCoordinatorFilterer) WatchDeActivated(opts *bind.WatchOpts, sink chan<- *DRBCoordinatorDeActivated) (event.Subscription, error) {

	logs, sub, err := _DRBCoordinator.contract.WatchLogs(opts, "DeActivated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DRBCoordinatorDeActivated)
				if err := _DRBCoordinator.contract.UnpackLog(event, "DeActivated", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseDeActivated is a log parse operation binding the contract event 0x5d10eb48d8c00fb4cc9120533a99e2eac5eb9d0f8ec06216b2e4d5b1ff175a4d.
//
// Solidity: event DeActivated(address operator)
func (_DRBCoordinator *DRBCoordinatorFilterer) ParseDeActivated(log types.Log) (*DRBCoordinatorDeActivated, error) {
	event := new(DRBCoordinatorDeActivated)
	if err := _DRBCoordinator.contract.UnpackLog(event, "DeActivated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// DRBCoordinatorL1FeeCalculationSetIterator is returned from FilterL1FeeCalculationSet and is used to iterate over the raw logs and unpacked data for L1FeeCalculationSet events raised by the DRBCoordinator contract.
type DRBCoordinatorL1FeeCalculationSetIterator struct {
	Event *DRBCoordinatorL1FeeCalculationSet // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *DRBCoordinatorL1FeeCalculationSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DRBCoordinatorL1FeeCalculationSet)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(DRBCoordinatorL1FeeCalculationSet)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *DRBCoordinatorL1FeeCalculationSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DRBCoordinatorL1FeeCalculationSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DRBCoordinatorL1FeeCalculationSet represents a L1FeeCalculationSet event raised by the DRBCoordinator contract.
type DRBCoordinatorL1FeeCalculationSet struct {
	Mode        uint8
	Coefficient uint8
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterL1FeeCalculationSet is a free log retrieval operation binding the contract event 0x8e63dc2f2e669ce73bebd2580bb9dd9a5d17fa2d046ac02057d8349fc0b0c2f3.
//
// Solidity: event L1FeeCalculationSet(uint8 mode, uint8 coefficient)
func (_DRBCoordinator *DRBCoordinatorFilterer) FilterL1FeeCalculationSet(opts *bind.FilterOpts) (*DRBCoordinatorL1FeeCalculationSetIterator, error) {

	logs, sub, err := _DRBCoordinator.contract.FilterLogs(opts, "L1FeeCalculationSet")
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorL1FeeCalculationSetIterator{contract: _DRBCoordinator.contract, event: "L1FeeCalculationSet", logs: logs, sub: sub}, nil
}

// WatchL1FeeCalculationSet is a free log subscription operation binding the contract event 0x8e63dc2f2e669ce73bebd2580bb9dd9a5d17fa2d046ac02057d8349fc0b0c2f3.
//
// Solidity: event L1FeeCalculationSet(uint8 mode, uint8 coefficient)
func (_DRBCoordinator *DRBCoordinatorFilterer) WatchL1FeeCalculationSet(opts *bind.WatchOpts, sink chan<- *DRBCoordinatorL1FeeCalculationSet) (event.Subscription, error) {

	logs, sub, err := _DRBCoordinator.contract.WatchLogs(opts, "L1FeeCalculationSet")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DRBCoordinatorL1FeeCalculationSet)
				if err := _DRBCoordinator.contract.UnpackLog(event, "L1FeeCalculationSet", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseL1FeeCalculationSet is a log parse operation binding the contract event 0x8e63dc2f2e669ce73bebd2580bb9dd9a5d17fa2d046ac02057d8349fc0b0c2f3.
//
// Solidity: event L1FeeCalculationSet(uint8 mode, uint8 coefficient)
func (_DRBCoordinator *DRBCoordinatorFilterer) ParseL1FeeCalculationSet(log types.Log) (*DRBCoordinatorL1FeeCalculationSet, error) {
	event := new(DRBCoordinatorL1FeeCalculationSet)
	if err := _DRBCoordinator.contract.UnpackLog(event, "L1FeeCalculationSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// DRBCoordinatorOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the DRBCoordinator contract.
type DRBCoordinatorOwnershipTransferredIterator struct {
	Event *DRBCoordinatorOwnershipTransferred // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *DRBCoordinatorOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DRBCoordinatorOwnershipTransferred)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(DRBCoordinatorOwnershipTransferred)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *DRBCoordinatorOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DRBCoordinatorOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DRBCoordinatorOwnershipTransferred represents a OwnershipTransferred event raised by the DRBCoordinator contract.
type DRBCoordinatorOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_DRBCoordinator *DRBCoordinatorFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*DRBCoordinatorOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _DRBCoordinator.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorOwnershipTransferredIterator{contract: _DRBCoordinator.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_DRBCoordinator *DRBCoordinatorFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *DRBCoordinatorOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _DRBCoordinator.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DRBCoordinatorOwnershipTransferred)
				if err := _DRBCoordinator.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_DRBCoordinator *DRBCoordinatorFilterer) ParseOwnershipTransferred(log types.Log) (*DRBCoordinatorOwnershipTransferred, error) {
	event := new(DRBCoordinatorOwnershipTransferred)
	if err := _DRBCoordinator.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// DRBCoordinatorRandomNumberRequestedIterator is returned from FilterRandomNumberRequested and is used to iterate over the raw logs and unpacked data for RandomNumberRequested events raised by the DRBCoordinator contract.
type DRBCoordinatorRandomNumberRequestedIterator struct {
	Event *DRBCoordinatorRandomNumberRequested // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *DRBCoordinatorRandomNumberRequestedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DRBCoordinatorRandomNumberRequested)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(DRBCoordinatorRandomNumberRequested)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *DRBCoordinatorRandomNumberRequestedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DRBCoordinatorRandomNumberRequestedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DRBCoordinatorRandomNumberRequested represents a RandomNumberRequested event raised by the DRBCoordinator contract.
type DRBCoordinatorRandomNumberRequested struct {
	Round              *big.Int
	ActivatedOperators []common.Address
	Raw                types.Log // Blockchain specific contextual infos
}

// FilterRandomNumberRequested is a free log retrieval operation binding the contract event 0xa76cb50e13ed7dc76531452d748a43f7f4822af11af8b0c27781247c32035b74.
//
// Solidity: event RandomNumberRequested(uint256 round, address[] activatedOperators)
func (_DRBCoordinator *DRBCoordinatorFilterer) FilterRandomNumberRequested(opts *bind.FilterOpts) (*DRBCoordinatorRandomNumberRequestedIterator, error) {

	logs, sub, err := _DRBCoordinator.contract.FilterLogs(opts, "RandomNumberRequested")
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorRandomNumberRequestedIterator{contract: _DRBCoordinator.contract, event: "RandomNumberRequested", logs: logs, sub: sub}, nil
}

// WatchRandomNumberRequested is a free log subscription operation binding the contract event 0xa76cb50e13ed7dc76531452d748a43f7f4822af11af8b0c27781247c32035b74.
//
// Solidity: event RandomNumberRequested(uint256 round, address[] activatedOperators)
func (_DRBCoordinator *DRBCoordinatorFilterer) WatchRandomNumberRequested(opts *bind.WatchOpts, sink chan<- *DRBCoordinatorRandomNumberRequested) (event.Subscription, error) {

	logs, sub, err := _DRBCoordinator.contract.WatchLogs(opts, "RandomNumberRequested")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DRBCoordinatorRandomNumberRequested)
				if err := _DRBCoordinator.contract.UnpackLog(event, "RandomNumberRequested", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRandomNumberRequested is a log parse operation binding the contract event 0xa76cb50e13ed7dc76531452d748a43f7f4822af11af8b0c27781247c32035b74.
//
// Solidity: event RandomNumberRequested(uint256 round, address[] activatedOperators)
func (_DRBCoordinator *DRBCoordinatorFilterer) ParseRandomNumberRequested(log types.Log) (*DRBCoordinatorRandomNumberRequested, error) {
	event := new(DRBCoordinatorRandomNumberRequested)
	if err := _DRBCoordinator.contract.UnpackLog(event, "RandomNumberRequested", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// DRBCoordinatorRefundIterator is returned from FilterRefund and is used to iterate over the raw logs and unpacked data for Refund events raised by the DRBCoordinator contract.
type DRBCoordinatorRefundIterator struct {
	Event *DRBCoordinatorRefund // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *DRBCoordinatorRefundIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DRBCoordinatorRefund)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(DRBCoordinatorRefund)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *DRBCoordinatorRefundIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DRBCoordinatorRefundIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DRBCoordinatorRefund represents a Refund event raised by the DRBCoordinator contract.
type DRBCoordinatorRefund struct {
	Round *big.Int
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterRefund is a free log retrieval operation binding the contract event 0x2e1897b0591d764356194f7a795238a87c1987c7a877568e50d829d547c92b97.
//
// Solidity: event Refund(uint256 round)
func (_DRBCoordinator *DRBCoordinatorFilterer) FilterRefund(opts *bind.FilterOpts) (*DRBCoordinatorRefundIterator, error) {

	logs, sub, err := _DRBCoordinator.contract.FilterLogs(opts, "Refund")
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorRefundIterator{contract: _DRBCoordinator.contract, event: "Refund", logs: logs, sub: sub}, nil
}

// WatchRefund is a free log subscription operation binding the contract event 0x2e1897b0591d764356194f7a795238a87c1987c7a877568e50d829d547c92b97.
//
// Solidity: event Refund(uint256 round)
func (_DRBCoordinator *DRBCoordinatorFilterer) WatchRefund(opts *bind.WatchOpts, sink chan<- *DRBCoordinatorRefund) (event.Subscription, error) {

	logs, sub, err := _DRBCoordinator.contract.WatchLogs(opts, "Refund")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DRBCoordinatorRefund)
				if err := _DRBCoordinator.contract.UnpackLog(event, "Refund", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRefund is a log parse operation binding the contract event 0x2e1897b0591d764356194f7a795238a87c1987c7a877568e50d829d547c92b97.
//
// Solidity: event Refund(uint256 round)
func (_DRBCoordinator *DRBCoordinatorFilterer) ParseRefund(log types.Log) (*DRBCoordinatorRefund, error) {
	event := new(DRBCoordinatorRefund)
	if err := _DRBCoordinator.contract.UnpackLog(event, "Refund", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// DRBCoordinatorRevealIterator is returned from FilterReveal and is used to iterate over the raw logs and unpacked data for Reveal events raised by the DRBCoordinator contract.
type DRBCoordinatorRevealIterator struct {
	Event *DRBCoordinatorReveal // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *DRBCoordinatorRevealIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DRBCoordinatorReveal)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(DRBCoordinatorReveal)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *DRBCoordinatorRevealIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DRBCoordinatorRevealIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DRBCoordinatorReveal represents a Reveal event raised by the DRBCoordinator contract.
type DRBCoordinatorReveal struct {
	Operator common.Address
	Round    *big.Int
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterReveal is a free log retrieval operation binding the contract event 0xf254aace0ef98d6ac1a0d84c95648f8e3f7a1881dbb43393709ecd004b00f103.
//
// Solidity: event Reveal(address operator, uint256 round)
func (_DRBCoordinator *DRBCoordinatorFilterer) FilterReveal(opts *bind.FilterOpts) (*DRBCoordinatorRevealIterator, error) {

	logs, sub, err := _DRBCoordinator.contract.FilterLogs(opts, "Reveal")
	if err != nil {
		return nil, err
	}
	return &DRBCoordinatorRevealIterator{contract: _DRBCoordinator.contract, event: "Reveal", logs: logs, sub: sub}, nil
}

// WatchReveal is a free log subscription operation binding the contract event 0xf254aace0ef98d6ac1a0d84c95648f8e3f7a1881dbb43393709ecd004b00f103.
//
// Solidity: event Reveal(address operator, uint256 round)
func (_DRBCoordinator *DRBCoordinatorFilterer) WatchReveal(opts *bind.WatchOpts, sink chan<- *DRBCoordinatorReveal) (event.Subscription, error) {

	logs, sub, err := _DRBCoordinator.contract.WatchLogs(opts, "Reveal")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DRBCoordinatorReveal)
				if err := _DRBCoordinator.contract.UnpackLog(event, "Reveal", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseReveal is a log parse operation binding the contract event 0xf254aace0ef98d6ac1a0d84c95648f8e3f7a1881dbb43393709ecd004b00f103.
//
// Solidity: event Reveal(address operator, uint256 round)
func (_DRBCoordinator *DRBCoordinatorFilterer) ParseReveal(log types.Log) (*DRBCoordinatorReveal, error) {
	event := new(DRBCoordinatorReveal)
	if err := _DRBCoordinator.contract.UnpackLog(event, "Reveal", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

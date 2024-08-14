package transactions

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/fatih/color"
	"github.com/tokamak-network/DRB-Node/utils"
)

// OperatorDeposit deposits a specified amount of Ether to the contract.
func OperatorDeposit(ctx context.Context, pofClient *utils.PoFClient) (common.Address, *types.Transaction, error) {

	// Use pofClient methods and properties
	chainID, err := pofClient.Client.NetworkID(ctx)
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to fetch network ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(pofClient.PrivateKey, chainID)
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to create authorized transactor: %v", err)
	}

	nonce, err := pofClient.Client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to fetch nonce: %v", err)
	}
	auth.Nonce = big.NewInt(int64(nonce))

	gasPrice, err := pofClient.Client.SuggestGasPrice(ctx)
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to suggest gas price: %v", err)
	}
	auth.GasPrice = gasPrice

	// Set the amount of Ether you want to send in the transaction
	amount := new(big.Int)
	amount.SetString("5000000000000000", 10) // 0.005 ether in wei
	auth.Value = amount                      // Setting the value of the transaction to 0.005 ether

	packedData, err := pofClient.ContractABI.Pack("operatorDeposit")
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to pack data for deposit: %v", err)
	}

	tx := types.NewTransaction(auth.Nonce.Uint64(), pofClient.ContractAddress, amount, 3000000, auth.GasPrice, packedData)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), pofClient.PrivateKey)
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to sign the transaction: %v", err)
	}

	if err := pofClient.Client.SendTransaction(ctx, signedTx); err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to send the signed transaction: %v", err)
	}

	receipt, err := bind.WaitMined(ctx, pofClient.Client, signedTx)
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to wait for transaction to be mined: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		errMsg := fmt.Sprintf("transaction %s reverted", signedTx.Hash().Hex())
		color.New(color.FgHiRed, color.Bold).Printf("❌ %s\n", errMsg)
		return common.Address{}, nil, fmt.Errorf("%s", errMsg)
	}

	color.New(color.FgHiGreen, color.Bold).Printf("✅  Deposit successful!!\n🔗 Tx Hash: %s\n", signedTx.Hash().Hex())
	return auth.From, signedTx, nil // Return the sender address and the transaction
}

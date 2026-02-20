package setup

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// TestAccount represents a test Ethereum account
type TestAccount struct {
	Address    common.Address
	PrivateKey *ecdsa.PrivateKey
	Auth       *bind.TransactOpts
}

// GethTestEnv manages a local Geth dev node for testing
type GethTestEnv struct {
	Client          *ethclient.Client
	RpcClient       *rpc.Client
	ChainID         *big.Int
	ContractAddress common.Address
	ContractABI     *abi.ABI
	ConsumerAddress common.Address
	ConsumerABI     *abi.ABI
	LeaderAccount   *TestAccount
	RegularAccounts []*TestAccount
	DevCoinbase     common.Address
	RpcURL          string
	ContractABIPath string
	ConsumerABIPath string
}

func StartGethDevNode(ctx context.Context) (*GethTestEnv, error) {
	fmt.Println(" Step 1: Starting Geth in dev mode...")

	fmt.Println("⏳ Waiting for Geth to be ready...")
	time.Sleep(5 * time.Second)

	rpcURL := "ws://127.0.0.1:8546"

	rpcClient, err := rpc.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to geth rpc: %w", err)
	}

	client := ethclient.NewClient(rpcClient)
	if client == nil {
		rpcClient.Close()
		return nil, fmt.Errorf("failed to create eth client")
	}

	// Verify connection and get chain ID
	chainID := new(big.Int).SetUint64(1337)

	fmt.Printf("Geth ready! Chain ID: %s\n", chainID.String())
	var accounts []string
	if err := rpcClient.CallContext(ctx, &accounts, "eth_accounts"); err != nil {
		client.Close()
		rpcClient.Close()
		return nil, fmt.Errorf("failed to get accounts: %w", err)
	}

	var devCoinbase common.Address
	if len(accounts) > 0 {
		devCoinbase = common.HexToAddress(accounts[0])
		fmt.Printf("💰 Dev coinbase account: %s\n", devCoinbase.Hex())
	} else {
		devCoinbase = common.HexToAddress("0x0")
		fmt.Println(" Warning: Could not get dev account, will use test accounts directly")
	}

	// Get project root
	cwd, err := os.Getwd()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	projectRoot := cwd
	for {
		goModPath := filepath.Join(projectRoot, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			projectRoot = cwd
			break
		}
		projectRoot = parent
	}

	contractABIPath := filepath.Join(projectRoot, "contract", "abi", "Commit2RevealDRB.json")
	consumerABIPath := filepath.Join(projectRoot, "contract", "abi", "ConsumerExampleV2.json")

	env := &GethTestEnv{
		Client:          client,
		RpcClient:       rpcClient,
		ChainID:         chainID,
		DevCoinbase:     devCoinbase,
		RpcURL:          rpcURL,
		ContractABIPath: contractABIPath,
		ConsumerABIPath: consumerABIPath,
	}

	// Create test accounts
	if err := env.setupTestAccounts(ctx); err != nil {
		env.Cleanup()
		return nil, err
	}

	return env, nil
}

// setupTestAccounts creates test accounts for Geth dev mode
func (env *GethTestEnv) setupTestAccounts(ctx context.Context) error {
	fmt.Println("Step 2: Setting up test accounts...")

	defaultKeys := []string{
		"0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
		"0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
		"0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a",
		"0x7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6",
	}

	// Leader account (first account)
	leaderKey, err := crypto.HexToECDSA(defaultKeys[0][2:])
	if err != nil {
		return fmt.Errorf("failed to parse leader key: %w", err)
	}

	leaderAuth, err := bind.NewKeyedTransactorWithChainID(leaderKey, env.ChainID)
	if err != nil {
		return fmt.Errorf("failed to create leader auth: %w", err)
	}

	env.LeaderAccount = &TestAccount{
		Address:    crypto.PubkeyToAddress(leaderKey.PublicKey),
		PrivateKey: leaderKey,
		Auth:       leaderAuth,
	}

	fmt.Printf("Leader Account: %s\n", env.LeaderAccount.Address.Hex())
	if err := env.fundAccount(ctx, env.LeaderAccount.Address); err != nil {
		return fmt.Errorf("failed to fund leader account: %w", err)
	}

	// Regular node accounts
	env.RegularAccounts = make([]*TestAccount, 3)
	for i := 0; i < 3; i++ {
		key, err := crypto.HexToECDSA(defaultKeys[i+1][2:])
		if err != nil {
			return fmt.Errorf("failed to parse regular key %d: %w", i, err)
		}

		auth, err := bind.NewKeyedTransactorWithChainID(key, env.ChainID)
		if err != nil {
			return fmt.Errorf("failed to create auth %d: %w", i, err)
		}

		env.RegularAccounts[i] = &TestAccount{
			Address:    crypto.PubkeyToAddress(key.PublicKey),
			PrivateKey: key,
			Auth:       auth,
		}

		fmt.Printf("  🔹 Regular Account %d: %s\n", i+1, env.RegularAccounts[i].Address.Hex())

		if err := env.fundAccount(ctx, env.RegularAccounts[i].Address); err != nil {
			return fmt.Errorf("failed to fund regular account %d: %w", i, err)
		}
	}

	fmt.Println(" Test accounts ready and funded!")
	return nil
}

// fundAccount funds an account by transferring ETH from the Geth dev coinbase
func (env *GethTestEnv) fundAccount(ctx context.Context, address common.Address) error {
	// Check if already funded
	balance, err := env.Client.BalanceAt(ctx, address, nil)
	if err != nil {
		return err
	}

	requiredBalance := new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18)) // 100 ETH
	if balance.Cmp(requiredBalance) >= 0 {
		fmt.Printf("    ✓ Account %s already funded with %s ETH\n", address.Hex(), new(big.Int).Div(balance, big.NewInt(1e18)).String())
		return nil
	}

	// Transfer from coinbase using eth_sendTransaction RPC
	fundAmount := new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e18)) // 1000 ETH

	type txParams struct {
		From  string `json:"from"`
		To    string `json:"to"`
		Value string `json:"value"`
	}

	params := txParams{
		From:  env.DevCoinbase.Hex(),
		To:    address.Hex(),
		Value: hexutil.EncodeBig(fundAmount),
	}

	var txHash string
	err = env.RpcClient.CallContext(ctx, &txHash, "eth_sendTransaction", params)
	if err != nil {
		return fmt.Errorf("failed to send funding transaction: %w", err)
	}

	fmt.Printf("Funding tx: %s\n", txHash)

	// Wait for transaction to be mined
	hash := common.HexToHash(txHash)
	if err := env.WaitForTransaction(ctx, &hash); err != nil {
		return fmt.Errorf("failed to wait for funding transaction: %w", err)
	}

	// Verify balance
	balance, err = env.Client.BalanceAt(ctx, address, nil)
	if err != nil {
		return err
	}

	fmt.Printf("Account %s funded with %s ETH\n", address.Hex(), new(big.Int).Div(balance, big.NewInt(1e18)).String())
	return nil
}

// DeployContracts deploys both the Commit2RevealDRB and ConsumerExample contracts
func (env *GethTestEnv) DeployContracts(ctx context.Context) error {

	// Set gas parameters
	env.LeaderAccount.Auth.GasLimit = 10_000_000
	env.LeaderAccount.Auth.GasPrice = big.NewInt(1000000000) // 1 gwei

	// Deploy main DRB contract (Commit2RevealDRB)
	fmt.Println("\nDeploying Commit2RevealDRB contract...")
	address, tx, parsedABI, err := DeployCommitReveal2L2(
		ctx,
		env.Client,
		env.LeaderAccount.Auth,
		env.ContractABIPath,
	)
	if err != nil {
		return fmt.Errorf("failed to deploy Commit2RevealDRB: %w", err)
	}

	env.ContractAddress = address
	env.ContractABI = parsedABI

	if err := env.WaitForTransaction(ctx, tx); err != nil {
		return fmt.Errorf("failed to wait for Commit2RevealDRB deployment: %w", err)
	}
	fmt.Printf("  Commit2RevealDRB deployed at: %s\n", address.Hex())

	// Reset nonce for the deployer account
	env.RegularAccounts[0].Auth.GasLimit = 10_000_000
	env.RegularAccounts[0].Auth.GasPrice = big.NewInt(1000000000)

	consumerAddr, tx2, consumerABI, err := DeployConsumerExample(
		ctx,
		env.Client,
		env.RegularAccounts[0].Auth,
		env.ConsumerABIPath,
		env.ContractAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to deploy ConsumerExampleV2: %w", err)
	}

	env.ConsumerAddress = consumerAddr
	env.ConsumerABI = consumerABI

	// Wait for deployment to be mined
	fmt.Println("Waiting for ConsumerExampleV2 deployment to be mined...")
	if err := env.WaitForTransaction(ctx, tx2); err != nil {
		return fmt.Errorf("failed to wait for ConsumerExampleV2 deployment: %w", err)
	}
	fmt.Printf("ConsumerExampleV2 deployed at: %s\n", consumerAddr.Hex())

	fmt.Println("\n All contracts deployed successfully!")
	return nil
}

// GetBalance returns the balance of an address
func (env *GethTestEnv) GetBalance(ctx context.Context, address common.Address) (*big.Int, error) {
	return env.Client.BalanceAt(ctx, address, nil)
}

// WaitForTransaction waits for a transaction to be mined
func (env *GethTestEnv) WaitForTransaction(ctx context.Context, tx *common.Hash) error {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for transaction %s", tx.Hex())
		case <-ticker.C:
			receipt, err := env.Client.TransactionReceipt(ctx, *tx)
			if err == nil {
				if receipt.Status == 1 {
					fmt.Printf("Transaction mined in block %d, gas used: %d\n", receipt.BlockNumber.Uint64(), receipt.GasUsed)
					return nil
				}
				// Get the transaction to see revert reason
				txData, _, err := env.Client.TransactionByHash(ctx, *tx)
				if err == nil {
					fmt.Printf(" Transaction reverted - Gas: %d, GasPrice: %s\n", txData.Gas(), txData.GasPrice().String())
				}
				return fmt.Errorf("transaction %s failed - status: %d, gasUsed: %d", tx.Hex(), receipt.Status, receipt.GasUsed)
			}
		}
	}
}

// WaitForNextBlock waits for the next block to be mined
func (env *GethTestEnv) WaitForNextBlock(ctx context.Context) error {
	currentBlock, err := env.Client.BlockNumber(ctx)
	if err != nil {
		return err
	}

	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for next block")
		case <-ticker.C:
			newBlock, err := env.Client.BlockNumber(ctx)
			if err != nil {
				return err
			}
			if newBlock > currentBlock {
				return nil
			}
		}
	}
}

// Cleanup stops Geth and cleans up resources
func (env *GethTestEnv) Cleanup() {
	fmt.Println("Cleaning up Geth environment...")

	if env.Client != nil {
		env.Client.Close()
	}

	if env.RpcClient != nil {
		env.RpcClient.Close()
	}

	fmt.Println("Geth cleanup complete")
}

// GetCurrentBlock returns the current block number
func (env *GethTestEnv) GetCurrentBlock(ctx context.Context) (uint64, error) {
	return env.Client.BlockNumber(ctx)
}

// GetNonce returns the nonce for an address
func (env *GethTestEnv) GetNonce(ctx context.Context, address common.Address) (uint64, error) {
	return env.Client.PendingNonceAt(ctx, address)
}

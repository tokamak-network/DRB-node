package eth

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
)

func Test_SendWithRetry(t *testing.T) {
	logger.InitLogger()
	envCfg := appconfig.Get()
	fallbackEthClient, err := fallback_ethclient.NewFallbackRPCClient(envCfg.RPCURLs)
	if err != nil {
		t.Fatalf("Failed to create fallback eth client: %v", err)
	}

	chainID, err := fallbackEthClient.ChainID(context.Background())
	require.NoError(t, err)

	privateKey, err := crypto.HexToECDSA(envCfg.PrivateKey)
	require.NoError(t, err)

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	receipt, signedTx, err := sendWithRetry(
		context.Background(),
		fallbackEthClient,
		chainID,
		auth,
		auth.From,
		big.NewInt(0),
		[]byte{},
	)
	if err != nil {
		t.Fatalf("Failed to send with retry: %v", err)
	}
	if receipt == nil {
		t.Fatalf("Failed to get receipt")
	}
	if signedTx == nil {
		t.Fatalf("Failed to get signed tx")
	}
	t.Logf("Transaction confirmed in block %v", receipt.BlockNumber)
	t.Logf("Receipt: %v", receipt)
	t.Logf("Signed tx: %v", signedTx)
}

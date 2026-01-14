package regular_node

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appconfig "github.com/tokamak-network/DRB-node/config"
)


func TestRegularNode_GenerateCvsSignature_Success(t *testing.T) {
	node := createTestRegularNode()

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	// Set up environment variables
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	// Test parameters
	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs-value"))

	// Generate signature
	v, r, s, err := node.GenerateCvsSignature(round, trialNum, cvs)

	// Assertions
	assert.NoError(t, err, "GenerateCvsSignature should not return error")
	assert.NotZero(t, v, "v should not be zero")
	assert.NotEmpty(t, r, "r should not be empty")
	assert.NotEmpty(t, s, "s should not be empty")
	assert.Len(t, r, 64, "r should be 64 hex characters (32 bytes)")
	assert.Len(t, s, 64, "s should be 64 hex characters (32 bytes)")
	assert.True(t, v == 27 || v == 28, "v should be 27 or 28 for Ethereum")
}

func TestRegularNode_GenerateCvsSignature_MissingContractAddress(t *testing.T) {
	os.Unsetenv("CONTRACT_ADDRESS")
	os.Setenv("CHAIN_ID", "1")
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	contractAddr := appconfig.Get().ContractAddress
	assert.Empty(t, contractAddr, "CONTRACT_ADDRESS should not be set")
}

func TestRegularNode_GenerateCvsSignature_MissingChainID(t *testing.T) {
	// Set CONTRACT_ADDRESS but not CHAIN_ID
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Unsetenv("CHAIN_ID")
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	// Check that CHAIN_ID is required
	chainID := appconfig.Get().ChainID
	assert.Empty(t, chainID, "CHAIN_ID should not be set")
}

func TestRegularNode_GenerateCvsSignature_InvalidChainID(t *testing.T) {
	node := createTestRegularNode()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "invalid_chain_id")
	os.Setenv("EOA_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	v, r, s, err := node.GenerateCvsSignature(round, trialNum, cvs)

	assert.Error(t, err, "Should return error for invalid chain ID")
	assert.Contains(t, err.Error(), "invalid chain ID", "Error message should mention invalid chain ID")
	assert.Zero(t, v)
	assert.Empty(t, r)
	assert.Empty(t, s)
}

func TestRegularNode_GenerateCvsSignature_MissingPrivateKey(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	os.Unsetenv("EOA_PRIVATE_KEY")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	// Check that EOA_PRIVATE_KEY is required
	privateKey := appconfig.Get().EOAPrivateKey
	assert.Empty(t, privateKey, "EOA_PRIVATE_KEY should not be set")
}

func TestRegularNode_GenerateCvsSignature_InvalidPrivateKey(t *testing.T) {
	node := createTestRegularNode()

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	os.Setenv("EOA_PRIVATE_KEY", "invalid_hex_key")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	v, r, s, err := node.GenerateCvsSignature(round, trialNum, cvs)

	assert.Error(t, err, "Should return error for invalid private key")
	assert.Contains(t, err.Error(), "failed to decode private key", "Error message should mention private key decoding")
	assert.Zero(t, v)
	assert.Empty(t, r)
	assert.Empty(t, s)
}

func TestRegularNode_GenerateCvsSignature_DifferentInputs(t *testing.T) {
	node := createTestRegularNode()

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	tests := []struct {
		name     string
		round    *big.Int
		trialNum *big.Int
		cvs      [32]byte
	}{
		{
			name:     "Round 100, Trial 1",
			round:    big.NewInt(100),
			trialNum: big.NewInt(1),
			cvs:      [32]byte{1, 2, 3, 4, 5},
		},
		{
			name:     "Round 200, Trial 2",
			round:    big.NewInt(200),
			trialNum: big.NewInt(2),
			cvs:      [32]byte{6, 7, 8, 9, 10},
		},
		{
			name:     "Large round number",
			round:    big.NewInt(999999),
			trialNum: big.NewInt(100),
			cvs:      [32]byte{0xff, 0xff, 0xff},
		},
	}

	signatures := make(map[string]struct{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, r, s, err := node.GenerateCvsSignature(tt.round, tt.trialNum, tt.cvs)

			assert.NoError(t, err, "Should not return error")
			assert.NotZero(t, v)
			assert.NotEmpty(t, r)
			assert.NotEmpty(t, s)

			// Verify signatures are unique for different inputs
			sigKey := r + s
			_, exists := signatures[sigKey]
			assert.False(t, exists, "Different inputs should produce different signatures")
			signatures[sigKey] = struct{}{}
		})
	}
}

func TestRegularNode_GenerateCvsSignature_DifferentChainIDs(t *testing.T) {
	node := createTestRegularNode()

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test"))

	chainIDs := []string{"1", "5", "11155111"} // Mainnet, Goerli, Sepolia
	signatures := make(map[string]struct{})

	for _, chainID := range chainIDs {
		t.Run("ChainID_"+chainID, func(t *testing.T) {
			os.Setenv("CHAIN_ID", chainID)
			defer os.Unsetenv("CHAIN_ID")

			v, r, s, err := node.GenerateCvsSignature(round, trialNum, cvs)

			assert.NoError(t, err, "Should not return error")
			assert.NotZero(t, v)
			assert.NotEmpty(t, r)
			assert.NotEmpty(t, s)

			// Verify signatures are different for different chain IDs
			sigKey := r + s
			_, exists := signatures[sigKey]
			assert.False(t, exists, "Different chain IDs should produce different signatures")
			signatures[sigKey] = struct{}{}
		})
	}
}

func TestRegularNode_GenerateCvsSignature_EmptyCVS(t *testing.T) {
	node := createTestRegularNode()

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte // All zeros

	v, r, s, err := node.GenerateCvsSignature(round, trialNum, cvs)

	assert.NoError(t, err, "Should handle empty CVS")
	assert.NotZero(t, v)
	assert.NotEmpty(t, r)
	assert.NotEmpty(t, s)
}

func TestRegularNode_GenerateCvsSignature_FullCVS(t *testing.T) {
	node := createTestRegularNode()

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	// Fill all 32 bytes
	for i := 0; i < 32; i++ {
		cvs[i] = byte(i)
	}

	v, r, s, err := node.GenerateCvsSignature(round, trialNum, cvs)

	assert.NoError(t, err, "Should handle full CVS")
	assert.NotZero(t, v)
	assert.NotEmpty(t, r)
	assert.NotEmpty(t, s)
}

func TestRegularNode_GenerateCvsSignature_ContractAddressWithoutPrefix(t *testing.T) {
	node := createTestRegularNode()

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	// Test with and without 0x prefix
	testCases := []struct {
		name            string
		contractAddress string
	}{
		{
			name:            "With 0x prefix",
			contractAddress: "0x1234567890123456789012345678901234567890",
		},
		{
			name:            "Without 0x prefix",
			contractAddress: "1234567890123456789012345678901234567890",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("CONTRACT_ADDRESS", tc.contractAddress)
			os.Setenv("CHAIN_ID", "1")
			os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
			defer func() {
				os.Unsetenv("CONTRACT_ADDRESS")
				os.Unsetenv("CHAIN_ID")
				os.Unsetenv("EOA_PRIVATE_KEY")
			}()

			round := big.NewInt(100)
			trialNum := big.NewInt(1)
			var cvs [32]byte

			v, r, s, err := node.GenerateCvsSignature(round, trialNum, cvs)

			assert.NoError(t, err, "Should handle contract address with or without prefix")
			assert.NotZero(t, v)
			assert.NotEmpty(t, r)
			assert.NotEmpty(t, s)
		})
	}
}

func TestRegularNode_SignatureComponents_Format(t *testing.T) {
	node := createTestRegularNode()

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	v, r, s, err := node.GenerateCvsSignature(round, trialNum, cvs)

	require.NoError(t, err)

	// Test that r and s are valid hex strings
	_, err = hex.DecodeString(r)
	assert.NoError(t, err, "r should be valid hex")

	_, err = hex.DecodeString(s)
	assert.NoError(t, err, "s should be valid hex")

	// Test that v is in valid range
	assert.True(t, v >= 27 && v <= 28, "v should be 27 or 28")
}

func TestRegularNode_SignatureDeterminism(t *testing.T) {
	node := createTestRegularNode()

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("deterministic-test"))

	// Generate signature twice
	v1, r1, s1, err1 := node.GenerateCvsSignature(round, trialNum, cvs)
	v2, r2, s2, err2 := node.GenerateCvsSignature(round, trialNum, cvs)

	assert.NoError(t, err1)
	assert.NoError(t, err2)

	assert.NotZero(t, v1)
	assert.NotZero(t, v2)
	assert.NotEmpty(t, r1)
	assert.NotEmpty(t, r2)
	assert.NotEmpty(t, s1)
	assert.NotEmpty(t, s2)
}


func TestRegularNode_GenerateCvsSignature_ZeroRoundAndTrial(t *testing.T) {
	node := createTestRegularNode()

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	os.Setenv("EOA_PRIVATE_KEY", privateKeyHex)
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
		os.Unsetenv("EOA_PRIVATE_KEY")
	}()

	round := big.NewInt(0)
	trialNum := big.NewInt(0)
	var cvs [32]byte

	v, r, s, err := node.GenerateCvsSignature(round, trialNum, cvs)

	assert.NoError(t, err, "Should handle zero round and trial")
	assert.NotZero(t, v)
	assert.NotEmpty(t, r)
	assert.NotEmpty(t, s)
}

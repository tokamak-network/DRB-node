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
)

func TestRegularNode_intToBytes(t *testing.T) {
	node := createTestRegularNode()

	tests := []struct {
		name     string
		input    *big.Int
		expected int // length of output
	}{
		{
			name:     "Zero value",
			input:    big.NewInt(0),
			expected: 32,
		},
		{
			name:     "Small number",
			input:    big.NewInt(123),
			expected: 32,
		},
		{
			name:     "Large number",
			input:    big.NewInt(123456789),
			expected: 32,
		},
		{
			name:     "Max uint256",
			input:    new(big.Int).Sub(new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil), big.NewInt(1)),
			expected: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := node.intToBytes(tt.input)
			assert.Len(t, result, tt.expected, "intToBytes should always return 32 bytes")

			// Verify the conversion is correct by converting back
			resultBigInt := new(big.Int).SetBytes(result)
			assert.Equal(t, tt.input.String(), resultBigInt.String(), "Round-trip conversion should match")
		})
	}
}

func TestRegularNode_abiEncode(t *testing.T) {
	node := createTestRegularNode()

	tests := []struct {
		name           string
		input          [][]byte
		expectedLength int
	}{
		{
			name:           "Empty input",
			input:          [][]byte{},
			expectedLength: 0,
		},
		{
			name: "Single element",
			input: [][]byte{
				[]byte("test"),
			},
			expectedLength: 32,
		},
		{
			name: "Multiple elements",
			input: [][]byte{
				[]byte("test1"),
				[]byte("test2"),
				[]byte("test3"),
			},
			expectedLength: 96, // 3 * 32
		},
		{
			name: "Element exactly 32 bytes",
			input: [][]byte{
				make([]byte, 32),
			},
			expectedLength: 32,
		},
		{
			name: "Element larger than 32 bytes",
			input: [][]byte{
				make([]byte, 40),
			},
			expectedLength: 40, // LeftPadBytes will keep elements larger than 32 bytes as is
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := node.abiEncode(tt.input...)
			assert.Len(t, result, tt.expectedLength, "abiEncode output length mismatch")

			// Verify each element is 32-byte padded
			if len(tt.input) > 0 {
				for i := 0; i < len(result)/32; i++ {
					chunk := result[i*32 : (i+1)*32]
					assert.Len(t, chunk, 32, "Each encoded chunk should be 32 bytes")
				}
			}
		})
	}
}

func TestRegularNode_abiEncodePacked(t *testing.T) {
	node := createTestRegularNode()

	tests := []struct {
		name           string
		input          [][]byte
		expectedLength int
	}{
		{
			name:           "Empty input",
			input:          [][]byte{},
			expectedLength: 0,
		},
		{
			name: "Single element",
			input: [][]byte{
				[]byte("test"),
			},
			expectedLength: 4,
		},
		{
			name: "Multiple elements of different sizes",
			input: [][]byte{
				[]byte("ab"),
				[]byte("cdef"),
				[]byte("ghij"),
			},
			expectedLength: 10,
		},
		{
			name: "32-byte elements",
			input: [][]byte{
				make([]byte, 32),
				make([]byte, 32),
			},
			expectedLength: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := node.abiEncodePacked(tt.input...)
			assert.Len(t, result, tt.expectedLength, "abiEncodePacked output length mismatch")
		})
	}
}

func TestRegularNode_abiEncodePacked_NosPadding(t *testing.T) {
	node := createTestRegularNode()

	// Test that abiEncodePacked does NOT add padding
	input1 := []byte{0x01, 0x02}
	input2 := []byte{0x03, 0x04, 0x05}

	result := node.abiEncodePacked(input1, input2)

	// Should be exactly 5 bytes (no padding)
	assert.Len(t, result, 5, "abiEncodePacked should not add padding")
	assert.Equal(t, []byte{0x01, 0x02, 0x03, 0x04, 0x05}, result, "Values should be concatenated without padding")
}

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

	contractAddr := os.Getenv("CONTRACT_ADDRESS")
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
	chainID := os.Getenv("CHAIN_ID")
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
	privateKey := os.Getenv("EOA_PRIVATE_KEY")
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

func TestRegularNode_intToBytes_NilInput(t *testing.T) {
	node := createTestRegularNode()
	zeroValue := big.NewInt(0)
	result := node.intToBytes(zeroValue)
	assert.Len(t, result, 32, "Should handle zero input")

	// All bytes should be zero
	for _, b := range result {
		assert.Equal(t, byte(0), b, "Zero input should produce all zeros")
	}
}

func TestRegularNode_abiEncode_NilElements(t *testing.T) {
	node := createTestRegularNode()

	// Test with nil elements
	result := node.abiEncode(nil, []byte("test"), nil)
	assert.Len(t, result, 96, "Should handle nil elements in the mix")
}

func TestRegularNode_abiEncodePacked_NilElements(t *testing.T) {
	node := createTestRegularNode()

	// Test with nil elements
	result := node.abiEncodePacked(nil, []byte("test"), nil)
	assert.Len(t, result, 4, "Should concatenate non-nil elements only")
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

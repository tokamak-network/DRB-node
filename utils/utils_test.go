package utils

import (
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUniqueKey(t *testing.T) {
	tests := []struct {
		name     string
		round    string
		trialNum string
		expected string
	}{
		{
			name:     "basic unique key",
			round:    "1",
			trialNum: "0",
			expected: "1-0",
		},
		{
			name:     "larger numbers",
			round:    "123",
			trialNum: "456",
			expected: "123-456",
		},
		{
			name:     "empty strings",
			round:    "",
			trialNum: "",
			expected: "-",
		},
		{
			name:     "special characters",
			round:    "abc-123",
			trialNum: "def_456",
			expected: "abc-123-def_456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUniqueKey(tt.round, tt.trialNum)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSignData(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	testData := "test data for signing"
	signature := SignData(testData, privateKey)

	assert.NotNil(t, signature)
	assert.Greater(t, len(signature), 0)

	hash := crypto.Keccak256Hash([]byte(testData))
	pubKey, err := crypto.SigToPub(hash.Bytes(), signature)
	require.NoError(t, err)

	expectedAddress := crypto.PubkeyToAddress(*pubKey)
	actualAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	assert.Equal(t, expectedAddress, actualAddress)
}

func TestVerifySignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	t.Run("valid signature", func(t *testing.T) {
		signature := SignData(address, privateKey)
		verification := Verification{
			EOAAddress: address,
			Signature:  signature,
		}

		result := VerifySignature(verification)
		assert.True(t, result)
	})

	t.Run("invalid signature", func(t *testing.T) {
		otherPrivateKey, err := crypto.GenerateKey()
		require.NoError(t, err)
		wrongSignature := SignData(address, otherPrivateKey)
		verification := Verification{
			EOAAddress: address,
			Signature:  wrongSignature,
		}

		result := VerifySignature(verification)
		assert.False(t, result)
	})

	t.Run("invalid signature format", func(t *testing.T) {
		verification := Verification{
			EOAAddress: address,
			Signature:  []byte("invalid signature"),
		}

		result := VerifySignature(verification)
		assert.False(t, result)
	})
}

func TestVerifySignatureForRegularNode(t *testing.T) {
	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	regularPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	regularEOA := crypto.PubkeyToAddress(regularPrivateKey.PublicKey).Hex()

	t.Run("valid leader signature", func(t *testing.T) {
		signature := SignData(regularEOA, leaderPrivateKey)

		verification := Verification{
			EOAAddress: regularEOA,
			Signature:  signature,
		}

		result := VerifySignatureForRegularNode(verification, leaderEOA)
		assert.True(t, result)
	})

	t.Run("invalid signature - wrong private key", func(t *testing.T) {
		wrongSignature := SignData(regularEOA, regularPrivateKey)

		verification := Verification{
			EOAAddress: regularEOA,
			Signature:  wrongSignature,
		}

		result := VerifySignatureForRegularNode(verification, leaderEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature format", func(t *testing.T) {
		verification := Verification{
			EOAAddress: regularEOA,
			Signature:  []byte("invalid signature"),
		}

		result := VerifySignatureForRegularNode(verification, leaderEOA)
		assert.False(t, result)
	})
}

func TestSignDataAdditionalCoverage(t *testing.T) {
	t.Run("additional success path coverage", func(t *testing.T) {
		privateKey, err := crypto.GenerateKey()
		require.NoError(t, err)

		data := "test data for additional coverage"
		signature := SignData(data, privateKey)

		assert.NotNil(t, signature)
		assert.Greater(t, len(signature), 0)

		// Verify the signature is valid
		hash := crypto.Keccak256Hash([]byte(data))
		pubKey, err := crypto.SigToPub(hash.Bytes(), signature)
		require.NoError(t, err)

		expectedAddress := crypto.PubkeyToAddress(*pubKey)
		actualAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
		assert.Equal(t, expectedAddress, actualAddress)
	})
}

func TestComputeCvsEIP712TypedDataHash_Success(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs-value"))

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.NoError(t, err)
	assert.NotEqual(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_MissingContractAddress(t *testing.T) {
	os.Unsetenv("CONTRACT_ADDRESS")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CONTRACT_ADDRESS")
	assert.Equal(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_MissingChainID(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Unsetenv("CHAIN_ID")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CHAIN_ID")
	assert.Equal(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_InvalidChainID(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "invalid")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid chain ID")
	assert.Equal(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_DifferentInputs(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
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
			cvs:      [32]byte{1, 2, 3},
		},
		{
			name:     "Round 200, Trial 2",
			round:    big.NewInt(200),
			trialNum: big.NewInt(2),
			cvs:      [32]byte{4, 5, 6},
		},
		{
			name:     "Round 0, Trial 0",
			round:    big.NewInt(0),
			trialNum: big.NewInt(0),
			cvs:      [32]byte{0},
		},
		{
			name:     "Large round number",
			round:    big.NewInt(999999),
			trialNum: big.NewInt(100),
			cvs:      [32]byte{0xff, 0xff, 0xff},
		},
	}

	hashes := make(map[common.Hash]bool)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := ComputeCvsEIP712TypedDataHash(tt.round, tt.trialNum, tt.cvs)

			assert.NoError(t, err)
			assert.NotEqual(t, common.Hash{}, hash)

			assert.False(t, hashes[hash], "Different inputs should produce different hashes")
			hashes[hash] = true
		})
	}
}

func TestComputeCvsEIP712TypedDataHash_Deterministic(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("deterministic-test"))

	hash1, err1 := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)
	hash2, err2 := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, hash1, hash2, "Same inputs should produce same hash")
}

func TestComputeCvsEIP712TypedDataHash_DifferentChainIDs(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test"))

	chainIDs := []string{"1", "5", "11155111"} 
	hashes := make(map[common.Hash]bool)

	for _, chainID := range chainIDs {
		t.Run("ChainID_"+chainID, func(t *testing.T) {
			os.Setenv("CHAIN_ID", chainID)
			defer os.Unsetenv("CHAIN_ID")

			hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

			assert.NoError(t, err)
			assert.NotEqual(t, common.Hash{}, hash)

			// Verify different chain IDs produce different hashes
			assert.False(t, hashes[hash], "Different chain IDs should produce different hashes")
			hashes[hash] = true
		})
	}
}

func TestComputeCvsEIP712TypedDataHash_ContractAddressWithoutPrefix(t *testing.T) {
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

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
			defer os.Unsetenv("CONTRACT_ADDRESS")

			hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

			assert.NoError(t, err)
			assert.NotEqual(t, common.Hash{}, hash)
		})
	}
}

func TestComputeCvsEIP712TypedDataHash_EmptyCVS(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte // All zeros

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.NoError(t, err)
	assert.NotEqual(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_FullCVS(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	// Fill all 32 bytes
	for i := 0; i < 32; i++ {
		cvs[i] = byte(i)
	}

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.NoError(t, err)
	assert.NotEqual(t, common.Hash{}, hash)
}

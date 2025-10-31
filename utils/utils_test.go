package utils

import (
	"testing"

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
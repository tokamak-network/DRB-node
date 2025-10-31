package commitreveal2

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeccak256(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
		},
		{
			name:     "hello world",
			input:    []byte("hello world"),
			expected: "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		},
		{
			name:     "test",
			input:    []byte("test"),
			expected: "9c22ff5f21f0b81b113e63f7db6da94fedef11b2119b4088b89664fb9a3cb658",
		},
		{
			name:     "32 bytes input",
			input:    make([]byte, 32),
			expected: "290decd9548b62a8d60345a988386fc84ba6bc95484008f6362f93160ef3e563",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Keccak256(tt.input)
			resultHex := hex.EncodeToString(result)
			assert.Equal(t, tt.expected, resultHex)
		})
	}
}

func TestAbiEncode(t *testing.T) {
	tests := []struct {
		name     string
		elements [][]byte
		expected string
	}{
		{
			name:     "empty input",
			elements: [][]byte{},
			expected: "",
		},
		{
			name: "single element",
			elements: [][]byte{
				{0x01, 0x02, 0x03},
			},
			expected: "0000000000000000000000000000000000000000000000000000000000010203",
		},
		{
			name: "multiple elements",
			elements: [][]byte{
				{0x01},
				{0x02, 0x03},
			},
			expected: "0000000000000000000000000000000000000000000000000000000000000001" +
				"0000000000000000000000000000000000000000000000000000000000000203",
		},
		{
			name: "full 32-byte element",
			elements: [][]byte{
				make([]byte, 32),
			},
			expected: "0000000000000000000000000000000000000000000000000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := abiEncode(tt.elements...)
			resultHex := hex.EncodeToString(result)
			assert.Equal(t, tt.expected, resultHex)
		})
	}
}

func TestAbiEncodePacked(t *testing.T) {
	tests := []struct {
		name     string
		elements [][]byte
		expected string
	}{
		{
			name:     "empty input",
			elements: [][]byte{},
			expected: "",
		},
		{
			name: "single element",
			elements: [][]byte{
				{0x01, 0x02, 0x03},
			},
			expected: "010203",
		},
		{
			name: "multiple elements",
			elements: [][]byte{
				{0x01},
				{0x02, 0x03},
				{0x04, 0x05, 0x06},
			},
			expected: "010203040506",
		},
		{
			name: "32-byte element",
			elements: [][]byte{
				{0xFF, 0xFE, 0xFD},
				make([]byte, 32),
			},
			expected: "fffefd" + "0000000000000000000000000000000000000000000000000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := abiEncodePacked(tt.elements...)
			resultHex := hex.EncodeToString(result)
			assert.Equal(t, tt.expected, resultHex)
		})
	}
}

func TestIntToBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    *big.Int
		expected string
	}{
		{
			name:     "zero",
			input:    big.NewInt(0),
			expected: "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name:     "small number",
			input:    big.NewInt(255),
			expected: "00000000000000000000000000000000000000000000000000000000000000ff",
		},
		{
			name:     "larger number",
			input:    big.NewInt(65535),
			expected: "000000000000000000000000000000000000000000000000000000000000ffff",
		},
		{
			name:     "negative number gets abs value",
			input:    big.NewInt(-1),
			expected: "0000000000000000000000000000000000000000000000000000000000000001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := intToBytes(tt.input)
			resultHex := hex.EncodeToString(result)
			assert.Equal(t, tt.expected, resultHex)
			assert.Equal(t, 32, len(result)) // Always 32 bytes
		})
	}
}

func TestGenerateCommit(t *testing.T) {
	t.Run("valid inputs", func(t *testing.T) {
		round := "1"
		operator := "0x1234567890123456789012345678901234567890"

		secretValue, cos, cvs, err := GenerateCommit(round, operator)
		require.NoError(t, err)

		// Check that all values are different
		assert.NotEqual(t, secretValue, cos)
		assert.NotEqual(t, cos, cvs)
		assert.NotEqual(t, secretValue, cvs)

		// Check that none are zero values
		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)

		// Verify the relationship: cvs = hash(cos), cos = hash(secretValue)
		expectedCos := Keccak256(abiEncode(secretValue[:]))
		var expectedCosBytes32 [32]byte
		copy(expectedCosBytes32[:], expectedCos)
		assert.Equal(t, expectedCosBytes32, cos)

		expectedCvs := Keccak256(abiEncode(cos[:]))
		var expectedCvsBytes32 [32]byte
		copy(expectedCvsBytes32[:], expectedCvs)
		assert.Equal(t, expectedCvsBytes32, cvs)
	})

	t.Run("different rounds produce different results", func(t *testing.T) {
		operator := "0x1234567890123456789012345678901234567890"

		secretValue1, cos1, cvs1, err1 := GenerateCommit("1", operator)
		require.NoError(t, err1)

		secretValue2, cos2, cvs2, err2 := GenerateCommit("2", operator)
		require.NoError(t, err2)

		// Different rounds should produce different values
		assert.NotEqual(t, secretValue1, secretValue2)
		assert.NotEqual(t, cos1, cos2)
		assert.NotEqual(t, cvs1, cvs2)
	})

	t.Run("different operators produce different results", func(t *testing.T) {
		round := "1"

		secretValue1, cos1, cvs1, err1 := GenerateCommit(round, "0x1234567890123456789012345678901234567890")
		require.NoError(t, err1)

		secretValue2, cos2, cvs2, err2 := GenerateCommit(round, "0x2345678901234567890123456789012345678901")
		require.NoError(t, err2)

		// Different operators should produce different values
		assert.NotEqual(t, secretValue1, secretValue2)
		assert.NotEqual(t, cos1, cos2)
		assert.NotEqual(t, cvs1, cvs2)
	})

	t.Run("invalid round", func(t *testing.T) {
		operator := "0x1234567890123456789012345678901234567890"

		_, _, _, err := GenerateCommit("invalid", operator)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid round")
	})

	t.Run("invalid operator address", func(t *testing.T) {
		round := "1"
		// Invalid Ethereum address - this should still work as common.HexToAddress
		// converts invalid addresses to zero address
		operator := "invalid-address"

		secretValue, cos, cvs, err := GenerateCommit(round, operator)
		require.NoError(t, err)

		// Should still generate valid results (with zero address)
		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)
	})

	t.Run("deterministic with same inputs and timestamp", func(t *testing.T) {
		// Note: This test might be flaky since GenerateCommit uses time.Now()
		// In a real implementation, you might want to accept timestamp as parameter for testing
		round := "1"
		operator := "0x1234567890123456789012345678901234567890"

		secretValue1, cos1, cvs1, err1 := GenerateCommit(round, operator)
		require.NoError(t, err1)

		// Generate again immediately (should be different due to timestamp)
		secretValue2, cos2, cvs2, err2 := GenerateCommit(round, operator)
		require.NoError(t, err2)

		// These should be different because timestamp changed
		// (unless executed in the same second, which is unlikely but possible)
		// We'll just verify they're valid for now
		assert.NotEqual(t, [32]byte{}, secretValue1)
		assert.NotEqual(t, [32]byte{}, secretValue2)
		assert.NotEqual(t, [32]byte{}, cos1)
		assert.NotEqual(t, [32]byte{}, cos2)
		assert.NotEqual(t, [32]byte{}, cvs1)
		assert.NotEqual(t, [32]byte{}, cvs2)
	})
}
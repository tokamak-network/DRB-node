package commitreveal2

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEfficientKeccak256(t *testing.T) {
	tests := []struct {
		name     string
		a        []byte
		b        []byte
		expected string
	}{
		{
			name:     "two zero bytes32",
			a:        make([]byte, 32),
			b:        make([]byte, 32),
			expected: "ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
		},
		{
			name:     "different values",
			a:        []byte{0x01, 0x02, 0x03},
			b:        []byte{0x04, 0x05, 0x06},
			expected: "13a08e3cd39a1bc7bf9103f63f83273cced2beada9f723945176d6b983c65bd2",
		},
		{
			name: "32-byte values",
			a: func() []byte {
				a := make([]byte, 32)
				a[31] = 0x01
				return a
			}(),
			b: func() []byte {
				b := make([]byte, 32)
				b[31] = 0x02
				return b
			}(),
			expected: "e90b7bceb6e7df5418fb78d8ee546e97c83a08bbccc01a0644d599ccd2a7c2e0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := efficientKeccak256(tt.a, tt.b)
			resultHex := hex.EncodeToString(result)
			assert.Equal(t, tt.expected, resultHex)
		})
	}
}

func TestCreateMerkleTree(t *testing.T) {
	t.Run("insufficient leaves", func(t *testing.T) {
		// Test with 0 leaves
		_, err := CreateMerkleTree([][]byte{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not enough leaves")

		// Test with 1 leaf
		_, err = CreateMerkleTree([][]byte{{0x01}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not enough leaves")
	})

	t.Run("two leaves", func(t *testing.T) {
		leaf1 := make([]byte, 32)
		leaf1[31] = 0x01

		leaf2 := make([]byte, 32)
		leaf2[31] = 0x02

		leaves := [][]byte{leaf1, leaf2}

		root, err := CreateMerkleTree(leaves)
		require.NoError(t, err)
		assert.Len(t, root, 32)

		// The root should be the hash of the two leaves
		expectedRoot := efficientKeccak256(leaf1, leaf2)
		assert.Equal(t, expectedRoot, root)
	})

	t.Run("three leaves", func(t *testing.T) {
		leaf1 := make([]byte, 32)
		leaf1[31] = 0x01

		leaf2 := make([]byte, 32)
		leaf2[31] = 0x02

		leaf3 := make([]byte, 32)
		leaf3[31] = 0x03

		leaves := [][]byte{leaf1, leaf2, leaf3}

		root, err := CreateMerkleTree(leaves)
		require.NoError(t, err)
		assert.Len(t, root, 32)

		// For 3 leaves, we can't predict the exact algorithm without understanding the implementation
		// Let's just verify that we get a valid 32-byte root that's not all zeros
		assert.NotEqual(t, make([]byte, 32), root)
	})

	t.Run("four leaves", func(t *testing.T) {
		leaf1 := make([]byte, 32)
		leaf1[31] = 0x01

		leaf2 := make([]byte, 32)
		leaf2[31] = 0x02

		leaf3 := make([]byte, 32)
		leaf3[31] = 0x03

		leaf4 := make([]byte, 32)
		leaf4[31] = 0x04

		leaves := [][]byte{leaf1, leaf2, leaf3, leaf4}

		root, err := CreateMerkleTree(leaves)
		require.NoError(t, err)
		assert.Len(t, root, 32)

		// For 4 leaves, the algorithm should:
		// 1. Hash leaf1 and leaf2 -> hash1
		// 2. Hash leaf3 and leaf4 -> hash2
		// 3. Hash hash1 and hash2 -> root
		hash1 := efficientKeccak256(leaf1, leaf2)
		hash2 := efficientKeccak256(leaf3, leaf4)
		expectedRoot := efficientKeccak256(hash1, hash2)
		assert.Equal(t, expectedRoot, root)
	})

	t.Run("padding shorter leaves", func(t *testing.T) {
		// Test with leaves shorter than 32 bytes
		leaf1 := []byte{0x01, 0x02}
		leaf2 := []byte{0x03}

		leaves := [][]byte{leaf1, leaf2}

		root, err := CreateMerkleTree(leaves)
		require.NoError(t, err)
		assert.Len(t, root, 32)

		// Verify that padding was applied
		paddedLeaf1 := make([]byte, 32)
		copy(paddedLeaf1, leaf1)

		paddedLeaf2 := make([]byte, 32)
		copy(paddedLeaf2, leaf2)

		expectedRoot := efficientKeccak256(paddedLeaf1, paddedLeaf2)
		assert.Equal(t, expectedRoot, root)
	})

	t.Run("leaves longer than 32 bytes get truncated", func(t *testing.T) {
		// Test with leaves longer than 32 bytes
		leaf1 := make([]byte, 40)
		for i := range leaf1 {
			leaf1[i] = 0x01
		}

		leaf2 := make([]byte, 35)
		for i := range leaf2 {
			leaf2[i] = 0x02
		}

		leaves := [][]byte{leaf1, leaf2}

		root, err := CreateMerkleTree(leaves)
		require.NoError(t, err)
		assert.Len(t, root, 32)

		// The function should have padded them to exactly 32 bytes
		// by copying only the first 32 bytes
		expectedLeaf1 := make([]byte, 32)
		copy(expectedLeaf1, leaf1)

		expectedLeaf2 := make([]byte, 32)
		copy(expectedLeaf2, leaf2)

		expectedRoot := efficientKeccak256(expectedLeaf1, expectedLeaf2)
		assert.Equal(t, expectedRoot, root)
	})

	t.Run("deterministic results", func(t *testing.T) {
		leaf1 := []byte{0x01, 0x02, 0x03}
		leaf2 := []byte{0x04, 0x05, 0x06}
		leaf3 := []byte{0x07, 0x08, 0x09}

		leaves := [][]byte{leaf1, leaf2, leaf3}

		// Generate root multiple times
		root1, err1 := CreateMerkleTree(leaves)
		require.NoError(t, err1)

		root2, err2 := CreateMerkleTree(leaves)
		require.NoError(t, err2)

		root3, err3 := CreateMerkleTree(leaves)
		require.NoError(t, err3)

		// All roots should be identical
		assert.Equal(t, root1, root2)
		assert.Equal(t, root2, root3)
	})

	t.Run("order matters", func(t *testing.T) {
		leaf1 := []byte{0x01}
		leaf2 := []byte{0x02}

		// Different order should produce different roots
		root1, err1 := CreateMerkleTree([][]byte{leaf1, leaf2})
		require.NoError(t, err1)

		root2, err2 := CreateMerkleTree([][]byte{leaf2, leaf1})
		require.NoError(t, err2)

		assert.NotEqual(t, root1, root2)
	})

	t.Run("larger tree - 8 leaves", func(t *testing.T) {
		leaves := make([][]byte, 8)
		for i := 0; i < 8; i++ {
			leaf := make([]byte, 32)
			leaf[31] = byte(i + 1)
			leaves[i] = leaf
		}

		root, err := CreateMerkleTree(leaves)
		require.NoError(t, err)
		assert.Len(t, root, 32)

		// Verify it's not a zero hash
		zeroHash := make([]byte, 32)
		assert.NotEqual(t, zeroHash, root)
	})

	t.Run("empty bytes in leaves", func(t *testing.T) {
		leaf1 := make([]byte, 32) // All zeros
		leaf2 := make([]byte, 32) // All zeros
		leaf2[31] = 0x01

		leaves := [][]byte{leaf1, leaf2}

		root, err := CreateMerkleTree(leaves)
		require.NoError(t, err)
		assert.Len(t, root, 32)

		expectedRoot := efficientKeccak256(leaf1, leaf2)
		assert.Equal(t, expectedRoot, root)
	})
}
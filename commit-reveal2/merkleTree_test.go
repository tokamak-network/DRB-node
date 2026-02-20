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


		hash0 := efficientKeccak256(leaf1, leaf2)
		expectedRoot := efficientKeccak256(leaf3, hash0)
		assert.Equal(t, expectedRoot, root)
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

		paddedLeaf1 := make([]byte, 32)
		copy(paddedLeaf1, leaf1)
		paddedLeaf2 := make([]byte, 32)
		copy(paddedLeaf2, leaf2)
		paddedLeaf3 := make([]byte, 32)
		copy(paddedLeaf3, leaf3)
		hash0 := efficientKeccak256(paddedLeaf1, paddedLeaf2)
		expectedRoot := efficientKeccak256(paddedLeaf3, hash0)
		assert.Equal(t, expectedRoot, root1)
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

		hash0 := efficientKeccak256(leaves[0], leaves[1])
		hash1 := efficientKeccak256(leaves[2], leaves[3])
		hash2 := efficientKeccak256(leaves[4], leaves[5])
		hash3 := efficientKeccak256(leaves[6], leaves[7])
		hash4 := efficientKeccak256(hash0, hash1)
		hash5 := efficientKeccak256(hash2, hash3)
		expectedRoot := efficientKeccak256(hash4, hash5)
		assert.Equal(t, expectedRoot, root)
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

func TestCreateMerkleTreeEdgeCases(t *testing.T) {
	t.Run("leaves with all same values", func(t *testing.T) {
		sameValue := make([]byte, 32)
		sameValue[31] = 0x42

		leaves := [][]byte{
			sameValue,
			sameValue,
			sameValue,
			sameValue,
		}

		root, err := CreateMerkleTree(leaves)
		assert.NoError(t, err)
		assert.Len(t, root, 32)
		assert.NotEqual(t, make([]byte, 32), root)

		hash1 := efficientKeccak256(leaves[0], leaves[1])
		hash2 := efficientKeccak256(leaves[2], leaves[3])
		expectedRoot := efficientKeccak256(hash1, hash2)
		assert.Equal(t, expectedRoot, root)
	})

	t.Run("many leaves", func(t *testing.T) {
		numLeaves := 16
		leaves := make([][]byte, numLeaves)

		for i := 0; i < numLeaves; i++ {
			leaf := make([]byte, 32)
			leaf[31] = byte(i)
			leaves[i] = leaf
		}

		root, err := CreateMerkleTree(leaves)
		assert.NoError(t, err)
		assert.Len(t, root, 32)
		assert.NotEqual(t, make([]byte, 32), root)

		hash0 := efficientKeccak256(leaves[0], leaves[1])
		hash1 := efficientKeccak256(leaves[2], leaves[3])
		hash2 := efficientKeccak256(leaves[4], leaves[5])
		hash3 := efficientKeccak256(leaves[6], leaves[7])
		hash4 := efficientKeccak256(leaves[8], leaves[9])
		hash5 := efficientKeccak256(leaves[10], leaves[11])
		hash6 := efficientKeccak256(leaves[12], leaves[13])
		hash7 := efficientKeccak256(leaves[14], leaves[15])

		hash8 := efficientKeccak256(hash0, hash1)
		hash9 := efficientKeccak256(hash2, hash3)
		hash10 := efficientKeccak256(hash4, hash5)
		hash11 := efficientKeccak256(hash6, hash7)
	
		hash12 := efficientKeccak256(hash8, hash9)
		hash13 := efficientKeccak256(hash10, hash11)
		// Level 4: Final root
		expectedRoot := efficientKeccak256(hash12, hash13)
		assert.Equal(t, expectedRoot, root)
	})

	t.Run("thirty-two leaves", func(t *testing.T) {
		numLeaves := 32
		leaves := make([][]byte, numLeaves)

		for i := 0; i < numLeaves; i++ {
			leaf := make([]byte, 32)
			leaf[31] = byte(i)
			leaves[i] = leaf
		}

		root, err := CreateMerkleTree(leaves)
		require.NoError(t, err)
		assert.Len(t, root, 32)
		assert.NotEqual(t, make([]byte, 32), root)

		h0 := efficientKeccak256(leaves[0], leaves[1])
		h1 := efficientKeccak256(leaves[2], leaves[3])
		h2 := efficientKeccak256(leaves[4], leaves[5])
		h3 := efficientKeccak256(leaves[6], leaves[7])
		h4 := efficientKeccak256(leaves[8], leaves[9])
		h5 := efficientKeccak256(leaves[10], leaves[11])
		h6 := efficientKeccak256(leaves[12], leaves[13])
		h7 := efficientKeccak256(leaves[14], leaves[15])
		h8 := efficientKeccak256(leaves[16], leaves[17])
		h9 := efficientKeccak256(leaves[18], leaves[19])
		h10 := efficientKeccak256(leaves[20], leaves[21])
		h11 := efficientKeccak256(leaves[22], leaves[23])
		h12 := efficientKeccak256(leaves[24], leaves[25])
		h13 := efficientKeccak256(leaves[26], leaves[27])
		h14 := efficientKeccak256(leaves[28], leaves[29])
		h15 := efficientKeccak256(leaves[30], leaves[31])
		// Level 2: Pair level 1 hashes (8 hashes)
		h16 := efficientKeccak256(h0, h1)
		h17 := efficientKeccak256(h2, h3)
		h18 := efficientKeccak256(h4, h5)
		h19 := efficientKeccak256(h6, h7)
		h20 := efficientKeccak256(h8, h9)
		h21 := efficientKeccak256(h10, h11)
		h22 := efficientKeccak256(h12, h13)
		h23 := efficientKeccak256(h14, h15)
		// Level 3: Pair level 2 hashes (4 hashes)
		h24 := efficientKeccak256(h16, h17)
		h25 := efficientKeccak256(h18, h19)
		h26 := efficientKeccak256(h20, h21)
		h27 := efficientKeccak256(h22, h23)
		// Level 4: Pair level 3 hashes (2 hashes)
		h28 := efficientKeccak256(h24, h25)
		h29 := efficientKeccak256(h26, h27)
		// Level 5: Final root
		expectedRoot := efficientKeccak256(h28, h29)
		assert.Equal(t, expectedRoot, root)
	})

	t.Run("leaves with nil bytes", func(t *testing.T) {
		leaves := [][]byte{
			nil,
			{0x01},
			nil,
			{0x02},
		}

		root, err := CreateMerkleTree(leaves)
		assert.NoError(t, err)
		assert.Len(t, root, 32)
	})

	t.Run("extremely long leaves", func(t *testing.T) {
		longLeaf1 := make([]byte, 1000)
		longLeaf2 := make([]byte, 500)

		for i := range longLeaf1 {
			longLeaf1[i] = 0xAA
		}
		for i := range longLeaf2 {
			longLeaf2[i] = 0xBB
		}

		leaves := [][]byte{longLeaf1, longLeaf2}

		root, err := CreateMerkleTree(leaves)
		assert.NoError(t, err)
		assert.Len(t, root, 32)
	})
}

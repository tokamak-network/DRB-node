package commitreveal2

import (
	"fmt"

	"golang.org/x/crypto/sha3"
)

// CreateMerkleTree creates a Merkle tree root from the provided leaves (similar to Solidity behavior)
// It now returns the Merkle root as bytes32 instead of a string
func CREATE_MERKLE_TREE(leaves [][]byte) ([]byte, error) {
	// Ensure that each leaf is 32 bytes long (padding if necessary)
	for i, leaf := range leaves {
		// If the leaf is not 32 bytes, pad it to the right size
		if len(leaf) != 32 {
			paddedLeaf := make([]byte, 32)
			copy(paddedLeaf, leaf)
			leaves[i] = paddedLeaf
		}
	}

	// Continue hashing the pairs of leaves until one root remains
	for len(leaves) > 1 {
		var newHashes [][]byte
		// Hash pairs of leaves (or previously calculated hashes)
		for i := 0; i < len(leaves)-1; i += 2 {
			// Efficiently hash pairs
			a := leaves[i]
			b := leaves[i+1]
			newHashes = append(newHashes, _efficientKeccak256(a, b))
		}
		// Handle odd number of elements, pair the last element with itself
		if len(leaves)%2 != 0 {
			last := leaves[len(leaves)-1]
			newHashes = append(newHashes, _efficientKeccak256(last, last))
		}
		leaves = newHashes
	}

	// The remaining single hash is the Merkle root (as bytes32)
	merkleRoot := leaves[0] // This is the bytes32 Merkle root

	// Print the Merkle root in bytes32 format
	fmt.Printf("Merkle Root: 0x%x\n", merkleRoot)

	return merkleRoot, nil
}

// _efficientKeccak256 hashes two bytes32 together using Keccak-256 (mimicking Solidity's assembly approach)
func _efficientKeccak256(a, b []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(a)
	hash.Write(b)
	return hash.Sum(nil)
}

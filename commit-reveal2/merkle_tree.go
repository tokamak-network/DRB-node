package commitreveal2

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/sha3"
)

// CreateMerkleTree generates a Merkle root using the provided leaves.
// The leaves are expected to be byte slices of length 32 (bytes32 in Solidity).
func CreateMerkleTree(leaves [][]byte) ([]byte, error) {
	leavesLen := len(leaves)

	// Ensure there are at least two leaves
	if leavesLen < 2 {
		return nil, errors.New("not enough leaves to generate a Merkle root")
	}

	// Ensure all leaves are padded to 32 bytes (mimicking bytes32 in Solidity)
	for i := range leaves {
		if len(leaves[i]) != 32 {
			paddedLeaf := make([]byte, 32)
			copy(paddedLeaf, leaves[i])
			leaves[i] = paddedLeaf
		}
	}

	// Calculate the total number of hashes needed
	hashCount := leavesLen - 1
	hashes := make([][]byte, hashCount)

	leafPos := 0
	hashPos := 0

	for i := 0; i < hashCount; i++ {
		var a, b []byte

		// Assign 'a' and 'b' based on the current position in leaves or hashes
		if leafPos < leavesLen {
			a = leaves[leafPos]
			leafPos++
		} else {
			a = hashes[hashPos]
			hashPos++
		}

		if leafPos < leavesLen {
			b = leaves[leafPos]
			leafPos++
		} else {
			b = hashes[hashPos]
			hashPos++
		}

		// Compute the hash for the pair (a, b)
		hashes[i] = efficientKeccak256(a, b)
	}

	// The last element in the hashes array is the Merkle root
	merkleRoot := hashes[hashCount-1]
	fmt.Printf("Merkle Root: 0x%x\n", merkleRoot)

	return merkleRoot, nil
}

// efficientKeccak256 hashes two bytes32 using Keccak256.
// Mimics Solidity's efficientKeccak256 function.
func efficientKeccak256(a, b []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(a)
	hash.Write(b)
	return hash.Sum(nil)
}

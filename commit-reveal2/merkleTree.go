package commitreveal2

import (
	"encoding/hex"

	"golang.org/x/crypto/sha3"
)

// CreateMerkleTree creates a Merkle tree root from the provided leaves.
func CREATE_MERKLE_TREE(leaves []string) (string, error) {
	// Convert leaves (strings) to bytes32 equivalents
	var leafHashes [][]byte
	for _, leaf := range leaves {
		// Convert each leaf string into bytes and hash it using Keccak-256 (bytes32 in Solidity)
		hash := sha3.New256()
		hash.Write([]byte(leaf))
		leafHashes = append(leafHashes, hash.Sum(nil))
	}

	// Continue hashing the pairs of leaves until one root remains
	for len(leafHashes) > 1 {
		var newHashes [][]byte
		// Hash pairs of leaves (or previously calculated hashes)
		for i := 0; i < len(leafHashes); i += 2 {
			// If there is an odd number of leaves, the last leaf is paired with itself
			var a, b []byte
			if i+1 < len(leafHashes) {
				a = leafHashes[i]
				b = leafHashes[i+1]
			} else {
				// Pair last element with itself if the count is odd
				a = leafHashes[i]
				b = leafHashes[i]
			}

			// Efficiently hash the pair (this is analogous to _efficientKeccak256 in Solidity)
			hash := sha3.New256()
			hash.Write(a)
			hash.Write(b)
			newHashes = append(newHashes, hash.Sum(nil))
		}
		leafHashes = newHashes
	}

	// The remaining single hash is the Merkle root
	// Return the root as a hex string
	merkleRoot := hex.EncodeToString(leafHashes[0])
	return merkleRoot, nil
}
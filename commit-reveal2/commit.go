package commitreveal2

import (
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/sha3"
)

// GenerateCommit generates the secret value, cos, and cvs for a single regular node.
func GENERATE_COMMIT() (string, string, string, error) {
	// Example: Generating commit values for a single operator (no arrays)
	// Current timestamp to simulate block timestamp in Solidity (since block timestamp is not available in Go)
	timestamp := time.Now().Unix()

	// Generate secret value using keccak256 (SHA3)
	secretValue := keccak256([]byte(fmt.Sprintf("%d%d%d", 0, timestamp, 0)))

	// Generate cos by hashing secretValue
	cos := keccak256(secretValue)

	// Generate cvs by hashing cos
	cvs := keccak256(cos)

	// Convert the values to hex strings for easier display
	secretValueHex := hex.EncodeToString(secretValue)
	cosHex := hex.EncodeToString(cos)
	cvsHex := hex.EncodeToString(cvs)

	// Return the hex values
	return secretValueHex, cosHex, cvsHex, nil
}

// keccak256 performs a Keccak-256 hash on the input data and returns the result.
func keccak256(data []byte) []byte {
	hash := sha3.New256()
	hash.Write(data)
	return hash.Sum(nil)
}

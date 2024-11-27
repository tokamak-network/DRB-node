package commitreveal2

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tokamak-network/DRB-node/utils"
	"golang.org/x/crypto/sha3"
)

// GenerateCommit generates the secret value, cos, and cvs for a single regular node.
func GENERATE_COMMIT(round string, operator string) ([32]byte, [32]byte, [32]byte, error) {
	// Current timestamp to simulate block timestamp in Solidity
	timestamp := time.Now().Unix()

	// Generate secret value using keccak256 (SHA3) for the given round, operator, and timestamp
	secretValue := keccak256([]byte(fmt.Sprintf("%d%d%d", round, operator, timestamp)))

	// Generate cos by hashing secretValue
	cos := keccak256(secretValue)

	// Generate cvs by hashing cos
	cvs := keccak256(cos)

	// Convert the results into [32]byte format (bytes32 in Solidity)
	var secretValueBytes32 [32]byte
	copy(secretValueBytes32[:], secretValue)

	var cosBytes32 [32]byte
	copy(cosBytes32[:], cos)

	var cvsBytes32 [32]byte
	copy(cvsBytes32[:], cvs)

	// Print the results in bytes32 (hexadecimal with 0x prefix) format
	fmt.Printf("Secret Value (bytes32): 0x%x\n", secretValueBytes32)
	fmt.Printf("COS (bytes32): 0x%x\n", cosBytes32)
	fmt.Printf("CVS (bytes32): 0x%x\n", cvsBytes32)

	return secretValueBytes32, cosBytes32, cvsBytes32, nil
}

// keccak256 performs a Keccak-256 hash on the input data and returns the result.
func keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

// SaveCommitData saves the commit data to a JSON file.
func SaveCommitData(commitData utils.CommitData) error {
	file, err := os.OpenFile("commit_data.json", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Encoding and writing commit data to file
	encoder := json.NewEncoder(file)
	err = encoder.Encode(commitData)
	if err != nil {
		return err
	}

	return nil
}

// SignCommitRequest signs the commit request with the EOA address and round.
func SignCommitRequest(round string, eoaAddress string) (string, error) {
	// This is a mock function, replace it with actual signing logic (e.g., using ECDSA or other methods)
	if eoaAddress == "" {
		return "", fmt.Errorf("EOA address cannot be empty")
	}

	// Just return a mock "signed" string for now
	signedRound := fmt.Sprintf("signed-%d-%s", round, eoaAddress)
	return signedRound, nil
}

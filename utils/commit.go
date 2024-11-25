package utils

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/sha3"
)

type CommitRequest struct {
	Round       string    `json:"round"`
	Cvs         string `json:"cvs"`
	EOAAddress  string `json:"eoa_address"`
	SignedRound string `json:"signed_round"`
}

// CommitData defines the structure for storing commit data for the regular node.
type CommitData struct {
	Round        string    `json:"round"`
	SecretValue  string `json:"secret_value"`
	Cos          string `json:"cos"`
	Cvs          string `json:"cvs"`
	SendToLeader bool   `json:"send_to_leader"`
}

// GenerateCommit generates the secret value, cos, and cvs for a single regular node.
func GENERATE_COMMIT(round string, eoaAddress string) (string, string, string, error) {
	// Example: Generating commit values for a single operator (no arrays)
	// Current timestamp to simulate block timestamp in Solidity (since block timestamp is not available in Go)
	timestamp := time.Now().Unix()

	// Generate secret value using keccak256 (SHA3)
	secretValue := keccak256([]byte(fmt.Sprintf("%d%d%s", round, timestamp, eoaAddress)))

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

// SaveCommitData saves the commit data to a JSON file.
func SaveCommitData(commitData CommitData) error {
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

package utils

import (
	"encoding/json"
	"fmt"
	"os"
)

const commitDataFile = "commits.json"

type CommitRequest struct {
	Round       string    `json:"round"`
	Cvs         [32]byte `json:"cvs"`
	EOAAddress  string `json:"eoa_address"`
	SignedRound string `json:"signed_round"`
}

type CosRequest struct {
    Round      string    `json:"round"`
    Cos        [32]byte `json:"cos"`
    EOAAddress string    `json:"eoa_address"`
	SignedRound string `json:"signed_round"`
}

// CommitData defines the structure for storing commit data for the regular node.
type CommitData struct {
	Round        string    `json:"round"`
	SecretValue  [32]byte `json:"secret_value"`
	Cos          [32]byte `json:"cos"`
	Cvs          [32]byte `json:"cvs"`
	SendToLeader bool   `json:"send_to_leader"`
	SendCosToLeader bool `json:"send_cos_to_leader"`
}

// loadCommitData loads the commit data for a given round number
func LOAD_COMMIT_DATA(roundNum string) (*CommitData, error) {
	// Open the commit file
	file, err := os.Open(commitDataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("commit not found") // No commits for this round
		}
		return nil, fmt.Errorf("error opening commit data file: %v", err)
	}
	defer file.Close()

	// Decode JSON data
	var commits map[string]CommitData
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&commits)
	if err != nil {
		return nil, fmt.Errorf("error decoding commit data: %v", err)
	}

	// Check if commit data exists for the given round
	commitData, exists := commits[roundNum]
	if !exists {
		return nil, fmt.Errorf("commit not found")
	}

	return &commitData, nil
}

// saveCommitData saves the commit data to a file
func SAVE_COMMIT_DATA(commitData CommitData) error {
	// Open the commit file (create if doesn't exist)
	file, err := os.OpenFile(commitDataFile, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error opening commit data file for writing: %v", err)
	}
	defer file.Close()

	// Read existing commits
	var commits map[string]CommitData
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&commits)
	if err != nil && err.Error() != "EOF" {
		return fmt.Errorf("error decoding existing commit data: %v", err)
	}

	// Add new commit data
	if commits == nil {
		commits = make(map[string]CommitData)
	}
	commits[commitData.Round] = commitData

	// Seek to the beginning of the file to overwrite it
	file.Seek(0, 0)

	// Encode and save the updated commit data
	encoder := json.NewEncoder(file)
	err = encoder.Encode(commits)
	if err != nil {
		return fmt.Errorf("error encoding commit data: %v", err)
	}

	return nil
}
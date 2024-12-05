package utils

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

const leaderCommitDataFile = "leader_commits.json"

// LeaderCommitData defines the structure for storing commit data in the leader node.
type LeaderCommitData struct {
	Round                 string            `json:"round"`
	EOAAddress            string            `json:"eoa_address"`
	Cvs                   [32]byte          `json:"cvs"`
	CvsHex                string            `json:"cvs_hex,omitempty"`
	Cos                   [32]byte          `json:"cos"`
	CosHex                string            `json:"cos_hex"`
	SecretValue           [32]byte          `json:"secret_value"`
	SecretValueHex        string            `json:"secret_value_hex"`
	Sign                  map[string]string `json:"sign"` // New field for v, r, s
	SubmitMerkleRootDone  bool              `json:"submit_merkle_root_done"`
	RandomNumberGenerated bool              `json:"random_number_generated"`
}

// LoadLeaderCommitData should load data from the file and return the commit data for a specific round and EOA
func LoadLeaderCommitData(roundNum, eoaAddress string) (*LeaderCommitData, error) {
    // Open the commit file
    file, err := os.Open(leaderCommitDataFile)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, fmt.Errorf("commit data not found")
        }
        return nil, fmt.Errorf("error opening leader commit data file: %v", err)
    }
    defer file.Close()

    // Decode JSON data
    var commits map[string]LeaderCommitData // Use LeaderCommitData
    decoder := json.NewDecoder(file)
    err = decoder.Decode(&commits)
    if err != nil {
        return nil, fmt.Errorf("error decoding leader commit data: %v", err)
    }

    // Construct the composite key: ROUND+EOA
    key := roundNum + "+" + eoaAddress
    log.Printf("Loading commit data for key: %s", key) // Debug log for the key

    // Check if commit data exists for the given key
    commitData, exists := commits[key]
    if !exists {
        return nil, fmt.Errorf("commit data not found for key: %s", key)
    }

    // Convert the CVS hex string back to a byte array
    if commitData.CvsHex != "" {
        cvsBytes, err := hex.DecodeString(commitData.CvsHex)
        if err != nil {
            return nil, fmt.Errorf("failed to decode CVS hex string: %v", err)
        }
        copy(commitData.Cvs[:], cvsBytes)
    }

    log.Printf("Loaded commit data for key: %s, CVS: %v", key, commitData.Cvs) // Debug log for loaded data

    return &commitData, nil
}


// SaveLeaderCommitData should save commit data in the correct format
func SaveLeaderCommitData(commitData LeaderCommitData) error {
	// Open the commit file (create if doesn't exist)
	file, err := os.OpenFile(leaderCommitDataFile, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error opening leader commit data file for writing: %v", err)
	}
	defer file.Close()

	// Read existing commits
	var commits map[string]LeaderCommitData
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&commits)
	if err != nil && err.Error() != "EOF" {
		return fmt.Errorf("error decoding existing leader commit data: %v", err)
	}

	// Construct the composite key: ROUND+EOA
	key := commitData.Round + "+" + commitData.EOAAddress

	// Add or update the commit data
	if commits == nil {
		commits = make(map[string]LeaderCommitData)
	}

	// If Cvs is present, also store the hex value
	if commitData.Cvs != [32]byte{} {
		commitData.CvsHex = hex.EncodeToString(commitData.Cvs[:]) // Convert Cvs byte array to hex string
	}

	commits[key] = commitData

	// Seek to the beginning of the file to overwrite it
	file.Seek(0, 0)

	// Encode and save the updated commit data
	encoder := json.NewEncoder(file)
	err = encoder.Encode(commits)
	if err != nil {
		return fmt.Errorf("error encoding leader commit data: %v", err)
	}

	log.Printf("Saved commit data for key: %s", key) // Debug log for commit save
	return nil
}

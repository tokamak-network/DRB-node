package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const commitDataFile = "commits.json"
const broadcastTrackerFile = "broadcast_trackers.json"

type CommitRequest struct {
	Round      string            `json:"round"`
	Cvs        [32]byte          `json:"cvs"`
	EOAAddress string            `json:"eoa_address"`
	Signature  []byte            `json:"signed_round"`
	Sign       map[string]string `json:"sign"` // New field for v, r, s
}

type CosRequest struct {
	Round      string   `json:"round"`
	Cos        [32]byte `json:"cos"`
	EOAAddress string   `json:"eoa_address"`
	Signature  []byte   `json:"signed_round"`
}

// CommitData defines the structure for storing commit data for the regular node.
type CommitData struct {
	Round           string            `json:"round"`
	SecretValue     [32]byte          `json:"secret_value"`
	Cos             [32]byte          `json:"cos"`
	Cvs             [32]byte          `json:"cvs"`
	SendToLeader    bool              `json:"send_to_leader"`
	SendCosToLeader bool              `json:"send_cos_to_leader"`
	Sign            map[string]string `json:"sign"` // New field for v, r, s
}

type PeerCommitData struct {
	Round       string   `json:"round"`
	SecretValue [32]byte `json:"secret_value"`
	Cos         [32]byte `json:"cos"`
	Cvs         [32]byte `json:"cvs"`
	EOAAddress  string   `json:"eoa_address"`
}

type Request struct {
	Round      string `json:"round"`
	EOAAddress string `json:"eoa_address"`
	Signature  []byte `json:"signed_round"`
}

// BroadcastMessage represents a message sent from leader to regular nodes
type BroadcastMessage struct {
	Round      string   `json:"round"`
	EOAAddress string   `json:"eoa_address"`
	MessageID  string   `json:"message_id"` // Unique identifier for this broadcast
	Type       string   `json:"type"`       // "cvs", "cos", or "secret"
	Data       [32]byte `json:"data"`       // The actual data being broadcast
}

// AcknowledgmentMessage represents acknowledgment from regular nodes to leader
type AcknowledgmentMessage struct {
	Round      string `json:"round"`
	EOAAddress string `json:"eoa_address"`
	MessageID  string `json:"message_id"` // References the broadcast message
	Type       string `json:"type"`       // "cvs", "cos", or "secret"
	Status     string `json:"status"`     // "received" or "error"
}

// BroadcastTracker tracks broadcast attempts and acknowledgments
type BroadcastTracker struct {
	Round        string          `json:"round"`
	EOAAddress   string          `json:"eoa_address"`
	Type         string          `json:"type"`
	MessageID    string          `json:"message_id"`
	Data         [32]byte        `json:"data"`
	Attempts     int             `json:"attempts"`
	MaxAttempts  int             `json:"max_attempts"`
	Acknowledged map[string]bool `json:"acknowledged"` // EOA -> acknowledged
	LastSent     int64           `json:"last_sent"`
	Timeout      int64           `json:"timeout"` // seconds
}

// LoadCommitData loads the commit data for a given round number
func LoadCommitData(roundNum string) (*CommitData, error) {
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
func SaveCommitData(commitData CommitData) error {
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

// SaveBroadcastTracker saves a broadcast tracker to file
func SaveBroadcastTracker(tracker BroadcastTracker) error {
	// Load existing trackers
	var trackers map[string]BroadcastTracker
	file, err := os.OpenFile(broadcastTrackerFile, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error opening broadcast tracker file: %v", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&trackers)
	if err != nil && err.Error() != "EOF" {
		return fmt.Errorf("error decoding existing broadcast trackers: %v", err)
	}

	if trackers == nil {
		trackers = make(map[string]BroadcastTracker)
	}

	// Create unique key for tracker
	key := fmt.Sprintf("%s_%s_%s_%s", tracker.Round, tracker.EOAAddress, tracker.Type, tracker.MessageID)
	trackers[key] = tracker

	// Seek to beginning and write
	file.Seek(0, 0)
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(trackers); err != nil {
		return fmt.Errorf("error encoding broadcast trackers: %v", err)
	}

	return nil
}

// LoadBroadcastTrackers loads all broadcast trackers from file
func LoadBroadcastTrackers() (map[string]BroadcastTracker, error) {
	file, err := os.Open(broadcastTrackerFile)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]BroadcastTracker), nil
		}
		return nil, fmt.Errorf("error opening broadcast tracker file: %v", err)
	}
	defer file.Close()

	var trackers map[string]BroadcastTracker
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&trackers); err != nil {
		if err.Error() == "EOF" {
			return make(map[string]BroadcastTracker), nil
		}
		return nil, fmt.Errorf("error decoding broadcast trackers: %v", err)
	}

	if trackers == nil {
		trackers = make(map[string]BroadcastTracker)
	}

	return trackers, nil
}

// UpdateBroadcastTracker updates an existing broadcast tracker
func UpdateBroadcastTracker(tracker BroadcastTracker) error {
	trackers, err := LoadBroadcastTrackers()
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s_%s_%s_%s", tracker.Round, tracker.EOAAddress, tracker.Type, tracker.MessageID)
	trackers[key] = tracker

	// Save updated trackers
	file, err := os.Create(broadcastTrackerFile)
	if err != nil {
		return fmt.Errorf("error creating broadcast tracker file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(trackers); err != nil {
		return fmt.Errorf("error encoding broadcast trackers: %v", err)
	}

	return nil
}

// GenerateMessageID creates a unique message ID for broadcasts
func GenerateMessageID(round, eoaAddress, messageType string) string {
	return fmt.Sprintf("%s_%s_%s_%d", round, eoaAddress, messageType, time.Now().UnixNano())
}

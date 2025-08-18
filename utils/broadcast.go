package utils

// BroadcastMessage represents a message sent from leader to regular nodes
type BroadcastMessage struct {
	Round      string   `json:"round"`
	TrialNum   string   `json:"trial_num"`
	EOAAddress string   `json:"eoa_address"`
	MessageID  string   `json:"message_id"` // Unique identifier for this broadcast
	Type       string   `json:"type"`       // "cvs", "cos", or "secret"
	Data       [32]byte `json:"data"`       // The actual data being broadcast
}

// AcknowledgmentMessage represents acknowledgment from regular nodes to leader
type AcknowledgmentMessage struct {
	Round      string `json:"round"`
	TrialNum   string `json:"trial_num"`
	EOAAddress string `json:"eoa_address"`
	MessageID  string `json:"message_id"` // References the broadcast message
	Type       string `json:"type"`       // "cvs", "cos", or "secret"
	Status     string `json:"status"`     // "received" or "error"
	Signature  []byte `json:"signature"`  // EOA signature for verification
}

// BroadcastTracker tracks broadcast attempts and acknowledgments
type BroadcastTracker struct {
	Round        string          `json:"round"`
	TrialNum     string          `json:"trial_num"`
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

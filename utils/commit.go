package utils

type CommitRequest struct {
	Round      string   `json:"round"`
	Cvs        [32]byte `json:"cvs"`
	EOAAddress string   `json:"eoa_address"`
	Signature  []byte   `json:"signed_round"`
	Sign       SignInfo `json:"sign"` // New field for v, r, s
}

type CosRequest struct {
	Round      string   `json:"round"`
	Cos        [32]byte `json:"cos"`
	EOAAddress string   `json:"eoa_address"`
	Signature  []byte   `json:"signed_round"`
}

// CommitData defines the structure for storing commit data for the regular node.
type CommitData struct {
	Round           string   `json:"round"`
	SecretValue     [32]byte `json:"secret_value"`
	Cos             [32]byte `json:"cos"`
	Cvs             [32]byte `json:"cvs"`
	SendToLeader    bool     `json:"send_to_leader"`
	SendCosToLeader bool     `json:"send_cos_to_leader"`
	Sign            SignInfo `json:"sign"` // New field for v, r, s
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

type SignInfo struct {
	R string `json:"r"`
	S string `json:"s"`
	V string `json:"v"`
}

func ConvertByteArray(b []byte) [32]byte {
	var arr [32]byte
	copy(arr[:], b)
	return arr
}

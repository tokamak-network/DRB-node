package utils

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

var CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
var CommittedNodesMu sync.RWMutex

// CommittedNodes thread-safe access functions
func GetCommittedNodes(uniqueKey string) (map[common.Address]LeaderCommitData, bool) {
	CommittedNodesMu.RLock()
	defer CommittedNodesMu.RUnlock()
	roundMap, exists := CommittedNodes[uniqueKey]
	if !exists {
		return nil, false
	}
	// Return a copy to prevent external modifications
	result := make(map[common.Address]LeaderCommitData)
	for k, v := range roundMap {
		result[k] = v
	}
	return result, true
}

func SetCommittedNodesRound(uniqueKey string, roundMap map[common.Address]LeaderCommitData) {
	CommittedNodesMu.Lock()
	defer CommittedNodesMu.Unlock()
	if CommittedNodes == nil {
		CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	}
	CommittedNodes[uniqueKey] = roundMap
}

func GetCommittedNodeData(uniqueKey string, eoa common.Address) (LeaderCommitData, bool) {
	CommittedNodesMu.RLock()
	defer CommittedNodesMu.RUnlock()
	roundMap, roundExists := CommittedNodes[uniqueKey]
	if !roundExists {
		return LeaderCommitData{}, false
	}
	data, exists := roundMap[eoa]
	return data, exists
}

func SetCommittedNodeData(uniqueKey string, eoa common.Address, data LeaderCommitData) {
	CommittedNodesMu.Lock()
	defer CommittedNodesMu.Unlock()
	if CommittedNodes == nil {
		CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	}
	if CommittedNodes[uniqueKey] == nil {
		CommittedNodes[uniqueKey] = make(map[common.Address]LeaderCommitData)
	}
	CommittedNodes[uniqueKey][eoa] = data
}

func EnsureCommittedNodesRoundExists(uniqueKey string) {
	CommittedNodesMu.Lock()
	defer CommittedNodesMu.Unlock()
	if CommittedNodes == nil {
		CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	}
	if CommittedNodes[uniqueKey] == nil {
		CommittedNodes[uniqueKey] = make(map[common.Address]LeaderCommitData)
	}
}

type CommitRequest struct {
	UniqueKey  string   `json:"unique_key"`
	Round      string   `json:"round"`
	TrialNum   string   `json:"trial_num"`
	Cvs        [32]byte `json:"cvs"`
	EOAAddress string   `json:"eoa_address"`
	Signature  []byte   `json:"signed_round"`
	Sign       SignInfo `json:"sign"` // New field for v, r, s
}

type CosRequest struct {
	UniqueKey  string   `json:"unique_key"`
	Round      string   `json:"round"`
	TrialNum   string   `json:"trial_num"`
	Cos        [32]byte `json:"cos"`
	EOAAddress string   `json:"eoa_address"`
	Signature  []byte   `json:"signed_round"`
}

// LeaderCommitData defines the structure for storing commit data in the leader node.
type LeaderCommitData struct {
	UniqueKey             string   `json:"unique_key"`
	Round                 string   `json:"round"`
	TrialNum              string   `json:"trial_num"`
	EOAAddress            string   `json:"eoa_address"`
	Cvs                   [32]byte `json:"cvs"`
	CvsHex                string   `json:"cvs_hex,omitempty"`
	Cos                   [32]byte `json:"cos"`
	CosHex                string   `json:"cos_hex"`
	SecretValue           [32]byte `json:"secret_value"`
	SecretValueHex        string   `json:"secret_value_hex"`
	Sign                  SignInfo `json:"sign"` // New field for v, r, s
	SubmitMerkleRootDone  bool     `json:"submit_merkle_root_done"`
	RandomNumberGenerated bool     `json:"random_number_generated"`
	CreatedAt             int64    `json:"created_at"` // Unix timestamp when this commit was created
}

// CommitData defines the structure for storing commit data for the regular node.
type CommitData struct {
	UniqueKey       string   `json:"unique_key"`
	Round           string   `json:"round"`
	TrialNum        string   `json:"trial_num"`
	SecretValue     [32]byte `json:"secret_value"`
	Cos             [32]byte `json:"cos"`
	Cvs             [32]byte `json:"cvs"`
	SendToLeader    bool     `json:"send_to_leader"`
	SendCosToLeader bool     `json:"send_cos_to_leader"`
	Sign            SignInfo `json:"sign"` // New field for v, r, s
}

type PeerCommitData struct {
	Round       string   `json:"round"`
	TrialNum    string   `json:"trial_num"`
	SecretValue [32]byte `json:"secret_value"`
	Cos         [32]byte `json:"cos"`
	Cvs         [32]byte `json:"cvs"`
	EOAAddress  string   `json:"eoa_address"`
}

type Request struct {
	Round      string `json:"round"`
	TrialNum   string `json:"trial_num"`
	EOAAddress string `json:"eoa_address"`
	Signature  []byte `json:"signed_round"`
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

// DeleteCommittedNodes deletes all data for a specific uniqueKey
func DeleteCommittedNodes(uniqueKey string) {
	CommittedNodesMu.Lock()
	defer CommittedNodesMu.Unlock()
	delete(CommittedNodes, uniqueKey)
}

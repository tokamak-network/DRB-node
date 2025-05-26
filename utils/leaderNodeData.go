package utils

import (
	"github.com/ethereum/go-ethereum/common"
)

const leaderCommitDataFile = "leader_commits.json"

var CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)

// LeaderCommitData defines the structure for storing commit data in the leader node.
type LeaderCommitData struct {
	Round                 string   `json:"round"`
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
}

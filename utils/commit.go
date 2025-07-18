package utils

import "github.com/ethereum/go-ethereum/common"

var CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)

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
	CreatedAt             int64    `json:"created_at"` // Unix timestamp when this commit was created
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

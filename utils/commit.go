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

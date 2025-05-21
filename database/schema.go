package database

type NodeInfo struct {
	IP         string `json:"ip"`
	Port       string `json:"port"`
	PeerID     string `json:"peer_id"`
	EoaAddress string `json:"eoa_address"`
	PrivateKey string `json:"private_key"`
}

type LeaderCommits struct {
	Round                 int      `json:"round"`
	EoaAddress            string   `json:"eoa_address"`
	Cvs                   [32]byte `json:"cvs"`
	CvsHex                string   `json:"cvs_hex"`
	Cos                   [32]byte `json:"cos"`
	CosHex                string   `json:"cos_hex"`
	SecretValue           [32]byte `json:"secret_value"`
	SecretValueHex        string   `json:"secret_value_hex"`
	Sign                  SignInfo `json:"sign"`
	SubmitMerkleRootDone  bool     `json:"submit_merkle_root_done"`
	RandomNumberGenerated bool     `json:"random_number_generated"`
}

type SignInfo struct {
	R string `json:"r"`
	S string `json:"s"`
	V int    `json:"v"`
}

type RevealOrders struct {
	OrderedNodes []string `json:"ordered_nodes"`
	RevealOrder  []int    `json:"reveal_order"`
	RV           string   `json:"rv"`
}

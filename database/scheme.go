package database

type NodeInfoScheme struct {
	ID         int    `pg:"id"`
	IP         string `pg:"ip,pk,notnull"`
	Port       string `pg:"port,notnull"`
	PeerID     string `pg:"peer_id,notnull"`
	EOAAddress string `pg:"eoa_address,notnull"`
}

type LeaderCommitScheme struct {
	ID                    int    `pg:"id"`
	Round                 string `pg:"round,pk,notnull"`
	TrialNum              string `pg:"trial_num,pk,notnull"`
	EOAAddress            string `pg:"eoa_address,pk,notnull"`
	Cvs                   []byte `pg:"cvs,type:bytea,notnull"`
	CvsHex                string `pg:"cvs_hex"`
	Cos                   []byte `pg:"cos,type:bytea,notnull"`
	CosHex                string `pg:"cos_hex"`
	SecretValue           []byte `pg:"secret_value,type:bytea,notnull"`
	SecretValueHex        string `pg:"secret_value_hex"`
	SignR                 string `pg:"sign_r"`
	SignS                 string `pg:"sign_s"`
	SignV                 string `pg:"sign_v"`
	SubmitMerkleRootDone  bool   `pg:"submit_merkle_root_done,notnull,use_zero"`
	RandomNumberGenerated bool   `pg:"random_number_generated,notnull,use_zero"`
	CreatedAt             int64  `pg:"created_at,notnull"`
}

type CommitDataScheme struct {
	ID              int    `pg:"id"`
	Round           string `pg:"round,pk,notnull"`
	TrialNum        string `pg:"trial_num,pk,notnull"`
	Cvs             []byte `pg:"cvs,type:bytea,notnull"`
	Cos             []byte `pg:"cos,type:bytea,notnull"`
	SecretValue     []byte `pg:"secret_value,type:bytea,notnull"`
	SignR           string `pg:"sign_r,notnull"`
	SignS           string `pg:"sign_s,notnull"`
	SignV           string `pg:"sign_v,notnull"`
	SendToLeader    bool   `pg:"send_to_leader,notnull"`
	SendCosToLeader bool   `pg:"send_cos_to_leader,notnull"`
}

type RevealOrderScheme struct {
	ID           int      `pg:"id"`
	Round        string   `pg:"round,pk,notnull"`
	TrialNum     string   `pg:"trial_num,pk,notnull"`
	OrderedNodes []string `pg:"ordered_nodes,array,notnull"`
	RevealOrder  []int    `pg:"reveal_order,array,notnull"`
	RV           string   `pg:"rv,notnull"`
}

type PeerCommitDataScheme struct {
	ID          int    `pg:"id"`
	Round       string `pg:"round,pk,notnull"`
	TrialNum    string `pg:"trial_num,pk,notnull"`
	EOAAddress  string `pg:"eoa_address,pk,notnull"`
	SecretValue []byte `pg:"secret_value,type:bytea"`
	Cos         []byte `pg:"cos,type:bytea"`
	Cvs         []byte `pg:"cvs,type:bytea"`
}

type BroadcastTrackerScheme struct {
	ID           int             `pg:"id,pk"`
	Round        string          `pg:"round,notnull"`
	TrialNum     string          `pg:"trial_num,notnull"`
	EOAAddress   string          `pg:"eoa_address,notnull"`
	Type         string          `pg:"type,notnull"`
	MessageID    string          `pg:"message_id,notnull"`
	Data         []byte          `pg:"data,type:bytea"`
	Attempts     int             `pg:"attempts"`
	MaxAttempts  int             `pg:"max_attempts"`
	Acknowledged map[string]bool `pg:"acknowledged,type:jsonb"`
	LastSent     int64           `pg:"last_sent"`
	Timeout      int64           `pg:"timeout"`
}

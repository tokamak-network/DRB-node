package utils

// NodeInfo structure to store information about the node
type NodeInfo struct {
	IP         string `json:"ip"`
	Port       string `json:"port"`
	PeerID     string `json:"peer_id"`
	EOAAddress string `json:"eoa_address"`
	PrivateKey []byte `json:"private_key"`
}

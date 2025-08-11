package utils

import (
	"crypto/ecdsa"
	"log"

	"github.com/ethereum/go-ethereum/crypto"
)

type RegistrationRequest struct {
	EOAAddress string `json:"eoa_address"`
	Signature  []byte `json:"signature"`
	PeerID     string `json:"peer_id"`
}

type SecretValueRequest struct {
	LeaderEoaAddress  string `json:"leader_eoa"`  // Sender's EOA address
	RegularEoaAddress string `json:"regular_eoa"` // EOA address of the node sending the request
	Round             string `json:"round"`       // Round number
	TrialNum          string `json:"trial_num"`   // Trial number
	Signature         []byte `json:"signature"`   // Signature
	SecretValue       []byte `json:"secret_value"`
	Order             int    `json:"order"` // Order in the reveal sequence
}

// VerifySignature checks if the signature matches the EOA addressp
func VerifySignature(req RegistrationRequest) bool {
	hash := crypto.Keccak256Hash([]byte(req.EOAAddress))
	pubKey, err := crypto.SigToPub(hash.Bytes(), req.Signature)
	if err != nil {
		log.Printf("Error recovering public key: %v", err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()

	return recoveredAddress == req.EOAAddress
}

// SignData signs the given data with the provided private key
func SignData(data string, privateKey *ecdsa.PrivateKey) []byte {
	hash := crypto.Keccak256Hash([]byte(data))
	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		log.Fatalf("Failed to sign data: %v", err)
	}
	return signature
}

func GetUniqueKey(round string, trialNum string) string {
	return round + "-" + trialNum
}

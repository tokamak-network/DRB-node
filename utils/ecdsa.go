package utils

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"

	"golang.org/x/crypto/sha3"
)

// Keccak256 hashing function
func keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

// SignData Function to sign data with ECDSA
func SignData(privateKey *ecdsa.PrivateKey, data []byte) (r, s *big.Int, err error) {
	hashed := keccak256(data)
	r, s, err = ecdsa.Sign(rand.Reader, privateKey, hashed)
	return r, s, err
}

// VerifySignature Function to verify ECDSA signature
func VerifySignature(publicKey *ecdsa.PublicKey, data []byte, r, s *big.Int) bool {
	hashed := keccak256(data)
	return ecdsa.Verify(publicKey, hashed, r, s)
}

// Public key recovery function
func recoverPublicKey(data, signature []byte) (*ecdsa.PublicKey, error) {
	hashed := keccak256(data)
	return crypto.SigToPub(hashed, signature)
}

// Function to get Ethereum address from public key
func getAddressFromPublicKey(pubKey *ecdsa.PublicKey) string {
	pubBytes := crypto.FromECDSAPub(pubKey)    // Convert public key to bytes
	hashed := keccak256(pubBytes[1:])          // Hash the public key
	address := hex.EncodeToString(hashed[12:]) // Take the last 20 bytes
	return "0x" + address                      // Convert to hex and add 0x prefix
}

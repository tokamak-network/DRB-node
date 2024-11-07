package main

import (
	"crypto/ecdsa"
	"encoding/hex"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"
)

// Helper function to generate a random ECDSA private key for testing
func generateTestPrivateKey() *ecdsa.PrivateKey {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		panic("Failed to generate test private key")
	}
	return privateKey
}

// TestKeccak256 ensures the keccak256 function hashes data correctly
func TestKeccak256(t *testing.T) {
	data := []byte("test data")
	expectedHash := keccak256(data)

	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(data)
	actualHash := hasher.Sum(nil)

	if hex.EncodeToString(actualHash) != hex.EncodeToString(expectedHash) {
		t.Errorf("keccak256 hash mismatch, got: %x, expected: %x", actualHash, expectedHash)
	}
}

// TestSignData ensures signing with ECDSA works and produces a valid signature
func TestSignData(t *testing.T) {
	privateKey := generateTestPrivateKey()
	data := []byte("test data")
	signature, err := signData(privateKey, data)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}

	// Verify the signature length (65 bytes for Ethereum ECDSA signatures)
	if len(signature) != 65 {
		t.Errorf("Expected signature length 65, got %d", len(signature))
	}
}

// TestRecoverPublicKey ensures that the recovered public key from a signature matches the original
func TestRecoverPublicKey(t *testing.T) {
	privateKey := generateTestPrivateKey()
	data := []byte("test data")
	signature, err := signData(privateKey, data)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}

	recoveredPubKey, err := recoverPublicKey(data, signature)
	if err != nil {
		t.Fatalf("Failed to recover public key: %v", err)
	}

	// Compare the recovered public key to the original
	originalPubKey := privateKey.PublicKey
	if hex.EncodeToString(crypto.FromECDSAPub(&originalPubKey)) != hex.EncodeToString(crypto.FromECDSAPub(recoveredPubKey)) {
		t.Error("Recovered public key does not match the original public key")
	}
}

// TestGetAddressFromPublicKey checks if the address derived from a public key is correct
func TestGetAddressFromPublicKey(t *testing.T) {
	privateKey := generateTestPrivateKey()
	address := getAddressFromPublicKey(&privateKey.PublicKey)

	// Derive expected address directly from the private key
	expectedAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	if address != expectedAddress {
		t.Errorf("Address mismatch, got: %s, expected: %s", address, expectedAddress)
	}
}

// TestLoadPrivateKeyFromEnv tests loading a private key from an .env file
func TestLoadPrivateKeyFromEnv(t *testing.T) {
	// Create a test .env file with a test private key
	privateKey := generateTestPrivateKey()
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))
	os.WriteFile("../.env", []byte("PRIVATE_KEY="+privateKeyHex), 0644)
	defer os.Remove("../.env")

	// Load the private key
	loadedPrivateKey, err := loadPrivateKeyFromEnv()
	if err != nil {
		t.Fatalf("Failed to load private key from .env file: %v", err)
	}

	// Compare the loaded private key to the original private key
	if hex.EncodeToString(crypto.FromECDSA(loadedPrivateKey)) != privateKeyHex {
		t.Error("Loaded private key does not match the original private key")
	}
}

// TestEndToEndSigningAndVerification performs an end-to-end test of signing, recovery, and verification
func TestEndToEndSigningAndVerification(t *testing.T) {
	privateKey := generateTestPrivateKey()
	data := []byte("192.168.0.1")

	// Sign the data
	signature, err := signData(privateKey, data)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}

	// Recover the public key from the signature
	recoveredPubKey, err := recoverPublicKey(data, signature)
	if err != nil {
		t.Fatalf("Failed to recover public key: %v", err)
	}

	// Compare addresses
	originalAddress := getAddressFromPublicKey(&privateKey.PublicKey)
	recoveredAddress := getAddressFromPublicKey(recoveredPubKey)
	if originalAddress != recoveredAddress {
		t.Errorf("Address recovery failed, got: %s, expected: %s", recoveredAddress, originalAddress)
	}
}

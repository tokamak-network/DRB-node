package main

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/sha3"
)

// Keccak256 hashing function
func keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

// ECDSA signing function
func signData(privateKey *ecdsa.PrivateKey, data []byte) ([]byte, error) {
	hashed := keccak256(data)
	return crypto.Sign(hashed, privateKey)
}

// Public key recovery function
func recoverPublicKey(data, signature []byte) (*ecdsa.PublicKey, error) {
	hashed := keccak256(data)
	return crypto.SigToPub(hashed, signature)
}

// Function to load the private key from .env file
func loadPrivateKeyFromEnv() (*ecdsa.PrivateKey, error) {
	err := godotenv.Load("../.env") // Specify the relative path to the .env file
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	privateKeyHex := os.Getenv("PRIVATE_KEY")
	if privateKeyHex == "" {
		return nil, fmt.Errorf("PRIVATE_KEY not set in .env file")
	}

	// Remove "0x" prefix if it exists
	privateKeyHex = strings.TrimPrefix(privateKeyHex, "0x")

	// Convert hex string to byte array
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key format: %v", err)
	}

	// Convert byte array to ECDSA private key
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to ECDSA private key: %v", err)
	}

	return privateKey, nil
}

// Function to get Ethereum address from public key
func getAddressFromPublicKey(pubKey *ecdsa.PublicKey) string {
	pubBytes := crypto.FromECDSAPub(pubKey)    // Convert public key to bytes
	hashed := keccak256(pubBytes[1:])          // Hash the public key
	address := hex.EncodeToString(hashed[12:]) // Take the last 20 bytes
	return "0x" + address                      // Convert to hex and add 0x prefix
}

func main() {
	// Load the private key from .env file
	privateKey, err := loadPrivateKeyFromEnv()
	if err != nil {
		log.Fatalf("Failed to load private key: %v", err)
	}

	// Hash and sign the IP address
	ipData := []byte("192.168.0.1")

	// Generate a signature for the hashed IP data
	signature, err := signData(privateKey, ipData)
	if err != nil {
		log.Fatalf("Failed to sign data: %v", err)
	}
	fmt.Printf("Signature: %x\n", signature)

	// Recover the public key from the signature
	recoveredPubKey, err := recoverPublicKey(ipData, signature)
	if err != nil {
		log.Fatalf("Failed to recover public key: %v", err)
	}

	// Convert the recovered public key to wallet address
	recoveredAddress := getAddressFromPublicKey(recoveredPubKey)
	fmt.Printf("Recovered Wallet Address: %s\n", recoveredAddress)

	// Get original wallet address from the private key
	originalAddress := getAddressFromPublicKey(&privateKey.PublicKey)
	fmt.Printf("Original Wallet Address: %s\n", originalAddress)

	// Verify if the recovered address matches the original
	if recoveredAddress == originalAddress {
		fmt.Println("Address successfully recovered and verified!")
	} else {
		fmt.Println("Address recovery failed.")
	}
}

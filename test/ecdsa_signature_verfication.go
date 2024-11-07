package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/sha3"
	"io/ioutil"
	"log"
	"math/big"
)

// Keccak256 hashes the data using the Keccak256 algorithm (Ethereum's standard hash function)
func keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

// GenerateECDSAKeys generates a new ECDSA private and public key pair
func GenerateECDSAKeys() (*ecdsa.PrivateKey, *ecdsa.PublicKey, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ECDSA key: %v", err)
	}
	return privateKey, &privateKey.PublicKey, nil
}

// SavePrivateKey saves the ECDSA private key to a PEM file
func SavePrivateKey(privateKey *ecdsa.PrivateKey, filename string) error {
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %v", err)
	}

	// Write the key as a PEM file
	pemData := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return ioutil.WriteFile(filename, pemData, 0600)
}

// SavePublicKey saves the ECDSA public key to a PEM file
func SavePublicKey(publicKey *ecdsa.PublicKey, filename string) error {
	keyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %v", err)
	}

	// Write the key as a PEM file
	pemData := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: keyBytes})
	return ioutil.WriteFile(filename, pemData, 0644)
}

// LoadPrivateKey loads an ECDSA private key from a PEM file
func LoadPrivateKey(filename string) (*ecdsa.PrivateKey, error) {
	pemData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %v", err)
	}

	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "EC PRIVATE KEY" {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}

	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ECDSA private key: %v", err)
	}

	return privateKey, nil
}

// LoadPublicKey loads an ECDSA public key from a PEM file
func LoadPublicKey(filename string) (*ecdsa.PublicKey, error) {
	pemData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %v", err)
	}

	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("failed to decode PEM block containing public key")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ECDSA public key: %v", err)
	}

	publicKey, ok := key.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an ECDSA public key")
	}

	return publicKey, nil
}

// SignMessage signs the message using the provided ECDSA private key
func SignMessage(privateKey *ecdsa.PrivateKey, message []byte) (r, s *big.Int, err error) {
	hashedMessage := keccak256(message)
	r, s, err = ecdsa.Sign(rand.Reader, privateKey, hashedMessage)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign message: %v", err)
	}
	return r, s, nil
}

// VerifySignature verifies the signature for the given message and ECDSA public key
func VerifySignature(publicKey *ecdsa.PublicKey, message []byte, r, s *big.Int) bool {
	hashedMessage := keccak256(message)
	return ecdsa.Verify(publicKey, hashedMessage, r, s)
}

func main() {
	// Step 1: Generate a new ECDSA key pair
	privateKey, publicKey, err := GenerateECDSAKeys()
	if err != nil {
		log.Fatalf("Failed to generate ECDSA keys: %v", err)
	}

	// Step 2: Save the keys to files
	err = SavePrivateKey(privateKey, "ecdsa_private.pem")
	if err != nil {
		log.Fatalf("Failed to save private key: %v", err)
	}

	err = SavePublicKey(publicKey, "ecdsa_public.pem")
	if err != nil {
		log.Fatalf("Failed to save public key: %v", err)
	}
	fmt.Println("🔐 ECDSA keys generated and saved successfully.")

	// Step 3: Load the keys from files to ensure they were saved correctly
	privateKeyLoaded, err := LoadPrivateKey("ecdsa_private.pem")
	if err != nil {
		log.Fatalf("Failed to load private key: %v", err)
	}

	publicKeyLoaded, err := LoadPublicKey("ecdsa_public.pem")
	if err != nil {
		log.Fatalf("Failed to load public key: %v", err)
	}
	fmt.Println("🔓 Keys loaded from files successfully.")

	// Step 4: Sign a sample message
	message := []byte("Hello, ECDSA verification in Go!")
	r, s, err := SignMessage(privateKeyLoaded, message)
	if err != nil {
		log.Fatalf("Failed to sign message: %v", err)
	}
	fmt.Printf("✍️ Message signed successfully. Signature:\nr: %s\ns: %s\n", r.Text(16), s.Text(16))

	// Step 5: Verify the signature
	isValid := VerifySignature(publicKeyLoaded, message, r, s)
	if isValid {
		fmt.Println("✅ Signature verified: The message is authentic and unaltered.")
	} else {
		fmt.Println("⛔ Signature verification failed: The message or signature is invalid.")
	}
}

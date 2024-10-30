package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
)

func TestSignAndVerifyWithFixedData(t *testing.T) {
	// Generate a new ECDSA key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	publicKey := &privateKey.PublicKey

	// Fixed data to sign
	data := []byte("fixed test data for ECDSA signing")

	// Sign the data
	r, s, err := SignData(privateKey, data)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}

	// Check that r and s are not nil (signing was successful)
	if r == nil || s == nil {
		t.Fatal("Signature values r or s are nil")
	}

	// Verify the signature
	isValid := VerifySignature(publicKey, data, r, s)
	if !isValid {
		t.Error("Signature verification failed, expected success")
	}
}

func TestVerifyInvalidSignatureWithFixedData(t *testing.T) {
	// Generate a new ECDSA key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	publicKey := &privateKey.PublicKey

	// Fixed data to sign
	data := []byte("fixed test data for ECDSA signing")

	// Sign the data
	r, s, err := SignData(privateKey, data)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}

	// Modify data to simulate an invalid signature
	invalidData := []byte("modified test data")

	// Verify the signature with modified data
	isValid := VerifySignature(publicKey, invalidData, r, s)
	if isValid {
		t.Error("Signature verification passed for invalid data, expected failure")
	}
}

func TestSpecificSignatureValues(t *testing.T) {
	// Fixed data and expected values for testing (sample values)
	// For a real fixed-value test, we'd need predetermined r and s values, which are difficult due to randomness
	// Here, we're just testing that signing and verifying works for the fixed data and public/private key

	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	data := []byte("another fixed data example")

	// Perform signing
	r, s, _ := SignData(privateKey, data)

	// Check that r and s are not nil
	if r == nil || s == nil {
		t.Fatal("Signature values r or s are nil")
	}

	// Perform verification
	if !VerifySignature(&privateKey.PublicKey, data, r, s) {
		t.Error("Signature verification failed, expected success")
	}
}

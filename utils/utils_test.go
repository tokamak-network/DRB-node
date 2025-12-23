package utils

import (
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUniqueKey(t *testing.T) {
	tests := []struct {
		name     string
		round    string
		trialNum string
		expected string
	}{
		{
			name:     "basic unique key",
			round:    "1",
			trialNum: "0",
			expected: "1-0",
		},
		{
			name:     "larger numbers",
			round:    "123",
			trialNum: "456",
			expected: "123-456",
		},
		{
			name:     "empty strings",
			round:    "",
			trialNum: "",
			expected: "-",
		},
		{
			name:     "special characters",
			round:    "abc-123",
			trialNum: "def_456",
			expected: "abc-123-def_456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUniqueKey(tt.round, tt.trialNum)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSignData(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	testData := "test data for signing"
	signature := SignData(testData, privateKey)

	assert.NotNil(t, signature)
	assert.Greater(t, len(signature), 0)

	hash := crypto.Keccak256Hash([]byte(testData))
	pubKey, err := crypto.SigToPub(hash.Bytes(), signature)
	require.NoError(t, err)

	expectedAddress := crypto.PubkeyToAddress(*pubKey)
	actualAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	assert.Equal(t, expectedAddress, actualAddress)
}

func TestVerifySignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	t.Run("valid signature", func(t *testing.T) {
		signature := SignData(address, privateKey)
		verification := Verification{
			EOAAddress: address,
			Signature:  signature,
		}

		result := VerifySignature(verification)
		assert.True(t, result)
	})

	t.Run("invalid signature", func(t *testing.T) {
		otherPrivateKey, err := crypto.GenerateKey()
		require.NoError(t, err)
		wrongSignature := SignData(address, otherPrivateKey)
		verification := Verification{
			EOAAddress: address,
			Signature:  wrongSignature,
		}

		result := VerifySignature(verification)
		assert.False(t, result)
	})

	t.Run("invalid signature format", func(t *testing.T) {
		verification := Verification{
			EOAAddress: address,
			Signature:  []byte("invalid signature"),
		}

		result := VerifySignature(verification)
		assert.False(t, result)
	})
}

func TestVerifySignatureForRegularNode(t *testing.T) {
	leaderPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	leaderEOA := crypto.PubkeyToAddress(leaderPrivateKey.PublicKey).Hex()

	regularPrivateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	regularEOA := crypto.PubkeyToAddress(regularPrivateKey.PublicKey).Hex()

	t.Run("valid leader signature", func(t *testing.T) {
		signature := SignData(regularEOA, leaderPrivateKey)

		verification := Verification{
			EOAAddress: regularEOA,
			Signature:  signature,
		}

		result := VerifySignatureForRegularNode(verification, leaderEOA)
		assert.True(t, result)
	})

	t.Run("invalid signature - wrong private key", func(t *testing.T) {
		wrongSignature := SignData(regularEOA, regularPrivateKey)

		verification := Verification{
			EOAAddress: regularEOA,
			Signature:  wrongSignature,
		}

		result := VerifySignatureForRegularNode(verification, leaderEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature format", func(t *testing.T) {
		verification := Verification{
			EOAAddress: regularEOA,
			Signature:  []byte("invalid signature"),
		}

		result := VerifySignatureForRegularNode(verification, leaderEOA)
		assert.False(t, result)
	})
}

func TestSignDataAdditionalCoverage(t *testing.T) {
	t.Run("additional success path coverage", func(t *testing.T) {
		privateKey, err := crypto.GenerateKey()
		require.NoError(t, err)

		data := "test data for additional coverage"
		signature := SignData(data, privateKey)

		assert.NotNil(t, signature)
		assert.Greater(t, len(signature), 0)

		// Verify the signature is valid
		hash := crypto.Keccak256Hash([]byte(data))
		pubKey, err := crypto.SigToPub(hash.Bytes(), signature)
		require.NoError(t, err)

		expectedAddress := crypto.PubkeyToAddress(*pubKey)
		actualAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
		assert.Equal(t, expectedAddress, actualAddress)
	})
}

func TestComputeCvsEIP712TypedDataHash_Success(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test-cvs-value"))

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.NoError(t, err)
	assert.NotEqual(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_MissingContractAddress(t *testing.T) {
	os.Unsetenv("CONTRACT_ADDRESS")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CONTRACT_ADDRESS")
	assert.Equal(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_MissingChainID(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Unsetenv("CHAIN_ID")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CHAIN_ID")
	assert.Equal(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_InvalidChainID(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "invalid")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid chain ID")
	assert.Equal(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_DifferentInputs(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	tests := []struct {
		name     string
		round    *big.Int
		trialNum *big.Int
		cvs      [32]byte
	}{
		{
			name:     "Round 100, Trial 1",
			round:    big.NewInt(100),
			trialNum: big.NewInt(1),
			cvs:      [32]byte{1, 2, 3},
		},
		{
			name:     "Round 200, Trial 2",
			round:    big.NewInt(200),
			trialNum: big.NewInt(2),
			cvs:      [32]byte{4, 5, 6},
		},
		{
			name:     "Round 0, Trial 0",
			round:    big.NewInt(0),
			trialNum: big.NewInt(0),
			cvs:      [32]byte{0},
		},
		{
			name:     "Large round number",
			round:    big.NewInt(999999),
			trialNum: big.NewInt(100),
			cvs:      [32]byte{0xff, 0xff, 0xff},
		},
	}

	hashes := make(map[common.Hash]bool)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := ComputeCvsEIP712TypedDataHash(tt.round, tt.trialNum, tt.cvs)

			assert.NoError(t, err)
			assert.NotEqual(t, common.Hash{}, hash)

			assert.False(t, hashes[hash], "Different inputs should produce different hashes")
			hashes[hash] = true
		})
	}
}

func TestComputeCvsEIP712TypedDataHash_Deterministic(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("deterministic-test"))

	hash1, err1 := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)
	hash2, err2 := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, hash1, hash2, "Same inputs should produce same hash")
}

func TestComputeCvsEIP712TypedDataHash_DifferentChainIDs(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	copy(cvs[:], []byte("test"))

	chainIDs := []string{"1", "5", "11155111"}
	hashes := make(map[common.Hash]bool)

	for _, chainID := range chainIDs {
		t.Run("ChainID_"+chainID, func(t *testing.T) {
			os.Setenv("CHAIN_ID", chainID)
			defer os.Unsetenv("CHAIN_ID")

			hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

			assert.NoError(t, err)
			assert.NotEqual(t, common.Hash{}, hash)

			// Verify different chain IDs produce different hashes
			assert.False(t, hashes[hash], "Different chain IDs should produce different hashes")
			hashes[hash] = true
		})
	}
}

func TestComputeCvsEIP712TypedDataHash_ContractAddressWithoutPrefix(t *testing.T) {
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte

	testCases := []struct {
		name            string
		contractAddress string
	}{
		{
			name:            "With 0x prefix",
			contractAddress: "0x1234567890123456789012345678901234567890",
		},
		{
			name:            "Without 0x prefix",
			contractAddress: "1234567890123456789012345678901234567890",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("CONTRACT_ADDRESS", tc.contractAddress)
			defer os.Unsetenv("CONTRACT_ADDRESS")

			hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

			assert.NoError(t, err)
			assert.NotEqual(t, common.Hash{}, hash)
		})
	}
}

func TestComputeCvsEIP712TypedDataHash_EmptyCVS(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte // All zeros

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.NoError(t, err)
	assert.NotEqual(t, common.Hash{}, hash)
}

func TestComputeCvsEIP712TypedDataHash_FullCVS(t *testing.T) {
	os.Setenv("CONTRACT_ADDRESS", "0x1234567890123456789012345678901234567890")
	os.Setenv("CHAIN_ID", "1")
	defer func() {
		os.Unsetenv("CONTRACT_ADDRESS")
		os.Unsetenv("CHAIN_ID")
	}()

	round := big.NewInt(100)
	trialNum := big.NewInt(1)
	var cvs [32]byte
	// Fill all 32 bytes
	for i := 0; i < 32; i++ {
		cvs[i] = byte(i)
	}

	hash, err := ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)

	assert.NoError(t, err)
	assert.NotEqual(t, common.Hash{}, hash)
}

func TestSignBroadcastMessageContent(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	var data [32]byte
	copy(data[:], []byte("test-broadcast-data"))
	signerEOA := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	message := BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-123",
		Type:       "CVS",
		Data:       data,
		SignerEOA:  signerEOA,
	}

	signature, err := SignBroadcastMessageContent(message, privateKey)

	assert.NoError(t, err)
	assert.NotNil(t, signature)
	assert.Greater(t, len(signature), 0)
}

// TestVerifyBroadcastMessageContentSignature tests verifying broadcast message signatures
func TestVerifyBroadcastMessageContentSignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	expectedEOA := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	var data [32]byte
	copy(data[:], []byte("test-broadcast-data"))
	signerEOA := expectedEOA

	baseMessage := BroadcastMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: "0x1234567890123456789012345678901234567890",
		MessageID:  "msg-123",
		Type:       "CVS",
		Data:       data,
		SignerEOA:  signerEOA,
	}

	t.Run("valid signature", func(t *testing.T) {
		message := baseMessage
		signature, err := SignBroadcastMessageContent(message, privateKey)
		require.NoError(t, err)
		message.Signature = signature

		result := VerifyBroadcastMessageContentSignature(message, expectedEOA)
		assert.True(t, result)
	})

	t.Run("invalid signature - wrong private key", func(t *testing.T) {
		wrongPrivateKey, err := crypto.GenerateKey()
		require.NoError(t, err)
		message := baseMessage
		wrongSignature, err := SignBroadcastMessageContent(message, wrongPrivateKey)
		require.NoError(t, err)
		message.Signature = wrongSignature

		result := VerifyBroadcastMessageContentSignature(message, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered round", func(t *testing.T) {
		message := baseMessage
		signature, err := SignBroadcastMessageContent(message, privateKey)
		require.NoError(t, err)
		message.Signature = signature
		message.Round = "200"

		result := VerifyBroadcastMessageContentSignature(message, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered data", func(t *testing.T) {
		message := baseMessage
		signature, err := SignBroadcastMessageContent(message, privateKey)
		require.NoError(t, err)
		message.Signature = signature
		var tamperedData [32]byte
		copy(tamperedData[:], []byte("tampered-broadcast-data"))
		message.Data = tamperedData

		result := VerifyBroadcastMessageContentSignature(message, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered eoaAddress", func(t *testing.T) {
		message := baseMessage
		signature, err := SignBroadcastMessageContent(message, privateKey)
		require.NoError(t, err)
		message.Signature = signature
		message.EOAAddress = "0x9876543210987654321098765432109876543210"

		result := VerifyBroadcastMessageContentSignature(message, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature format", func(t *testing.T) {
		message := baseMessage
		message.Signature = []byte("invalid signature")

		result := VerifyBroadcastMessageContentSignature(message, expectedEOA)
		assert.False(t, result)
	})
}

func TestSignCosRequestContent(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	var cos [32]byte
	copy(cos[:], []byte("test-cos-data"))
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	req := CosRequest{
		Round:      "100",
		TrialNum:   "1",
		Cos:        cos,
		EOAAddress: eoaAddress,
		UniqueKey:  "100-1",
	}

	signature, err := SignCosRequestContent(req, privateKey)

	assert.NoError(t, err)
	assert.NotNil(t, signature)
	assert.Greater(t, len(signature), 0)
}

func TestVerifyCosRequestContentSignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	expectedEOA := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	var cos [32]byte
	copy(cos[:], []byte("test-cos-data"))

	baseReq := CosRequest{
		Round:      "100",
		TrialNum:   "1",
		Cos:        cos,
		EOAAddress: expectedEOA,
		UniqueKey:  "100-1",
	}

	t.Run("valid signature", func(t *testing.T) {
		req := baseReq
		signature, err := SignCosRequestContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature

		result := VerifyCosRequestContentSignature(req, expectedEOA)
		assert.True(t, result)
	})

	t.Run("invalid signature - wrong private key", func(t *testing.T) {
		wrongPrivateKey, err := crypto.GenerateKey()
		require.NoError(t, err)
		req := baseReq
		wrongSignature, err := SignCosRequestContent(req, wrongPrivateKey)
		require.NoError(t, err)
		req.Signature = wrongSignature

		result := VerifyCosRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered round", func(t *testing.T) {
		req := baseReq
		signature, err := SignCosRequestContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature
		req.Round = "200"

		result := VerifyCosRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered trialNum", func(t *testing.T) {
		req := baseReq
		signature, err := SignCosRequestContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature
		req.TrialNum = "2"

		result := VerifyCosRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered cos", func(t *testing.T) {
		req := baseReq
		signature, err := SignCosRequestContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature
		var tamperedCos [32]byte
		copy(tamperedCos[:], []byte("tampered-cos-data"))
		req.Cos = tamperedCos

		result := VerifyCosRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature format", func(t *testing.T) {
		req := baseReq
		req.Signature = []byte("invalid signature")

		result := VerifyCosRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})
}

func TestSignAcknowledgmentContent(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	ack := AcknowledgmentMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: eoaAddress,
		MessageID:  "msg-123",
		Type:       "CVS",
		Status:     "received",
	}

	signature, err := SignAcknowledgmentContent(ack, privateKey)

	assert.NoError(t, err)
	assert.NotNil(t, signature)
	assert.Greater(t, len(signature), 0)
}

func TestVerifyAcknowledgmentContentSignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	expectedEOA := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	baseAck := AcknowledgmentMessage{
		Round:      "100",
		TrialNum:   "1",
		EOAAddress: expectedEOA,
		MessageID:  "msg-123",
		Type:       "CVS",
		Status:     "received",
	}

	t.Run("valid signature", func(t *testing.T) {
		ack := baseAck
		signature, err := SignAcknowledgmentContent(ack, privateKey)
		require.NoError(t, err)
		ack.Signature = signature

		result := VerifyAcknowledgmentContentSignature(ack, expectedEOA)
		assert.True(t, result)
	})

	t.Run("invalid signature - wrong private key", func(t *testing.T) {
		wrongPrivateKey, err := crypto.GenerateKey()
		require.NoError(t, err)
		ack := baseAck
		wrongSignature, err := SignAcknowledgmentContent(ack, wrongPrivateKey)
		require.NoError(t, err)
		ack.Signature = wrongSignature

		result := VerifyAcknowledgmentContentSignature(ack, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered round", func(t *testing.T) {
		ack := baseAck
		signature, err := SignAcknowledgmentContent(ack, privateKey)
		require.NoError(t, err)
		ack.Signature = signature
		ack.Round = "200"

		result := VerifyAcknowledgmentContentSignature(ack, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered ackType", func(t *testing.T) {
		ack := baseAck
		signature, err := SignAcknowledgmentContent(ack, privateKey)
		require.NoError(t, err)
		ack.Signature = signature
		ack.Type = "COS"

		result := VerifyAcknowledgmentContentSignature(ack, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered status", func(t *testing.T) {
		ack := baseAck
		signature, err := SignAcknowledgmentContent(ack, privateKey)
		require.NoError(t, err)
		ack.Signature = signature
		ack.Status = "error"

		result := VerifyAcknowledgmentContentSignature(ack, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature format", func(t *testing.T) {
		ack := baseAck
		ack.Signature = []byte("invalid signature")

		result := VerifyAcknowledgmentContentSignature(ack, expectedEOA)
		assert.False(t, result)
	})
}

func TestSignSecretValueRequestContent(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	leaderEoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	req := SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		Order:             0,
		LeaderEoaAddress:  leaderEoaAddress,
		RegularEoaAddress: "0x1234567890123456789012345678901234567890",
	}

	signature, err := SignSecretValueRequestContent(req, privateKey)

	assert.NoError(t, err)
	assert.NotNil(t, signature)
	assert.Greater(t, len(signature), 0)
}

func TestVerifySecretValueRequestContentSignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	expectedEOA := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	baseReq := SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		Order:             0,
		LeaderEoaAddress:  expectedEOA,
		RegularEoaAddress: "0x1234567890123456789012345678901234567890",
	}

	t.Run("valid signature", func(t *testing.T) {
		req := baseReq
		signature, err := SignSecretValueRequestContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature

		result := VerifySecretValueRequestContentSignature(req, expectedEOA)
		assert.True(t, result)
	})

	t.Run("invalid signature - wrong private key", func(t *testing.T) {
		wrongPrivateKey, err := crypto.GenerateKey()
		require.NoError(t, err)
		req := baseReq
		wrongSignature, err := SignSecretValueRequestContent(req, wrongPrivateKey)
		require.NoError(t, err)
		req.Signature = wrongSignature

		result := VerifySecretValueRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered round", func(t *testing.T) {
		req := baseReq
		signature, err := SignSecretValueRequestContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature
		req.Round = "200"

		result := VerifySecretValueRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered order", func(t *testing.T) {
		req := baseReq
		signature, err := SignSecretValueRequestContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature
		req.Order = 1

		result := VerifySecretValueRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered regularEoaAddress", func(t *testing.T) {
		req := baseReq
		signature, err := SignSecretValueRequestContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature
		req.RegularEoaAddress = "0x9876543210987654321098765432109876543210"

		result := VerifySecretValueRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature format", func(t *testing.T) {
		req := baseReq
		req.Signature = []byte("invalid signature")

		result := VerifySecretValueRequestContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("different orders produce different signatures", func(t *testing.T) {
		req1 := baseReq
		req1.Order = 0
		signature1, err := SignSecretValueRequestContent(req1, privateKey)
		require.NoError(t, err)

		req2 := baseReq
		req2.Order = 1
		signature2, err := SignSecretValueRequestContent(req2, privateKey)
		require.NoError(t, err)

		assert.NotEqual(t, signature1, signature2, "Different orders should produce different signatures")
	})
}

func TestSignSecretValueContent(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	var secretValue [32]byte
	copy(secretValue[:], []byte("test-secret-value"))
	regularEoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	req := SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		SecretValue:       secretValue[:],
		RegularEoaAddress: regularEoaAddress,
	}

	signature, err := SignSecretValueContent(req, privateKey)

	assert.NoError(t, err)
	assert.NotNil(t, signature)
	assert.Greater(t, len(signature), 0)
}

func TestVerifySecretValueContentSignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	expectedEOA := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	var secretValue [32]byte
	copy(secretValue[:], []byte("test-secret-value"))

	baseReq := SecretValueRequest{
		Round:             "100",
		TrialNum:          "1",
		SecretValue:       secretValue[:],
		RegularEoaAddress: expectedEOA,
	}

	t.Run("valid signature", func(t *testing.T) {
		req := baseReq
		signature, err := SignSecretValueContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature

		result := VerifySecretValueContentSignature(req, expectedEOA)
		assert.True(t, result)
	})

	t.Run("invalid signature - wrong private key", func(t *testing.T) {
		wrongPrivateKey, err := crypto.GenerateKey()
		require.NoError(t, err)
		req := baseReq
		wrongSignature, err := SignSecretValueContent(req, wrongPrivateKey)
		require.NoError(t, err)
		req.Signature = wrongSignature

		result := VerifySecretValueContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered round", func(t *testing.T) {
		req := baseReq
		signature, err := SignSecretValueContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature
		req.Round = "200"

		result := VerifySecretValueContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered secretValue", func(t *testing.T) {
		req := baseReq
		signature, err := SignSecretValueContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature
		var tamperedSecretValue [32]byte
		copy(tamperedSecretValue[:], []byte("tampered-secret-value"))
		req.SecretValue = tamperedSecretValue[:]

		result := VerifySecretValueContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature - tampered regularEoaAddress", func(t *testing.T) {
		req := baseReq
		signature, err := SignSecretValueContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature
		req.RegularEoaAddress = "0x9876543210987654321098765432109876543210"

		result := VerifySecretValueContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("invalid signature format", func(t *testing.T) {
		req := baseReq
		req.Signature = []byte("invalid signature")

		result := VerifySecretValueContentSignature(req, expectedEOA)
		assert.False(t, result)
	})

	t.Run("empty secret value", func(t *testing.T) {
		var emptySecretValue [32]byte
		req := SecretValueRequest{
			Round:             "100",
			TrialNum:          "1",
			SecretValue:       emptySecretValue[:],
			RegularEoaAddress: expectedEOA,
		}
		signature, err := SignSecretValueContent(req, privateKey)
		require.NoError(t, err)
		req.Signature = signature

		result := VerifySecretValueContentSignature(req, expectedEOA)
		assert.True(t, result)
	})

	t.Run("different secret values produce different signatures", func(t *testing.T) {
		var secretValue1 [32]byte
		copy(secretValue1[:], []byte("secret-value-1"))
		req1 := SecretValueRequest{
			Round:             "100",
			TrialNum:          "1",
			SecretValue:       secretValue1[:],
			RegularEoaAddress: expectedEOA,
		}
		signature1, err := SignSecretValueContent(req1, privateKey)
		require.NoError(t, err)

		var secretValue2 [32]byte
		copy(secretValue2[:], []byte("secret-value-2"))
		req2 := SecretValueRequest{
			Round:             "100",
			TrialNum:          "1",
			SecretValue:       secretValue2[:],
			RegularEoaAddress: expectedEOA,
		}
		signature2, err := SignSecretValueContent(req2, privateKey)
		require.NoError(t, err)

		assert.NotEqual(t, signature1, signature2, "Different secret values should produce different signatures")
	})
}

func TestSignBroadcastMessageContent_DifferentTypes(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	var data [32]byte
	copy(data[:], []byte("test-data"))
	signerEOA := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	types := []string{"CVS", "COS", "Secret"}
	signatures := make(map[string][]byte)

	for _, msgType := range types {
		t.Run("Type_"+msgType, func(t *testing.T) {
			message := BroadcastMessage{
				Round:      "100",
				TrialNum:   "1",
				EOAAddress: "0x1234567890123456789012345678901234567890",
				MessageID:  "msg-123",
				Type:       msgType,
				Data:       data,
				SignerEOA:  signerEOA,
			}
			signature, err := SignBroadcastMessageContent(message, privateKey)
			assert.NoError(t, err)
			assert.NotNil(t, signature)

			// Verify different types produce different signatures
			for existingType, existingSig := range signatures {
				if existingType != msgType {
					assert.NotEqual(t, existingSig, signature, "Different types should produce different signatures")
				}
			}
			signatures[msgType] = signature
		})
	}
}

func TestSignCosRequestContent_EdgeCases(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	var cos [32]byte
	copy(cos[:], []byte("test-cos-data"))
	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	tests := []struct {
		name      string
		round     string
		trialNum  string
		uniqueKey string
	}{
		{
			name:      "empty strings",
			round:     "",
			trialNum:  "",
			uniqueKey: "-",
		},
		{
			name:      "large numbers",
			round:     "999999",
			trialNum:  "888888",
			uniqueKey: "999999-888888",
		},
		{
			name:      "zero values",
			round:     "0",
			trialNum:  "0",
			uniqueKey: "0-0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := CosRequest{
				Round:      tt.round,
				TrialNum:   tt.trialNum,
				Cos:        cos,
				EOAAddress: eoaAddress,
				UniqueKey:  tt.uniqueKey,
			}
			signature, err := SignCosRequestContent(req, privateKey)
			assert.NoError(t, err)
			assert.NotNil(t, signature)
		})
	}
}

func TestSignAcknowledgmentContent_DifferentTypes(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	ackTypes := []string{"CVS", "COS", "Secret"}
	signatures := make(map[string][]byte)

	for _, ackType := range ackTypes {
		t.Run("Type_"+ackType, func(t *testing.T) {
			ack := AcknowledgmentMessage{
				Round:      "100",
				TrialNum:   "1",
				EOAAddress: eoaAddress,
				MessageID:  "msg-123",
				Type:       ackType,
				Status:     "received",
			}
			signature, err := SignAcknowledgmentContent(ack, privateKey)
			assert.NoError(t, err)
			assert.NotNil(t, signature)

			// Verify different types produce different signatures
			for existingType, existingSig := range signatures {
				if existingType != ackType {
					assert.NotEqual(t, existingSig, signature, "Different types should produce different signatures")
				}
			}
			signatures[ackType] = signature
		})
	}
}

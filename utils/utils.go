package utils

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	appconfig "github.com/tokamak-network/DRB-node/config"
)

type RegistrationRequest struct {
	EOAAddress string `json:"eoa_address"`
	Signature  []byte `json:"signature"`
	PeerID     string `json:"peer_id"`
}

type Verification struct {
	EOAAddress string `json:"eoa_address"`
	Signature  []byte `json:"signature"`
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

// VerifySignature checks if the signature matches the EOA address
func VerifySignature(req Verification) bool {
	hash := crypto.Keccak256Hash([]byte(req.EOAAddress))
	pubKey, err := crypto.SigToPub(hash.Bytes(), req.Signature)
	if err != nil {
		log.Printf("Error recovering public key: %v", err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	log.Printf("recoveredAddress........:%s", recoveredAddress)
	log.Printf("req.EOAAddress........:%s", req.EOAAddress)

	return recoveredAddress == req.EOAAddress
}

func VerifySignatureForRegularNode(req Verification, leaderEOA string) bool {
	hash := crypto.Keccak256Hash([]byte(req.EOAAddress))
	pubKey, err := crypto.SigToPub(hash.Bytes(), req.Signature)
	if err != nil {
		log.Printf("Error recovering public key: %v", err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	log.Printf("recoveredAddress........:%s", recoveredAddress)
	log.Printf("req.EOAAddress........:%s", leaderEOA)

	return recoveredAddress == leaderEOA
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

func ComputeCvsEIP712TypedDataHash(round *big.Int, trialNum *big.Int, cvs [32]byte) (common.Hash, error) {
	name := "Commit Reveal2"
	version := "1"

	envCfg := appconfig.Get()

	contractAddressEnv := envCfg.ContractAddress
	if contractAddressEnv == "" {
		return common.Hash{}, fmt.Errorf("CONTRACT_ADDRESS is not set in environment variables")
	}
	chainIDEnv := envCfg.ChainID
	if chainIDEnv == "" {
		return common.Hash{}, fmt.Errorf("CHAIN_ID is not set in environment variables")
	}
	contractAddressEnv = strings.TrimPrefix(contractAddressEnv, "0x")
	contractAddress := common.HexToAddress(contractAddressEnv)

	chainID := new(big.Int)
	chainID, ok := chainID.SetString(chainIDEnv, 10)
	if !ok {
		return common.Hash{}, fmt.Errorf("invalid chain ID: %s", chainIDEnv)
	}

	// Step 1: Compute domain separator
	domainTypeHash := crypto.Keccak256Hash([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	nameHash := crypto.Keccak256Hash([]byte(name))
	versionHash := crypto.Keccak256Hash([]byte(version))

	domainSeparator := crypto.Keccak256Hash(
		abiEncode(
			domainTypeHash.Bytes(),
			nameHash.Bytes(),
			versionHash.Bytes(),
			intToBytes(chainID),
			contractAddress.Bytes(),
		),
	)

	// Step 2: Compute message hash
	messageTypeHash := crypto.Keccak256Hash([]byte("Message(uint256 round,uint256 trialNum,bytes32 cv)"))
	messageHash := crypto.Keccak256Hash(
		abiEncode(
			messageTypeHash.Bytes(),
			intToBytes(round),
			intToBytes(trialNum),
			cvs[:],
		),
	)

	// Step 3: Compute the final typed data hash
	typedDataHash := crypto.Keccak256Hash(
		abiEncodePacked(
			[]byte{0x19, 0x01},
			domainSeparator.Bytes(),
			messageHash.Bytes(),
		),
	)

	return typedDataHash, nil
}

func abiEncode(elements ...[]byte) []byte {
	var encoded []byte
	for _, e := range elements {
		encoded = append(encoded, common.LeftPadBytes(e, 32)...)
	}
	return encoded
}

func abiEncodePacked(elements ...[]byte) []byte {
	var packed []byte
	for _, e := range elements {
		packed = append(packed, e...)
	}
	return packed
}

func intToBytes(num *big.Int) []byte {
	return common.LeftPadBytes(num.Bytes(), 32)
}

func SignBroadcastMessageContent(message BroadcastMessage, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	messageHash := crypto.Keccak256Hash(
		[]byte(message.Round),
		[]byte(message.TrialNum),
		[]byte(message.EOAAddress),
		[]byte(message.MessageID),
		[]byte(message.Type),
		message.Data[:],
		[]byte(message.SignerEOA),
	)

	signature, err := crypto.Sign(messageHash.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign broadcast message content: %v", err)
	}

	return signature, nil
}
func VerifyBroadcastMessageContentSignature(message BroadcastMessage, expectedSignerEOA string) bool {
	messageHash := crypto.Keccak256Hash(
		[]byte(message.Round),
		[]byte(message.TrialNum),
		[]byte(message.EOAAddress),
		[]byte(message.MessageID),
		[]byte(message.Type),
		message.Data[:],
		[]byte(message.SignerEOA),
	)

	// Recover public key from signature
	pubKey, err := crypto.SigToPub(messageHash.Bytes(), message.Signature)
	if err != nil {
		log.Printf("Error recovering public key from broadcast message signature: %v", err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	expectedAddress := common.HexToAddress(expectedSignerEOA).Hex()

	log.Printf("Broadcast message signature verification - Recovered: %s, Expected: %s", recoveredAddress, expectedAddress)

	return recoveredAddress == expectedAddress
}

func SignCosRequestContent(req CosRequest, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	messageHash := crypto.Keccak256Hash(
		[]byte(req.Round),
		[]byte(req.TrialNum),
		req.Cos[:],
		[]byte(req.EOAAddress),
		[]byte(req.UniqueKey),
	)

	signature, err := crypto.Sign(messageHash.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign COS request content: %v", err)
	}
	return signature, nil
}


func VerifyCosRequestContentSignature(req CosRequest, expectedSignerEOA string) bool {
	messageHash := crypto.Keccak256Hash(
		[]byte(req.Round),
		[]byte(req.TrialNum),
		req.Cos[:],
		[]byte(req.EOAAddress),
		[]byte(req.UniqueKey),
	)

	pubKey, err := crypto.SigToPub(messageHash.Bytes(), req.Signature)
	if err != nil {
		log.Printf("Error recovering public key from COS request signature: %v", err)
		return false
	}

	// Get address from public key
	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	expectedAddress := common.HexToAddress(expectedSignerEOA).Hex()
	log.Printf("COS request signature verification - Recovered: %s, Expected: %s", recoveredAddress, expectedAddress)

	return recoveredAddress == expectedAddress
}

func SignAcknowledgmentContent(ack AcknowledgmentMessage, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	messageHash := crypto.Keccak256Hash(
		[]byte(ack.Round),
		[]byte(ack.TrialNum),
		[]byte(ack.EOAAddress),
		[]byte(ack.MessageID),
		[]byte(ack.Type),
		[]byte(ack.Status),
	)

	signature, err := crypto.Sign(messageHash.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign acknowledgment content: %v", err)
	}

	return signature, nil
}

func VerifyAcknowledgmentContentSignature(ack AcknowledgmentMessage, expectedSignerEOA string) bool {
	messageHash := crypto.Keccak256Hash(
		[]byte(ack.Round),
		[]byte(ack.TrialNum),
		[]byte(ack.EOAAddress),
		[]byte(ack.MessageID),
		[]byte(ack.Type),
		[]byte(ack.Status),
	)

	pubKey, err := crypto.SigToPub(messageHash.Bytes(), ack.Signature)
	if err != nil {
		log.Printf("Error recovering public key from acknowledgment signature: %v", err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	expectedAddress := common.HexToAddress(expectedSignerEOA).Hex()

	log.Printf("Acknowledgment signature verification - Recovered: %s, Expected: %s", recoveredAddress, expectedAddress)

	return recoveredAddress == expectedAddress
}

func SignSecretValueRequestContent(req SecretValueRequest, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	orderStr := strconv.Itoa(req.Order)

	messageHash := crypto.Keccak256Hash(
		[]byte(req.Round),
		[]byte(req.TrialNum),
		[]byte(orderStr),
		[]byte(req.LeaderEoaAddress),
		[]byte(req.RegularEoaAddress),
	)

	signature, err := crypto.Sign(messageHash.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign secret value request content: %v", err)
	}

	return signature, nil
}

func VerifySecretValueRequestContentSignature(req SecretValueRequest, expectedSignerEOA string) bool {
	orderStr := strconv.Itoa(req.Order)

	messageHash := crypto.Keccak256Hash(
		[]byte(req.Round),
		[]byte(req.TrialNum),
		[]byte(orderStr),
		[]byte(req.LeaderEoaAddress),
		[]byte(req.RegularEoaAddress),
	)

	pubKey, err := crypto.SigToPub(messageHash.Bytes(), req.Signature)
	if err != nil {
		log.Printf("Error recovering public key from secret value request signature: %v", err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	expectedAddress := common.HexToAddress(expectedSignerEOA).Hex()

	log.Printf("Secret value request signature verification - Recovered: %s, Expected: %s", recoveredAddress, expectedAddress)

	return recoveredAddress == expectedAddress
}

func SignSecretValueContent(req SecretValueRequest, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	var secretValueArray [32]byte
	if len(req.SecretValue) != 32 {
		return nil, fmt.Errorf("invalid secret value length: expected 32 bytes, got %d", len(req.SecretValue))
	}
	copy(secretValueArray[:], req.SecretValue[:])

	messageHash := crypto.Keccak256Hash(
		[]byte(req.Round),
		[]byte(req.TrialNum),
		secretValueArray[:],
		[]byte(req.RegularEoaAddress),
	)

	signature, err := crypto.Sign(messageHash.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign secret value content: %v", err)
	}

	return signature, nil
}

func VerifySecretValueContentSignature(req SecretValueRequest, expectedSignerEOA string) bool {
	var secretValueArray [32]byte
	if len(req.SecretValue) != 32 {
		log.Printf("Invalid secret value length: expected 32 bytes, got %d", len(req.SecretValue))
		return false
	}
	copy(secretValueArray[:], req.SecretValue[:])

	messageHash := crypto.Keccak256Hash(
		[]byte(req.Round),
		[]byte(req.TrialNum),
		secretValueArray[:],
		[]byte(req.RegularEoaAddress),
	)

	pubKey, err := crypto.SigToPub(messageHash.Bytes(), req.Signature)
	if err != nil {
		log.Printf("Error recovering public key from secret value signature: %v", err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	expectedAddress := common.HexToAddress(expectedSignerEOA).Hex()

	log.Printf("Secret value signature verification - Recovered: %s, Expected: %s", recoveredAddress, expectedAddress)

	return recoveredAddress == expectedAddress
}

func SignCommitRequestContent(req CommitRequest, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	messageHash := crypto.Keccak256Hash(
		[]byte(req.Round),
		[]byte(req.TrialNum),
		req.Cvs[:],
		[]byte(req.EOAAddress),
		[]byte(req.UniqueKey),
	)

	signature, err := crypto.Sign(messageHash.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign commit request content: %v", err)
	}
	return signature, nil
}

func VerifyCommitRequestContentSignature(req CommitRequest, expectedSignerEOA string) bool {
	messageHash := crypto.Keccak256Hash(
		[]byte(req.Round),
		[]byte(req.TrialNum),
		req.Cvs[:],
		[]byte(req.EOAAddress),
		[]byte(req.UniqueKey),
	)

	pubKey, err := crypto.SigToPub(messageHash.Bytes(), req.Signature)
	if err != nil {
		log.Printf("Error recovering public key from commit request signature: %v", err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	expectedAddress := common.HexToAddress(expectedSignerEOA).Hex()

	log.Printf("Commit request signature verification - Recovered: %s, Expected: %s", recoveredAddress, expectedAddress)

	return recoveredAddress == expectedAddress
}

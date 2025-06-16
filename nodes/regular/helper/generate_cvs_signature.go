package regularNode_helper

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tokamak-network/DRB-node/logger"
)

var EoaAddress string

// GenerateCvsSignature generates the EIP-712 signature components (v, r, s) for a given round and CVS value.
func GenerateCvsSignature(startTimeStr string, cvs [32]byte) (uint8, string, string, error) {
	// Convert CVS to string for internal usage (optional, depending on use case)
	cvsString := hex.EncodeToString(cvs[:])
	logger.Infof("Received CVS as [32]byte: %x", cvs)
	logger.Infof("Converted CVS to String: %s", cvsString)

	// Define constants for EIP-712
	name := "Commit Reveal2"
	version := "1"

	// Fetch contract address and chain ID dynamically from the .env file
	contractAddressEnv := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressEnv == "" {
		logger.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	chainIDEnv := os.Getenv("CHAIN_ID")
	if chainIDEnv == "" {
		logger.Fatal("CHAIN_ID is not set in environment variables.")
	}
	contractAddressEnv = strings.TrimPrefix(contractAddressEnv, "0x")
	contractAddress := common.HexToAddress(contractAddressEnv)

	chainID := new(big.Int)
	chainID, okays := chainID.SetString(chainIDEnv, 10)
	if !okays {
		return 0, "", "", fmt.Errorf("invalid chain ID: %s", chainIDEnv)
	}

	// Parse startTimeStr as *big.Int
	startTime := new(big.Int)
	_, ok := startTime.SetString(startTimeStr, 10)
	if !ok {
		return 0, "", "", fmt.Errorf("invalid startTimeStr: %s", startTimeStr)
	}

	// Load the private key
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		logger.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to decode private key: %v", err)
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
	messageTypeHash := crypto.Keccak256Hash([]byte("Message(uint256 timestamp,bytes32 cv)"))
	fmt.Println("messageTypeHash", messageTypeHash)
	messageHash := crypto.Keccak256Hash(
		abiEncode(
			messageTypeHash.Bytes(),
			intToBytes(startTime), // uint256 startTime
			cvs[:],                // bytes32 CVS as [32]byte
		),
	)
	logger.Infof("Message Hash: %s", messageHash.Hex())

	// Step 3: Compute the final typed data hash
	typedDataHash := crypto.Keccak256Hash(
		abiEncodePacked(
			[]byte{0x19, 0x01}, // EIP-712 prefix
			domainSeparator.Bytes(),
			messageHash.Bytes(),
		),
	)
	logger.Infof("Typed Data Hash: %s", typedDataHash.Hex())

	// Step 4: Sign the typed data hash
	signature, err := crypto.Sign(typedDataHash.Bytes(), privateKey)
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to sign typed data: %v", err)
	}

	// Split the signature into r, s, and v
	r := hex.EncodeToString(signature[:32])
	s := hex.EncodeToString(signature[32:64])
	v := uint8(signature[64]) + 27 // Adjust for Ethereum recovery ID

	logger.Infof("Generated EIP-712 signature: v=%d, r=%s, s=%s", v, r, s)
	return v, r, s, nil
}

// Helper: abiEncode replicates Solidity's `abi.encode` behavior with 32-byte padding.
func abiEncode(elements ...[]byte) []byte {
	var encoded []byte
	for _, e := range elements {
		encoded = append(encoded, common.LeftPadBytes(e, 32)...)
	}
	return encoded
}

// Helper: abiEncodePacked replicates Solidity's `abi.encodePacked` behavior.
func abiEncodePacked(elements ...[]byte) []byte {
	var packed []byte
	for _, e := range elements {
		packed = append(packed, e...)
	}
	return packed
}

// Helper: intToBytes converts a *big.Int to its padded big-endian byte representation.
func intToBytes(n *big.Int) []byte {
	return common.LeftPadBytes(n.Bytes(), 32)
}

func Setup(eoaAddress string) {
	EoaAddress = eoaAddress
}

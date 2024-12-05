package regularNode_helper

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// GenerateCvsSignature generates the EIP-712 signature components (v, r, s) for a given round and CVS value.
func GenerateCvsSignature(roundNum string, cvs [32]byte) (uint8, string, string, error) {
	// Convert CVS to string for internal usage (optional, depending on use case)
	cvsString := hex.EncodeToString(cvs[:])
	log.Printf("Received CVS as [32]byte: %x", cvs)
	log.Printf("Converted CVS to String: %s", cvsString)

	// Define constants for EIP-712
	name := "Tokamak DRB"
	version := "1"

	// Fetch contract address and chain ID dynamically
	contractAddress := common.HexToAddress("31BCECA13c5be57b3677Ec116FB38fEde7Fe1217")
	chainID := big.NewInt(111551119090)

	// Parse roundNum as *big.Int
	round := new(big.Int)
	_, ok := round.SetString(roundNum, 10)
	if !ok {
		return 0, "", "", fmt.Errorf("invalid round number: %s", roundNum)
	}

	// Load the private key
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
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
	log.Printf("Domain Separator: %s", domainSeparator.Hex())

	// Step 2: Compute message hash
	messageTypeHash := crypto.Keccak256Hash([]byte("Message(uint256 round,bytes32 cv)"))

	messageHash := crypto.Keccak256Hash(
		abiEncode(
			messageTypeHash.Bytes(),
			intToBytes(round), // uint256 round
			cvs[:],            // bytes32 CVS as [32]byte
		),
	)
	log.Printf("Message Hash: %s", messageHash.Hex())

	// Step 3: Compute the final typed data hash
	typedDataHash := crypto.Keccak256Hash(
		abiEncodePacked(
			[]byte{0x19, 0x01}, // EIP-712 prefix
			domainSeparator.Bytes(),
			messageHash.Bytes(),
		),
	)
	log.Printf("Typed Data Hash: %s", typedDataHash.Hex())

	// Step 4: Sign the typed data hash
	signature, err := crypto.Sign(typedDataHash.Bytes(), privateKey)
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to sign typed data: %v", err)
	}

	// Split the signature into r, s, and v
	r := hex.EncodeToString(signature[:32])
	s := hex.EncodeToString(signature[32:64])
	v := uint8(signature[64]) + 27 // Adjust for Ethereum recovery ID

	log.Printf("Generated EIP-712 signature: v=%d, r=%s, s=%s", v, r, s)
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

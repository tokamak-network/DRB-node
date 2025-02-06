package commitreveal2

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/sha3"
)

// GenerateCommit generates the secret value, cos, and cvs for a single regular node.
func GenerateCommit(round string, operator string) ([32]byte, [32]byte, [32]byte, error) {
	// Current timestamp to simulate block.timestamp in Solidity
	timestamp := big.NewInt(time.Now().Unix())

	// Convert round to big.Int
	roundInt := new(big.Int)
	_, ok := roundInt.SetString(round, 10)
	if !ok {
		return [32]byte{}, [32]byte{}, [32]byte{}, fmt.Errorf("invalid round: %s", round)
	}

	// Convert operator to Ethereum address
	operatorAddress := common.HexToAddress(operator)

	// Generate secret value using keccak256(abi.encodePacked(round, operator, timestamp))
	secretValue := Keccak256(abiEncodePacked(intToBytes(roundInt), operatorAddress.Bytes(), intToBytes(timestamp)))

	// Generate cos by hashing the secretValue using abi.encode
	cos := Keccak256(abiEncode(secretValue))

	// Generate cvs by hashing the cos using abi.encode
	cvs := Keccak256(abiEncode(cos))

	// Convert results into [32]byte format (Solidity's bytes32)
	var secretValueBytes32, cosBytes32, cvsBytes32 [32]byte
	copy(secretValueBytes32[:], secretValue)
	copy(cosBytes32[:], cos)
	copy(cvsBytes32[:], cvs)

	// Print results
	log.Printf("Secret Value (bytes32): 0x%s", hex.EncodeToString(secretValue))
	log.Printf("COS (bytes32): 0x%s", hex.EncodeToString(cos))
	log.Printf("CVS (bytes32): 0x%s", hex.EncodeToString(cvs))

	return secretValueBytes32, cosBytes32, cvsBytes32, nil
}

// keccak256 performs a Keccak-256 hash on the input data and returns the result.
func Keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

// abiEncode replicates Solidity's abi.encode behavior with 32-byte padding.
func abiEncode(elements ...[]byte) []byte {
	var encoded []byte
	for _, e := range elements {
		encoded = append(encoded, common.LeftPadBytes(e, 32)...)
	}
	return encoded
}

// abiEncodePacked replicates Solidity's abi.encodePacked behavior.
func abiEncodePacked(elements ...[]byte) []byte {
	var packed []byte
	for _, e := range elements {
		packed = append(packed, e...)
	}
	return packed
}

// intToBytes converts a *big.Int to its padded big-endian byte representation.
func intToBytes(n *big.Int) []byte {
	return common.LeftPadBytes(n.Bytes(), 32)
}

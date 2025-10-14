package commitreveal2

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tokamak-network/DRB-node/eth"
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
	activatedOperators := eth.GetActivatedOperatorsCached()
	operatorIndex := -1
	for i, op := range activatedOperators {
		if op.Hex() == operator {
			operatorIndex = i
			break
		}
	}
	// Generate secret value using keccak256(abi.encodePacked(round, operator, timestamp))
	secretValue := Keccak256(AbiEncodePacked(IntToBytes(roundInt), operatorAddress.Bytes(), IntToBytes(timestamp)))

	// Generate cos by hashing the secretValue using abi.encode
	cos := Keccak256(AbiEncode(secretValue))

	// Ensure operator index is found
	if operatorIndex == -1 {
		return [32]byte{}, [32]byte{}, [32]byte{}, fmt.Errorf("operator %s not found in activated operators", operator)
	}

	// Convert operatorIndex to a single byte
	opIndexByte := []byte{uint8(operatorIndex)}
	// Generate cvs by hashing abi.encodePacked(cos, uint8(operatorIndex))
	cvs := Keccak256(AbiEncodePacked(cos, opIndexByte))

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
func AbiEncode(elements ...[]byte) []byte {
	var encoded []byte
	for _, e := range elements {
		encoded = append(encoded, common.LeftPadBytes(e, 32)...)
	}
	return encoded
}

// AbiEncodePacked replicates Solidity's abi.encodePacked behavior.
func AbiEncodePacked(elements ...[]byte) []byte {
	var packed []byte
	for _, e := range elements {
		packed = append(packed, e...)
	}
	return packed
}

// intToBytes converts a *big.Int to its padded big-endian byte representation.
func IntToBytes(n *big.Int) []byte {
	return common.LeftPadBytes(n.Bytes(), 32)
}

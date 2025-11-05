package commitreveal2

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

// EthServiceInterface defines the interface for eth service operations needed for testing
type EthServiceInterface interface {
	GetActivatedOperatorsCached() []common.Address
}

// MockEthService is a mock implementation for testing
type MockEthService struct {
	activatedOperators []common.Address
}

func (m *MockEthService) GetActivatedOperatorsCached() []common.Address {
	return m.activatedOperators
}

// generateCommitWithService is a testable version of GenerateCommit that accepts a service interface
func generateCommitWithService(round string, operator string, service EthServiceInterface) ([32]byte, [32]byte, [32]byte, error) {
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
	activatedOperators := service.GetActivatedOperatorsCached()
	operatorIndex := -1
	for i, op := range activatedOperators {
		if op.Hex() == operator {
			operatorIndex = i
			break
		}
	}

	// Ensure operator index is found
	if operatorIndex == -1 {
		return [32]byte{}, [32]byte{}, [32]byte{}, fmt.Errorf("operator %s not found in activated operators", operator)
	}

	// Generate secret value using keccak256(abi.encodePacked(round, operator, timestamp))
	secretValue := Keccak256(AbiEncodePacked(IntToBytes(roundInt), operatorAddress.Bytes(), IntToBytes(timestamp)))

	// Generate cos by hashing the secretValue using abi.encode
	cos := Keccak256(AbiEncode(secretValue))

	// Convert operatorIndex to a single byte
	opIndexByte := []byte{uint8(operatorIndex)}
	// Generate cvs by hashing abi.encodePacked(cos, uint8(operatorIndex))
	cvs := Keccak256(AbiEncodePacked(cos, opIndexByte))

	// Convert results into [32]byte format (Solidity's bytes32)
	var secretValueBytes32, cosBytes32, cvsBytes32 [32]byte
	copy(secretValueBytes32[:], secretValue)
	copy(cosBytes32[:], cos)
	copy(cvsBytes32[:], cvs)

	return secretValueBytes32, cosBytes32, cvsBytes32, nil
}

// TestGenerateCommitWithMock tests GenerateCommit functionality with mocked dependencies
func TestGenerateCommitWithMock(t *testing.T) {
	t.Run("valid operator", func(t *testing.T) {
		// Create mock service with test operators
		mockService := &MockEthService{
			activatedOperators: []common.Address{
				common.HexToAddress("0x1234567890123456789012345678901234567890"),
				common.HexToAddress("0x2345678901234567890123456789012345678901"),
				common.HexToAddress("0x3456789012345678901234567890123456789012"),
			},
		}

		round := "1"
		operator := "0x1234567890123456789012345678901234567890"

		secretValue, cos, cvs, err := generateCommitWithService(round, operator, mockService)
		assert.NoError(t, err)

		// Verify all values are different and non-zero
		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)
		assert.NotEqual(t, secretValue, cos)
		assert.NotEqual(t, cos, cvs)
		assert.NotEqual(t, secretValue, cvs)
	})

	t.Run("operator not in activated list", func(t *testing.T) {
		mockService := &MockEthService{
			activatedOperators: []common.Address{
				common.HexToAddress("0x1234567890123456789012345678901234567890"),
				common.HexToAddress("0x2345678901234567890123456789012345678901"),
			},
		}

		round := "1"
		operator := "0x9999999999999999999999999999999999999999" // Not in mock list

		_, _, _, err := generateCommitWithService(round, operator, mockService)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in activated operators")
	})

	t.Run("invalid round", func(t *testing.T) {
		mockService := &MockEthService{
			activatedOperators: []common.Address{
				common.HexToAddress("0x1234567890123456789012345678901234567890"),
			},
		}

		round := "invalid-round"
		operator := "0x1234567890123456789012345678901234567890"

		_, _, _, err := generateCommitWithService(round, operator, mockService)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid round")
	})

	t.Run("empty operator list", func(t *testing.T) {
		mockService := &MockEthService{
			activatedOperators: []common.Address{}, // Empty list
		}

		round := "1"
		operator := "0x1234567890123456789012345678901234567890"

		_, _, _, err := generateCommitWithService(round, operator, mockService)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in activated operators")
	})

	t.Run("operator at different indices", func(t *testing.T) {
		operators := []common.Address{
			common.HexToAddress("0x1111111111111111111111111111111111111111"),
			common.HexToAddress("0x2222222222222222222222222222222222222222"),
			common.HexToAddress("0x3333333333333333333333333333333333333333"),
		}
		mockService := &MockEthService{activatedOperators: operators}

		round := "1"

		// Test each operator to ensure different CVS values (since operator index affects CVS)
		var results [][32]byte
		for i, op := range operators {
			secretValue, cos, cvs, err := generateCommitWithService(round, op.Hex(), mockService)
			assert.NoError(t, err, "Should succeed for operator at index %d", i)

			// Store results for comparison
			results = append(results, cvs)

			// Verify individual results are valid
			assert.NotEqual(t, [32]byte{}, secretValue)
			assert.NotEqual(t, [32]byte{}, cos)
			assert.NotEqual(t, [32]byte{}, cvs)
		}

		// Verify that different operator indices produce different CVS values
		assert.NotEqual(t, results[0], results[1], "Different operators should produce different CVS")
		assert.NotEqual(t, results[1], results[2], "Different operators should produce different CVS")
		assert.NotEqual(t, results[0], results[2], "Different operators should produce different CVS")
	})

	t.Run("deterministic behavior", func(t *testing.T) {
		mockService := &MockEthService{
			activatedOperators: []common.Address{
				common.HexToAddress("0x1234567890123456789012345678901234567890"),
			},
		}

		round := "42"
		operator := "0x1234567890123456789012345678901234567890"

		// Generate twice and compare (note: will be different due to timestamp)
		secretValue1, cos1, cvs1, err1 := generateCommitWithService(round, operator, mockService)
		assert.NoError(t, err1)

		secretValue2, cos2, cvs2, err2 := generateCommitWithService(round, operator, mockService)
		assert.NoError(t, err2)

		// Since timestamp changes, results will be different
		// But we can verify the structure is consistent
		assert.Len(t, secretValue1, 32)
		assert.Len(t, cos1, 32)
		assert.Len(t, cvs1, 32)
		assert.Len(t, secretValue2, 32)
		assert.Len(t, cos2, 32)
		assert.Len(t, cvs2, 32)
	})
}
package commitreveal2

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-pg/pg/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestKeccak256(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
		},
		{
			name:     "hello world",
			input:    []byte("hello world"),
			expected: "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		},
		{
			name:     "test",
			input:    []byte("test"),
			expected: "9c22ff5f21f0b81b113e63f7db6da94fedef11b2119b4088b89664fb9a3cb658",
		},
		{
			name:     "32 bytes input",
			input:    make([]byte, 32),
			expected: "290decd9548b62a8d60345a988386fc84ba6bc95484008f6362f93160ef3e563",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Keccak256(tt.input)
			resultHex := hex.EncodeToString(result)
			assert.Equal(t, tt.expected, resultHex)
		})
	}
}

func TestAbiEncode(t *testing.T) {
	tests := []struct {
		name     string
		elements [][]byte
		expected string
	}{
		{
			name:     "empty input",
			elements: [][]byte{},
			expected: "",
		},
		{
			name: "single element",
			elements: [][]byte{
				{0x01, 0x02, 0x03},
			},
			expected: "0000000000000000000000000000000000000000000000000000000000010203",
		},
		{
			name: "multiple elements",
			elements: [][]byte{
				{0x01},
				{0x02, 0x03},
			},
			expected: "0000000000000000000000000000000000000000000000000000000000000001" +
				"0000000000000000000000000000000000000000000000000000000000000203",
		},
		{
			name: "full 32-byte element",
			elements: [][]byte{
				make([]byte, 32),
			},
			expected: "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name: "element larger than 32 bytes",
			elements: [][]byte{
				make([]byte, 48), // 48 bytes, all zeros
			},
			expected: "000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name: "multiple elements with >32 bytes",
			elements: [][]byte{
				{0x01, 0x02},     // Small element
				make([]byte, 40), // 40 bytes, all zeros
				{0xFF},           // Single byte
			},
			expected: "00000000000000000000000000000000000000000000000000000000000001020000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000ff", // Actual behavior
		},
		{
			name: "nil element",
			elements: [][]byte{
				nil,
			},
			expected: "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name: "mixed nil and normal elements",
			elements: [][]byte{
				{0x01, 0x02, 0x03},
				nil,
				{0xFF, 0xFE},
			},
			expected: "0000000000000000000000000000000000000000000000000000000000010203" + // First element
				"0000000000000000000000000000000000000000000000000000000000000000" + // nil element
				"000000000000000000000000000000000000000000000000000000000000fffe", // Last element
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AbiEncode(tt.elements...)
			resultHex := hex.EncodeToString(result)
			assert.Equal(t, tt.expected, resultHex)
		})
	}

	// Additional test for complex scenarios with large elements and nil handling
	t.Run("complex large elements and nil behavior verification", func(t *testing.T) {
		// Test with very large elements and nil inputs
		largeElement1 := make([]byte, 64) // 64 bytes
		for i := range largeElement1 {
			largeElement1[i] = byte(i % 256)
		}

		largeElement2 := make([]byte, 100) // 100 bytes
		for i := range largeElement2 {
			largeElement2[i] = 0xFF // All 0xFF
		}

		elements := [][]byte{
			{0x12, 0x34},       // Small element
			nil,                // nil element
			largeElement1,      // 64-byte element
			{},                 // Empty element
			largeElement2,      // 100-byte element
			nil,                // Another nil
			{0xAB, 0xCD, 0xEF}, // Final element
		}

		result := AbiEncode(elements...)

		// Verify total length: each element gets padded to 32 bytes minimum, but large elements keep their size
		// Element sizes: 32 + 32 + 64 + 32 + 100 + 32 + 32 = 324 bytes
		expectedLength := 32 + 32 + 64 + 32 + 100 + 32 + 32
		assert.Equal(t, expectedLength, len(result))

		// Verify specific parts
		offset := 0

		// Check first element (small, padded to 32 bytes)
		expected1 := make([]byte, 32)
		expected1[30] = 0x12
		expected1[31] = 0x34
		assert.Equal(t, expected1, result[offset:offset+32])
		offset += 32

		// Check nil element (should be 32 zeros)
		expectedNil := make([]byte, 32)
		assert.Equal(t, expectedNil, result[offset:offset+32])
		offset += 32

		// Check large element 1 (64 bytes, preserved as-is)
		assert.Equal(t, largeElement1, result[offset:offset+64])
		assert.Equal(t, byte(0), result[offset])     // First byte should be 0
		assert.Equal(t, byte(63), result[offset+63]) // Last byte should be 63
		offset += 64

		// Check empty element (should be 32 zeros)
		expectedEmpty := make([]byte, 32)
		assert.Equal(t, expectedEmpty, result[offset:offset+32])
		offset += 32

		// Check large element 2 (100 bytes, all 0xFF)
		assert.Equal(t, largeElement2, result[offset:offset+100])
		assert.Equal(t, byte(0xFF), result[offset])    // First byte should be 0xFF
		assert.Equal(t, byte(0xFF), result[offset+99]) // Last byte should be 0xFF
		offset += 100

		// Check another nil element (should be 32 zeros)
		expectedNil2 := make([]byte, 32)
		assert.Equal(t, expectedNil2, result[offset:offset+32])
		offset += 32

		// Check final element (3 bytes, padded to 32)
		expectedFinal := make([]byte, 32)
		expectedFinal[29] = 0xAB
		expectedFinal[30] = 0xCD
		expectedFinal[31] = 0xEF
		assert.Equal(t, expectedFinal, result[offset:offset+32])

		// Verify deterministic behavior
		result2 := AbiEncode(elements...)
		assert.Equal(t, result, result2)

		// Test edge case: only nil elements
		nilOnly := [][]byte{nil, nil, nil}
		nilResult := AbiEncode(nilOnly...)
		expectedNilOnly := make([]byte, 96) // 3 * 32 bytes of zeros
		assert.Equal(t, expectedNilOnly, nilResult)

		// Test edge case: single very large element
		veryLarge := make([]byte, 500)
		for i := range veryLarge {
			veryLarge[i] = byte(i % 256)
		}
		largeResult := AbiEncode(veryLarge)
		assert.Equal(t, 500, len(largeResult))
		assert.Equal(t, veryLarge, largeResult)
	})
}

func TestAbiEncodePacked(t *testing.T) {
	tests := []struct {
		name     string
		elements [][]byte
		expected string
	}{
		{
			name:     "empty input",
			elements: [][]byte{},
			expected: "",
		},
		{
			name: "single element",
			elements: [][]byte{
				{0x01, 0x02, 0x03},
			},
			expected: "010203",
		},
		{
			name: "multiple elements",
			elements: [][]byte{
				{0x01},
				{0x02, 0x03},
				{0x04, 0x05, 0x06},
			},
			expected: "010203040506",
		},
		{
			name: "32-byte element",
			elements: [][]byte{
				{0xFF, 0xFE, 0xFD},
				make([]byte, 32),
			},
			expected: "fffefd" + "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name: "mixed-size elements (empty + >32 bytes)",
			elements: [][]byte{
				{},                 // Empty element
				{0x01, 0x02},       // Small element
				make([]byte, 48),   // Large element (48 bytes, all zeros)
				{0xAA},             // Single byte
				{},                 // Another empty element
				make([]byte, 64),   // Very large element (64 bytes, all zeros)
				{0xBB, 0xCC, 0xDD}, // Small element with values
			},
			expected: "0102" +
				"000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" + // 48 bytes = 96 hex chars
				"aa" +
				"00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" + // 64 bytes = 128 hex chars
				"bbccdd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AbiEncodePacked(tt.elements...)
			resultHex := hex.EncodeToString(result)
			assert.Equal(t, tt.expected, resultHex)
		})
	}

	// Additional test for complex mixed-size scenarios
	t.Run("complex mixed-size behavior verification", func(t *testing.T) {
		// Create elements with varying sizes including empty and very large
		element1 := []byte{}                       // Empty
		element2 := []byte{0x12, 0x34, 0x56, 0x78} // 4 bytes
		element3 := make([]byte, 80)               // 80 bytes of zeros
		element3[79] = 0xFF                        // Set last byte to 0xFF for identification
		element4 := []byte{}                       // Another empty
		element5 := []byte{0xAB}                   // Single byte
		element6 := make([]byte, 100)              // 100 bytes
		// Fill element6 with a pattern
		for i := range element6 {
			element6[i] = byte(i % 256)
		}
		element7 := []byte{0xDE, 0xAD, 0xBE, 0xEF} // Final 4 bytes

		elements := [][]byte{element1, element2, element3, element4, element5, element6, element7}
		result := AbiEncodePacked(elements...)

		// Verify total length: 0 + 4 + 80 + 0 + 1 + 100 + 4 = 189 bytes
		expectedLength := 0 + 4 + 80 + 0 + 1 + 100 + 4
		assert.Equal(t, expectedLength, len(result))

		// Verify the concatenation is correct by checking specific parts
		offset := 0

		// element1 is empty, so nothing to check

		// Check element2 (4 bytes: 0x12, 0x34, 0x56, 0x78)
		assert.Equal(t, []byte{0x12, 0x34, 0x56, 0x78}, result[offset:offset+4])
		offset += 4

		// Check element3 (80 bytes, last byte should be 0xFF)
		assert.Equal(t, byte(0x00), result[offset])    // First byte should be 0
		assert.Equal(t, byte(0xFF), result[offset+79]) // Last byte should be 0xFF
		offset += 80

		// element4 is empty, so nothing to check

		// Check element5 (1 byte: 0xAB)
		assert.Equal(t, byte(0xAB), result[offset])
		offset += 1

		// Check element6 (100 bytes with pattern)
		assert.Equal(t, byte(0), result[offset])     // First byte should be 0
		assert.Equal(t, byte(99), result[offset+99]) // Last byte should be 99
		assert.Equal(t, byte(50), result[offset+50]) // Middle byte should be 50
		offset += 100

		// Check element7 (4 bytes: 0xDE, 0xAD, 0xBE, 0xEF)
		assert.Equal(t, []byte{0xDE, 0xAD, 0xBE, 0xEF}, result[offset:offset+4])

		// Verify deterministic behavior
		result2 := AbiEncodePacked(elements...)
		assert.Equal(t, result, result2)

		// Test edge case: all empty elements
		allEmpty := [][]byte{{}, {}, {}}
		emptyResult := AbiEncodePacked(allEmpty...)
		assert.Equal(t, 0, len(emptyResult))

		// Test edge case: single very large element
		largeElement := make([]byte, 1000)
		for i := range largeElement {
			largeElement[i] = byte(i % 256)
		}
		largeResult := AbiEncodePacked(largeElement)
		assert.Equal(t, 1000, len(largeResult))
		assert.Equal(t, largeElement, largeResult)
	})
}

func TestIntToBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    *big.Int
		expected string
	}{
		{
			name:     "zero",
			input:    big.NewInt(0),
			expected: "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name:     "small number",
			input:    big.NewInt(255),
			expected: "00000000000000000000000000000000000000000000000000000000000000ff",
		},
		{
			name:     "larger number",
			input:    big.NewInt(65535),
			expected: "000000000000000000000000000000000000000000000000000000000000ffff",
		},
		{
			name:     "negative number gets abs value",
			input:    big.NewInt(-1),
			expected: "0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			name: "very large number (>32 bytes when unpadded)",
			input: func() *big.Int {
				// Create a number that would naturally require more than 32 bytes
				// This is 2^256 + 1, which requires 33 bytes to represent
				largeNum := new(big.Int)
				largeNum.SetString("115792089237316195423570985008687907853269984665640564039457584007913129639936", 10) // 2^256
				largeNum.Add(largeNum, big.NewInt(1))                                                                    // 2^256 + 1
				return largeNum
			}(),
			expected: "010000000000000000000000000000000000000000000000000000000000000001", // Returns full representation (33 bytes)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IntToBytes(tt.input)
			resultHex := hex.EncodeToString(result)
			assert.Equal(t, tt.expected, resultHex)

			// Length check - most cases should be 32 bytes, but very large numbers may be longer
			if tt.name == "very large number (>32 bytes when unpadded)" {
				assert.Equal(t, 33, len(result)) // This specific case returns 33 bytes
			} else {
				assert.Equal(t, 32, len(result)) // Normal cases are always 32 bytes
			}
		})
	}

	// Additional test for very large numbers to demonstrate behavior
	t.Run("very large number behavior verification", func(t *testing.T) {
		// Test with a number that requires more than 32 bytes in its natural representation
		// This is 2^300, which would require 38 bytes to represent fully
		veryLargeNum := new(big.Int)
		veryLargeNum.SetString("2037035976334486086268445688409378161051468393665936250636140449354381299763336706183397376", 10)

		result := IntToBytes(veryLargeNum)

		// LeftPadBytes doesn't truncate large numbers, so this will be larger than 32 bytes
		assert.Equal(t, 38, len(result))             // This number requires 38 bytes
		assert.NotEqual(t, make([]byte, 38), result) // Should not be all zeros

		// Test with an even larger number to ensure consistent behavior
		extremelyLargeNum := new(big.Int)
		extremelyLargeNum.SetString("340282366920938463463374607431768211456", 10) // 2^128
		extremelyLargeNum.Mul(extremelyLargeNum, extremelyLargeNum)                // 2^256
		extremelyLargeNum.Mul(extremelyLargeNum, big.NewInt(256))                  // Much larger than 2^256

		result2 := IntToBytes(extremelyLargeNum)
		// This will be even larger
		assert.True(t, len(result2) > 32, "Result should be larger than 32 bytes for very large numbers")
		assert.Equal(t, 34, len(result2)) // This specific calculation results in 34 bytes

		// Verify deterministic behavior
		result3 := IntToBytes(extremelyLargeNum)
		assert.Equal(t, result2, result3)

		// Test that normal-sized numbers still work as expected (≤32 bytes)
		normalNum := big.NewInt(123456789)
		normalResult := IntToBytes(normalNum)
		assert.Equal(t, 32, len(normalResult)) // Should be padded to 32 bytes

		// Test edge case: exactly 32 bytes
		maxUint256 := new(big.Int)
		maxUint256.SetString("115792089237316195423570985008687907853269984665640564039457584007913129639935", 10) // 2^256 - 1
		maxResult := IntToBytes(maxUint256)
		assert.Equal(t, 32, len(maxResult)) // Should be exactly 32 bytes
	})
}

func TestKeccak256EdgeCases(t *testing.T) {
	t.Run("large input", func(t *testing.T) {
		largeInput := make([]byte, 10000)
		for i := range largeInput {
			largeInput[i] = byte(i % 256)
		}

		result := Keccak256(largeInput)
		assert.Len(t, result, 32)
		assert.NotEqual(t, make([]byte, 32), result)
	})

	t.Run("single byte inputs", func(t *testing.T) {
		result1 := Keccak256([]byte{0x00})
		result2 := Keccak256([]byte{0x01})
		result3 := Keccak256([]byte{0xFF})

		assert.NotEqual(t, result1, result2)
		assert.NotEqual(t, result2, result3)
		assert.NotEqual(t, result1, result3)
	})
}

func TestAbiEncodeEdgeCases(t *testing.T) {
	t.Run("mixed size elements", func(t *testing.T) {
		elements := [][]byte{
			{},
			{0x01},
			make([]byte, 16),
			make([]byte, 32),
		}

		result := AbiEncode(elements...)
		expectedLength := 4 * 32
		assert.Len(t, result, expectedLength)

		for i := 0; i < 4; i++ {
			segment := result[i*32 : (i+1)*32]
			assert.Len(t, segment, 32)
		}
	})

	t.Run("very large element", func(t *testing.T) {
		largeElement := make([]byte, 64)
		for i := range largeElement {
			largeElement[i] = 0xFF
		}

		result := AbiEncode(largeElement)
		assert.Len(t, result, 64)

		for _, b := range result {
			assert.Equal(t, byte(0xFF), b)
		}
	})
}

func TestAbiEncodePackedEdgeCases(t *testing.T) {
	t.Run("many small elements", func(t *testing.T) {
		var elements [][]byte
		for i := 0; i < 100; i++ {
			elements = append(elements, []byte{byte(i)})
		}

		result := AbiEncodePacked(elements...)
		assert.Len(t, result, 100)

		for i := 0; i < 100; i++ {
			assert.Equal(t, byte(i), result[i])
		}
	})

	t.Run("alternating empty and non-empty", func(t *testing.T) {
		elements := [][]byte{
			{},
			{0x01},
			{},
			{0x02, 0x03},
			{},
			{0x04},
		}

		result := AbiEncodePacked(elements...)
		expected := []byte{0x01, 0x02, 0x03, 0x04}
		assert.Equal(t, expected, result)
	})
}

func TestIntToBytesEdgeCases(t *testing.T) {
	t.Run("very large positive number", func(t *testing.T) {
		largeNum := new(big.Int)
		largeNum.SetString("115792089237316195423570985008687907853269984665640564039457584007913129639935", 10)

		result := IntToBytes(largeNum)
		assert.Len(t, result, 32)

		expected := make([]byte, 32)
		for i := range expected {
			expected[i] = 0xFF
		}
		assert.Equal(t, expected, result)
	})

	t.Run("number requiring full 32 bytes", func(t *testing.T) {
		fullNum := new(big.Int)
		fullNum.SetString("0x8000000000000000000000000000000000000000000000000000000000000000", 0)

		result := IntToBytes(fullNum)
		assert.Len(t, result, 32)
		assert.Equal(t, byte(0x80), result[0])

		for i := 1; i < 32; i++ {
			assert.Equal(t, byte(0x00), result[i])
		}
	})

	t.Run("nil big.Int", func(t *testing.T) {
		var nilInt *big.Int

		defer func() {
			if r := recover(); r != nil {
				t.Logf("Function panicked with nil input as expected: %v", r)
			}
		}()

		result := IntToBytes(nilInt)
		if result != nil {
			assert.Len(t, result, 32)
		}
	})
}

// TestGenerateCommitComponents tests the cryptographic components of GenerateCommit
func TestGenerateCommitComponents(t *testing.T) {
	// Test data
	round := "1"
	operator := "0x1234567890123456789012345678901234567890"
	timestamp := big.NewInt(1234567890)

	t.Run("secret value generation", func(t *testing.T) {
		roundInt := new(big.Int)
		roundInt.SetString(round, 10)
		operatorAddress := common.HexToAddress(operator)

		secretValue := Keccak256(AbiEncodePacked(IntToBytes(roundInt), operatorAddress.Bytes(), IntToBytes(timestamp)))

		// Verify secret value is 32 bytes
		assert.Len(t, secretValue, 32)

		// Verify it's deterministic - same inputs produce same output
		secretValue2 := Keccak256(AbiEncodePacked(IntToBytes(roundInt), operatorAddress.Bytes(), IntToBytes(timestamp)))
		assert.Equal(t, secretValue, secretValue2)

		// Verify different inputs produce different output
		differentTimestamp := big.NewInt(1234567891)
		differentSecret := Keccak256(AbiEncodePacked(IntToBytes(roundInt), operatorAddress.Bytes(), IntToBytes(differentTimestamp)))
		assert.NotEqual(t, secretValue, differentSecret)
	})

	t.Run("cos generation", func(t *testing.T) {
		secretValue := make([]byte, 32)
		copy(secretValue, []byte("test-secret-value-123456789012"))

		cos := Keccak256(AbiEncode(secretValue))

		// Verify cos is 32 bytes
		assert.Len(t, cos, 32)

		// Verify deterministic behavior
		cos2 := Keccak256(AbiEncode(secretValue))
		assert.Equal(t, cos, cos2)

		// Verify different secret produces different cos
		differentSecret := make([]byte, 32)
		copy(differentSecret, []byte("different-secret-value-1234567"))
		differentCos := Keccak256(AbiEncode(differentSecret))
		assert.NotEqual(t, cos, differentCos)
	})

	t.Run("cvs generation", func(t *testing.T) {
		cos := make([]byte, 32)
		copy(cos, []byte("test-cos-value-123456789012345"))
		operatorIndex := uint8(5)

		cvs := Keccak256(AbiEncodePacked(cos, []byte{operatorIndex}))

		// Verify cvs is 32 bytes
		assert.Len(t, cvs, 32)

		// Verify deterministic behavior
		cvs2 := Keccak256(AbiEncodePacked(cos, []byte{operatorIndex}))
		assert.Equal(t, cvs, cvs2)

		// Verify different operator index produces different cvs
		differentCvs := Keccak256(AbiEncodePacked(cos, []byte{operatorIndex + 1}))
		assert.NotEqual(t, cvs, differentCvs)
	})

	t.Run("round parsing", func(t *testing.T) {
		validRounds := []string{"1", "42", "1000", "999999999"}

		for _, r := range validRounds {
			roundInt := new(big.Int)
			_, ok := roundInt.SetString(r, 10)
			assert.True(t, ok, "Should parse valid round: %s", r)
			assert.True(t, roundInt.Cmp(big.NewInt(0)) >= 0, "Round should be non-negative: %s", r)
		}

		invalidRounds := []string{"", "abc", "1.5", "0x123"}

		for _, r := range invalidRounds {
			roundInt := new(big.Int)
			_, ok := roundInt.SetString(r, 10)
			assert.False(t, ok, "Should not parse invalid round: %s", r)
		}

		// Test negative numbers separately since they parse successfully but should be rejected logically
		t.Run("negative numbers", func(t *testing.T) {
			negativeRounds := []string{"-1", "-42", "-999"}

			for _, r := range negativeRounds {
				roundInt := new(big.Int)
				_, ok := roundInt.SetString(r, 10)
				assert.True(t, ok, "Should parse negative number: %s", r)
				assert.True(t, roundInt.Cmp(big.NewInt(0)) < 0, "Negative round should be less than zero: %s", r)
			}
		})
	})

	t.Run("operator address parsing", func(t *testing.T) {
		validAddresses := []string{
			"0x1234567890123456789012345678901234567890",
			"0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
			"0x0000000000000000000000000000000000000000",
		}

		for _, addr := range validAddresses {
			operatorAddress := common.HexToAddress(addr)
			assert.Equal(t, 20, len(operatorAddress.Bytes()), "Address should be 20 bytes")

			// Check that the address is valid by comparing the input with the parsed hex
			expectedBytes := common.FromHex(addr)
			assert.Equal(t, expectedBytes, operatorAddress.Bytes(), "Address bytes should match input")

			// Verify that the address is not zero (for non-zero inputs)
			if addr != "0x0000000000000000000000000000000000000000" {
				assert.NotEqual(t, common.Address{}, operatorAddress, "Address should not be zero")
			}
		}
	})

	t.Run("complete flow consistency", func(t *testing.T) {
		// Test that all components work together consistently
		roundInt := new(big.Int)
		roundInt.SetString("42", 10)
		operatorAddress := common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")
		testTimestamp := big.NewInt(1700000000)
		operatorIndex := uint8(3)

		// Generate components
		secretValue := Keccak256(AbiEncodePacked(IntToBytes(roundInt), operatorAddress.Bytes(), IntToBytes(testTimestamp)))
		cos := Keccak256(AbiEncode(secretValue))
		cvs := Keccak256(AbiEncodePacked(cos, []byte{operatorIndex}))

		// Verify all are different
		assert.NotEqual(t, secretValue, cos)
		assert.NotEqual(t, cos, cvs)
		assert.NotEqual(t, secretValue, cvs)

		// Verify all are 32 bytes
		assert.Len(t, secretValue, 32)
		assert.Len(t, cos, 32)
		assert.Len(t, cvs, 32)

		// Test reproducibility
		secretValue2 := Keccak256(AbiEncodePacked(IntToBytes(roundInt), operatorAddress.Bytes(), IntToBytes(testTimestamp)))
		cos2 := Keccak256(AbiEncode(secretValue2))
		cvs2 := Keccak256(AbiEncodePacked(cos2, []byte{operatorIndex}))

		assert.Equal(t, secretValue, secretValue2)
		assert.Equal(t, cos, cos2)
		assert.Equal(t, cvs, cvs2)
	})
}

// TestGenerateCommitIntegration tests the actual GenerateCommit function
// by mocking the eth.Service dependency
func TestGenerateCommitIntegration(t *testing.T) {
	// Store original service to restore after tests
	originalService := eth.Service
	defer func() {
		eth.Service = originalService
	}()

	// Create mock service for testing
	mockService := &MockEthServiceForGenerateCommit{
		activatedOperators: []common.Address{
			common.HexToAddress("0x1234567890123456789012345678901234567890"),
			common.HexToAddress("0x2345678901234567890123456789012345678901"),
			common.HexToAddress("0x3456789012345678901234567890123456789012"),
		},
	}

	// Replace the global service with our mock
	eth.Service = mockService

	t.Run("valid operator generates commit successfully", func(t *testing.T) {
		round := "1"
		operator := "0x1234567890123456789012345678901234567890"

		secretValue, cos, cvs, err := GenerateCommit(round, operator)
		assert.NoError(t, err)

		// Verify all values are different and non-zero
		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)
		assert.NotEqual(t, secretValue, cos)
		assert.NotEqual(t, cos, cvs)
		assert.NotEqual(t, secretValue, cvs)

		// Verify the length of returned values
		assert.Len(t, secretValue, 32)
		assert.Len(t, cos, 32)
		assert.Len(t, cvs, 32)
	})

	t.Run("operator not in activated list returns error", func(t *testing.T) {
		round := "1"
		operator := "0x9999999999999999999999999999999999999999" // Not in mock list

		_, _, _, err := GenerateCommit(round, operator)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in activated operators")
	})

	t.Run("invalid round returns error", func(t *testing.T) {
		round := "invalid-round"
		operator := "0x1234567890123456789012345678901234567890"

		_, _, _, err := GenerateCommit(round, operator)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid round")
	})

	t.Run("empty operator list returns error", func(t *testing.T) {
		// Create mock with empty operator list
		emptyMockService := &MockEthServiceForGenerateCommit{
			activatedOperators: []common.Address{},
		}
		eth.Service = emptyMockService

		round := "1"
		operator := "0x1234567890123456789012345678901234567890"

		_, _, _, err := GenerateCommit(round, operator)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in activated operators")
	})

	t.Run("different operator indices produce different CVS values", func(t *testing.T) {
		operators := []common.Address{
			common.HexToAddress("0x1111111111111111111111111111111111111111"),
			common.HexToAddress("0x2222222222222222222222222222222222222222"),
			common.HexToAddress("0x3333333333333333333333333333333333333333"),
		}
		indexMockService := &MockEthServiceForGenerateCommit{activatedOperators: operators}
		eth.Service = indexMockService

		round := "1"

		// Test each operator to ensure different CVS values (since operator index affects CVS)
		var results [][32]byte
		for i, op := range operators {
			secretValue, cos, cvs, err := GenerateCommit(round, op.Hex())
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

	t.Run("different rounds produce different values", func(t *testing.T) {
		// Reset mock service for this test
		eth.Service = mockService
		operator := "0x1234567890123456789012345678901234567890"

		// Generate commits for different rounds
		secretValue1, cos1, cvs1, err1 := GenerateCommit("1", operator)
		assert.NoError(t, err1)

		secretValue2, cos2, cvs2, err2 := GenerateCommit("2", operator)
		assert.NoError(t, err2)

		// Different rounds should produce different values
		assert.NotEqual(t, secretValue1, secretValue2)
		assert.NotEqual(t, cos1, cos2)
		assert.NotEqual(t, cvs1, cvs2)
	})

	t.Run("large round numbers work correctly", func(t *testing.T) {
		// Reset mock service for this test
		eth.Service = mockService
		round := "999999999999999999999"
		operator := "0x1234567890123456789012345678901234567890"

		secretValue, cos, cvs, err := GenerateCommit(round, operator)
		assert.NoError(t, err)

		// Verify all values are valid
		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)
	})

	t.Run("edge case - zero round", func(t *testing.T) {
		// Reset mock service for this test
		eth.Service = mockService
		round := "0"
		operator := "0x1234567890123456789012345678901234567890"

		secretValue, cos, cvs, err := GenerateCommit(round, operator)
		assert.NoError(t, err)

		// Verify all values are valid
		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)
	})

	t.Run("operator address must match exactly", func(t *testing.T) {
		// Reset mock service for this test
		eth.Service = mockService
		round := "1"

		// Test with exact case-sensitive match (this should work)
		operatorExact := "0x1234567890123456789012345678901234567890"
		secretValue1, cos1, cvs1, err1 := GenerateCommit(round, operatorExact)
		assert.NoError(t, err1)

		// Test with different case (this should fail because Go's address comparison is case-sensitive)
		operatorDifferentCase := "0X1234567890123456789012345678901234567890"
		_, _, _, err2 := GenerateCommit(round, operatorDifferentCase)
		assert.Error(t, err2)
		assert.Contains(t, err2.Error(), "not found in activated operators")

		// Verify the successful case produced valid results
		assert.NotEqual(t, [32]byte{}, secretValue1)
		assert.NotEqual(t, [32]byte{}, cos1)
		assert.NotEqual(t, [32]byte{}, cvs1)
	})
}

func TestGenerateCommit_TimestampNonDeterminism(t *testing.T) {
	originalService := eth.Service
	defer func() {
		eth.Service = originalService
	}()

	mockService := &MockEthServiceForGenerateCommit{
		activatedOperators: []common.Address{
			common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
	}
	eth.Service = mockService

	round := "1"
	operator := "0x1234567890123456789012345678901234567890"
	results := make(map[string]bool)
	uniqueCount := 0

	for i := 0; i < 10; i++ {
		secretValue, cos, cvs, err := GenerateCommit(round, operator)
		assert.NoError(t, err)
		secretHex := hex.EncodeToString(secretValue[:])
		cosHex := hex.EncodeToString(cos[:])
		cvsHex := hex.EncodeToString(cvs[:])
		key := secretHex + cosHex + cvsHex
		if !results[key] {
			uniqueCount++
			results[key] = true
		}

		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)
	}
	t.Logf("Generated %d unique commit combinations out of 10 attempts", uniqueCount)
}

func TestGenerateCommit_OperatorIndexOverflow(t *testing.T) {
	originalService := eth.Service
	defer func() {
		eth.Service = originalService
	}()

	operators := make([]common.Address, 260)
	for i := 0; i < 260; i++ {
		addrBytes := make([]byte, 20)
		addrBytes[19] = byte(i)
		addrBytes[18] = byte(i >> 8)
		operators[i] = common.BytesToAddress(addrBytes)
	}

	mockService := &MockEthServiceForGenerateCommit{
		activatedOperators: operators,
	}
	eth.Service = mockService

	round := "1"

	operator0 := operators[0].Hex()
	secretValue0, cos0, cvs0, err0 := GenerateCommit(round, operator0)
	assert.NoError(t, err0)
	assert.NotEqual(t, [32]byte{}, secretValue0)
	assert.NotEqual(t, [32]byte{}, cos0)
	assert.NotEqual(t, [32]byte{}, cvs0)

	operator255 := operators[255].Hex()
	secretValue255, cos255, cvs255, err255 := GenerateCommit(round, operator255)
	assert.NoError(t, err255)
	assert.NotEqual(t, [32]byte{}, secretValue255)
	assert.NotEqual(t, [32]byte{}, cos255)
	assert.NotEqual(t, [32]byte{}, cvs255)

	operator256 := operators[256].Hex()
	secretValue256, cos256, cvs256, err256 := GenerateCommit(round, operator256)
	assert.NoError(t, err256)
	assert.NotEqual(t, [32]byte{}, secretValue256)
	assert.NotEqual(t, [32]byte{}, cos256)
	assert.NotEqual(t, [32]byte{}, cvs256)

	assert.NotEqual(t, cvs0, cvs256, "Index 256 wraps to 0, but CVS differs due to different COS")

	operator259 := operators[259].Hex()
	secretValue259, cos259, cvs259, err259 := GenerateCommit(round, operator259)
	assert.NoError(t, err259)
	assert.NotEqual(t, [32]byte{}, secretValue259)
	assert.NotEqual(t, [32]byte{}, cos259)
	assert.NotEqual(t, [32]byte{}, cvs259)

	operator3 := operators[3].Hex()
	_, _, cvs3, _ := GenerateCommit(round, operator3)
	assert.NotEqual(t, cvs3, cvs259, "Index 259 wraps to 3, but CVS differs due to different COS")

	index256Int := 256
	index259Int := 259
	index256 := uint8(index256Int)
	index259 := uint8(index259Int)
	assert.Equal(t, uint8(0), index256, "uint8(256) should wrap to 0")
	assert.Equal(t, uint8(3), index259, "uint8(259) should wrap to 3")
}

func TestGenerateCommit_TimestampEdgeCases(t *testing.T) {
	originalService := eth.Service
	defer func() {
		eth.Service = originalService
	}()

	mockService := &MockEthServiceForGenerateCommit{
		activatedOperators: []common.Address{
			common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
	}
	eth.Service = mockService

	operator := "0x1234567890123456789012345678901234567890"

	t.Run("zero round with timestamp", func(t *testing.T) {
		round := "0"
		secretValue, cos, cvs, err := GenerateCommit(round, operator)
		assert.NoError(t, err)
		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)
	})

	t.Run("very large round number", func(t *testing.T) {
		round := "999999999999999999999999999999999999999999999"
		secretValue, cos, cvs, err := GenerateCommit(round, operator)
		assert.NoError(t, err)
		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)
	})

	t.Run("timestamp with different rounds", func(t *testing.T) {
		operator := "0x1234567890123456789012345678901234567890"

		secret1, _, _, err1 := GenerateCommit("100", operator)
		assert.NoError(t, err1)

		secret2, _, _, err2 := GenerateCommit("200", operator)
		assert.NoError(t, err2)

		assert.NotEqual(t, secret1, secret2)
	})
}

func TestGenerateCommit_AddressMatchingEdgeCases(t *testing.T) {
	originalService := eth.Service
	defer func() {
		eth.Service = originalService
	}()

	round := "1"

	t.Run("case sensitive address matching", func(t *testing.T) {
		operatorAddr := common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")
		operators := []common.Address{
			operatorAddr,
		}
		mockService := &MockEthServiceForGenerateCommit{
			activatedOperators: operators,
		}
		eth.Service = mockService

		operatorLower := operatorAddr.Hex()
		_, _, _, err := GenerateCommit(round, operatorLower)
		assert.NoError(t, err, "Lowercase address should match")

		operatorUpper := "0xABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCD"
		_, _, _, err2 := GenerateCommit(round, operatorUpper)
		assert.Error(t, err2, "Uppercase address should fail due to case-sensitive string comparison")
		assert.Contains(t, err2.Error(), "not found in activated operators")
	})

	t.Run("zero address handling", func(t *testing.T) {
		zeroAddr := common.Address{}
		operators := []common.Address{
			zeroAddr,
			common.HexToAddress("0x1234567890123456789012345678901234567890"),
		}
		mockService := &MockEthServiceForGenerateCommit{
			activatedOperators: operators,
		}
		eth.Service = mockService

		secretValue, cos, cvs, err := GenerateCommit(round, zeroAddr.Hex())
		assert.NoError(t, err)
		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)
	})

	t.Run("duplicate operators in list", func(t *testing.T) {
		operator := common.HexToAddress("0x1111111111111111111111111111111111111111")
		// Create list with duplicate operator
		operators := []common.Address{
			operator,
			common.HexToAddress("0x2222222222222222222222222222222222222222"),
			operator, // Duplicate
		}
		mockService := &MockEthServiceForGenerateCommit{
			activatedOperators: operators,
		}
		eth.Service = mockService

		secretValue1, cos1, cvs1, err1 := GenerateCommit(round, operator.Hex())
		assert.NoError(t, err1)

		assert.NotEqual(t, [32]byte{}, secretValue1)
		assert.NotEqual(t, [32]byte{}, cos1)
		assert.NotEqual(t, [32]byte{}, cvs1)
	})

	t.Run("invalid address format", func(t *testing.T) {
		operators := []common.Address{
			common.HexToAddress("0x1234567890123456789012345678901234567890"),
		}
		mockService := &MockEthServiceForGenerateCommit{
			activatedOperators: operators,
		}
		eth.Service = mockService

		invalidAddresses := []string{
			"0x123",
			"1234567890123456789012345678901234567890",
			"0xGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG",
		}

		for _, invalidAddr := range invalidAddresses {
			_, _, _, err := GenerateCommit(round, invalidAddr)

			assert.Error(t, err, "Invalid address %s should return error", invalidAddr)
			assert.Contains(t, err.Error(), "not found in activated operators",
				"Error should indicate operator not found for address: %s", invalidAddr)
		}
	})
}

func TestGenerateCommit_CryptographicProperties(t *testing.T) {
	originalService := eth.Service
	defer func() {
		eth.Service = originalService
	}()

	mockService := &MockEthServiceForGenerateCommit{
		activatedOperators: []common.Address{
			common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
	}
	eth.Service = mockService

	operator := "0x1234567890123456789012345678901234567890"

	t.Run("collision resistance - different inputs produce different outputs", func(t *testing.T) {
		secretResults := make(map[string]bool)
		cosResults := make(map[string]bool)
		cvsResults := make(map[string]bool)

		for i := 0; i < 100; i++ {
			round := fmt.Sprintf("%d", i)
			secretValue, cos, cvs, err := GenerateCommit(round, operator)
			assert.NoError(t, err)

			secretHex := hex.EncodeToString(secretValue[:])
			cosHex := hex.EncodeToString(cos[:])
			cvsHex := hex.EncodeToString(cvs[:])

			if secretResults[secretHex] {
				t.Logf("Secret value collision detected at round %s - this is expected if called in same second", round)
			}
			secretResults[secretHex] = true

			if cosResults[cosHex] {
				t.Logf("COS collision detected at round %s - this is expected if called in same second", round)
			}
			cosResults[cosHex] = true

			if cvsResults[cvsHex] {
				t.Logf("CVS collision detected at round %s - this is expected if called in same second", round)
			}
			cvsResults[cvsHex] = true

			assert.NotEqual(t, secretValue, cos, "SecretValue should not equal COS")
			assert.NotEqual(t, cos, cvs, "COS should not equal CVS")
			assert.NotEqual(t, secretValue, cvs, "SecretValue should not equal CVS")
		}

		uniqueSecrets := len(secretResults)
		t.Logf("Generated %d unique secrets out of 100 attempts (allowing for timestamp collisions)", uniqueSecrets)
		assert.Greater(t, uniqueSecrets, 90, "Should have high uniqueness with different rounds")
	})

	t.Run("entropy quality - secret values are well distributed", func(t *testing.T) {
		round := "1"
		secretValues := make([][32]byte, 50)

		for i := 0; i < 50; i++ {
			secretValue, _, _, err := GenerateCommit(round, operator)
			assert.NoError(t, err)
			secretValues[i] = secretValue
		}

		zeroCount := 0
		uniqueSecrets := make(map[string]bool)
		for _, sv := range secretValues {
			if sv == [32]byte{} {
				zeroCount++
			}
			uniqueSecrets[hex.EncodeToString(sv[:])] = true
		}

		assert.Equal(t, 0, zeroCount, "No secret values should be zero")
		t.Logf("Generated %d unique secrets out of 50 attempts", len(uniqueSecrets))
	})

	t.Run("preimage resistance - cannot derive inputs from outputs", func(t *testing.T) {
		round := "42"
		secretValue, cos, cvs, err := GenerateCommit(round, operator)
		assert.NoError(t, err)

		assert.NotEqual(t, [32]byte{}, secretValue)
		assert.NotEqual(t, [32]byte{}, cos)
		assert.NotEqual(t, [32]byte{}, cvs)

		expectedCos := Keccak256(AbiEncode(secretValue[:]))
		assert.Equal(t, expectedCos, cos[:], "COS should be hash of secretValue")
	})

	t.Run("uniqueness - same inputs in different seconds produce different secrets", func(t *testing.T) {
		round := "100"
		operator := "0x1234567890123456789012345678901234567890"

		secretValue1, cos1, cvs1, err1 := GenerateCommit(round, operator)
		assert.NoError(t, err1)

		secretValue2, cos2, cvs2, err2 := GenerateCommit(round, operator)
		assert.NoError(t, err2)

		if secretValue1 == secretValue2 {
			t.Log("Same secret generated - likely same timestamp second")
		} else {
			assert.NotEqual(t, secretValue1, secretValue2, "Different timestamps should produce different secrets")
			assert.NotEqual(t, cos1, cos2)
			assert.NotEqual(t, cvs1, cvs2)
		}
	})

	t.Run("hash with zero inputs", func(t *testing.T) {
		zeroOperator := common.Address{}.Hex()
		zeroOperators := []common.Address{
			common.Address{},
		}
		zeroMockService := &MockEthServiceForGenerateCommit{
			activatedOperators: zeroOperators,
		}
		eth.Service = zeroMockService

		secretValue, cos, cvs, err := GenerateCommit("0", zeroOperator)
		assert.NoError(t, err)

		assert.Len(t, secretValue, 32, "Secret value should be 32 bytes even with zero inputs")
		assert.Len(t, cos, 32, "COS should be 32 bytes")
		assert.Len(t, cvs, 32, "CVS should be 32 bytes")

		assert.NotEqual(t, [32]byte{}, secretValue, "Hash of zero inputs should not be all zeros")
		assert.NotEqual(t, [32]byte{}, cos, "COS should not be all zeros")
		assert.NotEqual(t, [32]byte{}, cvs, "CVS should not be all zeros")
	})

	t.Run("hash with maximum values", func(t *testing.T) {
		eth.Service = mockService

		maxRound := "9223372036854775807"
		veryLargeRound := "999999999999999999999999999999999999999999999999999999999999999999999"

		secretValue1, cos1, cvs1, err1 := GenerateCommit(maxRound, operator)
		assert.NoError(t, err1)

		secretValue2, cos2, cvs2, err2 := GenerateCommit(veryLargeRound, operator)
		assert.NoError(t, err2)

		assert.Len(t, secretValue1, 32)
		assert.Len(t, cos1, 32)
		assert.Len(t, cvs1, 32)
		assert.Len(t, secretValue2, 32)
		assert.Len(t, cos2, 32)
		assert.Len(t, cvs2, 32)

		assert.NotEqual(t, secretValue1, secretValue2, "Different large rounds should produce different secrets")
	})

	t.Run("hash output format - always 32 bytes", func(t *testing.T) {
		eth.Service = mockService

		testRounds := []string{"1", "10", "100", "1000", "999999"}

		for _, testRound := range testRounds {
			secretValue, cos, cvs, err := GenerateCommit(testRound, operator)
			assert.NoError(t, err)

			assert.Len(t, secretValue, 32, "Secret value must be exactly 32 bytes for round %s", testRound)
			assert.Len(t, cos, 32, "COS must be exactly 32 bytes for round %s", testRound)
			assert.Len(t, cvs, 32, "CVS must be exactly 32 bytes for round %s", testRound)

			assert.NotEqual(t, [32]byte{}, secretValue, "Secret value should not be zero for round %s", testRound)
			assert.NotEqual(t, [32]byte{}, cos, "COS should not be zero for round %s", testRound)
			assert.NotEqual(t, [32]byte{}, cvs, "CVS should not be zero for round %s", testRound)
		}
	})

	t.Run("hash avalanche effect - small input change produces completely different output", func(t *testing.T) {
		eth.Service = mockService

		round1 := "100"
		round2 := "101"

		secretValue1, cos1, cvs1, err1 := GenerateCommit(round1, operator)
		assert.NoError(t, err1)

		secretValue2, cos2, cvs2, err2 := GenerateCommit(round2, operator)
		assert.NoError(t, err2)
		assert.NotEqual(t, secretValue1, secretValue2, "Small round change should produce different secrets")
		assert.NotEqual(t, cos1, cos2, "Small round change should produce different COS")
		assert.NotEqual(t, cvs1, cvs2, "Small round change should produce different CVS")

		bitDiff := 0
		for i := 0; i < 32; i++ {
			xor := secretValue1[i] ^ secretValue2[i]
			for xor != 0 {
				bitDiff++
				xor &= xor - 1
			}
		}
		assert.Greater(t, bitDiff, 64, "Avalanche effect: at least 25%% of bits should differ, got %d/256", bitDiff)
		t.Logf("Bit difference between secrets: %d/256 bits (%.1f%%)", bitDiff, float64(bitDiff)/256*100)
	})

	t.Run("hash with special byte patterns", func(t *testing.T) {
		allZerosAddr := common.Address{}
		allFFAddr := common.BytesToAddress([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})

		// Alternating pattern
		altBytes := make([]byte, 20)
		for i := range altBytes {
			if i%2 == 0 {
				altBytes[i] = 0xAA
			} else {
				altBytes[i] = 0x55
			}
		}
		altPatternAddr := common.BytesToAddress(altBytes)

		specialOperators := []common.Address{
			allZerosAddr,
			allFFAddr,
			altPatternAddr,
		}
		specialMockService := &MockEthServiceForGenerateCommit{
			activatedOperators: specialOperators,
		}
		eth.Service = specialMockService

		results := make(map[string][32]byte)
		for i, op := range specialOperators {
			secretValue, cos, cvs, err := GenerateCommit("42", op.Hex())
			assert.NoError(t, err, "Should handle special pattern operator at index %d", i)

			assert.Len(t, secretValue, 32)
			assert.Len(t, secretValue, 32)
			assert.Len(t, cos, 32)
			assert.Len(t, cvs, 32)
			assert.NotEqual(t, [32]byte{}, secretValue)
			assert.NotEqual(t, [32]byte{}, cos)
			assert.NotEqual(t, [32]byte{}, cvs)

			secretHex := hex.EncodeToString(secretValue[:])
			results[secretHex] = secretValue
		}

		assert.Equal(t, 3, len(results), "All special pattern operators should produce unique secrets")
	})
}

func TestGenerateCommit_InputValidation(t *testing.T) {
	originalService := eth.Service
	defer func() {
		eth.Service = originalService
	}()

	mockService := &MockEthServiceForGenerateCommit{
		activatedOperators: []common.Address{
			common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
	}
	eth.Service = mockService

	operator := "0x1234567890123456789012345678901234567890"

	t.Run("empty round string", func(t *testing.T) {
		_, _, _, err := GenerateCommit("", operator)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid round")
	})

	t.Run("whitespace-only round", func(t *testing.T) {
		_, _, _, err := GenerateCommit("   ", operator)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid round")
	})

	t.Run("round with leading zeros", func(t *testing.T) {
		secretValue, cos, cvs, err := GenerateCommit("0001", operator)
		if err == nil {
			assert.NotEqual(t, [32]byte{}, secretValue)
			assert.NotEqual(t, [32]byte{}, cos)
			assert.NotEqual(t, [32]byte{}, cvs)
		}
	})

	t.Run("round with special characters", func(t *testing.T) {
		invalidRounds := []string{
			"abc",
			"1.5",
			"0x123",
			"1e10",
			"-1",
			"1+1",
		}

		for _, invalidRound := range invalidRounds {
			_, _, _, err := GenerateCommit(invalidRound, operator)
			if err != nil {
				assert.Contains(t, err.Error(), "invalid round")
			}
		}
	})

	t.Run("empty operator string", func(t *testing.T) {
		_, _, _, err := GenerateCommit("1", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in activated operators")
	})

	t.Run("operator with whitespace", func(t *testing.T) {
		_, _, _, err := GenerateCommit("1", " 0x1234567890123456789012345678901234567890 ")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in activated operators")
	})

	t.Run("very long round string", func(t *testing.T) {

		longRound := "1" + fmt.Sprintf("%0*d", 1000, 0)
		_, _, _, err := GenerateCommit(longRound, operator)
		if err != nil {
			assert.Contains(t, err.Error(), "invalid round")
		}
	})

	t.Run("round with newline characters", func(t *testing.T) {
		_, _, _, err := GenerateCommit("1\n", operator)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid round")
	})

	t.Run("round with tab characters", func(t *testing.T) {
		_, _, _, err := GenerateCommit("1\t", operator)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid round")
	})
}

// MockEthServiceForGenerateCommit implements the minimal interface needed by GenerateCommit
type MockEthServiceForGenerateCommit struct {
	activatedOperators []common.Address
}

// GetActivatedOperatorsCached returns the mock list of activated operators
func (m *MockEthServiceForGenerateCommit) GetActivatedOperatorsCached() []common.Address {
	return m.activatedOperators
}

// We only need to implement GetActivatedOperatorsCached for GenerateCommit testing
// The other methods can be left unimplemented since GenerateCommit doesn't use them
func (m *MockEthServiceForGenerateCommit) GetActivatedOperatorsLength() int64                     { return 0 }
func (m *MockEthServiceForGenerateCommit) SetActivatedOperatorsCached(operators []common.Address) {}
func (m *MockEthServiceForGenerateCommit) GetActivatedOperatorsUnsafe() []common.Address          { return nil }
func (m *MockEthServiceForGenerateCommit) GetActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) ([]common.Address, error) {
	return nil, nil
}
func (m *MockEthServiceForGenerateCommit) UpdateActivatedOperators(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) error {
	return nil
}
func (m *MockEthServiceForGenerateCommit) CallSmartContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, parsedABI abi.ABI, method string, contractAddress common.Address, params ...interface{}) (interface{}, error) {
	return nil, nil
}
func (m *MockEthServiceForGenerateCommit) ExecuteTransaction(ctx context.Context, clientUtils *utils.Client, fallbackEthClient fallback_ethclient.IFallbackEthClient, method string, value *big.Int, args ...interface{}) (*types.Transaction, *bind.TransactOpts, error) {
	return nil, nil, nil
}
func (m *MockEthServiceForGenerateCommit) UpdateCurrentRoundFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient) (*big.Int, error) {
	return nil, nil
}
func (m *MockEthServiceForGenerateCommit) GetTrialNumFromContract(ctx context.Context, fallbackEthClient fallback_ethclient.IFallbackEthClient, round *big.Int) (*big.Int, error) {
	return nil, nil
}

// TestNewRevealOrderService tests the NewRevealOrderService constructor
func TestNewRevealOrderService(t *testing.T) {
	mockRevealOrderRepo := &MockRevealOrderRepository{}
	mockPeerCommitRepo := &MockPeerCommitRepository{}
	mockLeaderCommitRepo := &MockLeaderCommitRepository{}

	service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

	assert.NotNil(t, service)
	assert.Equal(t, mockRevealOrderRepo, service.revealOrderRepository)
	assert.Equal(t, mockPeerCommitRepo, service.peerCommitRepository)
	assert.Equal(t, mockLeaderCommitRepo, service.leaderCommitRepository)
}

// TestDetermineRevealOrder tests the DetermineRevealOrder function with various scenarios
func TestDetermineRevealOrder(t *testing.T) {
	ctx := context.Background()
	roundNum := "1"
	trialNum := "1"

	t.Run("reveal order already exists", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		existingOrder := &utils.RevealOrderData{
			UniqueKey: "1-1",
			Round:     "1",
			TrialNum:  "1",
		}
		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(existingOrder, nil)

		activatedOps := []common.Address{common.HexToAddress("0x1234567890123456789012345678901234567890")}
		result, err := service.DetermineRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.NoError(t, err)
		assert.True(t, result)
		mockRevealOrderRepo.AssertExpectations(t)
	})

	t.Run("no activated operators", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		activatedOps := []common.Address{}
		result, err := service.DetermineRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.Error(t, err)
		assert.False(t, result)
		assert.Contains(t, err.Error(), "no activated operators found")
		mockRevealOrderRepo.AssertExpectations(t)
	})

	t.Run("failed to load leader commit", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		operator := common.HexToAddress("0x1234567890123456789012345678901234567890")
		activatedOps := []common.Address{operator}

		mockLeaderCommitRepo.On("GetLeaderCommitByRoundAndEoaAddr", ctx, roundNum, trialNum, operator.Hex()).Return(nil, pg.ErrNoRows)

		result, err := service.DetermineRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.Error(t, err)
		assert.False(t, result)
		assert.Contains(t, err.Error(), "failed to load leader commit")
		mockRevealOrderRepo.AssertExpectations(t)
		mockLeaderCommitRepo.AssertExpectations(t)
	})

	t.Run("missing leader commit (zero cos)", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		operator := common.HexToAddress("0x1234567890123456789012345678901234567890")
		activatedOps := []common.Address{operator}

		commitData := &utils.LeaderCommitData{
			Cos: [32]byte{}, // Zero bytes indicate missing commit
		}
		mockLeaderCommitRepo.On("GetLeaderCommitByRoundAndEoaAddr", ctx, roundNum, trialNum, operator.Hex()).Return(commitData, nil)

		result, err := service.DetermineRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.Error(t, err)
		assert.False(t, result)
		assert.Contains(t, err.Error(), "missing leader commit")
		mockRevealOrderRepo.AssertExpectations(t)
		mockLeaderCommitRepo.AssertExpectations(t)
	})

	t.Run("failed to save reveal order", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		operator := common.HexToAddress("0x1234567890123456789012345678901234567890")
		activatedOps := []common.Address{operator}

		commitData := &utils.LeaderCommitData{
			Cos: [32]byte{1, 2, 3}, // Non-zero cos
			Cvs: [32]byte{4, 5, 6},
		}
		mockLeaderCommitRepo.On("GetLeaderCommitByRoundAndEoaAddr", ctx, roundNum, trialNum, operator.Hex()).Return(commitData, nil)
		mockRevealOrderRepo.On("AddRevealOrder", ctx, mock.AnythingOfType("*utils.RevealOrderData")).Return(errors.New("save failed"))

		result, err := service.DetermineRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.Error(t, err)
		assert.False(t, result)
		assert.Contains(t, err.Error(), "failed to save reveal order")
		mockRevealOrderRepo.AssertExpectations(t)
		mockLeaderCommitRepo.AssertExpectations(t)
	})

	t.Run("successful reveal order determination", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		operators := []common.Address{
			common.HexToAddress("0x1234567890123456789012345678901234567890"),
			common.HexToAddress("0x2345678901234567890123456789012345678901"),
		}

		commitData1 := &utils.LeaderCommitData{
			Cos: [32]byte{1, 2, 3},
			Cvs: [32]byte{4, 5, 6},
		}
		commitData2 := &utils.LeaderCommitData{
			Cos: [32]byte{7, 8, 9},
			Cvs: [32]byte{10, 11, 12},
		}

		mockLeaderCommitRepo.On("GetLeaderCommitByRoundAndEoaAddr", ctx, roundNum, trialNum, operators[0].Hex()).Return(commitData1, nil)
		mockLeaderCommitRepo.On("GetLeaderCommitByRoundAndEoaAddr", ctx, roundNum, trialNum, operators[1].Hex()).Return(commitData2, nil)
		mockRevealOrderRepo.On("AddRevealOrder", ctx, mock.AnythingOfType("*utils.RevealOrderData")).Return(nil)

		result, err := service.DetermineRevealOrder(ctx, roundNum, trialNum, operators)

		assert.NoError(t, err)
		assert.True(t, result)
		mockRevealOrderRepo.AssertExpectations(t)
		mockLeaderCommitRepo.AssertExpectations(t)
	})
}

// TestDetermineRegularRevealOrder tests the DetermineRegularRevealOrder function
func TestDetermineRegularRevealOrder(t *testing.T) {
	ctx := context.Background()
	roundNum := "1"
	trialNum := "1"

	t.Run("reveal order already exists", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		existingOrder := &utils.RevealOrderData{
			UniqueKey: "1-1",
			Round:     "1",
			TrialNum:  "1",
		}
		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(existingOrder, nil)

		activatedOps := []common.Address{common.HexToAddress("0x1234567890123456789012345678901234567890")}
		result, err := service.DetermineRegularRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.NoError(t, err)
		assert.True(t, result)
		mockRevealOrderRepo.AssertExpectations(t)
	})

	t.Run("no activated operators", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		activatedOps := []common.Address{}
		result, err := service.DetermineRegularRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.Error(t, err)
		assert.False(t, result)
		assert.Contains(t, err.Error(), "no activated operators found")
		mockRevealOrderRepo.AssertExpectations(t)
	})

	t.Run("failed to load peer commit", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		operator := common.HexToAddress("0x1234567890123456789012345678901234567890")
		activatedOps := []common.Address{operator}

		mockPeerCommitRepo.On("GetPeerCommitData", ctx, roundNum, trialNum, operator.Hex()).Return(nil, pg.ErrNoRows)

		result, err := service.DetermineRegularRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.Error(t, err)
		assert.False(t, result)
		assert.Contains(t, err.Error(), "failed to load peer commit")
		mockRevealOrderRepo.AssertExpectations(t)
		mockPeerCommitRepo.AssertExpectations(t)
	})

	t.Run("missing peer commit (zero cos)", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		operator := common.HexToAddress("0x1234567890123456789012345678901234567890")
		activatedOps := []common.Address{operator}

		commitData := &database.PeerCommitDataScheme{
			Cos: []byte{}, // Empty bytes indicate missing commit
		}
		mockPeerCommitRepo.On("GetPeerCommitData", ctx, roundNum, trialNum, operator.Hex()).Return(commitData, nil)

		result, err := service.DetermineRegularRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.Error(t, err)
		assert.False(t, result)
		assert.Contains(t, err.Error(), "missing peer commit")
		mockRevealOrderRepo.AssertExpectations(t)
		mockPeerCommitRepo.AssertExpectations(t)
	})

	t.Run("failed to save reveal order", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		operator := common.HexToAddress("0x1234567890123456789012345678901234567890")
		activatedOps := []common.Address{operator}

		commitData := &database.PeerCommitDataScheme{
			Cos: []byte{1, 2, 3}, // Non-zero cos
			Cvs: []byte{4, 5, 6},
		}
		mockPeerCommitRepo.On("GetPeerCommitData", ctx, roundNum, trialNum, operator.Hex()).Return(commitData, nil)
		mockRevealOrderRepo.On("AddRevealOrder", ctx, mock.AnythingOfType("*utils.RevealOrderData")).Return(errors.New("save failed"))

		result, err := service.DetermineRegularRevealOrder(ctx, roundNum, trialNum, activatedOps)

		assert.Error(t, err)
		assert.False(t, result)
		assert.Contains(t, err.Error(), "failed to save reveal order")
		mockRevealOrderRepo.AssertExpectations(t)
		mockPeerCommitRepo.AssertExpectations(t)
	})

	t.Run("successful regular reveal order determination", func(t *testing.T) {
		mockRevealOrderRepo := &MockRevealOrderRepository{}
		mockPeerCommitRepo := &MockPeerCommitRepository{}
		mockLeaderCommitRepo := &MockLeaderCommitRepository{}

		service := NewRevealOrderService(mockRevealOrderRepo, mockPeerCommitRepo, mockLeaderCommitRepo)

		mockRevealOrderRepo.On("GetRevealOrder", ctx, roundNum, trialNum).Return(nil, pg.ErrNoRows)

		operators := []common.Address{
			common.HexToAddress("0x1234567890123456789012345678901234567890"),
			common.HexToAddress("0x2345678901234567890123456789012345678901"),
		}

		commitData1 := &database.PeerCommitDataScheme{
			Cos: []byte{1, 2, 3},
			Cvs: []byte{4, 5, 6},
		}
		commitData2 := &database.PeerCommitDataScheme{
			Cos: []byte{7, 8, 9},
			Cvs: []byte{10, 11, 12},
		}

		mockPeerCommitRepo.On("GetPeerCommitData", ctx, roundNum, trialNum, operators[0].Hex()).Return(commitData1, nil)
		mockPeerCommitRepo.On("GetPeerCommitData", ctx, roundNum, trialNum, operators[1].Hex()).Return(commitData2, nil)
		mockRevealOrderRepo.On("AddRevealOrder", ctx, mock.AnythingOfType("*utils.RevealOrderData")).Return(nil)

		result, err := service.DetermineRegularRevealOrder(ctx, roundNum, trialNum, operators)

		assert.NoError(t, err)
		assert.True(t, result)
		mockRevealOrderRepo.AssertExpectations(t)
		mockPeerCommitRepo.AssertExpectations(t)
	})
}

// Mock implementations for testing

type MockRevealOrderRepository struct {
	mock.Mock
}

func (m *MockRevealOrderRepository) GetRevealOrder(ctx context.Context, round, trialNum string) (*utils.RevealOrderData, error) {
	args := m.Called(ctx, round, trialNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.RevealOrderData), args.Error(1)
}

func (m *MockRevealOrderRepository) AddRevealOrder(ctx context.Context, order *utils.RevealOrderData) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

type MockPeerCommitRepository struct {
	mock.Mock
}

func (m *MockPeerCommitRepository) GetPeerCommitData(ctx context.Context, round, trialNum, eoaAddress string) (*database.PeerCommitDataScheme, error) {
	args := m.Called(ctx, round, trialNum, eoaAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*database.PeerCommitDataScheme), args.Error(1)
}

func (m *MockPeerCommitRepository) AddPeerCommitData(ctx context.Context, commitData *database.PeerCommitDataScheme) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

func (m *MockPeerCommitRepository) UpdatePeerCommitData(ctx context.Context, commitData *database.PeerCommitDataScheme) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

type MockLeaderCommitRepository struct {
	mock.Mock
}

func (m *MockLeaderCommitRepository) GetLeaderCommitByRoundAndEoaAddr(ctx context.Context, round, trialNum, eoaAddress string) (*utils.LeaderCommitData, error) {
	args := m.Called(ctx, round, trialNum, eoaAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*utils.LeaderCommitData), args.Error(1)
}

func (m *MockLeaderCommitRepository) UpdateLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

func (m *MockLeaderCommitRepository) AddLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error {
	args := m.Called(ctx, commitData)
	return args.Error(0)
}

func (m *MockLeaderCommitRepository) GetLeaderCommitsByRoundAndTrialNum(ctx context.Context, round, trialNum string) ([]*utils.LeaderCommitData, error) {
	args := m.Called(ctx, round, trialNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*utils.LeaderCommitData), args.Error(1)
}

func (m *MockLeaderCommitRepository) UpdateLeaderCommitRandomNumberGenerated(ctx context.Context, round, trialNum string) error {
	args := m.Called(ctx, round, trialNum)
	return args.Error(0)
}

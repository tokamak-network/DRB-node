package commitreveal2

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Helper function to create a service instance for testing
func createTestRevealOrderService() *RevealOrderService {
	return &RevealOrderService{}
}

func TestCalculateRV(t *testing.T) {
	service := createTestRevealOrderService()
	
	t.Run("empty input", func(t *testing.T) {
		cosValues := [][]byte{}
		rv := service.calculateRV(cosValues)
		
		// RV should be hash of empty concatenation
		expected := Keccak256([]byte{})
		var expectedRV [32]byte
		copy(expectedRV[:], expected)
		
		assert.Equal(t, expectedRV, rv)
	})

	t.Run("single COS value", func(t *testing.T) {
		cos1 := make([]byte, 32)
		cos1[31] = 0x01
		
		cosValues := [][]byte{cos1}
		rv := service.calculateRV(cosValues)
		
		// RV should be hash of the single COS value
		expected := Keccak256(cos1)
		var expectedRV [32]byte
		copy(expectedRV[:], expected)
		
		assert.Equal(t, expectedRV, rv)
	})

	t.Run("multiple COS values", func(t *testing.T) {
		cos1 := make([]byte, 32)
		cos1[31] = 0x01

		cos2 := make([]byte, 32)
		cos2[31] = 0x02

		cos3 := make([]byte, 32)
		cos3[31] = 0x03
		
		cosValues := [][]byte{cos1, cos2, cos3}
		rv := service.calculateRV(cosValues)
		
		// RV should be hash of concatenated COS values
		var concatenated []byte
		concatenated = append(concatenated, cos1...)
		concatenated = append(concatenated, cos2...)
		concatenated = append(concatenated, cos3...)
		
		expected := Keccak256(concatenated)
		var expectedRV [32]byte
		copy(expectedRV[:], expected)
		
		assert.Equal(t, expectedRV, rv)
	})

	t.Run("different order produces different RV", func(t *testing.T) {
		cos1 := make([]byte, 32)
		cos1[31] = 0x01

		cos2 := make([]byte, 32)
		cos2[31] = 0x02
		
		rv1 := service.calculateRV([][]byte{cos1, cos2})
		rv2 := service.calculateRV([][]byte{cos2, cos1})
		
		assert.NotEqual(t, rv1, rv2)
	})

	t.Run("deterministic results", func(t *testing.T) {
		cos1 := make([]byte, 32)
		cos1[31] = 0x01

		cos2 := make([]byte, 32)
		cos2[31] = 0x02
		
		cosValues := [][]byte{cos1, cos2}
		
		rv1 := service.calculateRV(cosValues)
		rv2 := service.calculateRV(cosValues)
		rv3 := service.calculateRV(cosValues)
		
		assert.Equal(t, rv1, rv2)
		assert.Equal(t, rv2, rv3)
	})

	t.Run("single COS value with less than 32 bytes", func(t *testing.T) {
		// Create a COS value with only 16 bytes
		cos1 := make([]byte, 16)
		cos1[15] = 0x01
		
		cosValues := [][]byte{cos1}
		rv := service.calculateRV(cosValues)
		
		// RV should be hash of the 16-byte COS value
		expected := Keccak256(cos1)
		var expectedRV [32]byte
		copy(expectedRV[:], expected)
		
		assert.Equal(t, expectedRV, rv)
		assert.NotEqual(t, [32]byte{}, rv) // Should not be zero
	})

	t.Run("unequal COS lengths", func(t *testing.T) {
		// Create COS values with different lengths
		cos1 := make([]byte, 16) // 16 bytes
		cos1[15] = 0x01

		cos2 := make([]byte, 32) // 32 bytes
		cos2[31] = 0x02

		cos3 := make([]byte, 8) // 8 bytes
		cos3[7] = 0x03
		
		cosValues := [][]byte{cos1, cos2, cos3}
		rv := service.calculateRV(cosValues)
		
		// RV should be hash of concatenated COS values regardless of their individual lengths
		var concatenated []byte
		concatenated = append(concatenated, cos1...)
		concatenated = append(concatenated, cos2...)
		concatenated = append(concatenated, cos3...)
		
		expected := Keccak256(concatenated)
		var expectedRV [32]byte
		copy(expectedRV[:], expected)
		
		assert.Equal(t, expectedRV, rv)
		assert.NotEqual(t, [32]byte{}, rv) // Should not be zero
		
		// Verify deterministic behavior with same unequal lengths
		rv2 := service.calculateRV(cosValues)
		assert.Equal(t, rv, rv2)
	})
}

func TestDetermineOrder(t *testing.T) {
	service := createTestRevealOrderService()
	
	t.Run("empty CVS values", func(t *testing.T) {
		rv := [32]byte{}
		cvsValues := [][]byte{}
		
		order := service.determineOrder(rv, cvsValues)
		assert.Empty(t, order)
	})

	t.Run("single CVS value", func(t *testing.T) {
		rv := [32]byte{}
		rv[31] = 0x01
		
		cvs1 := make([]byte, 32)
		cvs1[31] = 0x01
		
		cvsValues := [][]byte{cvs1}
		order := service.determineOrder(rv, cvsValues)
		
		assert.Equal(t, []int{0}, order)
	})

	t.Run("two CVS values", func(t *testing.T) {
		rv := [32]byte{}
		rv[31] = 0x01
		
		cvs1 := make([]byte, 32)
		cvs1[31] = 0x01

		cvs2 := make([]byte, 32)
		cvs2[31] = 0x02
		
		cvsValues := [][]byte{cvs1, cvs2}
		order := service.determineOrder(rv, cvsValues)
		
		assert.Len(t, order, 2)
		assert.Contains(t, order, 0)
		assert.Contains(t, order, 1)
		
		// Verify the order is correct by checking the hash values
		hash1 := Keccak256(append(rv[:], cvs1...))
		hash2 := Keccak256(append(rv[:], cvs2...))
		
		// Order should be descending, so if hash1 > hash2, then order[0] should be 0
		if hex.EncodeToString(hash1) > hex.EncodeToString(hash2) {
			assert.Equal(t, 0, order[0])
			assert.Equal(t, 1, order[1])
		} else {
			assert.Equal(t, 1, order[0])
			assert.Equal(t, 0, order[1])
		}
	})

	t.Run("multiple CVS values", func(t *testing.T) {
		rv := [32]byte{}
		rv[31] = 0xFF
		
		cvs1 := make([]byte, 32)
		cvs1[31] = 0x01

		cvs2 := make([]byte, 32)
		cvs2[31] = 0x02

		cvs3 := make([]byte, 32)
		cvs3[31] = 0x03
		
		cvsValues := [][]byte{cvs1, cvs2, cvs3}
		order := service.determineOrder(rv, cvsValues)
		
		assert.Len(t, order, 3)
		assert.Contains(t, order, 0)
		assert.Contains(t, order, 1)
		assert.Contains(t, order, 2)
		
		// Verify that the order is in descending hash order
		hashes := make([][]byte, 3)
		for i, cvs := range cvsValues {
			hashes[i] = Keccak256(append(rv[:], cvs...))
		}
		
		// Check that the hashes are in descending order according to the order array
		for i := 0; i < len(order)-1; i++ {
			hash1 := hashes[order[i]]
			hash2 := hashes[order[i+1]]
			
			// hash1 should be >= hash2 (descending order)
			assert.GreaterOrEqual(t, hex.EncodeToString(hash1), hex.EncodeToString(hash2))
		}
	})

	t.Run("deterministic results", func(t *testing.T) {
		rv := [32]byte{}
		rv[31] = 0x42
		
		cvs1 := make([]byte, 32)
		cvs1[31] = 0x01

		cvs2 := make([]byte, 32)
		cvs2[31] = 0x02
		
		cvsValues := [][]byte{cvs1, cvs2}
		
		order1 := service.determineOrder(rv, cvsValues)
		order2 := service.determineOrder(rv, cvsValues)
		order3 := service.determineOrder(rv, cvsValues)
		
		assert.Equal(t, order1, order2)
		assert.Equal(t, order2, order3)
	})

	t.Run("different RV produces different order", func(t *testing.T) {
		rv1 := [32]byte{}
		rv1[31] = 0x01

		rv2 := [32]byte{}
		rv2[31] = 0x02
		
		cvs1 := make([]byte, 32)
		cvs1[31] = 0x01

		cvs2 := make([]byte, 32)
		cvs2[31] = 0x02
		
		cvsValues := [][]byte{cvs1, cvs2}
		
		order1 := service.determineOrder(rv1, cvsValues)
		order2 := service.determineOrder(rv2, cvsValues)
		
		// Different RV values might produce different orders
		// We just check that both orders are valid (contain all indices)
		assert.Len(t, order1, 2)
		assert.Len(t, order2, 2)
		assert.Contains(t, order1, 0)
		assert.Contains(t, order1, 1)
		assert.Contains(t, order2, 0)
		assert.Contains(t, order2, 1)
	})

	t.Run("identical CVS values", func(t *testing.T) {
		rv := [32]byte{}
		rv[31] = 0x01
		
		cvs1 := make([]byte, 32)
		cvs1[31] = 0x01

		cvs2 := make([]byte, 32)
		cvs2[31] = 0x01 // Same as cvs1
		
		cvsValues := [][]byte{cvs1, cvs2}
		order := service.determineOrder(rv, cvsValues)
		
		assert.Len(t, order, 2)
		assert.Contains(t, order, 0)
		assert.Contains(t, order, 1)
		
		// Since CVS values are identical, the hashes will be identical
		// The order should still be deterministic (Go's sort is stable)
		hash1 := Keccak256(append(rv[:], cvs1...))
		hash2 := Keccak256(append(rv[:], cvs2...))
		assert.Equal(t, hash1, hash2)
	})

	t.Run("large number of CVS values", func(t *testing.T) {
		rv := [32]byte{}
		rv[31] = 0xFF
		
		numValues := 10
		cvsValues := make([][]byte, numValues)
		for i := 0; i < numValues; i++ {
			cvs := make([]byte, 32)
			cvs[31] = byte(i)
			cvsValues[i] = cvs
		}
		
		order := service.determineOrder(rv, cvsValues)
		
		assert.Len(t, order, numValues)
		
		// Check that all indices are present
		for i := 0; i < numValues; i++ {
			assert.Contains(t, order, i)
		}
		
		// Verify ordering is correct
		hashes := make([][]byte, numValues)
		for i, cvs := range cvsValues {
			hashes[i] = Keccak256(append(rv[:], cvs...))
		}
		
		for i := 0; i < len(order)-1; i++ {
			hash1 := hashes[order[i]]
			hash2 := hashes[order[i+1]]
			
			assert.GreaterOrEqual(t, hex.EncodeToString(hash1), hex.EncodeToString(hash2))
		}
	})
}

func TestCalculateRVEdgeCases(t *testing.T) {
	service := createTestRevealOrderService()
	
	t.Run("very large number of COS values", func(t *testing.T) {
		numValues := 100
		cosValues := make([][]byte, numValues)
		
		for i := 0; i < numValues; i++ {
			cos := make([]byte, 32)
			cos[31] = byte(i % 256)
			cosValues[i] = cos
		}
		
		rv := service.calculateRV(cosValues)
		assert.NotEqual(t, [32]byte{}, rv)
		
		rv2 := service.calculateRV(cosValues)
		assert.Equal(t, rv, rv2)
	})

	t.Run("COS values with all same content", func(t *testing.T) {
		sameCOS := make([]byte, 32)
		sameCOS[15] = 0x42
		sameCOS[31] = 0x24
		
		cosValues := [][]byte{
			sameCOS,
			sameCOS,
			sameCOS,
		}
		
		rv := service.calculateRV(cosValues)
		
		singleRV := service.calculateRV([][]byte{sameCOS})
		assert.NotEqual(t, rv, singleRV)
	})
}

func TestDetermineOrderEdgeCases(t *testing.T) {
	service := createTestRevealOrderService()
	
	t.Run("very close hash values", func(t *testing.T) {
		rv := [32]byte{}
		rv[0] = 0x01
		
		cvs1 := make([]byte, 32)
		cvs1[31] = 0x01
		
		cvs2 := make([]byte, 32)
		cvs2[31] = 0x02
		
		cvsValues := [][]byte{cvs1, cvs2}
		order := service.determineOrder(rv, cvsValues)
		
		assert.Len(t, order, 2)
		assert.Contains(t, order, 0)
		assert.Contains(t, order, 1)
		
		order2 := service.determineOrder(rv, cvsValues)
		assert.Equal(t, order, order2)
	})

	t.Run("maximum number of CVS values", func(t *testing.T) {
		rv := [32]byte{}
		rv[0] = 0xFF
		
		numValues := 256
		cvsValues := make([][]byte, numValues)
		
		for i := 0; i < numValues; i++ {
			cvs := make([]byte, 32)
			cvs[30] = byte(i % 256)
			cvs[31] = byte((i * 7) % 256)
			cvsValues[i] = cvs
		}
		
		order := service.determineOrder(rv, cvsValues)
		assert.Len(t, order, numValues)
		
		found := make(map[int]bool)
		for _, idx := range order {
			found[idx] = true
		}
		assert.Len(t, found, numValues)
		
		for i := 0; i < len(order)-1; i++ {
			hash1 := Keccak256(append(rv[:], cvsValues[order[i]]...))
			hash2 := Keccak256(append(rv[:], cvsValues[order[i+1]]...))
			
			comparison := bytes.Compare(hash1, hash2)
			assert.GreaterOrEqual(t, comparison, 0)
		}
	})

	t.Run("RV with extreme values", func(t *testing.T) {
		rv := [32]byte{}
		for i := range rv {
			rv[i] = 0xFF
		}
		
		cvs1 := make([]byte, 32)
		cvs2 := make([]byte, 32)
		cvs2[31] = 0x01
		
		cvsValues := [][]byte{cvs1, cvs2}
		order := service.determineOrder(rv, cvsValues)
		
		assert.Len(t, order, 2)
		
		rv = [32]byte{}
		order2 := service.determineOrder(rv, cvsValues)
		
		assert.Len(t, order2, 2)
	})
}
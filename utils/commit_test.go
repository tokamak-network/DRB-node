package utils

import (
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

func TestCommittedNodesThreadSafety(t *testing.T) {
	// Reset global state before test
	CommittedNodesMu.Lock()
	CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	CommittedNodesMu.Unlock()

	uniqueKey := "test-round-1-0"
	address1 := common.HexToAddress("0x1234567890123456789012345678901234567890")
	address2 := common.HexToAddress("0x2345678901234567890123456789012345678901")

	data1 := LeaderCommitData{
		UniqueKey:    uniqueKey,
		Round:        "1",
		TrialNum:     "0",
		EOAAddress:   address1.Hex(),
		CvsHex:       "0x1234",
		CosHex:       "0x5678",
		CreatedAt:    1234567890,
	}

	data2 := LeaderCommitData{
		UniqueKey:    uniqueKey,
		Round:        "1",
		TrialNum:     "0",
		EOAAddress:   address2.Hex(),
		CvsHex:       "0xabcd",
		CosHex:       "0xefgh",
		CreatedAt:    1234567891,
	}

	// Test concurrent writes
	var wg sync.WaitGroup
	numGoroutines := 10

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			if index%2 == 0 {
				SetCommittedNodeData(uniqueKey, address1, data1)
			} else {
				SetCommittedNodeData(uniqueKey, address2, data2)
			}
		}(i)
	}

	wg.Wait()

	// Verify data integrity
	retrievedData1, exists1 := GetCommittedNodeData(uniqueKey, address1)
	assert.True(t, exists1)
	assert.Equal(t, data1, retrievedData1)

	retrievedData2, exists2 := GetCommittedNodeData(uniqueKey, address2)
	assert.True(t, exists2)
	assert.Equal(t, data2, retrievedData2)

	// Test concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			nodes, exists := GetCommittedNodes(uniqueKey)
			assert.True(t, exists)
			assert.Len(t, nodes, 2)
		}()
	}

	wg.Wait()
}

func TestGetCommittedNodes(t *testing.T) {
	// Reset global state
	CommittedNodesMu.Lock()
	CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	CommittedNodesMu.Unlock()

	uniqueKey := "test-round-2-1"

	t.Run("non-existent round", func(t *testing.T) {
		nodes, exists := GetCommittedNodes("non-existent")
		assert.False(t, exists)
		assert.Nil(t, nodes)
	})

	t.Run("existing round with data", func(t *testing.T) {
		address := common.HexToAddress("0x1234567890123456789012345678901234567890")
		data := LeaderCommitData{
			UniqueKey:  uniqueKey,
			Round:      "2",
			TrialNum:   "1",
			EOAAddress: address.Hex(),
			CvsHex:     "0x1234",
		}

		SetCommittedNodeData(uniqueKey, address, data)

		nodes, exists := GetCommittedNodes(uniqueKey)
		assert.True(t, exists)
		assert.Len(t, nodes, 1)
		assert.Equal(t, data, nodes[address])

		// Verify it returns a copy (modifications don't affect original)
		nodes[address] = LeaderCommitData{UniqueKey: "modified"}
		originalData, _ := GetCommittedNodeData(uniqueKey, address)
		assert.Equal(t, data, originalData)
	})
}

func TestSetCommittedNodesRound(t *testing.T) {
	// Reset global state
	CommittedNodesMu.Lock()
	CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	CommittedNodesMu.Unlock()

	uniqueKey := "test-round-3-2"
	address1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	address2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	roundMap := map[common.Address]LeaderCommitData{
		address1: {
			UniqueKey:  uniqueKey,
			Round:      "3",
			TrialNum:   "2",
			EOAAddress: address1.Hex(),
			CvsHex:     "0xaaaa",
		},
		address2: {
			UniqueKey:  uniqueKey,
			Round:      "3",
			TrialNum:   "2",
			EOAAddress: address2.Hex(),
			CvsHex:     "0xbbbb",
		},
	}

	SetCommittedNodesRound(uniqueKey, roundMap)

	retrievedMap, exists := GetCommittedNodes(uniqueKey)
	assert.True(t, exists)
	assert.Len(t, retrievedMap, 2)
	assert.Equal(t, roundMap[address1], retrievedMap[address1])
	assert.Equal(t, roundMap[address2], retrievedMap[address2])
}

func TestGetCommittedNodeData(t *testing.T) {
	// Reset global state
	CommittedNodesMu.Lock()
	CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	CommittedNodesMu.Unlock()

	uniqueKey := "test-round-4-3"
	address := common.HexToAddress("0x3333333333333333333333333333333333333333")

	t.Run("non-existent round", func(t *testing.T) {
		data, exists := GetCommittedNodeData("non-existent", address)
		assert.False(t, exists)
		assert.Equal(t, LeaderCommitData{}, data)
	})

	t.Run("non-existent address in existing round", func(t *testing.T) {
		EnsureCommittedNodesRoundExists(uniqueKey)
		nonExistentAddr := common.HexToAddress("0x4444444444444444444444444444444444444444")
		
		data, exists := GetCommittedNodeData(uniqueKey, nonExistentAddr)
		assert.False(t, exists)
		assert.Equal(t, LeaderCommitData{}, data)
	})

	t.Run("existing data", func(t *testing.T) {
		expectedData := LeaderCommitData{
			UniqueKey:  uniqueKey,
			Round:      "4",
			TrialNum:   "3",
			EOAAddress: address.Hex(),
			CvsHex:     "0xcccc",
		}

		SetCommittedNodeData(uniqueKey, address, expectedData)

		data, exists := GetCommittedNodeData(uniqueKey, address)
		assert.True(t, exists)
		assert.Equal(t, expectedData, data)
	})
}

func TestSetCommittedNodeData(t *testing.T) {
	// Reset global state
	CommittedNodesMu.Lock()
	CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	CommittedNodesMu.Unlock()

	uniqueKey := "test-round-5-4"
	address := common.HexToAddress("0x5555555555555555555555555555555555555555")

	data := LeaderCommitData{
		UniqueKey:  uniqueKey,
		Round:      "5",
		TrialNum:   "4",
		EOAAddress: address.Hex(),
		CvsHex:     "0xdddd",
	}

	// Test setting data when round doesn't exist
	SetCommittedNodeData(uniqueKey, address, data)

	retrievedData, exists := GetCommittedNodeData(uniqueKey, address)
	assert.True(t, exists)
	assert.Equal(t, data, retrievedData)

	// Test updating existing data
	updatedData := data
	updatedData.CvsHex = "0xeeee"
	SetCommittedNodeData(uniqueKey, address, updatedData)

	retrievedData, exists = GetCommittedNodeData(uniqueKey, address)
	assert.True(t, exists)
	assert.Equal(t, updatedData, retrievedData)
}

func TestEnsureCommittedNodesRoundExists(t *testing.T) {
	// Reset global state
	CommittedNodesMu.Lock()
	CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	CommittedNodesMu.Unlock()

	uniqueKey := "test-round-6-5"

	// Ensure round exists when CommittedNodes is nil
	CommittedNodesMu.Lock()
	CommittedNodes = nil
	CommittedNodesMu.Unlock()

	EnsureCommittedNodesRoundExists(uniqueKey)

	CommittedNodesMu.RLock()
	assert.NotNil(t, CommittedNodes)
	assert.NotNil(t, CommittedNodes[uniqueKey])
	CommittedNodesMu.RUnlock()

	// Ensure round exists when CommittedNodes exists but round doesn't
	uniqueKey2 := "test-round-7-6"
	EnsureCommittedNodesRoundExists(uniqueKey2)

	CommittedNodesMu.RLock()
	assert.NotNil(t, CommittedNodes[uniqueKey])
	assert.NotNil(t, CommittedNodes[uniqueKey2])
	CommittedNodesMu.RUnlock()
}

func TestDeleteCommittedNodes(t *testing.T) {
	// Reset global state
	CommittedNodesMu.Lock()
	CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
	CommittedNodesMu.Unlock()

	uniqueKey := "test-round-8-7"
	address := common.HexToAddress("0x6666666666666666666666666666666666666666")

	data := LeaderCommitData{
		UniqueKey:  uniqueKey,
		Round:      "8",
		TrialNum:   "7",
		EOAAddress: address.Hex(),
	}

	// Set some data
	SetCommittedNodeData(uniqueKey, address, data)

	// Verify data exists
	_, exists := GetCommittedNodeData(uniqueKey, address)
	assert.True(t, exists)

	// Delete the round
	DeleteCommittedNodes(uniqueKey)

	// Verify data is deleted
	_, exists = GetCommittedNodes(uniqueKey)
	assert.False(t, exists)

	_, exists = GetCommittedNodeData(uniqueKey, address)
	assert.False(t, exists)
}

func TestConvertByteArray(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected [32]byte
	}{
		{
			name:     "exact 32 bytes",
			input:    make([]byte, 32),
			expected: [32]byte{},
		},
		{
			name:     "less than 32 bytes",
			input:    []byte{1, 2, 3, 4, 5},
			expected: func() [32]byte {
				var arr [32]byte
				copy(arr[:], []byte{1, 2, 3, 4, 5})
				return arr
			}(),
		},
		{
			name:  "more than 32 bytes",
			input: make([]byte, 40),
			expected: func() [32]byte {
				var arr [32]byte
				copy(arr[:], make([]byte, 40))
				return arr
			}(),
		},
		{
			name:     "empty slice",
			input:    []byte{},
			expected: [32]byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertByteArray(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommitDataNilHandlingAdditional(t *testing.T) {
	t.Run("SetCommittedNodesRound with nil CommittedNodes", func(t *testing.T) {
		// Reset global state to nil
		CommittedNodesMu.Lock()
		CommittedNodes = nil
		CommittedNodesMu.Unlock()

		uniqueKey := "test-nil-handling"
		address := common.HexToAddress("0x1234567890123456789012345678901234567890")
		
		roundMap := map[common.Address]LeaderCommitData{
			address: {
				UniqueKey:  uniqueKey,
				Round:      "1",
				TrialNum:   "0",
				EOAAddress: address.Hex(),
			},
		}

		// This should handle the nil case
		SetCommittedNodesRound(uniqueKey, roundMap)

		// Verify it was set correctly
		retrievedMap, exists := GetCommittedNodes(uniqueKey)
		assert.True(t, exists)
		assert.Len(t, retrievedMap, 1)
		assert.Equal(t, roundMap[address], retrievedMap[address])
	})

	t.Run("SetCommittedNodeData with nil maps", func(t *testing.T) {
		// Reset global state to nil
		CommittedNodesMu.Lock()
		CommittedNodes = nil
		CommittedNodesMu.Unlock()

		uniqueKey := "test-nil-node-data"
		address := common.HexToAddress("0x2345678901234567890123456789012345678901")
		
		data := LeaderCommitData{
			UniqueKey:  uniqueKey,
			Round:      "2",
			TrialNum:   "1",
			EOAAddress: address.Hex(),
		}

		// This should handle the nil cases
		SetCommittedNodeData(uniqueKey, address, data)

		// Verify it was set correctly
		retrievedData, exists := GetCommittedNodeData(uniqueKey, address)
		assert.True(t, exists)
		assert.Equal(t, data, retrievedData)
	})

	t.Run("SetCommittedNodeData with nil round map", func(t *testing.T) {
		// Initialize CommittedNodes but not the specific round
		CommittedNodesMu.Lock()
		CommittedNodes = make(map[string]map[common.Address]LeaderCommitData)
		CommittedNodesMu.Unlock()

		uniqueKey := "test-nil-round-map"
		address := common.HexToAddress("0x3456789012345678901234567890123456789012")
		
		data := LeaderCommitData{
			UniqueKey:  uniqueKey,
			Round:      "3",
			TrialNum:   "2",
			EOAAddress: address.Hex(),
		}

		// This should handle the nil round map case
		SetCommittedNodeData(uniqueKey, address, data)

		// Verify it was set correctly
		retrievedData, exists := GetCommittedNodeData(uniqueKey, address)
		assert.True(t, exists)
		assert.Equal(t, data, retrievedData)
	})

	t.Run("EnsureCommittedNodesRoundExists with nil CommittedNodes", func(t *testing.T) {
		// Reset global state to nil
		CommittedNodesMu.Lock()
		CommittedNodes = nil
		CommittedNodesMu.Unlock()

		uniqueKey := "test-ensure-nil"

		// This should handle the nil case
		EnsureCommittedNodesRoundExists(uniqueKey)

		// Verify CommittedNodes was initialized
		CommittedNodesMu.RLock()
		assert.NotNil(t, CommittedNodes)
		assert.NotNil(t, CommittedNodes[uniqueKey])
		CommittedNodesMu.RUnlock()
	})
}
package regular_node

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test regular node
func createTestRegularNode() *RegularNode {
	return NewRegularNode(nil, nil, nil, nil, nil, nil, nil, nil)
}

func TestRegularNode_Execution(t *testing.T) {
	node := createTestRegularNode()

	// Test default value
	assert.False(t, node.GetExecution(), "Execution should be false by default")

	// Test setting to true
	node.SetExecution(true)
	assert.True(t, node.GetExecution(), "Execution should be true after setting")

	// Test setting to false
	node.SetExecution(false)
	assert.False(t, node.GetExecution(), "Execution should be false after resetting")
}

func TestRegularNode_LeaderMonitoringActive(t *testing.T) {
	node := createTestRegularNode()

	// Test default value
	assert.False(t, node.GetLeaderMonitoringActive(), "LeaderMonitoringActive should be false by default")

	// Test setting to true
	node.SetLeaderMonitoringActive(true)
	assert.True(t, node.GetLeaderMonitoringActive(), "LeaderMonitoringActive should be true after setting")

	// Test setting to false
	node.SetLeaderMonitoringActive(false)
	assert.False(t, node.GetLeaderMonitoringActive(), "LeaderMonitoringActive should be false after resetting")
}

func TestRegularNode_MerkleRootSubmittedEventEmitted(t *testing.T) {
	node := createTestRegularNode()

	// Test default value
	assert.False(t, node.GetMerkleRootSubmittedEventEmitted(), "MerkleRootSubmittedEventEmitted should be false by default")

	// Test setting to true
	node.SetMerkleRootSubmittedEventEmitted(true)
	assert.True(t, node.GetMerkleRootSubmittedEventEmitted(), "MerkleRootSubmittedEventEmitted should be true after setting")

	// Test setting to false
	node.SetMerkleRootSubmittedEventEmitted(false)
	assert.False(t, node.GetMerkleRootSubmittedEventEmitted(), "MerkleRootSubmittedEventEmitted should be false after resetting")
}

func TestRegularNode_Halted(t *testing.T) {
	node := createTestRegularNode()

	// Test default value
	assert.False(t, node.GetHalted(), "Halted should be false by default")

	// Test setting to true
	node.SetHalted(true)
	assert.True(t, node.GetHalted(), "Halted should be true after setting")

	// Test setting to false
	node.SetHalted(false)
	assert.False(t, node.GetHalted(), "Halted should be false after resetting")
}

func TestRegularNode_CurrentRound(t *testing.T) {
	node := createTestRegularNode()

	// Test default value
	assert.Empty(t, node.GetCurrentRound(), "CurrentRound should be empty by default")

	// Test setting a value
	node.SetCurrentRound("100")
	assert.Equal(t, "100", node.GetCurrentRound(), "CurrentRound should match the set value")

	// Test updating to a new value
	node.SetCurrentRound("200")
	assert.Equal(t, "200", node.GetCurrentRound(), "CurrentRound should be updated")

	// Test setting to empty string
	node.SetCurrentRound("")
	assert.Empty(t, node.GetCurrentRound(), "CurrentRound should be empty after resetting")
}

func TestRegularNode_CurrentTrialNum(t *testing.T) {
	node := createTestRegularNode()

	// Test default value
	assert.Empty(t, node.GetCurrentTrialNum(), "CurrentTrialNum should be empty by default")

	// Test setting a value
	node.SetCurrentTrialNum("1")
	assert.Equal(t, "1", node.GetCurrentTrialNum(), "CurrentTrialNum should match the set value")

	// Test updating to a new value
	node.SetCurrentTrialNum("2")
	assert.Equal(t, "2", node.GetCurrentTrialNum(), "CurrentTrialNum should be updated")

	// Test setting to empty string
	node.SetCurrentTrialNum("")
	assert.Empty(t, node.GetCurrentTrialNum(), "CurrentTrialNum should be empty after resetting")
}

func TestRegularNode_RegularNodeEOA(t *testing.T) {
	node := createTestRegularNode()

	// Test default value
	assert.Empty(t, node.GetRegularNodeEOA(), "RegularNodeEOA should be empty by default")

	// Test setting a value
	testEOA := "0x1234567890123456789012345678901234567890"
	node.SetRegularNodeEOA(testEOA)
	assert.Equal(t, testEOA, node.GetRegularNodeEOA(), "RegularNodeEOA should match the set value")

	// Test updating to a new value
	newEOA := "0xABCDEF1234567890ABCDEF1234567890ABCDEF12"
	node.SetRegularNodeEOA(newEOA)
	assert.Equal(t, newEOA, node.GetRegularNodeEOA(), "RegularNodeEOA should be updated")

	// Test setting to empty string
	node.SetRegularNodeEOA("")
	assert.Empty(t, node.GetRegularNodeEOA(), "RegularNodeEOA should be empty after resetting")
}

func TestRegularNode_CvRequestIndices(t *testing.T) {
	node := createTestRegularNode()

	// Test default value
	indices := node.GetCvRequestIndices()
	assert.Empty(t, indices, "CvRequestIndices should be empty by default")

	// Test setting values
	testIndices := []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}
	node.SetCvRequestIndices(testIndices)

	retrievedIndices := node.GetCvRequestIndices()
	assert.Len(t, retrievedIndices, 3, "CvRequestIndices should have 3 elements")
	assert.Equal(t, big.NewInt(1), retrievedIndices[0], "First index should be 1")
	assert.Equal(t, big.NewInt(2), retrievedIndices[1], "Second index should be 2")
	assert.Equal(t, big.NewInt(3), retrievedIndices[2], "Third index should be 3")

	retrievedIndices[0] = big.NewInt(999)
	newRetrieved := node.GetCvRequestIndices()
	assert.Equal(t, big.NewInt(1), newRetrieved[0], "Internal value should not be affected by external modification")

	// Test clearing
	node.ClearCvRequestIndices()
	clearedIndices := node.GetCvRequestIndices()
	assert.Empty(t, clearedIndices, "CvRequestIndices should be empty after clearing")
}

func TestRegularNode_CvRequestIndices_EmptySlice(t *testing.T) {
	node := createTestRegularNode()

	// Set to empty slice explicitly
	node.SetCvRequestIndices([]*big.Int{})
	indices := node.GetCvRequestIndices()
	assert.Empty(t, indices, "CvRequestIndices should be empty")
	assert.NotNil(t, indices, "CvRequestIndices should not be nil")
}

func TestRegularNode_RoundData(t *testing.T) {
	node := createTestRegularNode()

	testKey := "100:1"
	testData := RoundData{
		MerkleRoot:   true,
		RandomNumber: false,
	}

	// Test getting non-existent key
	_, exists := node.GetRoundData(testKey)
	assert.False(t, exists, "RoundData should not exist initially")

	// Test setting a value
	node.SetRoundData(testKey, testData)
	retrievedData, exists := node.GetRoundData(testKey)
	assert.True(t, exists, "RoundData should exist after setting")
	assert.True(t, retrievedData.MerkleRoot, "MerkleRoot should match")
	assert.False(t, retrievedData.RandomNumber, "RandomNumber should match")

	// Test updating a value
	updatedData := RoundData{
		MerkleRoot:   false,
		RandomNumber: true,
	}
	node.SetRoundData(testKey, updatedData)
	retrievedData, _ = node.GetRoundData(testKey)
	assert.False(t, retrievedData.MerkleRoot, "MerkleRoot should be updated")
	assert.True(t, retrievedData.RandomNumber, "RandomNumber should be updated")

	// Test deleting a value
	node.DeleteRoundsData(testKey)
	_, exists = node.GetRoundData(testKey)
	assert.False(t, exists, "RoundData should not exist after deletion")
}

func TestRegularNode_StrictOrder(t *testing.T) {
	node := createTestRegularNode()

	testKey := "100:1"
	testOrder := []string{"node1", "node2", "node3"}

	// Test getting non-existent key
	_, exists := node.GetStrictOrder(testKey)
	assert.False(t, exists, "StrictOrder should not exist initially")

	// Test setting a value
	node.SetStrictOrder(testKey, testOrder)
	retrievedOrder, exists := node.GetStrictOrder(testKey)
	assert.True(t, exists, "StrictOrder should exist after setting")
	assert.Len(t, retrievedOrder, 3, "StrictOrder should have 3 elements")
	assert.Equal(t, "node1", retrievedOrder[0], "First node should match")

	// Test updating a value
	newOrder := []string{"nodeA", "nodeB"}
	node.SetStrictOrder(testKey, newOrder)
	retrievedOrder, _ = node.GetStrictOrder(testKey)
	assert.Len(t, retrievedOrder, 2, "Updated StrictOrder should have 2 elements")

	// Test deleting a value
	node.DeleteStrictOrder(testKey)
	_, exists = node.GetStrictOrder(testKey)
	assert.False(t, exists, "StrictOrder should not exist after deletion")
}

func TestRegularNode_SubmittedCvIndices_SetAndGet(t *testing.T) {
	node := createTestRegularNode()

	uniqueKey := "100:1"
	index := "0"

	// Test getting non-existent value
	_, exists := node.GetSubmittedCvIndicesValue(uniqueKey, index)
	assert.False(t, exists, "Value should not exist initially")

	// Test setting a value
	node.SetSubmittedCvIndicesValue(uniqueKey, index, true)
	value, exists := node.GetSubmittedCvIndicesValue(uniqueKey, index)
	assert.True(t, exists, "Value should exist after setting")
	assert.True(t, value, "Value should be true")

	// Test updating a value
	node.SetSubmittedCvIndicesValue(uniqueKey, index, false)
	value, exists = node.GetSubmittedCvIndicesValue(uniqueKey, index)
	assert.True(t, exists, "Value should still exist")
	assert.False(t, value, "Value should be false after update")
}

func TestRegularNode_SubmittedCvIndices_MapOperations(t *testing.T) {
	node := createTestRegularNode()

	uniqueKey := "100:1"
	testMap := map[string]bool{
		"0": true,
		"1": false,
		"2": true,
	}

	// Test getting non-existent map
	_, exists := node.GetSubmittedCvIndicesMap(uniqueKey)
	assert.False(t, exists, "Map should not exist initially")

	// Test setting a map
	node.SetSubmittedCvIndicesMap(uniqueKey, testMap)
	retrievedMap, exists := node.GetSubmittedCvIndicesMap(uniqueKey)
	assert.True(t, exists, "Map should exist after setting")
	assert.Len(t, retrievedMap, 3, "Map should have 3 elements")
	assert.True(t, retrievedMap["0"], "Index 0 should be true")
	assert.False(t, retrievedMap["1"], "Index 1 should be false")
	assert.True(t, retrievedMap["2"], "Index 2 should be true")

	// Test that returned map is a copy
	retrievedMap["0"] = false
	newRetrieved, _ := node.GetSubmittedCvIndicesMap(uniqueKey)
	assert.True(t, newRetrieved["0"], "Internal value should not be affected by external modification")

	// Test deleting a map
	node.DeleteSubmittedCvIndices(uniqueKey)
	_, exists = node.GetSubmittedCvIndicesMap(uniqueKey)
	assert.False(t, exists, "Map should not exist after deletion")
}

func TestRegularNode_SubmittedCvIndices_MultipleKeys(t *testing.T) {
	node := createTestRegularNode()

	key1 := "100:1"
	key2 := "200:2"

	// Set values for different keys
	node.SetSubmittedCvIndicesValue(key1, "0", true)
	node.SetSubmittedCvIndicesValue(key2, "0", false)

	// Verify independence
	value1, exists1 := node.GetSubmittedCvIndicesValue(key1, "0")
	value2, exists2 := node.GetSubmittedCvIndicesValue(key2, "0")

	assert.True(t, exists1, "Key1 should exist")
	assert.True(t, exists2, "Key2 should exist")
	assert.True(t, value1, "Key1 value should be true")
	assert.False(t, value2, "Key2 value should be false")
}

func TestRegularNode_RegularNodePrivateKey(t *testing.T) {
	node := createTestRegularNode()

	// Test default value (should be nil)
	assert.Nil(t, node.GetRegularNodePrivateKey(), "PrivateKey should be nil by default")

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "Should generate private key without error")

	// Test setting a private key
	node.SetRegularNodePrivateKey(privateKey)
	retrievedKey := node.GetRegularNodePrivateKey()
	assert.NotNil(t, retrievedKey, "PrivateKey should not be nil after setting")
	assert.Equal(t, privateKey, retrievedKey, "Retrieved key should match the set key")

	// Test setting to nil
	node.SetRegularNodePrivateKey(nil)
	assert.Nil(t, node.GetRegularNodePrivateKey(), "PrivateKey should be nil after resetting")
}

func TestRegularNode_MerkleRootSubmittedTOrRequestedCvTime(t *testing.T) {
	node := createTestRegularNode()

	// Test default value (should be nil)
	assert.Nil(t, node.GetMerkleRootSubmittedTOrRequestedCvTime(), "Time should be nil by default")

	// Test setting a value
	testTime := big.NewInt(1234567890)
	node.MerkleRootSubmittedTOrRequestedCvTime(testTime)
	retrievedTime := node.GetMerkleRootSubmittedTOrRequestedCvTime()
	assert.NotNil(t, retrievedTime, "Time should not be nil after setting")
	assert.Equal(t, testTime, retrievedTime, "Retrieved time should match the set time")

	// Test updating to a new value
	newTime := big.NewInt(9876543210)
	node.MerkleRootSubmittedTOrRequestedCvTime(newTime)
	retrievedTime = node.GetMerkleRootSubmittedTOrRequestedCvTime()
	assert.Equal(t, newTime, retrievedTime, "Time should be updated")

	// Test setting to nil
	node.MerkleRootSubmittedTOrRequestedCvTime(nil)
	assert.Nil(t, node.GetMerkleRootSubmittedTOrRequestedCvTime(), "Time should be nil after resetting")
}

func TestRegularNode_EnqueueUniqueKeyForCleanup(t *testing.T) {
	node := createTestRegularNode()

	// Add keys to test queue behavior
	for i := 1; i <= 4; i++ {
		uniqueKey := "round" + string(rune(i)) + ":1"
		node.EnqueueUniqueKeyForCleanup(uniqueKey)
	}

	assert.Equal(t, 4, node.cleanupQueue.Length(), "Queue should have 4 elements")

	node.EnqueueUniqueKeyForCleanup("round5:1")

	assert.True(t, node.cleanupQueue.Length() < 5, "Queue should be reduced after reaching threshold")
}

func TestRegularNode_EnqueueUniqueKeyForCleanup_MultipleCleanups(t *testing.T) {
	node := createTestRegularNode()

	// Add multiple keys in sequence
	for i := 1; i <= 10; i++ {
		uniqueKey := "round" + string(rune(i)) + ":1"
		node.EnqueueUniqueKeyForCleanup(uniqueKey)
	}

	// Queue should never exceed the threshold significantly
	assert.True(t, node.cleanupQueue.Length() < 5, "Queue should be maintained below threshold")
}

func TestRegularNode_ConcurrentBooleanAccess(t *testing.T) {
	node := createTestRegularNode()
	var wg sync.WaitGroup
	iterations := 100

	// Test concurrent execution access
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.SetExecution(true)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.GetExecution()
		}
	}()
	wg.Wait()

}

func TestRegularNode_ConcurrentStringAccess(t *testing.T) {
	node := createTestRegularNode()
	var wg sync.WaitGroup
	iterations := 100

	// Test concurrent round access
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.SetCurrentRound("100")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.GetCurrentRound()
		}
	}()
	wg.Wait()

}

func TestRegularNode_ConcurrentMapAccess(t *testing.T) {
	node := createTestRegularNode()
	var wg sync.WaitGroup
	iterations := 100

	// Test concurrent round data access
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.SetRoundData("100:1", RoundData{MerkleRoot: true, RandomNumber: false})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.GetRoundData("100:1")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations/2; i++ {
			node.DeleteRoundsData("100:1")
		}
	}()
	wg.Wait()

}

func TestRegularNode_ConcurrentSubmittedCvIndicesAccess(t *testing.T) {
	node := createTestRegularNode()
	var wg sync.WaitGroup
	iterations := 100

	// Test concurrent submittedCvIndices access
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.SetSubmittedCvIndicesValue("100:1", "0", true)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.GetSubmittedCvIndicesValue("100:1", "0")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations/2; i++ {
			node.DeleteSubmittedCvIndices("100:1")
		}
	}()
	wg.Wait()

}

func TestRegularNode_ConcurrentPrivateKeyAccess(t *testing.T) {
	node := createTestRegularNode()
	var wg sync.WaitGroup
	iterations := 50

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Test concurrent private key access
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.SetRegularNodePrivateKey(privateKey)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.GetRegularNodePrivateKey()
		}
	}()
	wg.Wait()

}

func TestRegularNode_ConcurrentCvRequestIndicesAccess(t *testing.T) {
	node := createTestRegularNode()
	var wg sync.WaitGroup
	iterations := 100

	testIndices := []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}

	// Test concurrent cvRequestIndices access
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.SetCvRequestIndices(testIndices)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			node.GetCvRequestIndices()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations/2; i++ {
			node.ClearCvRequestIndices()
		}
	}()
	wg.Wait()

}

func TestRegularNode_MultipleUpdatesInSequence(t *testing.T) {
	node := createTestRegularNode()

	// Test rapidly changing values
	for i := 0; i < 10; i++ {
		node.SetExecution(true)
		assert.True(t, node.GetExecution())
		node.SetExecution(false)
		assert.False(t, node.GetExecution())
	}
}

func TestRegularNode_NilAndEmptyValues(t *testing.T) {
	node := createTestRegularNode()

	// Test empty string
	node.SetCurrentRound("")
	assert.Empty(t, node.GetCurrentRound())

	// Test nil big.Int
	node.MerkleRootSubmittedTOrRequestedCvTime(nil)
	assert.Nil(t, node.GetMerkleRootSubmittedTOrRequestedCvTime())

	// Test empty slice
	node.SetCvRequestIndices([]*big.Int{})
	assert.Empty(t, node.GetCvRequestIndices())

	// Test nil private key
	node.SetRegularNodePrivateKey(nil)
	assert.Nil(t, node.GetRegularNodePrivateKey())
}

func TestRegularNode_LargeDataSets(t *testing.T) {
	node := createTestRegularNode()

	// Test with large slice
	largeSlice := make([]*big.Int, 1000)
	for i := 0; i < 1000; i++ {
		largeSlice[i] = big.NewInt(int64(i))
	}
	node.SetCvRequestIndices(largeSlice)
	retrieved := node.GetCvRequestIndices()
	assert.Len(t, retrieved, 1000, "Should handle large slices")

	// Test with large map
	largeMap := make(map[string]bool)
	for i := 0; i < 100; i++ {
		largeMap[string(rune(i))] = i%2 == 0
	}
	node.SetSubmittedCvIndicesMap("100:1", largeMap)
	retrievedMap, exists := node.GetSubmittedCvIndicesMap("100:1")
	assert.True(t, exists)
	assert.Len(t, retrievedMap, 100, "Should handle large maps")
}

func TestRegularNode_SpecialCharactersInKeys(t *testing.T) {
	node := createTestRegularNode()

	specialKeys := []string{
		"round:100:trial:1",
		"key-with-dashes",
		"key_with_underscores",
		"key.with.dots",
		"key/with/slashes",
	}

	for _, key := range specialKeys {
		node.SetRoundData(key, RoundData{MerkleRoot: true, RandomNumber: false})
		data, exists := node.GetRoundData(key)
		assert.True(t, exists, "Should handle special character key: %s", key)
		assert.True(t, data.MerkleRoot, "Data should be retrievable with special character key")
	}
}

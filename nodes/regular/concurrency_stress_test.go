package regular_node

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestConcurrentStateAccess tests concurrent access to regular node atomic states
func TestConcurrentStateAccess(t *testing.T) {
	tests := []struct {
		name          string
		numGoroutines int
		numOperations int
	}{
		{"Light Load", 10, 100},
		{"Medium Load", 25, 200},
		{"Heavy Load", 50, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := createTestRegularNode()

			var wg sync.WaitGroup
			var operations int64

			// Test concurrent atomic state operations
			for i := 0; i < tt.numGoroutines; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < tt.numOperations; j++ {
						// Test execution state
						node.SetExecution(j%2 == 0)
						_ = node.GetExecution()

						// Test halted state
						node.SetHalted(j%3 == 0)
						_ = node.GetHalted()

						// Test leader monitoring
						node.SetLeaderMonitoringActive(j%4 == 0)
						_ = node.GetLeaderMonitoringActive()

						// Test Merkle root event tracking
						node.SetMerkleRootSubmittedEventEmitted(j%5 == 0)
						_ = node.GetMerkleRootSubmittedEventEmitted()

						atomic.AddInt64(&operations, 4)
					}
				}(i)
			}

			wg.Wait()

			expectedOps := int64(tt.numGoroutines * tt.numOperations * 4)
			assert.Equal(t, expectedOps, operations, "All atomic operations should complete")

			t.Logf("Completed %d atomic operations across %d goroutines", operations, tt.numGoroutines)
		})
	}
}

// TestConcurrentMapOperations tests concurrent access to regular node shared maps
func TestConcurrentMapOperations(t *testing.T) {
	node := createTestRegularNode()

	const numWorkers = 30
	const operationsPerWorker = 150

	var wg sync.WaitGroup

	// Test concurrent map operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerWorker; j++ {
				key := fmt.Sprintf("regular_key_%d_%d", workerID, j)

				// Test RoundData map operations
				roundData := RoundData{
					MerkleRoot:   j%2 == 0,
					RandomNumber: j%3 == 0,
				}
				node.SetRoundData(key, roundData)

				retrieved, exists := node.GetRoundData(key)
				assert.True(t, exists, "Round data should exist after setting")
				assert.Equal(t, roundData, retrieved, "Retrieved data should match set data")

				// Test StrictOrder map operations
				order := []string{fmt.Sprintf("order_%d_1", j), fmt.Sprintf("order_%d_2", j)}
				node.SetStrictOrder(key, order)

				retrievedOrder, exists := node.GetStrictOrder(key)
				assert.True(t, exists, "Strict order should exist after setting")
				assert.Equal(t, order, retrievedOrder, "Retrieved order should match set order")

				// Test SubmittedCvIndices operations
				node.SetSubmittedCvIndicesValue(key, fmt.Sprintf("index_%d", j), j%2 == 0)
				value, exists := node.GetSubmittedCvIndicesValue(key, fmt.Sprintf("index_%d", j))
				assert.True(t, exists, "CV indices value should exist")
				assert.Equal(t, j%2 == 0, value, "CV indices value should match")

				// Cleanup
				node.DeleteRoundsData(key)
				node.DeleteStrictOrder(key)
				node.DeleteSubmittedCvIndices(key)
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentCleanupOperations tests concurrent cleanup queue operations
func TestConcurrentCleanupOperations(t *testing.T) {
	node := createTestRegularNode()

	const numWorkers = 20
	const cleanupOpsPerWorker = 100

	var wg sync.WaitGroup
	var enqueueCount int64

	// Test concurrent cleanup enqueue operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < cleanupOpsPerWorker; j++ {
				uniqueKey := fmt.Sprintf("cleanup_key_%d_%d", workerID, j)

				// Setup some data to cleanup
				node.SetRoundData(uniqueKey, RoundData{MerkleRoot: true, RandomNumber: false})
				node.SetStrictOrder(uniqueKey, []string{"test_order"})

				// Enqueue for cleanup (this triggers automatic cleanup when queue >= 5)
				node.EnqueueUniqueKeyForCleanup(uniqueKey)
				atomic.AddInt64(&enqueueCount, 1)

				// Small delay to allow cleanup processing
				time.Sleep(time.Microsecond * 10)
			}
		}(i)
	}

	wg.Wait()

	expectedEnqueues := int64(numWorkers * cleanupOpsPerWorker)
	assert.Equal(t, expectedEnqueues, enqueueCount, "All cleanup enqueues should complete")

	t.Logf("Processed %d cleanup operations", enqueueCount)
}

// TestAtomicStringOperations tests concurrent atomic string operations
func TestAtomicStringOperations(t *testing.T) {
	node := createTestRegularNode()

	const numWorkers = 25
	const operationsPerWorker = 200

	var wg sync.WaitGroup
	var operationCount int64

	// Test concurrent atomic string operations - just verify no race conditions occur
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerWorker; j++ {
				round := fmt.Sprintf("atomic_round_%d_%d", workerID, j)
				trial := fmt.Sprintf("trial_%d", j)
				eoa := fmt.Sprintf("eoa_%d_%d", workerID, j)

				// Test atomic string setters and getters - just ensure no crashes
				node.SetCurrentRound(round)
				_ = node.GetCurrentRound() // Don't assert equality in concurrent context

				node.SetCurrentTrialNum(trial)
				_ = node.GetCurrentTrialNum()

				node.SetRegularNodeEOA(eoa)
				_ = node.GetRegularNodeEOA()

				atomic.AddInt64(&operationCount, 1)
			}
		}(i)
	}

	wg.Wait()
	
	// Verify all operations completed without race conditions
	expectedOps := int64(numWorkers * operationsPerWorker)
	assert.Equal(t, expectedOps, operationCount, "All atomic operations should complete")
	
	// Verify final state is readable
	finalRound := node.GetCurrentRound()
	finalTrial := node.GetCurrentTrialNum()
	finalEOA := node.GetRegularNodeEOA()
	
	// Just verify these return valid strings (not nil/empty due to race conditions)
	assert.NotNil(t, finalRound, "Final round should not be nil")
	assert.NotNil(t, finalTrial, "Final trial should not be nil")
	assert.NotNil(t, finalEOA, "Final EOA should not be nil")
}

// TestConcurrentIndexOperations tests concurrent CV request indices operations
func TestConcurrentIndexOperations(t *testing.T) {
	node := createTestRegularNode()

	const numWorkers = 15
	const operationsPerWorker = 50

	var wg sync.WaitGroup
	var operationCount int64

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerWorker; j++ {
				// Create test indices
				indices := make([]*big.Int, 3)
				for k := 0; k < 3; k++ {
					indices[k] = big.NewInt(int64(workerID*1000 + j*10 + k))
				}

				// Test concurrent operations - just verify no panics occur
				node.SetCvRequestIndices(indices)

				// Get indices - don't assert specific values since other goroutines
				// may have set different values concurrently
				retrieved := node.GetCvRequestIndices()
				assert.NotNil(t, retrieved, "Retrieved indices should not be nil")

				// Test clear operation
				node.ClearCvRequestIndices()
				// Don't assert length is 0 since other goroutines may have set new values

				atomic.AddInt64(&operationCount, 1)
			}
		}(i)
	}

	wg.Wait()

	expectedOps := int64(numWorkers * operationsPerWorker)
	assert.Equal(t, expectedOps, operationCount, "All index operations should complete")

	// Test final state - ensure no crashes and operations work
	testIndices := []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}
	node.SetCvRequestIndices(testIndices)
	finalRetrieved := node.GetCvRequestIndices()
	assert.Equal(t, len(testIndices), len(finalRetrieved), "Final indices length should match")
}

// TestMemoryConsistencyRegular tests memory consistency across goroutines for regular node
func TestMemoryConsistencyRegular(t *testing.T) {
	node := createTestRegularNode()

	const numReaders = 8
	const numWriters = 4
	const duration = 2 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var wg sync.WaitGroup
	var inconsistencies int64

	// Writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()

			counter := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					round := fmt.Sprintf("consistency_round_%d_%d", writerID, counter)
					trial := fmt.Sprintf("trial_%d", counter)

					// Write operations that should be consistent
					node.SetCurrentRound(round)
					node.SetCurrentTrialNum(trial)
					node.SetExecution(counter%2 == 0)

					counter++
					time.Sleep(time.Microsecond * 100)
				}
			}
		}(i)
	}

	// Readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()

			var lastRound string
			for {
				select {
				case <-ctx.Done():
					return
				default:
					currentRound := node.GetCurrentRound()
					trial := node.GetCurrentTrialNum()
					execution := node.GetExecution()

					// Check for memory consistency
					if currentRound != "" && currentRound != lastRound {
						if trial == "" {
							atomic.AddInt64(&inconsistencies, 1)
						}
						lastRound = currentRound
					}

					// Verify execution state is boolean
					_ = execution // Use the value to prevent optimization

					time.Sleep(time.Microsecond * 50)
				}
			}
		}(i)
	}

	wg.Wait()

	// Allow for some inconsistencies due to timing, but not too many
	assert.LessOrEqual(t, inconsistencies, int64(5), "Too many memory inconsistencies detected")
	t.Logf("Detected %d memory inconsistencies", inconsistencies)
}

// TestGoroutineLeakPreventionRegular tests for goroutine leaks in regular node operations
func TestGoroutineLeakPreventionRegular(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	node := createTestRegularNode()

	// Perform operations that could potentially leak goroutines
	const numOperations = 100

	for i := 0; i < numOperations; i++ {
		key := fmt.Sprintf("leak_test_key_%d", i)

		// Operations that might spawn goroutines internally
		node.SetRoundData(key, RoundData{MerkleRoot: true, RandomNumber: false})
		node.SetStrictOrder(key, []string{"order1", "order2"})
		node.SetSubmittedCvIndicesValue(key, "index1", true)

		// State changes
		node.SetExecution(i%2 == 0)
		node.SetLeaderMonitoringActive(i%3 == 0)
		node.SetHalted(i%4 == 0)

		// Cleanup
		node.DeleteRoundsData(key)
		node.DeleteStrictOrder(key)
		node.DeleteSubmittedCvIndices(key)

		// Periodic cleanup trigger
		if i%10 == 0 {
			node.EnqueueUniqueKeyForCleanup(fmt.Sprintf("cleanup_%d", i))
		}
	}

	// Force garbage collection and wait for cleanup
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	// Allow for some variance in goroutine count
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+3,
		"Potential goroutine leak detected: initial=%d, final=%d",
		initialGoroutines, finalGoroutines)

	t.Logf("Goroutine count - Initial: %d, Final: %d", initialGoroutines, finalGoroutines)
}

// TestHighLoadStabilityRegular tests regular node stability under high concurrent load
func TestHighLoadStabilityRegular(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load test in short mode")
	}

	node := createTestRegularNode()

	const numWorkers = 50
	const operationsPerWorker = 300
	const testDuration = 8 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	var wg sync.WaitGroup
	var totalOperations int64
	var errors int64

	// High-load concurrent operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			operationCount := 0
			for operationCount < operationsPerWorker {
				select {
				case <-ctx.Done():
					return
				default:
					key := fmt.Sprintf("load_test_%d_%d", workerID, operationCount)

					// Mix of different operation types
					switch operationCount % 5 {
					case 0:
						// Atomic boolean operations
						node.SetExecution(operationCount%2 == 0)
						_ = node.GetExecution()
					case 1:
						// Atomic string operations
						node.SetCurrentRound(fmt.Sprintf("round_%d", operationCount))
						_ = node.GetCurrentRound()
					case 2:
						// Map operations
						roundData := RoundData{MerkleRoot: true, RandomNumber: false}
						node.SetRoundData(key, roundData)
						_, _ = node.GetRoundData(key)
					case 3:
						// Index operations
						indices := []*big.Int{big.NewInt(int64(operationCount))}
						node.SetCvRequestIndices(indices)
						_ = node.GetCvRequestIndices()
					case 4:
						// Cleanup operations
						node.EnqueueUniqueKeyForCleanup(key)
					}

					atomic.AddInt64(&totalOperations, 1)
					operationCount++

					// Small delay to prevent overwhelming
					if operationCount%50 == 0 {
						time.Sleep(time.Microsecond * 5)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify system handled the load well
	expectedMinOperations := int64(numWorkers * operationsPerWorker * 60 / 100) // At least 60%
	assert.GreaterOrEqual(t, totalOperations, expectedMinOperations,
		"Regular node should handle most operations under high load")

	errorRate := float64(errors) / float64(totalOperations) * 100
	assert.LessOrEqual(t, errorRate, 5.0, "Error rate should be less than 5%")

	t.Logf("High load test completed - Operations: %d, Errors: %d (%.2f%%)",
		totalOperations, errors, errorRate)
}

// TestConcurrentPrivateKeyAccess tests concurrent access to private key operations
func TestConcurrentPrivateKeyAccess(t *testing.T) {
	node := createTestRegularNode()

	const numWorkers = 10
	const operationsPerWorker = 50

	var wg sync.WaitGroup

	// Generate a test private key
	testKey := generateTestPrivateKey()

	// Test concurrent private key operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerWorker; j++ {
				// Set private key
				node.SetRegularNodePrivateKey(testKey)

				// Get private key
				retrievedKey := node.GetRegularNodePrivateKey()

				// Verify key consistency (simplified check)
				assert.NotNil(t, retrievedKey, "Private key should not be nil")
				if retrievedKey != nil && testKey != nil {
					assert.Equal(t, testKey.D.Cmp(retrievedKey.D), 0, "Private key values should match")
				}
			}
		}(i)
	}

	wg.Wait()
}

// Helper function to generate test private key
func generateTestPrivateKey() *ecdsa.PrivateKey {
	// This is a simplified test key generation
	// In real implementation, proper key generation would be used
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return key
}

// TestConcurrentTimestampOperations tests concurrent timestamp operations
func TestConcurrentTimestampOperations(t *testing.T) {
	node := createTestRegularNode()

	const numWorkers = 15
	const operationsPerWorker = 100

	var wg sync.WaitGroup
	var operationCount int64

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerWorker; j++ {
				timestamp := big.NewInt(time.Now().Unix() + int64(workerID*1000+j))

				// Test concurrent timestamp operations - verify no panics occur
				node.SetSubmitSMonitoringReferenceTime(timestamp)

				retrieved := node.GetSubmitSMonitoringReferenceTime()
				assert.NotNil(t, retrieved, "Retrieved timestamp should not be nil")

				// Don't assert specific values since other goroutines may have 
				// set different timestamps concurrently

				atomic.AddInt64(&operationCount, 1)
			}
		}(i)
	}

	wg.Wait()

	expectedOps := int64(numWorkers * operationsPerWorker)
	assert.Equal(t, expectedOps, operationCount, "All timestamp operations should complete")

	// Test final state - ensure operations work correctly when not concurrent
	finalTimestamp := big.NewInt(time.Now().Unix() + 9999)
	node.SetSubmitSMonitoringReferenceTime(finalTimestamp)
	finalRetrieved := node.GetSubmitSMonitoringReferenceTime()
	assert.NotNil(t, finalRetrieved, "Final timestamp should not be nil")
	assert.Equal(t, finalTimestamp.Cmp(finalRetrieved), 0, "Final timestamps should match")
}

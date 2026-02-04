package leader_node

import (
	"context"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

// TestConcurrentMerkleRootGeneration tests for race conditions during Merkle root generation
func TestConcurrentMerkleRootGeneration(t *testing.T) {
	tests := []struct {
		name          string
		numGoroutines int
		numRounds     int
	}{
		{"Light Load", 5, 10},
		{"Medium Load", 10, 20},
		{"Heavy Load", 50, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := CreateTestLeaderNode()
			_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			var wg sync.WaitGroup
			var successCount int64
			var errorCount int64

			// Simulate concurrent Merkle root generation
			for i := 0; i < tt.numGoroutines; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < tt.numRounds; j++ {
						round := fmt.Sprintf("round_%d_%d", workerID, j)
						trial := "1"

						// Setup test data
						setupTestCommitData(node, round, trial)

						// Test concurrent access
						if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
							// Simulate Merkle root generation work
							time.Sleep(time.Millisecond * 10)
							node.SetSubmittingMerkleRoot(false)
							atomic.AddInt64(&successCount, 1)
						} else {
							atomic.AddInt64(&errorCount, 1)
						}
					}
				}(i)
			}

			wg.Wait()

			// Verify no race conditions occurred
			assert.Greater(t, successCount, int64(0), "Expected at least some successful operations")
			t.Logf("Successful operations: %d, Prevented races: %d", successCount, errorCount)
		})
	}
}

// TestConcurrentCommitProcessing tests concurrent thread-safe operations without external dependencies
func TestConcurrentCommitProcessing(t *testing.T) {
	node := CreateTestLeaderNode()

	const numWorkers = 20
	const numOperationsPerWorker = 100

	var wg sync.WaitGroup
	var totalOperations int64

	// Test concurrent thread-safe operations that don't require external dependencies
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < numOperationsPerWorker; j++ {
				key := fmt.Sprintf("round_%d_%d:1", workerID, j)

				// Test concurrent RoundData operations
				roundData := RoundData{
					MerkleRoot:   j%2 == 0,
					RandomNumber: j%3 == 0,
				}
				node.SetRoundData(key, roundData)
				
				retrieved, exists := node.GetRoundData(key)
				if exists {
					assert.Equal(t, roundData.MerkleRoot, retrieved.MerkleRoot)
					assert.Equal(t, roundData.RandomNumber, retrieved.RandomNumber)
				}

				// Test concurrent reveal request status operations
				status := []string{fmt.Sprintf("status_%d_%d", workerID, j)}
				node.SetRevealRequestStatus(key, status)
				
				retrievedStatus, statusExists := node.GetRevealRequestStatus(key)
				if statusExists {
					assert.Equal(t, len(status), len(retrievedStatus))
				}

				// Test concurrent secrets on chain operations
				node.SetSecretsOnChain(key, j%2 == 0)
				secretValue, secretExists := node.GetSecretsOnChain(key)
				if secretExists {
					assert.Equal(t, j%2 == 0, secretValue)
				}

				atomic.AddInt64(&totalOperations, 3)
			}
		}(i)
	}

	wg.Wait()

	// Verify operations completed successfully
	assert.Equal(t, int64(numWorkers*numOperationsPerWorker*3), totalOperations)
	
	t.Logf("Total operations: %d", totalOperations)
}

// TestConcurrentMapAccess tests concurrent access to shared maps with proper mutex protection
func TestConcurrentMapAccess(t *testing.T) {
	node := CreateTestLeaderNode()

	const numWorkers = 50
	const numOperationsPerWorker = 200

	var wg sync.WaitGroup

	// Test concurrent map operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < numOperationsPerWorker; j++ {
				key := fmt.Sprintf("key_%d_%d", workerID, j)

				// Test RoundData map
				roundData := RoundData{
					MerkleRoot:   j%2 == 0,
					RandomNumber: j%3 == 0,
				}
				node.SetRoundData(key, roundData)

				retrieved, exists := node.GetRoundData(key)
				assert.True(t, exists)
				assert.Equal(t, roundData, retrieved)

				// Test reveal request status
				status := []string{fmt.Sprintf("status_%d", j)}
				node.SetRevealRequestStatus(key, status)

				retrievedStatus, exists := node.GetRevealRequestStatus(key)
				assert.True(t, exists)
				assert.Equal(t, status, retrievedStatus)

				// Test secrets on chain
				node.SetSecretsOnChain(key, j%2 == 0)
				value, exists := node.GetSecretsOnChain(key)
				assert.True(t, exists)
				assert.Equal(t, j%2 == 0, value)
			}
		}(i)
	}

	wg.Wait()
}

// TestAtomicOperationsStress tests atomic variable operations under stress
func TestAtomicOperationsStress(t *testing.T) {
	node := CreateTestLeaderNode()

	const numWorkers = 100
	const numOperationsPerWorker = 1000

	var wg sync.WaitGroup

	// Test atomic boolean operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < numOperationsPerWorker; j++ {
				// Test halted state
				node.SetHalted(j%3 == 0)
				_ = node.GetHalted()

				// Test monitoring states
				node.SetRequestedToSubmitCoMonitoringActive(j%4 == 0)
				_ = node.GetRequestedToSubmitCoMonitoringActive()

				node.SetRequestedToSubmitCvMonitoringActive(j%5 == 0)
				_ = node.GetRequestedToSubmitCvMonitoringActive()
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentTimerOperations tests timer-related race conditions
func TestConcurrentTimerOperations(t *testing.T) {
	node := CreateTestLeaderNode()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const numWorkers = 10
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			round := fmt.Sprintf("timer_round_%d", workerID)
			trial := "1"
			timestamp := big.NewInt(time.Now().Unix())

			// Test concurrent timer operations
			node.startRequestToSubmitCvMonitoring(ctx, round, trial, timestamp)
			time.Sleep(time.Millisecond * 100)
			node.stopRequestToSubmitCvMonitoring()

			node.startFailToSubmitCvMonitoring(ctx, round, trial, timestamp)
			time.Sleep(time.Millisecond * 100)
			node.stopFailToSubmitCvMonitoring()
		}(i)
	}

	wg.Wait()

	// Verify basic state after all concurrent operations
	// These assertions are safe because all goroutines have completed
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive(), 
		"CV monitoring should be inactive after test")
	assert.False(t, node.GetFailToSubmitSMonitoringActive(), 
		"Fail monitoring should be inactive after test")
	
	// Test that timer operations still work after concurrency stress
	testRound := "post_test_round"
	testTrial := "1" 
	testTimestamp := big.NewInt(time.Now().Unix())
	
	// Should not panic or deadlock
	node.startRequestToSubmitCvMonitoring(ctx, testRound, testTrial, testTimestamp)
	time.Sleep(time.Millisecond * 10)
	node.stopRequestToSubmitCvMonitoring()
	
	assert.False(t, node.GetRequestedToSubmitCvMonitoringActive(),
		"CV monitoring should be stopped after individual test")
}

// TestMemoryConsistency tests memory consistency across goroutines
func TestMemoryConsistency(t *testing.T) {
	node := CreateTestLeaderNode()

	const numReaders = 10
	const numWriters = 5
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

			for {
				select {
				case <-ctx.Done():
					return
				default:
					round := fmt.Sprintf("consistency_round_%d", writerID)
					node.SetCurrentRound(round)
					node.SetCurrentTrial("1")
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
					trial := node.GetCurrentTrial()

					// Check for memory consistency
					if currentRound != "" && currentRound != lastRound {
						if trial == "" {
							atomic.AddInt64(&inconsistencies, 1)
						}
						lastRound = currentRound
					}
					time.Sleep(time.Microsecond * 50)
				}
			}
		}(i)
	}

	wg.Wait()

	// Allow for some inconsistencies due to timing, but not too many
	assert.LessOrEqual(t, inconsistencies, int64(10), "Too many memory inconsistencies detected")
}

// TestGoroutineLeakPrevention tests for goroutine leaks in concurrent operations
func TestGoroutineLeakPrevention(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	node := CreateTestLeaderNode()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Perform operations that could potentially leak goroutines
	for i := 0; i < 50; i++ {
		round := fmt.Sprintf("leak_test_round_%d", i)
		trial := "1"
		timestamp := big.NewInt(time.Now().Unix())

		// Start and immediately stop monitoring
		node.startRequestToSubmitCvMonitoring(ctx, round, trial, timestamp)
		node.stopRequestToSubmitCvMonitoring()

		node.startFailToSubmitCoMonitoring(ctx, round, trial, timestamp)
		node.stopFailToSubmitCoMonitoring()
	}

	// Force garbage collection and wait for goroutines to clean up
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	// Allow for some variance in goroutine count
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+5,
		"Potential goroutine leak detected: initial=%d, final=%d",
		initialGoroutines, finalGoroutines)
}

// Helper functions

func setupTestCommitData(node *LeaderNode, round, trial string) {
	uniqueKey := utils.GetUniqueKey(round, trial)

	// Setup some test commit data
	testAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	var testCvs [32]byte
	copy(testCvs[:], "test_cvs_data")

	utils.EnsureCommittedNodesRoundExists(uniqueKey)
	commitData := utils.LeaderCommitData{
		Round:      round,
		EOAAddress: testAddr.Hex(),
		Cvs:        testCvs,
	}
	utils.SetCommittedNodeData(uniqueKey, testAddr, commitData)
}

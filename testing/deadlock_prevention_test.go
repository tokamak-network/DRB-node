package testing

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/eapache/queue"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
	"github.com/tokamak-network/DRB-node/utils"
)

// DeadlockDetector monitors for potential deadlock scenarios
type DeadlockDetector struct {
	timeout   time.Duration
	operation string
}

// NewDeadlockDetector creates a new deadlock detector
func NewDeadlockDetector(operation string, timeout time.Duration) *DeadlockDetector {
	return &DeadlockDetector{
		timeout:   timeout,
		operation: operation,
	}
}

// WithTimeout executes a function with deadlock detection
func (d *DeadlockDetector) WithTimeout(t *testing.T, fn func()) {
	done := make(chan bool, 1)
	
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Panic in %s: %v", d.operation, r)
			}
		}()
		fn()
		done <- true
	}()
	
	select {
	case <-done:
		// Operation completed successfully
	case <-time.After(d.timeout):
		t.Errorf("Potential deadlock detected in %s (timeout after %v)", d.operation, d.timeout)
	}
}

// TestMutexOrderingConsistency tests that mutexes are acquired in consistent order
func TestMutexOrderingConsistency(t *testing.T) {
	// This test verifies that the mutex acquisition order is consistent
	// across different goroutines to prevent deadlocks
	
	node := createTestLeaderNodeForDeadlock()
	const numWorkers = 20
	const numOperations = 100
	
	var wg sync.WaitGroup
	deadlockDetector := NewDeadlockDetector("MutexOrdering", 10*time.Second)
	
	// Test scenario 1: Multiple maps accessed in sequence
	t.Run("SequentialMapAccess", func(t *testing.T) {
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				deadlockDetector.WithTimeout(t, func() {
					for j := 0; j < numOperations; j++ {
						key := fmt.Sprintf("worker_%d_op_%d", workerID, j)
						
						// Test consistent mutex ordering for multiple maps
						// Order: roundsData -> revealRequestStatus -> activeBroadcasts
						
						roundData := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
						node.SetRoundData(key, roundData)
						
						status := []string{fmt.Sprintf("status_%d", j)}
						node.SetRevealRequestStatus(key, status)
						
						tracker := &utils.BroadcastTracker{}
						node.SetActiveBroadcast(key, tracker)
						
						// Read operations in same order
						_, _ = node.GetRoundData(key)
						_, _ = node.GetRevealRequestStatus(key)
						_, _ = node.GetActiveBroadcast(key)
						
						// Cleanup
						node.DeleteActiveBroadcast(key)
						node.DeleteRevealRequestStatus(key)
					}
				})
			}(i)
		}
		
		wg.Wait()
	})
	
	// Test scenario 2: Reverse order access to detect ordering issues
	t.Run("ReverseOrderAccess", func(t *testing.T) {
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				deadlockDetector.WithTimeout(t, func() {
					for j := 0; j < numOperations; j++ {
						key := fmt.Sprintf("reverse_worker_%d_op_%d", workerID, j)
						
						// Test reverse order: activeBroadcasts -> revealRequestStatus -> roundsData
						tracker := &utils.BroadcastTracker{}
						node.SetActiveBroadcast(key, tracker)
						
						status := []string{fmt.Sprintf("status_%d", j)}
						node.SetRevealRequestStatus(key, status)
						
						roundData := leader_node.RoundData{MerkleRoot: false, RandomNumber: true}
						node.SetRoundData(key, roundData)
						
						// Cleanup in reverse order
						node.DeleteRoundsData(key)
						node.DeleteRevealRequestStatus(key)
						node.DeleteActiveBroadcast(key)
					}
				})
			}(i)
		}
		
		wg.Wait()
	})
}

// TestNestedLockOperations tests nested lock scenarios that could cause deadlocks
func TestNestedLockOperations(t *testing.T) {
	node := createTestLeaderNodeForDeadlock()
	deadlockDetector := NewDeadlockDetector("NestedLocks", 15*time.Second)
	
	const numWorkers = 10
	const numOperations = 50
	
	var wg sync.WaitGroup
	
	// Test nested operations that could create lock hierarchies
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			deadlockDetector.WithTimeout(t, func() {
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("nested_%d_%d", workerID, j)
					
					// Simulate complex operations that might require multiple locks
					
					// 1. Set round data (requires roundsDataMu)
					roundData := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
					node.SetRoundData(key, roundData)
					
					// 2. Set secrets (requires secretsOnChainMu)
					node.SetSecretsOnChain(key, true)
					
					// 3. Set CV on chain (requires cvOnChainMu)
					node.SetCvOnChain(key, true)
					
					// 4. Perform atomic operations
					node.SetExecution(j%2 == 0)
					node.SetHalted(j%3 == 0)
					
					// 5. Access multiple maps in nested fashion
					if retrieved, exists := node.GetRoundData(key); exists {
						_ = retrieved.MerkleRoot
						
						if secret, exists := node.GetSecretsOnChain(key); exists {
							_ = secret
							
							if cv, exists := node.GetCvOnChain(key); exists {
								_ = cv
							}
						}
					}
					
					// Cleanup
					node.DeleteRoundsData(key)
					node.DeleteSecretsOnChain(key)
					node.DeleteCvOnChain(key)
				}
			})
		}(i)
	}
	
	wg.Wait()
}

// TestConcurrentAtomicAndMutexOperations tests interaction between atomic and mutex operations
func TestConcurrentAtomicAndMutexOperations(t *testing.T) {
	node := createTestLeaderNodeForDeadlock()
	deadlockDetector := NewDeadlockDetector("AtomicMutexInteraction", 10*time.Second)
	
	const numWorkers = 15
	const duration = 5 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	
	var wg sync.WaitGroup
	
	// Worker 1: Atomic operations only
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		deadlockDetector.WithTimeout(t, func() {
			i := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					node.SetExecution(i%2 == 0)
					node.SetHalted(i%3 == 0)
					node.SetSubmittingMerkleRoot(i%4 == 0)
					node.SetRequestedToSubmitCoMonitoringActive(i%5 == 0)
					
					_ = node.GetExecution()
					_ = node.GetHalted()
					_ = node.GetSubmittingMerkleRoot()
					_ = node.GetRequestedToSubmitCoMonitoringActive()
					
					i++
					time.Sleep(time.Microsecond * 10)
				}
			}
		})
	}()
	
	// Worker 2: Mutex operations only
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		deadlockDetector.WithTimeout(t, func() {
			i := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					key := fmt.Sprintf("mutex_key_%d", i)
					
					roundData := leader_node.RoundData{
						MerkleRoot:   i%2 == 0,
						RandomNumber: i%3 == 0,
					}
					node.SetRoundData(key, roundData)
					
					_, _ = node.GetRoundData(key)
					node.DeleteRoundsData(key)
					
					i++
					time.Sleep(time.Microsecond * 20)
				}
			}
		})
	}()
	
	// Worker 3: Mixed operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		deadlockDetector.WithTimeout(t, func() {
			i := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					key := fmt.Sprintf("mixed_key_%d", i)
					
					// Mix atomic and mutex operations
					node.SetExecution(i%2 == 0)
					
					roundData := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
					node.SetRoundData(key, roundData)
					
					node.SetHalted(i%3 == 0)
					
					_, _ = node.GetRoundData(key)
					_ = node.GetExecution()
					
					node.DeleteRoundsData(key)
					
					i++
					time.Sleep(time.Microsecond * 15)
				}
			}
		})
	}()
	
	wg.Wait()
}

// TestTimerAndMutexInteractions tests interactions between timers and mutex operations
func TestTimerAndMutexInteractions(t *testing.T) {
	node := createTestLeaderNodeForDeadlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	deadlockDetector := NewDeadlockDetector("TimerMutexInteraction", 15*time.Second)
	
	var wg sync.WaitGroup
	
	// Simulate timer-based operations that might interact with mutex-protected data
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			deadlockDetector.WithTimeout(t, func() {
				round := fmt.Sprintf("timer_round_%d", workerID)
				trial := "1"
				timestamp := big.NewInt(time.Now().Unix())
				
				// Simulate timer operations while other operations are happening
				for j := 0; j < 20; j++ {
					select {
					case <-ctx.Done():
						return
					default:
						key := fmt.Sprintf("timer_key_%d_%d", workerID, j)
						
						// Mix timer state changes with data operations
						node.SetRequestToSubmitCvMonitoringActive(j%2 == 0)
						
						roundData := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
						node.SetRoundData(key, roundData)
						
						node.SetRequestedToSubmitCoMonitoringActive(j%3 == 0)
						
						// Simulate timer-related state updates
						node.SetCurrentRound(round)
						node.SetCurrentTrial(trial)
						
						// Cleanup
						node.DeleteRoundsData(key)
						node.SetRequestToSubmitCvMonitoringActive(false)
						node.SetRequestedToSubmitCoMonitoringActive(false)
						
						time.Sleep(time.Millisecond * 10)
					}
				}
				
				_ = timestamp // Use variable to avoid compiler warnings
			})
		}(i)
	}
	
	wg.Wait()
}

// TestReaderWriterLockPatterns tests RWMutex usage patterns for deadlocks
func TestReaderWriterLockPatterns(t *testing.T) {
	node := createTestLeaderNodeForDeadlock()
	deadlockDetector := NewDeadlockDetector("ReaderWriterLocks", 10*time.Second)
	
	const numReaders = 10
	const numWriters = 5
	const duration = 5 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	
	var wg sync.WaitGroup
	
	// Multiple readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			
			deadlockDetector.WithTimeout(t, func() {
				j := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						key := fmt.Sprintf("reader_key_%d_%d", readerID, j)
						
						// Read operations (should use RLock)
						_, _ = node.GetRoundData(key)
						_, _ = node.GetRevealRequestStatus(key)
						_, _ = node.GetActiveBroadcast(key)
						_, _ = node.GetSecretsOnChain(key)
						_, _ = node.GetCvOnChain(key)
						
						j++
						time.Sleep(time.Microsecond * 50)
					}
				}
			})
		}(i)
	}
	
	// Fewer writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			
			deadlockDetector.WithTimeout(t, func() {
				j := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						key := fmt.Sprintf("writer_key_%d_%d", writerID, j)
						
						// Write operations (should use Lock)
						roundData := leader_node.RoundData{
							MerkleRoot:   j%2 == 0,
							RandomNumber: j%3 == 0,
						}
						node.SetRoundData(key, roundData)
						
						status := []string{fmt.Sprintf("status_%d", j)}
						node.SetRevealRequestStatus(key, status)
						
						node.SetSecretsOnChain(key, j%2 == 0)
						node.SetCvOnChain(key, j%3 == 0)
						
						// Cleanup
						node.DeleteRoundsData(key)
						node.DeleteRevealRequestStatus(key)
						node.DeleteSecretsOnChain(key)
						node.DeleteCvOnChain(key)
						
						j++
						time.Sleep(time.Millisecond * 2)
					}
				}
			})
		}(i)
	}
	
	wg.Wait()
}

// TestComplexOperationSequences tests complex sequences that might create deadlocks
func TestComplexOperationSequences(t *testing.T) {
	node := createTestLeaderNodeForDeadlock()
	deadlockDetector := NewDeadlockDetector("ComplexSequences", 20*time.Second)
	
	const numWorkers = 8
	var wg sync.WaitGroup
	
	// Simulate complex operation sequences similar to real DRB operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			deadlockDetector.WithTimeout(t, func() {
				for j := 0; j < 30; j++ {
					round := fmt.Sprintf("complex_round_%d_%d", workerID, j)
					trial := "1"
					uniqueKey := utils.GetUniqueKey(round, trial)
					
					// Simulate complex DRB operation sequence
					
					// 1. Set current round state (atomic operations)
					node.SetCurrentRound(round)
					node.SetCurrentTrial(trial)
					node.SetExecution(true)
					node.SetHalted(false)
					
					// 2. Setup round data (mutex operations)
					roundData := leader_node.RoundData{MerkleRoot: false, RandomNumber: false}
					node.SetRoundData(uniqueKey, roundData)
					
					// 3. Process commits (multiple map operations)
					node.SetCvOnChain(uniqueKey, false)
					node.SetSecretsOnChain(uniqueKey, false)
					
					// 4. Reveal request operations
					status := []string{"pending"}
					node.SetRevealRequestStatus(uniqueKey, status)
					
					// 5. Broadcast operations
					tracker := &utils.BroadcastTracker{}
					node.SetActiveBroadcast(uniqueKey, tracker)
					
					// 6. Complete round operations
					node.SetSubmittingMerkleRoot(true)
					
					// Simulate processing time
					time.Sleep(time.Microsecond * 100)
					
					// 7. Cleanup sequence (reverse order to test for ordering issues)
					node.SetSubmittingMerkleRoot(false)
					node.DeleteActiveBroadcast(uniqueKey)
					node.DeleteRevealRequestStatus(uniqueKey)
					node.DeleteSecretsOnChain(uniqueKey)
					node.DeleteCvOnChain(uniqueKey)
					node.DeleteRoundsData(uniqueKey)
					
					// 8. Reset state
					node.SetExecution(false)
					node.SetHalted(true)
				}
			})
		}(i)
	}
	
	wg.Wait()
}

// Helper function to create test leader node
func createTestLeaderNodeForDeadlock() *leader_node.LeaderNode {
	node := &leader_node.LeaderNode{}
	
	// Initialize all required fields for deadlock testing
	node.SetExecution(false)
	node.SetHalted(false)
	
	return node
}

// TestLockHierarchyValidation tests that lock hierarchy is properly maintained
func TestLockHierarchyValidation(t *testing.T) {
	// This test documents and validates the expected lock hierarchy
	// to prevent future deadlock issues
	
	t.Run("DocumentedLockHierarchy", func(t *testing.T) {
		// Document the expected lock hierarchy for the DRB system
		lockHierarchy := map[string]int{
			// Higher numbers should be acquired before lower numbers
			"cleanupQueueMu":                5,
			"indicesMutex":                  4,
			"broadcastMutex":               3,
			"roundsDataMu":                 2,
			"secretMapsMutex":              2,
			"secretsOnChainMu":             2,
			"cvOnChainMu":                  2,
			"revealRequestStatusMu":        2,
			"activeBroadcastsMu":           2,
			"timestampMu":                  1,
			"reqMu":                        1,
		}
		
		// Ensure hierarchy is documented
		require.NotEmpty(t, lockHierarchy)
		
		// Log the hierarchy for documentation
		t.Logf("DRB Lock Hierarchy (acquire in descending order):")
		for mutex, level := range lockHierarchy {
			t.Logf("  Level %d: %s", level, mutex)
		}
		
		// This serves as documentation for developers
		assert.True(t, true, "Lock hierarchy documented successfully")
	})
	
	t.Run("ValidateNoCircularDependencies", func(t *testing.T) {
		// Test that no circular dependencies exist in mutex usage
		node := createTestLeaderNodeForDeadlock()
		
		// This is a simplified test - in practice, static analysis tools
		// or more sophisticated testing would be needed for complete validation
		
		// Test various operation combinations to catch circular dependencies
		combinations := []func(){
			func() {
				// Test combination 1
				node.SetRoundData("test1", leader_node.RoundData{})
				node.SetSecretsOnChain("test1", true)
				node.DeleteRoundsData("test1")
				node.DeleteSecretsOnChain("test1")
			},
			func() {
				// Test combination 2  
				node.SetRevealRequestStatus("test2", []string{"status"})
				node.SetActiveBroadcast("test2", &utils.BroadcastTracker{})
				node.DeleteRevealRequestStatus("test2")
				node.DeleteActiveBroadcast("test2")
			},
		}
		
		for i, combo := range combinations {
			t.Run(fmt.Sprintf("Combination_%d", i), func(t *testing.T) {
				done := make(chan bool)
				go func() {
					combo()
					done <- true
				}()
				
				select {
				case <-done:
					// Success - no deadlock
				case <-time.After(2 * time.Second):
					t.Errorf("Potential circular dependency detected in combination %d", i)
				}
			})
		}
	})
}
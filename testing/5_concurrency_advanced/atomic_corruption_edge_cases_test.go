package concurrency_advanced

import (
	"context"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
	regular_node "github.com/tokamak-network/DRB-node/nodes/regular"
)

// TestAtomicStateCorruptionDuringContextSwitching tests atomic state corruption during rapid context switching
func TestAtomicStateCorruptionDuringContextSwitching(t *testing.T) {
	node := leader_node.CreateTestLeaderNode()
	
	const numContextSwitchers = 50
	const operationsPerSwitcher = 1000
	const testDuration = 10 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	var atomicCorruptions int64
	var contextSwitches int64
	var stateInconsistencies int64
	
	t.Run("AtomicStateCorruption", func(t *testing.T) {
		// Context switchers that rapidly change atomic states
		for switcher := 0; switcher < numContextSwitchers; switcher++ {
			wg.Add(1)
			go func(switcherID int) {
				defer wg.Done()
				
				localSwitches := 0
				for op := 0; op < operationsPerSwitcher; op++ {
					select {
					case <-ctx.Done():
						return
					default:
						// Rapid state changes that could cause corruption
						expectedExecution := (op + switcherID) % 2 == 0
						expectedHalted := (op + switcherID) % 3 == 0
						expectedMonitoring := (op + switcherID) % 5 == 0
						
						// Force context switch during critical state changes
						runtime.Gosched()
						
						// Atomic operations that should maintain consistency
						node.SetExecution(expectedExecution)
						actualExecution := node.GetExecution()
						
						runtime.Gosched() // Force context switch mid-operation
						
						node.SetHalted(expectedHalted)
						actualHalted := node.GetHalted()
						
						runtime.Gosched()
						
						node.SetRequestedToSubmitCvMonitoringActive(expectedMonitoring)
						actualMonitoring := node.GetRequestedToSubmitCvMonitoringActive()
						
						// Check for state corruption
						if actualExecution != expectedExecution {
							t.Logf("Execution state corruption detected: expected=%v, actual=%v", expectedExecution, actualExecution)
							atomic.AddInt64(&atomicCorruptions, 1)
						}
						
						if actualHalted != expectedHalted {
							t.Logf("Halted state corruption detected: expected=%v, actual=%v", expectedHalted, actualHalted)
							atomic.AddInt64(&atomicCorruptions, 1)
						}
						
						if actualMonitoring != expectedMonitoring {
							t.Logf("Monitoring state corruption detected: expected=%v, actual=%v", expectedMonitoring, actualMonitoring)
							atomic.AddInt64(&atomicCorruptions, 1)
						}
						
						// Check for cross-state inconsistencies
						if actualExecution && actualHalted && actualMonitoring {
							// This combination might indicate state inconsistency
							atomic.AddInt64(&stateInconsistencies, 1)
						}
						
						localSwitches++
						atomic.AddInt64(&contextSwitches, 1)
						
						// Introduce random delays to increase context switching probability
						if rand.Intn(100) < 5 {
							time.Sleep(time.Microsecond)
						}
					}
				}
			}(switcher)
		}
		
		wg.Wait()
		
		totalOperations := contextSwitches
		corruptionRate := float64(atomicCorruptions) / float64(totalOperations) * 100
		inconsistencyRate := float64(stateInconsistencies) / float64(totalOperations) * 100
		
		t.Logf("Atomic state corruption during context switching results:")
		t.Logf("  Total operations: %d", totalOperations)
		t.Logf("  Context switches: %d", contextSwitches)
		t.Logf("  Atomic corruptions: %d (%.4f%%)", atomicCorruptions, corruptionRate)
		t.Logf("  State inconsistencies: %d (%.4f%%)", stateInconsistencies, inconsistencyRate)
		t.Logf("  Context switchers: %d", numContextSwitchers)
		
		// Assert no atomic corruption should occur
		assert.Equal(t, int64(0), atomicCorruptions, "No atomic state corruption should occur during context switching")
		assert.Greater(t, totalOperations, int64(numContextSwitchers*operationsPerSwitcher/2), "Should complete most operations")
		assert.LessOrEqual(t, inconsistencyRate, 5.0, "State inconsistency rate should be <5%%")
	})
}

// TestTimerRaceConditionsWithContextCancellation tests timer race conditions during context cancellation
func TestTimerRaceConditionsWithContextCancellation(t *testing.T) {
	node := leader_node.CreateTestLeaderNode()
	
	const numTimerWorkers = 20
	const timersPerWorker = 100
	const testDuration = 8 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	var timerRaces int64
	var cancellationRaces int64
	var timerLeaks int64
	var successfulCancellations int64
	
	t.Run("TimerRaceConditions", func(t *testing.T) {
		// Track initial goroutine count
		initialGoroutines := runtime.NumGoroutine()
		
		for worker := 0; worker < numTimerWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for timer := 0; timer < timersPerWorker; timer++ {
					select {
					case <-ctx.Done():
						return
					default:
						// Create contexts with random cancellation timing
						timerCtx, timerCancel := context.WithTimeout(ctx, time.Duration(10+rand.Intn(100))*time.Millisecond)
						
						// Start timer-based monitoring (using public atomic operations)
						go func() {
							defer timerCancel()
							
							// Simulate rapid timer operations
							node.SetRequestedToSubmitCvMonitoringActive(true)
							
							// Random chance of early cancellation (race condition)
							if rand.Intn(100) < 20 {
								time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
								if node.GetRequestedToSubmitCvMonitoringActive() {
									node.SetRequestedToSubmitCvMonitoringActive(false)
									atomic.AddInt64(&successfulCancellations, 1)
								} else {
									atomic.AddInt64(&cancellationRaces, 1)
								}
							}
						}()
						
						// Concurrent timer operations that could race
						go func() {
							time.Sleep(time.Duration(rand.Intn(20)) * time.Millisecond)
							
							if timerCtx.Err() == nil {
								// Timer might be cancelled while we're accessing it
								if node.GetRequestedToSubmitCvMonitoringActive() {
									node.SetRequestedToSubmitCvMonitoringActive(false)
								} else {
									atomic.AddInt64(&timerRaces, 1)
								}
							}
						}()
						
						// Wait for context cancellation or timeout
						<-timerCtx.Done()
						
						// Check for timer cleanup races
						time.Sleep(time.Millisecond)
						if node.GetRequestedToSubmitCvMonitoringActive() {
							atomic.AddInt64(&timerLeaks, 1)
							node.SetRequestedToSubmitCvMonitoringActive(false) // Force cleanup
						}
					}
				}
			}(worker)
		}
		
		wg.Wait()
		
		// Allow cleanup time
		time.Sleep(200 * time.Millisecond)
		runtime.GC()
		finalGoroutines := runtime.NumGoroutine()
		
		totalTimers := int64(numTimerWorkers * timersPerWorker)
		raceRate := float64(timerRaces+cancellationRaces) / float64(totalTimers) * 100
		leakRate := float64(timerLeaks) / float64(totalTimers) * 100
		goroutineGrowth := finalGoroutines - initialGoroutines
		
		t.Logf("Timer race conditions with context cancellation results:")
		t.Logf("  Total timers created: %d", totalTimers)
		t.Logf("  Timer races detected: %d", timerRaces)
		t.Logf("  Cancellation races: %d", cancellationRaces)
		t.Logf("  Total race events: %d (%.2f%%)", timerRaces+cancellationRaces, raceRate)
		t.Logf("  Timer leaks: %d (%.2f%%)", timerLeaks, leakRate)
		t.Logf("  Successful cancellations: %d", successfulCancellations)
		t.Logf("  Goroutine growth: %d", goroutineGrowth)
		
		// Timer race condition assertions
		assert.LessOrEqual(t, raceRate, 10.0, "Timer race condition rate should be <10%%")
		assert.LessOrEqual(t, leakRate, 5.0, "Timer leak rate should be <5%%")
		assert.Greater(t, successfulCancellations, int64(0), "Some cancellations should succeed")
		assert.LessOrEqual(t, goroutineGrowth, 10, "Goroutine growth should be minimal after cleanup")
	})
}

// TestConcurrentRegularNodeAtomicEdgeCases tests edge cases in regular node atomic operations
func TestConcurrentRegularNodeAtomicEdgeCases(t *testing.T) {
	node := createTestRegularNode()
	
	const numAtomicWorkers = 30
	const atomicOpsPerWorker = 500
	const testDuration = 6 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	var atomicInconsistencies int64
	var rapidFlipDetected int64
	var stateCoherenceViolations int64
	var totalAtomicOps int64
	
	t.Run("RegularNodeAtomicEdgeCases", func(t *testing.T) {
		for worker := 0; worker < numAtomicWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for op := 0; op < atomicOpsPerWorker; op++ {
					select {
					case <-ctx.Done():
						return
					default:
						// Edge case: Rapid state flipping
						originalExecution := node.GetExecution()
						node.SetExecution(!originalExecution)
						
						// Force context switch during critical period
						runtime.Gosched()
						
						flippedExecution := node.GetExecution()
						if flippedExecution == originalExecution {
							atomic.AddInt64(&rapidFlipDetected, 1)
						}
						
						// Edge case: State coherence across multiple atomics
						node.SetExecution(true)
						node.SetHalted(false)
						node.SetLeaderMonitoringActive(true)
						
						// Read all states together (should be coherent)
						exec := node.GetExecution()
						halted := node.GetHalted()
						monitoring := node.GetLeaderMonitoringActive()
						
						// This combination might indicate a coherence violation
						if !exec && halted && monitoring {
							atomic.AddInt64(&stateCoherenceViolations, 1)
						}
						
						// Edge case: String atomic operations with concurrent modification
						originalRound := node.GetCurrentRound()
						newRound := generateRandomString(20)
						node.SetCurrentRound(newRound)
						
						runtime.Gosched() // Force potential race
						
						retrievedRound := node.GetCurrentRound()
						if retrievedRound != newRound && retrievedRound != originalRound {
							// String corruption detected
							atomic.AddInt64(&atomicInconsistencies, 1)
						}
						
						atomic.AddInt64(&totalAtomicOps, 4) // 4 atomic operations per iteration
						
						// Small delay to allow other goroutines
						if op%100 == 0 {
							time.Sleep(time.Microsecond * 10)
						}
					}
				}
			}(worker)
		}
		
		wg.Wait()
		
		inconsistencyRate := float64(atomicInconsistencies) / float64(totalAtomicOps) * 100
		flipRate := float64(rapidFlipDetected) / float64(totalAtomicOps) * 100
		violationRate := float64(stateCoherenceViolations) / float64(totalAtomicOps) * 100
		
		t.Logf("Regular node atomic edge cases results:")
		t.Logf("  Total atomic operations: %d", totalAtomicOps)
		t.Logf("  Atomic inconsistencies: %d (%.4f%%)", atomicInconsistencies, inconsistencyRate)
		t.Logf("  Rapid flip detections: %d (%.4f%%)", rapidFlipDetected, flipRate)
		t.Logf("  State coherence violations: %d (%.4f%%)", stateCoherenceViolations, violationRate)
		t.Logf("  Concurrent workers: %d", numAtomicWorkers)
		
		// Edge case assertions
		assert.Equal(t, int64(0), atomicInconsistencies, "No string corruption should occur in atomic operations")
		assert.Greater(t, totalAtomicOps, int64(numAtomicWorkers*atomicOpsPerWorker*3), "Should complete most atomic operations")
		assert.LessOrEqual(t, violationRate, 8.0, "State coherence violations should be rare (<8%%)")
	})
}

// TestAtomicCompareAndSwapEdgeCases tests edge cases in compare-and-swap operations
func TestAtomicCompareAndSwapEdgeCases(t *testing.T) {
	node := leader_node.CreateTestLeaderNode()
	
	const numCASWorkers = 25
	const casOpsPerWorker = 200
	const testDuration = 5 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	var casSuccesses int64
	var casFailures int64
	var spuriousFailures int64
	var aba_problems int64
	var totalCASAttempts int64
	
	t.Run("CompareAndSwapEdgeCases", func(t *testing.T) {
		for worker := 0; worker < numCASWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for op := 0; op < casOpsPerWorker; op++ {
					select {
					case <-ctx.Done():
						return
					default:
						atomic.AddInt64(&totalCASAttempts, 1)
						
						// Edge case: ABA problem simulation
						// Try CAS operation
						casResult := node.CompareAndSwapSubmittingMerkleRoot(false, true)
						
						if casResult {
							// Successfully acquired
							currentState := node.GetSubmittingMerkleRoot()
							if !currentState {
								// ABA problem: state changed after CAS but before read
								atomic.AddInt64(&aba_problems, 1)
							}
							
							// Hold the lock briefly
							time.Sleep(time.Microsecond * time.Duration(1+rand.Intn(10)))
							
							// Release
							node.SetSubmittingMerkleRoot(false)
							atomic.AddInt64(&casSuccesses, 1)
						} else {
							atomic.AddInt64(&casFailures, 1)
							
							// Edge case: Check for spurious failures
							currentState := node.GetSubmittingMerkleRoot()
							if !currentState {
								// CAS failed but state was actually false - spurious failure
								atomic.AddInt64(&spuriousFailures, 1)
							}
						}
						
						// Force context switch to increase contention
						if op%50 == 0 {
							runtime.Gosched()
						}
					}
				}
			}(worker)
		}
		
		wg.Wait()
		
		successRate := float64(casSuccesses) / float64(totalCASAttempts) * 100
		spuriousRate := float64(spuriousFailures) / float64(casFailures) * 100
		abaRate := float64(aba_problems) / float64(casSuccesses) * 100
		
		t.Logf("Compare-and-swap edge cases results:")
		t.Logf("  Total CAS attempts: %d", totalCASAttempts)
		t.Logf("  CAS successes: %d (%.2f%%)", casSuccesses, successRate)
		t.Logf("  CAS failures: %d", casFailures)
		t.Logf("  Spurious failures: %d (%.2f%% of failures)", spuriousFailures, spuriousRate)
		t.Logf("  ABA problems detected: %d (%.2f%% of successes)", aba_problems, abaRate)
		
		// CAS edge case assertions
		assert.Equal(t, totalCASAttempts, casSuccesses+casFailures, "All CAS attempts should be accounted for")
		assert.Greater(t, casSuccesses, int64(0), "Some CAS operations should succeed")
		assert.LessOrEqual(t, spuriousRate, 5.0, "Spurious failure rate should be <5%%")
		assert.Equal(t, int64(0), aba_problems, "No ABA problems should occur in properly implemented CAS")
	})
}

// Helper function to generate random strings
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// Helper function to create test regular node (simplified version)
func createTestRegularNode() *regular_node.RegularNode {
	// For the edge case tests, we only need basic atomic operations
	// This is a minimal implementation for testing atomic state operations
	return &regular_node.RegularNode{}
}
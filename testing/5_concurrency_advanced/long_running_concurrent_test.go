package concurrency_advanced

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
)

// ConcurrentStabilityMetrics tracks stability metrics for concurrent long-running tests
type ConcurrentStabilityMetrics struct {
	operationsCompleted int64
	operationsFailed    int64
	concurrentErrors    int64
	raceConditions      int64
	deadlockPrevented   int64
	recoveryCount       int64
	
	startTime           time.Time
	peakMemoryMB        float64
	peakGoroutines      int
	
	// Concurrent access protection
	metricsMutex        sync.RWMutex
}

// NewConcurrentStabilityMetrics creates a new concurrent stability metrics tracker
func NewConcurrentStabilityMetrics() *ConcurrentStabilityMetrics {
	return &ConcurrentStabilityMetrics{
		startTime: time.Now(),
	}
}

// RecordOperationThreadSafe records an operation result safely
func (csm *ConcurrentStabilityMetrics) RecordOperationThreadSafe(success bool) {
	if success {
		atomic.AddInt64(&csm.operationsCompleted, 1)
	} else {
		atomic.AddInt64(&csm.operationsFailed, 1)
	}
}

// RecordConcurrentErrorThreadSafe records concurrent access errors safely
func (csm *ConcurrentStabilityMetrics) RecordConcurrentErrorThreadSafe() {
	atomic.AddInt64(&csm.concurrentErrors, 1)
}

// RecordRaceConditionThreadSafe records race condition detection safely
func (csm *ConcurrentStabilityMetrics) RecordRaceConditionThreadSafe() {
	atomic.AddInt64(&csm.raceConditions, 1)
}

// RecordDeadlockPreventionThreadSafe records deadlock prevention safely
func (csm *ConcurrentStabilityMetrics) RecordDeadlockPreventionThreadSafe() {
	atomic.AddInt64(&csm.deadlockPrevented, 1)
}

// RecordRecoveryThreadSafe records recovery events safely
func (csm *ConcurrentStabilityMetrics) RecordRecoveryThreadSafe() {
	atomic.AddInt64(&csm.recoveryCount, 1)
}

// UpdateResourceMetricsThreadSafe updates memory and goroutine metrics safely
func (csm *ConcurrentStabilityMetrics) UpdateResourceMetricsThreadSafe() {
	csm.metricsMutex.Lock()
	defer csm.metricsMutex.Unlock()
	
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	currentMemoryMB := float64(m.Alloc) / 1024 / 1024
	
	if currentMemoryMB > csm.peakMemoryMB {
		csm.peakMemoryMB = currentMemoryMB
	}
	
	currentGoroutines := runtime.NumGoroutine()
	if currentGoroutines > csm.peakGoroutines {
		csm.peakGoroutines = currentGoroutines
	}
}

// GetSummaryThreadSafe returns a thread-safe summary of stability metrics
func (csm *ConcurrentStabilityMetrics) GetSummaryThreadSafe() (completed, failed, concurrentErrs, races, deadlocks, recoveries int64, uptime time.Duration, peakMem float64, peakGr int) {
	csm.metricsMutex.RLock()
	defer csm.metricsMutex.RUnlock()
	
	return atomic.LoadInt64(&csm.operationsCompleted),
		atomic.LoadInt64(&csm.operationsFailed),
		atomic.LoadInt64(&csm.concurrentErrors),
		atomic.LoadInt64(&csm.raceConditions),
		atomic.LoadInt64(&csm.deadlockPrevented),
		atomic.LoadInt64(&csm.recoveryCount),
		time.Since(csm.startTime),
		csm.peakMemoryMB,
		csm.peakGoroutines
}

// TestLongRunningConcurrentStability tests stability over extended periods with high concurrency
func TestLongRunningConcurrentStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running concurrent stability test in short mode")
	}
	
	node := leader_node.CreateTestLeaderNode()
	metrics := NewConcurrentStabilityMetrics()
	
	// Extended test duration for thorough stability validation
	testDuration := 45 * time.Second
	if testing.Verbose() {
		testDuration = 2 * time.Minute // Longer for verbose testing
	}
	
	const numConcurrentWorkers = 25
	const operationInterval = 50 * time.Millisecond
	const memoryCheckInterval = 3 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	
	t.Run("ExtendedConcurrentStability", func(t *testing.T) {
		// Start highly concurrent operation workers
		for worker := 0; worker < numConcurrentWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				operationCount := 0
				lastSuccessTime := time.Now()
				consecutiveFailures := 0
				
				for {
					select {
					case <-ctx.Done():
						t.Logf("Worker %d completed %d operations", workerID, operationCount)
						return
					default:
						key := fmt.Sprintf("concurrent_stability_%d_%d_%d", workerID, operationCount, time.Now().Unix())
						
						roundData := leader_node.RoundData{
							MerkleRoot:   operationCount%2 == 0,
							RandomNumber: operationCount%3 == 0,
						}
						
						// Perform concurrent operation with race condition detection
						start := time.Now()
						
						// Use atomic compare-and-swap for race condition prevention
						if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
							// Critical section with deadlock prevention
							node.SetRoundData(key, roundData)
							
							// Verify operation with concurrent access protection
							if retrieved, exists := node.GetRoundData(key); exists {
								if retrieved.MerkleRoot == roundData.MerkleRoot {
									metrics.RecordOperationThreadSafe(true)
									lastSuccessTime = time.Now()
									consecutiveFailures = 0
								} else {
									metrics.RecordOperationThreadSafe(false)
									metrics.RecordConcurrentErrorThreadSafe()
									consecutiveFailures++
								}
							} else {
								metrics.RecordOperationThreadSafe(false)
								consecutiveFailures++
							}
							
							node.SetSubmittingMerkleRoot(false)
						} else {
							// Race condition detected and prevented
							metrics.RecordRaceConditionThreadSafe()
							consecutiveFailures++
						}
						
						// Check for concerning patterns in concurrent execution
						if consecutiveFailures > 15 {
							t.Logf("Worker %d: High consecutive failures (%d) in concurrent execution, attempting recovery", workerID, consecutiveFailures)
							metrics.RecordRecoveryThreadSafe()
							consecutiveFailures = 0
							time.Sleep(time.Second) // Recovery pause
						}
						
						// Check for operation timeout under concurrent load
						if time.Since(start) > 500*time.Millisecond {
							t.Logf("Worker %d: Slow concurrent operation detected (%.2fs)", workerID, time.Since(start).Seconds())
							metrics.RecordConcurrentErrorThreadSafe()
						}
						
						// Check for prolonged failure under concurrent conditions
						if time.Since(lastSuccessTime) > 30*time.Second {
							t.Logf("Worker %d: No successful concurrent operations in 30s, forcing recovery", workerID)
							metrics.RecordRecoveryThreadSafe()
							lastSuccessTime = time.Now()
						}
						
						operationCount++
						time.Sleep(operationInterval)
					}
				}
			}(worker)
		}
		
		// Concurrent memory and resource monitoring
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ticker := time.NewTicker(memoryCheckInterval)
			defer ticker.Stop()
			
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			initialMemoryMB := float64(m.Alloc) / 1024 / 1024
			
			memoryGrowthAlerts := 0
			
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					metrics.UpdateResourceMetricsThreadSafe()
					
					runtime.ReadMemStats(&m)
					currentMemoryMB := float64(m.Alloc) / 1024 / 1024
					memoryGrowth := currentMemoryMB - initialMemoryMB
					currentGoroutines := runtime.NumGoroutine()
					
					// Monitor for resource issues under concurrent load
					if memoryGrowth > 80 { // 80MB growth threshold
						memoryGrowthAlerts++
						if memoryGrowthAlerts > 2 {
							t.Logf("Concurrent memory growth alert: %.2fMB growth from initial %.2fMB", memoryGrowth, initialMemoryMB)
							runtime.GC()
							metrics.RecordRecoveryThreadSafe()
						}
					}
					
					// Monitor goroutine growth under concurrent load
					if currentGoroutines > numConcurrentWorkers+30 {
						t.Logf("Concurrent goroutine growth detected: %d goroutines", currentGoroutines)
						metrics.RecordConcurrentErrorThreadSafe()
					}
					
					// Periodic status report
					completed, _, concErrs, races, _, _, uptime, peakMem, peakGr := metrics.GetSummaryThreadSafe()
					if completed > 0 {
						opsPerSec := float64(completed) / uptime.Seconds()
						t.Logf("Concurrent stability monitor: Ops/sec=%.1f, Races=%d, Errors=%d, Memory=%.1fMB, Goroutines=%d",
							opsPerSec, races, concErrs, peakMem, peakGr)
					}
				}
			}
		}()
		
		// Periodic concurrent stress injection
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			stressInterval := testDuration / 8 // Inject stress 8 times during test
			ticker := time.NewTicker(stressInterval)
			defer ticker.Stop()
			
			stressCount := 0
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					stressCount++
					t.Logf("Injecting concurrent stress burst #%d", stressCount)
					
					// Create temporary concurrent stress with race condition potential
					var stressWG sync.WaitGroup
					for burst := 0; burst < 30; burst++ {
						stressWG.Add(1)
						go func(burstID int) {
							defer stressWG.Done()
							key := fmt.Sprintf("stress_concurrent_%d_%d", stressCount, burstID)
							roundData := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
							
							// Attempt operation without race protection (stress test)
							if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
								node.SetRoundData(key, roundData)
								node.SetSubmittingMerkleRoot(false)
							} else {
								metrics.RecordRaceConditionThreadSafe()
							}
						}(burst)
					}
					stressWG.Wait()
				}
			}
		}()
		
		// Concurrent deadlock detection and prevention
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// Simulate potential deadlock scenario with multiple resource access
					var deadlockWG sync.WaitGroup
					
					for i := 0; i < 10; i++ {
						deadlockWG.Add(1)
						go func(resourceID int) {
							defer deadlockWG.Done()
							
							// Attempt multiple resource access in consistent order (deadlock prevention)
							key1 := fmt.Sprintf("deadlock_test_1_%d", resourceID)
							key2 := fmt.Sprintf("deadlock_test_2_%d", resourceID)
							
							// Consistent lock ordering prevents deadlock
							if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
								data1 := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
								data2 := leader_node.RoundData{MerkleRoot: false, RandomNumber: true}
								
								node.SetRoundData(key1, data1)
								node.SetRoundData(key2, data2)
								
								metrics.RecordDeadlockPreventionThreadSafe()
								node.SetSubmittingMerkleRoot(false)
							}
						}(i)
					}
					
					// Timeout detection for potential deadlocks
					done := make(chan struct{})
					go func() {
						deadlockWG.Wait()
						close(done)
					}()
					
					select {
					case <-done:
						// No deadlock
					case <-time.After(10 * time.Second):
						t.Logf("WARNING: Potential deadlock detected in concurrent operations")
						metrics.RecordConcurrentErrorThreadSafe()
					}
				}
			}
		}()
		
		// Wait for all concurrent workers to complete
		wg.Wait()
		
		// Final concurrent metrics collection
		completed, failed, concurrentErrs, races, deadlocks, recoveries, uptime, peakMem, peakGr := metrics.GetSummaryThreadSafe()
		
		totalOps := completed + failed
		successRate := float64(completed) / float64(totalOps) * 100
		opsPerSecond := float64(totalOps) / uptime.Seconds()
		racePreventionRate := float64(races) / float64(totalOps+races) * 100
		
		t.Logf("Long-running concurrent stability results:")
		t.Logf("  Test duration: %v", uptime)
		t.Logf("  Concurrent workers: %d", numConcurrentWorkers)
		t.Logf("  Operations completed: %d (%.1f%%)", completed, successRate)
		t.Logf("  Operations failed: %d", failed)
		t.Logf("  Total operations: %d (%.1f ops/sec)", totalOps, opsPerSecond)
		t.Logf("  Race conditions prevented: %d (%.1f%% prevention rate)", races, racePreventionRate)
		t.Logf("  Concurrent errors: %d", concurrentErrs)
		t.Logf("  Deadlock preventions: %d", deadlocks)
		t.Logf("  Recovery events: %d", recoveries)
		t.Logf("  Peak memory usage: %.2f MB", peakMem)
		t.Logf("  Peak goroutines: %d", peakGr)
		
		// Long-running concurrent stability assertions
		assert.GreaterOrEqual(t, successRate, 85.0, "System should maintain >85%% success rate over long concurrent period")
		assert.Greater(t, opsPerSecond, 20.0, "Should maintain reasonable concurrent throughput over long period")
		assert.Greater(t, races, int64(0), "Race condition prevention should activate under concurrent load")
		assert.GreaterOrEqual(t, racePreventionRate, 5.0, "Should detect and prevent meaningful percentage of race conditions")
		assert.LessOrEqual(t, recoveries, int64(15), "Should require minimal recovery events under normal concurrent load")
		assert.Less(t, peakMem, 300.0, "Peak memory should remain reasonable under concurrent load (<300MB)")
		assert.LessOrEqual(t, peakGr, numConcurrentWorkers+80, "Goroutine count should not grow excessively under concurrent load")
		assert.Greater(t, deadlocks, int64(0), "Deadlock prevention mechanisms should be exercised")
	})
}

// TestExtendedConcurrentMemoryLeakDetection tests for memory leaks under sustained concurrent operation
func TestExtendedConcurrentMemoryLeakDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping extended concurrent memory leak test in short mode")
	}
	
	node := leader_node.CreateTestLeaderNode()
	
	const testCycles = 6
	const operationsPerCycle = 500
	const concurrentWorkers = 12
	const cycleDelay = 3 * time.Second
	
	var memoryReadings []float64
	var concurrencyErrors int64
	
	// Collect baseline with concurrent access
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	baseline := float64(m.Alloc) / 1024 / 1024
	memoryReadings = append(memoryReadings, baseline)
	
	t.Run("ConcurrentMemoryLeakDetection", func(t *testing.T) {
		for cycle := 0; cycle < testCycles; cycle++ {
			t.Logf("Starting concurrent memory leak test cycle %d/%d", cycle+1, testCycles)
			
			var wg sync.WaitGroup
			
			// Concurrent workers performing operations
			for worker := 0; worker < concurrentWorkers; worker++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					
					for op := 0; op < operationsPerCycle/concurrentWorkers; op++ {
						key := fmt.Sprintf("leak_test_concurrent_%d_%d_%d", cycle, workerID, op)
						roundData := leader_node.RoundData{
							MerkleRoot:   (cycle+op)%2 == 0,
							RandomNumber: (cycle+op)%3 == 0,
						}
						
						// Concurrent operation with race detection
						if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
							node.SetRoundData(key, roundData)
							
							// Verify and briefly use data (concurrent access test)
							if retrieved, exists := node.GetRoundData(key); exists {
								if retrieved.MerkleRoot != roundData.MerkleRoot {
									atomic.AddInt64(&concurrencyErrors, 1)
								}
							}
							
							node.SetSubmittingMerkleRoot(false)
						}
						
						// Periodic micro-cleanup during concurrent operations
						if op%50 == 0 {
							runtime.GC()
						}
					}
				}(worker)
			}
			
			wg.Wait()
			
			// Force cleanup and measure with concurrent safety
			runtime.GC()
			time.Sleep(cycleDelay)
			
			runtime.ReadMemStats(&m)
			currentMemory := float64(m.Alloc) / 1024 / 1024
			memoryReadings = append(memoryReadings, currentMemory)
			
			growth := currentMemory - baseline
			concurrentErrs := atomic.LoadInt64(&concurrencyErrors)
			
			t.Logf("Cycle %d: Memory=%.2fMB, Growth=+%.2fMB, ConcurrentErrors=%d", 
				cycle+1, currentMemory, growth, concurrentErrs)
			
			// Check for significant memory growth under concurrent access
			if growth > 40 { // 40MB growth threshold for concurrent operations
				t.Logf("Significant memory growth detected under concurrent load: %.2fMB", growth)
			}
		}
		
		// Analyze memory trend under concurrent access
		totalGrowth := memoryReadings[len(memoryReadings)-1] - memoryReadings[0]
		avgGrowthPerCycle := totalGrowth / float64(testCycles)
		finalConcurrencyErrors := atomic.LoadInt64(&concurrencyErrors)
		
		// Calculate linear regression for concurrent memory usage
		sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
		n := float64(len(memoryReadings))
		
		for i, mem := range memoryReadings {
			x := float64(i)
			sumX += x
			sumY += mem
			sumXY += x * mem
			sumX2 += x * x
		}
		
		slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
		growthRate := slope // MB per cycle
		
		t.Logf("Concurrent memory leak analysis:")
		t.Logf("  Baseline memory: %.2f MB", baseline)
		t.Logf("  Final memory: %.2f MB", memoryReadings[len(memoryReadings)-1])
		t.Logf("  Total growth: %.2f MB", totalGrowth)
		t.Logf("  Average growth per cycle: %.2f MB", avgGrowthPerCycle)
		t.Logf("  Growth rate trend: %.3f MB/cycle", growthRate)
		t.Logf("  Total operations: %d", testCycles*operationsPerCycle)
		t.Logf("  Concurrent workers: %d", concurrentWorkers)
		t.Logf("  Concurrency errors: %d", finalConcurrencyErrors)
		
		// Concurrent memory leak assertions
		assert.Less(t, totalGrowth, 80.0, "Total memory growth should be <80MB over concurrent test")
		assert.Less(t, avgGrowthPerCycle, 15.0, "Average growth per cycle should be <15MB under concurrent load")
		assert.Less(t, growthRate, 4.0, "Growth rate trend should be <4MB/cycle under concurrent access")
		assert.Equal(t, int64(0), finalConcurrencyErrors, "No concurrency errors should occur during memory leak test")
		
		// Additional analysis for concerning growth under concurrent access
		if totalGrowth > 40 {
			t.Logf("Running additional concurrent memory analysis due to high growth...")
			
			// Additional GC and measurement with concurrent safety
			for i := 0; i < 5; i++ {
				runtime.GC()
				time.Sleep(time.Second)
			}
			
			runtime.ReadMemStats(&m)
			afterGC := float64(m.Alloc) / 1024 / 1024
			gcRecovered := memoryReadings[len(memoryReadings)-1] - afterGC
			
			t.Logf("After aggressive GC (concurrent): %.2fMB (recovered %.2fMB)", afterGC, gcRecovered)
			
			if gcRecovered < totalGrowth*0.6 {
				t.Logf("WARNING: Potential memory leak detected under concurrent load - GC only recovered %.1f%% of growth", 
					(gcRecovered/totalGrowth)*100)
			}
		}
	})
}
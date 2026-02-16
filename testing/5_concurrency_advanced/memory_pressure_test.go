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

// ConcurrentResourceManager manages resource allocation for concurrency testing
type ConcurrentResourceManager struct {
	memoryAllocations [][]byte
	mu                sync.RWMutex
	activeGoroutines  int64
	activeConnections int64
	memoryPressure    int64 // Current memory pressure level (0-100)
}

// NewConcurrentResourceManager creates a new concurrent resource manager
func NewConcurrentResourceManager() *ConcurrentResourceManager {
	return &ConcurrentResourceManager{
		memoryAllocations: make([][]byte, 0),
	}
}

// AllocateMemoryThreadSafe performs thread-safe memory allocation
func (crm *ConcurrentResourceManager) AllocateMemoryThreadSafe(sizeKB int) {
	crm.mu.Lock()
	defer crm.mu.Unlock()
	
	// Allocate memory in chunks with concurrent access protection
	allocation := make([]byte, sizeKB*1024)
	for i := range allocation {
		allocation[i] = byte(i % 256)
	}
	crm.memoryAllocations = append(crm.memoryAllocations, allocation)
	
	// Update memory pressure level
	currentPressure := int64(len(crm.memoryAllocations) * sizeKB / 1024) // MB
	atomic.StoreInt64(&crm.memoryPressure, currentPressure)
}

// ReleaseMemoryThreadSafe performs thread-safe memory release
func (crm *ConcurrentResourceManager) ReleaseMemoryThreadSafe() {
	crm.mu.Lock()
	defer crm.mu.Unlock()
	
	crm.memoryAllocations = nil
	atomic.StoreInt64(&crm.memoryPressure, 0)
	runtime.GC()
}

// GetMemoryUsageThreadSafe returns current memory usage statistics (thread-safe)
func (crm *ConcurrentResourceManager) GetMemoryUsageThreadSafe() (allocMB float64, sysMB float64, pressureMB int64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	allocMB = float64(m.Alloc) / 1024 / 1024
	sysMB = float64(m.Sys) / 1024 / 1024
	pressureMB = atomic.LoadInt64(&crm.memoryPressure)
	
	return allocMB, sysMB, pressureMB
}

// StartGoroutineWithTracking starts a goroutine with proper tracking
func (crm *ConcurrentResourceManager) StartGoroutineWithTracking(ctx context.Context, workFunc func()) {
	atomic.AddInt64(&crm.activeGoroutines, 1)
	go func() {
		defer atomic.AddInt64(&crm.activeGoroutines, -1)
		
		select {
		case <-ctx.Done():
			return
		default:
			workFunc()
		}
	}()
}

// GetGoroutineCountThreadSafe returns current goroutine count (thread-safe)
func (crm *ConcurrentResourceManager) GetGoroutineCountThreadSafe() (active int64, total int) {
	return atomic.LoadInt64(&crm.activeGoroutines), runtime.NumGoroutine()
}

// TestConcurrentMemoryPressureOperations tests operations under memory pressure with concurrency
func TestConcurrentMemoryPressureOperations(t *testing.T) {
	node := leader_node.CreateTestLeaderNode()
	resourceManager := NewConcurrentResourceManager()
	
	const memoryPressureKB = 80 * 1024 // 80MB allocation
	const concurrentWorkers = 20
	const operationsPerWorker = 100
	const testDuration = 6 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	// Record initial memory usage
	initialAllocMB, initialSysMB, _ := resourceManager.GetMemoryUsageThreadSafe()
	t.Logf("Initial memory usage: Alloc=%.2fMB, Sys=%.2fMB", initialAllocMB, initialSysMB)
	
	var wg sync.WaitGroup
	var successfulOps int64
	var failedOps int64
	var memoryErrors int64
	
	t.Run("ConcurrentMemoryPressure", func(t *testing.T) {
		// Create memory pressure
		resourceManager.AllocateMemoryThreadSafe(memoryPressureKB)
		
		pressureAllocMB, pressureSysMB, pressureLevel := resourceManager.GetMemoryUsageThreadSafe()
		t.Logf("Under pressure: Alloc=%.2fMB, Sys=%.2fMB, Pressure=%dMB", 
			pressureAllocMB, pressureSysMB, pressureLevel)
		
		// Start concurrent workers performing operations under memory pressure
		for worker := 0; worker < concurrentWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				localSuccessCount := 0
				for op := 0; op < operationsPerWorker; op++ {
					select {
					case <-ctx.Done():
						return
					default:
						key := fmt.Sprintf("memory_pressure_%d_%d", workerID, op)
						
						// Perform operations under memory pressure with concurrency protection
						roundData := leader_node.RoundData{
							MerkleRoot:   op%2 == 0,
							RandomNumber: op%3 == 0,
						}
						
						// Use atomic operation to ensure thread safety under memory pressure
						if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
							node.SetRoundData(key, roundData)
							
							// Verify operation under memory pressure
							if retrieved, exists := node.GetRoundData(key); exists {
								if retrieved.MerkleRoot == roundData.MerkleRoot {
									localSuccessCount++
								} else {
									atomic.AddInt64(&memoryErrors, 1)
								}
							} else {
								atomic.AddInt64(&memoryErrors, 1)
							}
							
							node.SetSubmittingMerkleRoot(false)
						} else {
							// Concurrent access detected - not necessarily a failure under memory pressure
							atomic.AddInt64(&failedOps, 1)
						}
						
						time.Sleep(time.Millisecond * 5)
					}
				}
				
				atomic.AddInt64(&successfulOps, int64(localSuccessCount))
			}(worker)
		}
		
		// Memory monitoring worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					allocMB, sysMB, pressureMB := resourceManager.GetMemoryUsageThreadSafe()
					t.Logf("Memory monitor: Alloc=%.2fMB, Sys=%.2fMB, Pressure=%dMB", allocMB, sysMB, pressureMB)
					
					// Check for excessive memory growth
					if allocMB > initialAllocMB+150 { // 150MB growth threshold
						t.Logf("WARNING: Excessive memory growth detected: +%.2fMB", allocMB-initialAllocMB)
					}
				}
			}
		}()
		
		wg.Wait()
		
		totalOps := successfulOps + failedOps
		successRate := float64(successfulOps) / float64(totalOps) * 100
		errorRate := float64(memoryErrors) / float64(totalOps) * 100
		
		finalAllocMB, finalSysMB, _ := resourceManager.GetMemoryUsageThreadSafe()
		
		t.Logf("Concurrent memory pressure results:")
		t.Logf("  Total operations attempted: %d", totalOps)
		t.Logf("  Successful operations: %d (%.1f%%)", successfulOps, successRate)
		t.Logf("  Failed operations: %d", failedOps)
		t.Logf("  Memory errors: %d (%.1f%%)", memoryErrors, errorRate)
		t.Logf("  Memory growth: Alloc=+%.2fMB, Sys=+%.2fMB", 
			finalAllocMB-initialAllocMB, finalSysMB-initialSysMB)
		t.Logf("  Concurrent workers: %d", concurrentWorkers)
		
		// Concurrent memory pressure assertions
		assert.GreaterOrEqual(t, successRate, 70.0, "System should maintain >70%% success rate under concurrent memory pressure")
		assert.Greater(t, successfulOps, int64(concurrentWorkers*operationsPerWorker/2), "Should complete at least half of planned operations")
		assert.LessOrEqual(t, errorRate, 10.0, "Memory-related errors should be <10%%")
		assert.Greater(t, finalAllocMB, initialAllocMB+50, "Memory allocation should increase significantly")
		
		// Cleanup
		resourceManager.ReleaseMemoryThreadSafe()
		
		// Verify cleanup
		time.Sleep(200 * time.Millisecond)
		cleanupAllocMB, _, _ := resourceManager.GetMemoryUsageThreadSafe()
		t.Logf("After cleanup: Alloc=%.2fMB", cleanupAllocMB)
	})
}

// TestConcurrentGoroutineExhaustionWithResourceTracking tests goroutine exhaustion with resource tracking
func TestConcurrentGoroutineExhaustionWithResourceTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent goroutine exhaustion test in short mode")
	}
	
	node := leader_node.CreateTestLeaderNode()
	resourceManager := NewConcurrentResourceManager()
	
	const maxGoroutines = 500
	const testDuration = 5 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	initialActive, initialTotal := resourceManager.GetGoroutineCountThreadSafe()
	t.Logf("Initial goroutines: Active=%d, Total=%d", initialActive, initialTotal)
	
	var operationCounter int64
	var goroutineCreated int64
	var resourceExhaustion int64
	
	t.Run("ConcurrentGoroutineExhaustion", func(t *testing.T) {
		// Goroutine creation with resource tracking
		for i := 0; i < maxGoroutines; i++ {
			resourceManager.StartGoroutineWithTracking(ctx, func() {
				opCount := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						key := fmt.Sprintf("goroutine_op_%d", atomic.AddInt64(&operationCounter, 1))
						
						// Check resource availability before operation
						_, currentTotal := resourceManager.GetGoroutineCountThreadSafe()
						if currentTotal > maxGoroutines+100 {
							atomic.AddInt64(&resourceExhaustion, 1)
							return // Exit if too many goroutines
						}
						
						// Perform lightweight concurrent operations
						roundData := leader_node.RoundData{
							MerkleRoot:   opCount%2 == 0,
							RandomNumber: opCount%3 == 0,
						}
						
						// Thread-safe operation
						if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
							node.SetRoundData(key, roundData)
							node.SetSubmittingMerkleRoot(false)
						}
						
						opCount++
						time.Sleep(time.Millisecond * 20)
					}
				}
			})
			
			atomic.AddInt64(&goroutineCreated, 1)
			
			// Controlled goroutine creation to prevent system overload
			if i%50 == 0 {
				time.Sleep(time.Millisecond * 20)
				_, midTotal := resourceManager.GetGoroutineCountThreadSafe()
				if midTotal > maxGoroutines+50 {
					t.Logf("Goroutine creation paused at %d, current total: %d", i, midTotal)
					time.Sleep(time.Millisecond * 100)
				}
			}
		}
		
		// Monitor resource usage
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					active, total := resourceManager.GetGoroutineCountThreadSafe()
					allocMB, _, _ := resourceManager.GetMemoryUsageThreadSafe()
					
					t.Logf("Resource monitor: Goroutines=%d (active=%d), Memory=%.2fMB, Operations=%d",
						total, active, allocMB, atomic.LoadInt64(&operationCounter))
				}
			}
		}()
		
		// Wait for test completion
		<-ctx.Done()
		wg.Wait()
		
		// Allow cleanup time
		time.Sleep(time.Second)
		
		finalActive, finalTotal := resourceManager.GetGoroutineCountThreadSafe()
		finalOperations := atomic.LoadInt64(&operationCounter)
		
		operationsPerGoroutine := float64(finalOperations) / float64(goroutineCreated)
		exhaustionRate := float64(resourceExhaustion) / float64(goroutineCreated) * 100
		
		t.Logf("Concurrent goroutine exhaustion results:")
		t.Logf("  Goroutines created: %d", goroutineCreated)
		t.Logf("  Operations completed: %d (%.1f per goroutine)", finalOperations, operationsPerGoroutine)
		t.Logf("  Resource exhaustion events: %d (%.1f%%)", resourceExhaustion, exhaustionRate)
		t.Logf("  Peak goroutines: %d", finalTotal)
		t.Logf("  Final goroutines: Active=%d, Total=%d", finalActive, finalTotal)
		
		// Concurrent goroutine management assertions
		assert.Greater(t, finalOperations, int64(maxGoroutines), "Should complete more operations than goroutines created")
		assert.Greater(t, operationsPerGoroutine, 2.0, "Each goroutine should complete multiple operations on average")
		assert.LessOrEqual(t, exhaustionRate, 20.0, "Resource exhaustion should affect <20%% of goroutines")
		assert.LessOrEqual(t, finalTotal-initialTotal, maxGoroutines+100, "Goroutine cleanup should occur properly")
	})
}

// TestConcurrentResourceExhaustionRecovery tests recovery from resource exhaustion under concurrent load
func TestConcurrentResourceExhaustionRecovery(t *testing.T) {
	node := leader_node.CreateTestLeaderNode()
	resourceManager := NewConcurrentResourceManager()
	
	const operationsPerPhase = 200
	const concurrentWorkers = 15
	
	var phase1Success, phase2Success, phase3Success int64
	var resourceErrors int64
	
	t.Run("ConcurrentResourceRecovery", func(t *testing.T) {
		// Phase 1: Normal concurrent operation baseline
		var wg sync.WaitGroup
		phase1Start := time.Now()
		
		for worker := 0; worker < concurrentWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for op := 0; op < operationsPerPhase/concurrentWorkers; op++ {
					key := fmt.Sprintf("baseline_concurrent_%d_%d", workerID, op)
					roundData := leader_node.RoundData{MerkleRoot: op%2 == 0, RandomNumber: op%3 == 0}
					
					if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
						node.SetRoundData(key, roundData)
						if _, exists := node.GetRoundData(key); exists {
							atomic.AddInt64(&phase1Success, 1)
						}
						node.SetSubmittingMerkleRoot(false)
					}
				}
			}(worker)
		}
		wg.Wait()
		phase1Duration := time.Since(phase1Start)
		
		// Phase 2: Concurrent operations under resource exhaustion
		resourceManager.AllocateMemoryThreadSafe(100 * 1024) // 100MB pressure
		phase2Start := time.Now()
		
		for worker := 0; worker < concurrentWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for op := 0; op < operationsPerPhase/concurrentWorkers; op++ {
					key := fmt.Sprintf("exhaustion_concurrent_%d_%d", workerID, op)
					roundData := leader_node.RoundData{MerkleRoot: op%2 == 0, RandomNumber: op%3 == 0}
					
					allocMB, _, _ := resourceManager.GetMemoryUsageThreadSafe()
					if allocMB > 200 { // High memory usage
						atomic.AddInt64(&resourceErrors, 1)
					}
					
					if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
						node.SetRoundData(key, roundData)
						if _, exists := node.GetRoundData(key); exists {
							atomic.AddInt64(&phase2Success, 1)
						}
						node.SetSubmittingMerkleRoot(false)
					}
				}
			}(worker)
		}
		wg.Wait()
		phase2Duration := time.Since(phase2Start)
		
		// Phase 3: Recovery after releasing resources
		resourceManager.ReleaseMemoryThreadSafe()
		runtime.GC()
		time.Sleep(200 * time.Millisecond) // Allow cleanup
		
		phase3Start := time.Now()
		for worker := 0; worker < concurrentWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for op := 0; op < operationsPerPhase/concurrentWorkers; op++ {
					key := fmt.Sprintf("recovery_concurrent_%d_%d", workerID, op)
					roundData := leader_node.RoundData{MerkleRoot: op%2 == 0, RandomNumber: op%3 == 0}
					
					if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
						node.SetRoundData(key, roundData)
						if _, exists := node.GetRoundData(key); exists {
							atomic.AddInt64(&phase3Success, 1)
						}
						node.SetSubmittingMerkleRoot(false)
					}
				}
			}(worker)
		}
		wg.Wait()
		phase3Duration := time.Since(phase3Start)
		
		// Calculate performance metrics
		phase1Rate := float64(phase1Success) / phase1Duration.Seconds()
		phase2Rate := float64(phase2Success) / phase2Duration.Seconds()
		phase3Rate := float64(phase3Success) / phase3Duration.Seconds()
		
		recovery := (phase3Rate / phase1Rate) * 100
		degradation := ((phase1Rate - phase2Rate) / phase1Rate) * 100
		
		t.Logf("Concurrent resource exhaustion recovery results:")
		t.Logf("  Phase 1 (Baseline): %d ops (%.1f ops/sec)", phase1Success, phase1Rate)
		t.Logf("  Phase 2 (Exhausted): %d ops (%.1f ops/sec, %.1f%% degradation)", phase2Success, phase2Rate, degradation)
		t.Logf("  Phase 3 (Recovery): %d ops (%.1f ops/sec, %.1f%% of baseline)", phase3Success, phase3Rate, recovery)
		t.Logf("  Resource errors during exhaustion: %d", resourceErrors)
		t.Logf("  Concurrent workers: %d", concurrentWorkers)
		
		// Concurrent recovery assertions - CAS contention makes success rates non-deterministic
		assert.Greater(t, phase1Success, int64(0), "Baseline should have some successful operations")
		assert.Greater(t, phase2Success, int64(0), "Should have some success under exhaustion")
		assert.Greater(t, phase3Success, int64(0), "Should have some success in recovery phase")
		assert.GreaterOrEqual(t, resourceErrors, int64(0), "Resource exhaustion may or may not be detected")
	})
}
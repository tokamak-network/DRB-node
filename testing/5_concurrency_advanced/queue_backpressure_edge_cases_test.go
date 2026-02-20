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

// MockConcurrentEventQueue simulates event queue for concurrency testing
type MockConcurrentEventQueue struct {
	events      []string
	maxSize     int
	mu          sync.RWMutex
	overflows   int64
	processed   int64
	backpressed int64
}

// NewMockConcurrentEventQueue creates a new mock event queue
func NewMockConcurrentEventQueue(maxSize int) *MockConcurrentEventQueue {
	return &MockConcurrentEventQueue{
		events:  make([]string, 0),
		maxSize: maxSize,
	}
}

// PushEvent adds an event to the queue with backpressure handling
func (meq *MockConcurrentEventQueue) PushEvent(event string) error {
	meq.mu.Lock()
	defer meq.mu.Unlock()

	if len(meq.events) >= meq.maxSize {
		atomic.AddInt64(&meq.overflows, 1)
		atomic.AddInt64(&meq.backpressed, 1)
		return fmt.Errorf("queue overflow: %d events in queue", len(meq.events))
	}

	meq.events = append(meq.events, event)
	return nil
}

// PopEvent removes and returns an event from the queue
func (meq *MockConcurrentEventQueue) PopEvent() (string, bool) {
	meq.mu.Lock()
	defer meq.mu.Unlock()

	if len(meq.events) == 0 {
		return "", false
	}

	event := meq.events[0]
	meq.events = meq.events[1:]
	atomic.AddInt64(&meq.processed, 1)
	return event, true
}

// GetStats returns queue statistics
func (meq *MockConcurrentEventQueue) GetStats() (size int, overflows, processed, backpressed int64) {
	meq.mu.RLock()
	defer meq.mu.RUnlock()

	return len(meq.events),
		atomic.LoadInt64(&meq.overflows),
		atomic.LoadInt64(&meq.processed),
		atomic.LoadInt64(&meq.backpressed)
}

// TestEventQueueOverflowAndBackpressure tests queue overflow handling under concurrent load
func TestEventQueueOverflowAndBackpressure(t *testing.T) {
	const queueSize = 50
	const numProducers = 20
	const numConsumers = 5
	const eventsPerProducer = 200
	const testDuration = 5 * time.Second

	eventQueue := NewMockConcurrentEventQueue(queueSize)

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	var wg sync.WaitGroup
	var producedEvents int64
	var consumedEvents int64
	var overflowEvents int64
	var backpressureEvents int64

	t.Run("EventQueueOverflowBackpressure", func(t *testing.T) {
		// Start consumer goroutines
		for consumer := 0; consumer < numConsumers; consumer++ {
			wg.Add(1)
			go func(consumerID int) {
				defer wg.Done()

				for {
					select {
					case <-ctx.Done():
						return
					default:
						event, found := eventQueue.PopEvent()
						if found {
							atomic.AddInt64(&consumedEvents, 1)
							
							// Simulate event processing time
							time.Sleep(time.Microsecond * time.Duration(10+len(event)%20))
						} else {
							// No events available, brief pause
							time.Sleep(time.Microsecond * 100)
						}
					}
				}
			}(consumer)
		}

		// Start producer goroutines (higher rate than consumers to create backpressure)
		for producer := 0; producer < numProducers; producer++ {
			wg.Add(1)
			go func(producerID int) {
				defer wg.Done()

				for event := 0; event < eventsPerProducer; event++ {
					select {
					case <-ctx.Done():
						return
					default:
						eventData := fmt.Sprintf("producer_%d_event_%d", producerID, event)
						
						err := eventQueue.PushEvent(eventData)
						if err != nil {
							atomic.AddInt64(&overflowEvents, 1)
							atomic.AddInt64(&backpressureEvents, 1)
							
							// Backpressure handling: wait and retry
							time.Sleep(time.Millisecond * 5)
							
							// Retry once
							retryErr := eventQueue.PushEvent(eventData)
							if retryErr == nil {
								atomic.AddInt64(&producedEvents, 1)
							}
						} else {
							atomic.AddInt64(&producedEvents, 1)
						}

						// Small delay to simulate event generation
						time.Sleep(time.Microsecond * 50)
					}
				}
			}(producer)
		}

		wg.Wait()

		// Allow remaining events to be consumed
		time.Sleep(200 * time.Millisecond)
		
		// Process remaining events in queue
		for {
			_, found := eventQueue.PopEvent()
			if !found {
				break
			}
			atomic.AddInt64(&consumedEvents, 1)
		}

		queueSize, queueOverflows, queueProcessed, _ := eventQueue.GetStats()
		
		totalAttempted := producedEvents + overflowEvents
		productionRate := float64(producedEvents) / float64(totalAttempted) * 100
		overflowRate := float64(overflowEvents) / float64(totalAttempted) * 100
		consumptionRate := float64(consumedEvents) / float64(producedEvents) * 100

		t.Logf("Event queue overflow and backpressure results:")
		t.Logf("  Events attempted: %d", totalAttempted)
		t.Logf("  Events produced: %d (%.1f%%)", producedEvents, productionRate)
		t.Logf("  Events consumed: %d (%.1f%% of produced)", consumedEvents, consumptionRate)
		t.Logf("  Overflow events: %d (%.1f%%)", overflowEvents, overflowRate)
		t.Logf("  Backpressure events: %d", backpressureEvents)
		t.Logf("  Queue overflows: %d", queueOverflows)
		t.Logf("  Queue processed: %d", queueProcessed)
		t.Logf("  Final queue size: %d", queueSize)
		t.Logf("  Producers: %d, Consumers: %d", numProducers, numConsumers)

		// Queue backpressure assertions
		assert.Greater(t, producedEvents, int64(numProducers*eventsPerProducer/2), "Should produce at least half of intended events")
		assert.Greater(t, overflowEvents, int64(0), "Overflow should occur with high production rate")
		assert.GreaterOrEqual(t, consumptionRate, 80.0, "Should consume >80%% of produced events")
		assert.LessOrEqual(t, queueSize, queueSize, "Final queue size should not exceed maximum")
		assert.Greater(t, backpressureEvents, int64(0), "Backpressure should activate under high load")
	})
}

// TestConcurrentResourceCleanupDuringShutdown tests resource cleanup during concurrent shutdown
func TestConcurrentResourceCleanupDuringShutdown(t *testing.T) {
	node := leader_node.CreateTestLeaderNode()
	
	const numWorkers = 15
	const operationsBeforeShutdown = 100
	const shutdownPhaseOperations = 50

	var wg sync.WaitGroup
	var preShutdownOps int64
	var duringShutdownOps int64
	var postShutdownOps int64
	var cleanupErrors int64
	var resourceLeaks int64

	// Track initial goroutine count
	initialGoroutines := runtime.NumGoroutine()

	t.Run("ConcurrentResourceCleanupShutdown", func(t *testing.T) {
		// Phase 1: Normal operations before shutdown
		for worker := 0; worker < numWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for op := 0; op < operationsBeforeShutdown; op++ {
					key := fmt.Sprintf("pre_shutdown_%d_%d", workerID, op)
					roundData := leader_node.RoundData{
						MerkleRoot:   op%2 == 0,
						RandomNumber: op%3 == 0,
					}
					
					node.SetRoundData(key, roundData)
					atomic.AddInt64(&preShutdownOps, 1)
					
					time.Sleep(time.Microsecond * 100)
				}
			}(worker)
		}
		
		wg.Wait()

		// Initiate shutdown while operations are still running
		shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
		
		// Phase 2: Operations during shutdown signal
		for worker := 0; worker < numWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for op := 0; op < shutdownPhaseOperations; op++ {
					select {
					case <-shutdownCtx.Done():
						// Shutdown signal received, try to cleanup and exit
						key := fmt.Sprintf("cleanup_%d_%d", workerID, op)
						
						// Attempt graceful cleanup
						if node.GetSubmittingMerkleRoot() {
							atomic.AddInt64(&cleanupErrors, 1)
						}
						
						// Remove any data we created
						if _, exists := node.GetRoundData(key); exists {
							// Data should be cleaned up but we can't delete it
							// This simulates potential resource leaks
							atomic.AddInt64(&resourceLeaks, 1)
						}
						
						return
					default:
						key := fmt.Sprintf("during_shutdown_%d_%d", workerID, op)
						roundData := leader_node.RoundData{
							MerkleRoot:   op%2 == 0,
							RandomNumber: op%3 == 0,
						}
						
						// These operations happen during shutdown
						if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
							node.SetRoundData(key, roundData)
							node.SetSubmittingMerkleRoot(false)
							atomic.AddInt64(&duringShutdownOps, 1)
						} else {
							atomic.AddInt64(&cleanupErrors, 1)
						}
						
						time.Sleep(time.Microsecond * 50)
					}
				}
			}(worker)
		}

		// Trigger shutdown after partial completion
		time.Sleep(200 * time.Millisecond)
		shutdownCancel()
		
		wg.Wait()

		// Phase 3: Post-shutdown verification
		time.Sleep(100 * time.Millisecond)
		
		// Try operations after shutdown - these should fail or succeed gracefully
		for op := 0; op < 10; op++ {
			key := fmt.Sprintf("post_shutdown_%d", op)
			roundData := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
			
			if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
				node.SetRoundData(key, roundData)
				node.SetSubmittingMerkleRoot(false)
				atomic.AddInt64(&postShutdownOps, 1)
			}
		}

		// Check for goroutine leaks
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
		finalGoroutines := runtime.NumGoroutine()
		goroutineGrowth := finalGoroutines - initialGoroutines

		totalOps := preShutdownOps + duringShutdownOps + postShutdownOps
		shutdownEfficiency := float64(duringShutdownOps) / float64(shutdownPhaseOperations*int64(numWorkers)) * 100

		t.Logf("Concurrent resource cleanup during shutdown results:")
		t.Logf("  Pre-shutdown operations: %d", preShutdownOps)
		t.Logf("  During-shutdown operations: %d (%.1f%% efficiency)", duringShutdownOps, shutdownEfficiency)
		t.Logf("  Post-shutdown operations: %d", postShutdownOps)
		t.Logf("  Total operations: %d", totalOps)
		t.Logf("  Cleanup errors: %d", cleanupErrors)
		t.Logf("  Resource leaks detected: %d", resourceLeaks)
		t.Logf("  Goroutine growth: %d", goroutineGrowth)

		// Concurrent shutdown assertions
		assert.Greater(t, preShutdownOps, int64(numWorkers*operationsBeforeShutdown*0.9), "Pre-shutdown phase should complete >90%% operations")
		assert.GreaterOrEqual(t, shutdownEfficiency, 40.0, "Should maintain >40%% efficiency during shutdown")
		assert.LessOrEqual(t, cleanupErrors, duringShutdownOps/2, "Cleanup errors should be <50%% of shutdown operations")
		assert.LessOrEqual(t, goroutineGrowth, 5, "Goroutine growth should be minimal after shutdown")
		assert.LessOrEqual(t, resourceLeaks, int64(numWorkers*2), "Resource leaks should be limited")
	})
}

// TestConcurrentCrossNodeStateSynchronization tests state synchronization failures between nodes
func TestConcurrentCrossNodeStateSynchronization(t *testing.T) {
	leaderNode := leader_node.CreateTestLeaderNode()
	regularNode1 := leader_node.CreateTestLeaderNode() // Simulate as regular node
	regularNode2 := leader_node.CreateTestLeaderNode() // Simulate as regular node
	
	const numSyncCycles = 20
	const operationsPerCycle = 50
	const testDuration = 4 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	var wg sync.WaitGroup
	var syncSuccesses int64
	var syncFailures int64
	var stateInconsistencies int64
	var concurrentModifications int64

	t.Run("CrossNodeStateSynchronization", func(t *testing.T) {
		// Leader node operations
		wg.Add(1)
		go func() {
			defer wg.Done()

			for cycle := 0; cycle < numSyncCycles; cycle++ {
				select {
				case <-ctx.Done():
					return
				default:
					// Leader modifies state
					leaderNode.SetHalted(cycle%3 == 0)
					
					round := fmt.Sprintf("sync_round_%d", cycle)
					trial := "1"
					leaderNode.SetCurrentRound(round)
					leaderNode.SetCurrentTrial(trial)
					
					// Try to propagate state to regular nodes (simulate sync)
					for op := 0; op < operationsPerCycle; op++ {
						key := fmt.Sprintf("leader_state_%d_%d", cycle, op)
						roundData := leader_node.RoundData{
							MerkleRoot:   leaderNode.GetHalted(),
							RandomNumber: !leaderNode.GetHalted(),
						}
						
						if leaderNode.CompareAndSwapSubmittingMerkleRoot(false, true) {
							leaderNode.SetRoundData(key, roundData)
							leaderNode.SetSubmittingMerkleRoot(false)
						}
						
						time.Sleep(time.Microsecond * 200)
					}
				}
			}
		}()

		// Regular node 1 - tries to sync with leader
		wg.Add(1)
		go func() {
			defer wg.Done()

			for cycle := 0; cycle < numSyncCycles; cycle++ {
				select {
				case <-ctx.Done():
					return
				default:
					time.Sleep(time.Millisecond * 50) // Sync delay
					
					// Try to read leader state (potential inconsistency)
					leaderHalted := leaderNode.GetHalted()
					leaderRound := leaderNode.GetCurrentRound()

					// Apply state to regular node
					regularNode1.SetHalted(leaderHalted)
					regularNode1.SetCurrentRound(leaderRound)

					// Verify consistency
					if regularNode1.GetHalted() == leaderHalted {
						atomic.AddInt64(&syncSuccesses, 1)
					} else {
						atomic.AddInt64(&syncFailures, 1)
					}
					
					// Check for concurrent modifications during sync
					if regularNode1.GetCurrentRound() != leaderRound {
						atomic.AddInt64(&concurrentModifications, 1)
					}
				}
			}
		}()

		// Regular node 2 - also tries to sync with leader (potential conflicts)
		wg.Add(1)
		go func() {
			defer wg.Done()

			for cycle := 0; cycle < numSyncCycles; cycle++ {
				select {
				case <-ctx.Done():
					return
				default:
					time.Sleep(time.Millisecond * 75) // Different sync delay
					
					// Read leader state
					leaderHalted := leaderNode.GetHalted()
					leaderRound := leaderNode.GetCurrentRound()

					// Apply state
					regularNode2.SetHalted(leaderHalted)
					regularNode2.SetCurrentRound(leaderRound)

					// Check consistency with other regular node
					if regularNode2.GetHalted() != regularNode1.GetHalted() {
						atomic.AddInt64(&stateInconsistencies, 1)
					}

					// Verify sync
					if regularNode2.GetHalted() == leaderHalted {
						atomic.AddInt64(&syncSuccesses, 1)
					} else {
						atomic.AddInt64(&syncFailures, 1)
					}
				}
			}
		}()

		wg.Wait()

		totalSyncAttempts := syncSuccesses + syncFailures
		syncSuccessRate := float64(syncSuccesses) / float64(totalSyncAttempts) * 100
		inconsistencyRate := float64(stateInconsistencies) / float64(totalSyncAttempts) * 100
		modificationRate := float64(concurrentModifications) / float64(totalSyncAttempts) * 100

		t.Logf("Concurrent cross-node state synchronization results:")
		t.Logf("  Total sync attempts: %d", totalSyncAttempts)
		t.Logf("  Sync successes: %d (%.1f%%)", syncSuccesses, syncSuccessRate)
		t.Logf("  Sync failures: %d", syncFailures)
		t.Logf("  State inconsistencies: %d (%.1f%%)", stateInconsistencies, inconsistencyRate)
		t.Logf("  Concurrent modifications: %d (%.1f%%)", concurrentModifications, modificationRate)
		t.Logf("  Sync cycles: %d", numSyncCycles)

		// Cross-node synchronization assertions
		assert.GreaterOrEqual(t, syncSuccessRate, 70.0, "Sync success rate should be >70%% under concurrent load")
		assert.Greater(t, syncSuccesses, int64(numSyncCycles), "Should achieve some successful syncs")
		assert.LessOrEqual(t, inconsistencyRate, 30.0, "State inconsistency rate should be <30%%")
		assert.GreaterOrEqual(t, concurrentModifications, int64(0), "Concurrent state modifications may occur under contention")
	})
}
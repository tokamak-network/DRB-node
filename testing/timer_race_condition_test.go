package testing

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
	"github.com/tokamak-network/DRB-node/utils"
)

// TimerRaceTracker tracks timer-related race conditions and timing issues
type TimerRaceTracker struct {
	timerStarts     int64
	timerStops      int64
	timerExpirations int64
	raceConditions   int64
}

// IncrementStarts atomically increments timer start count
func (t *TimerRaceTracker) IncrementStarts() {
	atomic.AddInt64(&t.timerStarts, 1)
}

// IncrementStops atomically increments timer stop count
func (t *TimerRaceTracker) IncrementStops() {
	atomic.AddInt64(&t.timerStops, 1)
}

// IncrementExpirations atomically increments timer expiration count
func (t *TimerRaceTracker) IncrementExpirations() {
	atomic.AddInt64(&t.timerExpirations, 1)
}

// IncrementRaceConditions atomically increments race condition count
func (t *TimerRaceTracker) IncrementRaceConditions() {
	atomic.AddInt64(&t.raceConditions, 1)
}

// GetStats returns current statistics
func (t *TimerRaceTracker) GetStats() (starts, stops, expirations, races int64) {
	return atomic.LoadInt64(&t.timerStarts),
		atomic.LoadInt64(&t.timerStops),
		atomic.LoadInt64(&t.timerExpirations),
		atomic.LoadInt64(&t.raceConditions)
}

// TestTimerStartStopRaceConditions tests race conditions in timer start/stop operations
func TestTimerStartStopRaceConditions(t *testing.T) {
	tests := []struct {
		name          string
		numWorkers    int
		operationsEach int
		timeoutDuration time.Duration
	}{
		{"Light Load", 5, 50, time.Second * 2},
		{"Medium Load", 10, 100, time.Second * 3},
		{"Heavy Load", 20, 200, time.Second * 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := createTestLeaderNodeForTimer()
			tracker := &TimerRaceTracker{}
			
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeoutDuration)
			defer cancel()
			
			var wg sync.WaitGroup
			
			// Test concurrent timer operations
			for i := 0; i < tt.numWorkers; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					
					for j := 0; j < tt.operationsEach; j++ {
						round := fmt.Sprintf("timer_race_round_%d_%d", workerID, j)
						trial := "1"
						timestamp := big.NewInt(time.Now().Unix())
						
						// Test rapid start/stop cycles that could create race conditions
						
						// Start monitoring
						tracker.IncrementStarts()
						
						// Simulate requestToSubmitCv monitoring
						if j%2 == 0 {
							// Test scenario: rapid start/stop
							startMonitoring(node, ctx, round, trial, timestamp, tracker)
							
							// Immediate stop (race condition potential)
							stopRequestToSubmitCvMonitoring(node, tracker)
						}
						
						// Simulate requestToSubmitCo monitoring  
						if j%3 == 0 {
							startFailToSubmitCoMonitoring(node, ctx, round, trial, timestamp, tracker)
							
							// Stop after short delay
							time.Sleep(time.Millisecond * 10)
							stopFailToSubmitCoMonitoring(node, tracker)
						}
						
						// Small delay to allow timer operations to process
						time.Sleep(time.Microsecond * 100)
					}
				}(i)
			}
			
			wg.Wait()
			
			// Verify timer statistics
			starts, stops, expirations, races := tracker.GetStats()
			t.Logf("Timer stats - Starts: %d, Stops: %d, Expirations: %d, Races: %d", 
				starts, stops, expirations, races)
			
			// Check for race conditions
			assert.Equal(t, int64(0), races, "Race conditions detected in timer operations")
			
			// Verify that stops don't exceed starts (sanity check)
			assert.LessOrEqual(t, stops, starts, "More stops than starts detected")
		})
	}
}

// TestConcurrentTimerModifications tests concurrent modifications to timer state
func TestConcurrentTimerModifications(t *testing.T) {
	node := createTestLeaderNodeForTimer()
	tracker := &TimerRaceTracker{}
	
	const numWorkers = 15
	const duration = 5 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	
	var wg sync.WaitGroup
	var timerConflicts int64
	
	// Worker 1: Continuously start/stop requestToSubmitCv monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
				round := fmt.Sprintf("cv_timer_%d", i)
				trial := "1"
				timestamp := big.NewInt(time.Now().Unix())
				
				// Check for existing active monitoring
				if node.GetRequestToSubmitCvMonitoringActive() {
					atomic.AddInt64(&timerConflicts, 1)
				}
				
				startMonitoring(node, ctx, round, trial, timestamp, tracker)
				time.Sleep(time.Millisecond * 50)
				stopRequestToSubmitCvMonitoring(node, tracker)
				
				i++
				time.Sleep(time.Millisecond * 20)
			}
		}
	}()
	
	// Worker 2: Continuously start/stop requestToSubmitCo monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
				round := fmt.Sprintf("co_timer_%d", i)
				trial := "1"
				timestamp := big.NewInt(time.Now().Unix())
				
				if node.GetRequestedToSubmitCoMonitoringActive() {
					atomic.AddInt64(&timerConflicts, 1)
				}
				
				startFailToSubmitCoMonitoring(node, ctx, round, trial, timestamp, tracker)
				time.Sleep(time.Millisecond * 75)
				stopFailToSubmitCoMonitoring(node, tracker)
				
				i++
				time.Sleep(time.Millisecond * 30)
			}
		}
	}()
	
	// Worker 3: Mixed atomic state changes
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// Rapidly change atomic states that affect timers
				node.SetExecution(i%2 == 0)
				node.SetHalted(i%3 == 0)
				
				// These state changes could affect timer behavior
				node.SetRequestToSubmitCvMonitoringActive(i%4 == 0)
				node.SetRequestedToSubmitCoMonitoringActive(i%5 == 0)
				
				i++
				time.Sleep(time.Microsecond * 500)
			}
		}
	}()
	
	wg.Wait()
	
	// Report statistics
	starts, stops, expirations, races := tracker.GetStats()
	conflicts := atomic.LoadInt64(&timerConflicts)
	
	t.Logf("Concurrent timer stats - Starts: %d, Stops: %d, Expirations: %d, Races: %d, Conflicts: %d", 
		starts, stops, expirations, races, conflicts)
	
	// Validate no race conditions detected
	assert.Equal(t, int64(0), races, "Race conditions detected in concurrent timer modifications")
	
	// Allow for some conflicts due to the nature of concurrent testing
	assert.LessOrEqual(t, conflicts, starts/10, "Excessive timer conflicts detected")
}

// TestTimerExpirationRaceConditions tests race conditions during timer expiration
func TestTimerExpirationRaceConditions(t *testing.T) {
	node := createTestLeaderNodeForTimer()
	tracker := &TimerRaceTracker{}
	
	const numTimers = 50
	const timerDuration = 100 * time.Millisecond
	
	var wg sync.WaitGroup
	
	// Create multiple timers with short expiration times
	for i := 0; i < numTimers; i++ {
		wg.Add(1)
		go func(timerID int) {
			defer wg.Done()
			
			round := fmt.Sprintf("expiration_timer_%d", timerID)
			trial := "1"
			timestamp := big.NewInt(time.Now().Unix())
			
			// Start timer
			tracker.IncrementStarts()
			
			// Create context that expires quickly
			ctx, cancel := context.WithTimeout(context.Background(), timerDuration)
			defer cancel()
			
			startMonitoring(node, ctx, round, trial, timestamp, tracker)
			
			// Wait for expiration or manual stop
			stopTime := time.Now().Add(timerDuration / 2)
			
			// 50% chance of manual stop before expiration
			if timerID%2 == 0 {
				time.Sleep(timerDuration / 3)
				stopRequestToSubmitCvMonitoring(node, tracker)
			} else {
				// Let it expire naturally
				<-ctx.Done()
				tracker.IncrementExpirations()
			}
			
			// Verify timer state consistency after expiration/stop
			if node.GetRequestToSubmitCvMonitoringActive() {
				// Timer should be inactive after stop/expiration
				tracker.IncrementRaceConditions()
				t.Errorf("Timer %d still active after expiration/stop", timerID)
			}
			
			_ = stopTime
		}(i)
	}
	
	wg.Wait()
	
	starts, stops, expirations, races := tracker.GetStats()
	t.Logf("Timer expiration stats - Starts: %d, Stops: %d, Expirations: %d, Races: %d", 
		starts, stops, expirations, races)
	
	assert.Equal(t, int64(0), races, "Race conditions detected in timer expiration handling")
	assert.Equal(t, int64(numTimers), starts, "Incorrect number of timer starts")
}

// TestPhaseTransitionTimerRaces tests race conditions during DRB phase transitions
func TestPhaseTransitionTimerRaces(t *testing.T) {
	node := createTestLeaderNodeForTimer()
	
	const numPhaseTransitions = 30
	const transitionDelay = 50 * time.Millisecond
	
	var wg sync.WaitGroup
	var phaseErrors int64
	
	// Simulate DRB phase transitions with timers
	for i := 0; i < numPhaseTransitions; i++ {
		wg.Add(1)
		go func(phaseID int) {
			defer wg.Done()
			
			round := fmt.Sprintf("phase_round_%d", phaseID)
			trial := "1"
			timestamp := big.NewInt(time.Now().Unix())
			ctx := context.Background()
			
			// Phase 1: CVS submission phase
			node.SetExecution(true)
			node.SetHalted(false)
			
			// Start CVS monitoring
			startMonitoring(node, ctx, round, trial, timestamp, &TimerRaceTracker{})
			
			time.Sleep(transitionDelay)
			
			// Phase 2: Transition to COS submission
			// Check for race condition: stopping CV while starting CO
			wasActive := node.GetRequestToSubmitCvMonitoringActive()
			stopRequestToSubmitCvMonitoring(node, &TimerRaceTracker{})
			
			// Immediately start CO monitoring (potential race)
			startFailToSubmitCoMonitoring(node, ctx, round, trial, timestamp, &TimerRaceTracker{})
			
			if wasActive && node.GetRequestedToSubmitCoMonitoringActive() {
				// Successful transition
			} else if !wasActive {
				// Previous timer might have expired naturally
			} else {
				atomic.AddInt64(&phaseErrors, 1)
			}
			
			time.Sleep(transitionDelay)
			
			// Phase 3: Complete round
			stopFailToSubmitCoMonitoring(node, &TimerRaceTracker{})
			
			// Phase 4: Reset for next round
			node.SetExecution(false)
			node.SetHalted(true)
			
			time.Sleep(transitionDelay / 2)
		}(i)
	}
	
	wg.Wait()
	
	errors := atomic.LoadInt64(&phaseErrors)
	t.Logf("Phase transition errors: %d out of %d transitions", errors, numPhaseTransitions)
	
	// Allow for minimal errors due to timing variations
	assert.LessOrEqual(t, errors, int64(numPhaseTransitions/20), 
		"Excessive phase transition errors detected (more than 5%%)")
}

// TestTimerMemoryLeaks tests for memory leaks in timer operations
func TestTimerMemoryLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}
	
	node := createTestLeaderNodeForTimer()
	
	// Baseline memory measurement
	initialGoroutines := runtime.NumGoroutine()
	
	const numCycles = 100
	const timersPerCycle = 10
	
	for cycle := 0; cycle < numCycles; cycle++ {
		// Create multiple timers in each cycle
		for i := 0; i < timersPerCycle; i++ {
			round := fmt.Sprintf("leak_test_%d_%d", cycle, i)
			trial := "1"
			timestamp := big.NewInt(time.Now().Unix())
			ctx := context.Background()
			
			// Start and immediately stop (stress test for cleanup)
			startMonitoring(node, ctx, round, trial, timestamp, &TimerRaceTracker{})
			stopRequestToSubmitCvMonitoring(node, &TimerRaceTracker{})
			
			startFailToSubmitCoMonitoring(node, ctx, round, trial, timestamp, &TimerRaceTracker{})
			stopFailToSubmitCoMonitoring(node, &TimerRaceTracker{})
		}
		
		// Periodic memory check
		if cycle%20 == 0 {
			runtime.GC()
			currentGoroutines := runtime.NumGoroutine()
			
			if currentGoroutines > initialGoroutines+10 {
				t.Logf("Potential memory leak at cycle %d: initial=%d, current=%d", 
					cycle, initialGoroutines, currentGoroutines)
			}
		}
	}
	
	// Final memory check
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()
	
	t.Logf("Timer memory test - Initial: %d, Final: %d goroutines", 
		initialGoroutines, finalGoroutines)
	
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+5, 
		"Potential timer memory leak detected")
}

// Helper functions for timer operations

func createTestLeaderNodeForTimer() *leader_node.LeaderNode {
	node := &leader_node.LeaderNode{}
	
	// Initialize required state for timer testing
	node.SetExecution(false)
	node.SetHalted(false)
	node.SetRequestToSubmitCvMonitoringActive(false)
	node.SetRequestedToSubmitCoMonitoringActive(false)
	
	return node
}

func startMonitoring(node *leader_node.LeaderNode, ctx context.Context, round, trial string, 
	timestamp *big.Int, tracker *TimerRaceTracker) {
	
	// Simulate startRequestToSubmitCvMonitoring
	if !node.GetRequestToSubmitCvMonitoringActive() {
		node.SetRequestToSubmitCvMonitoringActive(true)
		tracker.IncrementStarts()
		
		// Simulate timer logic (simplified for testing)
		go func() {
			select {
			case <-ctx.Done():
				// Timer expired
				if node.GetRequestToSubmitCvMonitoringActive() {
					node.SetRequestToSubmitCvMonitoringActive(false)
					tracker.IncrementExpirations()
				}
			case <-time.After(time.Second * 1): // Short timeout for testing
				// Normal expiration
				if node.GetRequestToSubmitCvMonitoringActive() {
					node.SetRequestToSubmitCvMonitoringActive(false)
					tracker.IncrementExpirations()
				}
			}
		}()
	} else {
		tracker.IncrementRaceConditions()
	}
}

func stopRequestToSubmitCvMonitoring(node *leader_node.LeaderNode, tracker *TimerRaceTracker) {
	if node.GetRequestToSubmitCvMonitoringActive() {
		node.SetRequestToSubmitCvMonitoringActive(false)
		tracker.IncrementStops()
	}
}

func startFailToSubmitCoMonitoring(node *leader_node.LeaderNode, ctx context.Context, 
	round, trial string, timestamp *big.Int, tracker *TimerRaceTracker) {
	
	if !node.GetRequestedToSubmitCoMonitoringActive() {
		node.SetRequestedToSubmitCoMonitoringActive(true)
		tracker.IncrementStarts()
		
		// Simulate timer logic
		go func() {
			select {
			case <-ctx.Done():
				if node.GetRequestedToSubmitCoMonitoringActive() {
					node.SetRequestedToSubmitCoMonitoringActive(false)
					tracker.IncrementExpirations()
				}
			case <-time.After(time.Millisecond * 800):
				if node.GetRequestedToSubmitCoMonitoringActive() {
					node.SetRequestedToSubmitCoMonitoringActive(false)
					tracker.IncrementExpirations()
				}
			}
		}()
	} else {
		tracker.IncrementRaceConditions()
	}
}

func stopFailToSubmitCoMonitoring(node *leader_node.LeaderNode, tracker *TimerRaceTracker) {
	if node.GetRequestedToSubmitCoMonitoringActive() {
		node.SetRequestedToSubmitCoMonitoringActive(false)
		tracker.IncrementStops()
	}
}

// Benchmark tests for timer performance under load

func BenchmarkTimerOperations(b *testing.B) {
	node := createTestLeaderNodeForTimer()
	tracker := &TimerRaceTracker{}
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		round := fmt.Sprintf("bench_round_%d", i)
		trial := "1"
		timestamp := big.NewInt(time.Now().Unix())
		
		startMonitoring(node, ctx, round, trial, timestamp, tracker)
		stopRequestToSubmitCvMonitoring(node, tracker)
	}
	
	b.StopTimer()
	
	starts, stops, _, races := tracker.GetStats()
	if races > 0 {
		b.Errorf("Race conditions detected in benchmark: %d", races)
	}
	b.Logf("Benchmark stats - Starts: %d, Stops: %d, Races: %d", starts, stops, races)
}

func BenchmarkConcurrentTimerOperations(b *testing.B) {
	node := createTestLeaderNodeForTimer()
	tracker := &TimerRaceTracker{}
	ctx := context.Background()
	
	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			round := fmt.Sprintf("concurrent_bench_%d", i)
			trial := "1"
			timestamp := big.NewInt(time.Now().Unix())
			
			startMonitoring(node, ctx, round, trial, timestamp, tracker)
			time.Sleep(time.Microsecond)
			stopRequestToSubmitCvMonitoring(node, tracker)
			
			i++
		}
	})
	
	b.StopTimer()
	
	starts, stops, _, races := tracker.GetStats()
	if races > 0 {
		b.Errorf("Race conditions detected in concurrent benchmark: %d", races)
	}
	b.Logf("Concurrent benchmark stats - Starts: %d, Stops: %d, Races: %d", starts, stops, races)
}
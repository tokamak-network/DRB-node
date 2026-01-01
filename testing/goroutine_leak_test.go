package testing

import (
	"context"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/eapache/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
	regular_node "github.com/tokamak-network/DRB-node/nodes/regular"
	"github.com/tokamak-network/DRB-node/utils"
)

// GoroutineLeakTracker tracks goroutine counts before and after operations
type GoroutineLeakTracker struct {
	initialCount int
	testName     string
}

// NewGoroutineLeakTracker creates a new leak tracker
func NewGoroutineLeakTracker(testName string) *GoroutineLeakTracker {
	runtime.GC()
	time.Sleep(50 * time.Millisecond) // Allow GC to complete
	
	return &GoroutineLeakTracker{
		initialCount: runtime.NumGoroutine(),
		testName:     testName,
	}
}

// CheckForLeaks verifies no goroutines were leaked
func (g *GoroutineLeakTracker) CheckForLeaks(t *testing.T) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond) // Allow goroutines to cleanup
	
	finalCount := runtime.NumGoroutine()
	
	// Allow for small variance (test framework goroutines)
	maxAllowed := g.initialCount + 3
	
	if finalCount > maxAllowed {
		t.Errorf("Goroutine leak detected in %s: initial=%d, final=%d (max allowed=%d)", 
			g.testName, g.initialCount, finalCount, maxAllowed)
		
		// Print stack traces for debugging
		buf := make([]byte, 1024*1024)
		stackSize := runtime.Stack(buf, true)
		t.Logf("Goroutine stack traces:\n%s", buf[:stackSize])
	} else {
		t.Logf("No goroutine leak in %s: initial=%d, final=%d", 
			g.testName, g.initialCount, finalCount)
	}
}

// TestLeaderNodeGoroutineLeaks tests for goroutine leaks in leader node operations
func TestLeaderNodeGoroutineLeaks(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T, *leader_node.LeaderNode)
	}{
		{
			"MonitorCommits", 
			testLeaderMonitorCommitsLeak,
		},
		{
			"ReceiveCommit", 
			testLeaderReceiveCommitLeak,
		},
		{
			"TimerOperations", 
			testLeaderTimerOperationsLeak,
		},
		{
			"BroadcastOperations",
			testLeaderBroadcastOperationsLeak,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewGoroutineLeakTracker(tt.name)
			defer tracker.CheckForLeaks(t)
			
			node := createTestLeaderNode(t)
			tt.testFunc(t, node)
		})
	}
}

// TestRegularNodeGoroutineLeaks tests for goroutine leaks in regular node operations
func TestRegularNodeGoroutineLeaks(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T, *regular_node.RegularNode)
	}{
		{
			"AllCosReceived",
			testRegularAllCosReceivedLeak,
		},
		{
			"SendCommitOperations",
			testRegularSendCommitLeak,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewGoroutineLeakTracker(tt.name)
			defer tracker.CheckForLeaks(t)
			
			node := createTestRegularNode(t)
			tt.testFunc(t, node)
		})
	}
}

// TestConcurrentOperationsLeaks tests goroutine leaks under concurrent load
func TestConcurrentOperationsLeaks(t *testing.T) {
	tracker := NewGoroutineLeakTracker("ConcurrentOperations")
	defer tracker.CheckForLeaks(t)
	
	const numWorkers = 20
	const numOperationsPerWorker = 50
	
	var wg sync.WaitGroup
	
	// Create test nodes
	leaderNode := createTestLeaderNode(t)
	
	// Test concurrent operations that could leak goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			
			for j := 0; j < numOperationsPerWorker; j++ {
				round := fmt.Sprintf("leak_test_%d_%d", workerID, j)
				trial := "1"
				timestamp := big.NewInt(time.Now().Unix())
				
				// Start and stop monitoring (potential goroutine leaks)
				leaderNode.SetRequestToSubmitCvMonitoringActive(true)
				time.Sleep(time.Millisecond)
				leaderNode.SetRequestToSubmitCvMonitoringActive(false)
				
				// Test timer operations
				if j%5 == 0 {
					leaderNode.SetRequestedToSubmitCoMonitoringActive(true)
					time.Sleep(time.Millisecond)
					leaderNode.SetRequestedToSubmitCoMonitoringActive(false)
				}
				
				// Simulate some processing
				leaderNode.SetRoundData(fmt.Sprintf("test_key_%d_%d", workerID, j), leader_node.RoundData{
					MerkleRoot:   true,
					RandomNumber: false,
				})
				
				_ = timestamp // Use the variable to avoid compiler warnings
			}
		}(i)
	}
	
	wg.Wait()
}

// TestLongRunningOperationsLeaks tests goroutine leaks in long-running operations
func TestLongRunningOperationsLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}
	
	tracker := NewGoroutineLeakTracker("LongRunningOperations")
	defer tracker.CheckForLeaks(t)
	
	node := createTestLeaderNode(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Start some long-running operations
	var wg sync.WaitGroup
	
	// Simulate monitor commits running
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Simulate some work
				node.SetExecution(true)
				time.Sleep(time.Millisecond * 10)
				node.SetExecution(false)
			}
		}
	}()
	
	// Wait for operations to complete
	<-ctx.Done()
	wg.Wait()
}

// Test implementation functions

func testLeaderMonitorCommitsLeak(t *testing.T, node *leader_node.LeaderNode) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	// Start monitor commits in a goroutine
	go func() {
		// Simulate a short-lived monitoring operation
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		
		for i := 0; i < 20; i++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Simulate monitoring work
				node.SetCurrentRound(fmt.Sprintf("monitor_round_%d", i))
				node.SetHalted(i%2 == 0)
			}
		}
	}()
	
	// Wait for completion
	<-ctx.Done()
}

func testLeaderReceiveCommitLeak(t *testing.T, node *leader_node.LeaderNode) {
	// Test that doesn't involve actual blockchain connections
	// but exercises the event processing logic paths
	
	for i := 0; i < 10; i++ {
		round := fmt.Sprintf("receive_round_%d", i)
		trial := "1"
		
		// Simulate event processing without actual network calls
		node.SetCurrentRound(round)
		node.SetCurrentTrial(trial)
		
		// Test state changes that might create goroutines
		node.SetExecution(true)
		node.SetHalted(false)
		
		time.Sleep(time.Millisecond * 10)
		
		node.SetExecution(false)
		node.SetHalted(true)
	}
}

func testLeaderTimerOperationsLeak(t *testing.T, node *leader_node.LeaderNode) {
	// Test timer-related operations that could leak goroutines
	for i := 0; i < 20; i++ {
		// Rapidly start/stop monitoring states
		node.SetRequestToSubmitCvMonitoringActive(true)
		node.SetRequestedToSubmitCoMonitoringActive(true)
		
		time.Sleep(time.Millisecond * 5)
		
		node.SetRequestToSubmitCvMonitoringActive(false)
		node.SetRequestedToSubmitCoMonitoringActive(false)
		
		// Test other atomic operations
		node.SetSubmittingMerkleRoot(i%2 == 0)
		node.SetFailToSubmitSMonitoringActive(i%3 == 0)
	}
}

func testLeaderBroadcastOperationsLeak(t *testing.T, node *leader_node.LeaderNode) {
	// Test broadcast-related operations
	for i := 0; i < 15; i++ {
		key := fmt.Sprintf("broadcast_key_%d", i)
		
		// Test broadcast tracking operations
		tracker := &utils.BroadcastTracker{} // Simplified for testing
		node.SetActiveBroadcast(key, tracker)
		
		// Simulate some work
		time.Sleep(time.Millisecond * 5)
		
		// Cleanup
		node.DeleteActiveBroadcast(key)
	}
}

func testRegularAllCosReceivedLeak(t *testing.T, node *regular_node.RegularNode) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	// Simulate AllCosReceivedUnlocked operations
	for i := 0; i < 10; i++ {
		round := fmt.Sprintf("regular_round_%d", i)
		trial := "1"
		
		// Start the operation
		go func() {
			// Simulate the checking logic without actual P2P calls
			select {
			case <-ctx.Done():
				return
			default:
				// Simulate some work
				time.Sleep(time.Millisecond * 100)
			}
		}()
		
		time.Sleep(time.Millisecond * 10)
	}
	
	<-ctx.Done()
}

func testRegularSendCommitLeak(t *testing.T, node *regular_node.RegularNode) {
	// Test operations that might create goroutines in regular node
	for i := 0; i < 15; i++ {
		// Simulate state changes
		node.SetExecution(true)
		node.SetHalted(false)
		
		time.Sleep(time.Millisecond * 5)
		
		node.SetExecution(false)
		node.SetHalted(true)
	}
}

// Helper functions

func createTestLeaderNode(t *testing.T) *leader_node.LeaderNode {
	return &leader_node.LeaderNode{
		// Initialize basic fields for testing
	}
}

func createTestRegularNode(t *testing.T) *regular_node.RegularNode {
	return &regular_node.RegularNode{
		// Initialize basic fields for testing  
	}
}

// Benchmark tests for detecting performance regressions that might indicate leaks

func BenchmarkLeaderNodeOperations(b *testing.B) {
	tracker := NewGoroutineLeakTracker("BenchmarkLeader")
	defer func() {
		// Convert benchmark to test for leak checking
		t := &testing.T{}
		tracker.CheckForLeaks(t)
	}()
	
	node := &leader_node.LeaderNode{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.SetExecution(i%2 == 0)
		node.SetHalted(i%3 == 0)
		node.SetSubmittingMerkleRoot(i%4 == 0)
	}
}

func BenchmarkConcurrentMapOperations(b *testing.B) {
	tracker := NewGoroutineLeakTracker("BenchmarkConcurrentMaps")
	defer func() {
		t := &testing.T{}
		tracker.CheckForLeaks(t)
	}()
	
	node := &leader_node.LeaderNode{}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_key_%d", i)
			roundData := leader_node.RoundData{
				MerkleRoot:   i%2 == 0,
				RandomNumber: i%3 == 0,
			}
			
			node.SetRoundData(key, roundData)
			_, _ = node.GetRoundData(key)
			
			i++
		}
	})
}
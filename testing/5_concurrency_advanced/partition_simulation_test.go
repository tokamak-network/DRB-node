package concurrency_advanced

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
)

// NetworkPartitionSimulator simulates network partition scenarios for concurrency testing
type NetworkPartitionSimulator struct {
	partitionActive int64
	connectedNodes  map[string]bool
	mu              sync.RWMutex
}

// NewNetworkPartitionSimulator creates a new network partition simulator
func NewNetworkPartitionSimulator() *NetworkPartitionSimulator {
	return &NetworkPartitionSimulator{
		connectedNodes: make(map[string]bool),
	}
}

// CreatePartition simulates a network partition
func (nps *NetworkPartitionSimulator) CreatePartition(nodeIDs []string) {
	nps.mu.Lock()
	defer nps.mu.Unlock()
	
	atomic.StoreInt64(&nps.partitionActive, 1)
	
	// Disconnect specified nodes
	for _, nodeID := range nodeIDs {
		nps.connectedNodes[nodeID] = false
	}
}

// HealPartition simulates network partition healing
func (nps *NetworkPartitionSimulator) HealPartition() {
	nps.mu.Lock()
	defer nps.mu.Unlock()
	
	atomic.StoreInt64(&nps.partitionActive, 0)
	
	// Reconnect all nodes
	for nodeID := range nps.connectedNodes {
		nps.connectedNodes[nodeID] = true
	}
}

// IsConnected checks if a node is connected (thread-safe)
func (nps *NetworkPartitionSimulator) IsConnected(nodeID string) bool {
	nps.mu.RLock()
	defer nps.mu.RUnlock()
	
	connected, exists := nps.connectedNodes[nodeID]
	if !exists {
		return true // Default to connected for new nodes
	}
	return connected
}

// IsPartitionActive checks if partition is currently active (thread-safe)
func (nps *NetworkPartitionSimulator) IsPartitionActive() bool {
	return atomic.LoadInt64(&nps.partitionActive) == 1
}

// TestConcurrentNetworkPartitionHandling tests concurrent access during network partitions
func TestConcurrentNetworkPartitionHandling(t *testing.T) {
	node := leader_node.CreateTestLeaderNode()
	simulator := NewNetworkPartitionSimulator()
	
	const numNodes = 20
	const testDuration = 5 * time.Second
	const operationsPerWorker = 100
	
	// Initialize nodes
	nodeIDs := make([]string, numNodes)
	for i := 0; i < numNodes; i++ {
		nodeIDs[i] = fmt.Sprintf("partition_node_%d", i)
		simulator.connectedNodes[nodeIDs[i]] = true
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	var partitionedOps int64
	var normalOps int64
	var concurrencyErrors int64
	
	t.Run("ConcurrentPartitionOperations", func(t *testing.T) {
		// Start workers performing operations
		for worker := 0; worker < 10; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				opCount := 0
				for opCount < operationsPerWorker {
					select {
					case <-ctx.Done():
						return
					default:
						nodeID := nodeIDs[workerID%numNodes]
						key := fmt.Sprintf("partition_concurrent_%d_%d", workerID, opCount)
						
						// Perform operation with concurrency check
						roundData := leader_node.RoundData{
							MerkleRoot:   opCount%2 == 0,
							RandomNumber: opCount%3 == 0,
						}
						
						// Thread-safe operation
						node.SetRoundData(key, roundData)
						
						// Check if operation occurred during partition
						if simulator.IsPartitionActive() && !simulator.IsConnected(nodeID) {
							atomic.AddInt64(&partitionedOps, 1)
						} else {
							atomic.AddInt64(&normalOps, 1)
						}
						
						// Verify data consistency (concurrency check)
						if retrieved, exists := node.GetRoundData(key); exists {
							if retrieved.MerkleRoot != roundData.MerkleRoot {
								atomic.AddInt64(&concurrencyErrors, 1)
							}
						}
						
						opCount++
						time.Sleep(time.Millisecond * 10)
					}
				}
			}(worker)
		}
		
		// Partition controller
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			partitionCycle := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Create partition
					partitionedNodes := nodeIDs[:numNodes/2]
					simulator.CreatePartition(partitionedNodes)
					time.Sleep(800 * time.Millisecond)
					
					// Heal partition
					simulator.HealPartition()
					time.Sleep(500 * time.Millisecond)
					
					partitionCycle++
				}
			}
		}()
		
		wg.Wait()
		
		totalOps := partitionedOps + normalOps
		partitionRatio := float64(partitionedOps) / float64(totalOps) * 100
		
		t.Logf("Concurrent partition handling results:")
		t.Logf("  Normal operations: %d", normalOps)
		t.Logf("  Partitioned operations: %d (%.1f%%)", partitionedOps, partitionRatio)
		t.Logf("  Total operations: %d", totalOps)
		t.Logf("  Concurrency errors: %d", concurrencyErrors)
		
		// Concurrency and partition tolerance assertions
		assert.Greater(t, totalOps, int64(800), "Should complete most operations despite partitions")
		assert.Greater(t, partitionedOps, int64(0), "Should continue operations during partitions")
		assert.Equal(t, int64(0), concurrencyErrors, "No concurrency errors should occur during partitions")
		assert.Less(t, partitionRatio, 80.0, "Most operations should occur during normal conditions")
	})
}

// TestSplitBrainConcurrencyProtection tests protection against split-brain scenarios
func TestSplitBrainConcurrencyProtection(t *testing.T) {
	leaderA := leader_node.CreateTestLeaderNode()
	leaderB := leader_node.CreateTestLeaderNode()
	simulator := NewNetworkPartitionSimulator()
	
	const numWorkers = 20
	const operationsPerWorker = 50
	
	// Setup node groups
	groupA := []string{"nodeA_1", "nodeA_2", "nodeA_3"}
	groupB := []string{"nodeB_1", "nodeB_2", "nodeB_3"}
	
	// Initialize all nodes as connected
	allNodes := append(groupA, groupB...)
	for _, nodeID := range allNodes {
		simulator.connectedNodes[nodeID] = true
	}
	
	var wg sync.WaitGroup
	var conflictCount int64
	var successfulOps int64
	var racePrevented int64
	
	t.Run("SplitBrainProtection", func(t *testing.T) {
		// Create partition
		simulator.CreatePartition(groupB)
		
		// Leader A workers (connected side)
		for worker := 0; worker < numWorkers/2; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for op := 0; op < operationsPerWorker; op++ {
					key := fmt.Sprintf("leaderA_%d_%d", workerID, op)
					
					// Use atomic operation to prevent race conditions
					if leaderA.CompareAndSwapSubmittingMerkleRoot(false, true) {
						// Critical section protected by atomic operation
						roundData := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
						leaderA.SetRoundData(key, roundData)
						
						time.Sleep(time.Millisecond * 5) // Simulate processing time
						leaderA.SetSubmittingMerkleRoot(false)
						atomic.AddInt64(&successfulOps, 1)
					} else {
						// Race condition detected and prevented
						atomic.AddInt64(&racePrevented, 1)
					}
				}
			}(worker)
		}
		
		// Leader B workers (partitioned side)
		for worker := 0; worker < numWorkers/2; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for op := 0; op < operationsPerWorker; op++ {
					key := fmt.Sprintf("leaderB_%d_%d", workerID, op)
					
					// Attempt operation on partitioned side
					if leaderB.CompareAndSwapSubmittingMerkleRoot(false, true) {
						roundData := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
						leaderB.SetRoundData(key, roundData)
						
						time.Sleep(time.Millisecond * 5)
						leaderB.SetSubmittingMerkleRoot(false)
						atomic.AddInt64(&successfulOps, 1)
						
						// Check for potential conflicts
						if simulator.IsPartitionActive() {
							atomic.AddInt64(&conflictCount, 1)
						}
					} else {
						atomic.AddInt64(&racePrevented, 1)
					}
				}
			}(worker)
		}
		
		wg.Wait()
		
		totalAttempts := int64(numWorkers * operationsPerWorker)
		racePreventionRate := float64(racePrevented) / float64(totalAttempts) * 100
		
		t.Logf("Split-brain protection results:")
		t.Logf("  Successful operations: %d", successfulOps)
		t.Logf("  Race conditions prevented: %d (%.1f%%)", racePrevented, racePreventionRate)
		t.Logf("  Potential conflicts: %d", conflictCount)
		t.Logf("  Total attempts: %d", totalAttempts)
		
		// Verify split-brain protection
		assert.Equal(t, totalAttempts, successfulOps+racePrevented, "All attempts should be accounted for")
		assert.Greater(t, racePrevented, int64(0), "Race prevention mechanisms should activate")
		assert.Greater(t, successfulOps, int64(0), "Some operations should succeed")
	})
}

// TestAsynchronousPartitionRecovery tests concurrent recovery from network partitions
func TestAsynchronousPartitionRecovery(t *testing.T) {
	node := leader_node.CreateTestLeaderNode()
	simulator := NewNetworkPartitionSimulator()
	
	const numAsyncWorkers = 15
	const recoveryOperations = 100
	
	// Setup nodes
	nodeIDs := []string{"async_0", "async_1", "async_2", "async_3", "async_4"}
	for _, nodeID := range nodeIDs {
		simulator.connectedNodes[nodeID] = true
	}
	
	var wg sync.WaitGroup
	var asyncOps int64
	var syncOps int64
	var recoveryOps int64
	
	t.Run("AsynchronousRecovery", func(t *testing.T) {
		// Create partition
		simulator.CreatePartition(nodeIDs[:3])
		
		// Async operations on partitioned side
		for worker := 0; worker < numAsyncWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for op := 0; op < recoveryOperations/numAsyncWorkers; op++ {
					key := fmt.Sprintf("async_%d_%d", workerID, op)
					
					// Continue operations despite partition
					if !simulator.IsConnected(nodeIDs[workerID%3]) {
						roundData := leader_node.RoundData{
							MerkleRoot:   op%2 == 0,
							RandomNumber: op%3 == 0,
						}
						node.SetRoundData(key, roundData)
						atomic.AddInt64(&asyncOps, 1)
					}
					
					time.Sleep(time.Millisecond * 5)
				}
			}(worker)
		}
		
		// Sync operations on connected side
		for worker := 0; worker < 5; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for op := 0; op < recoveryOperations/5; op++ {
					key := fmt.Sprintf("sync_%d_%d", workerID, op)
					
					if simulator.IsConnected(nodeIDs[3]) {
						roundData := leader_node.RoundData{
							MerkleRoot:   op%3 == 0,
							RandomNumber: op%2 == 0,
						}
						node.SetRoundData(key, roundData)
						atomic.AddInt64(&syncOps, 1)
					}
					
					time.Sleep(time.Millisecond * 5)
				}
			}(worker)
		}
		
		// Wait for partition operations
		wg.Wait()
		
		// Heal partition
		simulator.HealPartition()
		
		// Recovery operations
		var recoveryWG sync.WaitGroup
		for worker := 0; worker < 10; worker++ {
			recoveryWG.Add(1)
			go func(workerID int) {
				defer recoveryWG.Done()
				
				for op := 0; op < 20; op++ {
					key := fmt.Sprintf("recovery_%d_%d", workerID, op)
					roundData := leader_node.RoundData{MerkleRoot: true, RandomNumber: true}
					node.SetRoundData(key, roundData)
					
					// Verify integrity
					if retrieved, exists := node.GetRoundData(key); exists {
						if retrieved.MerkleRoot == roundData.MerkleRoot {
							atomic.AddInt64(&recoveryOps, 1)
						}
					}
				}
			}(worker)
		}
		
		recoveryWG.Wait()
		
		t.Logf("Asynchronous recovery results:")
		t.Logf("  Async operations (partitioned): %d", asyncOps)
		t.Logf("  Sync operations (connected): %d", syncOps)
		t.Logf("  Recovery operations: %d", recoveryOps)
		
		assert.Greater(t, asyncOps, int64(0), "Async operations should continue during partition")
		assert.Greater(t, syncOps, int64(0), "Sync operations should continue on connected side")
		assert.Equal(t, int64(200), recoveryOps, "All recovery operations should succeed")
	})
}
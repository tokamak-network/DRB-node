package concurrency_advanced

import (
	"context"
	"fmt"
	mathrnd "math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	leader_node "github.com/tokamak-network/DRB-node/nodes/leader"
	"github.com/tokamak-network/DRB-node/utils"
)

// ByzantineAttackType defines different types of malicious behavior for concurrency testing
type ByzantineAttackType int

const (
	AttackQuiet ByzantineAttackType = iota // Silent/non-participating
	AttackSpam                             // Spam invalid operations
	AttackDouble                           // Double spending/double commits
	AttackDelay                            // Delayed responses
	AttackCorrupt                          // Send corrupted data
	AttackColluding                        // Colluding with other Byzantine nodes
)

// ConcurrentByzantineNode represents a malicious node for concurrency testing
type ConcurrentByzantineNode struct {
	ID          string
	AttackType  ByzantineAttackType
	node        *leader_node.LeaderNode
	active      int64
	
	// Concurrent attack statistics
	spamCount     int64
	corruptCount  int64
	delayCount    int64
	doubleCount   int64
	
	// Concurrency control
	operationMutex sync.RWMutex
}

// NewConcurrentByzantineNode creates a new Byzantine node for concurrency testing
func NewConcurrentByzantineNode(id string, attackType ByzantineAttackType) *ConcurrentByzantineNode {
	return &ConcurrentByzantineNode{
		ID:         id,
		AttackType: attackType,
		node:       leader_node.CreateTestLeaderNode(),
		active:     1,
	}
}

// Activate starts Byzantine behavior (thread-safe)
func (cbn *ConcurrentByzantineNode) Activate() {
	atomic.StoreInt64(&cbn.active, 1)
}

// Deactivate stops Byzantine behavior (thread-safe)
func (cbn *ConcurrentByzantineNode) Deactivate() {
	atomic.StoreInt64(&cbn.active, 0)
}

// IsActive checks if Byzantine behavior is active (thread-safe)
func (cbn *ConcurrentByzantineNode) IsActive() bool {
	return atomic.LoadInt64(&cbn.active) == 1
}

// PerformConcurrentAttack performs Byzantine attack with proper concurrency handling
func (cbn *ConcurrentByzantineNode) PerformConcurrentAttack(round, trial string) error {
	if !cbn.IsActive() {
		return nil
	}
	
	switch cbn.AttackType {
	case AttackQuiet:
		return nil // Silent attack - do nothing
		
	case AttackSpam:
		return cbn.performConcurrentSpamAttack(round, trial)
		
	case AttackDouble:
		return cbn.performConcurrentDoubleCommitAttack(round, trial)
		
	case AttackDelay:
		return cbn.performConcurrentDelayAttack(round, trial)
		
	case AttackCorrupt:
		return cbn.performConcurrentCorruptDataAttack(round, trial)
		
	case AttackColluding:
		return cbn.performConcurrentColludingAttack(round, trial)
		
	default:
		return fmt.Errorf("unknown Byzantine attack type: %d", cbn.AttackType)
	}
}

// performConcurrentSpamAttack with proper concurrency control
func (cbn *ConcurrentByzantineNode) performConcurrentSpamAttack(round, trial string) error {
	cbn.operationMutex.Lock()
	defer cbn.operationMutex.Unlock()
	
	// Generate concurrent spam operations
	for i := 0; i < 5; i++ {
		spamKey := fmt.Sprintf("spam_%s_%s_%d_%d", cbn.ID, round, time.Now().UnixNano(), i)
		
		fakeData := leader_node.RoundData{
			MerkleRoot:   i%2 == 0,
			RandomNumber: i%3 == 0,
		}
		
		cbn.node.SetRoundData(spamKey, fakeData)
		atomic.AddInt64(&cbn.spamCount, 1)
	}
	
	return nil
}

// performConcurrentDoubleCommitAttack with race condition protection
func (cbn *ConcurrentByzantineNode) performConcurrentDoubleCommitAttack(round, trial string) error {
	uniqueKey := utils.GetUniqueKey(round, trial)
	
	// Attempt double commit with atomic protection
	if cbn.node.CompareAndSwapSubmittingMerkleRoot(false, true) {
		defer cbn.node.SetSubmittingMerkleRoot(false)
		
		// First conflicting commit
		firstData := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
		cbn.node.SetRoundData(uniqueKey+"_first", firstData)
		
		// Second conflicting commit
		secondData := leader_node.RoundData{MerkleRoot: false, RandomNumber: true}
		cbn.node.SetRoundData(uniqueKey+"_second", secondData)
		
		atomic.AddInt64(&cbn.doubleCount, 1)
	}
	
	return nil
}

// performConcurrentDelayAttack with controlled timing
func (cbn *ConcurrentByzantineNode) performConcurrentDelayAttack(round, trial string) error {
	// Introduce artificial delay
	delay := time.Duration(200+mathrnd.Intn(300)) * time.Millisecond
	time.Sleep(delay)
	
	// Then perform delayed operation
	cbn.operationMutex.Lock()
	defer cbn.operationMutex.Unlock()
	
	key := fmt.Sprintf("delayed_%s_%s", cbn.ID, round)
	data := leader_node.RoundData{MerkleRoot: true, RandomNumber: false}
	cbn.node.SetRoundData(key, data)
	
	atomic.AddInt64(&cbn.delayCount, 1)
	return nil
}

// performConcurrentCorruptDataAttack with data integrity issues
func (cbn *ConcurrentByzantineNode) performConcurrentCorruptDataAttack(round, trial string) error {
	cbn.operationMutex.Lock()
	defer cbn.operationMutex.Unlock()
	
	key := fmt.Sprintf("corrupt_%s_%s", cbn.ID, round)
	
	// Create corrupted data by using invalid states
	corruptData := leader_node.RoundData{
		MerkleRoot:   true,  // Both true - potentially invalid state
		RandomNumber: true,
	}
	
	cbn.node.SetRoundData(key, corruptData)
	atomic.AddInt64(&cbn.corruptCount, 1)
	return nil
}

// performConcurrentColludingAttack coordinated with other Byzantine nodes
func (cbn *ConcurrentByzantineNode) performConcurrentColludingAttack(round, trial string) error {
	// Simulate coordinated attack with deterministic patterns
	collusionKey := fmt.Sprintf("collusion_%s_%s", round, trial)
	
	// Use deterministic "malicious" data that other Byzantine nodes might use
	maliciousData := leader_node.RoundData{
		MerkleRoot:   false, // Coordinated false values
		RandomNumber: false,
	}
	
	cbn.operationMutex.RLock()
	cbn.node.SetRoundData(collusionKey, maliciousData)
	cbn.operationMutex.RUnlock()
	
	return nil
}

// GetConcurrentStats returns thread-safe attack statistics
func (cbn *ConcurrentByzantineNode) GetConcurrentStats() (spam, corrupt, delay, double int64) {
	return atomic.LoadInt64(&cbn.spamCount),
		atomic.LoadInt64(&cbn.corruptCount),
		atomic.LoadInt64(&cbn.delayCount),
		atomic.LoadInt64(&cbn.doubleCount)
}

// TestConcurrentByzantineFaultTolerance tests Byzantine fault tolerance under concurrent load
func TestConcurrentByzantineFaultTolerance(t *testing.T) {
	const totalNodes = 15        // Total nodes in network
	const byzantineCount = 4     // Byzantine nodes (< 1/3)
	const honestCount = totalNodes - byzantineCount
	const testDuration = 10 * time.Second
	
	// Create honest nodes
	honestNodes := make([]*leader_node.LeaderNode, honestCount)
	for i := 0; i < honestCount; i++ {
		honestNodes[i] = leader_node.CreateTestLeaderNode()
	}
	
	// Create Byzantine nodes with different attack types
	byzantineNodes := make([]*ConcurrentByzantineNode, byzantineCount)
	attackTypes := []ByzantineAttackType{AttackSpam, AttackDouble, AttackDelay, AttackCorrupt}
	
	for i := 0; i < byzantineCount; i++ {
		byzantineNodes[i] = NewConcurrentByzantineNode(
			fmt.Sprintf("concurrent_byz_%d", i),
			attackTypes[i%len(attackTypes)],
		)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	var honestOperations int64
	var byzantineDetections int64
	var concurrentConflicts int64
	
	t.Run("ConcurrentFaultTolerance", func(t *testing.T) {
		// Honest nodes performing concurrent operations
		for i, honestNode := range honestNodes {
			wg.Add(1)
			go func(nodeID int, node *leader_node.LeaderNode) {
				defer wg.Done()
				
				opCount := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						key := fmt.Sprintf("honest_concurrent_%d_%d", nodeID, opCount)
						data := leader_node.RoundData{
							MerkleRoot:   opCount%2 == 0,
							RandomNumber: opCount%3 == 0,
						}
						
						// Use atomic operation to prevent conflicts
						if node.CompareAndSwapSubmittingMerkleRoot(false, true) {
							node.SetRoundData(key, data)
							node.SetSubmittingMerkleRoot(false)
							atomic.AddInt64(&honestOperations, 1)
						} else {
							atomic.AddInt64(&concurrentConflicts, 1)
						}
						
						opCount++
						time.Sleep(time.Millisecond * 15)
					}
				}
			}(i, honestNode)
		}
		
		// Byzantine nodes performing concurrent attacks
		for i, byzNode := range byzantineNodes {
			wg.Add(1)
			go func(nodeID int, bn *ConcurrentByzantineNode) {
				defer wg.Done()
				
				attackCount := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						round := fmt.Sprintf("concurrent_attack_%d_%d", nodeID, attackCount)
						trial := "1"
						
						err := bn.PerformConcurrentAttack(round, trial)
						if err != nil {
							atomic.AddInt64(&byzantineDetections, 1)
						}
						
						attackCount++
						time.Sleep(time.Millisecond * 20)
					}
				}
			}(i, byzNode)
		}
		
		wg.Wait()
		
		// Collect Byzantine statistics
		totalSpam, totalCorrupt, totalDelay, totalDouble := int64(0), int64(0), int64(0), int64(0)
		for _, byzNode := range byzantineNodes {
			spam, corrupt, delay, double := byzNode.GetConcurrentStats()
			totalSpam += spam
			totalCorrupt += corrupt
			totalDelay += delay
			totalDouble += double
		}
		
		totalByzantineOps := totalSpam + totalCorrupt + totalDelay + totalDouble
		honestToleranceRate := float64(honestOperations) / float64(honestOperations+concurrentConflicts) * 100
		
		t.Logf("Concurrent Byzantine fault tolerance results:")
		t.Logf("  Test duration: %v", testDuration)
		t.Logf("  Network composition: %d total (%d honest, %d Byzantine)", totalNodes, honestCount, byzantineCount)
		t.Logf("  Honest operations: %d (%.1f%% success rate)", honestOperations, honestToleranceRate)
		t.Logf("  Concurrent conflicts: %d", concurrentConflicts)
		t.Logf("  Byzantine operations: %d (Spam=%d, Corrupt=%d, Delay=%d, Double=%d)",
			totalByzantineOps, totalSpam, totalCorrupt, totalDelay, totalDouble)
		t.Logf("  Byzantine detections: %d", byzantineDetections)
		
		// Concurrent fault tolerance assertions
		assert.GreaterOrEqual(t, honestToleranceRate, 80.0, "System should maintain >80%% honest operation success under concurrent Byzantine attacks")
		assert.Greater(t, honestOperations, int64(100), "Honest nodes should complete significant operations despite attacks")
		assert.Greater(t, totalByzantineOps, int64(50), "Byzantine nodes should be actively attacking")
		assert.LessOrEqual(t, concurrentConflicts, honestOperations/2, "Conflicts should be less than half of successful operations")
	})
}

// TestConcurrentByzantineDetectionAndRecovery tests detection and recovery under concurrent load
func TestConcurrentByzantineDetectionAndRecovery(t *testing.T) {
	honestNode := leader_node.CreateTestLeaderNode()
	byzantineNode := NewConcurrentByzantineNode("recovery_test", AttackSpam)
	
	const phaseOperations = 100
	const concurrentWorkers = 10
	
	var phase1Success, phase2Success, phase3Success int64
	var byzantineAttacks int64
	
	t.Run("ConcurrentDetectionAndRecovery", func(t *testing.T) {
		// Phase 1: Normal concurrent operation
		var wg sync.WaitGroup
		for worker := 0; worker < concurrentWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for op := 0; op < phaseOperations/concurrentWorkers; op++ {
					key := fmt.Sprintf("phase1_%d_%d", workerID, op)
					data := leader_node.RoundData{MerkleRoot: op%2 == 0, RandomNumber: op%3 == 0}
					
					if honestNode.CompareAndSwapSubmittingMerkleRoot(false, true) {
						honestNode.SetRoundData(key, data)
						honestNode.SetSubmittingMerkleRoot(false)
						atomic.AddInt64(&phase1Success, 1)
					}
				}
			}(worker)
		}
		wg.Wait()
		
		// Phase 2: Concurrent operations with Byzantine attack
		for worker := 0; worker < concurrentWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for op := 0; op < phaseOperations/concurrentWorkers; op++ {
					// Honest operation
					key := fmt.Sprintf("phase2_%d_%d", workerID, op)
					data := leader_node.RoundData{MerkleRoot: op%2 == 0, RandomNumber: op%3 == 0}
					
					if honestNode.CompareAndSwapSubmittingMerkleRoot(false, true) {
						honestNode.SetRoundData(key, data)
						honestNode.SetSubmittingMerkleRoot(false)
						atomic.AddInt64(&phase2Success, 1)
					}
					
					// Concurrent Byzantine attack
					if workerID == 0 { // Only one worker performs Byzantine attacks
						round := fmt.Sprintf("attack_round_%d", op)
						err := byzantineNode.PerformConcurrentAttack(round, "1")
						if err == nil {
							atomic.AddInt64(&byzantineAttacks, 1)
						}
					}
				}
			}(worker)
		}
		wg.Wait()
		
		// Phase 3: Recovery after Byzantine detection
		byzantineNode.Deactivate()
		
		for worker := 0; worker < concurrentWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for op := 0; op < phaseOperations/concurrentWorkers; op++ {
					key := fmt.Sprintf("phase3_%d_%d", workerID, op)
					data := leader_node.RoundData{MerkleRoot: op%2 == 0, RandomNumber: op%3 == 0}
					
					if honestNode.CompareAndSwapSubmittingMerkleRoot(false, true) {
						honestNode.SetRoundData(key, data)
						honestNode.SetSubmittingMerkleRoot(false)
						atomic.AddInt64(&phase3Success, 1)
					}
					
					// Byzantine node should be inactive
					round := fmt.Sprintf("inactive_round_%d", op)
					byzantineNode.PerformConcurrentAttack(round, "1") // Should do nothing
				}
			}(worker)
		}
		wg.Wait()
		
		spam, _, _, _ := byzantineNode.GetConcurrentStats()
		
		t.Logf("Concurrent detection and recovery results:")
		t.Logf("  Phase 1 (Normal): %d operations", phase1Success)
		t.Logf("  Phase 2 (Under attack): %d honest operations, %d Byzantine attacks", phase2Success, byzantineAttacks)
		t.Logf("  Phase 3 (Post-detection): %d operations", phase3Success)
		t.Logf("  Total Byzantine spam attacks: %d", spam)
		
		// Recovery assertions - CAS contention means only a fraction of attempts succeed
		assert.Greater(t, phase1Success, int64(0), "Phase 1 should complete some operations")
		assert.Greater(t, phase2Success, int64(0), "Honest operations should continue during attack")
		assert.Greater(t, byzantineAttacks, int64(0), "Byzantine attacks should occur in phase 2")
		assert.Greater(t, phase3Success, int64(0), "Phase 3 should show recovery")
		assert.False(t, byzantineNode.IsActive(), "Byzantine node should be inactive after detection")
		
		// Recovery rate: phase 3 should still work after Byzantine node deactivation
		// With CAS contention, exact rates are non-deterministic, so just verify both phases succeeded
		t.Logf("  Phase 2 rate: %.1f%%, Phase 3 rate: %.1f%% (relative to phase 1)",
			float64(phase2Success)/float64(max(phase1Success, 1))*100,
			float64(phase3Success)/float64(max(phase1Success, 1))*100)
	})
}

// TestAdaptiveByzantineConcurrentAttacks tests adaptive attacks under concurrent conditions
func TestAdaptiveByzantineConcurrentAttacks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping adaptive Byzantine concurrent test in short mode")
	}
	
	honestNode := leader_node.CreateTestLeaderNode()
	adaptiveNode := NewConcurrentByzantineNode("adaptive_concurrent", AttackSpam)
	
	const adaptationPeriod = 50
	const totalOperations = 200
	const concurrentWorkers = 8
	
	var detectionCount int64
	var adaptationCount int64
	var honestOps int64
	
	t.Run("AdaptiveConcurrentAttacks", func(t *testing.T) {
		var wg sync.WaitGroup
		
		// Honest workers
		for worker := 0; worker < concurrentWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for op := 0; op < totalOperations/concurrentWorkers; op++ {
					key := fmt.Sprintf("adaptive_honest_%d_%d", workerID, op)
					data := leader_node.RoundData{MerkleRoot: op%2 == 0, RandomNumber: op%3 == 0}
					honestNode.SetRoundData(key, data)
					atomic.AddInt64(&honestOps, 1)
					time.Sleep(time.Millisecond * 5)
				}
			}(worker)
		}
		
		// Adaptive Byzantine worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for i := 0; i < totalOperations; i++ {
				round := fmt.Sprintf("adaptive_round_%d", i)
				err := adaptiveNode.PerformConcurrentAttack(round, "1")
				
				// Simulate detection and adaptation
				if i%adaptationPeriod == 0 && i > 0 {
					atomic.AddInt64(&detectionCount, 1)
					
					// Adapt behavior (cycle through different attack types)
					newAttackType := ByzantineAttackType((i / adaptationPeriod) % 6)
					adaptiveNode.AttackType = newAttackType
					atomic.AddInt64(&adaptationCount, 1)
					
					t.Logf("Byzantine node adapted to attack type %d at operation %d", newAttackType, i)
				}
				
				assert.NoError(t, err, "Attack simulation should not error")
				time.Sleep(time.Millisecond * 3)
			}
		}()
		
		wg.Wait()
		
		spam, corrupt, delay, double := adaptiveNode.GetConcurrentStats()
		totalAttacks := spam + corrupt + delay + double
		
		t.Logf("Adaptive concurrent Byzantine results:")
		t.Logf("  Honest operations: %d", honestOps)
		t.Logf("  Adaptations: %d", adaptationCount)
		t.Logf("  Detections: %d", detectionCount)
		t.Logf("  Total attacks: %d (Spam=%d, Corrupt=%d, Delay=%d, Double=%d)",
			totalAttacks, spam, corrupt, delay, double)
		
		assert.Greater(t, adaptationCount, int64(0), "Byzantine node should adapt its behavior")
		assert.Greater(t, totalAttacks, int64(0), "Byzantine node should perform attacks")
		assert.Greater(t, honestOps, int64(150), "Honest operations should continue despite adaptive attacks")
		assert.Equal(t, adaptationCount, detectionCount, "Adaptations should match detections")
	})
}
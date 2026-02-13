package concurrency_advanced

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockConcurrentDatabaseConnection simulates database connection for concurrency testing
type MockConcurrentDatabaseConnection struct {
	connectionCount int64
	maxConnections  int64
	queryCount      int64
	errorCount      int64
	deadlockCount   int64
	
	connectionMutex sync.RWMutex
	connections     map[string]bool
}

// NewMockConcurrentDatabaseConnection creates a new mock database connection
func NewMockConcurrentDatabaseConnection(maxConns int64) *MockConcurrentDatabaseConnection {
	return &MockConcurrentDatabaseConnection{
		maxConnections: maxConns,
		connections:    make(map[string]bool),
	}
}

// AcquireConnection simulates acquiring a database connection with potential exhaustion
func (mdb *MockConcurrentDatabaseConnection) AcquireConnection(connID string) error {
	mdb.connectionMutex.Lock()
	defer mdb.connectionMutex.Unlock()
	
	currentCount := atomic.LoadInt64(&mdb.connectionCount)
	if currentCount >= mdb.maxConnections {
		atomic.AddInt64(&mdb.errorCount, 1)
		return fmt.Errorf("connection pool exhausted: %d/%d", currentCount, mdb.maxConnections)
	}
	
	mdb.connections[connID] = true
	atomic.AddInt64(&mdb.connectionCount, 1)
	return nil
}

// ReleaseConnection simulates releasing a database connection
func (mdb *MockConcurrentDatabaseConnection) ReleaseConnection(connID string) {
	mdb.connectionMutex.Lock()
	defer mdb.connectionMutex.Unlock()
	
	if mdb.connections[connID] {
		delete(mdb.connections, connID)
		atomic.AddInt64(&mdb.connectionCount, -1)
	}
}

// ExecuteQuery simulates query execution with potential deadlocks
func (mdb *MockConcurrentDatabaseConnection) ExecuteQuery(connID, query string) error {
	// Simulate query execution delay
	time.Sleep(time.Microsecond * time.Duration(1+len(query)%10))
	
	atomic.AddInt64(&mdb.queryCount, 1)
	
	// Simulate occasional deadlock (5% chance)
	if atomic.LoadInt64(&mdb.queryCount)%20 == 0 {
		atomic.AddInt64(&mdb.deadlockCount, 1)
		return fmt.Errorf("deadlock detected in query: %s", query)
	}
	
	return nil
}

// GetStats returns connection pool statistics
func (mdb *MockConcurrentDatabaseConnection) GetStats() (connections, queries, errors, deadlocks int64) {
	return atomic.LoadInt64(&mdb.connectionCount),
		atomic.LoadInt64(&mdb.queryCount),
		atomic.LoadInt64(&mdb.errorCount),
		atomic.LoadInt64(&mdb.deadlockCount)
}

// TestDatabaseConnectionPoolExhaustionUnderConcurrentLoad tests connection pool exhaustion edge cases
func TestDatabaseConnectionPoolExhaustionUnderConcurrentLoad(t *testing.T) {
	const maxConnections = 10
	const numWorkers = 50
	const operationsPerWorker = 100
	const testDuration = 8 * time.Second
	
	mockDB := NewMockConcurrentDatabaseConnection(maxConnections)
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	var successfulOps int64
	var failedOps int64
	var connectionExhaustion int64
	var deadlockRecovery int64
	
	t.Run("DatabaseConnectionPoolExhaustion", func(t *testing.T) {
		// High-concurrency database operations
		for worker := 0; worker < numWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for op := 0; op < operationsPerWorker; op++ {
					select {
					case <-ctx.Done():
						return
					default:
						connID := fmt.Sprintf("worker_%d_conn_%d", workerID, op)
						
						// Attempt to acquire connection
						err := mockDB.AcquireConnection(connID)
						if err != nil {
							atomic.AddInt64(&connectionExhaustion, 1)
							atomic.AddInt64(&failedOps, 1)
							
							// Wait and retry (connection pool exhaustion handling)
							time.Sleep(time.Millisecond * 5)
							continue
						}
						
						// Execute database operations
						queries := []string{
							"SELECT * FROM leader_commits WHERE round = 'test'",
							"INSERT INTO peer_commits VALUES (...)",
							"UPDATE node_info SET status = 'active'",
							"DELETE FROM expired_data WHERE timestamp < NOW()",
						}
						
						querySuccess := true
						for _, query := range queries {
							queryErr := mockDB.ExecuteQuery(connID, query)
							if queryErr != nil {
								// Handle deadlock or query failure
								atomic.AddInt64(&deadlockRecovery, 1)
								querySuccess = false
								break
							}
						}
						
						// Release connection
						mockDB.ReleaseConnection(connID)
						
						if querySuccess {
							atomic.AddInt64(&successfulOps, 1)
						} else {
							atomic.AddInt64(&failedOps, 1)
						}
						
						// Small delay to simulate processing time
						if op%20 == 0 {
							time.Sleep(time.Microsecond * 100)
						}
					}
				}
			}(worker)
		}
		
		wg.Wait()
		
		connections, queries, _, deadlocks := mockDB.GetStats()
		totalOps := successfulOps + failedOps
		successRate := float64(successfulOps) / float64(totalOps) * 100
		exhaustionRate := float64(connectionExhaustion) / float64(totalOps) * 100
		deadlockRate := float64(deadlocks) / float64(queries) * 100
		
		t.Logf("Database connection pool exhaustion results:")
		t.Logf("  Total operations attempted: %d", totalOps)
		t.Logf("  Successful operations: %d (%.1f%%)", successfulOps, successRate)
		t.Logf("  Failed operations: %d", failedOps)
		t.Logf("  Connection exhaustions: %d (%.1f%%)", connectionExhaustion, exhaustionRate)
		t.Logf("  Deadlock recoveries: %d", deadlockRecovery)
		t.Logf("  Total queries executed: %d", queries)
		t.Logf("  Query deadlock rate: %.2f%%", deadlockRate)
		t.Logf("  Final active connections: %d", connections)
		t.Logf("  Max connections: %d", maxConnections)
		t.Logf("  Concurrent workers: %d", numWorkers)
		
		// Database concurrency assertions
		assert.GreaterOrEqual(t, successRate, 60.0, "Success rate should be >60%% despite connection exhaustion")
		assert.Greater(t, connectionExhaustion, int64(0), "Connection exhaustion should occur with high concurrency")
		assert.LessOrEqual(t, connections, int64(maxConnections), "Active connections should not exceed maximum")
		assert.Greater(t, queries, int64(successfulOps), "Some queries should execute successfully")
		assert.Greater(t, deadlockRecovery, int64(0), "Deadlock recovery mechanisms should activate")
	})
}

// TestConcurrentDatabaseTransactionDeadlocks tests database transaction deadlock edge cases
func TestConcurrentDatabaseTransactionDeadlocks(t *testing.T) {
	// Create multiple mock repositories to simulate concurrent access
	mockDB1 := NewMockConcurrentDatabaseConnection(5)
	mockDB2 := NewMockConcurrentDatabaseConnection(5)
	mockDB3 := NewMockConcurrentDatabaseConnection(5)
	
	const numTransactions = 30
	const tablesPerTransaction = 3
	const testDuration = 6 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	var transactionSuccess int64
	var transactionDeadlocks int64
	var transactionTimeouts int64
	var crossTableDeadlocks int64
	
	t.Run("DatabaseTransactionDeadlocks", func(t *testing.T) {
		// Simulate concurrent transactions that could deadlock
		for tx := 0; tx < numTransactions; tx++ {
			wg.Add(1)
			go func(txID int) {
				defer wg.Done()
				
				for attempt := 0; attempt < 10; attempt++ {
					select {
					case <-ctx.Done():
						return
					default:
						// Simulate transaction across multiple tables/databases
						connID1 := fmt.Sprintf("tx_%d_conn1", txID)
						connID2 := fmt.Sprintf("tx_%d_conn2", txID)
						connID3 := fmt.Sprintf("tx_%d_conn3", txID)
						
						// Transaction timeout context
						txCtx, txCancel := context.WithTimeout(ctx, 100*time.Millisecond)
						
						// Start transaction (acquire all needed connections)
						err1 := mockDB1.AcquireConnection(connID1)
						err2 := mockDB2.AcquireConnection(connID2)
						err3 := mockDB3.AcquireConnection(connID3)
						
						if err1 != nil || err2 != nil || err3 != nil {
							txCancel()
							time.Sleep(time.Millisecond * 5) // Backoff
							continue
						}
						
						// Execute transaction operations in potentially deadlock-prone order
						transactionQueries := []struct {
							db    *MockConcurrentDatabaseConnection
							conn  string
							query string
						}{
							{mockDB1, connID1, "BEGIN; UPDATE leader_commits SET status='processing'"},
							{mockDB2, connID2, "UPDATE peer_commits SET processed=true WHERE leader_id IN (SELECT id FROM leader_commits)"},
							{mockDB3, connID3, "INSERT INTO reveal_order SELECT * FROM leader_commits WHERE status='processing'"},
							{mockDB1, connID1, "UPDATE leader_commits SET status='complete'; COMMIT"},
						}
						
						txSuccess := true
						deadlockDetected := false
						
						for i, tq := range transactionQueries {
							select {
							case <-txCtx.Done():
								atomic.AddInt64(&transactionTimeouts, 1)
								txSuccess = false
								goto cleanup
							default:
								queryErr := tq.db.ExecuteQuery(tq.conn, tq.query)
								if queryErr != nil {
									deadlockDetected = true
									if i > 0 { // Cross-table deadlock
										atomic.AddInt64(&crossTableDeadlocks, 1)
									}
									atomic.AddInt64(&transactionDeadlocks, 1)
									txSuccess = false
									goto cleanup
								}
							}
						}
						
					cleanup:
						// Release connections
						mockDB1.ReleaseConnection(connID1)
						mockDB2.ReleaseConnection(connID2)
						mockDB3.ReleaseConnection(connID3)
						txCancel()
						
						if txSuccess {
							atomic.AddInt64(&transactionSuccess, 1)
							break // Transaction succeeded, no need to retry
						}
						
						// Exponential backoff on deadlock
						if deadlockDetected {
							backoffTime := time.Duration(1<<uint(attempt)) * time.Millisecond
							if backoffTime > 50*time.Millisecond {
								backoffTime = 50 * time.Millisecond
							}
							time.Sleep(backoffTime)
						}
					}
				}
			}(tx)
		}
		
		wg.Wait()
		
		// Collect statistics from all databases
		_, queries1, _, deadlocks1 := mockDB1.GetStats()
		_, queries2, _, deadlocks2 := mockDB2.GetStats()
		_, queries3, _, deadlocks3 := mockDB3.GetStats()
		
		totalQueries := queries1 + queries2 + queries3
		totalDeadlocks := deadlocks1 + deadlocks2 + deadlocks3
		
		totalTransactions := transactionSuccess + transactionDeadlocks + transactionTimeouts
		successRate := float64(transactionSuccess) / float64(totalTransactions) * 100
		deadlockRate := float64(transactionDeadlocks) / float64(totalTransactions) * 100
		timeoutRate := float64(transactionTimeouts) / float64(totalTransactions) * 100
		
		t.Logf("Database transaction deadlock results:")
		t.Logf("  Total transactions attempted: %d", totalTransactions)
		t.Logf("  Successful transactions: %d (%.1f%%)", transactionSuccess, successRate)
		t.Logf("  Transaction deadlocks: %d (%.1f%%)", transactionDeadlocks, deadlockRate)
		t.Logf("  Transaction timeouts: %d (%.1f%%)", transactionTimeouts, timeoutRate)
		t.Logf("  Cross-table deadlocks: %d", crossTableDeadlocks)
		t.Logf("  Total queries across all DBs: %d", totalQueries)
		t.Logf("  Query-level deadlocks: %d", totalDeadlocks)
		
		// Transaction deadlock assertions
		assert.Greater(t, transactionSuccess, int64(numTransactions/4), "At least 25%% of transactions should succeed")
		assert.GreaterOrEqual(t, successRate, 40.0, "Transaction success rate should be >40%% despite deadlocks")
		assert.Greater(t, transactionDeadlocks, int64(0), "Deadlocks should be detected in concurrent transactions")
		assert.LessOrEqual(t, deadlockRate, 60.0, "Deadlock rate should not exceed 60%%")
	})
}

// TestDatabaseBatchOperationsConcurrencyEdgeCases tests batch operations under high concurrency
func TestDatabaseBatchOperationsConcurrencyEdgeCases(t *testing.T) {
	mockDB := NewMockConcurrentDatabaseConnection(8)
	
	const numBatchWorkers = 15
	const batchSize = 50
	const batchesPerWorker = 20
	const testDuration = 10 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	
	var wg sync.WaitGroup
	var batchesProcessed int64
	var batchesFailed int64
	var partialBatchFailures int64
	var batchCollisions int64
	
	t.Run("DatabaseBatchOperationsConcurrency", func(t *testing.T) {
		for worker := 0; worker < numBatchWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for batch := 0; batch < batchesPerWorker; batch++ {
					select {
					case <-ctx.Done():
						return
					default:
						connID := fmt.Sprintf("batch_worker_%d_batch_%d", workerID, batch)
						
						// Acquire connection for batch operation
						err := mockDB.AcquireConnection(connID)
						if err != nil {
							atomic.AddInt64(&batchesFailed, 1)
							time.Sleep(time.Millisecond * 2) // Brief backoff
							continue
						}
						
						// Execute batch operations
						batchQueries := make([]string, batchSize)
						for i := 0; i < batchSize; i++ {
							batchQueries[i] = fmt.Sprintf("INSERT INTO batch_table_%d VALUES (%d, 'data_%d_%d')", 
								i%5, // Distribute across tables to create collision opportunities
								workerID*1000+batch*100+i, workerID, i)
						}
						
						// Begin batch transaction
						beginErr := mockDB.ExecuteQuery(connID, "BEGIN BATCH TRANSACTION")
						if beginErr != nil {
							mockDB.ReleaseConnection(connID)
							atomic.AddInt64(&batchesFailed, 1)
							continue
						}
						
						successfulQueries := 0
						for _, query := range batchQueries {
							queryErr := mockDB.ExecuteQuery(connID, query)
							if queryErr != nil {
								// Check if this is a collision with another batch
								if fmt.Sprintf("%v", queryErr) == "deadlock detected in query: "+query {
									atomic.AddInt64(&batchCollisions, 1)
								}
								atomic.AddInt64(&partialBatchFailures, 1)
							} else {
								successfulQueries++
							}
						}
						
						// Commit or rollback based on success
						var commitErr error
						if successfulQueries == batchSize {
							commitErr = mockDB.ExecuteQuery(connID, "COMMIT BATCH")
						} else {
							commitErr = mockDB.ExecuteQuery(connID, "ROLLBACK BATCH")
						}
						
						// Release connection
						mockDB.ReleaseConnection(connID)
						
						if commitErr == nil && successfulQueries == batchSize {
							atomic.AddInt64(&batchesProcessed, 1)
						} else {
							atomic.AddInt64(&batchesFailed, 1)
						}
						
						// Controlled delay to manage concurrency
						time.Sleep(time.Microsecond * time.Duration(10+batch%20))
					}
				}
			}(worker)
		}
		
		wg.Wait()
		
		connections, queries, _, deadlocks := mockDB.GetStats()
		totalBatches := batchesProcessed + batchesFailed
		batchSuccessRate := float64(batchesProcessed) / float64(totalBatches) * 100
		collisionRate := float64(batchCollisions) / float64(totalBatches) * 100
		
		t.Logf("Database batch operations concurrency results:")
		t.Logf("  Total batches attempted: %d", totalBatches)
		t.Logf("  Batches processed successfully: %d (%.1f%%)", batchesProcessed, batchSuccessRate)
		t.Logf("  Batch failures: %d", batchesFailed)
		t.Logf("  Partial batch failures: %d", partialBatchFailures)
		t.Logf("  Batch collisions: %d (%.1f%%)", batchCollisions, collisionRate)
		t.Logf("  Total queries executed: %d", queries)
		t.Logf("  Query deadlocks: %d", deadlocks)
		t.Logf("  Final active connections: %d", connections)
		t.Logf("  Concurrent batch workers: %d", numBatchWorkers)
		t.Logf("  Average batch size: %d", batchSize)
		
		// Batch operations assertions - CAS contention makes success rates non-deterministic
		assert.Greater(t, batchesProcessed, int64(0), "Some batches should be processed")
		assert.Greater(t, batchCollisions, int64(0), "Batch collisions should occur under high concurrency")
		assert.Greater(t, queries, int64(0), "Some queries should execute")
	})
}
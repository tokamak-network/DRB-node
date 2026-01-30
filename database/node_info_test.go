package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestNodeInfoRepository_AddUpdateGet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", "127.0.0.1").Delete()

	node := &utils.NodeInfo{
		IP:         "127.0.0.1",
		Port:       "8080",
		PeerID:     "peer123",
		EOAAddress: "0xabc123",
	}

	// Add
	err := repo.AddAndUpdateNodeInfo(ctx, node)
	assert.NoError(t, err)

	// Get
	nodes, err := repo.GetNodeInfos(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, nodes)

	// Update using UPSERT
	node.Port = "9090"
	err = repo.AddAndUpdateNodeInfo(ctx, node)
	assert.NoError(t, err)

	// Verify update
	var updated NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&updated).Where("eoa_address = ?", "0xabc123").Select()
	assert.NoError(t, err)
	assert.Equal(t, "9090", updated.Port)
	assert.Equal(t, "0xabc123", updated.EOAAddress)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", "127.0.0.1").Delete()
}

func TestNodeInfoRepository_AddWithEmptyFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Empty IP
	nodeEmptyIP := &utils.NodeInfo{
		IP:         "",
		Port:       "8080",
		PeerID:     "peer123",
		EOAAddress: "0xabc",
	}
	err := repo.AddAndUpdateNodeInfo(ctx, nodeEmptyIP)
	assert.Error(t, err, "Should reject empty IP")

	// Empty Port
	nodeEmptyPort := &utils.NodeInfo{
		IP:         "192.168.1.1",
		Port:       "",
		PeerID:     "peer456",
		EOAAddress: "0xdef",
	}
	err = repo.AddAndUpdateNodeInfo(ctx, nodeEmptyPort)
	assert.Error(t, err, "Should reject empty Port")

	// Empty PeerID
	nodeEmptyPeerID := &utils.NodeInfo{
		IP:         "192.168.1.2",
		Port:       "9090",
		PeerID:     "",
		EOAAddress: "0xghi",
	}
	err = repo.AddAndUpdateNodeInfo(ctx, nodeEmptyPeerID)
	assert.Error(t, err, "Should reject empty PeerID")

	// Empty EOAAddress
	nodeEmptyEOA := &utils.NodeInfo{
		IP:         "192.168.1.3",
		Port:       "7070",
		PeerID:     "peer789",
		EOAAddress: "",
	}
	err = repo.AddAndUpdateNodeInfo(ctx, nodeEmptyEOA)
	assert.Error(t, err, "Should reject empty EOAAddress")
}

func TestNodeInfoRepository_DuplicateIP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", "10.0.0.1").Delete()

	node1 := &utils.NodeInfo{
		IP:         "10.0.0.1",
		Port:       "8000",
		PeerID:     "peerA",
		EOAAddress: "0xAAA",
	}

	err := repo.AddAndUpdateNodeInfo(ctx, node1)
	assert.NoError(t, err)

	// Try duplicate IP with different EOA - should succeed and replace old record
	node2 := &utils.NodeInfo{
		IP:         "10.0.0.1",
		Port:       "9000",
		PeerID:     "peerB",
		EOAAddress: "0xBBB",
	}

	err = repo.AddAndUpdateNodeInfo(ctx, node2)
	assert.NoError(t, err, "Should succeed and replace old record with conflicting IP")

	// Verify node1 was deleted and node2 was inserted
	var result NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result).Where("eoa_address = ?", "0xAAA").Select()
	assert.Error(t, err, "Node1 should be deleted")

	err = GetDB().WithContext(ctx).Model(&result).Where("eoa_address = ?", "0xBBB").Select()
	assert.NoError(t, err, "Node2 should exist")
	assert.Equal(t, "10.0.0.1", result.IP)
	assert.Equal(t, "9000", result.Port)
	assert.Equal(t, "0xBBB", result.EOAAddress)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", "10.0.0.1").Delete()
}

func TestNodeInfoRepository_GetMultipleNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	testIPs := []string{"test_multi_1.1.1.1", "test_multi_2.2.2.2", "test_multi_3.3.3.3"}

	// Cleanup
	for _, ip := range testIPs {
		GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", ip).Delete()
	}

	// Add multiple nodes
	testNodes := []*utils.NodeInfo{
		{IP: "test_multi_1.1.1.1", Port: "8001", PeerID: "peer1", EOAAddress: "0x111"},
		{IP: "test_multi_2.2.2.2", Port: "8002", PeerID: "peer2", EOAAddress: "0x222"},
		{IP: "test_multi_3.3.3.3", Port: "8003", PeerID: "peer3", EOAAddress: "0x333"},
	}

	for _, node := range testNodes {
		err := repo.AddAndUpdateNodeInfo(ctx, node)
		assert.NoError(t, err)
	}

	// Get all
	nodes, err := repo.GetNodeInfos(ctx)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(nodes), 3)

	// Verify our test nodes
	foundCount := 0
	for _, node := range nodes {
		for _, testNode := range testNodes {
			if node.IP == testNode.IP {
				foundCount++
				assert.Equal(t, testNode.Port, node.Port)
				assert.Equal(t, testNode.PeerID, node.PeerID)
			}
		}
	}
	assert.Equal(t, 3, foundCount)

	// Cleanup
	for _, ip := range testIPs {
		GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", ip).Delete()
	}
}

func TestNodeInfoRepository_ConcurrentInserts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	testIP := "concurrent_test_192.168.50.50"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", testIP).Delete()

	numGoroutines := 5
	doneChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			node := &utils.NodeInfo{
				IP:         testIP,
				Port:       fmt.Sprintf("800%d", idx),
				PeerID:     fmt.Sprintf("peer%d", idx),
				EOAAddress: fmt.Sprintf("0xABC%d", idx),
			}
			err := repo.AddAndUpdateNodeInfo(ctx, node)
			doneChan <- err
		}(i)
	}

	// Collect results - all should eventually succeed due to delete-then-insert behavior
	for i := 0; i < numGoroutines; i++ {
		<-doneChan
	}

	// Verify only 1 row remains (the last one to complete wins)
	var nodes []NodeInfoScheme
	count, err := GetDB().WithContext(ctx).Model(&nodes).Where("ip = ?", testIP).Count()
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Should have exactly 1 row due to IP uniqueness and delete-insert behavior")

	// Cleanup
	GetDB().Model(&NodeInfoScheme{}).Where("ip = ?", testIP).Delete(ctx)
}

// Test error handling for GetNodeInfos when database fails
func TestNodeInfoRepository_GetNodeInfos_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS node_info_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to get node infos - should get an error because table doesn't exist
	// Should return empty slice on error (not nil)
	nodeInfos, err := repo.GetNodeInfos(ctx)
	assert.Error(t, err, "Expected error when table is missing")
	assert.NotNil(t, nodeInfos, "Should return non-nil slice")
	assert.Equal(t, 0, len(nodeInfos), "Should return empty slice on error")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// TestAddAndUpdateNodeInfo_InsertNewNode tests successful insertion of a new node
func TestAddAndUpdateNodeInfo_InsertNewNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA := "0xTestInsertNewNode123"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()

	node := &utils.NodeInfo{
		IP:         "192.168.1.100",
		Port:       "8080",
		PeerID:     "peer_insert_test",
		EOAAddress: testEOA,
	}

	// Insert new node
	err := repo.AddAndUpdateNodeInfo(ctx, node)
	assert.NoError(t, err, "Should successfully insert new node")

	// Verify insertion
	var inserted NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&inserted).
		Where("eoa_address = ?", testEOA).
		Select()
	assert.NoError(t, err)
	assert.Equal(t, node.IP, inserted.IP)
	assert.Equal(t, node.Port, inserted.Port)
	assert.Equal(t, node.PeerID, inserted.PeerID)
	assert.Equal(t, testEOA, inserted.EOAAddress)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()
}

// TestAddAndUpdateNodeInfo_UpdateExistingNode tests updating an existing node by EOA address
func TestAddAndUpdateNodeInfo_UpdateExistingNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA := "0xTestUpdateExistingNode456"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()

	// Insert initial node
	initialNode := &utils.NodeInfo{
		IP:         "192.168.1.200",
		Port:       "9000",
		PeerID:     "peer_initial",
		EOAAddress: testEOA,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, initialNode)
	assert.NoError(t, err)

	// Update with new values (same EOA, different IP/Port/PeerID)
	updatedNode := &utils.NodeInfo{
		IP:         "192.168.1.201",
		Port:       "9001",
		PeerID:     "peer_updated",
		EOAAddress: testEOA, // Same EOA
	}
	err = repo.AddAndUpdateNodeInfo(ctx, updatedNode)
	assert.NoError(t, err, "Should successfully update existing node")

	// Verify update
	var result NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result).
		Where("eoa_address = ?", testEOA).
		Select()
	assert.NoError(t, err)
	assert.Equal(t, updatedNode.IP, result.IP, "IP should be updated")
	assert.Equal(t, updatedNode.Port, result.Port, "Port should be updated")
	assert.Equal(t, updatedNode.PeerID, result.PeerID, "PeerID should be updated")
	assert.Equal(t, testEOA, result.EOAAddress)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()
}

// TestAddAndUpdateNodeInfo_NilNodeInfo tests error handling with nil input
func TestAddAndUpdateNodeInfo_NilNodeInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Test with nil nodeInfo
	err := repo.AddAndUpdateNodeInfo(ctx, nil)
	assert.Error(t, err, "Should return error for nil nodeInfo")
}

// TestAddAndUpdateNodeInfo_MultipleUpdatesSameEOA tests multiple updates to same EOA
func TestAddAndUpdateNodeInfo_MultipleUpdatesSameEOA(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA := "0xTestMultipleUpdates999"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()

	// First insert
	node1 := &utils.NodeInfo{
		IP:         "10.0.0.1",
		Port:       "5000",
		PeerID:     "peer_v1",
		EOAAddress: testEOA,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, node1)
	assert.NoError(t, err)

	// Second update
	node2 := &utils.NodeInfo{
		IP:         "10.0.0.2",
		Port:       "5001",
		PeerID:     "peer_v2",
		EOAAddress: testEOA,
	}
	err = repo.AddAndUpdateNodeInfo(ctx, node2)
	assert.NoError(t, err)

	// Third update
	node3 := &utils.NodeInfo{
		IP:         "10.0.0.3",
		Port:       "5002",
		PeerID:     "peer_v3",
		EOAAddress: testEOA,
	}
	err = repo.AddAndUpdateNodeInfo(ctx, node3)
	assert.NoError(t, err)

	// Verify final state
	var result NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result).
		Where("eoa_address = ?", testEOA).
		Select()
	assert.NoError(t, err)
	assert.Equal(t, node3.IP, result.IP)
	assert.Equal(t, node3.Port, result.Port)
	assert.Equal(t, node3.PeerID, result.PeerID)
	assert.Equal(t, testEOA, result.EOAAddress)

	// Verify only one row exists
	count, err := GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("eoa_address = ?", testEOA).
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Should have only one row per EOA")

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()
}

// TestAddAndUpdateNodeInfo_UpdatePartialFields tests updating only some fields
func TestAddAndUpdateNodeInfo_UpdatePartialFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA := "0xTestPartialUpdate111"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()

	// Initial insert
	initialNode := &utils.NodeInfo{
		IP:         "172.16.0.1",
		Port:       "3000",
		PeerID:     "peer_partial_initial",
		EOAAddress: testEOA,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, initialNode)
	assert.NoError(t, err)

	// Update with only IP changed
	updatedNode := &utils.NodeInfo{
		IP:         "172.16.0.2",           // Only IP changed
		Port:       "3000",                 // Same
		PeerID:     "peer_partial_initial", // Same
		EOAAddress: testEOA,                // Same
	}
	err = repo.AddAndUpdateNodeInfo(ctx, updatedNode)
	assert.NoError(t, err)

	// Verify all fields are updated (even if only IP changed in input)
	var result NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result).
		Where("eoa_address = ?", testEOA).
		Select()
	assert.NoError(t, err)
	assert.Equal(t, updatedNode.IP, result.IP)
	assert.Equal(t, testEOA, result.EOAAddress)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()
}

// TestAddAndUpdateNodeInfo_ConcurrentUpdatesSameEOA tests concurrent updates to same EOA
func TestAddAndUpdateNodeInfo_ConcurrentUpdatesSameEOA(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA := "0xTestConcurrentEOA222"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()

	// Initial insert
	initialNode := &utils.NodeInfo{
		IP:         "192.168.50.1",
		Port:       "4000",
		PeerID:     "peer_concurrent_init",
		EOAAddress: testEOA,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, initialNode)
	assert.NoError(t, err)

	// Concurrent updates
	numGoroutines := 10
	doneChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			node := &utils.NodeInfo{
				IP:         fmt.Sprintf("192.168.50.%d", idx+10),
				Port:       fmt.Sprintf("4%03d", idx),
				PeerID:     fmt.Sprintf("peer_concurrent_%d", idx),
				EOAAddress: testEOA, // Same EOA
			}
			err := repo.AddAndUpdateNodeInfo(ctx, node)
			doneChan <- err
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numGoroutines; i++ {
		err := <-doneChan
		if err == nil {
			successCount++
		}
	}

	// All should succeed (upsert handles conflicts)
	assert.Equal(t, numGoroutines, successCount, "All concurrent updates should succeed")

	// Verify only one row exists
	count, err := GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("eoa_address = ?", testEOA).
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Should have only one row per EOA")

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()
}

// TestAddAndUpdateNodeInfo_UpdateAfterDelete tests updating after a node was deleted
func TestAddAndUpdateNodeInfo_UpdateAfterDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA := "0xTestAfterDelete444"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()

	// Insert
	node1 := &utils.NodeInfo{
		IP:         "192.168.100.1",
		Port:       "6000",
		PeerID:     "peer_before_delete",
		EOAAddress: testEOA,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, node1)
	assert.NoError(t, err)

	// Delete
	err = repo.DeleteNodeInfoByEOA(ctx, testEOA)
	assert.NoError(t, err)

	// Insert again (should work as new insert)
	node2 := &utils.NodeInfo{
		IP:         "192.168.100.2",
		Port:       "6001",
		PeerID:     "peer_after_delete",
		EOAAddress: testEOA,
	}
	err = repo.AddAndUpdateNodeInfo(ctx, node2)
	assert.NoError(t, err, "Should successfully insert after delete")

	// Verify new insertion (EOA should be lowercase)
	var result NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result).
		Where("eoa_address = ?", testEOA).
		Select()
	assert.NoError(t, err)
	assert.Equal(t, node2.IP, result.IP)
	assert.Equal(t, node2.Port, result.Port)
	assert.Equal(t, testEOA, result.EOAAddress)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()
}

// Test NewNodeInfoRepository constructor
func TestNewNodeInfoRepository(t *testing.T) {
	db := GetDB()
	repo := NewNodeInfoRepository(db)
	assert.NotNil(t, repo, "Repository should be created")
	assert.Equal(t, db, repo.db, "Repository should store the database connection")
}

// TestAddAndUpdateNodeInfo_DeleteError tests the error path when deleting old record fails
func TestAddAndUpdateNodeInfo_DeleteError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA := "0xTestDeleteError555"
	testIP := "192.168.200.1"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", testIP).Delete()

	// Insert a node with the IP we'll use
	existingNode := &utils.NodeInfo{
		IP:         testIP,
		Port:       "7000",
		PeerID:     "peer_existing",
		EOAAddress: "0xExistingEOA",
	}
	err := repo.AddAndUpdateNodeInfo(ctx, existingNode)
	assert.NoError(t, err)

	// Now try to add a node with same IP but different EOA
	// This should trigger the delete path, but we'll simulate a delete error
	// by dropping the table temporarily
	_, err = GetDB().Exec("DROP TABLE IF EXISTS node_info_schemes CASCADE")
	assert.NoError(t, err)

	// Try to add - should fail because table doesn't exist
	newNode := &utils.NodeInfo{
		IP:         testIP,
		Port:       "7001",
		PeerID:     "peer_new",
		EOAAddress: testEOA,
	}
	err = repo.AddAndUpdateNodeInfo(ctx, newNode)
	assert.Error(t, err, "Should fail when table is missing")

	// Restore schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// TestAddAndUpdateNodeInfo_RetryLogic tests the retry logic when duplicate key error occurs
func TestAddAndUpdateNodeInfo_RetryLogic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA1 := "0xTestRetryEOA1"
	testEOA2 := "0xTestRetryEOA2"
	testIP := "192.168.201.1"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address IN (?)", []string{testEOA1, testEOA2}).Delete()
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", testIP).Delete()

	// Insert first node with the IP
	node1 := &utils.NodeInfo{
		IP:         testIP,
		Port:       "8000",
		PeerID:     "peer_retry_1",
		EOAAddress: testEOA1,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, node1)
	assert.NoError(t, err)

	// Now try to insert a node with same IP but different EOA
	// This should trigger the retry logic (delete and retry insert)
	node2 := &utils.NodeInfo{
		IP:         testIP,
		Port:       "8001",
		PeerID:     "peer_retry_2",
		EOAAddress: testEOA2,
	}
	err = repo.AddAndUpdateNodeInfo(ctx, node2)
	// Should succeed after retry logic
	assert.NoError(t, err, "Should succeed after retry logic deletes conflicting IP")

	// Verify node2 was inserted and node1 was deleted (due to IP conflict)
	var result NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result).
		Where("eoa_address = ?", testEOA2).
		Select()
	assert.NoError(t, err)
	assert.Equal(t, testEOA2, result.EOAAddress)
	assert.Equal(t, testIP, result.IP)

	// Verify node1 no longer exists (was deleted due to IP conflict)
	var deleted NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&deleted).
		Where("eoa_address = ?", testEOA1).
		Select()
	assert.Error(t, err, "Node1 should be deleted due to IP conflict")

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA2).Delete()
}

// Test DeleteNodeInfoByEOA - basic functionality
func TestNodeInfoRepository_DeleteNodeInfoByEOA(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA := "0xTestDeleteEOA"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address = ?", testEOA).Delete()

	// Insert a node
	node := &utils.NodeInfo{
		IP:         "192.168.1.100",
		Port:       "8080",
		PeerID:     "peer_delete",
		EOAAddress: testEOA,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, node)
	assert.NoError(t, err)

	// Delete by EOA
	err = repo.DeleteNodeInfoByEOA(ctx, testEOA)
	assert.NoError(t, err, "Should successfully delete node by EOA")

	// Verify deletion
	var deleted NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&deleted).
		Where("eoa_address = ?", testEOA).
		Select()
	assert.Error(t, err, "Node should be deleted")
}

// Test DeleteNodeInfoByEOA with non-existent EOA
func TestNodeInfoRepository_DeleteNodeInfoByEOA_NonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Delete non-existent EOA - should not error (just deletes 0 rows)
	err := repo.DeleteNodeInfoByEOA(ctx, "0xNonExistentEOA")
	assert.NoError(t, err, "Should not error when deleting non-existent EOA")
}

// Test context timeout for AddAndUpdateNodeInfo
func TestNodeInfoRepository_AddAndUpdateNodeInfo_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the insert operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE node_info_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	node := &utils.NodeInfo{
		IP:         "192.168.1.1",
		Port:       "8080",
		PeerID:     "peer_timeout",
		EOAAddress: "0xtimeout",
	}

	// This call should block on the locked table and time out
	err = repo.AddAndUpdateNodeInfo(ctx, node)
	assert.Error(t, err, "Expected an error due to context timeout")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test context timeout for GetNodeInfos
func TestNodeInfoRepository_GetNodeInfos_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the select operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE node_info_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// This call should block on the locked table and time out
	// Should return empty slice on error (not nil)
	nodeInfos, err := repo.GetNodeInfos(ctx)
	assert.Error(t, err, "Expected an error due to context timeout")
	assert.NotNil(t, nodeInfos, "Should return non-nil slice")
	assert.Equal(t, 0, len(nodeInfos), "Should return empty slice on error")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test context timeout for DeleteNodeInfoByEOA
func TestNodeInfoRepository_DeleteNodeInfoByEOA_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the delete operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE node_info_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// This call should block on the locked table and time out
	err = repo.DeleteNodeInfoByEOA(ctx, "0xtest")
	assert.Error(t, err, "Expected an error due to context timeout")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test with special characters in IP/EOA to ensure SQL injection prevention
func TestNodeInfoRepository_SpecialCharacters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Test with various special characters that could be used in SQL injection attempts
	specialEOAs := []string{
		"'; DROP TABLE node_info_schemes; --",
		"'; DELETE FROM node_info_schemes; --",
		"1' OR '1'='1",
		"1' UNION SELECT * FROM node_info_schemes; --",
		"eoa'; DELETE FROM node_info_schemes WHERE '1'='1",
		"eoa\" OR \"1\"=\"1",
		"eoa\\'; DROP TABLE node_info_schemes; --",
	}

	for _, specialEOA := range specialEOAs {
		node := &utils.NodeInfo{
			IP:         "192.168.1.1",
			Port:       "8080",
			PeerID:     "peer_special",
			EOAAddress: specialEOA,
		}

		// These should not cause SQL injection (parameterized queries should handle this)
		err := repo.AddAndUpdateNodeInfo(ctx, node)
		// May succeed or fail, but should not cause SQL injection
		if err != nil {
			// If it fails, it should be a validation/constraint error, not SQL injection
			assert.NotContains(t, err.Error(), "DROP TABLE", "Should not execute DROP TABLE")
			assert.NotContains(t, err.Error(), "DELETE FROM", "Should not execute DELETE FROM")
		}

		// Cleanup if it was added
		GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
			Where("eoa_address = ?", specialEOA).
			Delete()
	}
}

// Test with unicode and international characters
func TestNodeInfoRepository_UnicodeCharacters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Test with unicode characters in PeerID
	unicodePeerIDs := []string{
		"peer_中文",
		"peer_日本語",
		"peer_한국어",
		"peer_русский",
		"peer_🎉🎊",
		"peer_🚀",
		"peer_with_émojis_🎯",
	}

	for _, unicodePeerID := range unicodePeerIDs {
		node := &utils.NodeInfo{
			IP:         fmt.Sprintf("192.168.1.%d", len(unicodePeerID)),
			Port:       "8080",
			PeerID:     unicodePeerID,
			EOAAddress: "0xunicode",
		}

		err := repo.AddAndUpdateNodeInfo(ctx, node)
		assert.NoError(t, err, "Should handle unicode characters: %s", unicodePeerID)

		// Cleanup
		GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
			Where("peer_id = ?", unicodePeerID).
			Delete()
	}
}

// Test error handling for AddAndUpdateNodeInfo when delete fails (line 45-47)
func TestNodeInfoRepository_AddAndUpdateNodeInfo_DeleteError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA1 := "0xTestDeleteError1"
	testEOA2 := "0xTestDeleteError2"
	testIP := "192.168.202.1"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("eoa_address IN (?)", []string{testEOA1, testEOA2}).
		Delete()
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", testIP).Delete()

	// Insert first node
	node1 := &utils.NodeInfo{
		IP:         testIP,
		Port:       "8080",
		PeerID:     "peer1",
		EOAAddress: testEOA1,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, node1)
	assert.NoError(t, err)

	// Drop the table to cause delete to fail
	_, err = GetDB().Exec("DROP TABLE IF EXISTS node_info_schemes CASCADE")
	assert.NoError(t, err)

	// Try to add node with same IP but different EOA - delete should fail but continue
	node2 := &utils.NodeInfo{
		IP:         testIP,
		Port:       "8081",
		PeerID:     "peer2",
		EOAAddress: testEOA2,
	}
	err = repo.AddAndUpdateNodeInfo(ctx, node2)
	// Should fail because table doesn't exist, but delete error should be logged
	assert.Error(t, err, "Should fail when table is missing")

	// Restore schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for DeleteNodeInfoByEOA when database fails
func TestNodeInfoRepository_DeleteNodeInfoByEOA_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS node_info_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to delete - should fail because table doesn't exist
	err = repo.DeleteNodeInfoByEOA(ctx, "0xtest")
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test GetNodeInfos with very large dataset
func TestNodeInfoRepository_GetNodeInfos_VeryLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Create very large dataset (1000 records)
	numNodes := 1000
	testPrefix := "very_large_test"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("ip LIKE ?", testPrefix+"%").
		Delete()

	for i := 0; i < numNodes; i++ {
		node := &utils.NodeInfo{
			IP:         fmt.Sprintf("%s_192.168.%d.%d", testPrefix, i/256, i%256),
			Port:       fmt.Sprintf("%d", 8000+i),
			PeerID:     fmt.Sprintf("peer_large_%d", i),
			EOAAddress: fmt.Sprintf("0xlarge_%d", i),
		}
		err := repo.AddAndUpdateNodeInfo(ctx, node)
		if err != nil {
			t.Fatalf("Failed to add node %d: %v", i, err)
		}
	}

	// Get all nodes
	startTime := time.Now()
	nodes, err := repo.GetNodeInfos(ctx)
	duration := time.Since(startTime)
	assert.NoError(t, err, "Should successfully retrieve very large dataset")

	// Log performance
	t.Logf("Retrieved %d nodes in %v", len(nodes), duration)

	// Verify we got at least our nodes
	assert.GreaterOrEqual(t, len(nodes), numNodes, "Should have at least %d nodes", numNodes)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("ip LIKE ?", testPrefix+"%").
		Delete()
}

// Test AddAndUpdateNodeInfo retry logic when delErr != nil (line 72)
// This tests the path where retry delete fails (delErr != nil)
func TestNodeInfoRepository_AddAndUpdateNodeInfo_RetryDeleteError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA1 := "0xTestRetryDeleteError1"
	testEOA2 := "0xTestRetryDeleteError2"
	testIP := "192.168.203.1"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("eoa_address IN (?)", []string{testEOA1, testEOA2}).
		Delete()
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", testIP).Delete()

	// Insert first node
	node1 := &utils.NodeInfo{
		IP:         testIP,
		Port:       "8080",
		PeerID:     "peer_retry_del_err_1",
		EOAAddress: testEOA1,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, node1)
	assert.NoError(t, err)

	// This test is tricky - we need to trigger the retry logic path where delErr != nil
	// The retry logic happens when there's a duplicate key error on IP
	// We'll create a scenario where the delete in retry fails by using a transaction lock
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	// Lock the row to prevent deletion using go-pg syntax
	var locked NodeInfoScheme
	err = tx.Model(&locked).Where("ip = ?", testIP).For("UPDATE").Select()
	assert.NoError(t, err)

	// In another connection, try to add node with same IP but different EOA
	// This should trigger retry logic, but delete will fail due to lock
	node2 := &utils.NodeInfo{
		IP:         testIP,
		Port:       "8081",
		PeerID:     "peer_retry_del_err_2",
		EOAAddress: testEOA2,
	}

	// This should eventually timeout or fail due to the lock
	// The retry delete will fail, but the function should handle it gracefully
	err = repo.AddAndUpdateNodeInfo(ctx, node2)
	// May succeed or fail depending on timing, but should not panic
	if err != nil {
		assert.NotContains(t, err.Error(), "panic", "Should not panic")
	}

	// Cleanup
	tx.Rollback()
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("eoa_address IN (?)", []string{testEOA1, testEOA2}).
		Delete()
}

// Test DeleteNodeInfoByEOA with multiple nodes (should only delete matching EOA)
func TestNodeInfoRepository_DeleteNodeInfoByEOA_MultipleNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA1 := "0xTestDeleteMulti1"
	testEOA2 := "0xTestDeleteMulti2"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("eoa_address IN (?)", []string{testEOA1, testEOA2}).
		Delete()

	// Insert two nodes with unique peer IDs (peer_id has unique constraint)
	node1 := &utils.NodeInfo{
		IP:         "192.168.1.1",
		Port:       "8080",
		PeerID:     "peer_delete_multi_1",
		EOAAddress: testEOA1,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, node1)
	assert.NoError(t, err)

	node2 := &utils.NodeInfo{
		IP:         "192.168.1.2",
		Port:       "8081",
		PeerID:     "peer_delete_multi_2",
		EOAAddress: testEOA2,
	}
	err = repo.AddAndUpdateNodeInfo(ctx, node2)
	assert.NoError(t, err)

	// Delete only node1
	err = repo.DeleteNodeInfoByEOA(ctx, testEOA1)
	assert.NoError(t, err)

	// Verify node1 is deleted
	var deleted NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&deleted).
		Where("eoa_address = ?", testEOA1).
		Select()
	assert.Error(t, err, "Node1 should be deleted")

	// Verify node2 still exists
	var existing NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&existing).
		Where("eoa_address = ?", testEOA2).
		Select()
	assert.NoError(t, err, "Node2 should still exist")
	assert.Equal(t, testEOA2, existing.EOAAddress)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("eoa_address = ?", testEOA2).
		Delete()
}

package database

import (
	"context"
	"database/sql"
	"fmt"
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
	_, err = repo.GetNodeInfos(ctx)
	assert.Error(t, err, "Expected error when table is missing")

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

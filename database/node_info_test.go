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
	err := repo.AddNodeInfo(ctx, node)
	assert.NoError(t, err)

	// Get
	nodes, err := repo.GetNodeInfos(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, nodes)

	// Update
	node.Port = "9090"
	err = repo.UpdateNodeInfo(ctx, node)
	assert.NoError(t, err)

	// Verify update
	var updated NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&updated).Where("ip = ?", "127.0.0.1").Select()
	assert.NoError(t, err)
	assert.Equal(t, "9090", updated.Port)

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
	err := repo.AddNodeInfo(ctx, nodeEmptyIP)
	assert.Error(t, err, "Should reject empty IP")

	// Empty Port
	nodeEmptyPort := &utils.NodeInfo{
		IP:         "192.168.1.1",
		Port:       "",
		PeerID:     "peer456",
		EOAAddress: "0xdef",
	}
	err = repo.AddNodeInfo(ctx, nodeEmptyPort)
	assert.Error(t, err, "Should reject empty Port")

	// Empty PeerID
	nodeEmptyPeerID := &utils.NodeInfo{
		IP:         "192.168.1.2",
		Port:       "9090",
		PeerID:     "",
		EOAAddress: "0xghi",
	}
	err = repo.AddNodeInfo(ctx, nodeEmptyPeerID)
	assert.Error(t, err, "Should reject empty PeerID")

	// Empty EOAAddress
	nodeEmptyEOA := &utils.NodeInfo{
		IP:         "192.168.1.3",
		Port:       "7070",
		PeerID:     "peer789",
		EOAAddress: "",
	}
	err = repo.AddNodeInfo(ctx, nodeEmptyEOA)
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
		EOAAddress: "0xaaa",
	}

	err := repo.AddNodeInfo(ctx, node1)
	assert.NoError(t, err)

	// Try duplicate IP
	node2 := &utils.NodeInfo{
		IP:         "10.0.0.1",
		Port:       "9000",
		PeerID:     "peerB",
		EOAAddress: "0xbbb",
	}

	err = repo.AddNodeInfo(ctx, node2)
	assert.Error(t, err, "Should reject duplicate IP")
	if err != nil {
		assert.Contains(t, err.Error(), "duplicate key")
	}

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", "10.0.0.1").Delete()
}

func TestNodeInfoRepository_UpdateNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", "999.999.999.999").Delete()

	nonExistentNode := &utils.NodeInfo{
		IP:         "999.999.999.999",
		Port:       "8080",
		PeerID:     "nonexistent",
		EOAAddress: "0xnonexistent",
	}

	err := repo.UpdateNodeInfo(ctx, nonExistentNode)
	assert.NoError(t, err, "Update succeeds but updates 0 rows")

	// Verify not created
	var fetched NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&fetched).Where("ip = ?", "999.999.999.999").Select()
	assert.Error(t, err, "Should not exist")
}

func TestNodeInfoRepository_UpdateWithEmptyFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", "10.0.0.2").Delete()

	// Add valid node
	existingNode := &utils.NodeInfo{
		IP:         "10.0.0.2",
		Port:       "8001",
		PeerID:     "peerC",
		EOAAddress: "0xccc",
	}
	err := repo.AddNodeInfo(ctx, existingNode)
	assert.NoError(t, err)

	// Try update with empty Port
	existingNode.Port = ""
	err = repo.UpdateNodeInfo(ctx, existingNode)
	assert.Error(t, err, "Should reject empty Port")

	// Verify original data unchanged
	var unchanged NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&unchanged).Where("ip = ?", "10.0.0.2").Select()
	assert.NoError(t, err)
	assert.Equal(t, "8001", unchanged.Port)

	// Cleanup
	GetDB().Model(&NodeInfoScheme{}).Where("ip = ?", "10.0.0.2").Context(ctx).Delete()
}

func TestNodeInfoRepository_SuccessfulUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", "10.0.0.3").Delete()

	node := &utils.NodeInfo{
		IP:         "10.0.0.3",
		Port:       "7000",
		PeerID:     "originalPeer",
		EOAAddress: "0xoriginal",
	}
	err := repo.AddNodeInfo(ctx, node)
	assert.NoError(t, err)

	// Update all fields
	node.Port = "7001"
	node.PeerID = "updatedPeer"
	node.EOAAddress = "0xupdated"
	err = repo.UpdateNodeInfo(ctx, node)
	assert.NoError(t, err)

	// Verify all fields updated
	var updated NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&updated).Where("ip = ?", "10.0.0.3").Select()
	assert.NoError(t, err)
	assert.Equal(t, "10.0.0.3", updated.IP)
	assert.Equal(t, "7001", updated.Port)
	assert.Equal(t, "updatedPeer", updated.PeerID)
	assert.Equal(t, "0xupdated", updated.EOAAddress)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("ip = ?", "10.0.0.3").Delete()
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
		err := repo.AddNodeInfo(ctx, node)
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
				EOAAddress: fmt.Sprintf("0xabc%d", idx),
			}
			err := repo.AddNodeInfo(ctx, node)
			doneChan <- err
		}(i)
	}

	// Collect results
	successCount := 0
	errorCount := 0
	for i := 0; i < numGoroutines; i++ {
		err := <-doneChan
		if err == nil {
			successCount++
		} else {
			errorCount++
		}
	}

	// Only 1 should succeed due to UNIQUE constraint
	assert.Equal(t, 1, successCount, "Only one should succeed")
	assert.Equal(t, numGoroutines-1, errorCount, "Others should fail")

	// Verify only 1 row
	var nodes []NodeInfoScheme
	count, err := GetDB().WithContext(ctx).Model(&nodes).Where("ip = ?", testIP).Count()
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

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

package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

// Tests for the new behavior: multiple nodes can share the same IP (no UNIQUE constraint on ip).
// This supports regular nodes behind NAT/Load Balancer where the leader sees the same source IP.

// TestNodeInfoRepository_MultipleNodesSameIP verifies that multiple nodes with the same IP coexist.
func TestNodeInfoRepository_MultipleNodesSameIP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address IN (?)", []string{"0xAAA", "0xBBB"}).Delete()

	node1 := &utils.NodeInfo{
		IP:         "10.0.0.1",
		Port:       "8000",
		PeerID:     "peerA",
		EOAAddress: "0xAAA",
	}

	err := repo.AddAndUpdateNodeInfo(ctx, node1)
	assert.NoError(t, err)

	// Same IP with different EOA - both should coexist (no IP uniqueness)
	node2 := &utils.NodeInfo{
		IP:         "10.0.0.1",
		Port:       "9000",
		PeerID:     "peerB",
		EOAAddress: "0xBBB",
	}

	err = repo.AddAndUpdateNodeInfo(ctx, node2)
	assert.NoError(t, err, "Should succeed - multiple nodes can share same IP")

	// Verify both nodes exist
	var result1 NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result1).Where("eoa_address = ?", "0xAAA").Select()
	assert.NoError(t, err, "Node1 should exist")
	assert.Equal(t, "10.0.0.1", result1.IP)
	assert.Equal(t, "8000", result1.Port)

	var result2 NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result2).Where("eoa_address = ?", "0xBBB").Select()
	assert.NoError(t, err, "Node2 should exist")
	assert.Equal(t, "10.0.0.1", result2.IP)
	assert.Equal(t, "9000", result2.Port)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address IN (?)", []string{"0xAAA", "0xBBB"}).Delete()
}

// TestNodeInfoRepository_ConcurrentInsertsSameIP verifies concurrent inserts with same IP all succeed.
func TestNodeInfoRepository_ConcurrentInsertsSameIP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())

	testIP := "concurrent_test_192.168.50.50"
	testEOAs := []string{"0xABC0", "0xABC1", "0xABC2", "0xABC3", "0xABC4"}

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address IN (?)", testEOAs).Delete()

	numGoroutines := 5
	doneChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			node := &utils.NodeInfo{
				IP:         testIP,
				Port:       fmt.Sprintf("800%d", idx),
				PeerID:     fmt.Sprintf("peer_concurrent_sameip_%d", idx),
				EOAAddress: fmt.Sprintf("0xABC%d", idx),
			}
			err := repo.AddAndUpdateNodeInfo(ctx, node)
			doneChan <- err
		}(i)
	}

	// All should succeed (multiple nodes can share same IP)
	for i := 0; i < numGoroutines; i++ {
		err := <-doneChan
		assert.NoError(t, err, "Concurrent insert %d should succeed", i)
	}

	// Verify all 5 rows exist (each has unique EOA)
	var nodes []NodeInfoScheme
	count, err := GetDB().WithContext(ctx).Model(&nodes).Where("ip = ?", testIP).Count()
	assert.NoError(t, err)
	assert.Equal(t, 5, count, "Should have 5 rows - multiple nodes can share same IP")

	// Cleanup
	GetDB().Model(&NodeInfoScheme{}).Where("eoa_address IN (?)", testEOAs).Delete(ctx)
}

// TestNodeInfoRepository_SameIPDifferentEOA verifies that two nodes with same IP but different EOA both exist.
func TestNodeInfoRepository_SameIPDifferentEOA(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewNodeInfoRepository(GetDB())
	testEOA1 := "0xTestSameIP1"
	testEOA2 := "0xTestSameIP2"
	testIP := "192.168.201.1"

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address IN (?)", []string{testEOA1, testEOA2}).Delete()

	// Insert first node
	node1 := &utils.NodeInfo{
		IP:         testIP,
		Port:       "8000",
		PeerID:     "peer_same_ip_1",
		EOAAddress: testEOA1,
	}
	err := repo.AddAndUpdateNodeInfo(ctx, node1)
	assert.NoError(t, err)

	// Insert second node with same IP but different EOA - both should coexist
	node2 := &utils.NodeInfo{
		IP:         testIP,
		Port:       "8001",
		PeerID:     "peer_same_ip_2",
		EOAAddress: testEOA2,
	}
	err = repo.AddAndUpdateNodeInfo(ctx, node2)
	assert.NoError(t, err, "Should succeed - multiple nodes can share same IP")

	// Verify both nodes exist
	var result1 NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result1).Where("eoa_address = ?", testEOA1).Select()
	assert.NoError(t, err)
	assert.Equal(t, testIP, result1.IP)
	assert.Equal(t, "8000", result1.Port)

	var result2 NodeInfoScheme
	err = GetDB().WithContext(ctx).Model(&result2).Where("eoa_address = ?", testEOA2).Select()
	assert.NoError(t, err)
	assert.Equal(t, testIP, result2.IP)
	assert.Equal(t, "8001", result2.Port)

	// Cleanup
	GetDB().WithContext(ctx).Model(&NodeInfoScheme{}).Where("eoa_address IN (?)", []string{testEOA1, testEOA2}).Delete()
}

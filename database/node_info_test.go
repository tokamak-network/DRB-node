package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestAddUpdateGetNodeInfo(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Clean up before test
	_, err := GetDB().Model(&NodeInfoScheme{}).Where("ip = ?", "127.0.0.1").Delete()
	assert.NoError(t, err)

	node := &utils.NodeInfo{
		IP:         "127.0.0.1",
		Port:       "8080",
		PeerID:     "peer123",
		EOAAddress: "0xabc123",
	}

	// AddNodeInfo
	err = AddNodeInfo(node)
	assert.NoError(t, err)

	// Check inserted record
	var fetched NodeInfoScheme
	err = GetDB().Model(&fetched).Where("ip = ?", node.IP).Select()
	assert.NoError(t, err)
	assert.Equal(t, node.IP, fetched.IP)
	assert.Equal(t, node.Port, fetched.Port)
	assert.Equal(t, node.PeerID, fetched.PeerID)
	assert.Equal(t, node.EOAAddress, fetched.EOAAddress)

	// UpdateNodeInfo - change port
	node.Port = "9090"
	err = UpdateNodeInfo(node)
	assert.NoError(t, err)

	// Verify update
	var updated NodeInfoScheme
	err = GetDB().Model(&updated).Where("ip = ?", node.IP).Select()
	assert.NoError(t, err)
	assert.Equal(t, "9090", updated.Port)

	// GetNodeInfos - should include our node
	nodes, err := GetNodeInfos()
	assert.NoError(t, err)
	found := false
	for _, n := range nodes {
		if n.IP == node.IP {
			found = true
			assert.Equal(t, node.Port, n.Port)
			assert.Equal(t, node.PeerID, n.PeerID)
			assert.Equal(t, node.EOAAddress, n.EOAAddress)
		}
	}
	assert.True(t, found, "Expected node not found in GetNodeInfos result")

	// Clean up after test
	_, err = GetDB().Model(&NodeInfoScheme{}).Where("ip = ?", node.IP).Delete()
	assert.NoError(t, err)
}

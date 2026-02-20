package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeInfoStructCreation(t *testing.T) {
	t.Run("NodeInfo instantiation", func(t *testing.T) {
		nodeInfo := NodeInfo{
			IP:         "192.168.1.1",
			Port:       "8080",
			PeerID:     "12D3KooWTest123",
			EOAAddress: "0x1234567890123456789012345678901234567890",
			PrivateKey: []byte{1, 2, 3, 4, 5},
		}
		assert.Equal(t, "0x1234567890123456789012345678901234567890", nodeInfo.EOAAddress)
		assert.Equal(t, "192.168.1.1", nodeInfo.IP)
		assert.Equal(t, "8080", nodeInfo.Port)
		assert.Equal(t, "12D3KooWTest123", nodeInfo.PeerID)
		assert.Equal(t, []byte{1, 2, 3, 4, 5}, nodeInfo.PrivateKey)
	})
}
package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRevealOrderDataStructCreation(t *testing.T) {
	t.Run("RevealOrderData instantiation", func(t *testing.T) {
		revealOrder := RevealOrderData{
			UniqueKey:    "1-0",
			Round:        "1",
			TrialNum:     "0",
			OrderedNodes: []string{"0x1111", "0x2222", "0x3333"},
			RevealOrder:  []int{2, 0, 1},
			RV:           "0xabcdef123456",
		}
		assert.Equal(t, "1", revealOrder.Round)
		assert.Equal(t, "0", revealOrder.TrialNum)
		assert.Equal(t, "1-0", revealOrder.UniqueKey)
		assert.Equal(t, 3, len(revealOrder.OrderedNodes))
		assert.Equal(t, []int{2, 0, 1}, revealOrder.RevealOrder)
		assert.Equal(t, "0xabcdef123456", revealOrder.RV)
	})
}
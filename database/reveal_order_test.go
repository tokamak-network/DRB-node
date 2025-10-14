package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestRevealOrderCRUD(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	round := "test_round"
	trialNum := "test_trial"

	// Clean up before test
	_, err := GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
	assert.NoError(t, err)

	// Prepare test data
	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"node1", "node2", "node3"},
		RevealOrder:  []int{2, 0, 1},
		RV:           "some-rv-string",
	}

	// Test AddRevealOrder
	err = AddRevealOrder(revealOrder)
	assert.NoError(t, err)

	// Test GetRevealOrder
	fetched, err := GetRevealOrder(round, trialNum)
	assert.NoError(t, err)
	assert.NotNil(t, fetched)
	assert.Equal(t, revealOrder.Round, fetched.Round)
	assert.Equal(t, revealOrder.TrialNum, fetched.TrialNum)
	assert.Equal(t, revealOrder.OrderedNodes, fetched.OrderedNodes)
	assert.Equal(t, revealOrder.RevealOrder, fetched.RevealOrder)
	assert.Equal(t, revealOrder.RV, fetched.RV)

	// Clean up after test
	_, err = GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
	assert.NoError(t, err)
}

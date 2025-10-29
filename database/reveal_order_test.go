package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestRevealOrderRepository_AddAndGet(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "test_round"
	trialNum := "test_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2", "0xnode3"},
		RevealOrder:  []int{0, 1, 2},
		RV:           "random_value_123",
	}

	// Add
	err := repo.AddRevealOrder(orderData)
	assert.NoError(t, err)

	// Get
	fetched, err := repo.GetRevealOrder(round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.Equal(t, 3, len(fetched.OrderedNodes))
	assert.Equal(t, 3, len(fetched.RevealOrder))
	assert.Equal(t, orderData.RV, fetched.RV)

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRevealOrderRepository_AddWithEmptyFields(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	// Empty round
	emptyRound := &utils.RevealOrderData{
		Round:        "",
		TrialNum:     "trial1",
		OrderedNodes: []string{"0xnode1"},
		RevealOrder:  []int{0},
		RV:           "rv1",
	}
	err := repo.AddRevealOrder(emptyRound)
	assert.Error(t, err, "Should reject empty round")

	emptyTrial := &utils.RevealOrderData{
		Round:        "round1",
		TrialNum:     "",
		OrderedNodes: []string{"0xnode1"},
		RevealOrder:  []int{0},
		RV:           "rv1",
	}
	err = repo.AddRevealOrder(emptyTrial)
	assert.Error(t, err, "Should reject empty trialNum")

	// Empty RV
	emptyRV := &utils.RevealOrderData{
		Round:        "round1",
		TrialNum:     "trial1",
		OrderedNodes: []string{"0xnode1"},
		RevealOrder:  []int{0},
		RV:           "",
	}
	err = repo.AddRevealOrder(emptyRV)
	assert.Error(t, err, "Should reject empty RV")
}

func TestRevealOrderRepository_GetNonExistent(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	_, err := repo.GetRevealOrder("nonexistent", "nonexistent")
	assert.Error(t, err, "Should return error for non-existent")
}

func TestRevealOrderRepository_MismatchedArrayLengths(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "mismatch_round"
	trialNum := "mismatch_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2", "0xnode3"},
		RevealOrder:  []int{0, 1},
		RV:           "rv_mismatch",
	}

	err := repo.AddRevealOrder(orderData)
	assert.Error(t, err, "Should reject mismatched array lengths")
	assert.Contains(t, err.Error(), "must match")

	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRevealOrderRepository_LargeNodeArrays(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "large_round"
	trialNum := "large_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	nodes := make([]string, 100)
	order := make([]int, 100)
	for i := 0; i < 100; i++ {
		nodes[i] = "0xnode" + string(rune(i))
		order[i] = i
	}

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: nodes,
		RevealOrder:  order,
		RV:           "rv_large",
	}

	err := repo.AddRevealOrder(orderData)
	assert.NoError(t, err)

	fetched, err := repo.GetRevealOrder(round, trialNum)
	assert.NoError(t, err)
	assert.Len(t, fetched.OrderedNodes, 100)
	assert.Len(t, fetched.RevealOrder, 100)

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRevealOrderRepository_SpecialCharacters(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "special_round"
	trialNum := "special_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0x'node", "0x\"node", "0x\\node"},
		RevealOrder:  []int{0, 1, 2},
		RV:           "rv_special_'\"\\",
	}

	err := repo.AddRevealOrder(orderData)
	assert.NoError(t, err)

	fetched, err := repo.GetRevealOrder(round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, orderData.OrderedNodes, fetched.OrderedNodes)
	assert.Equal(t, orderData.RV, fetched.RV)

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRevealOrderRepository_DuplicateIndices(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "dup_indices_round"
	trialNum := "dup_indices_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2", "0xnode3"},
		RevealOrder:  []int{0, 0, 0}, // Duplicate indices - should be rejected
		RV:           "rv_dup",
	}

	// properly rejects duplicate indices
	err := repo.AddRevealOrder(orderData)
	assert.Error(t, err, "Should reject duplicate indices")
	assert.Contains(t, err.Error(), "duplicate index")

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRevealOrderRepository_NegativeIndices(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "neg_round"
	trialNum := "neg_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2"},
		RevealOrder:  []int{-1, 0}, // Negative indices
		RV:           "rv_neg",
	}

	// rejects negative indices
	err := repo.AddRevealOrder(orderData)
	assert.Error(t, err, "Should reject negative indices")
	assert.Contains(t, err.Error(), "negative index")

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRevealOrderRepository_EmptyArrays(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "empty_round"
	trialNum := "empty_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{},
		RevealOrder:  []int{},
		RV:           "rv_empty",
	}

	err := repo.AddRevealOrder(orderData)
	assert.Error(t, err, "Should reject empty arrays")
	assert.Contains(t, err.Error(), "cannot be empty")

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

// Test out-of-bounds indices in reveal order
func TestRevealOrderRepository_OutOfBoundsIndices(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "bounds_round"
	trialNum := "bounds_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	// Test with index greater than maxIndex
	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2", "0xnode3"}, // 3 nodes (indices 0-2)
		RevealOrder:  []int{0, 1, 5},                            // Index 5 is out of bounds!
		RV:           "rv_bounds",
	}

	err := repo.AddRevealOrder(orderData)
	assert.Error(t, err, "Should reject out-of-bounds indices")
	assert.Contains(t, err.Error(), "out-of-bounds")
	assert.Contains(t, err.Error(), "max: 2")

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

// Test with only empty orderedNodes (to cover that specific check at line 32)
func TestRevealOrderRepository_EmptyOrderedNodes(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	orderData := &utils.RevealOrderData{
		Round:        "test_round",
		TrialNum:     "test_trial",
		OrderedNodes: []string{},     // Empty
		RevealOrder:  []int{0, 1, 2}, // Not empty
		RV:           "rv_test",
	}

	err := repo.AddRevealOrder(orderData)
	assert.Error(t, err, "Should reject empty orderedNodes")
	assert.Contains(t, err.Error(), "orderedNodes cannot be empty")
}

// Test with only empty revealOrder (to cover that specific check at line 35)
func TestRevealOrderRepository_EmptyRevealOrder(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	orderData := &utils.RevealOrderData{
		Round:        "test_round",
		TrialNum:     "test_trial",
		OrderedNodes: []string{"node1", "node2"}, // Not empty
		RevealOrder:  []int{},                    // Empty
		RV:           "rv_test",
	}

	err := repo.AddRevealOrder(orderData)
	assert.Error(t, err, "Should reject empty revealOrder")
	assert.Contains(t, err.Error(), "revealOrder cannot be empty")
}

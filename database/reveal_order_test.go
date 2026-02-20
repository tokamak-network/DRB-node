package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestRevealOrderRepository_AddAndGet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "test_round"
	trialNum := "test_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2", "0xnode3"},
		RevealOrder:  []int{0, 1, 2},
		RV:           "random_value_123",
	}

	// Add
	err := repo.AddRevealOrder(ctx, orderData)
	assert.NoError(t, err)

	// Get
	fetched, err := repo.GetRevealOrder(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.Equal(t, 3, len(fetched.OrderedNodes))
	assert.Equal(t, 3, len(fetched.RevealOrder))
	assert.Equal(t, orderData.RV, fetched.RV)

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRevealOrderRepository_AddWithEmptyFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	err := repo.AddRevealOrder(ctx, emptyRound)
	assert.Error(t, err, "Should reject empty round")

	emptyTrial := &utils.RevealOrderData{
		Round:        "round1",
		TrialNum:     "",
		OrderedNodes: []string{"0xnode1"},
		RevealOrder:  []int{0},
		RV:           "rv1",
	}
	err = repo.AddRevealOrder(ctx, emptyTrial)
	assert.Error(t, err, "Should reject empty trialNum")

	// Empty RV
	emptyRV := &utils.RevealOrderData{
		Round:        "round1",
		TrialNum:     "trial1",
		OrderedNodes: []string{"0xnode1"},
		RevealOrder:  []int{0},
		RV:           "",
	}
	err = repo.AddRevealOrder(ctx, emptyRV)
	assert.Error(t, err, "Should reject empty RV")
}

func TestRevealOrderRepository_GetNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	_, err := repo.GetRevealOrder(ctx, "nonexistent", "nonexistent")
	assert.Error(t, err, "Should return error for non-existent")
}

func TestRevealOrderRepository_MismatchedArrayLengths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "mismatch_round"
	trialNum := "mismatch_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2", "0xnode3"},
		RevealOrder:  []int{0, 1},
		RV:           "rv_mismatch",
	}

	err := repo.AddRevealOrder(ctx, orderData)
	assert.Error(t, err, "Should reject mismatched array lengths")
	assert.Contains(t, err.Error(), "must match")

	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRevealOrderRepository_LargeNodeArrays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "large_round"
	trialNum := "large_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
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

	err := repo.AddRevealOrder(ctx, orderData)
	assert.NoError(t, err)

	fetched, err := repo.GetRevealOrder(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.Len(t, fetched.OrderedNodes, 100)
	assert.Len(t, fetched.RevealOrder, 100)

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRevealOrderRepository_SpecialCharacters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "special_round"
	trialNum := "special_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0x'node", "0x\"node", "0x\\node"},
		RevealOrder:  []int{0, 1, 2},
		RV:           "rv_special_'\"\\",
	}

	err := repo.AddRevealOrder(ctx, orderData)
	assert.NoError(t, err)

	fetched, err := repo.GetRevealOrder(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, orderData.OrderedNodes, fetched.OrderedNodes)
	assert.Equal(t, orderData.RV, fetched.RV)

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRevealOrderRepository_DuplicateIndices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "dup_indices_round"
	trialNum := "dup_indices_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2", "0xnode3"},
		RevealOrder:  []int{0, 0, 0}, // Duplicate indices - should be rejected
		RV:           "rv_dup",
	}

	// properly rejects duplicate indices
	err := repo.AddRevealOrder(ctx, orderData)
	assert.Error(t, err, "Should reject duplicate indices")
	assert.Contains(t, err.Error(), "duplicate index")

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRevealOrderRepository_NegativeIndices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "neg_round"
	trialNum := "neg_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2"},
		RevealOrder:  []int{-1, 0}, // Negative indices
		RV:           "rv_neg",
	}

	// rejects negative indices
	err := repo.AddRevealOrder(ctx, orderData)
	assert.Error(t, err, "Should reject negative indices")
	assert.Contains(t, err.Error(), "negative index")

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRevealOrderRepository_EmptyArrays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "empty_round"
	trialNum := "empty_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{},
		RevealOrder:  []int{},
		RV:           "rv_empty",
	}

	err := repo.AddRevealOrder(ctx, orderData)
	assert.Error(t, err, "Should reject empty arrays")
	assert.Contains(t, err.Error(), "cannot be empty")

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test out-of-bounds indices in reveal order
func TestRevealOrderRepository_OutOfBoundsIndices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "bounds_round"
	trialNum := "bounds_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Test with index greater than maxIndex
	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2", "0xnode3"}, // 3 nodes (indices 0-2)
		RevealOrder:  []int{0, 1, 5},                            // Index 5 is out of bounds!
		RV:           "rv_bounds",
	}

	err := repo.AddRevealOrder(ctx, orderData)
	assert.Error(t, err, "Should reject out-of-bounds indices")
	assert.Contains(t, err.Error(), "out-of-bounds")
	assert.Contains(t, err.Error(), "max: 2")

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test with only empty orderedNodes (to cover that specific check at line 32)
func TestRevealOrderRepository_EmptyOrderedNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	orderData := &utils.RevealOrderData{
		Round:        "test_round",
		TrialNum:     "test_trial",
		OrderedNodes: []string{},     // Empty
		RevealOrder:  []int{0, 1, 2}, // Not empty
		RV:           "rv_test",
	}

	err := repo.AddRevealOrder(ctx, orderData)
	assert.Error(t, err, "Should reject empty orderedNodes")
	assert.Contains(t, err.Error(), "orderedNodes cannot be empty")
}

// Test with only empty revealOrder (to cover that specific check at line 35)
func TestRevealOrderRepository_EmptyRevealOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	orderData := &utils.RevealOrderData{
		Round:        "test_round",
		TrialNum:     "test_trial",
		OrderedNodes: []string{"node1", "node2"}, // Not empty
		RevealOrder:  []int{},                    // Empty
		RV:           "rv_test",
	}

	err := repo.AddRevealOrder(ctx, orderData)
	assert.Error(t, err, "Should reject empty revealOrder")
	assert.Contains(t, err.Error(), "revealOrder cannot be empty")
}

// Test NewRevealOrderRepository constructor
func TestNewRevealOrderRepository(t *testing.T) {
	db := GetDB()
	repo := NewRevealOrderRepository(db)
	assert.NotNil(t, repo, "Repository should be created")
	assert.Equal(t, db, repo.db, "Repository should store the database connection")
}

func TestRevealOrderRepository_AddRevealOrder_ContextTimeout(t *testing.T) {
	repo := NewRevealOrderRepository(GetDB())

	// Create a context that will timeout immediately
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	orderData := &utils.RevealOrderData{
		Round:        "timeout_round",
		TrialNum:     "timeout_trial",
		OrderedNodes: []string{"0xnode1", "0xnode2"},
		RevealOrder:  []int{0, 1},
		RV:           "rv_timeout",
	}

	// Lock the table to ensure timeout
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec("LOCK TABLE reveal_order_schemes IN ACCESS EXCLUSIVE MODE")
	assert.NoError(t, err)

	// Try to add reveal order with expired context
	err = repo.AddRevealOrder(ctx, orderData)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "i/o timeout"), "Error should indicate timeout")
}

func TestRevealOrderRepository_GetRevealOrder_ContextTimeout(t *testing.T) {
	repo := NewRevealOrderRepository(GetDB())

	// Create a context that will timeout immediately
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	// Lock the table to ensure timeout
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec("LOCK TABLE reveal_order_schemes IN ACCESS EXCLUSIVE MODE")
	assert.NoError(t, err)

	// Try to get reveal order with expired context
	_, err = repo.GetRevealOrder(ctx, "timeout_round", "timeout_trial")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "i/o timeout"), "Error should indicate timeout")
}

func TestRevealOrderRepository_ConcurrentOperations(t *testing.T) {
	repo := NewRevealOrderRepository(GetDB())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Cleanup before test
	for i := 0; i < numGoroutines; i++ {
		round := fmt.Sprintf("concurrent_round_%d", i)
		trialNum := fmt.Sprintf("concurrent_trial_%d", i)
		GetDB().Model(&RevealOrderScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()
	}

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()

			round := fmt.Sprintf("concurrent_round_%d", idx)
			trialNum := fmt.Sprintf("concurrent_trial_%d", idx)

			orderData := &utils.RevealOrderData{
				Round:        round,
				TrialNum:     trialNum,
				OrderedNodes: []string{fmt.Sprintf("0xnode%d", idx), fmt.Sprintf("0xnode%d", idx+1)},
				RevealOrder:  []int{0, 1},
				RV:           fmt.Sprintf("rv_concurrent_%d", idx),
			}

			err := repo.AddRevealOrder(ctx, orderData)
			assert.NoError(t, err)

			// Try to get it
			fetched, err := repo.GetRevealOrder(ctx, round, trialNum)
			assert.NoError(t, err)
			assert.Equal(t, round, fetched.Round)
			assert.Equal(t, trialNum, fetched.TrialNum)
			assert.Equal(t, 2, len(fetched.OrderedNodes))
		}(i)
	}

	wg.Wait()

	// Cleanup after test
	for i := 0; i < numGoroutines; i++ {
		round := fmt.Sprintf("concurrent_round_%d", i)
		trialNum := fmt.Sprintf("concurrent_trial_%d", i)
		GetDB().Model(&RevealOrderScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()
	}
}

func TestRevealOrderRepository_UnicodeCharacters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	unicodeRounds := []string{
		"round_中文",
		"round_日本語",
		"round_한국어",
		"round_русский",
		"round_العربية",
		"round_עברית",
		"round_🎉🎊",
	}

	for _, round := range unicodeRounds {
		trialNum := "unicode_trial"

		// Cleanup
		GetDB().Model(&RevealOrderScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()

		orderData := &utils.RevealOrderData{
			Round:        round,
			TrialNum:     trialNum,
			OrderedNodes: []string{"0xnode1", "0xnode2"},
			RevealOrder:  []int{0, 1},
			RV:           fmt.Sprintf("rv_unicode_%s", round),
		}

		err := repo.AddRevealOrder(ctx, orderData)
		assert.NoError(t, err, fmt.Sprintf("Should handle unicode characters in round: %s", round))

		fetched, err := repo.GetRevealOrder(ctx, round, trialNum)
		assert.NoError(t, err)
		assert.Equal(t, round, fetched.Round)
		assert.Equal(t, orderData.RV, fetched.RV)

		// Cleanup
		GetDB().Model(&RevealOrderScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()
	}
}

func TestRevealOrderRepository_AddRevealOrder_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	// Test with extremely long round name (should be handled by DB constraints)
	longRound := strings.Repeat("a", 1000)
	orderData := &utils.RevealOrderData{
		Round:        longRound,
		TrialNum:     "trial",
		OrderedNodes: []string{"0xnode1"},
		RevealOrder:  []int{0},
		RV:           "rv_test",
	}

	// This might succeed or fail depending on DB schema, but should not panic
	assert.NotPanics(t, func() {
		_ = repo.AddRevealOrder(ctx, orderData)
	})
}

func TestRevealOrderRepository_GetRevealOrder_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	// Test with empty strings
	_, err := repo.GetRevealOrder(ctx, "", "")
	assert.Error(t, err, "Should return error for empty round and trialNum")

	// Test with extremely long strings
	longRound := strings.Repeat("c", 1000)
	longTrial := strings.Repeat("d", 1000)
	_, err = repo.GetRevealOrder(ctx, longRound, longTrial)
	// Should return error (not found) but not panic
	assert.Error(t, err)
}

func TestRevealOrderRepository_GetRevealOrder_VeryLargeDataset(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "large_dataset_round"
	trialNum := "large_dataset_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Add a reveal order
	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2", "0xnode3"},
		RevealOrder:  []int{0, 1, 2},
		RV:           "rv_large_dataset",
	}

	err := repo.AddRevealOrder(ctx, orderData)
	assert.NoError(t, err)

	// Add many other reveal orders to create a large dataset
	for i := 0; i < 100; i++ {
		otherRound := fmt.Sprintf("other_round_%d", i)
		otherTrialNum := fmt.Sprintf("other_trial_%d", i)

		// Cleanup
		GetDB().Model(&RevealOrderScheme{}).
			Where("round = ? AND trial_num = ?", otherRound, otherTrialNum).
			Context(ctx).
			Delete()

		otherOrderData := &utils.RevealOrderData{
			Round:        otherRound,
			TrialNum:     otherTrialNum,
			OrderedNodes: []string{"0xnode1", "0xnode2"},
			RevealOrder:  []int{0, 1},
			RV:           fmt.Sprintf("rv_other_%d", i),
		}

		err := repo.AddRevealOrder(ctx, otherOrderData)
		assert.NoError(t, err)
	}

	// Try to get the original reveal order
	fetched, err := repo.GetRevealOrder(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.Equal(t, "rv_large_dataset", fetched.RV)

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ?", round).
		Context(ctx).
		Delete()

	// Cleanup other rounds
	for i := 0; i < 100; i++ {
		otherRound := fmt.Sprintf("other_round_%d", i)
		otherTrialNum := fmt.Sprintf("other_trial_%d", i)
		GetDB().Model(&RevealOrderScheme{}).
			Where("round = ? AND trial_num = ?", otherRound, otherTrialNum).
			Context(ctx).
			Delete()
	}
}

func TestRevealOrderRepository_AddRevealOrder_VeryLargeArrays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "very_large_arrays_round"
	trialNum := "very_large_arrays_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Create very large arrays (1000 nodes)
	nodes := make([]string, 1000)
	order := make([]int, 1000)
	for i := 0; i < 1000; i++ {
		nodes[i] = fmt.Sprintf("0xnode%d", i)
		order[i] = i
	}

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: nodes,
		RevealOrder:  order,
		RV:           "rv_very_large",
	}

	err := repo.AddRevealOrder(ctx, orderData)
	assert.NoError(t, err)

	fetched, err := repo.GetRevealOrder(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, 1000, len(fetched.OrderedNodes))
	assert.Equal(t, 1000, len(fetched.RevealOrder))
	assert.Equal(t, nodes, fetched.OrderedNodes)
	assert.Equal(t, order, fetched.RevealOrder)

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRevealOrderRepository_GetRevealOrder_MultipleOrders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	// Add multiple reveal orders with different trial numbers
	round := "multi_order_round"
	trialNums := []string{"trial1", "trial2", "trial3"}

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ?", round).
		Context(ctx).
		Delete()

	for i, trialNum := range trialNums {
		orderData := &utils.RevealOrderData{
			Round:        round,
			TrialNum:     trialNum,
			OrderedNodes: []string{fmt.Sprintf("0xnode%d", i), fmt.Sprintf("0xnode%d", i+1)},
			RevealOrder:  []int{0, 1},
			RV:           fmt.Sprintf("rv_%s", trialNum),
		}

		err := repo.AddRevealOrder(ctx, orderData)
		assert.NoError(t, err)

		// Verify each reveal order can be retrieved
		fetched, err := repo.GetRevealOrder(ctx, round, trialNum)
		assert.NoError(t, err)
		assert.Equal(t, round, fetched.Round)
		assert.Equal(t, trialNum, fetched.TrialNum)
		assert.Equal(t, fmt.Sprintf("rv_%s", trialNum), fetched.RV)
	}

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ?", round).
		Context(ctx).
		Delete()
}

func TestRevealOrderRepository_AddRevealOrder_DuplicateKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	round := "dup_key_round"
	trialNum := "dup_key_trial"

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	orderData := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode1", "0xnode2"},
		RevealOrder:  []int{0, 1},
		RV:           "rv_dup",
	}

	err := repo.AddRevealOrder(ctx, orderData)
	assert.NoError(t, err)

	// Try to add duplicate
	err = repo.AddRevealOrder(ctx, orderData)
	assert.Error(t, err, "Should reject duplicate composite key")

	// Cleanup
	GetDB().Model(&RevealOrderScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRevealOrderRepository_AddRevealOrder_AllValidationPaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRevealOrderRepository(GetDB())

	// Test all validation error paths
	testCases := []struct {
		name      string
		orderData *utils.RevealOrderData
		errorMsg  string
	}{
		{
			name: "empty round",
			orderData: &utils.RevealOrderData{
				Round:        "",
				TrialNum:     "trial",
				OrderedNodes: []string{"0xnode1"},
				RevealOrder:  []int{0},
				RV:           "rv",
			},
			errorMsg: "round cannot be empty",
		},
		{
			name: "empty trialNum",
			orderData: &utils.RevealOrderData{
				Round:        "round",
				TrialNum:     "",
				OrderedNodes: []string{"0xnode1"},
				RevealOrder:  []int{0},
				RV:           "rv",
			},
			errorMsg: "trialNum cannot be empty",
		},
		{
			name: "empty RV",
			orderData: &utils.RevealOrderData{
				Round:        "round",
				TrialNum:     "trial",
				OrderedNodes: []string{"0xnode1"},
				RevealOrder:  []int{0},
				RV:           "",
			},
			errorMsg: "RV cannot be empty",
		},
		{
			name: "empty orderedNodes",
			orderData: &utils.RevealOrderData{
				Round:        "round",
				TrialNum:     "trial",
				OrderedNodes: []string{},
				RevealOrder:  []int{0},
				RV:           "rv",
			},
			errorMsg: "orderedNodes cannot be empty",
		},
		{
			name: "empty revealOrder",
			orderData: &utils.RevealOrderData{
				Round:        "round",
				TrialNum:     "trial",
				OrderedNodes: []string{"0xnode1"},
				RevealOrder:  []int{},
				RV:           "rv",
			},
			errorMsg: "revealOrder cannot be empty",
		},
		{
			name: "mismatched lengths",
			orderData: &utils.RevealOrderData{
				Round:        "round",
				TrialNum:     "trial",
				OrderedNodes: []string{"0xnode1", "0xnode2"},
				RevealOrder:  []int{0},
				RV:           "rv",
			},
			errorMsg: "must match",
		},
		{
			name: "negative index",
			orderData: &utils.RevealOrderData{
				Round:        "round",
				TrialNum:     "trial",
				OrderedNodes: []string{"0xnode1"},
				RevealOrder:  []int{-1},
				RV:           "rv",
			},
			errorMsg: "negative index",
		},
		{
			name: "out of bounds index",
			orderData: &utils.RevealOrderData{
				Round:        "round",
				TrialNum:     "trial",
				OrderedNodes: []string{"0xnode1"},
				RevealOrder:  []int{5},
				RV:           "rv",
			},
			errorMsg: "out-of-bounds",
		},
		{
			name: "duplicate index",
			orderData: &utils.RevealOrderData{
				Round:        "round",
				TrialNum:     "trial",
				OrderedNodes: []string{"0xnode1", "0xnode2"},
				RevealOrder:  []int{0, 0},
				RV:           "rv",
			},
			errorMsg: "duplicate index",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := repo.AddRevealOrder(ctx, tc.orderData)
			assert.Error(t, err, fmt.Sprintf("Should reject %s", tc.name))
			assert.Contains(t, err.Error(), tc.errorMsg, "Error message should contain expected text")
		})
	}
}

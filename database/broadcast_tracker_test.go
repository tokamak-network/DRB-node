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

func TestBroadcastTrackerRepository_CRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "test_round"
	trialNum := "test_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	tracker := &utils.BroadcastTracker{
		Round:        round,
		TrialNum:     trialNum,
		EOAAddress:   "0xtest",
		Type:         "cvs",
		MessageID:    "msg123",
		Data:         [32]byte{0x01, 0x02},
		Attempts:     1,
		MaxAttempts:  3,
		Acknowledged: map[string]bool{"0xpeer1": true},
		LastSent:     time.Now().Unix(),
		Timeout:      time.Now().Unix() + 60,
	}

	// Add
	err := repo.AddBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Get
	trackers, err := repo.GetBroadcastTrackers(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, trackers)

	// Update
	tracker.Attempts = 2
	err = repo.UpdateBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Delete
	err = repo.DeleteBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestBroadcastTrackerRepository_AddWithEmptyFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// Empty round
	emptyRound := &utils.BroadcastTracker{
		Round:      "",
		TrialNum:   "trial1",
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "msg1",
	}
	err := repo.AddBroadcastTracker(ctx, emptyRound)
	assert.Error(t, err, "Should reject empty round")

	// Empty type
	emptyType := &utils.BroadcastTracker{
		Round:      "round1",
		TrialNum:   "trial1",
		EOAAddress: "0xtest",
		Type:       "",
		MessageID:  "msg1",
	}
	err = repo.AddBroadcastTracker(ctx, emptyType)
	assert.Error(t, err, "Should reject empty type")

	// Empty messageID
	emptyMsgID := &utils.BroadcastTracker{
		Round:      "round1",
		TrialNum:   "trial1",
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "",
	}
	err = repo.AddBroadcastTracker(ctx, emptyMsgID)
	assert.Error(t, err, "Should reject empty messageID")
}

func TestBroadcastTrackerRepository_UpdateNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	tracker := &utils.BroadcastTracker{
		Round:      "nonexistent",
		TrialNum:   "nonexistent",
		EOAAddress: "0xnonexistent",
		Type:       "cvs",
		MessageID:  "nonexistent",
	}

	err := repo.UpdateBroadcastTracker(ctx, tracker)
	assert.NoError(t, err, "Update succeeds but updates 0 rows")
}

func TestBroadcastTrackerRepository_DeleteNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	tracker := &utils.BroadcastTracker{
		Round:      "nonexistent",
		TrialNum:   "nonexistent",
		EOAAddress: "0xnonexistent",
		Type:       "cvs",
		MessageID:  "nonexistent",
	}

	err := repo.DeleteBroadcastTracker(ctx, tracker)
	assert.NoError(t, err, "Delete succeeds but deletes 0 rows")
}

func TestBroadcastTrackerRepository_JSONBMaps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "jsonb_round"
	trialNum := "jsonb_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	tracker := &utils.BroadcastTracker{
		Round:        round,
		TrialNum:     trialNum,
		EOAAddress:   "0xjsonb",
		Type:         "cvs",
		MessageID:    "msg_jsonb",
		Acknowledged: map[string]bool{"0xpeer1": true, "0xpeer2": false, "0xpeer3": true},
	}

	err := repo.AddBroadcastTracker(ctx, tracker)
	assert.NoError(t, err, "Should store JSONB maps")

	// Verify can retrieve
	trackers, err := repo.GetBroadcastTrackers(ctx)
	assert.NoError(t, err)

	found := false
	for _, tr := range trackers {
		if tr.MessageID == "msg_jsonb" {
			found = true
			assert.NotNil(t, tr.Acknowledged)
			assert.True(t, tr.Acknowledged["0xpeer1"])
			assert.False(t, tr.Acknowledged["0xpeer2"])
			break
		}
	}
	assert.True(t, found, "Should find tracker with JSONB map")

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestBroadcastTrackerRepository_AttemptsAndTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "attempts_round"
	trialNum := "attempts_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	nowUnix := time.Now().Unix()

	tracker := &utils.BroadcastTracker{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  "0xattempts",
		Type:        "cvs",
		MessageID:   "msg_attempts",
		Attempts:    0,
		MaxAttempts: 5,
		LastSent:    nowUnix,
		Timeout:     nowUnix + 120,
	}

	err := repo.AddBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Increment attempts
	tracker.Attempts = 1
	tracker.LastSent = nowUnix + 10
	err = repo.UpdateBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	tracker.Attempts = 2
	tracker.LastSent = nowUnix + 20
	err = repo.UpdateBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestBroadcastTrackerRepository_MultipleTrackers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "multi_round"
	trialNum := "multi_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	trackers := []*utils.BroadcastTracker{
		{Round: round, TrialNum: trialNum, EOAAddress: "0x1", Type: "cvs", MessageID: "msg1"},
		{Round: round, TrialNum: trialNum, EOAAddress: "0x2", Type: "cos", MessageID: "msg2"},
		{Round: round, TrialNum: trialNum, EOAAddress: "0x3", Type: "secret", MessageID: "msg3"},
	}

	for _, tracker := range trackers {
		err := repo.AddBroadcastTracker(ctx, tracker)
		assert.NoError(t, err)
	}

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestBroadcastTrackerRepository_AddAll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "batch_round"
	trialNum := "batch_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	trackers := []*utils.BroadcastTracker{
		{Round: round, TrialNum: trialNum, EOAAddress: "0xbatch1", Type: "cvs", MessageID: "msg1"},
		{Round: round, TrialNum: trialNum, EOAAddress: "0xbatch2", Type: "cos", MessageID: "msg2"},
	}

	err := repo.AddAllBroadcastTrackers(ctx, trackers)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test error handling for GetBroadcastTrackers when database fails
func TestBroadcastTrackerRepository_GetBroadcastTrackers_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to get trackers - should get an error because table doesn't exist
	_, err = repo.GetBroadcastTrackers(ctx)
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for AddAllBroadcastTrackers when insert fails
func TestBroadcastTrackerRepository_AddAllBroadcastTrackers_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// Drop the table to force an error during batch insert
	_, err := GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to add trackers - should fail because table doesn't exist
	trackers := []*utils.BroadcastTracker{
		{Round: "test", TrialNum: "test", EOAAddress: "0xtest1", Type: "cvs", MessageID: "msg1"},
		{Round: "test", TrialNum: "test", EOAAddress: "0xtest2", Type: "cvs", MessageID: "msg2"},
	}

	err = repo.AddAllBroadcastTrackers(ctx, trackers)
	assert.Error(t, err, "Expected error when table is missing")
	assert.Contains(t, err.Error(), "failed to add broadcast tracker data", "Error should contain expected message")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for AddBroadcastTracker when insert fails
func TestBroadcastTrackerRepository_AddBroadcastTracker_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to add tracker - should fail because table doesn't exist
	tracker := &utils.BroadcastTracker{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "msg1",
		Data:       [32]byte{0x01},
	}

	err = repo.AddBroadcastTracker(ctx, tracker)
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for UpdateBroadcastTracker when update fails
func TestBroadcastTrackerRepository_UpdateBroadcastTracker_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to update tracker - should fail because table doesn't exist
	tracker := &utils.BroadcastTracker{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "msg1",
		Data:       [32]byte{0x01},
	}

	err = repo.UpdateBroadcastTracker(ctx, tracker)
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for DeleteBroadcastTracker when delete fails
func TestBroadcastTrackerRepository_DeleteBroadcastTracker_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to delete tracker - should fail because table doesn't exist
	tracker := &utils.BroadcastTracker{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "msg1",
	}

	err = repo.DeleteBroadcastTracker(ctx, tracker)
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test mapping functions with edge cases
func TestBroadcastTrackerRepository_MapFunctions_EdgeCases(t *testing.T) {
	repo := NewBroadcastTrackerRepository(GetDB())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	round := "map_test_round"
	trialNum := "map_test_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Test with empty data array
	tracker := &utils.BroadcastTracker{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "msg1",
		Data:       [32]byte{}, // Empty byte array
	}

	err := repo.AddBroadcastTracker(ctx, tracker)
	assert.NoError(t, err, "Should handle empty data array")

	// Test with nil acknowledged map
	tracker2 := &utils.BroadcastTracker{
		Round:        round,
		TrialNum:     trialNum,
		EOAAddress:   "0xtest2",
		Type:         "cos",
		MessageID:    "msg2",
		Data:         [32]byte{0x01},
		Acknowledged: nil, // nil map
	}

	err = repo.AddBroadcastTracker(ctx, tracker2)
	assert.NoError(t, err, "Should handle nil acknowledged map")

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test NewBroadcastTrackerRepository constructor
func TestNewBroadcastTrackerRepository(t *testing.T) {
	db := GetDB()
	repo := NewBroadcastTrackerRepository(db)
	assert.NotNil(t, repo, "Repository should be created")
	assert.Equal(t, db, repo.db, "Repository should store the database connection")
}

// Test context timeout for AddBroadcastTracker
func TestBroadcastTrackerRepository_AddBroadcastTracker_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the insert operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE broadcast_tracker_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())
	tracker := &utils.BroadcastTracker{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "msg1",
		Data:       [32]byte{0x01},
	}

	// This call should block on the locked table and time out
	err = repo.AddBroadcastTracker(ctx, tracker)
	assert.Error(t, err, "Expected an error due to context timeout")
	assert.Contains(t, err.Error(), "i/o timeout", "Error should be related to i/o timeout")
}

// Test context timeout for GetBroadcastTrackers
func TestBroadcastTrackerRepository_GetBroadcastTrackers_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the select operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE broadcast_tracker_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// This call should block on the locked table and time out
	_, err = repo.GetBroadcastTrackers(ctx)
	assert.Error(t, err, "Expected an error due to context timeout")
	assert.Contains(t, err.Error(), "i/o timeout", "Error should be related to i/o timeout")
}

// Test context timeout for UpdateBroadcastTracker
func TestBroadcastTrackerRepository_UpdateBroadcastTracker_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the update operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE broadcast_tracker_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())
	tracker := &utils.BroadcastTracker{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "msg1",
		Data:       [32]byte{0x01},
	}

	// This call should block on the locked table and time out
	err = repo.UpdateBroadcastTracker(ctx, tracker)
	assert.Error(t, err, "Expected an error due to context timeout")
	assert.Contains(t, err.Error(), "i/o timeout", "Error should be related to i/o timeout")
}

// Test context timeout for DeleteBroadcastTracker
func TestBroadcastTrackerRepository_DeleteBroadcastTracker_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the delete operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE broadcast_tracker_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())
	tracker := &utils.BroadcastTracker{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "msg1",
	}

	// This call should block on the locked table and time out
	err = repo.DeleteBroadcastTracker(ctx, tracker)
	assert.Error(t, err, "Expected an error due to context timeout")
	assert.Contains(t, err.Error(), "i/o timeout", "Error should be related to i/o timeout")
}

// Test concurrent operations
func TestBroadcastTrackerRepository_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())
	round := "concurrent_test_round"
	trialNum := "concurrent_test_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Run concurrent add operations
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			tracker := &utils.BroadcastTracker{
				Round:      round,
				TrialNum:   trialNum,
				EOAAddress: fmt.Sprintf("0xconcurrent_%d", idx),
				Type:       "cvs",
				MessageID:  fmt.Sprintf("msg_%d", idx),
				Data:       [32]byte{byte(idx)},
			}
			err := repo.AddBroadcastTracker(ctx, tracker)
			done <- err
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		err := <-done
		assert.NoError(t, err, "Concurrent operations should not error")
	}

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test AddAllBroadcastTrackers with empty slice
func TestBroadcastTrackerRepository_AddAllBroadcastTrackers_EmptySlice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// Empty slice should not error
	err := repo.AddAllBroadcastTrackers(ctx, []*utils.BroadcastTracker{})
	assert.NoError(t, err, "Should handle empty slice gracefully")
}

// Test AddAllBroadcastTrackers with partial failure - when one tracker fails mid-batch
func TestBroadcastTrackerRepository_AddAllBroadcastTrackers_PartialFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())
	round := "partial_failure_round"
	trialNum := "partial_failure_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Add first tracker successfully
	tracker1 := &utils.BroadcastTracker{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: "0xpartial1",
		Type:       "cvs",
		MessageID:  "msg1",
		Data:       [32]byte{0x01},
	}
	err := repo.AddBroadcastTracker(ctx, tracker1)
	assert.NoError(t, err)

	// Drop the table to cause failure on second tracker
	_, err = GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to add multiple trackers - should fail on the second one
	trackers := []*utils.BroadcastTracker{
		{Round: round, TrialNum: trialNum, EOAAddress: "0xpartial2", Type: "cvs", MessageID: "msg2", Data: [32]byte{0x02}},
		{Round: round, TrialNum: trialNum, EOAAddress: "0xpartial3", Type: "cvs", MessageID: "msg3", Data: [32]byte{0x03}},
	}

	err = repo.AddAllBroadcastTrackers(ctx, trackers)
	assert.Error(t, err, "Should fail when table is missing")
	assert.Contains(t, err.Error(), "failed to add broadcast tracker data", "Error should contain expected message")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test AddAllBroadcastTrackers with large batch
func TestBroadcastTrackerRepository_AddAllBroadcastTrackers_LargeBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())
	round := "large_batch_round"
	trialNum := "large_batch_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Create large batch of trackers (100 records)
	numTrackers := 100
	trackers := make([]*utils.BroadcastTracker, 0, numTrackers)
	for i := 0; i < numTrackers; i++ {
		trackers = append(trackers, &utils.BroadcastTracker{
			Round:      round,
			TrialNum:   trialNum,
			EOAAddress: fmt.Sprintf("0xlarge_%d", i),
			Type:       "cvs",
			MessageID:  fmt.Sprintf("msg_%d", i),
			Data:       [32]byte{byte(i)},
		})
	}

	// Add all trackers
	err := repo.AddAllBroadcastTrackers(ctx, trackers)
	assert.NoError(t, err, "Should successfully add large batch")

	// Verify all were added
	allTrackers, err := repo.GetBroadcastTrackers(ctx)
	assert.NoError(t, err)
	count := 0
	for _, tr := range allTrackers {
		if tr.Round == round && tr.TrialNum == trialNum {
			count++
		}
	}
	assert.GreaterOrEqual(t, count, numTrackers, "Should have at least %d trackers", numTrackers)

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test with special characters in round/trial names to ensure SQL injection prevention
func TestBroadcastTrackerRepository_SpecialCharactersInRoundNames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// Test with various special characters that could be used in SQL injection attempts
	specialRounds := []string{
		"'; DROP TABLE broadcast_tracker_schemes; --",
		"'; DELETE FROM broadcast_tracker_schemes; --",
		"1' OR '1'='1",
		"1' UNION SELECT * FROM broadcast_tracker_schemes; --",
		"round'; DELETE FROM broadcast_tracker_schemes WHERE '1'='1",
		"round\" OR \"1\"=\"1",
		"round\\'; DROP TABLE broadcast_tracker_schemes; --",
	}

	for _, specialRound := range specialRounds {
		tracker := &utils.BroadcastTracker{
			Round:      specialRound,
			TrialNum:   "trial",
			EOAAddress: "0xtest",
			Type:       "cvs",
			MessageID:  "msg1",
			Data:       [32]byte{0x01},
		}

		// These should not cause SQL injection (parameterized queries should handle this)
		err := repo.AddBroadcastTracker(ctx, tracker)
		// May succeed or fail, but should not cause SQL injection
		if err != nil {
			// If it fails, it should be a validation/constraint error, not SQL injection
			assert.NotContains(t, err.Error(), "DROP TABLE", "Should not execute DROP TABLE")
			assert.NotContains(t, err.Error(), "DELETE FROM", "Should not execute DELETE FROM")
		}

		// Cleanup if it was added
		GetDB().Model(&BroadcastTrackerScheme{}).
			Where("round = ?", specialRound).
			Context(ctx).
			Delete()
	}
}

// Test with unicode and international characters in round/trial names
func TestBroadcastTrackerRepository_UnicodeCharactersInRoundNames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	// Test with unicode characters
	unicodeRounds := []string{
		"round_中文",
		"round_日本語",
		"round_한국어",
		"round_русский",
		"round_العربية",
		"round_עברית",
		"round_🎉🎊",
		"round_🚀",
		"round_with_émojis_🎯",
	}

	for _, unicodeRound := range unicodeRounds {
		tracker := &utils.BroadcastTracker{
			Round:      unicodeRound,
			TrialNum:   "trial",
			EOAAddress: "0xtest",
			Type:       "cvs",
			MessageID:  "msg1",
			Data:       [32]byte{0x01},
		}

		err := repo.AddBroadcastTracker(ctx, tracker)
		assert.NoError(t, err, "Should handle unicode characters: %s", unicodeRound)

		// Cleanup
		GetDB().Model(&BroadcastTrackerScheme{}).
			Where("round = ?", unicodeRound).
			Context(ctx).
			Delete()
	}
}

// Test GetBroadcastTrackers with very large dataset
func TestBroadcastTrackerRepository_GetBroadcastTrackers_VeryLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())
	round := "very_large_round"
	trialNum := "very_large_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Create very large dataset (1000 records)
	numTrackers := 1000
	for i := 0; i < numTrackers; i++ {
		tracker := &utils.BroadcastTracker{
			Round:      round,
			TrialNum:   trialNum,
			EOAAddress: fmt.Sprintf("0xlarge_%d", i),
			Type:       "cvs",
			MessageID:  fmt.Sprintf("msg_%d", i),
			Data:       [32]byte{byte(i % 256)},
		}
		err := repo.AddBroadcastTracker(ctx, tracker)
		if err != nil {
			t.Fatalf("Failed to add tracker %d: %v", i, err)
		}
	}

	// Get all trackers
	startTime := time.Now()
	trackers, err := repo.GetBroadcastTrackers(ctx)
	duration := time.Since(startTime)
	assert.NoError(t, err, "Should successfully retrieve very large dataset")

	// Log performance
	t.Logf("Retrieved %d trackers in %v", len(trackers), duration)

	// Verify we got at least our trackers
	count := 0
	for _, tr := range trackers {
		if tr.Round == round && tr.TrialNum == trialNum {
			count++
		}
	}
	assert.GreaterOrEqual(t, count, numTrackers, "Should have at least %d trackers", numTrackers)

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test UpdateBroadcastTracker with all fields updated
func TestBroadcastTrackerRepository_UpdateBroadcastTracker_AllFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())
	round := "update_all_round"
	trialNum := "update_all_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Add initial tracker
	nowUnix := time.Now().Unix()
	tracker := &utils.BroadcastTracker{
		Round:        round,
		TrialNum:     trialNum,
		EOAAddress:   "0xupdate",
		Type:         "cvs",
		MessageID:    "msg_update",
		Data:         [32]byte{0x01},
		Attempts:     1,
		MaxAttempts:  3,
		Acknowledged: map[string]bool{"0xpeer1": true},
		LastSent:     nowUnix,
		Timeout:      nowUnix + 60,
	}

	err := repo.AddBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Update all fields
	tracker.Data = [32]byte{0x02, 0x03}
	tracker.Attempts = 2
	tracker.MaxAttempts = 5
	tracker.Acknowledged = map[string]bool{"0xpeer1": true, "0xpeer2": false, "0xpeer3": true}
	tracker.LastSent = nowUnix + 10
	tracker.Timeout = nowUnix + 120

	err = repo.UpdateBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Verify update
	trackers, err := repo.GetBroadcastTrackers(ctx)
	assert.NoError(t, err)
	found := false
	for _, tr := range trackers {
		if tr.Round == round && tr.TrialNum == trialNum && tr.MessageID == "msg_update" {
			found = true
			assert.Equal(t, byte(0x02), tr.Data[0], "Data should be updated")
			assert.Equal(t, 2, tr.Attempts, "Attempts should be updated")
			assert.Equal(t, 5, tr.MaxAttempts, "MaxAttempts should be updated")
			assert.Equal(t, 3, len(tr.Acknowledged), "Acknowledged should be updated")
			break
		}
	}
	assert.True(t, found, "Should find updated tracker")

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

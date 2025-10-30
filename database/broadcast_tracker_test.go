package database

import (
	"context"
	"database/sql"
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

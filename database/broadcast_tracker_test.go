package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestBroadcastTrackerRepository_CRUD(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "test_round"
	trialNum := "test_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
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
	err := repo.AddBroadcastTracker(tracker)
	assert.NoError(t, err)

	// Get
	trackers, err := repo.GetBroadcastTrackers()
	assert.NoError(t, err)
	assert.NotEmpty(t, trackers)

	// Update
	tracker.Attempts = 2
	err = repo.UpdateBroadcastTracker(tracker)
	assert.NoError(t, err)

	// Delete
	err = repo.DeleteBroadcastTracker(tracker)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestBroadcastTrackerRepository_AddWithEmptyFields(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	err := repo.AddBroadcastTracker(emptyRound)
	assert.Error(t, err, "Should reject empty round")

	// Empty type
	emptyType := &utils.BroadcastTracker{
		Round:      "round1",
		TrialNum:   "trial1",
		EOAAddress: "0xtest",
		Type:       "",
		MessageID:  "msg1",
	}
	err = repo.AddBroadcastTracker(emptyType)
	assert.Error(t, err, "Should reject empty type")

	// Empty messageID
	emptyMsgID := &utils.BroadcastTracker{
		Round:      "round1",
		TrialNum:   "trial1",
		EOAAddress: "0xtest",
		Type:       "cvs",
		MessageID:  "",
	}
	err = repo.AddBroadcastTracker(emptyMsgID)
	assert.Error(t, err, "Should reject empty messageID")
}

func TestBroadcastTrackerRepository_UpdateNonExistent(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	tracker := &utils.BroadcastTracker{
		Round:      "nonexistent",
		TrialNum:   "nonexistent",
		EOAAddress: "0xnonexistent",
		Type:       "cvs",
		MessageID:  "nonexistent",
	}

	err := repo.UpdateBroadcastTracker(tracker)
	assert.NoError(t, err, "Update succeeds but updates 0 rows")
}

func TestBroadcastTrackerRepository_DeleteNonExistent(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	tracker := &utils.BroadcastTracker{
		Round:      "nonexistent",
		TrialNum:   "nonexistent",
		EOAAddress: "0xnonexistent",
		Type:       "cvs",
		MessageID:  "nonexistent",
	}

	err := repo.DeleteBroadcastTracker(tracker)
	assert.NoError(t, err, "Delete succeeds but deletes 0 rows")
}

func TestBroadcastTrackerRepository_JSONBMaps(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "jsonb_round"
	trialNum := "jsonb_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	tracker := &utils.BroadcastTracker{
		Round:        round,
		TrialNum:     trialNum,
		EOAAddress:   "0xjsonb",
		Type:         "cvs",
		MessageID:    "msg_jsonb",
		Acknowledged: map[string]bool{"0xpeer1": true, "0xpeer2": false, "0xpeer3": true},
	}

	err := repo.AddBroadcastTracker(tracker)
	assert.NoError(t, err, "Should store JSONB maps")

	// Verify can retrieve
	trackers, err := repo.GetBroadcastTrackers()
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
		Delete()
}

func TestBroadcastTrackerRepository_AttemptsAndTimeout(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "attempts_round"
	trialNum := "attempts_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
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

	err := repo.AddBroadcastTracker(tracker)
	assert.NoError(t, err)

	// Increment attempts
	tracker.Attempts = 1
	tracker.LastSent = nowUnix + 10
	err = repo.UpdateBroadcastTracker(tracker)
	assert.NoError(t, err)

	tracker.Attempts = 2
	tracker.LastSent = nowUnix + 20
	err = repo.UpdateBroadcastTracker(tracker)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestBroadcastTrackerRepository_MultipleTrackers(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "multi_round"
	trialNum := "multi_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	trackers := []*utils.BroadcastTracker{
		{Round: round, TrialNum: trialNum, EOAAddress: "0x1", Type: "cvs", MessageID: "msg1"},
		{Round: round, TrialNum: trialNum, EOAAddress: "0x2", Type: "cos", MessageID: "msg2"},
		{Round: round, TrialNum: trialNum, EOAAddress: "0x3", Type: "secret", MessageID: "msg3"},
	}

	for _, tracker := range trackers {
		err := repo.AddBroadcastTracker(tracker)
		assert.NoError(t, err)
	}

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestBroadcastTrackerRepository_AddAll(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBroadcastTrackerRepository(GetDB())

	round := "batch_round"
	trialNum := "batch_trial"

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	trackers := []*utils.BroadcastTracker{
		{Round: round, TrialNum: trialNum, EOAAddress: "0xbatch1", Type: "cvs", MessageID: "msg1"},
		{Round: round, TrialNum: trialNum, EOAAddress: "0xbatch2", Type: "cos", MessageID: "msg2"},
	}

	err := repo.AddAllBroadcastTrackers(trackers)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&BroadcastTrackerScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

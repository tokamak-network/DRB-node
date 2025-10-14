package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func createTestTracker() *utils.BroadcastTracker {
	return &utils.BroadcastTracker{
		Round:       "testRound1",
		TrialNum:    "trial1",
		EOAAddress:  "0x12345",
		Type:        "testType",
		MessageID:   "msg123",
		Data:        [32]byte{1, 2, 3}, // fixed size array
		Attempts:    1,
		MaxAttempts: 3,
		Acknowledged: map[string]bool{
			"0xabc": true,
		},
		LastSent: time.Now().Unix(),
		Timeout:  30,
	}
}

func TestBroadcastTrackerCRUD(t *testing.T) {
	tracker := createTestTracker()

	// Add tracker
	err := AddBroadcastTracker(tracker)
	assert.NoError(t, err, "AddBroadcastTracker should succeed")

	// Get trackers
	trackers, err := GetBroadcastTrackers()
	assert.NoError(t, err, "GetBroadcastTrackers should succeed")

	found := false
	for _, tr := range trackers {
		if tr.Round == tracker.Round && tr.MessageID == tracker.MessageID {
			found = true
			// Check key fields including map
			assert.Equal(t, tracker.Acknowledged, tr.Acknowledged, "Acknowledged map should match")
			break
		}
	}
	assert.True(t, found, "Added tracker should be found")

	// Update tracker: modify map and attempts
	tracker.Acknowledged["0xdef"] = true
	tracker.Attempts = 2

	err = UpdateBroadcastTracker(tracker)
	assert.NoError(t, err, "UpdateBroadcastTracker should succeed")

	// Verify update
	trackers, err = GetBroadcastTrackers()
	assert.NoError(t, err)
	updatedFound := false
	for _, tr := range trackers {
		if tr.Round == tracker.Round && tr.MessageID == tracker.MessageID {
			updatedFound = true
			assert.Equal(t, 2, tr.Attempts, "Attempts should be updated")
			assert.Equal(t, tracker.Acknowledged, tr.Acknowledged, "Acknowledged map should be updated")
			break
		}
	}
	assert.True(t, updatedFound, "Updated tracker should be found")

	// Delete tracker
	err = DeleteBroadcastTracker(tracker)
	assert.NoError(t, err, "DeleteBroadcastTracker should succeed")

	// Confirm deletion
	trackers, err = GetBroadcastTrackers()
	assert.NoError(t, err)
	for _, tr := range trackers {
		assert.False(t, tr.Round == tracker.Round && tr.MessageID == tracker.MessageID, "Deleted tracker should not be found")
	}
}

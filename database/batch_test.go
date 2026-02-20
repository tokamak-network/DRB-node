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

func TestBatchRepository_DeleteRoundTrialDataForLeaderNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())

	round := "delete_leader_round"
	trialNum := "delete_leader_trial"

	// Cleanup first
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&RevealOrderScheme{}).Where("round = ?", round).Delete(ctx)

	// Add test data
	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	leaderCommit := &utils.LeaderCommitData{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  "0xleader",
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
	}
	err := leaderRepo.AddLeaderCommit(ctx, leaderCommit)
	assert.NoError(t, err)

	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"node1"},
		RevealOrder:  []int{0},
		RV:           "rv_test",
	}
	err = revealRepo.AddRevealOrder(ctx, revealOrder)
	assert.NoError(t, err)

	// Delete
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, round, trialNum)
	assert.NoError(t, err)

	// Verify deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, "0xleader")
	assert.Error(t, err)
}

func TestBatchRepository_DeleteRoundTrialDataForRegularNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())

	round := "delete_regular_round"
	trialNum := "delete_regular_trial"

	// Cleanup first
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&RevealOrderScheme{}).Where("round = ?", round).Delete(ctx)

	// Add test data
	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	commit := &utils.CommitData{
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
	}
	err := regularRepo.AddCommit(ctx, commit)
	assert.NoError(t, err)

	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"node1"},
		RevealOrder:  []int{0},
		RV:           "rv_test",
	}
	err = revealRepo.AddRevealOrder(ctx, revealOrder)
	assert.NoError(t, err)

	// Delete
	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, round, trialNum)
	assert.NoError(t, err)

	// Verify deleted
	_, err = regularRepo.GetCommitByRound(ctx, round, trialNum)
	assert.Error(t, err)
}

func TestBatchRepository_DeleteOldRoundsForLeaderNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())

	currentRound := "current_3"
	oldRound := "old_1"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round IN (?)", []string{currentRound, oldRound}).Delete()

	// Add data
	var cvs [32]byte
	cvs[0] = 0x01

	currentCommit := &utils.LeaderCommitData{
		Round:      currentRound,
		TrialNum:   "trial1",
		EOAAddress: "0xleader",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, currentCommit)
	assert.NoError(t, err)

	oldCommit := &utils.LeaderCommitData{
		Round:      oldRound,
		TrialNum:   "trial1",
		EOAAddress: "0xleader_old",
		Cvs:        cvs,
	}
	err = leaderRepo.AddLeaderCommit(ctx, oldCommit)
	assert.NoError(t, err)

	// Delete old
	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, currentRound)
	assert.NoError(t, err)

	// Verify current exists, old deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, currentRound, "trial1", "0xleader")
	assert.NoError(t, err)

	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, oldRound, "trial1", "0xleader_old")
	assert.Error(t, err)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", currentRound).Delete()
}

func TestBatchRepository_DeleteOldRoundsForRegularNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())

	currentRound := "current_4"
	oldRound := "old_2"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round IN (?)", []string{currentRound, oldRound}).Delete()

	// Add data
	var cvs [32]byte
	cvs[0] = 0x01

	currentCommit := &utils.CommitData{
		Round:    currentRound,
		TrialNum: "trial1",
		Cvs:      cvs,
	}
	err := regularRepo.AddCommit(ctx, currentCommit)
	assert.NoError(t, err)

	oldCommit := &utils.CommitData{
		Round:    oldRound,
		TrialNum: "trial1",
		Cvs:      cvs,
	}
	err = regularRepo.AddCommit(ctx, oldCommit)
	assert.NoError(t, err)

	// Delete old
	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, currentRound)
	assert.NoError(t, err)

	// Verify current exists, old deleted
	_, err = regularRepo.GetCommitByRound(ctx, currentRound, "trial1")
	assert.NoError(t, err)

	_, err = regularRepo.GetCommitByRound(ctx, oldRound, "trial1")
	assert.Error(t, err)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", currentRound).Delete()
}

// Test deleting multiple rounds and trials for leader node
func TestBatchRepository_DeleteMultipleRoundsLeaderNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())

	var cvs [32]byte
	cvs[0] = 0xFF

	// Add multiple commits with different rounds and trials
	testCases := []struct {
		round string
		trial string
		eoa   string
	}{
		{"multi_round_1", "trial_1", "0xeoa1"},
		{"multi_round_1", "trial_2", "0xeoa2"},
		{"multi_round_2", "trial_1", "0xeoa3"},
	}

	for _, tc := range testCases {
		commit := &utils.LeaderCommitData{
			Round:      tc.round,
			TrialNum:   tc.trial,
			EOAAddress: tc.eoa,
			Cvs:        cvs,
		}
		err := leaderRepo.AddLeaderCommit(ctx, commit)
		assert.NoError(t, err)
	}

	// Delete one specific round-trial combination
	err := batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, "multi_round_1", "trial_1")
	assert.NoError(t, err)

	// Verify only the specific one is deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, "multi_round_1", "trial_1", "0xeoa1")
	assert.Error(t, err, "Should be deleted")

	// Others should still exist
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, "multi_round_1", "trial_2", "0xeoa2")
	assert.NoError(t, err, "Should still exist")

	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, "multi_round_2", "trial_1", "0xeoa3")
	assert.NoError(t, err, "Should still exist")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round LIKE ?", "multi_round_%").Delete()
}

// Test deleting multiple rounds and trials for regular node
func TestBatchRepository_DeleteMultipleRoundsRegularNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	peerRepo := NewPeerCommitRepository(GetDB())

	var cvs [32]byte
	cvs[0] = 0xFF

	round := "multi_reg_round"
	trial1 := "trial_1"
	trial2 := "trial_2"

	// Add commits
	commit1 := &utils.CommitData{
		Round:    round,
		TrialNum: trial1,
		Cvs:      cvs,
	}
	err := regularRepo.AddCommit(ctx, commit1)
	assert.NoError(t, err)

	commit2 := &utils.CommitData{
		Round:    round,
		TrialNum: trial2,
		Cvs:      cvs,
	}
	err = regularRepo.AddCommit(ctx, commit2)
	assert.NoError(t, err)

	// Add peer commit data
	peerData := &PeerCommitDataScheme{
		Round:      round,
		TrialNum:   trial1,
		EOAAddress: "0xpeer1",
		Cvs:        cvs[:],
	}
	err = peerRepo.AddPeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	// Delete one trial
	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, round, trial1)
	assert.NoError(t, err)

	// Verify trial1 deleted, trial2 exists
	_, err = regularRepo.GetCommitByRound(ctx, round, trial1)
	assert.Error(t, err, "Trial 1 should be deleted")

	_, err = regularRepo.GetCommitByRound(ctx, round, trial2)
	assert.NoError(t, err, "Trial 2 should still exist")

	_, err = peerRepo.GetPeerCommitData(ctx, round, trial1, "0xpeer1")
	assert.Error(t, err, "Peer commit should be deleted")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", round).Delete()
}

// Test deletion with empty database (no records to delete)
func TestBatchRepository_DeleteNonExistentData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

	// These should not error even if no records exist
	err := batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, "nonexistent_round", "nonexistent_trial")
	assert.NoError(t, err, "Should not error when deleting non-existent data")

	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, "nonexistent_round", "nonexistent_trial")
	assert.NoError(t, err, "Should not error when deleting non-existent data")

	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, "current_round_999")
	assert.NoError(t, err, "Should not error when no old data exists")

	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, "current_round_999")
	assert.NoError(t, err, "Should not error when no old data exists")
}

// Test deletion preserves current round data correctly
func TestBatchRepository_PreservesCurrentRoundData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())

	var cvs [32]byte
	cvs[0] = 0xAA

	currentRound := "preserve_current_123"
	oldRounds := []string{"preserve_old_100", "preserve_old_101", "preserve_old_102"}

	// Add current round data
	currentCommit := &utils.LeaderCommitData{
		Round:      currentRound,
		TrialNum:   "trial_current",
		EOAAddress: "0xcurrent",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, currentCommit)
	assert.NoError(t, err)

	// Add old rounds data
	for _, oldRound := range oldRounds {
		oldCommit := &utils.LeaderCommitData{
			Round:      oldRound,
			TrialNum:   "trial_old",
			EOAAddress: "0xold_" + oldRound,
			Cvs:        cvs,
		}
		err := leaderRepo.AddLeaderCommit(ctx, oldCommit)
		assert.NoError(t, err)
	}

	// Delete old rounds
	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, currentRound)
	assert.NoError(t, err)

	// Verify current round still exists
	retrieved, err := leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, currentRound, "trial_current", "0xcurrent")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, currentRound, retrieved.Round)

	// Verify all old rounds are deleted
	for _, oldRound := range oldRounds {
		_, err := leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, oldRound, "trial_old", "0xold_"+oldRound)
		assert.Error(t, err, "Old round %s should be deleted", oldRound)
	}

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round LIKE ?", "preserve_%").Delete()
}

// Test with BroadcastTracker data to ensure all tables are deleted in leader node
func TestBatchRepository_DeleteWithBroadcastTrackerLeader(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())

	round := "broadcast_leader_round"
	trial := "broadcast_trial"

	var cvs [32]byte
	cvs[0] = 0xBB

	// Add leader commit
	commit := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trial,
		EOAAddress: "0xleader_broadcast",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, commit)
	assert.NoError(t, err)

	// Add broadcast tracker
	tracker := &utils.BroadcastTracker{
		Round:      round,
		TrialNum:   trial,
		EOAAddress: "0xleader_broadcast",
		Type:       "cvs",
		MessageID:  "msg1",
		Data:       cvs,
	}
	err = broadcastRepo.AddBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Add reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trial,
		OrderedNodes: []string{"node1", "node2"},
		RevealOrder:  []int{0, 1},
		RV:           "rv_broadcast",
	}
	err = revealRepo.AddRevealOrder(ctx, revealOrder)
	assert.NoError(t, err)

	// Delete all data for this round-trial
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, round, trial)
	assert.NoError(t, err)

	// Verify all data deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trial, "0xleader_broadcast")
	assert.Error(t, err)

	trackers, err := broadcastRepo.GetBroadcastTrackers(ctx)
	assert.NoError(t, err)
	for _, tracker := range trackers {
		if tracker.Round == round && tracker.TrialNum == trial {
			assert.Fail(t, "Broadcast tracker should be deleted")
		}
	}

	_, err = revealRepo.GetRevealOrder(ctx, round, trial)
	assert.Error(t, err)
}

// Test with all tables for regular node deletion
func TestBatchRepository_DeleteWithAllTablesRegular(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	peerRepo := NewPeerCommitRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())

	round := "all_tables_regular"
	trial := "trial_all"

	var cvs [32]byte
	cvs[0] = 0xCC

	// Add regular commit
	commit := &utils.CommitData{
		Round:    round,
		TrialNum: trial,
		Cvs:      cvs,
	}
	err := regularRepo.AddCommit(ctx, commit)
	assert.NoError(t, err)

	// Add peer commit
	peerData := &PeerCommitDataScheme{
		Round:      round,
		TrialNum:   trial,
		EOAAddress: "0xpeer_all",
		Cvs:        cvs[:],
	}
	err = peerRepo.AddPeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	// Add broadcast tracker
	tracker := &utils.BroadcastTracker{
		Round:      round,
		TrialNum:   trial,
		EOAAddress: "0xregular_all",
		Type:       "cvs",
		MessageID:  "msg_all",
		Data:       cvs,
	}
	err = broadcastRepo.AddBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Add reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trial,
		OrderedNodes: []string{"node1"},
		RevealOrder:  []int{0},
		RV:           "rv_all",
	}
	err = revealRepo.AddRevealOrder(ctx, revealOrder)
	assert.NoError(t, err)

	// Delete all data
	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, round, trial)
	assert.NoError(t, err)

	// Verify all deleted
	_, err = regularRepo.GetCommitByRound(ctx, round, trial)
	assert.Error(t, err)

	_, err = peerRepo.GetPeerCommitData(ctx, round, trial, "0xpeer_all")
	assert.Error(t, err)

	_, err = revealRepo.GetRevealOrder(ctx, round, trial)
	assert.Error(t, err)

	trackers, err := broadcastRepo.GetBroadcastTrackers(ctx)
	assert.NoError(t, err)
	for _, tr := range trackers {
		if tr.Round == round && tr.TrialNum == trial {
			assert.Fail(t, "Broadcast tracker should be deleted")
		}
	}
}

// Test deletion with multiple old rounds exercising all delete paths
func TestBatchRepository_MultipleOldRoundsForRegularNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	peerRepo := NewPeerCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())

	currentRound := "multi_old_current"
	oldRounds := []string{"multi_old_1", "multi_old_2"}

	var cvs [32]byte
	cvs[0] = 0xDD

	// Add current round data
	currentCommit := &utils.CommitData{
		Round:    currentRound,
		TrialNum: "trial",
		Cvs:      cvs,
	}
	err := regularRepo.AddCommit(ctx, currentCommit)
	assert.NoError(t, err)

	// Add old rounds data with all table types
	for _, oldRound := range oldRounds {
		// Regular commit
		oldCommit := &utils.CommitData{
			Round:    oldRound,
			TrialNum: "trial",
			Cvs:      cvs,
		}
		err := regularRepo.AddCommit(ctx, oldCommit)
		assert.NoError(t, err)

		// Peer commit
		peerData := &PeerCommitDataScheme{
			Round:      oldRound,
			TrialNum:   "trial",
			EOAAddress: "0xpeer_" + oldRound,
			Cvs:        cvs[:],
		}
		err = peerRepo.AddPeerCommitData(ctx, peerData)
		assert.NoError(t, err)

		// Reveal order
		revealOrder := &utils.RevealOrderData{
			Round:        oldRound,
			TrialNum:     "trial",
			OrderedNodes: []string{"node"},
			RevealOrder:  []int{0},
			RV:           "rv_" + oldRound,
		}
		err = revealRepo.AddRevealOrder(ctx, revealOrder)
		assert.NoError(t, err)

		// Broadcast tracker
		tracker := &utils.BroadcastTracker{
			Round:      oldRound,
			TrialNum:   "trial",
			EOAAddress: "0xeoa_" + oldRound,
			Type:       "cvs",
			MessageID:  "msg_" + oldRound,
			Data:       cvs,
		}
		err = broadcastRepo.AddBroadcastTracker(ctx, tracker)
		assert.NoError(t, err)
	}

	// Delete old rounds - this exercises all 4 delete operations in DeleteOldRoundDataForRegularNode
	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, currentRound)
	assert.NoError(t, err)

	// Verify current round data still exists
	_, err = regularRepo.GetCommitByRound(ctx, currentRound, "trial")
	assert.NoError(t, err)

	// Verify all old rounds data deleted
	for _, oldRound := range oldRounds {
		_, err := regularRepo.GetCommitByRound(ctx, oldRound, "trial")
		assert.Error(t, err, "Old regular commit should be deleted")

		_, err = peerRepo.GetPeerCommitData(ctx, oldRound, "trial", "0xpeer_"+oldRound)
		assert.Error(t, err, "Old peer commit should be deleted")

		_, err = revealRepo.GetRevealOrder(ctx, oldRound, "trial")
		assert.Error(t, err, "Old reveal order should be deleted")
	}

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round LIKE ?", "multi_old_%").Delete()
	GetDB().Model(&BroadcastTrackerScheme{}).Where("round LIKE ?", "multi_old_%").Delete()
}

// Test deletion with multiple old rounds for leader node
func TestBatchRepository_MultipleOldRoundsForLeaderNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())

	currentRound := "leader_multi_current"
	oldRounds := []string{"leader_old_1", "leader_old_2", "leader_old_3"}

	var cvs [32]byte
	cvs[0] = 0xEE

	// Add current round
	currentCommit := &utils.LeaderCommitData{
		Round:      currentRound,
		TrialNum:   "trial",
		EOAAddress: "0xcurrent",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, currentCommit)
	assert.NoError(t, err)

	// Add old rounds with all table types
	for _, oldRound := range oldRounds {
		// Leader commit
		oldCommit := &utils.LeaderCommitData{
			Round:      oldRound,
			TrialNum:   "trial",
			EOAAddress: "0xleader_" + oldRound,
			Cvs:        cvs,
		}
		err := leaderRepo.AddLeaderCommit(ctx, oldCommit)
		assert.NoError(t, err)

		// Reveal order
		revealOrder := &utils.RevealOrderData{
			Round:        oldRound,
			TrialNum:     "trial",
			OrderedNodes: []string{"node"},
			RevealOrder:  []int{0},
			RV:           "rv_" + oldRound,
		}
		err = revealRepo.AddRevealOrder(ctx, revealOrder)
		assert.NoError(t, err)

		// Broadcast tracker
		tracker := &utils.BroadcastTracker{
			Round:      oldRound,
			TrialNum:   "trial",
			EOAAddress: "0xeoa_" + oldRound,
			Type:       "cvs",
			MessageID:  "msg_" + oldRound,
			Data:       cvs,
		}
		err = broadcastRepo.AddBroadcastTracker(ctx, tracker)
		assert.NoError(t, err)
	}

	// Delete old rounds - exercises all 3 delete operations in DeleteOldRoundDataForLeaderNode
	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, currentRound)
	assert.NoError(t, err)

	// Verify current round still exists
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, currentRound, "trial", "0xcurrent")
	assert.NoError(t, err)

	// Verify all old rounds deleted
	for _, oldRound := range oldRounds {
		_, err := leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, oldRound, "trial", "0xleader_"+oldRound)
		assert.Error(t, err, "Old leader commit should be deleted")

		_, err = revealRepo.GetRevealOrder(ctx, oldRound, "trial")
		assert.Error(t, err, "Old reveal order should be deleted")
	}

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round LIKE ?", "leader_%").Delete()
	GetDB().Model(&BroadcastTrackerScheme{}).Where("round LIKE ?", "leader_%").Delete()
}

// Test error handling by temporarily dropping a table for leader node round-trial deletion
func TestBatchRepository_DeleteRoundTrialDataForLeaderNode_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

	// Drop the broadcast_tracker_schemes table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to delete - should get an error because table doesn't exist
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when table is missing")
	assert.Contains(t, err.Error(), "failed to delete", "Error should contain 'failed to delete'")

	// Restore the schema by running migrations again
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling by temporarily dropping a table for regular node round-trial deletion
func TestBatchRepository_DeleteRoundTrialDataForRegularNode_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

	// Drop the peer_commit_data_schemes table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS peer_commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to delete - should get an error because table doesn't exist
	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when table is missing")
	assert.Contains(t, err.Error(), "failed to delete", "Error should contain 'failed to delete'")

	// Restore the schema by running migrations again
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for DeleteOldRoundDataForLeaderNode
func TestBatchRepository_DeleteOldRoundDataForLeaderNode_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	// Test error in first delete operation (leader_commit_schemes)
	_, err := GetDB().Exec("DROP TABLE IF EXISTS leader_commit_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, "current_round")
	assert.Error(t, err, "Expected error when leader_commit_schemes table is missing")
	assert.Contains(t, err.Error(), "failed to delete from leader_commit_schemes", "Error should mention leader_commit_schemes")

	// Restore schema
	sqlDB, _ := sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error in second delete operation (reveal_order_schemes)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS reveal_order_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, "current_round")
	assert.Error(t, err, "Expected error when reveal_order_schemes table is missing")
	assert.Contains(t, err.Error(), "failed to delete from reveal_order_schemes", "Error should mention reveal_order_schemes")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error in third delete operation (broadcast_tracker_schemes)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, "current_round")
	assert.Error(t, err, "Expected error when broadcast_tracker_schemes table is missing")
	assert.Contains(t, err.Error(), "failed to delete from broadcast_tracker_schemes", "Error should mention broadcast_tracker_schemes")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for DeleteOldRoundDataForRegularNode
func TestBatchRepository_DeleteOldRoundDataForRegularNode_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	// Test error in first delete operation (commit_data_schemes)
	_, err := GetDB().Exec("DROP TABLE IF EXISTS commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, "current_round")
	assert.Error(t, err, "Expected error when commit_data_schemes table is missing")
	assert.Contains(t, err.Error(), "failed to delete from commit_data_schemes", "Error should mention commit_data_schemes")

	// Restore schema
	sqlDB, _ := sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error in second delete operation (reveal_order_schemes)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS reveal_order_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, "current_round")
	assert.Error(t, err, "Expected error when reveal_order_schemes table is missing")
	assert.Contains(t, err.Error(), "failed to delete from reveal_order_schemes", "Error should mention reveal_order_schemes")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error in third delete operation (peer_commit_data_schemes)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS peer_commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, "current_round")
	assert.Error(t, err, "Expected error when peer_commit_data_schemes table is missing")
	assert.Contains(t, err.Error(), "failed to delete from peer_commit_data_schemes", "Error should mention peer_commit_data_schemes")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error in fourth delete operation (broadcast_tracker_schemes)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, "current_round")
	assert.Error(t, err, "Expected error when broadcast_tracker_schemes table is missing")
	assert.Contains(t, err.Error(), "failed to delete from broadcast_tracker_schemes", "Error should mention broadcast_tracker_schemes")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test NewBatchRepository constructor
func TestNewBatchRepository(t *testing.T) {
	db := GetDB()
	repo := NewBatchRepository(db)
	assert.NotNil(t, repo, "Repository should be created")
	assert.Equal(t, db, repo.db, "Repository should store the database connection")
}

// Test error handling for each table in DeleteRoundTrialDataForLeaderNode loop
func TestBatchRepository_DeleteRoundTrialDataForLeaderNode_ErrorHandlingEachTable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	// Test error from first table (LeaderCommitScheme)
	_, err := GetDB().Exec("DROP TABLE IF EXISTS leader_commit_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when leader_commit_schemes table is missing")
	assert.Contains(t, err.Error(), "failed to delete from", "Error should contain 'failed to delete from'")
	assert.Contains(t, err.Error(), "LeaderCommitScheme", "Error should mention LeaderCommitScheme")

	// Restore schema
	sqlDB, _ := sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error from second table (RevealOrderScheme)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS reveal_order_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when reveal_order_schemes table is missing")
	assert.Contains(t, err.Error(), "RevealOrderScheme", "Error should mention RevealOrderScheme")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error from third table (BroadcastTrackerScheme)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when broadcast_tracker_schemes table is missing")
	assert.Contains(t, err.Error(), "BroadcastTrackerScheme", "Error should mention BroadcastTrackerScheme")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for each table in DeleteRoundTrialDataForRegularNode loop
func TestBatchRepository_DeleteRoundTrialDataForRegularNode_ErrorHandlingEachTable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	// Test error from first table (CommitDataScheme)
	_, err := GetDB().Exec("DROP TABLE IF EXISTS commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when commit_data_schemes table is missing")
	assert.Contains(t, err.Error(), "CommitDataScheme", "Error should mention CommitDataScheme")

	// Restore schema
	sqlDB, _ := sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error from second table (RevealOrderScheme)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS reveal_order_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when reveal_order_schemes table is missing")
	assert.Contains(t, err.Error(), "RevealOrderScheme", "Error should mention RevealOrderScheme")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error from third table (PeerCommitDataScheme)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS peer_commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when peer_commit_data_schemes table is missing")
	assert.Contains(t, err.Error(), "PeerCommitDataScheme", "Error should mention PeerCommitDataScheme")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Test error from fourth table (BroadcastTrackerScheme)
	_, err = GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when broadcast_tracker_schemes table is missing")
	assert.Contains(t, err.Error(), "BroadcastTrackerScheme", "Error should mention BroadcastTrackerScheme")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test edge cases: empty strings, nil context, etc.
func TestBatchRepository_EdgeCases(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

	// Test with empty round and trialNum - should not error (just deletes nothing)
	err := batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, "", "")
	assert.NoError(t, err, "Should handle empty strings gracefully")

	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, "", "")
	assert.NoError(t, err, "Should handle empty strings gracefully")

	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, "")
	assert.NoError(t, err, "Should handle empty currentRound gracefully")

	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, "")
	assert.NoError(t, err, "Should handle empty currentRound gracefully")

	// Test with very long strings
	longString := string(make([]byte, 1000))
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, longString, longString)
	assert.NoError(t, err, "Should handle long strings")

	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, longString)
	assert.NoError(t, err, "Should handle long strings")
}

// Test with background context (valid context usage)
func TestBatchRepository_WithBackgroundContext(t *testing.T) {
	batchRepo := NewBatchRepository(GetDB())
	ctx := context.Background()

	// Test with background context - should work fine
	err := batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, "test_round", "test_trial")
	assert.NoError(t, err, "Should handle background context")

	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, "test_round", "test_trial")
	assert.NoError(t, err, "Should handle background context")

	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, "test_round")
	assert.NoError(t, err, "Should handle background context")

	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, "test_round")
	assert.NoError(t, err, "Should handle background context")
}

// Test concurrent operations
func TestBatchRepository_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	round := "concurrent_test_round"
	trialNum := "concurrent_test_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", round).Delete(ctx)

	// Run concurrent delete operations
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			err := batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, round, trialNum)
			done <- err
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		err := <-done
		assert.NoError(t, err, "Concurrent operations should not error")
	}
}

// Test that all tables are processed in DeleteRoundTrialDataForLeaderNode
func TestBatchRepository_DeleteRoundTrialDataForLeaderNode_AllTablesProcessed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())

	round := "all_tables_leader_test"
	trialNum := "all_tables_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&RevealOrderScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&BroadcastTrackerScheme{}).Where("round = ?", round).Delete(ctx)

	// Add data to all three tables
	var cvs [32]byte
	cvs[0] = 0x01

	leaderCommit := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: "0xleader_all",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, leaderCommit)
	assert.NoError(t, err)

	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"node1"},
		RevealOrder:  []int{0},
		RV:           "rv_all",
	}
	err = revealRepo.AddRevealOrder(ctx, revealOrder)
	assert.NoError(t, err)

	tracker := &utils.BroadcastTracker{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: "0xleader_all",
		Type:       "cvs",
		MessageID:  "msg_all",
		Data:       cvs,
	}
	err = broadcastRepo.AddBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Delete all
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, round, trialNum)
	assert.NoError(t, err)

	// Verify all are deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, "0xleader_all")
	assert.Error(t, err, "Leader commit should be deleted")

	_, err = revealRepo.GetRevealOrder(ctx, round, trialNum)
	assert.Error(t, err, "Reveal order should be deleted")

	trackers, err := broadcastRepo.GetBroadcastTrackers(ctx)
	assert.NoError(t, err)
	found := false
	for _, tr := range trackers {
		if tr.Round == round && tr.TrialNum == trialNum {
			found = true
			break
		}
	}
	assert.False(t, found, "Broadcast tracker should be deleted")
}

// Test that all tables are processed in DeleteRoundTrialDataForRegularNode
func TestBatchRepository_DeleteRoundTrialDataForRegularNode_AllTablesProcessed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	peerRepo := NewPeerCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())

	round := "all_tables_regular_test"
	trialNum := "all_tables_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&PeerCommitDataScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&RevealOrderScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&BroadcastTrackerScheme{}).Where("round = ?", round).Delete(ctx)

	// Add data to all four tables
	var cvs [32]byte
	cvs[0] = 0x01

	commit := &utils.CommitData{
		Round:    round,
		TrialNum: trialNum,
		Cvs:      cvs,
	}
	err := regularRepo.AddCommit(ctx, commit)
	assert.NoError(t, err)

	peerData := &PeerCommitDataScheme{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: "0xpeer_all",
		Cvs:        cvs[:],
	}
	err = peerRepo.AddPeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"node1"},
		RevealOrder:  []int{0},
		RV:           "rv_all",
	}
	err = revealRepo.AddRevealOrder(ctx, revealOrder)
	assert.NoError(t, err)

	tracker := &utils.BroadcastTracker{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: "0xregular_all",
		Type:       "cvs",
		MessageID:  "msg_all",
		Data:       cvs,
	}
	err = broadcastRepo.AddBroadcastTracker(ctx, tracker)
	assert.NoError(t, err)

	// Delete all
	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, round, trialNum)
	assert.NoError(t, err)

	// Verify all are deleted
	_, err = regularRepo.GetCommitByRound(ctx, round, trialNum)
	assert.Error(t, err, "Regular commit should be deleted")

	_, err = peerRepo.GetPeerCommitData(ctx, round, trialNum, "0xpeer_all")
	assert.Error(t, err, "Peer commit should be deleted")

	_, err = revealRepo.GetRevealOrder(ctx, round, trialNum)
	assert.Error(t, err, "Reveal order should be deleted")

	trackers, err := broadcastRepo.GetBroadcastTrackers(ctx)
	assert.NoError(t, err)
	found := false
	for _, tr := range trackers {
		if tr.Round == round && tr.TrialNum == trialNum {
			found = true
			break
		}
	}
	assert.False(t, found, "Broadcast tracker should be deleted")
}

// Test large batch deletion for leader node - tests performance and handling of large datasets
func TestBatchRepository_DeleteRoundTrialDataForLeaderNode_LargeBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())

	round := "large_batch_leader_round"
	trialNum := "large_batch_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&RevealOrderScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&BroadcastTrackerScheme{}).Where("round = ?", round).Delete(ctx)

	// Create large batch of data (100 records per table)
	numRecords := 100
	var cvs [32]byte
	cvs[0] = 0x01

	// Add large number of leader commits
	for i := 0; i < numRecords; i++ {
		commit := &utils.LeaderCommitData{
			Round:      round,
			TrialNum:   trialNum,
			EOAAddress: fmt.Sprintf("0xleader_%d", i),
			Cvs:        cvs,
		}
		err := leaderRepo.AddLeaderCommit(ctx, commit)
		assert.NoError(t, err, "Should add leader commit %d", i)
	}

	// Add one reveal order (reveal_order_schemes has unique constraint on round+trial_num)
	nodes := make([]string, 10)
	order := make([]int, 10)
	for j := 0; j < 10; j++ {
		nodes[j] = fmt.Sprintf("0xnode_%d", j)
		order[j] = j
	}
	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: nodes,
		RevealOrder:  order,
		RV:           "rv_large",
	}
	err := revealRepo.AddRevealOrder(ctx, revealOrder)
	assert.NoError(t, err, "Should add reveal order")

	// Add large number of broadcast trackers
	for i := 0; i < numRecords; i++ {
		tracker := &utils.BroadcastTracker{
			Round:      round,
			TrialNum:   trialNum,
			EOAAddress: fmt.Sprintf("0xtracker_%d", i),
			Type:       "cvs",
			MessageID:  fmt.Sprintf("msg_%d", i),
			Data:       cvs,
		}
		err := broadcastRepo.AddBroadcastTracker(ctx, tracker)
		assert.NoError(t, err, "Should add tracker %d", i)
	}

	// Verify data was added
	count, err := GetDB().WithContext(ctx).Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Count()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, numRecords, "Should have at least %d leader commits", numRecords)

	// Delete all in batch
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, round, trialNum)
	assert.NoError(t, err, "Should successfully delete large batch")

	// Verify all deleted
	finalCount, err := GetDB().WithContext(ctx).Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 0, finalCount, "All records should be deleted")
}

// Test large batch deletion for regular node
func TestBatchRepository_DeleteRoundTrialDataForRegularNode_LargeBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	peerRepo := NewPeerCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())

	round := "large_batch_regular_round"
	trialNum := "large_batch_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&PeerCommitDataScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&RevealOrderScheme{}).Where("round = ?", round).Delete(ctx)
	GetDB().Model(&BroadcastTrackerScheme{}).Where("round = ?", round).Delete(ctx)

	// Create large batch of data
	numRecords := 100
	var cvs [32]byte
	cvs[0] = 0x02

	// Add one regular commit (commit_data_schemes has unique constraint on round+trial_num)
	commit := &utils.CommitData{
		Round:    round,
		TrialNum: trialNum,
		Cvs:      cvs,
	}
	err := regularRepo.AddCommit(ctx, commit)
	assert.NoError(t, err, "Should add commit")

	// Add large number of peer commits
	for i := 0; i < numRecords; i++ {
		peerData := &PeerCommitDataScheme{
			Round:      round,
			TrialNum:   trialNum,
			EOAAddress: fmt.Sprintf("0xpeer_%d", i),
			Cvs:        cvs[:],
		}
		err := peerRepo.AddPeerCommitData(ctx, peerData)
		assert.NoError(t, err, "Should add peer commit %d", i)
	}

	// Add one reveal order (reveal_order_schemes has unique constraint on round+trial_num)
	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"0xnode_0"},
		RevealOrder:  []int{0},
		RV:           "rv_large",
	}
	err = revealRepo.AddRevealOrder(ctx, revealOrder)
	assert.NoError(t, err, "Should add reveal order")

	// Add large number of broadcast trackers
	for i := 0; i < numRecords; i++ {
		tracker := &utils.BroadcastTracker{
			Round:      round,
			TrialNum:   trialNum,
			EOAAddress: fmt.Sprintf("0xtracker_%d", i),
			Type:       "cvs",
			MessageID:  fmt.Sprintf("msg_%d", i),
			Data:       cvs,
		}
		err := broadcastRepo.AddBroadcastTracker(ctx, tracker)
		assert.NoError(t, err, "Should add tracker %d", i)
	}

	// Verify data was added
	count, err := GetDB().WithContext(ctx).Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Should have exactly 1 commit (unique constraint on round+trial_num)")

	// Verify peer commits were added (these can have multiple per round/trial)
	peerCount, err := GetDB().WithContext(ctx).Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Count()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, peerCount, numRecords, "Should have at least %d peer commits", numRecords)

	// Delete all in batch
	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, round, trialNum)
	assert.NoError(t, err, "Should successfully delete large batch")

	// Verify all deleted
	finalCount, err := GetDB().WithContext(ctx).Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 0, finalCount, "All records should be deleted")
}

// Test large batch deletion of old rounds for leader node
func TestBatchRepository_DeleteOldRoundDataForLeaderNode_LargeBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())

	currentRound := "large_current_round"
	numOldRounds := 20
	var cvs [32]byte
	cvs[0] = 0x03

	// Create many old rounds with data
	for i := 0; i < numOldRounds; i++ {
		oldRound := fmt.Sprintf("large_old_round_%d", i)

		// Add leader commit
		commit := &utils.LeaderCommitData{
			Round:      oldRound,
			TrialNum:   "trial1",
			EOAAddress: fmt.Sprintf("0xleader_%d", i),
			Cvs:        cvs,
		}
		err := leaderRepo.AddLeaderCommit(ctx, commit)
		assert.NoError(t, err)

		// Add reveal order
		revealOrder := &utils.RevealOrderData{
			Round:        oldRound,
			TrialNum:     "trial1",
			OrderedNodes: []string{fmt.Sprintf("0xnode_%d", i)},
			RevealOrder:  []int{0},
			RV:           fmt.Sprintf("rv_%d", i),
		}
		err = revealRepo.AddRevealOrder(ctx, revealOrder)
		assert.NoError(t, err)

		// Add broadcast tracker
		tracker := &utils.BroadcastTracker{
			Round:      oldRound,
			TrialNum:   "trial1",
			EOAAddress: fmt.Sprintf("0xtracker_%d", i),
			Type:       "cvs",
			MessageID:  fmt.Sprintf("msg_%d", i),
			Data:       cvs,
		}
		err = broadcastRepo.AddBroadcastTracker(ctx, tracker)
		assert.NoError(t, err)
	}

	// Add current round data
	currentCommit := &utils.LeaderCommitData{
		Round:      currentRound,
		TrialNum:   "trial1",
		EOAAddress: "0xcurrent",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, currentCommit)
	assert.NoError(t, err)

	// Verify old rounds exist
	count, err := GetDB().WithContext(ctx).Model(&LeaderCommitScheme{}).
		Where("round LIKE ?", "large_old_round_%").
		Count()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, numOldRounds, "Should have old round data")

	// Delete old rounds
	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, currentRound)
	assert.NoError(t, err, "Should successfully delete large batch of old rounds")

	// Verify old rounds deleted
	oldCount, err := GetDB().WithContext(ctx).Model(&LeaderCommitScheme{}).
		Where("round LIKE ?", "large_old_round_%").
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 0, oldCount, "All old rounds should be deleted")

	// Verify current round still exists
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, currentRound, "trial1", "0xcurrent")
	assert.NoError(t, err, "Current round should still exist")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", currentRound).Delete(ctx)
}

// Test large batch deletion of old rounds for regular node
func TestBatchRepository_DeleteOldRoundDataForRegularNode_LargeBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	peerRepo := NewPeerCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())
	broadcastRepo := NewBroadcastTrackerRepository(GetDB())

	currentRound := "large_reg_current_round"
	numOldRounds := 20
	var cvs [32]byte
	cvs[0] = 0x04

	// Create many old rounds with data
	for i := 0; i < numOldRounds; i++ {
		oldRound := fmt.Sprintf("large_reg_old_round_%d", i)

		// Add regular commit
		commit := &utils.CommitData{
			Round:    oldRound,
			TrialNum: "trial1",
			Cvs:      cvs,
		}
		err := regularRepo.AddCommit(ctx, commit)
		assert.NoError(t, err)

		// Add peer commit
		peerData := &PeerCommitDataScheme{
			Round:      oldRound,
			TrialNum:   "trial1",
			EOAAddress: fmt.Sprintf("0xpeer_%d", i),
			Cvs:        cvs[:],
		}
		err = peerRepo.AddPeerCommitData(ctx, peerData)
		assert.NoError(t, err)

		// Add reveal order
		revealOrder := &utils.RevealOrderData{
			Round:        oldRound,
			TrialNum:     "trial1",
			OrderedNodes: []string{fmt.Sprintf("0xnode_%d", i)},
			RevealOrder:  []int{0},
			RV:           fmt.Sprintf("rv_%d", i),
		}
		err = revealRepo.AddRevealOrder(ctx, revealOrder)
		assert.NoError(t, err)

		// Add broadcast tracker
		tracker := &utils.BroadcastTracker{
			Round:      oldRound,
			TrialNum:   "trial1",
			EOAAddress: fmt.Sprintf("0xtracker_%d", i),
			Type:       "cvs",
			MessageID:  fmt.Sprintf("msg_%d", i),
			Data:       cvs,
		}
		err = broadcastRepo.AddBroadcastTracker(ctx, tracker)
		assert.NoError(t, err)
	}

	// Add current round data
	currentCommit := &utils.CommitData{
		Round:    currentRound,
		TrialNum: "trial1",
		Cvs:      cvs,
	}
	err := regularRepo.AddCommit(ctx, currentCommit)
	assert.NoError(t, err)

	// Verify old rounds exist
	count, err := GetDB().WithContext(ctx).Model(&CommitDataScheme{}).
		Where("round LIKE ?", "large_reg_old_round_%").
		Count()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, numOldRounds, "Should have old round data")

	// Delete old rounds
	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, currentRound)
	assert.NoError(t, err, "Should successfully delete large batch of old rounds")

	// Verify old rounds deleted
	oldCount, err := GetDB().WithContext(ctx).Model(&CommitDataScheme{}).
		Where("round LIKE ?", "large_reg_old_round_%").
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 0, oldCount, "All old rounds should be deleted")

	// Verify current round still exists
	_, err = regularRepo.GetCommitByRound(ctx, currentRound, "trial1")
	assert.NoError(t, err, "Current round should still exist")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", currentRound).Delete(ctx)
}

// Test batch deletion with context timeout - simulates failure due to timeout
func TestBatchRepository_DeleteRoundTrialDataForLeaderNode_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the delete operation will block.
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE leader_commit_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

	// This call should now block on the locked table and time out.
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected an error due to context timeout")
	// The underlying driver might return a different error message, but it should be related to the context.
	assert.Contains(t, err.Error(), "i/o timeout", "Error should be related to i/o timeout")
}

// Test batch deletion with context timeout for regular node
func TestBatchRepository_DeleteRoundTrialDataForRegularNode_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the delete operation will block.
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE commit_data_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

	// This call should now block on the locked table and time out.
	err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected an error due to context timeout")
	// The underlying driver might return a different error message, but it should be related to the context.
	assert.Contains(t, err.Error(), "i/o timeout", "Error should be related to i/o timeout")
}

// Test partial failure in batch deletion - when one table fails mid-operation
func TestBatchRepository_DeleteRoundTrialDataForLeaderNode_PartialFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	round := "partial_failure_round"
	trialNum := "partial_failure_trial"

	// Add some data
	var cvs [32]byte
	cvs[0] = 0x05
	commit := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: "0xpartial",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, commit)
	assert.NoError(t, err)

	// Drop a table mid-operation to simulate partial failure
	_, err = GetDB().Exec("DROP TABLE IF EXISTS reveal_order_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to delete - should fail on the second table
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, round, trialNum)
	assert.Error(t, err, "Should fail when table is missing")
	assert.Contains(t, err.Error(), "RevealOrderScheme", "Error should mention RevealOrderScheme")

	// Restore schema
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", round).Delete(ctx)
}

// Test batch deletion with very large dataset - stress test
func TestBatchRepository_DeleteRoundTrialDataForLeaderNode_VeryLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())

	round := "very_large_leader_round"
	trialNum := "very_large_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", round).Delete(ctx)

	// Create very large dataset (1000 records)
	numRecords := 1000
	var cvs [32]byte
	cvs[0] = 0x06

	// Add records in batches to avoid overwhelming the database
	batchSize := 100
	for batch := 0; batch < numRecords/batchSize; batch++ {
		for i := 0; i < batchSize; i++ {
			idx := batch*batchSize + i
			commit := &utils.LeaderCommitData{
				Round:      round,
				TrialNum:   trialNum,
				EOAAddress: fmt.Sprintf("0xlarge_%d", idx),
				Cvs:        cvs,
			}
			err := leaderRepo.AddLeaderCommit(ctx, commit)
			if err != nil {
				t.Fatalf("Failed to add commit %d: %v", idx, err)
			}
		}
	}

	// Verify data was added
	count, err := GetDB().WithContext(ctx).Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Count()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, numRecords, "Should have at least %d records", numRecords)

	// Delete all in batch
	startTime := time.Now()
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, round, trialNum)
	duration := time.Since(startTime)
	assert.NoError(t, err, "Should successfully delete very large batch")

	// Log performance
	t.Logf("Deleted %d records in %v", count, duration)

	// Verify all deleted
	finalCount, err := GetDB().WithContext(ctx).Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 0, finalCount, "All records should be deleted")
}

// Test batch deletion failure recovery - verify system can recover after failure
func TestBatchRepository_DeleteRoundTrialDataForLeaderNode_FailureRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	round := "recovery_test_round"
	trialNum := "recovery_test_trial"

	// Add data
	var cvs [32]byte
	cvs[0] = 0x07
	commit := &utils.LeaderCommitData{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: "0xrecovery",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, commit)
	assert.NoError(t, err)

	// Cause a failure by dropping a table
	_, err = GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err)

	// First attempt should fail
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, round, trialNum)
	assert.Error(t, err, "Should fail when table is missing")

	// Restore schema
	sqlDB, _ := sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Second attempt should succeed after recovery
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, round, trialNum)
	assert.NoError(t, err, "Should succeed after schema is restored")

	// Verify data is deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, "0xrecovery")
	assert.Error(t, err, "Data should be deleted after successful retry")
}

// Test batch deletion with mixed success/failure scenarios
func TestBatchRepository_DeleteOldRoundDataForLeaderNode_MixedScenarios(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())

	currentRound := "mixed_current"
	var cvs [32]byte
	cvs[0] = 0x08

	// Add current round
	currentCommit := &utils.LeaderCommitData{
		Round:      currentRound,
		TrialNum:   "trial1",
		EOAAddress: "0xcurrent",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, currentCommit)
	assert.NoError(t, err)

	// Add many old rounds
	for i := 0; i < 50; i++ {
		oldRound := fmt.Sprintf("mixed_old_%d", i)
		oldCommit := &utils.LeaderCommitData{
			Round:      oldRound,
			TrialNum:   "trial1",
			EOAAddress: fmt.Sprintf("0xold_%d", i),
			Cvs:        cvs,
		}
		err := leaderRepo.AddLeaderCommit(ctx, oldCommit)
		assert.NoError(t, err)
	}

	// Delete old rounds
	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, currentRound)
	assert.NoError(t, err, "Should successfully delete all old rounds")

	// Verify current round exists
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, currentRound, "trial1", "0xcurrent")
	assert.NoError(t, err, "Current round should exist")

	// Verify old rounds deleted
	oldCount, err := GetDB().WithContext(ctx).Model(&LeaderCommitScheme{}).
		Where("round LIKE ?", "mixed_old_%").
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 0, oldCount, "All old rounds should be deleted")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", currentRound).Delete(ctx)
}

// Test context timeout for DeleteOldRoundDataForLeaderNode
func TestBatchRepository_DeleteOldRoundDataForLeaderNode_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the delete operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE leader_commit_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

	// This call should block on the locked table and time out
	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, "test_round")
	assert.Error(t, err, "Expected an error due to context timeout")
	assert.Contains(t, err.Error(), "i/o timeout", "Error should be related to i/o timeout")
}

// Test context timeout for DeleteOldRoundDataForRegularNode
func TestBatchRepository_DeleteOldRoundDataForRegularNode_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the delete operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE commit_data_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

	// This call should block on the locked table and time out
	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, "test_round")
	assert.Error(t, err, "Expected an error due to context timeout")
	assert.Contains(t, err.Error(), "i/o timeout", "Error should be related to i/o timeout")
}

// Test concurrent operations for DeleteOldRoundDataForLeaderNode
func TestBatchRepository_DeleteOldRoundDataForLeaderNode_Concurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	currentRound := "concurrent_old_leader_round"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round LIKE ?", "concurrent_old_leader_%").Delete(ctx)

	// Run concurrent delete operations
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			err := batchRepo.DeleteOldRoundDataForLeaderNode(ctx, currentRound)
			done <- err
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		err := <-done
		assert.NoError(t, err, "Concurrent operations should not error")
	}
}

// Test concurrent operations for DeleteOldRoundDataForRegularNode
func TestBatchRepository_DeleteOldRoundDataForRegularNode_Concurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	currentRound := "concurrent_old_regular_round"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round LIKE ?", "concurrent_old_regular_%").Delete(ctx)

	// Run concurrent delete operations
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			err := batchRepo.DeleteOldRoundDataForRegularNode(ctx, currentRound)
			done <- err
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		err := <-done
		assert.NoError(t, err, "Concurrent operations should not error")
	}
}

// Test failure recovery for DeleteOldRoundDataForRegularNode
func TestBatchRepository_DeleteOldRoundDataForRegularNode_FailureRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	currentRound := "recovery_old_reg_current"
	oldRound := "recovery_old_reg_old"

	// Add data
	var cvs [32]byte
	cvs[0] = 0x09
	oldCommit := &utils.CommitData{
		Round:    oldRound,
		TrialNum: "trial1",
		Cvs:      cvs,
	}
	err := regularRepo.AddCommit(ctx, oldCommit)
	assert.NoError(t, err)

	// Cause a failure by dropping a table
	_, err = GetDB().Exec("DROP TABLE IF EXISTS commit_data_schemes CASCADE")
	assert.NoError(t, err)

	// First attempt should fail
	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, currentRound)
	assert.Error(t, err, "Should fail when table is missing")

	// Restore schema
	sqlDB, _ := sql.Open("postgres", dsn)
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
	sqlDB.Close()

	// Second attempt should succeed after recovery
	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, currentRound)
	assert.NoError(t, err, "Should succeed after schema is restored")

	// Verify old round data is deleted
	_, err = regularRepo.GetCommitByRound(ctx, oldRound, "trial1")
	assert.Error(t, err, "Old round data should be deleted after successful retry")
}

// Test with special characters in round names to ensure SQL injection prevention
func TestBatchRepository_SpecialCharactersInRoundNames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

	// Test with various special characters that could be used in SQL injection attempts
	specialRounds := []string{
		"'; DROP TABLE leader_commit_schemes; --",
		"'; DELETE FROM leader_commit_schemes; --",
		"1' OR '1'='1",
		"1' UNION SELECT * FROM leader_commit_schemes; --",
		"round'; DELETE FROM leader_commit_schemes WHERE '1'='1",
		"round\" OR \"1\"=\"1",
		"round\\'; DROP TABLE leader_commit_schemes; --",
	}

	for _, specialRound := range specialRounds {
		// These should not error (just delete nothing if the round doesn't exist)
		// The important thing is that they don't cause SQL injection
		err := batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, specialRound, "trial")
		assert.NoError(t, err, "Should handle special characters safely: %s", specialRound)

		err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, specialRound, "trial")
		assert.NoError(t, err, "Should handle special characters safely: %s", specialRound)

		err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, specialRound)
		assert.NoError(t, err, "Should handle special characters safely: %s", specialRound)

		err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, specialRound)
		assert.NoError(t, err, "Should handle special characters safely: %s", specialRound)
	}
}

// Test with unicode and international characters in round names
func TestBatchRepository_UnicodeCharactersInRoundNames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())

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
		err := batchRepo.DeleteRoundTrialDataForLeaderNode(ctx, unicodeRound, "trial")
		assert.NoError(t, err, "Should handle unicode characters: %s", unicodeRound)

		err = batchRepo.DeleteRoundTrialDataForRegularNode(ctx, unicodeRound, "trial")
		assert.NoError(t, err, "Should handle unicode characters: %s", unicodeRound)

		err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, unicodeRound)
		assert.NoError(t, err, "Should handle unicode characters: %s", unicodeRound)

		err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, unicodeRound)
		assert.NoError(t, err, "Should handle unicode characters: %s", unicodeRound)
	}
}

// Test partial failure in DeleteOldRoundDataForRegularNode - when one table fails mid-operation
func TestBatchRepository_DeleteOldRoundDataForRegularNode_PartialFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	currentRound := "partial_old_reg_current"
	oldRound := "partial_old_reg_old"

	// Add some data
	var cvs [32]byte
	cvs[0] = 0x0A
	oldCommit := &utils.CommitData{
		Round:    oldRound,
		TrialNum: "trial1",
		Cvs:      cvs,
	}
	err := regularRepo.AddCommit(ctx, oldCommit)
	assert.NoError(t, err)

	// Drop a table mid-operation to simulate partial failure
	_, err = GetDB().Exec("DROP TABLE IF EXISTS peer_commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to delete - should fail on the third table (peer_commit_data_schemes)
	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, currentRound)
	assert.Error(t, err, "Should fail when table is missing")
	assert.Contains(t, err.Error(), "peer_commit_data_schemes", "Error should mention peer_commit_data_schemes")

	// Restore schema
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", oldRound).Delete(ctx)
}

// Test partial failure in DeleteOldRoundDataForLeaderNode - when one table fails mid-operation
func TestBatchRepository_DeleteOldRoundDataForLeaderNode_PartialFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	currentRound := "partial_old_leader_current"
	oldRound := "partial_old_leader_old"

	// Add some data
	var cvs [32]byte
	cvs[0] = 0x0B
	oldCommit := &utils.LeaderCommitData{
		Round:      oldRound,
		TrialNum:   "trial1",
		EOAAddress: "0xpartial_old",
		Cvs:        cvs,
	}
	err := leaderRepo.AddLeaderCommit(ctx, oldCommit)
	assert.NoError(t, err)

	// Drop a table mid-operation to simulate partial failure
	_, err = GetDB().Exec("DROP TABLE IF EXISTS reveal_order_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to delete - should fail on the second table (reveal_order_schemes)
	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, currentRound)
	assert.Error(t, err, "Should fail when table is missing")
	assert.Contains(t, err.Error(), "reveal_order_schemes", "Error should mention reveal_order_schemes")

	// Restore schema
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", oldRound).Delete(ctx)
}

// Test DeleteOldRoundDataForRegularNode with very large dataset - stress test
func TestBatchRepository_DeleteOldRoundDataForRegularNode_VeryLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())

	currentRound := "very_large_old_reg_current"
	numOldRounds := 100
	var cvs [32]byte
	cvs[0] = 0x0C

	// Create many old rounds with data
	for i := 0; i < numOldRounds; i++ {
		oldRound := fmt.Sprintf("very_large_old_reg_old_%d", i)
		oldCommit := &utils.CommitData{
			Round:    oldRound,
			TrialNum: "trial1",
			Cvs:      cvs,
		}
		err := regularRepo.AddCommit(ctx, oldCommit)
		if err != nil {
			t.Fatalf("Failed to add commit %d: %v", i, err)
		}
	}

	// Verify old rounds exist
	count, err := GetDB().WithContext(ctx).Model(&CommitDataScheme{}).
		Where("round LIKE ?", "very_large_old_reg_old_%").
		Count()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, numOldRounds, "Should have old round data")

	// Delete old rounds
	startTime := time.Now()
	err = batchRepo.DeleteOldRoundDataForRegularNode(ctx, currentRound)
	duration := time.Since(startTime)
	assert.NoError(t, err, "Should successfully delete very large batch of old rounds")

	// Log performance
	t.Logf("Deleted %d old rounds in %v", count, duration)

	// Verify old rounds deleted
	oldCount, err := GetDB().WithContext(ctx).Model(&CommitDataScheme{}).
		Where("round LIKE ?", "very_large_old_reg_old_%").
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 0, oldCount, "All old rounds should be deleted")
}

// Test DeleteOldRoundDataForLeaderNode with very large dataset - stress test
func TestBatchRepository_DeleteOldRoundDataForLeaderNode_VeryLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())

	currentRound := "very_large_old_leader_current"
	numOldRounds := 100
	var cvs [32]byte
	cvs[0] = 0x0D

	// Create many old rounds with data
	for i := 0; i < numOldRounds; i++ {
		oldRound := fmt.Sprintf("very_large_old_leader_old_%d", i)
		oldCommit := &utils.LeaderCommitData{
			Round:      oldRound,
			TrialNum:   "trial1",
			EOAAddress: fmt.Sprintf("0xleader_%d", i),
			Cvs:        cvs,
		}
		err := leaderRepo.AddLeaderCommit(ctx, oldCommit)
		if err != nil {
			t.Fatalf("Failed to add commit %d: %v", i, err)
		}
	}

	// Verify old rounds exist
	count, err := GetDB().WithContext(ctx).Model(&LeaderCommitScheme{}).
		Where("round LIKE ?", "very_large_old_leader_old_%").
		Count()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, numOldRounds, "Should have old round data")

	// Delete old rounds
	startTime := time.Now()
	err = batchRepo.DeleteOldRoundDataForLeaderNode(ctx, currentRound)
	duration := time.Since(startTime)
	assert.NoError(t, err, "Should successfully delete very large batch of old rounds")

	// Log performance
	t.Logf("Deleted %d old rounds in %v", count, duration)

	// Verify old rounds deleted
	oldCount, err := GetDB().WithContext(ctx).Model(&LeaderCommitScheme{}).
		Where("round LIKE ?", "very_large_old_leader_old_%").
		Count()
	assert.NoError(t, err)
	assert.Equal(t, 0, oldCount, "All old rounds should be deleted")
}

package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestBatchRepository_DeleteRoundTrialDataForLeaderNode(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	leaderRepo := NewLeaderCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())

	round := "delete_leader_round"
	trialNum := "delete_leader_trial"

	// Cleanup first
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", round).Delete()
	GetDB().Model(&RevealOrderScheme{}).Where("round = ?", round).Delete()

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
	err := leaderRepo.AddLeaderCommit(leaderCommit)
	assert.NoError(t, err)

	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"node1"},
		RevealOrder:  []int{0},
		RV:           "rv_test",
	}
	err = revealRepo.AddRevealOrder(revealOrder)
	assert.NoError(t, err)

	// Delete
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(round, trialNum)
	assert.NoError(t, err)

	// Verify deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(round, trialNum, "0xleader")
	assert.Error(t, err)
}

func TestBatchRepository_DeleteRoundTrialDataForRegularNode(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batchRepo := NewBatchRepository(GetDB())
	regularRepo := NewRegularCommitRepository(GetDB())
	revealRepo := NewRevealOrderRepository(GetDB())

	round := "delete_regular_round"
	trialNum := "delete_regular_trial"

	// Cleanup first
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", round).Delete()
	GetDB().Model(&RevealOrderScheme{}).Where("round = ?", round).Delete()

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
	err := regularRepo.AddCommit(commit)
	assert.NoError(t, err)

	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trialNum,
		OrderedNodes: []string{"node1"},
		RevealOrder:  []int{0},
		RV:           "rv_test",
	}
	err = revealRepo.AddRevealOrder(revealOrder)
	assert.NoError(t, err)

	// Delete
	err = batchRepo.DeleteRoundTrialDataForRegularNode(round, trialNum)
	assert.NoError(t, err)

	// Verify deleted
	_, err = regularRepo.GetCommitByRound(round, trialNum)
	assert.Error(t, err)
}

func TestBatchRepository_DeleteOldRoundsForLeaderNode(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	err := leaderRepo.AddLeaderCommit(currentCommit)
	assert.NoError(t, err)

	oldCommit := &utils.LeaderCommitData{
		Round:      oldRound,
		TrialNum:   "trial1",
		EOAAddress: "0xleader_old",
		Cvs:        cvs,
	}
	err = leaderRepo.AddLeaderCommit(oldCommit)
	assert.NoError(t, err)

	// Delete old
	err = batchRepo.DeleteOldRoundDataForLeaderNode(currentRound)
	assert.NoError(t, err)

	// Verify current exists, old deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(currentRound, "trial1", "0xleader")
	assert.NoError(t, err)

	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(oldRound, "trial1", "0xleader_old")
	assert.Error(t, err)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round = ?", currentRound).Delete()
}

func TestBatchRepository_DeleteOldRoundsForRegularNode(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	err := regularRepo.AddCommit(currentCommit)
	assert.NoError(t, err)

	oldCommit := &utils.CommitData{
		Round:    oldRound,
		TrialNum: "trial1",
		Cvs:      cvs,
	}
	err = regularRepo.AddCommit(oldCommit)
	assert.NoError(t, err)

	// Delete old
	err = batchRepo.DeleteOldRoundDataForRegularNode(currentRound)
	assert.NoError(t, err)

	// Verify current exists, old deleted
	_, err = regularRepo.GetCommitByRound(currentRound, "trial1")
	assert.NoError(t, err)

	_, err = regularRepo.GetCommitByRound(oldRound, "trial1")
	assert.Error(t, err)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", currentRound).Delete()
}

// Test deleting multiple rounds and trials for leader node
func TestBatchRepository_DeleteMultipleRoundsLeaderNode(t *testing.T) {
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
		err := leaderRepo.AddLeaderCommit(commit)
		assert.NoError(t, err)
	}

	// Delete one specific round-trial combination
	err := batchRepo.DeleteRoundTrialDataForLeaderNode("multi_round_1", "trial_1")
	assert.NoError(t, err)

	// Verify only the specific one is deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr("multi_round_1", "trial_1", "0xeoa1")
	assert.Error(t, err, "Should be deleted")

	// Others should still exist
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr("multi_round_1", "trial_2", "0xeoa2")
	assert.NoError(t, err, "Should still exist")

	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr("multi_round_2", "trial_1", "0xeoa3")
	assert.NoError(t, err, "Should still exist")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round LIKE ?", "multi_round_%").Delete()
}

// Test deleting multiple rounds and trials for regular node
func TestBatchRepository_DeleteMultipleRoundsRegularNode(t *testing.T) {
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
	err := regularRepo.AddCommit(commit1)
	assert.NoError(t, err)

	commit2 := &utils.CommitData{
		Round:    round,
		TrialNum: trial2,
		Cvs:      cvs,
	}
	err = regularRepo.AddCommit(commit2)
	assert.NoError(t, err)

	// Add peer commit data
	peerData := &PeerCommitDataScheme{
		Round:      round,
		TrialNum:   trial1,
		EOAAddress: "0xpeer1",
		Cvs:        cvs[:],
	}
	err = peerRepo.AddPeerCommitData(peerData)
	assert.NoError(t, err)

	// Delete one trial
	err = batchRepo.DeleteRoundTrialDataForRegularNode(round, trial1)
	assert.NoError(t, err)

	// Verify trial1 deleted, trial2 exists
	_, err = regularRepo.GetCommitByRound(round, trial1)
	assert.Error(t, err, "Trial 1 should be deleted")

	_, err = regularRepo.GetCommitByRound(round, trial2)
	assert.NoError(t, err, "Trial 2 should still exist")

	_, err = peerRepo.GetPeerCommitData(round, trial1, "0xpeer1")
	assert.Error(t, err, "Peer commit should be deleted")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round = ?", round).Delete()
}

// Test deletion with empty database (no records to delete)
func TestBatchRepository_DeleteNonExistentData(t *testing.T) {
	batchRepo := NewBatchRepository(GetDB())

	// These should not error even if no records exist
	err := batchRepo.DeleteRoundTrialDataForLeaderNode("nonexistent_round", "nonexistent_trial")
	assert.NoError(t, err, "Should not error when deleting non-existent data")

	err = batchRepo.DeleteRoundTrialDataForRegularNode("nonexistent_round", "nonexistent_trial")
	assert.NoError(t, err, "Should not error when deleting non-existent data")

	err = batchRepo.DeleteOldRoundDataForLeaderNode("current_round_999")
	assert.NoError(t, err, "Should not error when no old data exists")

	err = batchRepo.DeleteOldRoundDataForRegularNode("current_round_999")
	assert.NoError(t, err, "Should not error when no old data exists")
}

// Test deletion preserves current round data correctly
func TestBatchRepository_PreservesCurrentRoundData(t *testing.T) {
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
	err := leaderRepo.AddLeaderCommit(currentCommit)
	assert.NoError(t, err)

	// Add old rounds data
	for _, oldRound := range oldRounds {
		oldCommit := &utils.LeaderCommitData{
			Round:      oldRound,
			TrialNum:   "trial_old",
			EOAAddress: "0xold_" + oldRound,
			Cvs:        cvs,
		}
		err := leaderRepo.AddLeaderCommit(oldCommit)
		assert.NoError(t, err)
	}

	// Delete old rounds
	err = batchRepo.DeleteOldRoundDataForLeaderNode(currentRound)
	assert.NoError(t, err)

	// Verify current round still exists
	retrieved, err := leaderRepo.GetLeaderCommitByRoundAndEoaAddr(currentRound, "trial_current", "0xcurrent")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, currentRound, retrieved.Round)

	// Verify all old rounds are deleted
	for _, oldRound := range oldRounds {
		_, err := leaderRepo.GetLeaderCommitByRoundAndEoaAddr(oldRound, "trial_old", "0xold_"+oldRound)
		assert.Error(t, err, "Old round %s should be deleted", oldRound)
	}

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round LIKE ?", "preserve_%").Delete()
}

// Test with BroadcastTracker data to ensure all tables are deleted in leader node
func TestBatchRepository_DeleteWithBroadcastTrackerLeader(t *testing.T) {
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
	err := leaderRepo.AddLeaderCommit(commit)
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
	err = broadcastRepo.AddBroadcastTracker(tracker)
	assert.NoError(t, err)

	// Add reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trial,
		OrderedNodes: []string{"node1", "node2"},
		RevealOrder:  []int{0, 1},
		RV:           "rv_broadcast",
	}
	err = revealRepo.AddRevealOrder(revealOrder)
	assert.NoError(t, err)

	// Delete all data for this round-trial
	err = batchRepo.DeleteRoundTrialDataForLeaderNode(round, trial)
	assert.NoError(t, err)

	// Verify all data deleted
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(round, trial, "0xleader_broadcast")
	assert.Error(t, err)

	trackers, err := broadcastRepo.GetBroadcastTrackers()
	assert.NoError(t, err)
	for _, tracker := range trackers {
		if tracker.Round == round && tracker.TrialNum == trial {
			assert.Fail(t, "Broadcast tracker should be deleted")
		}
	}

	_, err = revealRepo.GetRevealOrder(round, trial)
	assert.Error(t, err)
}

// Test with all tables for regular node deletion
func TestBatchRepository_DeleteWithAllTablesRegular(t *testing.T) {
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
	err := regularRepo.AddCommit(commit)
	assert.NoError(t, err)

	// Add peer commit
	peerData := &PeerCommitDataScheme{
		Round:      round,
		TrialNum:   trial,
		EOAAddress: "0xpeer_all",
		Cvs:        cvs[:],
	}
	err = peerRepo.AddPeerCommitData(peerData)
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
	err = broadcastRepo.AddBroadcastTracker(tracker)
	assert.NoError(t, err)

	// Add reveal order
	revealOrder := &utils.RevealOrderData{
		Round:        round,
		TrialNum:     trial,
		OrderedNodes: []string{"node1"},
		RevealOrder:  []int{0},
		RV:           "rv_all",
	}
	err = revealRepo.AddRevealOrder(revealOrder)
	assert.NoError(t, err)

	// Delete all data
	err = batchRepo.DeleteRoundTrialDataForRegularNode(round, trial)
	assert.NoError(t, err)

	// Verify all deleted
	_, err = regularRepo.GetCommitByRound(round, trial)
	assert.Error(t, err)

	_, err = peerRepo.GetPeerCommitData(round, trial, "0xpeer_all")
	assert.Error(t, err)

	_, err = revealRepo.GetRevealOrder(round, trial)
	assert.Error(t, err)

	trackers, err := broadcastRepo.GetBroadcastTrackers()
	assert.NoError(t, err)
	for _, tr := range trackers {
		if tr.Round == round && tr.TrialNum == trial {
			assert.Fail(t, "Broadcast tracker should be deleted")
		}
	}
}

// Test deletion with multiple old rounds exercising all delete paths
func TestBatchRepository_MultipleOldRoundsForRegularNode(t *testing.T) {
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
	err := regularRepo.AddCommit(currentCommit)
	assert.NoError(t, err)

	// Add old rounds data with all table types
	for _, oldRound := range oldRounds {
		// Regular commit
		oldCommit := &utils.CommitData{
			Round:    oldRound,
			TrialNum: "trial",
			Cvs:      cvs,
		}
		err := regularRepo.AddCommit(oldCommit)
		assert.NoError(t, err)

		// Peer commit
		peerData := &PeerCommitDataScheme{
			Round:      oldRound,
			TrialNum:   "trial",
			EOAAddress: "0xpeer_" + oldRound,
			Cvs:        cvs[:],
		}
		err = peerRepo.AddPeerCommitData(peerData)
		assert.NoError(t, err)

		// Reveal order
		revealOrder := &utils.RevealOrderData{
			Round:        oldRound,
			TrialNum:     "trial",
			OrderedNodes: []string{"node"},
			RevealOrder:  []int{0},
			RV:           "rv_" + oldRound,
		}
		err = revealRepo.AddRevealOrder(revealOrder)
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
		err = broadcastRepo.AddBroadcastTracker(tracker)
		assert.NoError(t, err)
	}

	// Delete old rounds - this exercises all 4 delete operations in DeleteOldRoundDataForRegularNode
	err = batchRepo.DeleteOldRoundDataForRegularNode(currentRound)
	assert.NoError(t, err)

	// Verify current round data still exists
	_, err = regularRepo.GetCommitByRound(currentRound, "trial")
	assert.NoError(t, err)

	// Verify all old rounds data deleted
	for _, oldRound := range oldRounds {
		_, err := regularRepo.GetCommitByRound(oldRound, "trial")
		assert.Error(t, err, "Old regular commit should be deleted")

		_, err = peerRepo.GetPeerCommitData(oldRound, "trial", "0xpeer_"+oldRound)
		assert.Error(t, err, "Old peer commit should be deleted")

		_, err = revealRepo.GetRevealOrder(oldRound, "trial")
		assert.Error(t, err, "Old reveal order should be deleted")
	}

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).Where("round LIKE ?", "multi_old_%").Delete()
	GetDB().Model(&BroadcastTrackerScheme{}).Where("round LIKE ?", "multi_old_%").Delete()
}

// Test deletion with multiple old rounds for leader node
func TestBatchRepository_MultipleOldRoundsForLeaderNode(t *testing.T) {
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
	err := leaderRepo.AddLeaderCommit(currentCommit)
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
		err := leaderRepo.AddLeaderCommit(oldCommit)
		assert.NoError(t, err)

		// Reveal order
		revealOrder := &utils.RevealOrderData{
			Round:        oldRound,
			TrialNum:     "trial",
			OrderedNodes: []string{"node"},
			RevealOrder:  []int{0},
			RV:           "rv_" + oldRound,
		}
		err = revealRepo.AddRevealOrder(revealOrder)
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
		err = broadcastRepo.AddBroadcastTracker(tracker)
		assert.NoError(t, err)
	}

	// Delete old rounds - exercises all 3 delete operations in DeleteOldRoundDataForLeaderNode
	err = batchRepo.DeleteOldRoundDataForLeaderNode(currentRound)
	assert.NoError(t, err)

	// Verify current round still exists
	_, err = leaderRepo.GetLeaderCommitByRoundAndEoaAddr(currentRound, "trial", "0xcurrent")
	assert.NoError(t, err)

	// Verify all old rounds deleted
	for _, oldRound := range oldRounds {
		_, err := leaderRepo.GetLeaderCommitByRoundAndEoaAddr(oldRound, "trial", "0xleader_"+oldRound)
		assert.Error(t, err, "Old leader commit should be deleted")

		_, err = revealRepo.GetRevealOrder(oldRound, "trial")
		assert.Error(t, err, "Old reveal order should be deleted")
	}

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).Where("round LIKE ?", "leader_%").Delete()
	GetDB().Model(&BroadcastTrackerScheme{}).Where("round LIKE ?", "leader_%").Delete()
}

// Test error handling by temporarily dropping a table for leader node round-trial deletion
func TestBatchRepository_DeleteRoundTrialDataForLeaderNode_ErrorHandling(t *testing.T) {
	batchRepo := NewBatchRepository(GetDB())

	// Drop the broadcast_tracker_schemes table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS broadcast_tracker_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to delete - should get an error because table doesn't exist
	err = batchRepo.DeleteRoundTrialDataForLeaderNode("test_round", "test_trial")
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
	batchRepo := NewBatchRepository(GetDB())

	// Drop the peer_commit_data_schemes table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS peer_commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to delete - should get an error because table doesn't exist
	err = batchRepo.DeleteRoundTrialDataForRegularNode("test_round", "test_trial")
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
	batchRepo := NewBatchRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	// Test error in first delete operation (leader_commit_schemes)
	_, err := GetDB().Exec("DROP TABLE IF EXISTS leader_commit_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteOldRoundDataForLeaderNode("current_round")
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

	err = batchRepo.DeleteOldRoundDataForLeaderNode("current_round")
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

	err = batchRepo.DeleteOldRoundDataForLeaderNode("current_round")
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
	batchRepo := NewBatchRepository(GetDB())
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"

	// Test error in first delete operation (commit_data_schemes)
	_, err := GetDB().Exec("DROP TABLE IF EXISTS commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	err = batchRepo.DeleteOldRoundDataForRegularNode("current_round")
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

	err = batchRepo.DeleteOldRoundDataForRegularNode("current_round")
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

	err = batchRepo.DeleteOldRoundDataForRegularNode("current_round")
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

	err = batchRepo.DeleteOldRoundDataForRegularNode("current_round")
	assert.Error(t, err, "Expected error when broadcast_tracker_schemes table is missing")
	assert.Contains(t, err.Error(), "failed to delete from broadcast_tracker_schemes", "Error should mention broadcast_tracker_schemes")

	// Restore schema
	sqlDB, _ = sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

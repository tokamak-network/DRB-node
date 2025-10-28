package database

import (
	"context"
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

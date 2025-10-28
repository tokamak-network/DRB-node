package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestRegularCommitRepository_AddGetUpdate(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "test_round"
	trialNum := "test_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01
	cos[0] = 0x02
	secretValue[0] = 0x03

	commitData := &utils.CommitData{
		Round:           round,
		TrialNum:        trialNum,
		Cvs:             cvs,
		Cos:             cos,
		SecretValue:     secretValue,
		SendToLeader:    true,
		SendCosToLeader: false,
	}

	// Add
	err := repo.AddCommit(commitData)
	assert.NoError(t, err)

	// Get
	fetched, err := repo.GetCommitByRound(round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.True(t, fetched.SendToLeader)

	// Update
	commitData.SendCosToLeader = true
	err = repo.UpdateCommit(commitData)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRegularCommitRepository_AddWithEmptyFields(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	var cvs [32]byte

	// Empty round
	emptyRound := &utils.CommitData{
		Round:    "",
		TrialNum: "trial1",
		Cvs:      cvs,
		Cos:      cvs,
		SecretValue: cvs,
	}
	err := repo.AddCommit(emptyRound)
	assert.Error(t, err, "Should reject empty round")

	// Empty trialNum
	emptyTrial := &utils.CommitData{
		Round:    "round1",
		TrialNum: "",
		Cvs:      cvs,
		Cos:      cvs,
		SecretValue: cvs,
	}
	err = repo.AddCommit(emptyTrial)
	assert.Error(t, err, "Should reject empty trialNum")
}

func TestRegularCommitRepository_DuplicateKey(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "dup_round"
	trialNum := "dup_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	commitData := &utils.CommitData{
		Round:    round,
		TrialNum: trialNum,
		Cvs:      cvs,
		Cos:      cvs,
		SecretValue: cvs,
	}

	err := repo.AddCommit(commitData)
	assert.NoError(t, err)

	// Try duplicate
	err = repo.AddCommit(commitData)
	assert.Error(t, err, "Should reject duplicate composite key")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRegularCommitRepository_GetNonExistent(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	_, err := repo.GetCommitByRound("nonexistent", "nonexistent")
	assert.Error(t, err, "Should return error for non-existent")
}

func TestRegularCommitRepository_UpdateNonExistent(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	var cvs [32]byte

	nonExistent := &utils.CommitData{
		Round:    "nonexistent",
		TrialNum: "nonexistent",
		Cvs:      cvs,
		Cos:      cvs,
		SecretValue: cvs,
	}

	err := repo.UpdateCommit(nonExistent)
	assert.NoError(t, err, "Update succeeds but updates 0 rows")
}

func TestRegularCommitRepository_WithZeroValueArrays(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "zero_round"
	trialNum := "zero_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	commitData := &utils.CommitData{
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         [32]byte{},
		Cos:         [32]byte{},
		SecretValue: [32]byte{},
	}

	err := repo.AddCommit(commitData)
	assert.NoError(t, err, "Should allow zero-valued arrays")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRegularCommitRepository_BooleanFlags(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "bool_round"
	trialNum := "bool_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	testCases := []struct {
		sendToLeader    bool
		sendCosToLeader bool
	}{
		{true, true},
		{true, false},
		{false, true},
		{false, false},
	}

	for _, tc := range testCases {
		// Cleanup each iteration
		GetDB().Model(&CommitDataScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Delete()

		commitData := &utils.CommitData{
			Round:           round,
			TrialNum:        trialNum,
			Cvs:             cvs,
			Cos:             cvs,
			SecretValue:     cvs,
			SendToLeader:    tc.sendToLeader,
			SendCosToLeader: tc.sendCosToLeader,
		}

		err := repo.AddCommit(commitData)
		assert.NoError(t, err)

		fetched, err := repo.GetCommitByRound(round, trialNum)
		assert.NoError(t, err)
		assert.Equal(t, tc.sendToLeader, fetched.SendToLeader)
		assert.Equal(t, tc.sendCosToLeader, fetched.SendCosToLeader)
	}

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRegularCommitRepository_PartialFieldUpdates(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "partial_round"
	trialNum := "partial_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01
	cos[0] = 0x02
	secretValue[0] = 0x03

	commitData := &utils.CommitData{
		Round:           round,
		TrialNum:        trialNum,
		Cvs:             cvs,
		Cos:             cos,
		SecretValue:     secretValue,
		SendToLeader:    false,
		SendCosToLeader: false,
	}

	err := repo.AddCommit(commitData)
	assert.NoError(t, err)

	// Update only SendToLeader flag
	commitData.SendToLeader = true
	err = repo.UpdateCommit(commitData)
	assert.NoError(t, err)

	fetched, err := repo.GetCommitByRound(round, trialNum)
	assert.NoError(t, err)
	assert.True(t, fetched.SendToLeader)
	assert.False(t, fetched.SendCosToLeader)

	// Update SendCosToLeader flag
	commitData.SendCosToLeader = true
	err = repo.UpdateCommit(commitData)
	assert.NoError(t, err)

	fetched, err = repo.GetCommitByRound(round, trialNum)
	assert.NoError(t, err)
	assert.True(t, fetched.SendToLeader)
	assert.True(t, fetched.SendCosToLeader)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestRegularCommitRepository_UpdateWithEmptySignatureFields(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "sig_round"
	trialNum := "sig_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	commitData := &utils.CommitData{
		Round:    round,
		TrialNum: trialNum,
		Cvs:      cvs,
		Cos:      cvs,
		SecretValue: cvs,
		Sign: utils.SignInfo{
			R: "",
			S: "",
			V: "",
		},
	}

	err := repo.AddCommit(commitData)
	assert.NoError(t, err, "Should allow empty signature fields")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

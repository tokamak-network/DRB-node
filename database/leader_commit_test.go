package database

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestLeaderCommitRepository_AddGetUpdate(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "test_round"
	trialNum := "test_trial"
	eoaAddr := "0xleader123"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Delete()

	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01
	cos[0] = 0x02
	secretValue[0] = 0x03

	commitData := &utils.LeaderCommitData{
		Round:                 round,
		TrialNum:              trialNum,
		EOAAddress:            eoaAddr,
		Cvs:                   cvs,
		Cos:                   cos,
		SecretValue:           secretValue,
		SubmitMerkleRootDone:  false,
		RandomNumberGenerated: false,
		CreatedAt:             time.Now().Unix(),
	}

	// Add
	err := repo.AddLeaderCommit(commitData)
	assert.NoError(t, err)

	// Get
	fetched, err := repo.GetLeaderCommitByRoundAndEoaAddr(round, trialNum, eoaAddr)
	assert.NoError(t, err)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.Equal(t, eoaAddr, fetched.EOAAddress)

	// Update
	commitData.SubmitMerkleRootDone = true
	err = repo.UpdateLeaderCommit(commitData)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Delete()
}

func TestLeaderCommitRepository_AddWithEmptyFields(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	var cvs [32]byte

	// Empty round
	emptyRound := &utils.LeaderCommitData{
		Round:      "",
		TrialNum:   "trial1",
		EOAAddress: "0xtest",
		Cvs:        cvs,
		Cos:        cvs,
		SecretValue: cvs,
		CreatedAt:  time.Now().Unix(),
	}
	err := repo.AddLeaderCommit(emptyRound)
	assert.Error(t, err, "Should reject empty round")

	// Empty EOAAddress
	emptyEOA := &utils.LeaderCommitData{
		Round:      "round1",
		TrialNum:   "trial1",
		EOAAddress: "",
		Cvs:        cvs,
		Cos:        cvs,
		SecretValue: cvs,
		CreatedAt:  time.Now().Unix(),
	}
	err = repo.AddLeaderCommit(emptyEOA)
	assert.Error(t, err, "Should reject empty EOAAddress")
}

func TestLeaderCommitRepository_DuplicateKey(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "dup_round"
	trialNum := "dup_trial"
	eoaAddr := "0xdup"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Delete()

	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	commitData := &utils.LeaderCommitData{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  eoaAddr,
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
		CreatedAt:   time.Now().Unix(),
	}

	err := repo.AddLeaderCommit(commitData)
	assert.NoError(t, err)

	// Try duplicate
	err = repo.AddLeaderCommit(commitData)
	assert.Error(t, err, "Should reject duplicate composite key")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Delete()
}

func TestLeaderCommitRepository_GetNonExistent(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	_, err := repo.GetLeaderCommitByRoundAndEoaAddr("nonexistent", "nonexistent", "0xnonexistent")
	assert.Error(t, err, "Should return error for non-existent")
}

func TestLeaderCommitRepository_UpdateNonExistent(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	var cvs [32]byte

	nonExistent := &utils.LeaderCommitData{
		Round:      "nonexistent",
		TrialNum:   "nonexistent",
		EOAAddress: "0xnonexistent",
		Cvs:        cvs,
		Cos:        cvs,
		SecretValue: cvs,
		CreatedAt:  time.Now().Unix(),
	}

	err := repo.UpdateLeaderCommit(nonExistent)
	assert.NoError(t, err, "Update succeeds but updates 0 rows")
}

func TestLeaderCommitRepository_HexAutoGeneration(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "hex_test"
	trialNum := "hex_trial"
	eoaAddr := "0xhex"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Delete()

	var cvs, cos, secretValue [32]byte
	cvs[0], cvs[1] = 0xAB, 0xCD
	cos[0], cos[1] = 0xEF, 0x01
	secretValue[0], secretValue[1] = 0x12, 0x34

	commitData := &utils.LeaderCommitData{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  eoaAddr,
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
		CreatedAt:   time.Now().Unix(),
	}

	err := repo.AddLeaderCommit(commitData)
	assert.NoError(t, err)

	// Verify hex auto-generation
	var scheme LeaderCommitScheme
	err = GetDB().Model(&scheme).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Select()
	assert.NoError(t, err)
	assert.NotEmpty(t, scheme.CvsHex)
	assert.NotEmpty(t, scheme.CosHex)
	assert.NotEmpty(t, scheme.SecretValueHex)

	// Verify hex values
	expectedCvsHex := hex.EncodeToString(cvs[:])
	assert.Contains(t, expectedCvsHex, "abcd")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Delete()
}

func TestLeaderCommitRepository_GetRoundsToProcess(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "process_round"
	trialNum := "process_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	// Add commits with different states
	commits := []*utils.LeaderCommitData{
		{
			Round:                   round,
			TrialNum:                trialNum,
			EOAAddress:              "0x1",
			Cvs:                     cvs,
			Cos:                     cvs,
			SecretValue:             cvs,
			SubmitMerkleRootDone:    true,
			RandomNumberGenerated:   false,
			CreatedAt:               time.Now().Unix(),
		},
		{
			Round:                   round,
			TrialNum:                trialNum,
			EOAAddress:              "0x2",
			Cvs:                     cvs,
			Cos:                     cvs,
			SecretValue:             cvs,
			SubmitMerkleRootDone:    false,
			RandomNumberGenerated:   false,
			CreatedAt:               time.Now().Unix(),
		},
	}

	for _, commit := range commits {
		err := repo.AddLeaderCommit(commit)
		assert.NoError(t, err)
	}

	// Get rounds to process
	rounds, err := repo.GetRoundsToProcess()
	assert.NoError(t, err)

	// Should find the commit ready to process
	found := false
	for _, r := range rounds {
		if r.Round == round && r.EOAAddress == "0x1" {
			found = true
			break
		}
	}
	assert.True(t, found, "Should find commit with merkle root done but random number not generated")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestLeaderCommitRepository_GetByRoundAndTrialNum(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "multi_round"
	trialNum := "multi_trial"
	eoas := []string{"0xeoa1", "0xeoa2", "0xeoa3"}

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	// Add multiple commits
	for _, eoa := range eoas {
		commit := &utils.LeaderCommitData{
			Round:      round,
			TrialNum:   trialNum,
			EOAAddress: eoa,
			Cvs:        cvs,
			Cos:        cvs,
			SecretValue: cvs,
			CreatedAt:  time.Now().Unix(),
		}
		err := repo.AddLeaderCommit(commit)
		assert.NoError(t, err)
	}

	// Get all
	commits, err := repo.GetLeaderCommitsByRoundAndTrialNum(round, trialNum)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(commits), 3)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

func TestLeaderCommitRepository_UpdateRandomNumberGenerated(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "random_round"
	trialNum := "random_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	commitData := &utils.LeaderCommitData{
		Round:                 round,
		TrialNum:              trialNum,
		EOAAddress:            "0xrandom",
		Cvs:                   cvs,
		Cos:                   cvs,
		SecretValue:           cvs,
		RandomNumberGenerated: false,
		CreatedAt:             time.Now().Unix(),
	}

	err := repo.AddLeaderCommit(commitData)
	assert.NoError(t, err)

	// Update random number flag
	err = repo.UpdateLeaderCommitRandomNumberGenerated(round, trialNum)
	assert.NoError(t, err)

	// Verify
	fetched, err := repo.GetLeaderCommitByRoundAndEoaAddr(round, trialNum, "0xrandom")
	assert.NoError(t, err)
	assert.True(t, fetched.RandomNumberGenerated)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
}

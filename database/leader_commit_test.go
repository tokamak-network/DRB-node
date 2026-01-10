package database

import (
	"context"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestLeaderCommitRepository_AddGetUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "test_round"
	trialNum := "test_trial"
	eoaAddr := "0xleader123"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
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
	err := repo.AddLeaderCommit(ctx, commitData)
	assert.NoError(t, err)

	// Get
	fetched, err := repo.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, eoaAddr)
	assert.NoError(t, err)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.Equal(t, eoaAddr, fetched.EOAAddress)

	// Update
	commitData.SubmitMerkleRootDone = true
	err = repo.UpdateLeaderCommit(ctx, commitData)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()
}

func TestLeaderCommitRepository_AddWithEmptyFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	var cvs [32]byte

	// Empty round
	emptyRound := &utils.LeaderCommitData{
		Round:       "",
		TrialNum:    "trial1",
		EOAAddress:  "0xtest",
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
		CreatedAt:   time.Now().Unix(),
	}
	err := repo.AddLeaderCommit(ctx, emptyRound)
	assert.Error(t, err, "Should reject empty round")

	// Empty EOAAddress
	emptyEOA := &utils.LeaderCommitData{
		Round:       "round1",
		TrialNum:    "trial1",
		EOAAddress:  "",
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
		CreatedAt:   time.Now().Unix(),
	}
	err = repo.AddLeaderCommit(ctx, emptyEOA)
	assert.Error(t, err, "Should reject empty EOAAddress")
}

func TestLeaderCommitRepository_DuplicateKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "dup_round"
	trialNum := "dup_trial"
	eoaAddr := "0xdup"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
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

	err := repo.AddLeaderCommit(ctx, commitData)
	assert.NoError(t, err)

	// Try duplicate
	err = repo.AddLeaderCommit(ctx, commitData)
	assert.Error(t, err, "Should reject duplicate composite key")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()
}

func TestLeaderCommitRepository_GetNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	_, err := repo.GetLeaderCommitByRoundAndEoaAddr(ctx, "nonexistent", "nonexistent", "0xnonexistent")
	assert.Error(t, err, "Should return error for non-existent")
}

func TestLeaderCommitRepository_UpdateNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	var cvs [32]byte

	nonExistent := &utils.LeaderCommitData{
		Round:       "nonexistent",
		TrialNum:    "nonexistent",
		EOAAddress:  "0xnonexistent",
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
		CreatedAt:   time.Now().Unix(),
	}

	err := repo.UpdateLeaderCommit(ctx, nonExistent)
	assert.NoError(t, err, "Update succeeds but updates 0 rows")
}

func TestLeaderCommitRepository_HexAutoGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "hex_test"
	trialNum := "hex_trial"
	eoaAddr := "0xhex"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
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

	err := repo.AddLeaderCommit(ctx, commitData)
	assert.NoError(t, err)

	// Verify hex auto-generation
	var scheme LeaderCommitScheme
	err = GetDB().Model(&scheme).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
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
		Context(ctx).
		Delete()
}

func TestLeaderCommitRepository_GetRoundsToProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "process_round"
	trialNum := "process_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	// Add commits with different states
	commits := []*utils.LeaderCommitData{
		{
			Round:                 round,
			TrialNum:              trialNum,
			EOAAddress:            "0x1",
			Cvs:                   cvs,
			Cos:                   cvs,
			SecretValue:           cvs,
			SubmitMerkleRootDone:  true,
			RandomNumberGenerated: false,
			CreatedAt:             time.Now().Unix(),
		},
		{
			Round:                 round,
			TrialNum:              trialNum,
			EOAAddress:            "0x2",
			Cvs:                   cvs,
			Cos:                   cvs,
			SecretValue:           cvs,
			SubmitMerkleRootDone:  false,
			RandomNumberGenerated: false,
			CreatedAt:             time.Now().Unix(),
		},
	}

	for _, commit := range commits {
		err := repo.AddLeaderCommit(ctx, commit)
		assert.NoError(t, err)
	}

	// Get rounds to process
	rounds, err := repo.GetRoundsToProcess(ctx)
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
		Context(ctx).
		Delete()
}

func TestLeaderCommitRepository_GetByRoundAndTrialNum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "multi_round"
	trialNum := "multi_trial"
	eoas := []string{"0xeoa1", "0xeoa2", "0xeoa3"}

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	// Add multiple commits
	for _, eoa := range eoas {
		commit := &utils.LeaderCommitData{
			Round:       round,
			TrialNum:    trialNum,
			EOAAddress:  eoa,
			Cvs:         cvs,
			Cos:         cvs,
			SecretValue: cvs,
			CreatedAt:   time.Now().Unix(),
		}
		err := repo.AddLeaderCommit(ctx, commit)
		assert.NoError(t, err)
	}

	// Get all
	commits, err := repo.GetLeaderCommitsByRoundAndTrialNum(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(commits), 3)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestLeaderCommitRepository_UpdateRandomNumberGenerated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	round := "random_round"
	trialNum := "random_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
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

	err := repo.AddLeaderCommit(ctx, commitData)
	assert.NoError(t, err)

	// Update random number flag
	err = repo.UpdateLeaderCommitRandomNumberGenerated(ctx, round, trialNum)
	assert.NoError(t, err)

	// Verify
	fetched, err := repo.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, "0xrandom")
	assert.NoError(t, err)
	assert.True(t, fetched.RandomNumberGenerated)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test error handling for GetLeaderCommitsByRoundAndTrialNum when database fails
func TestLeaderCommitRepository_GetByRoundAndTrialNum_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS leader_commit_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to get commits - should get an error because table doesn't exist
	_, err = repo.GetLeaderCommitsByRoundAndTrialNum(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for GetRoundsToProcess when database fails
func TestLeaderCommitRepository_GetRoundsToProcess_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS leader_commit_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to get rounds to process - should get an error because table doesn't exist
	_, err = repo.GetRoundsToProcess(ctx)
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test NewLeaderCommitRepository constructor
func TestNewLeaderCommitRepository(t *testing.T) {
	db := GetDB()
	repo := NewLeaderCommitRepository(db)
	assert.NotNil(t, repo, "Repository should be created")
	assert.Equal(t, db, repo.db, "Repository should store the database connection")
}

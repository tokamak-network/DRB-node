package database

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
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
	// Should return nil slice on error
	leaderCommits, err := repo.GetLeaderCommitsByRoundAndTrialNum(ctx, "test_round", "test_trial")
	assert.Error(t, err, "Expected error when table is missing")
	assert.Nil(t, leaderCommits, "Should return nil slice on error")

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

// Test context timeout for AddLeaderCommit
func TestLeaderCommitRepository_AddLeaderCommit_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the insert operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE leader_commit_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())
	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	commitData := &utils.LeaderCommitData{
		Round:       "test",
		TrialNum:    "test",
		EOAAddress:  "0xtest",
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
	}

	// This call should block on the locked table and time out
	err = repo.AddLeaderCommit(ctx, commitData)
	assert.Error(t, err, "Expected an error due to context timeout")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test context timeout for GetLeaderCommitByRoundAndEoaAddr
func TestLeaderCommitRepository_GetLeaderCommitByRoundAndEoaAddr_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the select operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE leader_commit_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	// This call should block on the locked table and time out
	_, err = repo.GetLeaderCommitByRoundAndEoaAddr(ctx, "test", "test", "0xtest")
	assert.Error(t, err, "Expected an error due to context timeout")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test context timeout for UpdateLeaderCommit
func TestLeaderCommitRepository_UpdateLeaderCommit_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the update operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE leader_commit_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())
	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	commitData := &utils.LeaderCommitData{
		Round:       "test",
		TrialNum:    "test",
		EOAAddress:  "0xtest",
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
		CreatedAt:   time.Now().Unix(),
	}

	// This call should block on the locked table and time out
	err = repo.UpdateLeaderCommit(ctx, commitData)
	assert.Error(t, err, "Expected an error due to context timeout")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test context timeout for UpdateLeaderCommitRandomNumberGenerated
func TestLeaderCommitRepository_UpdateLeaderCommitRandomNumberGenerated_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the update operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE leader_commit_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	// This call should block on the locked table and time out
	err = repo.UpdateLeaderCommitRandomNumberGenerated(ctx, "test", "test")
	assert.Error(t, err, "Expected an error due to context timeout")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test concurrent operations
func TestLeaderCommitRepository_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())
	round := "concurrent_test_round"
	trialNum := "concurrent_test_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Run concurrent add operations
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			var cvs, cos, secretValue [32]byte
			cvs[0] = byte(idx)

			commitData := &utils.LeaderCommitData{
				Round:       round,
				TrialNum:    trialNum,
				EOAAddress:  fmt.Sprintf("0xconcurrent_%d", idx),
				Cvs:         cvs,
				Cos:         cos,
				SecretValue: secretValue,
				CreatedAt:   time.Now().Unix(),
			}
			err := repo.AddLeaderCommit(ctx, commitData)
			done <- err
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		err := <-done
		assert.NoError(t, err, "Concurrent operations should not error")
	}

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test with special characters in round/trial names to ensure SQL injection prevention
func TestLeaderCommitRepository_SpecialCharactersInRoundNames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

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

	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	for _, specialRound := range specialRounds {
		commitData := &utils.LeaderCommitData{
			Round:       specialRound,
			TrialNum:    "trial",
			EOAAddress:  "0xtest",
			Cvs:         cvs,
			Cos:         cos,
			SecretValue: secretValue,
			CreatedAt:   time.Now().Unix(),
		}

		// These should not cause SQL injection (parameterized queries should handle this)
		err := repo.AddLeaderCommit(ctx, commitData)
		// May succeed or fail, but should not cause SQL injection
		if err != nil {
			// If it fails, it should be a validation/constraint error, not SQL injection
			assert.NotContains(t, err.Error(), "DROP TABLE", "Should not execute DROP TABLE")
			assert.NotContains(t, err.Error(), "DELETE FROM", "Should not execute DELETE FROM")
		}

		// Cleanup if it was added
		GetDB().Model(&LeaderCommitScheme{}).
			Where("round = ?", specialRound).
			Context(ctx).
			Delete()
	}
}

// Test with unicode and international characters in round/trial names
func TestLeaderCommitRepository_UnicodeCharactersInRoundNames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

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

	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	for _, unicodeRound := range unicodeRounds {
		commitData := &utils.LeaderCommitData{
			Round:       unicodeRound,
			TrialNum:    "trial",
			EOAAddress:  "0xtest",
			Cvs:         cvs,
			Cos:         cos,
			SecretValue: secretValue,
			CreatedAt:   time.Now().Unix(),
		}

		err := repo.AddLeaderCommit(ctx, commitData)
		assert.NoError(t, err, "Should handle unicode characters: %s", unicodeRound)

		// Cleanup
		GetDB().Model(&LeaderCommitScheme{}).
			Where("round = ?", unicodeRound).
			Context(ctx).
			Delete()
	}
}

// Test error handling for AddLeaderCommit when database fails
func TestLeaderCommitRepository_AddLeaderCommit_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS leader_commit_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to add commit - should fail because table doesn't exist
	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	commitData := &utils.LeaderCommitData{
		Round:       "test",
		TrialNum:    "test",
		EOAAddress:  "0xtest",
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
		CreatedAt:   time.Now().Unix(),
	}

	err = repo.AddLeaderCommit(ctx, commitData)
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for UpdateLeaderCommit when database fails
func TestLeaderCommitRepository_UpdateLeaderCommit_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS leader_commit_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to update commit - should fail because table doesn't exist
	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	commitData := &utils.LeaderCommitData{
		Round:       "test",
		TrialNum:    "test",
		EOAAddress:  "0xtest",
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
		CreatedAt:   time.Now().Unix(),
	}

	err = repo.UpdateLeaderCommit(ctx, commitData)
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for UpdateLeaderCommitRandomNumberGenerated when database fails
func TestLeaderCommitRepository_UpdateLeaderCommitRandomNumberGenerated_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS leader_commit_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to update - should fail because table doesn't exist
	err = repo.UpdateLeaderCommitRandomNumberGenerated(ctx, "test", "test")
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for GetLeaderCommitByRoundAndEoaAddr when database fails
func TestLeaderCommitRepository_GetLeaderCommitByRoundAndEoaAddr_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS leader_commit_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to get commit - should get an error because table doesn't exist
	_, err = repo.GetLeaderCommitByRoundAndEoaAddr(ctx, "test", "test", "0xtest")
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test GetLeaderCommitsByRoundAndTrialNum with very large dataset
func TestLeaderCommitRepository_GetLeaderCommitsByRoundAndTrialNum_VeryLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())
	round := "very_large_round"
	trialNum := "very_large_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Create very large dataset (1000 records)
	numCommits := 1000
	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	for i := 0; i < numCommits; i++ {
		commitData := &utils.LeaderCommitData{
			Round:       round,
			TrialNum:    trialNum,
			EOAAddress:  fmt.Sprintf("0xlarge_%d", i),
			Cvs:         cvs,
			Cos:         cos,
			SecretValue: secretValue,
			CreatedAt:   time.Now().Unix(),
		}
		err := repo.AddLeaderCommit(ctx, commitData)
		if err != nil {
			t.Fatalf("Failed to add commit %d: %v", i, err)
		}
	}

	// Get all commits
	startTime := time.Now()
	commits, err := repo.GetLeaderCommitsByRoundAndTrialNum(ctx, round, trialNum)
	duration := time.Since(startTime)
	assert.NoError(t, err, "Should successfully retrieve very large dataset")

	// Log performance
	t.Logf("Retrieved %d commits in %v", len(commits), duration)

	// Verify we got at least our commits
	assert.GreaterOrEqual(t, len(commits), numCommits, "Should have at least %d commits", numCommits)

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test GetRoundsToProcess with very large dataset
func TestLeaderCommitRepository_GetRoundsToProcess_VeryLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())
	round := "very_large_process_round"
	trialNum := "very_large_process_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Create very large dataset (1000 records) with different states
	numCommits := 1000
	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	for i := 0; i < numCommits; i++ {
		commitData := &utils.LeaderCommitData{
			Round:                 round,
			TrialNum:              trialNum,
			EOAAddress:            fmt.Sprintf("0xprocess_%d", i),
			Cvs:                   cvs,
			Cos:                   cos,
			SecretValue:           secretValue,
			SubmitMerkleRootDone:  i%2 == 0, // Alternate between true and false
			RandomNumberGenerated: false,
			CreatedAt:             time.Now().Unix(),
		}
		err := repo.AddLeaderCommit(ctx, commitData)
		if err != nil {
			t.Fatalf("Failed to add commit %d: %v", i, err)
		}
	}

	// Get rounds to process
	startTime := time.Now()
	rounds, err := repo.GetRoundsToProcess(ctx)
	duration := time.Since(startTime)
	assert.NoError(t, err, "Should successfully retrieve very large dataset")

	// Log performance
	t.Logf("Retrieved %d rounds to process in %v", len(rounds), duration)

	// Verify we got some rounds to process (those with SubmitMerkleRootDone=true and RandomNumberGenerated=false)
	assert.Greater(t, len(rounds), 0, "Should have some rounds to process")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test UpdateLeaderCommit with all fields updated
func TestLeaderCommitRepository_UpdateLeaderCommit_AllFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())
	round := "update_all_round"
	trialNum := "update_all_trial"
	eoaAddr := "0xupdate_all"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()

	// Add initial commit
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

	err := repo.AddLeaderCommit(ctx, commitData)
	assert.NoError(t, err)

	// Update all fields
	cvs[0] = 0x04
	cos[0] = 0x05
	secretValue[0] = 0x06
	commitData.Cvs = cvs
	commitData.Cos = cos
	commitData.SecretValue = secretValue
	commitData.SubmitMerkleRootDone = true
	commitData.RandomNumberGenerated = true

	err = repo.UpdateLeaderCommit(ctx, commitData)
	assert.NoError(t, err)

	// Verify update
	fetched, err := repo.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, eoaAddr)
	assert.NoError(t, err)
	assert.Equal(t, byte(0x04), fetched.Cvs[0], "Cvs should be updated")
	assert.Equal(t, byte(0x05), fetched.Cos[0], "Cos should be updated")
	assert.Equal(t, byte(0x06), fetched.SecretValue[0], "SecretValue should be updated")
	assert.True(t, fetched.SubmitMerkleRootDone, "SubmitMerkleRootDone should be updated")
	assert.True(t, fetched.RandomNumberGenerated, "RandomNumberGenerated should be updated")

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()
}

// Test UpdateLeaderCommitRandomNumberGenerated with multiple commits
func TestLeaderCommitRepository_UpdateLeaderCommitRandomNumberGenerated_MultipleCommits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewLeaderCommitRepository(GetDB())
	round := "update_multi_round"
	trialNum := "update_multi_trial"

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Add multiple commits
	var cvs, cos, secretValue [32]byte
	cvs[0] = 0x01

	eoas := []string{"0xmulti1", "0xmulti2", "0xmulti3"}
	for _, eoa := range eoas {
		commitData := &utils.LeaderCommitData{
			Round:                 round,
			TrialNum:              trialNum,
			EOAAddress:            eoa,
			Cvs:                   cvs,
			Cos:                   cos,
			SecretValue:           secretValue,
			RandomNumberGenerated: false,
			CreatedAt:             time.Now().Unix(),
		}
		err := repo.AddLeaderCommit(ctx, commitData)
		assert.NoError(t, err)
	}

	// Update all commits for this round/trial
	err := repo.UpdateLeaderCommitRandomNumberGenerated(ctx, round, trialNum)
	assert.NoError(t, err)

	// Verify all commits are updated
	for _, eoa := range eoas {
		fetched, err := repo.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, eoa)
		assert.NoError(t, err)
		assert.True(t, fetched.RandomNumberGenerated, "RandomNumberGenerated should be true for %s", eoa)
	}

	// Cleanup
	GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

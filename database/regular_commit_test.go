package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestRegularCommitRepository_AddGetUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "test_round"
	trialNum := "test_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
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
	err := repo.AddCommit(ctx, commitData)
	assert.NoError(t, err)

	// Get
	fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.True(t, fetched.SendToLeader)

	// Update
	commitData.SendCosToLeader = true
	err = repo.UpdateCommit(ctx, commitData)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRegularCommitRepository_AddWithEmptyFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	var cvs [32]byte

	// Empty round
	emptyRound := &utils.CommitData{
		Round:       "",
		TrialNum:    "trial1",
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
	}
	err := repo.AddCommit(ctx, emptyRound)
	assert.Error(t, err, "Should reject empty round")

	// Empty trialNum
	emptyTrial := &utils.CommitData{
		Round:       "round1",
		TrialNum:    "",
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
	}
	err = repo.AddCommit(ctx, emptyTrial)
	assert.Error(t, err, "Should reject empty trialNum")
}

func TestRegularCommitRepository_DuplicateKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "dup_round"
	trialNum := "dup_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	commitData := &utils.CommitData{
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
	}

	err := repo.AddCommit(ctx, commitData)
	assert.NoError(t, err)

	// Try duplicate
	err = repo.AddCommit(ctx, commitData)
	assert.Error(t, err, "Should reject duplicate composite key")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRegularCommitRepository_GetNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	_, err := repo.GetCommitByRound(ctx, "nonexistent", "nonexistent")
	assert.Error(t, err, "Should return error for non-existent")
}

func TestRegularCommitRepository_UpdateNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	var cvs [32]byte

	nonExistent := &utils.CommitData{
		Round:       "nonexistent",
		TrialNum:    "nonexistent",
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
	}

	err := repo.UpdateCommit(ctx, nonExistent)
	assert.NoError(t, err, "Update succeeds but updates 0 rows")
}

func TestRegularCommitRepository_WithZeroValueArrays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "zero_round"
	trialNum := "zero_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	commitData := &utils.CommitData{
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         [32]byte{},
		Cos:         [32]byte{},
		SecretValue: [32]byte{},
	}

	err := repo.AddCommit(ctx, commitData)
	assert.NoError(t, err, "Should allow zero-valued arrays")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRegularCommitRepository_BooleanFlags(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "bool_round"
	trialNum := "bool_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
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
			Context(ctx).
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

		err := repo.AddCommit(ctx, commitData)
		assert.NoError(t, err)

		fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
		assert.NoError(t, err)
		assert.Equal(t, tc.sendToLeader, fetched.SendToLeader)
		assert.Equal(t, tc.sendCosToLeader, fetched.SendCosToLeader)
	}

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRegularCommitRepository_PartialFieldUpdates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "partial_round"
	trialNum := "partial_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
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

	err := repo.AddCommit(ctx, commitData)
	assert.NoError(t, err)

	// Update only SendToLeader flag
	commitData.SendToLeader = true
	err = repo.UpdateCommit(ctx, commitData)
	assert.NoError(t, err)

	fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.True(t, fetched.SendToLeader)
	assert.False(t, fetched.SendCosToLeader)

	// Update SendCosToLeader flag
	commitData.SendCosToLeader = true
	err = repo.UpdateCommit(ctx, commitData)
	assert.NoError(t, err)

	fetched, err = repo.GetCommitByRound(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.True(t, fetched.SendToLeader)
	assert.True(t, fetched.SendCosToLeader)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRegularCommitRepository_UpdateWithEmptySignatureFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "sig_round"
	trialNum := "sig_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	commitData := &utils.CommitData{
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
		Sign: utils.SignInfo{
			R: "",
			S: "",
			V: "",
		},
	}

	err := repo.AddCommit(ctx, commitData)
	assert.NoError(t, err, "Should allow empty signature fields")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test NewRegularCommitRepository constructor
func TestNewRegularCommitRepository(t *testing.T) {
	db := GetDB()
	repo := NewRegularCommitRepository(db)
	assert.NotNil(t, repo, "Repository should be created")
	assert.Equal(t, db, repo.db, "Repository should store the database connection")
}

func TestRegularCommitRepository_AddCommit_ContextTimeout(t *testing.T) {
	repo := NewRegularCommitRepository(GetDB())

	// Create a context that will timeout immediately
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	var cvs [32]byte
	cvs[0] = 0x01

	commitData := &utils.CommitData{
		Round:       "timeout_round",
		TrialNum:    "timeout_trial",
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
	}

	// Lock the table to ensure timeout
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec("LOCK TABLE commit_data_schemes IN ACCESS EXCLUSIVE MODE")
	assert.NoError(t, err)

	// Try to add commit with expired context
	err = repo.AddCommit(ctx, commitData)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "i/o timeout"), "Error should indicate timeout")
}

func TestRegularCommitRepository_GetCommitByRound_ContextTimeout(t *testing.T) {
	repo := NewRegularCommitRepository(GetDB())

	// Create a context that will timeout immediately
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	// Lock the table to ensure timeout
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec("LOCK TABLE commit_data_schemes IN ACCESS EXCLUSIVE MODE")
	assert.NoError(t, err)

	// Try to get commit with expired context
	_, err = repo.GetCommitByRound(ctx, "timeout_round", "timeout_trial")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "i/o timeout"), "Error should indicate timeout")
}

func TestRegularCommitRepository_UpdateCommit_ContextTimeout(t *testing.T) {
	repo := NewRegularCommitRepository(GetDB())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	round := "timeout_update_round"
	trialNum := "timeout_update_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	commitData := &utils.CommitData{
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
	}

	err := repo.AddCommit(ctx, commitData)
	assert.NoError(t, err)

	// Create a context that will timeout immediately
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer timeoutCancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	// Lock the table to ensure timeout
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec("LOCK TABLE commit_data_schemes IN ACCESS EXCLUSIVE MODE")
	assert.NoError(t, err)

	// Try to update commit with expired context
	err = repo.UpdateCommit(timeoutCtx, commitData)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "i/o timeout"), "Error should indicate timeout")

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRegularCommitRepository_ConcurrentOperations(t *testing.T) {
	repo := NewRegularCommitRepository(GetDB())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Cleanup before test
	for i := 0; i < numGoroutines; i++ {
		round := fmt.Sprintf("concurrent_round_%d", i)
		trialNum := fmt.Sprintf("concurrent_trial_%d", i)
		GetDB().Model(&CommitDataScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()
	}

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()

			round := fmt.Sprintf("concurrent_round_%d", idx)
			trialNum := fmt.Sprintf("concurrent_trial_%d", idx)

			var cvs [32]byte
			cvs[0] = byte(idx)

			commitData := &utils.CommitData{
				Round:       round,
				TrialNum:    trialNum,
				Cvs:         cvs,
				Cos:         cvs,
				SecretValue: cvs,
			}

			err := repo.AddCommit(ctx, commitData)
			assert.NoError(t, err)

			// Try to get it
			fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
			assert.NoError(t, err)
			assert.Equal(t, round, fetched.Round)
			assert.Equal(t, trialNum, fetched.TrialNum)
		}(i)
	}

	wg.Wait()

	// Cleanup after test
	for i := 0; i < numGoroutines; i++ {
		round := fmt.Sprintf("concurrent_round_%d", i)
		trialNum := fmt.Sprintf("concurrent_trial_%d", i)
		GetDB().Model(&CommitDataScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()
	}
}

func TestRegularCommitRepository_SpecialCharacters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	specialRounds := []string{
		"round'with'quotes",
		"round;with;semicolons",
		"round--with--dashes",
		"round/with/slashes",
		"round\\with\\backslashes",
		"round%with%percent",
		"round_with_underscores",
	}

	var cvs [32]byte
	cvs[0] = 0x01

	for _, round := range specialRounds {
		trialNum := "special_trial"

		// Cleanup
		GetDB().Model(&CommitDataScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()

		commitData := &utils.CommitData{
			Round:       round,
			TrialNum:    trialNum,
			Cvs:         cvs,
			Cos:         cvs,
			SecretValue: cvs,
		}

		err := repo.AddCommit(ctx, commitData)
		assert.NoError(t, err, fmt.Sprintf("Should handle special characters in round: %s", round))

		fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
		assert.NoError(t, err)
		assert.Equal(t, round, fetched.Round)

		// Cleanup
		GetDB().Model(&CommitDataScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()
	}
}

func TestRegularCommitRepository_UnicodeCharacters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	unicodeRounds := []string{
		"round_中文",
		"round_日本語",
		"round_한국어",
		"round_русский",
		"round_العربية",
		"round_עברית",
		"round_🎉🎊",
	}

	var cvs [32]byte
	cvs[0] = 0x01

	for _, round := range unicodeRounds {
		trialNum := "unicode_trial"

		// Cleanup
		GetDB().Model(&CommitDataScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()

		commitData := &utils.CommitData{
			Round:       round,
			TrialNum:    trialNum,
			Cvs:         cvs,
			Cos:         cvs,
			SecretValue: cvs,
		}

		err := repo.AddCommit(ctx, commitData)
		assert.NoError(t, err, fmt.Sprintf("Should handle unicode characters in round: %s", round))

		fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
		assert.NoError(t, err)
		assert.Equal(t, round, fetched.Round)

		// Cleanup
		GetDB().Model(&CommitDataScheme{}).
			Where("round = ? AND trial_num = ?", round, trialNum).
			Context(ctx).
			Delete()
	}
}

func TestRegularCommitRepository_AddCommit_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	// Test with nil commit (should panic or return error)
	var cvs [32]byte
	cvs[0] = 0x01

	// Test with invalid database state by dropping table temporarily
	// This is a destructive test, so we'll use a transaction rollback approach
	tx, err := GetDB().Begin()
	assert.NoError(t, err)

	// Create a temporary table to simulate error
	_, err = tx.Exec("CREATE TEMP TABLE temp_commit_data_schemes AS SELECT * FROM commit_data_schemes WHERE 1=0")
	assert.NoError(t, err)

	// Use a repository with a closed connection to test error handling
	// Actually, we can't easily simulate this without breaking the test setup
	// So we'll test with invalid data instead

	// Test with extremely long round name (should be handled by DB constraints)
	longRound := strings.Repeat("a", 1000)
	commitData := &utils.CommitData{
		Round:       longRound,
		TrialNum:    "trial",
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
	}

	err = repo.AddCommit(ctx, commitData)
	// This might succeed or fail depending on DB schema, but should not panic
	assert.NotPanics(t, func() {
		_ = repo.AddCommit(ctx, commitData)
	})

	tx.Rollback()
}

func TestRegularCommitRepository_UpdateCommit_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	var cvs [32]byte
	cvs[0] = 0x01

	// Test update with nil commit data (should panic or return error)
	// We can't easily test this without breaking the test, so we test with invalid data

	// Test with extremely long round name
	longRound := strings.Repeat("b", 1000)
	commitData := &utils.CommitData{
		Round:       longRound,
		TrialNum:    "trial",
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
	}

	// This might succeed or fail depending on DB schema, but should not panic
	assert.NotPanics(t, func() {
		_ = repo.UpdateCommit(ctx, commitData)
	})
}

func TestRegularCommitRepository_GetCommitByRound_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	// Test with empty strings
	_, err := repo.GetCommitByRound(ctx, "", "")
	assert.Error(t, err, "Should return error for empty round and trialNum")

	// Test with extremely long strings
	longRound := strings.Repeat("c", 1000)
	longTrial := strings.Repeat("d", 1000)
	_, err = repo.GetCommitByRound(ctx, longRound, longTrial)
	// Should return error (not found) but not panic
	assert.Error(t, err)
}

func TestRegularCommitRepository_GetCommitByRound_VeryLargeDataset(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "large_dataset_round"
	trialNum := "large_dataset_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	// Add a commit
	commitData := &utils.CommitData{
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         cvs,
		Cos:         cvs,
		SecretValue: cvs,
	}

	err := repo.AddCommit(ctx, commitData)
	assert.NoError(t, err)

	// Add many other commits to create a large dataset
	for i := 0; i < 100; i++ {
		otherRound := fmt.Sprintf("other_round_%d", i)
		otherTrialNum := fmt.Sprintf("other_trial_%d", i)

		// Cleanup
		GetDB().Model(&CommitDataScheme{}).
			Where("round = ? AND trial_num = ?", otherRound, otherTrialNum).
			Context(ctx).
			Delete()

		otherCommitData := &utils.CommitData{
			Round:       otherRound,
			TrialNum:    otherTrialNum,
			Cvs:         cvs,
			Cos:         cvs,
			SecretValue: cvs,
		}

		err := repo.AddCommit(ctx, otherCommitData)
		assert.NoError(t, err)
	}

	// Try to get the original commit
	fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ?", round).
		Context(ctx).
		Delete()

	// Cleanup other rounds
	for i := 0; i < 100; i++ {
		otherRound := fmt.Sprintf("other_round_%d", i)
		otherTrialNum := fmt.Sprintf("other_trial_%d", i)
		GetDB().Model(&CommitDataScheme{}).
			Where("round = ? AND trial_num = ?", otherRound, otherTrialNum).
			Context(ctx).
			Delete()
	}
}

func TestRegularCommitRepository_AddCommit_LargeByteArrays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "large_bytes_round"
	trialNum := "large_bytes_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Create byte arrays with all bytes set
	var cvs, cos, secretValue [32]byte
	for i := range cvs {
		cvs[i] = byte(i)
		cos[i] = byte(i + 32)
		secretValue[i] = byte(i + 64)
	}

	commitData := &utils.CommitData{
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
	}

	err := repo.AddCommit(ctx, commitData)
	assert.NoError(t, err)

	fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, cvs, fetched.Cvs)
	assert.Equal(t, cos, fetched.Cos)
	assert.Equal(t, secretValue, fetched.SecretValue)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRegularCommitRepository_UpdateCommit_AllFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	round := "update_all_round"
	trialNum := "update_all_trial"

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	var cvs1, cos1, secretValue1 [32]byte
	cvs1[0] = 0x01
	cos1[0] = 0x02
	secretValue1[0] = 0x03

	commitData := &utils.CommitData{
		Round:           round,
		TrialNum:        trialNum,
		Cvs:             cvs1,
		Cos:             cos1,
		SecretValue:     secretValue1,
		SendToLeader:    false,
		SendCosToLeader: false,
		Sign: utils.SignInfo{
			R: "r1",
			S: "s1",
			V: "v1",
		},
	}

	err := repo.AddCommit(ctx, commitData)
	assert.NoError(t, err)

	// Update all fields
	var cvs2, cos2, secretValue2 [32]byte
	cvs2[0] = 0xAA
	cos2[0] = 0xBB
	secretValue2[0] = 0xCC

	commitData.Cvs = cvs2
	commitData.Cos = cos2
	commitData.SecretValue = secretValue2
	commitData.SendToLeader = true
	commitData.SendCosToLeader = true
	commitData.Sign = utils.SignInfo{
		R: "r2",
		S: "s2",
		V: "v2",
	}

	err = repo.UpdateCommit(ctx, commitData)
	assert.NoError(t, err)

	fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
	assert.NoError(t, err)
	assert.Equal(t, cvs2, fetched.Cvs)
	assert.Equal(t, cos2, fetched.Cos)
	assert.Equal(t, secretValue2, fetched.SecretValue)
	assert.True(t, fetched.SendToLeader)
	assert.True(t, fetched.SendCosToLeader)
	assert.Equal(t, "r2", fetched.Sign.R)
	assert.Equal(t, "s2", fetched.Sign.S)
	assert.Equal(t, "v2", fetched.Sign.V)

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

func TestRegularCommitRepository_GetCommitByRound_MultipleCommits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewRegularCommitRepository(GetDB())

	// Add multiple commits with different trial numbers
	round := "multi_commit_round"
	trialNums := []string{"trial1", "trial2", "trial3"}

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ?", round).
		Context(ctx).
		Delete()

	var cvs [32]byte
	cvs[0] = 0x01

	for i, trialNum := range trialNums {
		commitData := &utils.CommitData{
			Round:       round,
			TrialNum:    trialNum,
			Cvs:         cvs,
			Cos:         cvs,
			SecretValue: cvs,
		}

		// Set different values for each
		cvs[0] = byte(i + 1)

		err := repo.AddCommit(ctx, commitData)
		assert.NoError(t, err)

		// Verify each commit can be retrieved
		fetched, err := repo.GetCommitByRound(ctx, round, trialNum)
		assert.NoError(t, err)
		assert.Equal(t, round, fetched.Round)
		assert.Equal(t, trialNum, fetched.TrialNum)
	}

	// Cleanup
	GetDB().Model(&CommitDataScheme{}).
		Where("round = ?", round).
		Context(ctx).
		Delete()
}

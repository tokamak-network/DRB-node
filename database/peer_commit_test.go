package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPeerCommitRepository_AddGetUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	round := "test_round"
	trialNum := "test_trial"
	eoaAddr := "0xpeer123"

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()

	peerData := &PeerCommitDataScheme{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  eoaAddr,
		SecretValue: []byte{0x01, 0x02},
		Cos:         []byte{0x03, 0x04},
		Cvs:         []byte{0x05, 0x06},
	}

	// Add
	err := repo.AddPeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	// Get
	fetched, err := repo.GetPeerCommitData(ctx, round, trialNum, eoaAddr)
	assert.NoError(t, err)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.Equal(t, eoaAddr, fetched.EOAAddress)

	// Update
	peerData.SecretValue = []byte{0xFF}
	err = repo.UpdatePeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()
}

func TestPeerCommitRepository_AddWithEmptyFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	// Empty round
	emptyRound := &PeerCommitDataScheme{
		Round:      "",
		TrialNum:   "trial1",
		EOAAddress: "0xtest",
		Cvs:        []byte{0x01},
	}
	err := repo.AddPeerCommitData(ctx, emptyRound)
	assert.Error(t, err, "Should reject empty round")

	// Empty trialNum
	emptyTrial := &PeerCommitDataScheme{
		Round:      "round1",
		TrialNum:   "",
		EOAAddress: "0xtest",
		Cvs:        []byte{0x01},
	}
	err = repo.AddPeerCommitData(ctx, emptyTrial)
	assert.Error(t, err, "Should reject empty trialNum")

	// Empty EOAAddress
	emptyEOA := &PeerCommitDataScheme{
		Round:      "round1",
		TrialNum:   "trial1",
		EOAAddress: "",
		Cvs:        []byte{0x01},
	}
	err = repo.AddPeerCommitData(ctx, emptyEOA)
	assert.Error(t, err, "Should reject empty EOAAddress")
}

func TestPeerCommitRepository_DuplicateKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	round := "dup_round"
	trialNum := "dup_trial"
	eoaAddr := "0xdup"

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()

	peerData := &PeerCommitDataScheme{
		Round:      round,
		TrialNum:   trialNum,
		EOAAddress: eoaAddr,
		Cvs:        []byte{0x01},
	}

	err := repo.AddPeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	// Try duplicate
	err = repo.AddPeerCommitData(ctx, peerData)
	assert.Error(t, err, "Should reject duplicate composite key")
	assert.Contains(t, err.Error(), "duplicate key")

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()
}

func TestPeerCommitRepository_NullableByteArrays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", "nil_round", "nil_trial", "0xnil").
		Context(ctx).
		Delete()

	// Add with nil byte arrays
	nilData := &PeerCommitDataScheme{
		Round:       "nil_round",
		TrialNum:    "nil_trial",
		EOAAddress:  "0xnil",
		SecretValue: nil,
		Cos:         nil,
		Cvs:         nil,
	}
	err := repo.AddPeerCommitData(ctx, nilData)
	assert.NoError(t, err, "Should allow nil byte arrays")

	// Verify
	fetched, err := repo.GetPeerCommitData(ctx, "nil_round", "nil_trial", "0xnil")
	assert.NoError(t, err)
	assert.Nil(t, fetched.SecretValue)
	assert.Nil(t, fetched.Cos)
	assert.Nil(t, fetched.Cvs)

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", "nil_round", "nil_trial", "0xnil").
		Context(ctx).
		Delete()
}

func TestPeerCommitRepository_GetNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	_, err := repo.GetPeerCommitData(ctx, "nonexistent", "nonexistent", "0xnonexistent")
	assert.Error(t, err, "Should return error for non-existent")

	_, err = repo.GetPeerCommitData(ctx, "", "", "")
	assert.Error(t, err, "Should return error for empty params")
}

func TestPeerCommitRepository_IncrementalUpdates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", "update_test", "update_trial", "0xupdate").
		Context(ctx).
		Delete()

	// Start with nil fields
	peerData := &PeerCommitDataScheme{
		Round:       "update_test",
		TrialNum:    "update_trial",
		EOAAddress:  "0xupdate",
		SecretValue: nil,
		Cos:         nil,
		Cvs:         nil,
	}
	err := repo.AddPeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	// Update only SecretValue
	peerData.SecretValue = []byte{0x01, 0x02}
	err = repo.UpdatePeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	fetched, err := repo.GetPeerCommitData(ctx, "update_test", "update_trial", "0xupdate")
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02}, fetched.SecretValue)
	assert.Nil(t, fetched.Cos)
	assert.Nil(t, fetched.Cvs)

	// Update only Cos
	peerData.Cos = []byte{0x03, 0x04}
	err = repo.UpdatePeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	fetched, err = repo.GetPeerCommitData(ctx, "update_test", "update_trial", "0xupdate")
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02}, fetched.SecretValue)
	assert.Equal(t, []byte{0x03, 0x04}, fetched.Cos)
	assert.Nil(t, fetched.Cvs)

	// Update only Cvs
	peerData.Cvs = []byte{0x05, 0x06}
	err = repo.UpdatePeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	fetched, err = repo.GetPeerCommitData(ctx, "update_test", "update_trial", "0xupdate")
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02}, fetched.SecretValue)
	assert.Equal(t, []byte{0x03, 0x04}, fetched.Cos)
	assert.Equal(t, []byte{0x05, 0x06}, fetched.Cvs)

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", "update_test", "update_trial", "0xupdate").
		Context(ctx).
		Delete()
}

func TestPeerCommitRepository_MultipleEOAs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	round := "multi_eoa_round"
	trialNum := "multi_eoa_trial"
	eoas := []string{"0xeoa1", "0xeoa2", "0xeoa3"}

	// Cleanup
	for _, eoa := range eoas {
		GetDB().Model(&PeerCommitDataScheme{}).
			Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoa).
			Context(ctx).
			Delete()
	}

	// Add multiple EOAs
	for i, eoa := range eoas {
		data := &PeerCommitDataScheme{
			Round:       round,
			TrialNum:    trialNum,
			EOAAddress:  eoa,
			SecretValue: []byte{byte(i), 0x01},
			Cos:         []byte{byte(i), 0x02},
			Cvs:         []byte{byte(i), 0x03},
		}
		err := repo.AddPeerCommitData(ctx, data)
		assert.NoError(t, err)
	}

	// Verify each EOA
	for i, eoa := range eoas {
		fetched, err := repo.GetPeerCommitData(ctx, round, trialNum, eoa)
		assert.NoError(t, err)
		assert.Equal(t, []byte{byte(i), 0x01}, fetched.SecretValue)
		assert.Equal(t, []byte{byte(i), 0x02}, fetched.Cos)
		assert.Equal(t, []byte{byte(i), 0x03}, fetched.Cvs)
	}

	// Cleanup
	for _, eoa := range eoas {
		GetDB().Model(&PeerCommitDataScheme{}).
			Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoa).
			Context(ctx).
			Delete()
	}
}

func TestPeerCommitRepository_UpdateNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", "ghost", "ghost", "0xghost").
		Context(ctx).
		Delete()

	ghostData := &PeerCommitDataScheme{
		Round:       "ghost",
		TrialNum:    "ghost",
		EOAAddress:  "0xghost",
		SecretValue: []byte{0xFF},
	}
	err := repo.UpdatePeerCommitData(ctx, ghostData)
	assert.NoError(t, err, "Update succeeds but updates 0 rows")

	// Verify not created
	_, err = repo.GetPeerCommitData(ctx, "ghost", "ghost", "0xghost")
	assert.Error(t, err, "Should not exist")
}

// Test NewPeerCommitRepository constructor
func TestNewPeerCommitRepository(t *testing.T) {
	db := GetDB()
	repo := NewPeerCommitRepository(db)
	assert.NotNil(t, repo, "Repository should be created")
	assert.Equal(t, db, repo.db, "Repository should store the database connection")
}

// Test context timeout for AddPeerCommitData
func TestPeerCommitRepository_AddPeerCommitData_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the insert operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE peer_commit_data_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())
	peerData := &PeerCommitDataScheme{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Cvs:        []byte{0x01},
	}

	// This call should block on the locked table and time out
	err = repo.AddPeerCommitData(ctx, peerData)
	assert.Error(t, err, "Expected an error due to context timeout")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test context timeout for GetPeerCommitData
func TestPeerCommitRepository_GetPeerCommitData_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the select operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE peer_commit_data_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	// This call should block on the locked table and time out
	_, err = repo.GetPeerCommitData(ctx, "test", "test", "0xtest")
	assert.Error(t, err, "Expected an error due to context timeout")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test context timeout for UpdatePeerCommitData
func TestPeerCommitRepository_UpdatePeerCommitData_ContextTimeout(t *testing.T) {
	// Start a transaction and lock a table to ensure the update operation will block
	tx, err := GetDB().Begin()
	assert.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.Exec(`LOCK TABLE peer_commit_data_schemes IN ACCESS EXCLUSIVE MODE`)
	assert.NoError(t, err)

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())
	peerData := &PeerCommitDataScheme{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Cvs:        []byte{0x01},
	}

	// This call should block on the locked table and time out
	err = repo.UpdatePeerCommitData(ctx, peerData)
	assert.Error(t, err, "Expected an error due to context timeout")
	// The error can be either "context deadline exceeded" or "i/o timeout" depending on the driver
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout"),
		"Error should be related to context timeout, got: %s", err.Error())
}

// Test concurrent operations
func TestPeerCommitRepository_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())
	round := "concurrent_test_round"
	trialNum := "concurrent_test_trial"

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Run concurrent add operations
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			peerData := &PeerCommitDataScheme{
				Round:      round,
				TrialNum:   trialNum,
				EOAAddress: fmt.Sprintf("0xconcurrent_%d", idx),
				Cvs:        []byte{byte(idx)},
			}
			err := repo.AddPeerCommitData(ctx, peerData)
			done <- err
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		err := <-done
		assert.NoError(t, err, "Concurrent operations should not error")
	}

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test with special characters in round/trial/EOA to ensure SQL injection prevention
func TestPeerCommitRepository_SpecialCharacters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	// Test with various special characters that could be used in SQL injection attempts
	specialRounds := []string{
		"'; DROP TABLE peer_commit_data_schemes; --",
		"'; DELETE FROM peer_commit_data_schemes; --",
		"1' OR '1'='1",
		"1' UNION SELECT * FROM peer_commit_data_schemes; --",
		"round'; DELETE FROM peer_commit_data_schemes WHERE '1'='1",
		"round\" OR \"1\"=\"1",
		"round\\'; DROP TABLE peer_commit_data_schemes; --",
	}

	for _, specialRound := range specialRounds {
		peerData := &PeerCommitDataScheme{
			Round:      specialRound,
			TrialNum:   "trial",
			EOAAddress: "0xtest",
			Cvs:        []byte{0x01},
		}

		// These should not cause SQL injection (parameterized queries should handle this)
		err := repo.AddPeerCommitData(ctx, peerData)
		// May succeed or fail, but should not cause SQL injection
		if err != nil {
			// If it fails, it should be a validation/constraint error, not SQL injection
			assert.NotContains(t, err.Error(), "DROP TABLE", "Should not execute DROP TABLE")
			assert.NotContains(t, err.Error(), "DELETE FROM", "Should not execute DELETE FROM")
		}

		// Cleanup if it was added
		GetDB().Model(&PeerCommitDataScheme{}).
			Where("round = ?", specialRound).
			Context(ctx).
			Delete()
	}
}

// Test with unicode and international characters
func TestPeerCommitRepository_UnicodeCharacters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

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
		peerData := &PeerCommitDataScheme{
			Round:      unicodeRound,
			TrialNum:   "trial",
			EOAAddress: "0xtest",
			Cvs:        []byte{0x01},
		}

		err := repo.AddPeerCommitData(ctx, peerData)
		assert.NoError(t, err, "Should handle unicode characters: %s", unicodeRound)

		// Cleanup
		GetDB().Model(&PeerCommitDataScheme{}).
			Where("round = ?", unicodeRound).
			Context(ctx).
			Delete()
	}
}

// Test error handling for AddPeerCommitData when database fails
func TestPeerCommitRepository_AddPeerCommitData_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS peer_commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to add peer commit - should fail because table doesn't exist
	peerData := &PeerCommitDataScheme{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Cvs:        []byte{0x01},
	}

	err = repo.AddPeerCommitData(ctx, peerData)
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for UpdatePeerCommitData when database fails
func TestPeerCommitRepository_UpdatePeerCommitData_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS peer_commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to update peer commit - should fail because table doesn't exist
	peerData := &PeerCommitDataScheme{
		Round:      "test",
		TrialNum:   "test",
		EOAAddress: "0xtest",
		Cvs:        []byte{0x01},
	}

	err = repo.UpdatePeerCommitData(ctx, peerData)
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test error handling for GetPeerCommitData when database fails
func TestPeerCommitRepository_GetPeerCommitData_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())

	// Drop the table to force an error
	_, err := GetDB().Exec("DROP TABLE IF EXISTS peer_commit_data_schemes CASCADE")
	assert.NoError(t, err, "Failed to drop table for test")

	// Try to get peer commit - should get an error because table doesn't exist
	_, err = repo.GetPeerCommitData(ctx, "test", "test", "0xtest")
	assert.Error(t, err, "Expected error when table is missing")

	// Restore the schema
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	sqlDB, _ := sql.Open("postgres", dsn)
	defer sqlDB.Close()
	MigrationsDown(sqlDB)
	MigrationsUp(sqlDB)
}

// Test GetPeerCommitData with very large dataset
func TestPeerCommitRepository_GetPeerCommitData_VeryLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())
	round := "very_large_peer_round"
	trialNum := "very_large_peer_trial"

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Create very large dataset (1000 records)
	numCommits := 1000
	for i := 0; i < numCommits; i++ {
		peerData := &PeerCommitDataScheme{
			Round:       round,
			TrialNum:    trialNum,
			EOAAddress:  fmt.Sprintf("0xlarge_peer_%d", i),
			Cvs:         []byte{byte(i % 256)},
			Cos:         []byte{byte((i + 1) % 256)},
			SecretValue: []byte{byte((i + 2) % 256)},
		}
		err := repo.AddPeerCommitData(ctx, peerData)
		if err != nil {
			t.Fatalf("Failed to add peer commit %d: %v", i, err)
		}
	}

	// Get a sample of commits
	startTime := time.Now()
	for i := 0; i < 100; i++ {
		_, err := repo.GetPeerCommitData(ctx, round, trialNum, fmt.Sprintf("0xlarge_peer_%d", i))
		if err != nil {
			t.Fatalf("Failed to get peer commit %d: %v", i, err)
		}
	}
	duration := time.Since(startTime)

	// Log performance
	t.Logf("Retrieved 100 peer commits in %v", duration)

	// Verify we can get commits
	fetched, err := repo.GetPeerCommitData(ctx, round, trialNum, "0xlarge_peer_0")
	assert.NoError(t, err, "Should successfully retrieve peer commit")
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

// Test UpdatePeerCommitData with all fields updated
func TestPeerCommitRepository_UpdatePeerCommitData_AllFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())
	round := "update_all_peer_round"
	trialNum := "update_all_peer_trial"
	eoaAddr := "0xupdate_all_peer"

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()

	// Add initial peer commit
	peerData := &PeerCommitDataScheme{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  eoaAddr,
		Cvs:         []byte{0x01},
		Cos:         []byte{0x02},
		SecretValue: []byte{0x03},
	}

	err := repo.AddPeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	// Update all fields
	peerData.Cvs = []byte{0x04, 0x05}
	peerData.Cos = []byte{0x06, 0x07}
	peerData.SecretValue = []byte{0x08, 0x09}

	err = repo.UpdatePeerCommitData(ctx, peerData)
	assert.NoError(t, err)

	// Verify update
	fetched, err := repo.GetPeerCommitData(ctx, round, trialNum, eoaAddr)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x04, 0x05}, fetched.Cvs, "Cvs should be updated")
	assert.Equal(t, []byte{0x06, 0x07}, fetched.Cos, "Cos should be updated")
	assert.Equal(t, []byte{0x08, 0x09}, fetched.SecretValue, "SecretValue should be updated")

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()
}

// Test AddPeerCommitData with very large byte arrays
func TestPeerCommitRepository_AddPeerCommitData_LargeByteArrays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())
	round := "large_bytes_round"
	trialNum := "large_bytes_trial"
	eoaAddr := "0xlarge_bytes"

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()

	// Create large byte arrays (10KB each)
	largeCvs := make([]byte, 10240)
	largeCos := make([]byte, 10240)
	largeSecretValue := make([]byte, 10240)
	for i := range largeCvs {
		largeCvs[i] = byte(i % 256)
		largeCos[i] = byte((i + 1) % 256)
		largeSecretValue[i] = byte((i + 2) % 256)
	}

	peerData := &PeerCommitDataScheme{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  eoaAddr,
		Cvs:         largeCvs,
		Cos:         largeCos,
		SecretValue: largeSecretValue,
	}

	err := repo.AddPeerCommitData(ctx, peerData)
	assert.NoError(t, err, "Should handle large byte arrays")

	// Verify
	fetched, err := repo.GetPeerCommitData(ctx, round, trialNum, eoaAddr)
	assert.NoError(t, err)
	assert.Equal(t, len(largeCvs), len(fetched.Cvs), "Cvs should be preserved")
	assert.Equal(t, len(largeCos), len(fetched.Cos), "Cos should be preserved")
	assert.Equal(t, len(largeSecretValue), len(fetched.SecretValue), "SecretValue should be preserved")

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Context(ctx).
		Delete()
}

// Test GetPeerCommitData with multiple commits for same round/trial
func TestPeerCommitRepository_GetPeerCommitData_MultipleCommits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewPeerCommitRepository(GetDB())
	round := "multi_peer_round"
	trialNum := "multi_peer_trial"

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()

	// Add multiple commits with different EOAs
	eoas := []string{"0xmulti1", "0xmulti2", "0xmulti3"}
	for i, eoa := range eoas {
		peerData := &PeerCommitDataScheme{
			Round:       round,
			TrialNum:    trialNum,
			EOAAddress:  eoa,
			Cvs:         []byte{byte(i)},
			Cos:         []byte{byte(i + 1)},
			SecretValue: []byte{byte(i + 2)},
		}
		err := repo.AddPeerCommitData(ctx, peerData)
		assert.NoError(t, err)
	}

	// Verify each EOA
	for i, eoa := range eoas {
		fetched, err := repo.GetPeerCommitData(ctx, round, trialNum, eoa)
		assert.NoError(t, err, "Should get commit for EOA %s", eoa)
		assert.Equal(t, []byte{byte(i)}, fetched.Cvs)
		assert.Equal(t, []byte{byte(i + 1)}, fetched.Cos)
		assert.Equal(t, []byte{byte(i + 2)}, fetched.SecretValue)
	}

	// Cleanup
	GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Context(ctx).
		Delete()
}

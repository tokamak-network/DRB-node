package database

import (
	"context"
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

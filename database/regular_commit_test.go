package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestCommitCRUD(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	round := "test_round"
	trialNum := "test_trial"

	// Clean up before test
	_, err := GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
	assert.NoError(t, err)

	// Prepare fixed-size arrays
	var cvs, cos, secretValue [32]byte
	copy(cvs[:], []byte{0x01, 0x02, 0x03})
	copy(cos[:], []byte{0x04, 0x05})
	copy(secretValue[:], []byte{0x06, 0x07})

	commitData := &utils.CommitData{
		Round:           round,
		TrialNum:        trialNum,
		Cvs:             cvs,
		Cos:             cos,
		SecretValue:     secretValue,
		Sign:            utils.SignInfo{R: "rval", S: "sval", V: "vval"},
		SendToLeader:    true,
		SendCosToLeader: false,
	}

	// Test AddCommit
	err = AddCommit(commitData)
	assert.NoError(t, err)

	// Test GetCommitByRound
	fetched, err := GetCommitByRound(round, trialNum)
	assert.NoError(t, err)
	assert.NotNil(t, fetched)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.Equal(t, commitData.Cvs, fetched.Cvs)
	assert.Equal(t, commitData.Cos, fetched.Cos)
	assert.Equal(t, commitData.SecretValue, fetched.SecretValue)
	assert.Equal(t, commitData.Sign.R, fetched.Sign.R)
	assert.Equal(t, commitData.Sign.S, fetched.Sign.S)
	assert.Equal(t, commitData.Sign.V, fetched.Sign.V)
	assert.Equal(t, commitData.SendToLeader, fetched.SendToLeader)
	assert.Equal(t, commitData.SendCosToLeader, fetched.SendCosToLeader)

	// Update some fields
	fetched.SendToLeader = false
	fetched.SendCosToLeader = true
	fetched.Sign = utils.SignInfo{R: "newR", S: "newS", V: "newV"}

	// Test UpdateCommit
	err = UpdateCommit(fetched)
	assert.NoError(t, err)

	// Verify update
	updated, err := GetCommitByRound(round, trialNum)
	assert.NoError(t, err)
	assert.False(t, updated.SendToLeader)
	assert.True(t, updated.SendCosToLeader)
	assert.Equal(t, "newR", updated.Sign.R)
	assert.Equal(t, "newS", updated.Sign.S)
	assert.Equal(t, "newV", updated.Sign.V)

	// Clean up after test
	_, err = GetDB().Model(&CommitDataScheme{}).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Delete()
	assert.NoError(t, err)
}

package database

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tokamak-network/DRB-node/utils"
)

func TestLeaderCommitCRUD(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	round := "round_test"
	trialNum := "trial_test"
	eoaAddr := "0xeoa_test"

	// Clean up before test
	_, err := GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Delete()
	assert.NoError(t, err)

	var cvs [32]byte
	copy(cvs[:], []byte{0x01, 0x02, 0x03})

	var cos [32]byte
	copy(cos[:], []byte{0x04, 0x05})

	var secretValue [32]byte
	copy(secretValue[:], []byte{0x06, 0x07})

	commitData := &utils.LeaderCommitData{
		Round:          round,
		TrialNum:       trialNum,
		EOAAddress:     eoaAddr,
		Cvs:            cvs,
		CvsHex:         "",
		Cos:            cos,
		CosHex:         "",
		SecretValue:    secretValue,
		SecretValueHex: "",
		Sign: utils.SignInfo{
			R: "rval",
			S: "sval",
			V: "vval",
		},
		SubmitMerkleRootDone:  true,
		RandomNumberGenerated: false,
		CreatedAt:             time.Now().Unix(),
	}

	// Test AddLeaderCommit
	err = AddLeaderCommit(commitData)
	assert.NoError(t, err)

	// Test GetLeaderCommitByRoundAndEoaAddr
	fetched, err := GetLeaderCommitByRoundAndEoaAddr(round, trialNum, eoaAddr)
	assert.NoError(t, err)
	assert.NotNil(t, fetched)
	assert.Equal(t, round, fetched.Round)
	assert.Equal(t, trialNum, fetched.TrialNum)
	assert.Equal(t, eoaAddr, fetched.EOAAddress)
	assert.Equal(t, hex.EncodeToString(commitData.Cvs[:]), fetched.CvsHex)
	assert.Equal(t, hex.EncodeToString(commitData.Cos[:]), fetched.CosHex)
	assert.Equal(t, hex.EncodeToString(commitData.SecretValue[:]), fetched.SecretValueHex)
	assert.Equal(t, commitData.Sign.R, fetched.Sign.R)
	assert.Equal(t, commitData.Sign.S, fetched.Sign.S)
	assert.Equal(t, commitData.Sign.V, fetched.Sign.V)

	// Test GetLeaderCommitsByRoundAndTrialNum
	commits, err := GetLeaderCommitsByRoundAndTrialNum(round, trialNum)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(commits), 1)

	// Test UpdateLeaderCommit - change SubmitMerkleRootDone and Sign
	fetched.SubmitMerkleRootDone = false
	fetched.Sign = utils.SignInfo{R: "newR", S: "newS", V: "newV"}
	err = UpdateLeaderCommit(fetched)
	assert.NoError(t, err)

	// Verify update
	updated, err := GetLeaderCommitByRoundAndEoaAddr(round, trialNum, eoaAddr)
	assert.NoError(t, err)
	assert.False(t, updated.SubmitMerkleRootDone)
	assert.Equal(t, "newR", updated.Sign.R)
	assert.Equal(t, "newS", updated.Sign.S)
	assert.Equal(t, "newV", updated.Sign.V)

	// Test UpdateLeaderCommitRandomNumberGenerated
	err = UpdateLeaderCommitRandomNumberGenerated(round, trialNum)
	assert.NoError(t, err)

	// Verify RandomNumberGenerated update
	updated2, err := GetLeaderCommitByRoundAndEoaAddr(round, trialNum, eoaAddr)
	assert.NoError(t, err)
	assert.True(t, updated2.RandomNumberGenerated)

	// Test GetRoundsToProcess (should include commits with RandomNumberGenerated=true and SubmitMerkleRootDone=false)
	rounds, err := GetRoundsToProcess()
	assert.NoError(t, err)

	found := true
	for _, c := range rounds {
		if c.Round == round && c.TrialNum == trialNum && c.EOAAddress == eoaAddr {
			found = true
			assert.True(t, c.RandomNumberGenerated)
			assert.False(t, c.SubmitMerkleRootDone)
		}
	}
	assert.True(t, found)

	// Clean up after test
	_, err = GetDB().Model(&LeaderCommitScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Delete()
	assert.NoError(t, err)
}

package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAddGetUpdatePeerCommitData(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	round := "round1"
	trialNum := "trial1"
	eoaAddress := "0xpeercommit"

	// Clean up before test
	_, err := GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddress).
		Delete()
	assert.NoError(t, err)

	peerData := &PeerCommitDataScheme{
		Round:       round,
		TrialNum:    trialNum,
		EOAAddress:  eoaAddress,
		SecretValue: []byte{0x01, 0x02, 0x03},
		Cos:         []byte{0x04, 0x05},
		Cvs:         []byte{0x06, 0x07},
	}

	// AddPeerCommitData
	err = AddPeerCommitData(peerData)
	assert.NoError(t, err)

	// GetPeerCommitData
	fetched, err := GetPeerCommitData(round, trialNum, eoaAddress)
	assert.NoError(t, err)
	assert.NotNil(t, fetched)
	assert.Equal(t, peerData.Round, fetched.Round)
	assert.Equal(t, peerData.TrialNum, fetched.TrialNum)
	assert.Equal(t, peerData.EOAAddress, fetched.EOAAddress)
	assert.Equal(t, peerData.SecretValue, fetched.SecretValue)
	assert.Equal(t, peerData.Cos, fetched.Cos)
	assert.Equal(t, peerData.Cvs, fetched.Cvs)

	// UpdatePeerCommitData - change SecretValue
	fetched.SecretValue = []byte{0x0A, 0x0B}
	err = UpdatePeerCommitData(fetched)
	assert.NoError(t, err)

	// Verify update
	updated, err := GetPeerCommitData(round, trialNum, eoaAddress)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x0A, 0x0B}, updated.SecretValue)

	// Clean up after test
	_, err = GetDB().Model(&PeerCommitDataScheme{}).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddress).
		Delete()
	assert.NoError(t, err)
}

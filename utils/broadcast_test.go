package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBroadcastStructs(t *testing.T) {
	t.Run("BroadcastMessage creation", func(t *testing.T) {
		msg := BroadcastMessage{
			Round:      "1",
			TrialNum:   "0",
			EOAAddress: "0x1234567890123456789012345678901234567890",
			MessageID:  "msg-1",
			Type:       "cvs",
			Data:       [32]byte{1, 2, 3},
			SignerEOA:  "0x2345678901234567890123456789012345678901",
			Signature:  []byte{4, 5, 6},
		}

		assert.Equal(t, "1", msg.Round)
		assert.Equal(t, "0", msg.TrialNum)
		assert.Equal(t, "cvs", msg.Type)
		assert.Equal(t, byte(1), msg.Data[0])
		assert.Equal(t, "0x1234567890123456789012345678901234567890", msg.EOAAddress)
		assert.Equal(t, "msg-1", msg.MessageID)
		assert.Equal(t, "0x2345678901234567890123456789012345678901", msg.SignerEOA)
		assert.Equal(t, []byte{4, 5, 6}, msg.Signature)
	})

	t.Run("AcknowledgmentMessage creation", func(t *testing.T) {
		ack := AcknowledgmentMessage{
			Round:      "1",
			TrialNum:   "0",
			EOAAddress: "0x1234567890123456789012345678901234567890",
			MessageID:  "msg-1",
			Type:       "cvs",
			Status:     "received",
			Signature:  []byte{7, 8, 9},
		}

		assert.Equal(t, "1", ack.Round)
		assert.Equal(t, "received", ack.Status)
		assert.Equal(t, "0", ack.TrialNum)
		assert.Equal(t, "0x1234567890123456789012345678901234567890", ack.EOAAddress)
		assert.Equal(t, "msg-1", ack.MessageID)
		assert.Equal(t, "cvs", ack.Type)
		assert.Equal(t, []byte{7, 8, 9}, ack.Signature)
	})

	t.Run("BroadcastTracker creation", func(t *testing.T) {
		tracker := BroadcastTracker{
			Round:        "1",
			TrialNum:     "0",
			EOAAddress:   "0x1234567890123456789012345678901234567890",
			Type:         "cvs",
			MessageID:    "msg-1",
			Data:         [32]byte{1, 2, 3},
			Attempts:     3,
			MaxAttempts:  5,
			Acknowledged: map[string]bool{"0x1111": true, "0x2222": false},
			LastSent:     1234567890,
			Timeout:      300,
		}

		assert.Equal(t, "1", tracker.Round)
		assert.Equal(t, 3, tracker.Attempts)
		assert.True(t, tracker.Acknowledged["0x1111"])
		assert.False(t, tracker.Acknowledged["0x2222"])
		assert.Equal(t, "0", tracker.TrialNum)
		assert.Equal(t, "0x1234567890123456789012345678901234567890", tracker.EOAAddress)
		assert.Equal(t, "cvs", tracker.Type)
		assert.Equal(t, "msg-1", tracker.MessageID)
		assert.Equal(t, 5, tracker.MaxAttempts)
		assert.Equal(t, int64(1234567890), tracker.LastSent)
		assert.Equal(t, int64(300), tracker.Timeout)
	})
}
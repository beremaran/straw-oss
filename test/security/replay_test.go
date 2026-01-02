package security

import (
	"fmt"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplayProtection_StaleTimestamp(t *testing.T) {
	secret := []byte("test-secret-123")
	maxAge := 1 * time.Minute

	// Helper to create a task with a specific timestamp
	createTask := func(ts int64) *protocol.SignedTask {
		req := &protocol.Request{
			URL:    "http://example.com",
			Method: "GET",
		}

		payload, err := protocol.MarshalCompressed(req)
		require.NoError(t, err)

		signatureData := append(payload, []byte(fmt.Sprintf("%d", ts))...)
		signature := protocol.Sign(signatureData, secret)

		return &protocol.SignedTask{
			Payload:   payload,
			Signature: signature,
			Timestamp: ts,
		}
	}

	t.Run("ValidTimestamp", func(t *testing.T) {
		ts := time.Now().Unix()
		task := createTask(ts)

		req, err := protocol.ValidateSignedTask(task, secret, maxAge)
		require.NoError(t, err)
		require.NotNil(t, req)
		assert.Equal(t, "http://example.com", req.URL)
	})

	t.Run("StaleTimestamp_TooOld", func(t *testing.T) {
		// Create timestamp older than maxAge
		ts := time.Now().Add(-maxAge - 10*time.Second).Unix()
		task := createTask(ts)

		req, err := protocol.ValidateSignedTask(task, secret, maxAge)
		require.Error(t, err)
		require.Nil(t, req)

		// Assert error code
		var valErr *protocol.ValidationError
		require.ErrorAs(t, err, &valErr)
		assert.Equal(t, protocol.ErrCodeReplayAttack, valErr.Code)
		assert.Contains(t, valErr.Message, "too old")
	})

	t.Run("FutureTimestamp_ClockSkew", func(t *testing.T) {
		// Create timestamp significantly in future (beyond maxAge allowance for skew)
		// ValidateSignedTask treats age < 0 as positive age (magnitude).
		// So if future > maxAge, it fails.
		ts := time.Now().Add(maxAge + 10*time.Second).Unix()
		task := createTask(ts)

		req, err := protocol.ValidateSignedTask(task, secret, maxAge)
		require.Error(t, err)
		require.Nil(t, req)

		var valErr *protocol.ValidationError
		require.ErrorAs(t, err, &valErr)
		assert.Equal(t, protocol.ErrCodeReplayAttack, valErr.Code)
	})

	t.Run("InvalidSignature_TamperedTimestamp", func(t *testing.T) {
		ts := time.Now().Unix()
		task := createTask(ts)

		// Tamper with timestamp without resigning
		task.Timestamp -= 100 // Request appears older (or just different)

		req, err := protocol.ValidateSignedTask(task, secret, maxAge)
		require.Error(t, err)
		require.Nil(t, req)

		var valErr *protocol.ValidationError
		require.ErrorAs(t, err, &valErr)
		// Should fail signature check, NOT replay check first?
		// Logic:
		// 1. Check timestamp age.
		//    Original TS: valid. Tampered TS: valid (just 100s ago or whatever).
		//    Wait, 100s might be > maxAge (60s).
		//    Let's reduce tamper amount to be within valid range but different.
		//    e.g. 1 second different.

		// Let's create specific case
		ts = time.Now().Unix()
		task = createTask(ts)
		task.Timestamp -= 5 // 5 seconds ago, valid age

		req, err = protocol.ValidateSignedTask(task, secret, maxAge)
		require.Error(t, err)

		// This should be signature invalid because signature covers timestamp
		require.ErrorAs(t, err, &valErr)
		assert.Equal(t, protocol.ErrCodeSignatureInvalid, valErr.Code)
	})
}

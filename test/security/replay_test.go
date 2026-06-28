package security

import (
	"fmt"
	"testing"
	"time"

	"github.com/beremaran/straw/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplayProtection_StaleTimestamp(t *testing.T) {
	secret := []byte("test-secret-123")
	maxAge := 1 * time.Minute

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
		ts := time.Now().Add(-maxAge - 10*time.Second).Unix()
		task := createTask(ts)

		req, err := protocol.ValidateSignedTask(task, secret, maxAge)
		require.Error(t, err)
		require.Nil(t, req)

		var valErr *protocol.ValidationError
		require.ErrorAs(t, err, &valErr)
		assert.Equal(t, protocol.ErrCodeReplayAttack, valErr.Code)
		assert.Contains(t, valErr.Message, "too old")
	})

	t.Run("FutureTimestamp_ClockSkew", func(t *testing.T) {
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

		task.Timestamp -= 100

		req, err := protocol.ValidateSignedTask(task, secret, maxAge)
		require.Error(t, err)
		require.Nil(t, req)

		var valErr *protocol.ValidationError
		require.ErrorAs(t, err, &valErr)

		ts = time.Now().Unix()
		task = createTask(ts)
		task.Timestamp -= 5

		_, err = protocol.ValidateSignedTask(task, secret, maxAge)
		require.Error(t, err)

		require.ErrorAs(t, err, &valErr)
		assert.Equal(t, protocol.ErrCodeSignatureInvalid, valErr.Code)
	})
}

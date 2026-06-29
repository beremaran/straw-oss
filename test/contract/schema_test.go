package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"

	"github.com/beremaran/straw/pkg/protocol"
)

func loadSchema(t *testing.T, filename string) *gojsonschema.Schema {
	wd, err := os.Getwd()
	require.NoError(t, err)

	path := filepath.Join(wd, "schemas", filename)
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		path = filepath.Join(wd, "test", "contract", "schemas", filename)
	}

	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		path = filepath.Join("test", "contract", "schemas", filename)
	}

	absPath, err := filepath.Abs(path)
	require.NoError(t, err)

	loader := gojsonschema.NewReferenceLoader("file://" + absPath)
	schema, err := gojsonschema.NewSchema(loader)
	require.NoError(t, err, "failed to load schema %s", filename)

	return schema
}

func validateAgainstSchema(t *testing.T, schema *gojsonschema.Schema, data any) {
	jsonBytes, err := json.Marshal(data)
	require.NoError(t, err)

	loader := gojsonschema.NewBytesLoader(jsonBytes)
	result, err := schema.Validate(loader)
	require.NoError(t, err)

	if !result.Valid() {
		for _, desc := range result.Errors() {
			t.Errorf("- %s", desc)
		}
		t.FailNow()
	}
}

func TestRequestSchema(t *testing.T) {
	schema := loadSchema(t, "request.schema.json")

	t.Run("ValidRequest", func(t *testing.T) {
		req := protocol.Request{
			ID:     "req-123",
			Method: "GET",
			URL:    "https://example.com/api",
			Headers: protocol.HeaderMap{
				{Key: "User-Agent", Value: "StrawBot/1.0"},
				{Key: "Accept", Value: "application/json"},
			},
			Timeout:     30 * time.Second,
			Fingerprint: "chrome-130",
			SessionID:   "sess-abc",
			TraceID:     "trace-xyz",
		}
		validateAgainstSchema(t, schema, req)
	})

	t.Run("ValidRequest_Minimal", func(t *testing.T) {
		req := protocol.Request{
			ID:      "req-456",
			Method:  "POST",
			URL:     "https://foo.bar",
			Headers: protocol.HeaderMap{},
		}
		validateAgainstSchema(t, schema, req)
	})

	t.Run("InvalidRequest_MissingRequired", func(t *testing.T) {
		invalidReq := map[string]any{
			"id": "req-bad",
		}

		loader := gojsonschema.NewGoLoader(invalidReq)
		result, err := schema.Validate(loader)
		require.NoError(t, err)
		assert.False(t, result.Valid())
	})
}

func TestResponseSchema(t *testing.T) {
	schema := loadSchema(t, "response.schema.json")

	t.Run("ValidResponse_Success", func(t *testing.T) {
		resp := protocol.Response{
			RequestID:  "req-123",
			StatusCode: 200,
			Headers: protocol.HeaderMap{
				{Key: "Content-Type", Value: "application/json"},
			},
			Body:       []byte(`{"ok":true}`),
			EndpointID: "ep-1",
			Timing: &protocol.TimingInfo{
				Total: 150 * time.Millisecond,
			},
		}
		validateAgainstSchema(t, schema, resp)
	})

	t.Run("ValidResponse_Error", func(t *testing.T) {
		resp := protocol.Response{
			RequestID:  "req-failed",
			StatusCode: 503,
			Headers:    protocol.HeaderMap{},
			Error: &protocol.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   "Gateway Timeout",
				Retryable: true,
			},
		}
		validateAgainstSchema(t, schema, resp)
	})
}

func TestSignedTaskSchema(t *testing.T) {
	schema := loadSchema(t, "signed_task.schema.json")

	t.Run("ValidSignedTask", func(t *testing.T) {
		task := protocol.SignedTask{
			Payload:   []byte("lzma-compressed-data"),
			Signature: "hmac-sha256-signature",
			Timestamp: time.Now().Unix(),
		}
		validateAgainstSchema(t, schema, task)
	})
}

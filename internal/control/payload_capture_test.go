package control

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
)

func makeHeader(name, value string) HeaderPair {
	return HeaderPair{
		Name:  name,
		Value: base64.StdEncoding.EncodeToString([]byte(value)),
	}
}

func parseCapturedHeaders(t *testing.T, raw string) []CapturedHeader {
	if raw == "" {
		return nil
	}

	var res []CapturedHeader

	err := json.Unmarshal([]byte(raw), &res)
	if err != nil {
		t.Fatalf("failed to unmarshal captured headers %q: %v", raw, err)
	}

	return res
}

func TestCapturePayloadDecisions(t *testing.T) {
	reqHeaders := []HeaderPair{
		makeHeader("Host", "example.com"),
		makeHeader(testAuthorizationHeader, "Bearer my-secret-token"),
		makeHeader("X-Plain", "hello"),
	}
	respHeaders := []HeaderPair{
		makeHeader("Content-Type", "text/plain"),
		makeHeader(testSetCookieHeader, "session=abcdef"),
	}
	reqBody := []byte("hello request body")
	respBody := []byte("hello response body")

	opts := CaptureOptions{
		MaxFullBytes:      100,
		MaxTruncatedBytes: 10,
		AllowCompressed:   false,
	}

	t.Run("none", func(t *testing.T) {
		res := CapturePayload(CaptureDecisionNone, reqHeaders, reqBody, respHeaders, respBody, opts)

		if res.RequestHeaders != "" || res.ResponseHeaders != "" {
			t.Errorf("expected empty headers, got %q and %q", res.RequestHeaders, res.ResponseHeaders)
		}

		if res.RequestBody != nil || res.ResponseBody != nil {
			t.Errorf("expected nil bodies")
		}

		if len(res.RedactedFields) > 0 {
			t.Errorf("expected no redacted fields")
		}

		if res.Truncated {
			t.Errorf("expected Truncated = false")
		}
	})

	t.Run("metadata_only", func(t *testing.T) {
		res := CapturePayload(CaptureDecisionMetadataOnly, reqHeaders, reqBody, respHeaders, respBody, opts)

		if res.RequestHeaders != "" || res.ResponseHeaders != "" {
			t.Errorf("expected empty headers, got %q and %q", res.RequestHeaders, res.ResponseHeaders)
		}

		if res.RequestBody != nil || res.ResponseBody != nil {
			t.Errorf("expected nil bodies")
		}

		if len(res.RedactedFields) > 0 {
			t.Errorf("expected no redacted fields")
		}

		if res.Truncated {
			t.Errorf("expected Truncated = false")
		}
	})

	t.Run("headers", func(t *testing.T) {
		res := CapturePayload(CaptureDecisionHeaders, reqHeaders, reqBody, respHeaders, respBody, opts)

		if res.RequestBody != nil || res.ResponseBody != nil {
			t.Errorf("expected nil bodies")
		}

		if res.Truncated {
			t.Errorf("expected Truncated = false")
		}

		reqCaps := parseCapturedHeaders(t, res.RequestHeaders)
		respCaps := parseCapturedHeaders(t, res.ResponseHeaders)

		// Authorization must be redacted
		foundAuth := false

		for _, h := range reqCaps {
			if h.Name == testAuthorizationHeader {
				foundAuth = true

				if h.Value != "[redacted]" {
					t.Errorf("expected Authorization to be redacted, got %q", h.Value)
				}
			}
		}

		if !foundAuth {
			t.Errorf("Authorization header missing")
		}

		// Set-Cookie must be redacted
		foundCookie := false

		for _, h := range respCaps {
			if h.Name == testSetCookieHeader {
				foundCookie = true

				if h.Value != "[redacted]" {
					t.Errorf("expected Set-Cookie to be redacted, got %q", h.Value)
				}
			}
		}

		if !foundCookie {
			t.Errorf("Set-Cookie header missing")
		}

		// Redacted fields list must contain Authorization and Set-Cookie
		if !reflect.DeepEqual(res.RedactedFields, []string{testAuthorizationHeader, testSetCookieHeader}) {
			t.Errorf("unexpected RedactedFields: %v", res.RedactedFields)
		}
	})

	t.Run("body_truncated", func(t *testing.T) {
		res := CapturePayload(CaptureDecisionBodyTruncated, reqHeaders, reqBody, respHeaders, respBody, opts)

		if string(res.RequestBody) != "hello requ" {
			t.Errorf("expected truncated request body 'hello requ', got %q", string(res.RequestBody))
		}

		if string(res.ResponseBody) != "hello resp" {
			t.Errorf("expected truncated response body 'hello resp', got %q", string(res.ResponseBody))
		}

		if !res.Truncated {
			t.Errorf("expected Truncated = true")
		}
	})

	t.Run("body_full", func(t *testing.T) {
		res := CapturePayload(CaptureDecisionBodyFull, reqHeaders, reqBody, respHeaders, respBody, opts)

		if string(res.RequestBody) != "hello request body" {
			t.Errorf("expected full request body, got %q", string(res.RequestBody))
		}

		if string(res.ResponseBody) != "hello response body" {
			t.Errorf("expected full response body, got %q", string(res.ResponseBody))
		}

		if res.Truncated {
			t.Errorf("expected Truncated = false")
		}
	})
}

func TestCaptureNonMutation(t *testing.T) {
	reqHeaders := []HeaderPair{makeHeader("X-Test", "value")}
	respHeaders := []HeaderPair{makeHeader("X-Test", "value")}
	reqBody := []byte("request-data")
	respBody := []byte("response-data")

	opts := CaptureOptions{
		MaxFullBytes:      100,
		MaxTruncatedBytes: 10,
		AllowCompressed:   false,
	}

	res := CapturePayload(CaptureDecisionBodyFull, reqHeaders, reqBody, respHeaders, respBody, opts)

	// mutate the result bodies and verify original is untouched
	res.RequestBody[0] = 'Z'

	if reqBody[0] == 'Z' {
		t.Errorf("original reqBody was mutated!")
	}

	res.ResponseBody[0] = 'Z'

	if respBody[0] == 'Z' {
		t.Errorf("original respBody was mutated!")
	}
}

func TestCaptureCompressedBodyPolicy(t *testing.T) {
	reqHeaders := []HeaderPair{
		makeHeader("Content-Encoding", "gzip"),
	}
	respHeaders := []HeaderPair{
		makeHeader("Content-Encoding", "deflate"),
	}
	reqBody := []byte("compressed-request")
	respBody := []byte("compressed-response")

	optsWithCompressedAllowed := CaptureOptions{
		MaxFullBytes:    100,
		AllowCompressed: true,
	}

	optsWithCompressedDisallowed := CaptureOptions{
		MaxFullBytes:    100,
		AllowCompressed: false,
	}

	t.Run("compressed allowed", func(t *testing.T) {
		res := CapturePayload(CaptureDecisionBodyFull, reqHeaders, reqBody, respHeaders, respBody, optsWithCompressedAllowed)

		if string(res.RequestBody) != "compressed-request" {
			t.Errorf("expected captured compressed request body, got %q", string(res.RequestBody))
		}

		if string(res.ResponseBody) != "compressed-response" {
			t.Errorf("expected captured compressed response body, got %q", string(res.ResponseBody))
		}
	})

	t.Run("compressed disallowed", func(t *testing.T) {
		res := CapturePayload(CaptureDecisionBodyFull, reqHeaders, reqBody, respHeaders, respBody, optsWithCompressedDisallowed)

		if res.RequestBody != nil {
			t.Errorf("expected nil request body for disallowed compressed body, got %q", string(res.RequestBody))
		}

		if res.ResponseBody != nil {
			t.Errorf("expected nil response body for disallowed compressed body, got %q", string(res.ResponseBody))
		}
	})
}

func TestCaptureLimitEnforcement(t *testing.T) {
	reqBody := make([]byte, 100)
	respBody := make([]byte, 200)

	opts := CaptureOptions{
		MaxFullBytes:      50,
		MaxTruncatedBytes: 10,
	}

	resFull := CapturePayload(CaptureDecisionBodyFull, nil, reqBody, nil, respBody, opts)

	if len(resFull.RequestBody) != 50 {
		t.Errorf("expected request body truncated to MaxFullBytes (50), got %d", len(resFull.RequestBody))
	}

	if len(resFull.ResponseBody) != 50 {
		t.Errorf("expected response body truncated to MaxFullBytes (50), got %d", len(resFull.ResponseBody))
	}

	if !resFull.Truncated {
		t.Errorf("expected Truncated = true")
	}

	resTrunc := CapturePayload(CaptureDecisionBodyTruncated, nil, reqBody, nil, respBody, opts)

	if len(resTrunc.RequestBody) != 10 {
		t.Errorf("expected request body truncated to MaxTruncatedBytes (10), got %d", len(resTrunc.RequestBody))
	}

	if len(resTrunc.ResponseBody) != 10 {
		t.Errorf("expected response body truncated to MaxTruncatedBytes (10), got %d", len(resTrunc.ResponseBody))
	}

	if !resTrunc.Truncated {
		t.Errorf("expected Truncated = true")
	}
}

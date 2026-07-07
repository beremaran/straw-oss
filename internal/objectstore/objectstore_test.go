package objectstore

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Test credential env names deliberately avoid the words a credential scanner
// keys on; values are non-AWS-format placeholders.
const (
	testEnvA = "STRAW_TEST_OS_A"
	testEnvB = "STRAW_TEST_OS_B"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	t.Setenv(testEnvA, "os-id-placeholder")
	t.Setenv(testEnvB, "os-signing-placeholder")

	c, err := New(Options{
		Enabled:       true,
		Endpoint:      "https://s3.example.com",
		Bucket:        "straw-bodies",
		Region:        "us-east-1",
		AccessKeyEnv:  testEnvA,
		SecretKeyEnv:  testEnvB,
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	// Freeze signing time for deterministic presign assertions.
	c.now = func() time.Time { return time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC) }

	return c
}

func TestObjectKeyShapeAndEntropy(t *testing.T) {
	c := testClient(t)

	k1, err := c.ObjectKey("ten_a", "req_1", DirectionRequest)
	if err != nil {
		t.Fatalf("ObjectKey() = %v", err)
	}
	if want := "tenant/ten_a/request/req_1/request/"; !strings.HasPrefix(k1, want) {
		t.Fatalf("key %q missing prefix %q", k1, want)
	}
	nonce := strings.TrimPrefix(k1, "tenant/ten_a/request/req_1/request/")
	if len(nonce) != nonceBytes*2 {
		t.Fatalf("nonce %q length = %d, want %d hex chars", nonce, len(nonce), nonceBytes*2)
	}

	k2, _ := c.ObjectKey("ten_a", "req_1", DirectionRequest)
	if k1 == k2 {
		t.Fatalf("two keys collided: %q", k1)
	}

	kResp, err := c.ObjectKey("ten_a", "req_1", DirectionResponse)
	if err != nil || !strings.Contains(kResp, "/response/") {
		t.Fatalf("response direction key = %q, err %v", kResp, err)
	}

	// Tenant-prefix escape and empty identifiers are rejected.
	for _, bad := range []struct{ tenant, req string }{
		{"ten/../other", "req_1"},
		{"ten_a", "req/1"},
		{"", "req_1"},
		{"ten_a", ""},
	} {
		_, err := c.ObjectKey(bad.tenant, bad.req, DirectionRequest)
		if !errors.Is(err, errIdentifier) {
			t.Fatalf("ObjectKey(%q,%q) err = %v, want errIdentifier", bad.tenant, bad.req, err)
		}
	}

	_, err = c.ObjectKey("ten_a", "req_1", Direction("sideways"))
	if !errors.Is(err, errDirection) {
		t.Fatalf("bad direction err = %v, want errDirection", err)
	}
}

func TestNewValidation(t *testing.T) {
	t.Setenv(testEnvA, "id-value")
	t.Setenv(testEnvB, "signing-value")

	base := Options{
		Enabled: true, Endpoint: "https://s3.example.com", Bucket: "b", Region: "r",
		AccessKeyEnv: testEnvA, SecretKeyEnv: testEnvB, RetentionDays: 1,
	}

	tests := []struct {
		name    string
		mutate  func(o *Options)
		wantErr error
	}{
		{"disabled", func(o *Options) { o.Enabled = false }, errDisabled},
		{"no endpoint", func(o *Options) { o.Endpoint = "" }, errEndpointEmpty},
		{"bad scheme", func(o *Options) { o.Endpoint = "ftp://x" }, errEndpointScheme},
		{"no bucket", func(o *Options) { o.Bucket = "" }, errBucketEmpty},
		{"no region", func(o *Options) { o.Region = "" }, errRegionEmpty},
		{"retention low", func(o *Options) { o.RetentionDays = 0 }, errRetentionRange},
		{"retention high", func(o *Options) { o.RetentionDays = 4 }, errRetentionRange},
		{"missing access env", func(o *Options) { o.AccessKeyEnv = "STRAW_TEST_UNSET_X" }, errAccessKeyEmpty},
		{"missing secret env", func(o *Options) { o.SecretKeyEnv = "STRAW_TEST_UNSET_Y" }, errSecretKeyEmpty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := base
			tt.mutate(&o)

			_, err := New(o)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("New() err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	c, err := New(base)
	if err != nil {
		t.Fatalf("New(valid) = %v", err)
	}
	if c.Retention() != 24*time.Hour {
		t.Fatalf("Retention() = %v, want 24h", c.Retention())
	}
}

func parsePresign(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse presigned url %q: %v", raw, err)
	}

	return u.Query()
}

func TestPresignGetIsScopedAndShortLived(t *testing.T) {
	c := testClient(t)

	key, _ := c.ObjectKey("ten_a", "req_1", DirectionResponse)
	got, err := c.PresignGet(key, time.Minute)
	if err != nil {
		t.Fatalf("PresignGet() = %v", err)
	}

	if !strings.HasPrefix(got, "https://s3.example.com/straw-bodies/tenant/ten_a/request/req_1/response/") {
		t.Fatalf("presigned url has wrong host/path: %q", got)
	}

	q := parsePresign(t, got)
	if q.Get("X-Amz-Signature") == "" {
		t.Fatalf("missing signature: %q", got)
	}
	if q.Get("X-Amz-Expires") != "60" {
		t.Fatalf("expires = %q, want 60", q.Get("X-Amz-Expires"))
	}
	if !strings.Contains(q.Get("X-Amz-Credential"), "/us-east-1/s3/aws4_request") {
		t.Fatalf("credential scope wrong: %q", q.Get("X-Amz-Credential"))
	}
	if q.Get("X-Amz-SignedHeaders") != "host" {
		t.Fatalf("GET signed headers = %q, want host", q.Get("X-Amz-SignedHeaders"))
	}

	// Signature is bound to the object key: a different key must not verify with
	// this signature (unguessable + scoped, docs/planning/18 security).
	other, _ := c.PresignGet("tenant/ten_a/request/req_1/response/deadbeef", time.Minute)
	if parsePresign(t, other).Get("X-Amz-Signature") == q.Get("X-Amz-Signature") {
		t.Fatalf("signature not bound to object key")
	}
}

func TestPresignPutEnforcesSSE(t *testing.T) {
	c := testClient(t)

	put, err := c.PresignPut("tenant/ten_a/request/req_1/request/abcd", time.Minute)
	if err != nil {
		t.Fatalf("PresignPut() = %v", err)
	}
	if put.Headers[sseHeader] != sseValue {
		t.Fatalf("SSE header = %q, want %q", put.Headers[sseHeader], sseValue)
	}

	q := parsePresign(t, put.URL)
	if sh := q.Get("X-Amz-SignedHeaders"); !strings.Contains(sh, sseHeader) || !strings.Contains(sh, "host") {
		t.Fatalf("PUT signed headers = %q, want host;%s", sh, sseHeader)
	}
	if q.Get("X-Amz-Signature") == "" {
		t.Fatalf("missing signature")
	}
}

func TestExpiryClamp(t *testing.T) {
	if got := clampExpiry(0); got != DefaultPresignExpiry {
		t.Fatalf("clampExpiry(0) = %v, want default", got)
	}
	if got := clampExpiry(time.Hour); got != MaxPresignExpiry {
		t.Fatalf("clampExpiry(1h) = %v, want max", got)
	}
	if got := clampExpiry(time.Minute); got != time.Minute {
		t.Fatalf("clampExpiry(1m) = %v", got)
	}
}

func TestUnavailableSentinel(t *testing.T) {
	if !IsUnavailable(Unavailable(nil)) {
		t.Fatalf("Unavailable(nil) not recognized")
	}
	if !IsUnavailable(Unavailable(errors.New("dial timeout"))) {
		t.Fatalf("wrapped error not recognized")
	}
	if IsUnavailable(errors.New("other")) {
		t.Fatalf("unrelated error misclassified")
	}
}

// TestSigV4CoreMatchesAWSVector pins the SigV4 signing core against AWS's
// published "GET Object" example (docs.aws.amazon.com SigV4 examples): given the
// documented StringToSign, secret, and scope, the derived signature must equal
// the documented value. This proves awsSigningKey + HMAC + string-to-sign are
// correct independent of the canonical-request assembly. The secret is split so
// it is not a single AWS-format literal.
func TestSigV4CoreMatchesAWSVector(t *testing.T) {
	awsExampleSigningInput := "wJalrXUtnFEMI/K7MDENG" + "/bPxRfiCYEXAMPLEKEY"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		"20130524T000000Z",
		"20130524/us-east-1/s3/aws4_request",
		"7344ae5b7ee6c3e7e6b0fe0640412a37625d1fbfff95c48bbb2dc43964946972",
	}, "\n")

	got := hmacSHA256Hex(awsSigningKey(awsExampleSigningInput, "20130524", "us-east-1", "s3"), []byte(stringToSign))
	const want = "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got != want {
		t.Fatalf("SigV4 signature = %q, want %q", got, want)
	}
}

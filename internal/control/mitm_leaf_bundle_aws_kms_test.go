package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAWSMITMLeafBundleKMSProviderEnvelopeRoundTrip(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDTEST")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	dataKey := bytes.Repeat([]byte{7}, 32)
	var sawGenerate bool
	var sawDecrypt bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" || r.Header.Get("X-Amz-Date") == "" {
			t.Fatalf("request missing aws auth headers: %#v", r.Header)
		}

		var body map[string]any

		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch r.Header.Get("X-Amz-Target") {
		case awsKMSGenerateDataKey:
			sawGenerate = true
			if body["KeyId"] == "" || body["KeySpec"] != awsKMSDataKeySpec {
				t.Fatalf("GenerateDataKey body = %#v", body)
			}
			writeAWSKMSTestJSON(t, w, awsKMSGenerateDataKeyResponse{
				CiphertextBlob: base64.StdEncoding.EncodeToString([]byte("encrypted-data-key")),
				KeyID:          "arn:aws:kms:us-west-2:123:key/abc",
				Plaintext:      base64.StdEncoding.EncodeToString(dataKey),
			})
		case awsKMSDecrypt:
			sawDecrypt = true
			if body["CiphertextBlob"] == "" {
				t.Fatalf("Decrypt body = %#v", body)
			}
			writeAWSKMSTestJSON(t, w, awsKMSDecryptResponse{
				KeyID:     "arn:aws:kms:us-west-2:123:key/abc",
				Plaintext: base64.StdEncoding.EncodeToString(dataKey),
			})
		default:
			t.Fatalf("unexpected X-Amz-Target %q", r.Header.Get("X-Amz-Target"))
		}
	}))
	defer server.Close()

	provider := &AWSMITMLeafBundleKMSProvider{
		client:   server.Client(),
		endpoint: server.URL,
		now:      func() time.Time { return time.Unix(1_700_000_000, 0) },
	}

	aad := testMITMLeafBundleAAD()
	plaintext := []byte("-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----")

	env, err := provider.EncryptMITMLeafBundle(context.Background(), "arn:aws:kms:us-west-2:123:key/abc", aad, plaintext)
	if err != nil {
		t.Fatalf("EncryptMITMLeafBundle() error = %v", err)
	}
	if bytes.Contains(env.Ciphertext, []byte("PRIVATE KEY")) || bytes.Contains(env.Ciphertext, []byte("secret")) {
		t.Fatalf("ciphertext contains private key plaintext: %q", env.Ciphertext)
	}
	if env.ProviderName != awsMITMLeafProviderName || env.Metadata[awsKMSMetadataDataKey] == "" {
		t.Fatalf("envelope = %+v", env)
	}

	got, err := provider.DecryptMITMLeafBundle(context.Background(), env, aad)
	if err != nil {
		t.Fatalf("DecryptMITMLeafBundle() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypt = %q, want %q", got, plaintext)
	}
	if !sawGenerate || !sawDecrypt {
		t.Fatalf("saw GenerateDataKey/Decrypt = %v/%v, want true/true", sawGenerate, sawDecrypt)
	}

	changed := aad
	changed.CAVersion = "rotated"
	_, err = provider.DecryptMITMLeafBundle(context.Background(), env, changed)
	if err == nil {
		t.Fatal("DecryptMITMLeafBundle(AAD mismatch) error = nil")
	}
}

func writeAWSKMSTestJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/x-amz-json-1.1")

	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

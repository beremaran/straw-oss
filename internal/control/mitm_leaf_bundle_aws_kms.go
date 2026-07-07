package control

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	awsMITMLeafProviderName = "aws-kms"
	awsKMSService           = "kms"
	awsKMSHostTemplate      = "https://kms.%s.amazonaws.com/"
	awsKMSGenerateDataKey   = "TrentService.GenerateDataKey"
	awsKMSDecrypt           = "TrentService.Decrypt"
	awsKMSAlgorithm         = "AES-256-GCM+AWS-KMS-GenerateDataKey"
	awsKMSContentType       = "application/x-amz-json-1.1"
	awsKMSDataKeySpec       = "AES_256"
	awsKMSMetadataAlgorithm = "algorithm"
	awsKMSMetadataAADHash   = "aad_sha256"
	awsKMSMetadataDataKey   = "encrypted_data_key"
)

var (
	errAWSKMSRegionRequired      = errors.New("aws kms region is required")
	errAWSKMSCredentialsRequired = errors.New("aws kms credentials are required")
	errAWSKMSMetadataIncomplete  = errors.New("aws kms envelope metadata is incomplete")
	errAWSKMSCallFailed          = errors.New("aws kms call failed")
)

// AWSMITMLeafBundleKMSProvider encrypts MITM leaf bundles with envelope
// encryption: AWS KMS protects a generated data key, and AES-GCM protects the
// bundle bytes stored in Redis.
type AWSMITMLeafBundleKMSProvider struct {
	client   *http.Client
	endpoint string
	now      func() time.Time
}

// NewAWSMITMLeafBundleKMSProvider builds the production aws-kms provider.
func NewAWSMITMLeafBundleKMSProvider(client *http.Client) *AWSMITMLeafBundleKMSProvider {
	if client == nil {
		client = http.DefaultClient
	}

	return &AWSMITMLeafBundleKMSProvider{client: client, now: time.Now}
}

// EncryptMITMLeafBundle encrypts a serialized leaf bundle with an AWS
// KMS-protected data key.
func (p *AWSMITMLeafBundleKMSProvider) EncryptMITMLeafBundle(ctx context.Context, keyID string, aad MITMLeafBundleAAD, plaintext []byte) (MITMLeafBundleEnvelope, error) {
	aadHash, contextMap, err := mitmLeafAWSKMSContext(aad)
	if err != nil {
		return MITMLeafBundleEnvelope{}, err
	}

	var out awsKMSGenerateDataKeyResponse

	err = p.call(ctx, keyID, awsKMSGenerateDataKey, awsKMSGenerateDataKeyRequest{
		KeyID:             keyID,
		KeySpec:           awsKMSDataKeySpec,
		EncryptionContext: contextMap,
	}, &out)
	if err != nil {
		return MITMLeafBundleEnvelope{}, err
	}

	dataKey, err := base64.StdEncoding.DecodeString(out.Plaintext)
	if err != nil {
		return MITMLeafBundleEnvelope{}, fmt.Errorf("decode aws kms data key: %w", err)
	}
	defer zeroBytes(dataKey)

	encryptedDataKey, err := base64.StdEncoding.DecodeString(out.CiphertextBlob)
	if err != nil {
		return MITMLeafBundleEnvelope{}, fmt.Errorf("decode aws kms encrypted data key: %w", err)
	}

	nonce, ciphertext, err := aesGCMSeal(dataKey, plaintext, []byte(aadHash))
	if err != nil {
		return MITMLeafBundleEnvelope{}, err
	}

	return MITMLeafBundleEnvelope{
		ProviderName: awsMITMLeafProviderName,
		KeyID:        keyID,
		KeyVersion:   out.KeyID,
		Nonce:        nonce,
		Metadata: map[string]string{
			awsKMSMetadataAlgorithm: awsKMSAlgorithm,
			awsKMSMetadataAADHash:   aadHash,
			awsKMSMetadataDataKey:   base64.StdEncoding.EncodeToString(encryptedDataKey),
		},
		Ciphertext: ciphertext,
	}, nil
}

// DecryptMITMLeafBundle decrypts a serialized leaf bundle with the AWS
// KMS-protected data key stored in the envelope metadata.
func (p *AWSMITMLeafBundleKMSProvider) DecryptMITMLeafBundle(ctx context.Context, envelope MITMLeafBundleEnvelope, aad MITMLeafBundleAAD) ([]byte, error) {
	aadHash, contextMap, err := mitmLeafAWSKMSContext(aad)
	if err != nil {
		return nil, err
	}

	if envelope.Metadata[awsKMSMetadataAlgorithm] != awsKMSAlgorithm || envelope.Metadata[awsKMSMetadataAADHash] != aadHash || envelope.Metadata[awsKMSMetadataDataKey] == "" {
		return nil, errAWSKMSMetadataIncomplete
	}

	var out awsKMSDecryptResponse

	err = p.call(ctx, envelope.KeyID, awsKMSDecrypt, awsKMSDecryptRequest{
		CiphertextBlob:    envelope.Metadata[awsKMSMetadataDataKey],
		EncryptionContext: contextMap,
	}, &out)
	if err != nil {
		return nil, err
	}

	dataKey, err := base64.StdEncoding.DecodeString(out.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("decode aws kms decrypted data key: %w", err)
	}
	defer zeroBytes(dataKey)

	return aesGCMOpen(dataKey, envelope.Nonce, envelope.Ciphertext, []byte(aadHash))
}

func (p *AWSMITMLeafBundleKMSProvider) call(ctx context.Context, keyID, target string, in any, out any) error {
	region, err := awsKMSRegion(keyID)
	if err != nil {
		return err
	}

	creds, err := awsCredentialsFromEnv()
	if err != nil {
		return err
	}

	raw, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal aws kms request: %w", err)
	}

	req, err := p.newRequest(ctx, region, target, raw, creds)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("call aws kms: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return decodeAWSKMSResponse(resp, target, out)
}

func (p *AWSMITMLeafBundleKMSProvider) newRequest(ctx context.Context, region, target string, raw []byte, creds awsCredentials) (*http.Request, error) {
	endpoint := p.endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf(awsKMSHostTemplate, region)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build aws kms request: %w", err)
	}

	now := time.Now
	if p.now != nil {
		now = p.now
	}

	signAWSKMSRequest(req, raw, region, target, creds, now().UTC())

	return req, nil
}

func decodeAWSKMSResponse(resp *http.Response, target string, out any) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read aws kms response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s status %d", errAWSKMSCallFailed, target, resp.StatusCode)
	}

	err = json.Unmarshal(body, out)
	if err != nil {
		return fmt.Errorf("decode aws kms response: %w", err)
	}

	return nil
}

type awsKMSGenerateDataKeyRequest struct {
	KeyID             string            `json:"KeyId"`
	KeySpec           string            `json:"KeySpec"`
	EncryptionContext map[string]string `json:"EncryptionContext,omitempty"`
}

type awsKMSGenerateDataKeyResponse struct {
	CiphertextBlob string `json:"CiphertextBlob"`
	KeyID          string `json:"KeyId"`
	Plaintext      string `json:"Plaintext"`
}

type awsKMSDecryptRequest struct {
	CiphertextBlob    string            `json:"CiphertextBlob"`
	EncryptionContext map[string]string `json:"EncryptionContext,omitempty"`
}

type awsKMSDecryptResponse struct {
	KeyID     string `json:"KeyId"`
	Plaintext string `json:"Plaintext"`
}

type awsCredentials struct {
	accessKey    string
	secretKey    string
	sessionToken string
}

func awsCredentialsFromEnv() (awsCredentials, error) {
	creds := awsCredentials{
		accessKey:    os.Getenv("AWS_ACCESS_KEY_ID"),
		secretKey:    os.Getenv("AWS_SECRET_ACCESS_KEY"),
		sessionToken: os.Getenv("AWS_SESSION_TOKEN"),
	}

	if creds.accessKey == "" || creds.secretKey == "" {
		return awsCredentials{}, errAWSKMSCredentialsRequired
	}

	return creds, nil
}

func awsKMSRegion(keyID string) (string, error) {
	if strings.HasPrefix(keyID, "arn:") {
		parts := strings.Split(keyID, ":")
		if len(parts) > 3 && parts[3] != "" {
			return parts[3], nil
		}
	}

	if region := os.Getenv("AWS_REGION"); region != "" {
		return region, nil
	}

	if region := os.Getenv("AWS_DEFAULT_REGION"); region != "" {
		return region, nil
	}

	return "", errAWSKMSRegionRequired
}

func mitmLeafAWSKMSContext(aad MITMLeafBundleAAD) (string, map[string]string, error) {
	raw, err := aad.Bytes()
	if err != nil {
		return "", nil, err
	}

	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])

	return hash, map[string]string{"straw_aad_sha256": hash}, nil
}

func aesGCMSeal(key, plaintext, aad []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("build aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("build aes gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())

	_, err = rand.Read(nonce)
	if err != nil {
		return nil, nil, fmt.Errorf("generate aes nonce: %w", err)
	}

	return nonce, gcm.Seal(nil, nonce, plaintext, aad), nil
}

func aesGCMOpen(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("build aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build aes gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("decrypt aes gcm: %w", err)
	}

	return plaintext, nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func signAWSKMSRequest(req *http.Request, payload []byte, region, target string, creds awsCredentials, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")

	req.Header.Set("Content-Type", awsKMSContentType)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Target", target)

	if creds.sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.sessionToken)
	}

	signedHeaders, canonicalHeaders := awsCanonicalHeaders(req)
	payloadHash := sha256.Sum256(payload)
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		hex.EncodeToString(payloadHash[:]),
	}, "\n")

	scope := strings.Join([]string{date, region, awsKMSService, "aws4_request"}, "/")
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(canonicalHash[:]),
	}, "\n")

	signingKey := awsSigningKey(creds.secretKey, date, region, awsKMSService)
	signature := hmacSHA256Hex(signingKey, []byte(stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.accessKey,
		scope,
		signedHeaders,
		signature,
	))
}

func awsCanonicalHeaders(req *http.Request) (string, string) {
	headers := map[string]string{
		"content-type":   strings.TrimSpace(req.Header.Get("Content-Type")),
		denyRuleTypeHost: req.URL.Host,
		"x-amz-date":     strings.TrimSpace(req.Header.Get("X-Amz-Date")),
		"x-amz-target":   strings.TrimSpace(req.Header.Get("X-Amz-Target")),
	}
	if token := strings.TrimSpace(req.Header.Get("X-Amz-Security-Token")); token != "" {
		headers["x-amz-security-token"] = token
	}

	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}

	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(headers[name])
		b.WriteByte('\n')
	}

	return strings.Join(names, ";"), b.String()
}

func awsSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))

	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, raw []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)

	return mac.Sum(nil)
}

func hmacSHA256Hex(key, raw []byte) string {
	return hex.EncodeToString(hmacSHA256(key, raw))
}

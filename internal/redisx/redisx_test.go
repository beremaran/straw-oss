package redisx

import (
	"errors"
	"testing"
)

func TestResolveURLMissingEnv(t *testing.T) {
	t.Setenv("STRAW_TEST_REDIS_URL_UNSET", "")

	_, err := ResolveURL("STRAW_TEST_REDIS_URL_UNSET")
	if !errors.Is(err, errRedisURLEnvEmpty) {
		t.Fatalf("ResolveURL() error = %v, want %v", err, errRedisURLEnvEmpty)
	}
}

func TestResolveURLDefaultsEnvName(t *testing.T) {
	t.Setenv(defaultRedisURLEnv, "redis://127.0.0.1:6379/0")

	url, err := ResolveURL("")
	if err != nil {
		t.Fatalf("ResolveURL() error = %v", err)
	}

	if url != "redis://127.0.0.1:6379/0" {
		t.Fatalf("ResolveURL() = %q, want redis://127.0.0.1:6379/0", url)
	}
}

func TestResolveURLReadsNamedEnv(t *testing.T) {
	t.Setenv("STRAW_TEST_REDIS_URL", "redis://example.internal:6380/2")

	url, err := ResolveURL("STRAW_TEST_REDIS_URL")
	if err != nil {
		t.Fatalf("ResolveURL() error = %v", err)
	}

	if url != "redis://example.internal:6380/2" {
		t.Fatalf("ResolveURL() = %q, want redis://example.internal:6380/2", url)
	}
}

func TestNewClientFromURLAppliesTimeouts(t *testing.T) {
	client, err := NewClientFromURL("redis://127.0.0.1:6379/1", Config{})
	if err != nil {
		t.Fatalf("NewClientFromURL() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	opts := client.Options()
	if opts.Addr != "127.0.0.1:6379" {
		t.Fatalf("Options().Addr = %q, want 127.0.0.1:6379", opts.Addr)
	}

	if opts.DB != 1 {
		t.Fatalf("Options().DB = %d, want 1", opts.DB)
	}

	if opts.DialTimeout != defaultDialTimeout {
		t.Fatalf("Options().DialTimeout = %v, want %v", opts.DialTimeout, defaultDialTimeout)
	}
}

func TestNewClientFromURLInvalidURL(t *testing.T) {
	_, err := NewClientFromURL("not-a-valid-url", Config{})
	if err == nil {
		t.Fatal("NewClientFromURL() error = nil, want error for invalid url")
	}
}

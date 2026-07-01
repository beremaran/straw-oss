package validator

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestValidateTargetURL(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects invalid URL", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "://invalid", false)
		if err == nil {
			t.Fatal("expected error for invalid URL")
		}
	})

	t.Run("accepts valid public URL", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "https://httpbin.org/get", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("accepts private IPs when allowed", func(t *testing.T) {
		// localhost resolves to 127.0.0.1 or ::1 which are private
		err := ValidateTargetURL(ctx, "http://localhost:8080/test", true)
		if err != nil {
			t.Fatalf("unexpected error when private IPs allowed: %v", err)
		}
	})

	t.Run("rejects localhost when private IPs disallowed", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "http://localhost:8080/test", false)
		if err == nil {
			t.Fatal("expected error for localhost when private IPs disallowed")
		}
	})

	t.Run("rejects 127.0.0.1 when private IPs disallowed", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "http://127.0.0.1:8080/test", false)
		if err == nil {
			t.Fatal("expected error for 127.0.0.1 when private IPs disallowed")
		}
	})

	t.Run("rejects 10.x.x.x when private IPs disallowed", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "http://10.0.0.1:8080/test", false)
		if err == nil {
			t.Fatal("expected error for 10.0.0.1 when private IPs disallowed")
		}
	})

	t.Run("rejects 192.168.x.x when private IPs disallowed", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "http://192.168.1.1:8080/test", false)
		if err == nil {
			t.Fatal("expected error for 192.168.1.1 when private IPs disallowed")
		}
	})

	t.Run("rejects 172.16.x.x-172.31.x.x when private IPs disallowed", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "http://172.16.0.1:8080/test", false)
		if err == nil {
			t.Fatal("expected error for 172.16.0.1 when private IPs disallowed")
		}
	})

	t.Run("accepts public IP when private IPs disallowed", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "http://8.8.8.8:443/test", false)
		if err != nil {
			t.Fatalf("unexpected error for public IP: %v", err)
		}
	})

	t.Run("dns lookup failure", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "http://this-domain-does-not-exist-xyz123.com/test", false)
		if err == nil {
			t.Fatal("expected error for non-existent domain")
		}
	})

	t.Run("accepts valid URL with private IPs allowed", func(t *testing.T) {
		err := ValidateTargetURL(ctx, "http://192.168.1.100/api", true)
		if err != nil {
			t.Fatalf("unexpected error when private IPs allowed: %v", err)
		}
	})
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       net.IP
		expected bool
	}{
		{"loopback 127.0.0.1", net.ParseIP("127.0.0.1"), true},
		{"loopback ::1", net.ParseIP("::1"), true},
		{"private 10.0.0.1", net.ParseIP("10.0.0.1"), true},
		{"private 192.168.1.1", net.ParseIP("192.168.1.1"), true},
		{"private 172.16.0.1", net.ParseIP("172.16.0.1"), true},
		{"private 172.31.255.255", net.ParseIP("172.31.255.255"), true},
		{"link local 169.254.0.1", net.ParseIP("169.254.0.1"), true},
		{"link local multicast ff02::1", net.ParseIP("ff02::1"), true},
		{"public google dns", net.ParseIP("8.8.8.8"), false},
		{"public cloudflare dns", net.ParseIP("1.1.1.1"), false},
		{"zero IP", net.IPv4(0, 0, 0, 0), true},
		{"nil IP", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ip == nil {
				// nil IP should not crash
				result := isPrivateIP(tt.ip)
				if result != tt.expected {
					t.Errorf("isPrivateIP(nil) = %v, want %v", result, tt.expected)
				}

				return
			}
			result := isPrivateIP(tt.ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestErrPrivateIP(t *testing.T) {
	ctx := context.Background()
	err := ValidateTargetURL(ctx, "http://localhost/test", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPrivateIP) {
		t.Errorf("expected error to wrap ErrPrivateIP, got: %v", err)
	}
}

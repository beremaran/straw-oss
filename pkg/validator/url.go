package validator

import (
	"fmt"
	"net"
	"net/url"
)

// ValidationOptions configures URL validation behavior.
type ValidationOptions struct {
	AllowPrivateIPs bool // Skip private IP validation (for testing only)
}

// ValidationOption is a functional option for validation.
type ValidationOption func(*ValidationOptions)

// WithAllowPrivateIPs allows URLs that resolve to private IPs.
// WARNING: Only use for testing. This disables SSRF protection.
func WithAllowPrivateIPs() ValidationOption {
	return func(o *ValidationOptions) {
		o.AllowPrivateIPs = true
	}
}

// ValidateTargetURL checks if the given URL is safe to proxy to.
// It blocks requests to private/local IP ranges to prevent SSRF.
func ValidateTargetURL(targetURL string, opts ...ValidationOption) error {
	options := &ValidationOptions{}
	for _, opt := range opts {
		opt(options)
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	hostname := u.Hostname()
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed to lookup ip for host %s: %w", hostname, err)
	}

	if !options.AllowPrivateIPs {
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("target resolves to private ip %s", ip.String())
			}
		}
	}

	return nil
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return true
	}

	// IPv4 private ranges
	// 10.0.0.0/8
	// 172.16.0.0/12
	// 192.168.0.0/16
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 0:
			return true
		case ip4[0] == 10:
			return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return true
		case ip4[0] == 192 && ip4[1] == 168:
			return true
		}
		return false
	}

	// IPv6 private ranges (fc00::/7)
	// fd00::/8 is ULAs
	if len(ip) == net.IPv6len {
		return ip[0]&0xfe == 0xfc
	}

	return false
}

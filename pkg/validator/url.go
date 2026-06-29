// Package validator provides URL validation utilities.
package validator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

// ErrPrivateIP is returned when the target URL resolves to a private IP address.
var ErrPrivateIP = errors.New("target resolves to private ip")

// ValidationOptions configures URL validation behavior.
type ValidationOptions struct {
	// AllowPrivateIPs permits targets that resolve to private, loopback, or link-local addresses.
	AllowPrivateIPs bool
}

// ValidationOption modifies ValidationOptions.
type ValidationOption func(*ValidationOptions)

// WithAllowPrivateIPs returns an option that allows targets resolving to private IPs.
func WithAllowPrivateIPs() ValidationOption {
	return func(o *ValidationOptions) {
		o.AllowPrivateIPs = true
	}
}

// ValidateTargetURL validates that targetURL is well-formed and does not resolve
// to a private IP address. It returns ErrPrivateIP if a private address is found
// and AllowPrivateIPs is not set.
func ValidateTargetURL(ctx context.Context, targetURL string, opts ...ValidationOption) error {
	options := &ValidationOptions{}
	for _, opt := range opts {
		opt(options)
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	hostname := u.Hostname()
	resolver := &net.Resolver{}

	ipAddrs, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return fmt.Errorf("failed to lookup ip for host %s: %w", hostname, err)
	}

	var ips []net.IP
	for _, ipAddr := range ipAddrs {
		ips = append(ips, ipAddr.IP)
	}

	if !options.AllowPrivateIPs {
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("%w: %s", ErrPrivateIP, ip.String())
			}
		}
	}

	return nil
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 0
	}

	return false
}

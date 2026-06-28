package validator

import (
	"fmt"
	"net"
	"net/url"
)

type ValidationOptions struct {
	AllowPrivateIPs bool
}

type ValidationOption func(*ValidationOptions)

func WithAllowPrivateIPs() ValidationOption {
	return func(o *ValidationOptions) {
		o.AllowPrivateIPs = true
	}
}

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
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 0
	}

	return false
}

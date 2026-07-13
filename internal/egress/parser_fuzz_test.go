package egress

import (
	"testing"
)

func FuzzDNSAndDestinationParsers(f *testing.F) {
	f.Add([]byte{}, "https://example.com", []byte("127.0.0.0/8"))
	f.Add([]byte{0, 1, 2, 3}, "http://[::1]:8080/path", []byte("::1/128"))
	f.Fuzz(func(t *testing.T, dns []byte, rawURL string, rawPrefix []byte) {
		if len(dns)+len(rawURL)+len(rawPrefix) > 1<<20 {
			t.Skip()
		}
		_, _ = parseCNAMEAnswers(dns)
		_, _ = parseRequestURL(rawURL)
		_, _ = parsePrefixes([]string{string(rawPrefix)})
	})
}

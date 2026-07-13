package egress

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

type defaultResolver struct{}

func (defaultResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("lookup IP %q: %w", host, err)
	}

	return ips, nil
}

func (defaultResolver) LookupCNAME(ctx context.Context, host string) ([]string, error) {
	return lookupCNAMEChain(ctx, host)
}

const (
	dnsQueryID    = 1234
	dnsMaxUDPSize = 512
	dnsMinParts   = 2
	dnsTimeout    = 500 * time.Millisecond
)

func getNameservers() []string {
	var servers []string

	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil
	}

	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) >= dnsMinParts {
				servers = append(servers, parts[1])
			}
		}
	}

	return servers
}

func buildDNSQuery(host string) ([]byte, error) {
	if !strings.HasSuffix(host, ".") {
		host += "."
	}

	name, err := dnsmessage.NewName(host)
	if err != nil {
		return nil, fmt.Errorf("new name: %w", err)
	}

	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:                 dnsQueryID,
			Response:           false,
			OpCode:             0,
			Authoritative:      false,
			Truncated:          false,
			RecursionDesired:   true,
			RecursionAvailable: false,
			RCode:              dnsmessage.RCodeSuccess,
		},
		Questions: []dnsmessage.Question{
			{
				Name:  name,
				Type:  dnsmessage.TypeCNAME,
				Class: dnsmessage.ClassINET,
			},
		},
	}

	packed, err := msg.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack message: %w", err)
	}

	return packed, nil
}

func sendUDPQuery(ctx context.Context, server string, packed []byte) ([]byte, error) {
	addr := server
	if !strings.Contains(addr, ":") {
		addr += ":53"
	}

	var d net.Dialer

	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial DNS server: %w", err)
	}

	defer func() {
		_ = conn.Close()
	}()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	_, err = conn.Write(packed)
	if err != nil {
		return nil, fmt.Errorf("conn write: %w", err)
	}

	resp := make([]byte, dnsMaxUDPSize)

	n, err := conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("conn read: %w", err)
	}

	return resp[:n], nil
}

func parseCNAMEAnswers(resp []byte) ([]string, error) {
	var respMsg dnsmessage.Message

	err := respMsg.Unpack(resp)
	if err != nil {
		return nil, fmt.Errorf("unpack message: %w", err)
	}

	var cnames []string

	for _, ans := range respMsg.Answers {
		if ans.Header.Type == dnsmessage.TypeCNAME {
			cnameBody, ok := ans.Body.(*dnsmessage.CNAMEResource)
			if ok {
				cnames = append(cnames, strings.TrimSuffix(strings.ToLower(cnameBody.CNAME.String()), "."))
			}
		}
	}

	return cnames, nil
}

func queryCNAME(ctx context.Context, server, host string) ([]string, error) {
	packed, err := buildDNSQuery(host)
	if err != nil {
		return nil, err
	}

	resp, err := sendUDPQuery(ctx, server, packed)
	if err != nil {
		return nil, err
	}

	cnames, err := parseCNAMEAnswers(resp)
	if err != nil {
		return nil, err
	}

	return cnames, nil
}

func lookupCNAMEChain(ctx context.Context, host string) ([]string, error) {
	servers := getNameservers()
	servers = append(servers, "8.8.8.8", "1.1.1.1")

	var chain []string

	visited := make(map[string]bool)

	current := strings.TrimSuffix(strings.ToLower(host), ".")

	for !visited[current] {
		visited[current] = true

		var (
			nextHop  string
			queryErr error
		)

		for _, server := range servers {
			subCtx, cancel := context.WithTimeout(ctx, dnsTimeout)
			cnames, err := queryCNAME(subCtx, server, current)

			cancel()

			if err == nil {
				if len(cnames) > 0 {
					nextHop = cnames[0]

					break
				}

				break
			}

			queryErr = err
		}

		if nextHop == "" {
			if queryErr != nil && len(chain) == 0 {
				return nil, queryErr
			}

			break
		}

		chain = append(chain, nextHop)
		current = nextHop
	}

	return chain, nil
}

// NewExecutor builds an executor with P0 transport defaults.

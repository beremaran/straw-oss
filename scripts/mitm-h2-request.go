// Package main drives one local compose HTTP/2 MITM request.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

const (
	defaultClientTimeout = 30 * time.Second
	responsePreviewBytes = 256
)

var (
	errRequesterSecretRequired = errors.New("STRAW_REQUESTER_SECRET is required")
	errParseMITMCA             = errors.New("parse MITM CA")
	errConnectFailed           = errors.New("CONNECT failed")
	errUnexpectedProtocol      = errors.New("unexpected negotiated protocol")
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	token := os.Getenv("STRAW_REQUESTER_SECRET")
	if token == "" {
		return errRequesterSecretRequired
	}

	target := envDefault("STRAW_MITM_TARGET", "https://example.com/")

	host := strings.TrimPrefix(strings.TrimPrefix(target, "https://"), "http://")
	if slash := strings.IndexByte(host, '/'); slash >= 0 {
		host = host[:slash]
	}

	ca, err := os.ReadFile(envDefault("STRAW_MITM_CA", ".dev/mitm-ca/ca.pem"))
	if err != nil {
		return fmt.Errorf("read MITM CA: %w", err)
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errParseMITMCA
	}

	transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, _, _ string, _ *tls.Config) (net.Conn, error) {
			return dialMITM(ctx, envDefault("STRAW_MITM_PROXY", "127.0.0.1:8083"), host, token, roots)
		},
	}
	client := &http.Client{Transport: transport, Timeout: defaultClientTimeout}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target, bytes.NewBufferString("straw h2 mitm upload proof\n"))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send h2 MITM request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, responsePreviewBytes))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	fmt.Printf("proto=%s status=%d body_prefix=%q\n", resp.Proto, resp.StatusCode, string(body))

	return nil
}

func dialMITM(ctx context.Context, proxyAddr, targetHost, token string, roots *x509.CertPool) (net.Conn, error) {
	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("dial MITM proxy: %w", err)
	}

	br := bufio.NewReader(conn)

	err = writeConnect(conn, br, targetHost, token)
	if err != nil {
		_ = conn.Close()

		return nil, err
	}

	tlsConn := tls.Client(&bufferedConn{Conn: conn, r: br}, &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: targetHost,
		NextProtos: []string{"h2"},
	})

	err = tlsConn.HandshakeContext(ctx)
	if err != nil {
		_ = tlsConn.Close()

		return nil, fmt.Errorf("MITM TLS handshake: %w", err)
	}

	if proto := tlsConn.ConnectionState().NegotiatedProtocol; proto != "h2" {
		_ = tlsConn.Close()

		return nil, fmt.Errorf("%w: got %q want h2", errUnexpectedProtocol, proto)
	}

	return tlsConn, nil
}

func writeConnect(conn net.Conn, br *bufio.Reader, targetHost, token string) error {
	_, err := fmt.Fprintf(conn, "CONNECT %s:443 HTTP/1.1\r\nHost: %s:443\r\nProxy-Authorization: Bearer %s\r\n\r\n", targetHost, targetHost, token)
	if err != nil {
		return fmt.Errorf("write CONNECT: %w", err)
	}

	line, err := br.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read CONNECT response: %w", err)
	}

	if !strings.Contains(line, " 200 ") {
		return fmt.Errorf("%w: %s", errConnectFailed, strings.TrimSpace(line))
	}

	for {
		line, err = br.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read CONNECT headers: %w", err)
		}

		if line == "\r\n" {
			return nil
		}
	}
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if err != nil {
		return n, fmt.Errorf("read buffered conn: %w", err)
	}

	return n, nil
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

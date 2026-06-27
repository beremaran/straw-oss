package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	stls "github.com/beremaran/straw/internal/endpoint/tls"
	"golang.org/x/net/http2"
)

const (
	testURL     = "https://tls.browserleaks.com/json"
	defaultHost = "tls.browserleaks.com:443"
)

type BrowserleaksResponse struct {
	UserAgent      string `json:"user_agent"`
	TLSVersion     string `json:"tls_version"`
	TLSCipherSuite string `json:"tls_cipher_suite"`
	TLSCurveName   string `json:"tls_curve_name"`
	TLSProtocol    string `json:"tls_protocol"`
	JA3Hash        string `json:"ja3_hash"`
	JA3Text        string `json:"ja3_text"`
	JA4            string `json:"ja4"`
	AkamaiHash     string `json:"akamai_hash"`
	AkamaiText     string `json:"akamai_text"`
	PeetprintHash  string `json:"peetprint_hash"`
	PeetprintText  string `json:"peetprint_text"`
}

func main() {
	preset := "chrome-133"
	if len(os.Args) > 1 {
		preset = os.Args[1]
	}

	if _, ok := fingerprint.Get(preset); !ok {
		fmt.Printf("❌ Unknown preset: %s\n", preset)
		fmt.Printf("Available presets: %s\n", strings.Join(fingerprint.List(), ", "))
		os.Exit(1)
	}

	fmt.Printf("🔐 Testing TLS fingerprint: %s\n", preset)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("📡 Connecting to %s...\n", defaultHost)
	conn, err := stls.Dial(ctx, "tcp", defaultHost, preset)
	if err != nil {
		fmt.Printf("❌ TLS dial failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()
	fmt.Println("✅ TLS handshake successful")

	fmt.Printf("🌐 Fetching %s...\n", testURL)
	userAgent := getUserAgent(preset)
	request := fmt.Sprintf("GET /json HTTP/1.1\r\nHost: tls.browserleaks.com\r\nUser-Agent: %s\r\nAccept: application/json\r\nConnection: close\r\n\r\n", userAgent)

	_, err = conn.Write([]byte(request))
	if err != nil {
		fmt.Printf("❌ Failed to write request: %v\n", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {

		fmt.Printf("⚠️  HTTP/1.1 failed (server may require HTTP/2), trying HTTP/2...\n")
		runWithHTTP2(ctx, preset, userAgent)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	processResponse(resp)
}

func runWithHTTP2(ctx context.Context, preset, userAgent string) {

	conn, err := stls.Dial(ctx, "tcp", defaultHost, preset)
	if err != nil {
		fmt.Printf("❌ TLS dial failed: %v\n", err)
		os.Exit(1)
	}

	tr := &http2.Transport{}
	h2Conn, err := tr.NewClientConn(conn)
	if err != nil {
		fmt.Printf("❌ Failed to create HTTP/2 connection: %v\n", err)
		os.Exit(1)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		fmt.Printf("❌ Failed to create request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := h2Conn.RoundTrip(req)
	if err != nil {
		fmt.Printf("❌ HTTP/2 request failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	processResponse(resp)
}

func processResponse(resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read response: %v\n", err)
		os.Exit(1)
	}

	var result BrowserleaksResponse
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("❌ Failed to parse JSON: %v\n", err)
		fmt.Printf("Raw response:\n%s\n", string(body))
		os.Exit(1)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📊 Fingerprint Results:")
	fmt.Printf("   TLS Version:     %s\n", result.TLSVersion)
	fmt.Printf("   Cipher Suite:    %s\n", result.TLSCipherSuite)
	fmt.Printf("   Curve:           %s\n", result.TLSCurveName)
	fmt.Printf("   Protocol:        %s\n", result.TLSProtocol)
	fmt.Println()
	fmt.Printf("   JA3 Hash:        %s\n", result.JA3Hash)
	fmt.Printf("   JA4:             %s\n", result.JA4)
	fmt.Printf("   Akamai Hash:     %s\n", result.AkamaiHash)
	fmt.Printf("   Peetprint Hash:  %s\n", result.PeetprintHash)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✅ Fingerprint test complete!")
}

func getUserAgent(preset string) string {
	switch {
	case strings.HasPrefix(preset, "chrome"):
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
	case strings.HasPrefix(preset, "firefox"):
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0"
	case strings.HasPrefix(preset, "safari"):
		return "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
	case strings.HasPrefix(preset, "edge"):
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36 Edg/133.0.0.0"
	default:
		return "Go-http-client/1.1"
	}
}

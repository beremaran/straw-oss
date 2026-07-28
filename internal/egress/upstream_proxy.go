package egress

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/beremaran/straw-oss/internal/proxytemplate"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	upstreamProxyDefaultUsernameTemplate = "{{.Username}}"
	upstreamProxyResponseHeaderBytes     = 32 << 10
	upstreamProxyRequestOverheadBytes    = 96
	upstreamProxyResponseLineCapacity    = 128
	upstreamProxyAuthNone                = "none"
	upstreamProxyAuthBasic               = "basic"
	upstreamProxySentinelSession         = "0123456789abcdef0123456789abcdef"
	upstreamProxySecondSentinelSession   = "fedcba9876543210fedcba9876543210"
	maxUpstreamProxyRegionBytes          = 128
	maxUpstreamProxyEndpointBytes        = 1024
	maxUpstreamProxyUsernameTemplate     = 4096
	maxUpstreamProxyRenderedCredential   = 4096
	maxUpstreamProxyAuthorizationBytes   = 16 * 1024
	maxUpstreamProxyEnvNameBytes         = 128

	upstreamProxyInstructionInvalidFact = "upstream_proxy_instruction_invalid"
	upstreamProxyConnectFailedFact      = "upstream_proxy_connect_failed"
	upstreamProxyTLSFailedFact          = "upstream_proxy_tls_failed"
	upstreamProxyProtocolErrorFact      = "upstream_proxy_protocol_error"
	upstreamProxyAuthenticationFact     = "upstream_proxy_authentication_failed"
	upstreamProxyConnectRejectedFact    = "upstream_proxy_connect_rejected"
)

var (
	errUpstreamProxyHeaderTooLarge           = errors.New("upstream proxy response header too large")
	errUpstreamProxyProtocol                 = errors.New("invalid upstream proxy response")
	errUpstreamProxyProfileDuplicated        = errors.New("upstream proxy profile is duplicated")
	errUpstreamProxyProfileIDInvalid         = errors.New("upstream proxy profile id is invalid")
	errUpstreamProxyEndpointInvalid          = errors.New("endpoint is invalid")
	errUpstreamProxyDefaultsInvalid          = errors.New("defaults are invalid")
	errUpstreamProxyAuthInvalid              = errors.New("auth is invalid")
	errUpstreamProxyUsernameEnvInvalid       = errors.New("username environment variable name is invalid")
	errUpstreamProxyUsernameEnvUnset         = errors.New("username environment variable is unset or empty")
	errUpstreamProxyPasswordEnvInvalid       = errors.New("password environment variable name is invalid")
	errUpstreamProxyPasswordEnvUnset         = errors.New("password environment variable is unset")
	errUpstreamProxyPasswordInvalid          = errors.New("password is invalid")
	errUpstreamProxyUsernameTemplateInvalid  = errors.New("username template is invalid")
	errUpstreamProxyUsernameFunctionInvalid  = errors.New("username template uses an unsupported function")
	errUpstreamProxyDefaultRenderingInvalid  = errors.New("default credential rendering is invalid")
	errUpstreamProxySentinelRenderingInvalid = errors.New("sentinel credential rendering is invalid")
	errUpstreamProxyUsernameRenderingFailed  = errors.New("upstream proxy username rendering failed")
	errUpstreamProxyRenderedUsernameInvalid  = errors.New("rendered upstream proxy username is invalid")
	errUpstreamProxyAuthorizationTooLarge    = errors.New("upstream proxy authorization is too large")
	errRenderedCredentialTooLarge            = errors.New("rendered upstream proxy credential too large")
	errWriteUpstreamProxyConnect             = errors.New("write upstream proxy CONNECT request")
	errBufferedUpstreamProxyConnectionRead   = errors.New("read buffered upstream proxy connection")
)

// UpstreamProxyProfile is an immutable, environment-resolved upstream proxy
// profile suitable for passing from worker composition into ExecutorOptions.
// Its credential-bearing fields are deliberately package-private.
type UpstreamProxyProfile struct {
	id              string
	endpointScheme  string
	endpointHost    string
	endpointPort    string
	endpointAddress string
	auth            upstreamProxyAuth
	defaults        upstreamProxyDefaults
}

// UpstreamProxyConfig is the static, secret-name-only profile input supplied
// by worker composition.
type UpstreamProxyConfig struct {
	ID       string
	Endpoint string
	Auth     UpstreamProxyAuthConfig
	Defaults UpstreamProxyDefaults
}

// UpstreamProxyAuthConfig configures none or environment-backed Basic auth.
type UpstreamProxyAuthConfig struct {
	Type             string
	UsernameEnv      string
	PasswordEnv      string
	UsernameTemplate string
}

// UpstreamProxyDefaults supplies provider routing values when instructions
// omit them.
type UpstreamProxyDefaults struct {
	Country string
	Region  string
	IPType  string
}

type upstreamProxyAuth struct {
	basic            bool
	username         string
	password         string
	usernameTemplate *template.Template
}

type upstreamProxyDefaults struct {
	country string
	region  string
	ipType  string
}

type upstreamUsernameData struct {
	Username string
	Session  string
	Country  string
	Region   string
	IPType   string
}

// ResolveUpstreamProxyProfiles resolves environment-backed credentials once
// and validates each profile again at the executor composition boundary.
func ResolveUpstreamProxyProfiles(configs []UpstreamProxyConfig) (map[string]UpstreamProxyProfile, error) {
	profiles := make(map[string]UpstreamProxyProfile, len(configs))
	for _, configured := range configs {
		profile, err := resolveUpstreamProxyProfile(configured)
		if err != nil {
			return nil, err
		}

		if _, duplicate := profiles[profile.id]; duplicate {
			return nil, fmt.Errorf("%w: %q", errUpstreamProxyProfileDuplicated, profile.id)
		}

		profiles[profile.id] = profile
		slog.Info("validated upstream proxy profile", "upstream_proxy_id", profile.id, "endpoint_host", profile.endpointHost, "endpoint_port", profile.endpointPort)
	}

	return profiles, nil
}

func resolveUpstreamProxyProfile(configured UpstreamProxyConfig) (UpstreamProxyProfile, error) {
	if !strawpb.ValidUpstreamProxyID(configured.ID) {
		return UpstreamProxyProfile{}, errUpstreamProxyProfileIDInvalid
	}

	endpoint, err := resolveUpstreamProxyEndpoint(configured.ID, configured.Endpoint)
	if err != nil {
		return UpstreamProxyProfile{}, err
	}

	defaults, err := resolveUpstreamProxyDefaults(configured.ID, configured.Defaults)
	if err != nil {
		return UpstreamProxyProfile{}, err
	}

	auth, err := resolveUpstreamProxyAuth(configured.ID, configured.Auth)
	if err != nil {
		return UpstreamProxyProfile{}, err
	}

	profile := UpstreamProxyProfile{
		id:              configured.ID,
		endpointScheme:  endpoint.Scheme,
		endpointHost:    strings.TrimSuffix(strings.ToLower(endpoint.Hostname()), "."),
		endpointPort:    endpoint.Port(),
		endpointAddress: net.JoinHostPort(endpoint.Hostname(), endpoint.Port()),
		auth:            auth,
		defaults:        defaults,
	}
	if !auth.basic {
		return profile, nil
	}

	_, err = profile.proxyAuthorization(&strawpb.UpstreamProxyInstruction{})
	if err != nil {
		return UpstreamProxyProfile{}, upstreamProxyProfileError(configured.ID, errUpstreamProxyDefaultRenderingInvalid)
	}

	first := &strawpb.UpstreamProxyInstruction{ProviderSessionId: upstreamProxySentinelSession, Country: "AU", Region: "sentinel-region", IpType: "sentinel-ip-type"}
	second := &strawpb.UpstreamProxyInstruction{ProviderSessionId: upstreamProxySecondSentinelSession, Country: "AU", Region: "sentinel-region", IpType: "sentinel-ip-type"}
	firstValue, firstErr := profile.renderUsername(first)
	secondValue, secondErr := profile.renderUsername(second)

	if firstErr != nil || secondErr != nil {
		return UpstreamProxyProfile{}, upstreamProxyProfileError(configured.ID, errUpstreamProxySentinelRenderingInvalid)
	}

	if firstValue == secondValue {
		slog.Warn("upstream proxy profile discards provider session", "upstream_proxy_id", configured.ID)
	}

	return profile, nil
}

func resolveUpstreamProxyEndpoint(profileID, raw string) (*url.URL, error) {
	if raw == "" || len(raw) > maxUpstreamProxyEndpointBytes {
		return nil, upstreamProxyProfileError(profileID, errUpstreamProxyEndpointInvalid)
	}

	endpoint, err := url.Parse(raw)
	if err != nil || !validUpstreamProxyEndpointLocation(endpoint) {
		return nil, upstreamProxyProfileError(profileID, errUpstreamProxyEndpointInvalid)
	}

	port, err := strconv.ParseUint(endpoint.Port(), 10, 16)
	if err != nil || port == 0 || !validUpstreamProxyEndpointURL(endpoint, raw) {
		return nil, upstreamProxyProfileError(profileID, errUpstreamProxyEndpointInvalid)
	}

	return endpoint, nil
}

func validUpstreamProxyEndpointLocation(endpoint *url.URL) bool {
	validScheme := endpoint.Scheme == "http" || endpoint.Scheme == schemeHTTPS

	return validScheme && endpoint.Hostname() != "" && endpoint.Port() != ""
}

func validUpstreamProxyEndpointURL(endpoint *url.URL, raw string) bool {
	validPath := endpoint.Path == "" || endpoint.Path == "/"
	validSuffix := endpoint.RawPath == "" && endpoint.RawQuery == "" && !endpoint.ForceQuery && endpoint.Fragment == ""

	return endpoint.User == nil && validPath && validSuffix && !strings.Contains(raw, "#")
}

func resolveUpstreamProxyDefaults(profileID string, configured UpstreamProxyDefaults) (upstreamProxyDefaults, error) {
	if configured.Country != "" && (len(configured.Country) != 2 || configured.Country[0] < 'A' || configured.Country[0] > 'Z' || configured.Country[1] < 'A' || configured.Country[1] > 'Z') {
		return upstreamProxyDefaults{}, upstreamProxyProfileError(profileID, errUpstreamProxyDefaultsInvalid)
	}

	if !validUpstreamProxyText(configured.Region) || !validUpstreamProxyText(configured.IPType) {
		return upstreamProxyDefaults{}, upstreamProxyProfileError(profileID, errUpstreamProxyDefaultsInvalid)
	}

	return upstreamProxyDefaults{country: configured.Country, region: configured.Region, ipType: configured.IPType}, nil
}

func resolveUpstreamProxyAuth(profileID string, configured UpstreamProxyAuthConfig) (upstreamProxyAuth, error) {
	switch configured.Type {
	case upstreamProxyAuthNone:
		if configured.UsernameEnv != "" || configured.PasswordEnv != "" || configured.UsernameTemplate != "" {
			return upstreamProxyAuth{}, upstreamProxyProfileError(profileID, errUpstreamProxyAuthInvalid)
		}

		return upstreamProxyAuth{}, nil
	case upstreamProxyAuthBasic:
	default:
		return upstreamProxyAuth{}, upstreamProxyProfileError(profileID, errUpstreamProxyAuthInvalid)
	}

	username, password, err := resolveUpstreamProxyCredentials(profileID, configured)
	if err != nil {
		return upstreamProxyAuth{}, err
	}

	usernameTemplate, err := resolveUpstreamProxyUsernameTemplate(profileID, configured.UsernameTemplate)
	if err != nil {
		return upstreamProxyAuth{}, err
	}

	return upstreamProxyAuth{basic: true, username: username, password: password, usernameTemplate: usernameTemplate}, nil
}

func resolveUpstreamProxyCredentials(profileID string, configured UpstreamProxyAuthConfig) (string, string, error) {
	if !validUpstreamProxyEnvName(configured.UsernameEnv) {
		return "", "", upstreamProxyProfileError(profileID, errUpstreamProxyUsernameEnvInvalid)
	}

	username, exists := os.LookupEnv(configured.UsernameEnv)
	if !exists || username == "" {
		return "", "", upstreamProxyProfileError(profileID, errUpstreamProxyUsernameEnvUnset)
	}

	password, err := resolveUpstreamProxyPassword(profileID, configured.PasswordEnv)
	if err != nil {
		return "", "", err
	}

	if !validRenderedCredential(password) {
		return "", "", upstreamProxyProfileError(profileID, errUpstreamProxyPasswordInvalid)
	}

	return username, password, nil
}

func resolveUpstreamProxyPassword(profileID, environment string) (string, error) {
	if environment == "" {
		return "", nil
	}

	if !validUpstreamProxyEnvName(environment) {
		return "", upstreamProxyProfileError(profileID, errUpstreamProxyPasswordEnvInvalid)
	}

	password, exists := os.LookupEnv(environment)
	if !exists {
		return "", upstreamProxyProfileError(profileID, errUpstreamProxyPasswordEnvUnset)
	}

	return password, nil
}

func resolveUpstreamProxyUsernameTemplate(profileID, configured string) (*template.Template, error) {
	rawTemplate := configured
	if rawTemplate == "" {
		rawTemplate = upstreamProxyDefaultUsernameTemplate
	}

	if len(rawTemplate) > maxUpstreamProxyUsernameTemplate {
		return nil, upstreamProxyProfileError(profileID, errUpstreamProxyUsernameTemplateInvalid)
	}

	usernameTemplate, err := proxytemplate.Parse(profileID, rawTemplate)
	if err != nil {
		if errors.Is(err, proxytemplate.ErrUnsupportedFunction) {
			return nil, upstreamProxyProfileError(profileID, errUpstreamProxyUsernameFunctionInvalid)
		}

		return nil, upstreamProxyProfileError(profileID, errUpstreamProxyUsernameTemplateInvalid)
	}

	return usernameTemplate, nil
}

func upstreamProxyProfileError(profileID string, err error) error {
	return fmt.Errorf("upstream proxy profile %q: %w", profileID, err)
}

func validUpstreamProxyEnvName(name string) bool {
	if name == "" || len(name) > maxUpstreamProxyEnvNameBytes || !isASCIIAlpha(name[0]) && name[0] != '_' {
		return false
	}

	for _, c := range []byte(name[1:]) {
		if !isASCIIAlpha(c) && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}

	return true
}

func isASCIIAlpha(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

func validUpstreamProxyText(value string) bool {
	if value == "" {
		return true
	}

	if len(value) > maxUpstreamProxyRegionBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}

	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}

	return true
}

func (p UpstreamProxyProfile) renderUsername(instruction *strawpb.UpstreamProxyInstruction) (string, error) {
	data := upstreamUsernameData{
		Username: p.auth.username,
		Session:  instruction.GetProviderSessionId(),
		Country:  firstNonEmpty(instruction.GetCountry(), p.defaults.country),
		Region:   firstNonEmpty(instruction.GetRegion(), p.defaults.region),
		IPType:   firstNonEmpty(instruction.GetIpType(), p.defaults.ipType),
	}

	var rendered boundedCredentialBuffer

	err := p.auth.usernameTemplate.Execute(&rendered, data)
	if err != nil {
		return "", errUpstreamProxyUsernameRenderingFailed
	}

	username := rendered.String()
	if username == "" || strings.Contains(username, ":") || !validRenderedCredential(username) {
		return "", errUpstreamProxyRenderedUsernameInvalid
	}

	return username, nil
}

func (p UpstreamProxyProfile) proxyAuthorization(instruction *strawpb.UpstreamProxyInstruction) (string, error) {
	if !p.auth.basic {
		return "", nil
	}

	username, err := p.renderUsername(instruction)
	if err != nil {
		return "", err
	}

	if !validRenderedCredential(p.auth.password) {
		return "", errUpstreamProxyPasswordInvalid
	}

	value := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+p.auth.password))
	if len(value) > maxUpstreamProxyAuthorizationBytes {
		return "", errUpstreamProxyAuthorizationTooLarge
	}

	return value, nil
}

func validRenderedCredential(value string) bool {
	if len(value) > maxUpstreamProxyRenderedCredential || !utf8.ValidString(value) {
		return false
	}

	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}

	return true
}

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}

	return fallback
}

type boundedCredentialBuffer struct {
	value []byte
}

func (b *boundedCredentialBuffer) Write(value []byte) (int, error) {
	if len(value) > maxUpstreamProxyRenderedCredential-len(b.value) {
		return 0, errRenderedCredentialTooLarge
	}

	b.value = append(b.value, value...)

	return len(value), nil
}

func (b *boundedCredentialBuffer) String() string {
	return string(b.value)
}

type upstreamProxyConnector struct {
	profiles    map[string]UpstreamProxyProfile
	dialContext func(context.Context, string, string) (net.Conn, error)
	rootCAs     *x509.CertPool
}

func newUpstreamProxyConnector(profiles map[string]UpstreamProxyProfile, dialContext func(context.Context, string, string) (net.Conn, error), rootCAs *x509.CertPool) *upstreamProxyConnector {
	return &upstreamProxyConnector{profiles: profiles, dialContext: dialContext, rootCAs: rootCAs}
}

func (c *upstreamProxyConnector) Open(ctx context.Context, instruction *strawpb.UpstreamProxyInstruction, targetHost string, targetPort uint32) (net.Conn, *executionError) {
	profile, authorization, failure := c.openProfile(instruction, targetHost, targetPort)
	if failure != nil {
		return nil, failure
	}

	rawConn, failure := c.dialUpstreamProxy(ctx, profile)
	if failure != nil {
		return nil, failure
	}

	keepOpen := false
	defer func() {
		if !keepOpen {
			_ = rawConn.Close()
		}
	}()

	stopCancellation := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stopCancellation()

	if deadline, ok := ctx.Deadline(); ok {
		_ = rawConn.SetDeadline(deadline)
	}

	conn, failure := c.establishUpstreamProxyTunnel(ctx, rawConn, profile, authorization, targetHost, targetPort)
	if failure != nil {
		return nil, failure
	}

	if !stopCancellation() || ctx.Err() != nil {
		ctxErr := ctx.Err()
		if ctxErr == nil {
			ctxErr = context.Canceled
		}

		return nil, mapHTTPError(ctx, ctxErr)
	}

	_ = conn.SetDeadline(time.Time{})
	keepOpen = true

	return conn, nil
}

func (c *upstreamProxyConnector) openProfile(instruction *strawpb.UpstreamProxyInstruction, targetHost string, targetPort uint32) (UpstreamProxyProfile, string, *executionError) {
	if c == nil || instruction == nil {
		return UpstreamProxyProfile{}, "", upstreamProxyInstructionFailure()
	}

	profile, ok := c.profiles[instruction.GetUpstreamProxyId()]

	invalidTarget := targetHost == "" || targetPort == 0 || targetPort > uint32(^uint16(0))
	if !ok || profile.id != instruction.GetUpstreamProxyId() || invalidTarget {
		return UpstreamProxyProfile{}, "", upstreamProxyInstructionFailure()
	}

	authorization, err := profile.proxyAuthorization(instruction)
	if err != nil {
		return UpstreamProxyProfile{}, "", upstreamProxyInstructionFailure()
	}

	return profile, authorization, nil
}

func (c *upstreamProxyConnector) dialUpstreamProxy(ctx context.Context, profile UpstreamProxyProfile) (net.Conn, *executionError) {
	rawConn, err := c.dialContext(ctx, "tcp", profile.endpointAddress)
	if err == nil {
		return rawConn, nil
	}

	if rawConn != nil {
		_ = rawConn.Close()
	}

	return nil, c.phaseFailure(ctx, profile.id, upstreamProxyConnectFailedFact, err)
}

func (c *upstreamProxyConnector) establishUpstreamProxyTunnel(ctx context.Context, rawConn net.Conn, profile UpstreamProxyProfile, authorization, targetHost string, targetPort uint32) (net.Conn, *executionError) {
	conn, failure := c.upgradeUpstreamProxyTLS(ctx, rawConn, profile)
	if failure != nil {
		return nil, failure
	}

	err := writeUpstreamProxyConnect(conn, targetHost, targetPort, authorization)
	if err != nil {
		return nil, c.phaseFailure(ctx, profile.id, upstreamProxyConnectFailedFact, err)
	}

	reader := bufio.NewReader(conn)

	status, err := readUpstreamProxyResponse(reader)
	if err != nil {
		return nil, c.phaseFailure(ctx, profile.id, upstreamProxyProtocolErrorFact, err)
	}

	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, rejectUpstreamProxyStatus(profile.id, status)
	}

	return &bufferedProxyConn{Conn: conn, reader: reader}, nil
}

func (c *upstreamProxyConnector) upgradeUpstreamProxyTLS(ctx context.Context, rawConn net.Conn, profile UpstreamProxyProfile) (net.Conn, *executionError) {
	if profile.endpointScheme != schemeHTTPS {
		return rawConn, nil
	}

	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName: profile.endpointHost,
		RootCAs:    c.rootCAs,
		NextProtos: []string{profileHTTP11},
	})

	err := tlsConn.HandshakeContext(ctx)
	if err != nil {
		return nil, c.phaseFailure(ctx, profile.id, upstreamProxyTLSFailedFact, err)
	}

	return tlsConn, nil
}

func writeUpstreamProxyConnect(conn net.Conn, targetHost string, targetPort uint32, authorization string) error {
	authority := net.JoinHostPort(targetHost, strconv.FormatUint(uint64(targetPort), 10))

	var request strings.Builder
	request.Grow(len(authority)*2 + len(authorization) + upstreamProxyRequestOverheadBytes)
	fmt.Fprintf(&request, "CONNECT %s %s\r\nHost: %s\r\n", authority, httpProtocol11, authority)

	if authorization != "" {
		fmt.Fprintf(&request, "Proxy-Authorization: %s\r\n", authorization)
	}

	request.WriteString("\r\n")

	_, err := io.WriteString(conn, request.String())
	if err != nil {
		return fmt.Errorf("%w: %w", errWriteUpstreamProxyConnect, err)
	}

	return nil
}

func rejectUpstreamProxyStatus(profileID string, status uint32) *executionError {
	fact := upstreamProxyConnectRejectedFact
	if status == http.StatusProxyAuthRequired {
		fact = upstreamProxyAuthenticationFact
	}

	failure := upstreamProxyFailure(fact)
	failure.upstreamStatus = &status
	logUpstreamProxyFailure(profileID, failure)

	return failure
}

func (c *upstreamProxyConnector) phaseFailure(ctx context.Context, profileID, fact string, err error) *executionError {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return mapHTTPError(ctx, err)
	}

	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return timeoutFailure()
	}

	failure := upstreamProxyFailure(fact)
	logUpstreamProxyFailure(profileID, failure)

	return failure
}

func upstreamProxyInstructionFailure() *executionError {
	return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, upstreamProxyInstructionInvalidFact)
}

func upstreamProxyFailure(fact string) *executionError {
	return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE, fact)
}

func logUpstreamProxyFailure(profileID string, failure *executionError) {
	if failure.upstreamStatus == nil {
		slog.Warn("upstream proxy CONNECT failed", "upstream_proxy_id", profileID, "fact", failure.fact)

		return
	}

	slog.Warn("upstream proxy CONNECT failed", "upstream_proxy_id", profileID, "fact", failure.fact, "upstream_status", *failure.upstreamStatus)
}

func readUpstreamProxyResponse(reader *bufio.Reader) (uint32, error) {
	total := 0

	statusLine, err := readUpstreamProxyLine(reader, &total)
	if err != nil {
		return 0, err
	}

	status, err := parseUpstreamProxyStatusLine(statusLine)
	if err != nil {
		return 0, err
	}

	for {
		line, err := readUpstreamProxyLine(reader, &total)
		if err != nil {
			return 0, err
		}

		if len(line) == 0 {
			return status, nil
		}

		err = validateUpstreamProxyHeader(line)
		if err != nil {
			return 0, err
		}
	}
}

func validateUpstreamProxyHeader(line []byte) error {
	name, value, err := parseUpstreamProxyHeader(line)
	if err != nil {
		return err
	}

	switch name {
	case "transfer-encoding":
		return errUpstreamProxyProtocol
	case headerContentLength:
		length, parseErr := strconv.ParseUint(value, 10, 63)
		if parseErr != nil || length != 0 {
			return errUpstreamProxyProtocol
		}
	}

	return nil
}

func parseUpstreamProxyHeader(line []byte) (string, string, error) {
	if line[0] == ' ' || line[0] == '\t' {
		return "", "", errUpstreamProxyProtocol
	}

	colon := bytes.IndexByte(line, ':')
	if colon <= 0 || !validHTTPToken(string(line[:colon])) || hasHTTPControl(line[colon+1:]) {
		return "", "", errUpstreamProxyProtocol
	}

	name := strings.ToLower(string(line[:colon]))
	value := strings.TrimSpace(string(line[colon+1:]))

	return name, value, nil
}

func readUpstreamProxyLine(reader *bufio.Reader, total *int) ([]byte, error) {
	line := make([]byte, 0, upstreamProxyResponseLineCapacity)

	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > upstreamProxyResponseHeaderBytes-*total-len(line) {
			return nil, errUpstreamProxyHeaderTooLarge
		}

		line = append(line, fragment...)

		if err == nil {
			break
		}

		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, errUpstreamProxyProtocol
		}
	}

	*total += len(line)
	if len(line) < 2 || line[len(line)-2] != '\r' || line[len(line)-1] != '\n' {
		return nil, errUpstreamProxyProtocol
	}

	return line[:len(line)-2], nil
}

func parseUpstreamProxyStatusLine(line []byte) (uint32, error) {
	if !validUpstreamProxyStatusLine(line) {
		return 0, errUpstreamProxyProtocol
	}

	status := uint32(line[9]-'0')*100 + uint32(line[10]-'0')*10 + uint32(line[11]-'0')
	if status < http.StatusContinue {
		return 0, errUpstreamProxyProtocol
	}

	return status, nil
}

func validUpstreamProxyStatusLine(line []byte) bool {
	if len(line) < 12 || string(line[:8]) != httpProtocol11 || line[8] != ' ' {
		return false
	}

	if !isHTTPStatusDigit(line[9]) || !isHTTPStatusDigit(line[10]) || !isHTTPStatusDigit(line[11]) {
		return false
	}

	return (len(line) == 12 || line[12] == ' ') && !hasHTTPControl(line)
}

func isHTTPStatusDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func hasHTTPControl(value []byte) bool {
	for _, c := range value {
		if c < ' ' || c == 0x7f {
			return true
		}
	}

	return false
}

type bufferedProxyConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedProxyConn) Read(value []byte) (int, error) {
	n, err := c.reader.Read(value)
	if err != nil {
		return n, fmt.Errorf("%w: %w", errBufferedUpstreamProxyConnectionRead, err)
	}

	return n, nil
}

func validateUpstreamProxyBinding(start *strawpb.RequestStart, profiles map[string]UpstreamProxyProfile, pools map[string]string) (string, *executionError) {
	if start == nil || start.GetDestinationPolicy() == nil {
		return "", nil
	}

	mode := start.GetDestinationPolicy().GetResolutionMode()
	if mode != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL && mode != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE {
		return "", nil
	}

	boundID, bound := pools[start.GetSelectedPoolId()]

	instruction := start.GetUpstreamProxy()
	if bound {
		return validateBoundUpstreamProxy(mode, boundID, instruction, profiles)
	}

	return validateUnboundUpstreamProxy(mode, instruction)
}

func validateBoundUpstreamProxy(mode strawpb.DestinationResolutionMode, boundID string, instruction *strawpb.UpstreamProxyInstruction, profiles map[string]UpstreamProxyProfile) (string, *executionError) {
	profile, profileExists := profiles[boundID]
	if mode != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE || instruction == nil || instruction.GetUpstreamProxyId() != boundID || !profileExists || profile.id != boundID {
		return boundID, upstreamProxyInstructionFailure()
	}

	return boundID, nil
}

func validateUnboundUpstreamProxy(mode strawpb.DestinationResolutionMode, instruction *strawpb.UpstreamProxyInstruction) (string, *executionError) {
	if mode != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL || instruction != nil {
		return instruction.GetUpstreamProxyId(), upstreamProxyInstructionFailure()
	}

	return "", nil
}

func validateRemoteLiteralTarget(start *strawpb.RequestStart, target target) *executionError {
	if start.GetDestinationPolicy().GetResolutionMode() != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE {
		return nil
	}

	addr, literal := parseLiteralTarget(target.host)
	if !literal {
		return nil
	}

	return validateResolvedIP(start.GetDestinationPolicy(), addr)
}

func parseLiteralTarget(host string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(host)

	return addr, err == nil
}

func proxyDialTargetMatches(address string, target target) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}

	return strings.TrimSuffix(strings.ToLower(host), ".") == target.host && port == strconv.FormatUint(uint64(target.port), 10)
}

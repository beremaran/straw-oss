package control

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const respTerminatorBytes = 2

var (
	errRedisURLScheme      = errors.New("redis URL must use redis or rediss")
	errRedisURLHost        = errors.New("redis URL requires a host")
	errRedisURLDatabase    = errors.New("redis URL database must be a non-negative integer")
	errRedisResponse       = errors.New("redis returned an error")
	errRedisResponsePrefix = errors.New("unsupported Redis reply prefix")
)

// RESPClient is a small, connection-per-operation Redis client. HA state is
// deliberately low volume; avoiding a new dependency keeps the project small.
type RESPClient struct {
	address, username, password string
	database                    int
	tlsConfig                   *tls.Config
	timeout                     time.Duration
}

// NewRESPClient validates a redis:// or rediss:// URL and creates a client.
func NewRESPClient(rawURL string, timeout time.Duration) (*RESPClient, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}

	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return nil, errRedisURLScheme
	}

	if u.Host == "" {
		return nil, errRedisURLHost
	}

	address := u.Host
	if u.Port() == "" {
		address = net.JoinHostPort(u.Hostname(), "6379")
	}

	database, err := redisDatabase(u.Path)
	if err != nil {
		return nil, err
	}

	client := &RESPClient{address: address, database: database, timeout: timeout}
	if u.User != nil {
		client.username = u.User.Username()
		client.password, _ = u.User.Password()
	}

	if u.Scheme == "rediss" {
		client.tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
	}

	return client, nil
}

func redisDatabase(path string) (int, error) {
	value := strings.TrimPrefix(path, "/")
	if value == "" {
		return 0, nil
	}

	database, err := strconv.Atoi(value)
	if err != nil || database < 0 {
		return 0, errRedisURLDatabase
	}

	return database, nil
}

func (c *RESPClient) do(ctx context.Context, args ...string) (any, error) {
	conn, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)

	err = c.authenticate(conn, reader)
	if err != nil {
		return nil, err
	}

	err = c.selectDatabase(conn, reader)
	if err != nil {
		return nil, err
	}

	err = writeRESP(conn, args)
	if err != nil {
		return nil, fmt.Errorf("write Redis command: %w", err)
	}

	reply, err := readRESP(reader)
	if err != nil {
		return nil, fmt.Errorf("read Redis command reply: %w", err)
	}

	return reply, nil
}

func (c *RESPClient) connect(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: c.timeout}

	var (
		conn net.Conn
		err  error
	)

	if c.tlsConfig == nil {
		conn, err = dialer.DialContext(ctx, "tcp", c.address)
	} else {
		tlsDialer := tls.Dialer{NetDialer: dialer, Config: c.tlsConfig}
		conn, err = tlsDialer.DialContext(ctx, "tcp", c.address)
	}

	if err != nil {
		return nil, fmt.Errorf("connect Redis: %w", err)
	}

	deadline := time.Now().Add(c.timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}

	_ = conn.SetDeadline(deadline)

	return conn, nil
}

func (c *RESPClient) authenticate(conn net.Conn, reader *bufio.Reader) error {
	if c.password == "" {
		return nil
	}

	auth := []string{"AUTH", c.password}
	if c.username != "" {
		auth = []string{"AUTH", c.username, c.password}
	}

	err := writeRESP(conn, auth)
	if err != nil {
		return fmt.Errorf("write Redis authentication: %w", err)
	}

	_, err = readRESP(reader)
	if err != nil {
		return fmt.Errorf("authenticate Redis: %w", err)
	}

	return nil
}

func (c *RESPClient) selectDatabase(conn net.Conn, reader *bufio.Reader) error {
	if c.database == 0 {
		return nil
	}

	err := writeRESP(conn, []string{"SELECT", strconv.Itoa(c.database)})
	if err != nil {
		return fmt.Errorf("write Redis database selection: %w", err)
	}

	_, err = readRESP(reader)
	if err != nil {
		return fmt.Errorf("select Redis database: %w", err)
	}

	return nil
}

func writeRESP(writer io.Writer, args []string) error {
	_, err := fmt.Fprintf(writer, "*%d\r\n", len(args))
	if err != nil {
		return fmt.Errorf("write RESP array header: %w", err)
	}

	for _, arg := range args {
		_, err = fmt.Fprintf(writer, "$%d\r\n%s\r\n", len(arg), arg)
		if err != nil {
			return fmt.Errorf("write RESP bulk string: %w", err)
		}
	}

	return nil
}

func readRESP(reader *bufio.Reader) (any, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read RESP prefix: %w", err)
	}

	switch prefix {
	case '+':
		return readRESPLine(reader)
	case '-':
		line, lineErr := readRESPLine(reader)
		if lineErr != nil {
			return nil, lineErr
		}

		return nil, fmt.Errorf("%w: %s", errRedisResponse, line)
	case ':':
		return readRESPInteger(reader)
	case '$':
		return readRESPBulk(reader)
	case '*':
		return readRESPArray(reader)
	default:
		return nil, fmt.Errorf("%w: %q", errRedisResponsePrefix, prefix)
	}
}

func readRESPLine(reader *bufio.Reader) (string, error) {
	raw, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read RESP line: %w", err)
	}

	return strings.TrimSuffix(strings.TrimSuffix(raw, "\n"), "\r"), nil
}

func readRESPInteger(reader *bufio.Reader) (int64, error) {
	line, err := readRESPLine(reader)
	if err != nil {
		return 0, err
	}

	value, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse RESP integer: %w", err)
	}

	return value, nil
}

func readRESPLength(reader *bufio.Reader) (int, error) {
	line, err := readRESPLine(reader)
	if err != nil {
		return 0, err
	}

	length, err := strconv.Atoi(line)
	if err != nil {
		return 0, fmt.Errorf("parse RESP length: %w", err)
	}

	return length, nil
}

func readRESPBulk(reader *bufio.Reader) (any, error) {
	length, err := readRESPLength(reader)
	if err != nil {
		return nil, err
	}

	if length == -1 {
		return nil, nil
	}

	value := make([]byte, length+respTerminatorBytes)

	_, err = io.ReadFull(reader, value)
	if err != nil {
		return nil, fmt.Errorf("read RESP bulk string: %w", err)
	}

	return value[:length], nil
}

func readRESPArray(reader *bufio.Reader) (any, error) {
	length, err := readRESPLength(reader)
	if err != nil {
		return nil, err
	}

	if length == -1 {
		return nil, nil
	}

	values := make([]any, length)
	for i := range values {
		values[i], err = readRESP(reader)
		if err != nil {
			return nil, err
		}
	}

	return values, nil
}

package egress

import (
	"net/url"
	"strconv"
	"strings"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

type target struct {
	host string
	port uint32
}

func parseTarget(start *strawpb.RequestStart) (target, *executionError) {
	if start == nil {
		return target{}, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	parsed, failure := parseRequestURL(start.GetUrl())
	if failure != nil {
		return target{}, failure
	}

	port, failure := targetPort(parsed)
	if failure != nil {
		return target{}, failure
	}

	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")

	return target{host: host, port: port}, nil
}

func parseRequestURL(raw string) (*url.URL, *executionError) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	return parsed, nil
}

func targetPort(parsed *url.URL) (uint32, *executionError) {
	port := parsed.Port()

	if port == "" {
		port = defaultPort(parsed.Scheme)
	}

	if port == "" {
		return 0, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	n, err := strconv.ParseUint(port, 10, 32)
	if err != nil || n == 0 || n > 65535 {
		return 0, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	return uint32(n), nil
}

func defaultPort(scheme string) string {
	switch scheme {
	case "http":
		return defaultHTTPPort
	case "https":
		return defaultHTTPSPort
	default:
		return ""
	}
}

func schemeFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return parsed.Scheme
}

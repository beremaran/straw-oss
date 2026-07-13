package egress

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sort"
	"strings"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const headerContentLength = "content-length"

func validateStart(start *strawpb.RequestStart) *executionError {
	err := start.Validate()
	if err != nil {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetMode() != strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetRedirectPolicy() != strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetDestinationPolicy().GetResolutionMode() != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED, unsupportedModeFact)
	}

	if strings.EqualFold(start.GetMethod(), http.MethodConnect) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	return nil
}

func validateTunnelStart(start *strawpb.RequestStart) *executionError {
	err := start.Validate()
	if err != nil {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetMode() != strawpb.RequestMode_REQUEST_MODE_RAW_TUNNEL {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if !strings.EqualFold(start.GetMethod(), http.MethodConnect) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetRedirectPolicy() != strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetDestinationPolicy().GetResolutionMode() != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED, unsupportedModeFact)
	}

	if start.GetFingerprintInstruction() != "" {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT, unsupportedFingerprint)
	}

	if len(start.GetInjectionOperations()) != 0 || len(start.GetHeaders()) != 0 {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	return nil
}

func buildHTTPRequest(ctx context.Context, start *strawpb.RequestStart, body []byte) (*http.Request, *executionError) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, start.GetMethod(), start.GetUrl(), reader)
	if err != nil {
		return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	for _, h := range start.GetHeaders() {
		if !safeOutboundHeader(h.GetName(), h.GetValue()) {
			return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
		}

		req.Header.Add(h.GetName(), string(h.GetValue()))
	}

	failure := applyInjection(req.Header, start.GetInjectionOperations())
	if failure != nil {
		return nil, failure
	}

	return req, nil
}

func applyInjection(headers http.Header, ops []*strawpb.InjectionOperation) *executionError {
	seenSet := map[string]struct{}{}

	for _, op := range ops {
		name := op.GetHeaderName()
		if !safeOutboundHeader(name, op.GetValue()) {
			return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
		}

		key := strings.ToLower(name)

		switch op.GetOp() {
		case opSet:
			if _, ok := seenSet[key]; ok {
				return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
			}

			seenSet[key] = struct{}{}

			headers.Set(name, string(op.GetValue()))
		case opAppend:
			headers.Add(name, string(op.GetValue()))
		case opRemove:
			headers.Del(name)
		default:
			return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
		}
	}

	return nil
}

func safeOutboundHeader(name string, value []byte) bool {
	if !validHTTPToken(name) {
		return false
	}

	if deniedHeader(name) {
		return false
	}

	return !bytes.ContainsAny(value, "\r\n")
}

func deniedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", headerContentLength, "transfer-encoding", "connection", "proxy-authorization":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(name), "x-straw-")
	}
}

func validHTTPToken(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		idx := int(r)
		if idx > 255 || !httpTokenAllowed[idx] {
			return false
		}
	}

	return true
}

var httpTokenAllowed = func() [256]bool {
	var allowed [256]bool

	for _, r := range "!#$%&'*+-.^_`|~" {
		allowed[byte(r)] = true
	}

	for r := byte('0'); r <= '9'; r++ {
		allowed[r] = true
	}

	for r := byte('A'); r <= 'Z'; r++ {
		allowed[r] = true
	}

	for r := byte('a'); r <= 'z'; r++ {
		allowed[r] = true
	}

	return allowed
}()

func responseHeaders(headers http.Header) []*strawpb.Header {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}

	sort.Strings(names)

	out := make([]*strawpb.Header, 0, len(headers))
	for _, name := range names {
		for _, value := range headers.Values(name) {
			out = append(out, &strawpb.Header{Name: name, Value: []byte(value)})
		}
	}

	return out
}

package http

const (
	// AcceptHeader is the HTTP Accept header name.
	AcceptHeader = "Accept"
	// AcceptLanguageHeader is the HTTP Accept-Language header name.
	AcceptLanguageHeader = "Accept-Language"
	// ContentTypeHeader is the HTTP Content-Type header name.
	ContentTypeHeader = "Content-Type"
	// ContentEncoding is the HTTP Content-Encoding header name.
	ContentEncoding = "Content-Encoding"
	// AcceptAll is the default Accept header value for browser-like requests.
	AcceptAll = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"

	// HeaderValueTextPlain is the text/plain content type value.
	HeaderValueTextPlain = "text/plain"
	// HeaderValueApplicationJSON is the application/json content type value.
	HeaderValueApplicationJSON = "application/json"
	// HeaderValueXCustom is a custom header name used in tests.
	HeaderValueXCustom = "X-Custom"
	// HeaderValueValue1 is a test header value.
	HeaderValueValue1 = "value1"

	// StatusCodeRetryable429 is the HTTP 429 Too Many Requests status code.
	StatusCodeRetryable429 = 429
	// StatusCodeRetryable500 is the HTTP 500 Internal Server Error status code.
	StatusCodeRetryable500 = 500
	// StatusCodeRetryable502 is the HTTP 502 Bad Gateway status code.
	StatusCodeRetryable502 = 502
	// StatusCodeRetryable503 is the HTTP 503 Service Unavailable status code.
	StatusCodeRetryable503 = 503
	// StatusCodeRetryable504 is the HTTP 504 Gateway Timeout status code.
	StatusCodeRetryable504 = 504
	// StatusCodeEscalate403 is the HTTP 403 Forbidden status code.
	StatusCodeEscalate403 = 403
	// StatusCodeEscalate407 is the HTTP 407 Proxy Authentication Required status code.
	StatusCodeEscalate407 = 407
)

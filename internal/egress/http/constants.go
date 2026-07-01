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
)

package dto

// PaginatedResponse is a generic paginated API response.
type PaginatedResponse[T any] struct {
	Data  []T `json:"data"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// ErrorResponse represents an error response from the API.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details any    `json:"details,omitempty"`
}

// HeaderDTO represents a key-value header pair.
type HeaderDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

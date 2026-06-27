package dto

import "time"

type CreateFingerprintRequest struct {
	ID string `json:"id" validate:"required"`

	Name string `json:"name" validate:"required"`

	Config map[string]interface{} `json:"config"`
}

type FingerprintResponse struct {
	ID string `json:"id"`

	Name string `json:"name"`

	Config map[string]interface{} `json:"config"`

	CreatedAt time.Time `json:"created_at"`

	UpdatedAt time.Time `json:"updated_at"`
}

type ListFingerprintsResponse []FingerprintResponse

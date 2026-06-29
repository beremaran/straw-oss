package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrCostMultiplierNotFound is returned when a cost multiplier does not exist.
	ErrCostMultiplierNotFound = errors.New("cost multiplier not found")
	// ErrCostMultiplierVersionConflict is returned when optimistic locking rejects an update.
	ErrCostMultiplierVersionConflict = errors.New("cost multiplier version conflict")
	// ErrCostMultiplierNegative is returned when a multiplier is below zero.
	ErrCostMultiplierNegative = errors.New("multiplier must be greater than or equal to 0")
)

// CostMultiplier configures cost units for endpoints matching a tag.
type CostMultiplier struct {
	ID          string
	EndpointTag string
	Multiplier  float64
	Description string
	IsActive    bool
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NormalizeCostMultiplierTag validates and canonicalizes an endpoint tag.
func NormalizeCostMultiplierTag(endpointTag string) (string, error) {
	tag, err := ParseTag(endpointTag)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint_tag: %w", err)
	}

	return tag.String(), nil
}

// ValidateCostMultiplier checks fields that must be valid before persistence.
func ValidateCostMultiplier(multiplier *CostMultiplier) error {
	endpointTag, err := NormalizeCostMultiplierTag(multiplier.EndpointTag)
	if err != nil {
		return err
	}

	if multiplier.Multiplier < 0 {
		return ErrCostMultiplierNegative
	}

	multiplier.EndpointTag = endpointTag
	multiplier.Description = strings.TrimSpace(multiplier.Description)

	return nil
}

// CostMultiplierRepository provides persistence operations for cost multipliers.
type CostMultiplierRepository interface {
	List(ctx context.Context, limit, offset int) ([]CostMultiplier, int, error)
	ListActive(ctx context.Context) ([]CostMultiplier, error)
	GetByID(ctx context.Context, id string) (*CostMultiplier, error)
	Create(ctx context.Context, multiplier *CostMultiplier) error
	Update(ctx context.Context, multiplier *CostMultiplier) error
	Deactivate(ctx context.Context, id string) (*CostMultiplier, error)
}

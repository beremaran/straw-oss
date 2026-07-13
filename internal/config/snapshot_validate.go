package config

import (
	"errors"
	"fmt"
)

// ErrInvalidSnapshot indicates that runtime configuration cannot be activated.
var ErrInvalidSnapshot = errors.New("invalid runtime configuration")

// ValidateSnapshot rejects a runtime configuration before it can become active.
func ValidateSnapshot(s Snapshot) error {
	if s.ConfigVersion == 0 {
		return fmt.Errorf("%w: config_version must be positive", ErrInvalidSnapshot)
	}

	if s.DefaultTimeoutMs == 0 || s.MaxTimeoutMs == 0 || s.DefaultTimeoutMs > s.MaxTimeoutMs {
		return fmt.Errorf("%w: timeouts must be positive and default_timeout_ms must not exceed max_timeout_ms", ErrInvalidSnapshot)
	}

	poolIDs, err := validatePools(s.ExecutorPools)
	if err != nil {
		return err
	}

	err = validateRoutes(s.RoutingRules, poolIDs)
	if err != nil {
		return err
	}

	return validateWorkerSettings(s.WorkerSettings)
}

func validatePools(pools []ExecutorPool) (map[string]struct{}, error) {
	ids := make(map[string]struct{}, len(pools))
	for _, pool := range pools {
		if pool.ID == "" || pool.ExecutorType == "" {
			return nil, fmt.Errorf("%w: pool id and executor_type are required", ErrInvalidSnapshot)
		}

		if _, exists := ids[pool.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate pool %q", ErrInvalidSnapshot, pool.ID)
		}

		ids[pool.ID] = struct{}{}
	}

	return ids, nil
}

func validateRoutes(routes []RoutingRule, pools map[string]struct{}) error {
	ids := make(map[string]struct{}, len(routes))
	for _, rule := range routes {
		if rule.ID == "" {
			return fmt.Errorf("%w: route id is required", ErrInvalidSnapshot)
		}

		if _, exists := ids[rule.ID]; exists {
			return fmt.Errorf("%w: duplicate route %q", ErrInvalidSnapshot, rule.ID)
		}

		ids[rule.ID] = struct{}{}
		if _, exists := pools[rule.TargetPoolID]; !exists {
			return fmt.Errorf("%w: route %q targets unknown pool %q", ErrInvalidSnapshot, rule.ID, rule.TargetPoolID)
		}
	}

	return nil
}

func validateWorkerSettings(settings []WorkerSetting) error {
	ids := make(map[string]struct{}, len(settings))
	for _, worker := range settings {
		if worker.WorkerID == "" {
			return fmt.Errorf("%w: worker_id is required", ErrInvalidSnapshot)
		}

		if _, exists := ids[worker.WorkerID]; exists {
			return fmt.Errorf("%w: duplicate worker setting %q", ErrInvalidSnapshot, worker.WorkerID)
		}

		ids[worker.WorkerID] = struct{}{}
	}

	return nil
}

package control

import (
	"fmt"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/fingerprint"
	"github.com/beremaran/straw-oss/internal/natsx"
)

// prepareRuntimeSnapshot is the single runtime-admin snapshot boundary. The
// config package validates and canonicalizes the data shape; Control then
// checks that the records name executable runtime capabilities.
func prepareRuntimeSnapshot(snapshot config.Snapshot) (config.Snapshot, error) {
	prepared, err := config.PrepareSnapshot(snapshot)
	if err != nil {
		return config.Snapshot{}, fmt.Errorf("prepare runtime snapshot: %w", err)
	}

	err = validateExecutableSnapshot(prepared)
	if err != nil {
		return config.Snapshot{}, fmt.Errorf("validate executable runtime snapshot: %w", err)
	}

	return prepared, nil
}

func validateExecutableSnapshot(snapshot config.Snapshot) error {
	err := validateExecutablePools(snapshot.ExecutorPools)
	if err != nil {
		return err
	}

	err = validateExecutableWorkerSettings(snapshot.WorkerSettings)
	if err != nil {
		return err
	}

	return validateExecutableProfiles(snapshot.FingerprintProfiles)
}

func validateExecutablePools(pools []config.ExecutorPool) error {
	for _, pool := range pools {
		if pool.ExecutorType != errorCategoryEgress {
			return fmt.Errorf("%w: pool %q uses unsupported executor_type %q", config.ErrInvalidSnapshot, pool.ID, pool.ExecutorType)
		}
	}

	return nil
}

func validateExecutableWorkerSettings(settings []config.WorkerSetting) error {
	for _, setting := range settings {
		if natsx.ValidateSubjectToken(setting.WorkerID) != nil {
			return fmt.Errorf("%w: worker setting %q has an invalid worker_id", config.ErrInvalidSnapshot, setting.WorkerID)
		}
	}

	return nil
}

func validateExecutableProfiles(profiles []config.FingerprintProfile) error {
	for _, profile := range profiles {
		if profile.Name == defaultFingerprintProfileName {
			continue
		}

		if !fingerprint.Contains(profile.Name) {
			return fmt.Errorf("%w: fingerprint profile %q is not executable by the official worker", config.ErrInvalidSnapshot, profile.Name)
		}

		if profile.ExecutorType != errorCategoryEgress || profile.ProfileRef != profile.Name || profile.ContractRevision != fingerprint.ContractRevision {
			return fmt.Errorf("%w: fingerprint profile %q has non-executable metadata", config.ErrInvalidSnapshot, profile.Name)
		}
	}

	return nil
}

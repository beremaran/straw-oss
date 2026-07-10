package control

import (
	"slices"
	"testing"
	"time"
)

func TestRedisWorkerRuntimeRoundTripsImmutableFingerprintCapabilities(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewRedisWorkerRuntimeStore(client)
	entry := &workerEntry{
		current: &runtimeSession{
			sessionID:                    "sess_current",
			supportedFingerprintProfiles: []string{workerRegTestChrome120},
			registeredAt:                 time.Unix(1_700_000_000, 0),
		},
		superseded: &runtimeSession{
			sessionID:                    "sess_superseded",
			supportedFingerprintProfiles: []string{workerRegTestChrome120},
			registeredAt:                 time.Unix(1_699_999_999, 0),
		},
	}

	err := store.save(workerRegTestWorker1, entry, time.Minute)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	entry.current.supportedFingerprintProfiles[0] = "mutated_after_save"
	entry.superseded.supportedFingerprintProfiles[0] = "mutated_after_save"

	loaded, err := store.loadAll()
	if err != nil {
		t.Fatalf("loadAll: %v", err)
	}
	got := loaded[workerRegTestWorker1]
	if !slices.Equal(got.current.supportedFingerprintProfiles, []string{workerRegTestChrome120}) {
		t.Fatalf("current profiles = %v, want [chrome_120]", got.current.supportedFingerprintProfiles)
	}
	if !slices.Equal(got.superseded.supportedFingerprintProfiles, []string{workerRegTestChrome120}) {
		t.Fatalf("superseded profiles = %v, want [chrome_120]", got.superseded.supportedFingerprintProfiles)
	}

	got.current.supportedFingerprintProfiles[0] = "mutated_after_load"
	reloaded, err := store.loadAll()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if profiles := reloaded[workerRegTestWorker1].current.supportedFingerprintProfiles; !slices.Equal(profiles, []string{workerRegTestChrome120}) {
		t.Fatalf("reloaded current profiles = %v, want immutable [chrome_120]", profiles)
	}
}

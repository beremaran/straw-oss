package config

import (
	"encoding/json"
	"testing"
)

func FuzzConfigAndSnapshotJSON(f *testing.F) {
	f.Add([]byte(`{"config_version":"v1","control":{}}`))
	f.Add([]byte(`{"config_version":1,"default_timeout_ms":60000,"max_timeout_ms":300000}`))
	f.Add([]byte(`not-json`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > maxConfigFileBytes {
			t.Skip()
		}
		_, _ = decodeFile(raw)
		var snapshot Snapshot
		_ = json.Unmarshal(raw, &snapshot)
		_ = snapshot.Clone()
	})
}

package receipt

import (
	"bytes"
	"testing"
)

func FuzzDecodeReceiptRecord(f *testing.F) {
	f.Add([]byte(`{"receipt_id":"rcpt_00000000000000000000000000000000","state":"verified","size_bytes":1}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<20 {
			t.Skip()
		}
		record, err := decodeRecord(bytes.NewReader(raw))
		if err == nil {
			_ = canTransition(record.State, StateExpired)
		}
	})
}

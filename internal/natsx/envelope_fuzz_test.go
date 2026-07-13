package natsx

import "testing"

func FuzzUnmarshalEnvelope(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x0a, 0x00})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<20 {
			t.Skip()
		}
		envelope, err := UnmarshalEnvelope(raw)
		if err != nil {
			return
		}
		_, _ = MarshalEnvelope(envelope)
	})
}

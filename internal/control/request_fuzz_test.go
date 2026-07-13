package control

import "testing"

func FuzzValidateRequest(f *testing.F) {
	f.Add([]byte(`{"method":"GET","url":"https://example.com"}`))
	f.Add([]byte(`{"method":"POST","url":"http://[::1]/","headers":[]}`))
	f.Add([]byte(`not-json`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<20 {
			t.Skip()
		}
		_, _ = ValidateRequest(raw, 1<<20, 60_000)
	})
}

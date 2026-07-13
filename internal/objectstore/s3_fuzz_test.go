package objectstore

import (
	"bytes"
	"testing"
)

func FuzzDecodeS3List(f *testing.F) {
	f.Add([]byte(`<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>a</Key><Size>1</Size></Contents></ListBucketResult>`))
	f.Add([]byte(`not-xml`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 4<<20 {
			t.Skip()
		}
		_, _ = decodeS3List(bytes.NewReader(raw))
	})
}

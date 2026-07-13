package natsx

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

func FuzzStreamFrameSequences(f *testing.F) {
	f.Add([]byte{}, uint64(1024))
	f.Add([]byte{0x08, 0x01}, uint64(0))
	f.Fuzz(func(t *testing.T, raw []byte, credit uint64) {
		if len(raw) > 1<<20 {
			t.Skip()
		}
		frame := &strawpb.StreamFrame{}
		if proto.Unmarshal(raw, frame) != nil {
			return
		}
		validator := NewStreamValidatorWithOptions(StreamValidatorOptions{Attempt: frame.GetAttempt(), InitialCredit: credit, IdleTimeout: time.Second})
		_ = validator.Accept(frame)
		_ = validator.Accept(frame)
	})
}

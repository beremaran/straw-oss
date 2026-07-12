package natsx

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

// ErrNilEnvelope is returned when a nil envelope is passed to MarshalEnvelope.
var ErrNilEnvelope = errors.New("marshal envelope: nil")

// MarshalEnvelope encodes a Straw protobuf Envelope for NATS transport.
func MarshalEnvelope(env *strawpb.Envelope) ([]byte, error) {
	if env == nil {
		return nil, ErrNilEnvelope
	}

	raw, err := proto.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}

	return raw, nil
}

// UnmarshalEnvelope decodes a Straw protobuf Envelope from NATS transport.
func UnmarshalEnvelope(raw []byte) (*strawpb.Envelope, error) {
	env := &strawpb.Envelope{}

	err := proto.Unmarshal(raw, env)
	if err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}

	return env, nil
}

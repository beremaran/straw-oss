package orchestrator

import (
	"sync"

	"github.com/beremaran/straw/pkg/protocol"
)

const resultHeaderCapacity = 10

var resultMessagePool = sync.Pool{
	New: func() any {
		return &ResultMessage{
			Headers: make(protocol.HeaderMap, 0, resultHeaderCapacity),
		}
	},
}

// AcquireResultMessage gets a reusable ResultMessage from the pool.
func AcquireResultMessage() *ResultMessage {
	return resultMessagePool.Get().(*ResultMessage)
}

// ReleaseResultMessage returns a ResultMessage to the pool after resetting it.
func ReleaseResultMessage(msg *ResultMessage) {
	if msg == nil {
		return
	}

	msg.Reset()
	resultMessagePool.Put(msg)
}

// Reset clears all fields of a ResultMessage for reuse.
func (r *ResultMessage) Reset() {
	r.RequestID = ""
	r.EndpointID = ""
	r.SessionID = ""
	r.StatusCode = 0
	r.Headers = r.Headers[:0]
	r.CompressedBody = nil
	r.BodyCompressed = false
	r.Error = nil
	r.Timing = nil
}

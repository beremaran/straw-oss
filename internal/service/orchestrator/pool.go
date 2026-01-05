package orchestrator

import (
	"sync"

	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

var (
	resultMessagePool = sync.Pool{
		New: func() any {
			return &ResultMessage{
				Headers: make(protocol.HeaderMap, 0, 10),
			}
		},
	}
)

// AcquireResultMessage retrieves a ResultMessage from the pool.
func AcquireResultMessage() *ResultMessage {
	return resultMessagePool.Get().(*ResultMessage)
}

// ReleaseResultMessage resets and returns a ResultMessage to the pool.
func ReleaseResultMessage(msg *ResultMessage) {
	if msg == nil {
		return
	}
	msg.Reset()
	resultMessagePool.Put(msg)
}

// Reset clears the ResultMessage for reuse.
func (r *ResultMessage) Reset() {
	r.RequestID = ""
	r.EndpointID = ""
	r.SessionID = ""
	r.StatusCode = 0
	r.Headers = r.Headers[:0]
	r.CompressedBody = nil // Allow GC of body
	r.BodyCompressed = false
	r.Error = nil
	r.Timing = nil
}

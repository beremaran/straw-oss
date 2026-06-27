package orchestrator

import (
	"sync"

	"github.com/beremaran/straw/pkg/protocol"
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

func AcquireResultMessage() *ResultMessage {
	return resultMessagePool.Get().(*ResultMessage)
}

func ReleaseResultMessage(msg *ResultMessage) {
	if msg == nil {
		return
	}
	msg.Reset()
	resultMessagePool.Put(msg)
}

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

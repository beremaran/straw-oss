package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	// Reset registry for test (though tough with sync.Once, we can just check it returns non-nil)
	reg := Init()
	assert.NotNil(t, reg)

	reg2 := GetRegistry()
	assert.Equal(t, reg, reg2)
}

func TestHandler(t *testing.T) {
	h := Handler()
	assert.NotNil(t, h)
}

// Note: Testing actual metrics output is better done with integration tests or by
// inspecting the registry gathering, but standard collectors init is hard to test
// repeatedly due to global state in some libraries. Init() uses its own registry
// so it should be fine.

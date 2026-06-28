package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	reg := Init()
	assert.NotNil(t, reg)

	reg2 := GetRegistry()
	assert.Equal(t, reg, reg2)
}

func TestHandler(t *testing.T) {
	h := Handler()
	assert.NotNil(t, h)
}

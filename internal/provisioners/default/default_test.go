package _default

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/score-spec/score-k8s/internal/provisioners/loader"
)

func TestDefaultProvisioners(t *testing.T) {
	p, err := loader.LoadProvisioners([]byte(DefaultProvisioners))
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Greater(t, len(p), 0)
}

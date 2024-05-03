package project

import (
	"github.com/score-spec/score-go/framework"
)

type StateExtras struct {
	// Don't actually persist these manifests, we just hold them here so we can pass them around.
	Manifests []map[string]interface{} `yaml:"-"`
}

type State = framework.State[StateExtras, framework.NoExtras]

package project

import (
	"github.com/score-spec/score-go/framework"
)

type StateExtras struct {
	Manifests []map[string]interface{} `yaml:"manifests"`
}

type State = framework.State[StateExtras, framework.NoExtras]

// Copyright 2024 The Score Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package templateprov

import (
	"context"
	"testing"

	"github.com/score-spec/score-go/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/score-spec/score-k8s/internal/provisioners"
)

func TestProvision(t *testing.T) {
	resUid := framework.NewResourceUid("w", "r", "thing", nil, nil)
	p, err := Parse(map[string]interface{}{
		"uri":              "template://example",
		"type":             resUid.Type(),
		"description":      "desc",
		"expected_outputs": []string{"b", "c"},
		"supported_params": []string{"ptest"},
		"init": `
a: {{ .Uid }}
b: {{ .Type }}
`,
		"state": `
a: {{ .Init.a }}
b: stuff
`,
		"shared": `
c: 1
`,
		"outputs": `
b: {{ .State.b | upper }}
c: {{ .Shared.c }}
`,
		"manifests": `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: thing
  data:
    key: value
`,
	})
	require.NoError(t, err)
	out, err := p.Provision(context.Background(), &provisioners.Input{
		ResourceUid:      string(resUid),
		ResourceType:     resUid.Type(),
		ResourceClass:    resUid.Class(),
		ResourceId:       resUid.Id(),
		ResourceParams:   map[string]interface{}{"pk": "pv"},
		ResourceMetadata: map[string]interface{}{"mk": "mv"},
		WorkloadMetadata: map[string]interface{}{"name": "w", "customField": "customValue"},
		ResourceState:    map[string]interface{}{"sk": "sv"},
		SharedState:      map[string]interface{}{"ssk": "ssv"},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, map[string]interface{}{
		"a": "thing.default#w.r",
		"b": "stuff",
	}, out.ResourceState)
	assert.Equal(t, map[string]interface{}{"c": 1}, out.SharedState)
	assert.Equal(t, map[string]interface{}{"b": "STUFF", "c": 1}, out.ResourceOutputs)
	assert.Len(t, out.Manifests, 1)
	assert.Equal(t, []string{"b", "c"}, p.Outputs())
	assert.Equal(t, []string{"ptest"}, p.Params())
	assert.Equal(t, "(any)", p.Class())
	assert.Equal(t, resUid.Type(), p.Type())
}

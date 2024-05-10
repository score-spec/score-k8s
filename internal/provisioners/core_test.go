// Copyright 2024 Humanitec
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

package provisioners

import (
	"testing"

	"github.com/score-spec/score-go/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/score-spec/score-k8s/internal/project"
)

func TestApplyToStateAndProject(t *testing.T) {
	resUid := framework.NewResourceUid("w", "r", "t", nil, nil)
	startState := &project.State{
		Resources: map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
			resUid: {},
		},
	}

	t.Run("set first provision with no outputs", func(t *testing.T) {
		output := &ProvisionOutput{}
		afterState, err := output.ApplyToStateAndProject(startState, resUid)
		require.NoError(t, err)
		assert.Equal(t, framework.ScoreResourceState[project.ResourceExtras]{
			State:   map[string]interface{}{},
			Outputs: map[string]interface{}{},
			Extras: project.ResourceExtras{
				Manifests: make([]map[string]interface{}, 0),
			},
		}, afterState.Resources[resUid])
	})

	t.Run("set first provision with some outputs", func(t *testing.T) {
		output := &ProvisionOutput{
			ResourceState:   map[string]interface{}{"a": "b", "c": nil},
			ResourceOutputs: map[string]interface{}{"x": "y"},
			SharedState:     map[string]interface{}{"i": "j", "k": nil},
			Manifests: []map[string]interface{}{
				{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "thing"},
					"data":       map[string]interface{}{"key": "value"},
				},
			},
		}
		afterState, err := output.ApplyToStateAndProject(startState, resUid)
		require.NoError(t, err)
		assert.Equal(t, framework.ScoreResourceState[project.ResourceExtras]{
			State:   map[string]interface{}{"a": "b", "c": nil},
			Outputs: map[string]interface{}{"x": "y"},
			Extras: project.ResourceExtras{
				Manifests: []map[string]interface{}{
					{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "thing"},
						"data":       map[string]interface{}{"key": "value"},
					},
				},
			},
		}, afterState.Resources[resUid])
		assert.Equal(t, map[string]interface{}{"i": "j"}, afterState.SharedState)
	})

}

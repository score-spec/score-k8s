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

package convert

import (
	"testing"

	scoretypes "github.com/score-spec/score-go/types"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/score-spec/score-k8s/internal"
)

func Test_buildResourceList_nominal(t *testing.T) {
	rl, err := buildResourceList(&scoretypes.ResourcesLimits{
		Cpu:    internal.Ref("100m"),
		Memory: internal.Ref("20Mi"),
	})
	assert.NoError(t, err)
	assert.Equal(t, coreV1.ResourceList{
		"cpu":    resource.MustParse("100m"),
		"memory": resource.MustParse("20Mi"),
	}, rl)
}

func Test_convertContainerResources_nominal(t *testing.T) {
	rl, err := convertContainerResources(&scoretypes.ContainerResources{
		Limits: &scoretypes.ResourcesLimits{
			Memory: internal.Ref("20Mi"),
		},
		Requests: &scoretypes.ResourcesLimits{
			Cpu: internal.Ref("100m"),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, coreV1.ResourceRequirements{
		Limits: map[coreV1.ResourceName]resource.Quantity{
			"memory": resource.MustParse("20Mi"),
		},
		Requests: map[coreV1.ResourceName]resource.Quantity{
			"cpu": resource.MustParse("100m"),
		},
	}, rl)
}

func Test_convertContainerResources_partial(t *testing.T) {
	rl, err := convertContainerResources(&scoretypes.ContainerResources{
		Limits: &scoretypes.ResourcesLimits{
			Memory: internal.Ref("20Mi"),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, coreV1.ResourceRequirements{
		Limits: map[coreV1.ResourceName]resource.Quantity{
			"memory": resource.MustParse("20Mi"),
		},
	}, rl)
}

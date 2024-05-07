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

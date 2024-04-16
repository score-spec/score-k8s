package convert

import (
	"maps"
	"os"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/score-spec/score-k8s/internal/project"
)

func ConvertResource(state *project.State, resId framework.ResourceUid) ([]v1.Object, *project.State, error) {
	res := state.Resources[resId]
	resOutputs, err := state.GetResourceOutputForWorkload(res.SourceWorkload)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to generate outputs")
	}
	sf := framework.BuildSubstitutionFunction(state.Workloads[res.SourceWorkload].Spec.Metadata, resOutputs)

	newState := *state
	newState.Resources = maps.Clone(state.Resources)

	_, err = framework.Substitute(res.Params, sf)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to substitute params")
	}

	if res.Type == "environment" && res.Class == "default" {
		res.OutputLookupFunc = func(keys ...string) (interface{}, error) {
			if len(keys) != 1 {
				return nil, errors.New("expected only 1 key for an environment resource")
			}
			return os.Getenv(keys[0]), nil
		}
	}

	return nil, nil, errors.New("unsupported resource")
}

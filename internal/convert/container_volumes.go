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

package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	coreV1 "k8s.io/api/core/v1"
	machineryMeta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/score-spec/score-k8s/internal"
	"github.com/score-spec/score-k8s/internal/project"
)

func convertContainerVolume(
	index int, volume scoretypes.ContainerVolumesElem,
	resources map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras],
	substitutionFunc func(string) (string, error),
) (coreV1.VolumeMount, *coreV1.Volume, *coreV1.PersistentVolumeClaim, error) {
	volName := fmt.Sprintf("vol-%d", index)
	mount := coreV1.VolumeMount{
		Name:      volName,
		MountPath: volume.Target,
		SubPath:   internal.DerefOr(volume.Path, ""),
		ReadOnly:  internal.DerefOr(volume.ReadOnly, false),
	}

	resolvedVolumeSource, err := framework.SubstituteString(volume.Source, substitutionFunc)
	if err != nil {
		return mount, nil, nil, errors.Wrap(err, "source: failed to resolve placeholder")
	}

	res, ok := resources[framework.ResourceUid(resolvedVolumeSource)]
	if !ok {
		return mount, nil, nil, errors.Errorf("source: resource '%s' does not exist", resolvedVolumeSource)
	}

	// convert the outputs into a spec
	raw, _ := json.Marshal(res.Outputs)
	var anon struct {
		Source    *coreV1.VolumeSource              `json:"source"`
		ClaimSpec *coreV1.PersistentVolumeClaimSpec `json:"claimSpec"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&anon); err != nil {
		return mount, nil, nil, errors.Wrapf(err, "failed to convert resource '%s' outputs into a Kubernetes volume", resolvedVolumeSource)
	}
	if (anon.ClaimSpec == nil) == (anon.Source == nil) {
		return mount, nil, nil, errors.Errorf("failed to convert resource '%s' outputs into volume: either 'source' or 'claimSpec' required", resolvedVolumeSource)
	} else if anon.ClaimSpec != nil {
		if anon.ClaimSpec.Size() == 0 {
			return mount, nil, nil, errors.Errorf("failed to convert resource '%s' outputs into volume: claimSpec is empty", resolvedVolumeSource)
		}
		return mount, nil, &coreV1.PersistentVolumeClaim{
			ObjectMeta: machineryMeta.ObjectMeta{
				Name: volName,
			},
			Spec: *anon.ClaimSpec,
		}, nil
	}
	if anon.Source.Size() == 0 {
		return mount, nil, nil, errors.Errorf("failed to convert resource '%s' outputs into volume: source is empty", resolvedVolumeSource)
	}
	return mount, &coreV1.Volume{
		Name:         volName,
		VolumeSource: *anon.Source,
	}, nil, nil
}

type volumeAndMount struct {
	Volume      coreV1.Volume
	VolumeMount coreV1.VolumeMount
}

// collapseVolumeMounts needs to allow multiple config maps and secrets being mounted into the same directory.
// See https://kubernetes.io/docs/concepts/storage/projected-volumes/.
// So the general idea is:
// 1. find all the volumes mounted into the same directory
func collapseVolumeMounts(volumes []coreV1.Volume, mounts []coreV1.VolumeMount) ([]coreV1.Volume, []coreV1.VolumeMount, error) {
	outputMounts := make([]coreV1.VolumeMount, 0, len(mounts))
	outputVols := make([]coreV1.Volume, 0, len(volumes))

	// First phase we're going to filter out all the configmaps and secrets and group them up
	groups := make(map[string][]volumeAndMount)
	for _, mount := range mounts {
		volInd := slices.IndexFunc(volumes, func(volume coreV1.Volume) bool {
			return volume.Name == mount.Name
		})
		if volInd >= 0 {
			vol := volumes[volInd]
			if vol.ConfigMap != nil || vol.Secret != nil {
				group, ok := groups[mount.MountPath]
				if ok {
					groups[mount.MountPath] = append(group, volumeAndMount{
						Volume:      vol,
						VolumeMount: mount,
					})
				} else {
					groups[mount.MountPath] = []volumeAndMount{{
						Volume:      vol,
						VolumeMount: mount,
					}}
				}
			} else {
				outputMounts = append(outputMounts, mount)
				outputVols = append(outputVols, vol)
			}
		} else {
			outputMounts = append(outputMounts, mount)
		}
	}

	// Next phase we can add in all the groups again
	var projectedVolumeIndex int
	orderedMountPaths := make([]string, 0, len(groups))
	for s := range groups {
		orderedMountPaths = append(orderedMountPaths, s)
	}
	slices.Sort(orderedMountPaths)

	for _, mountPath := range orderedMountPaths {
		group := groups[mountPath]

		// First we can add all the singleton groups back into the volume list.
		if len(group) == 1 {
			outputVols = append(outputVols, group[0].Volume)
			outputMounts = append(outputMounts, group[0].VolumeMount)
			continue
		}

		// And for any larger sets we can convert them into projected volumes.
		sources := make([]coreV1.VolumeProjection, 0, len(group))
		for _, groupEntry := range group {
			if groupEntry.Volume.ConfigMap != nil {
				sources = append(sources, coreV1.VolumeProjection{
					ConfigMap: &coreV1.ConfigMapProjection{
						LocalObjectReference: groupEntry.Volume.ConfigMap.LocalObjectReference,
						Items:                groupEntry.Volume.ConfigMap.Items,
					},
				})
			} else if groupEntry.Volume.Secret != nil {
				sources = append(sources, coreV1.VolumeProjection{
					Secret: &coreV1.SecretProjection{
						LocalObjectReference: coreV1.LocalObjectReference{Name: groupEntry.Volume.Secret.SecretName},
						Items:                groupEntry.Volume.Secret.Items,
					},
				})
			}
		}
		newVol := coreV1.Volume{
			Name: fmt.Sprintf("proj-vol-%d", projectedVolumeIndex),
			VolumeSource: coreV1.VolumeSource{
				Projected: &coreV1.ProjectedVolumeSource{
					Sources: sources,
				},
			},
		}
		newMount := coreV1.VolumeMount{
			Name:      newVol.Name,
			MountPath: mountPath,
			ReadOnly:  true,
		}

		outputVols = append(outputVols, newVol)
		outputMounts = append(outputMounts, newMount)
		projectedVolumeIndex += 1
	}

	return outputVols, outputMounts, nil
}

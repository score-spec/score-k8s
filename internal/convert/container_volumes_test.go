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
	"testing"

	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/score-spec/score-k8s/internal"
	"github.com/score-spec/score-k8s/internal/project"
)

func Test_convertContainerVolume_not_found(t *testing.T) {
	_, _, _, err := convertContainerVolume(0, scoretypes.ContainerVolumesElem{
		Source: "unknown",
	}, map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{}, noSubstitutesFunction)
	assert.EqualError(t, err, "source: resource 'unknown' does not exist")
}

func Test_convertContainerVolume_no_outputs(t *testing.T) {
	_, _, _, err := convertContainerVolume(0, scoretypes.ContainerVolumesElem{
		Source: "volume.default#my-workload.thing",
	}, map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
		"volume.default#my-workload.thing": {
			Outputs: map[string]interface{}{},
		},
	}, noSubstitutesFunction)
	assert.EqualError(t, err, "failed to convert resource 'volume.default#my-workload.thing' outputs into volume: either 'source' or 'claimSpec' required")
}

func Test_convertContainerVolume_empty_source(t *testing.T) {
	_, _, _, err := convertContainerVolume(0, scoretypes.ContainerVolumesElem{
		Source: "volume.default#my-workload.thing",
	}, map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
		"volume.default#my-workload.thing": {
			Outputs: map[string]interface{}{
				"source": map[string]interface{}{},
			},
		},
	}, noSubstitutesFunction)
	assert.EqualError(t, err, "failed to convert resource 'volume.default#my-workload.thing' outputs into volume: source is empty")
}

func Test_convertContainerVolume_bad_source(t *testing.T) {
	_, _, _, err := convertContainerVolume(0, scoretypes.ContainerVolumesElem{
		Source: "volume.default#my-workload.thing",
	}, map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
		"volume.default#my-workload.thing": {
			Outputs: map[string]interface{}{
				"source": map[string]interface{}{
					"emptyDir": map[string]interface{}{
						"fruit": "banana",
					},
				},
			},
		},
	}, noSubstitutesFunction)
	assert.EqualError(t, err, "failed to convert resource 'volume.default#my-workload.thing' outputs into a Kubernetes volume: json: unknown field \"fruit\"")
}

func Test_convertContainerVolume_bad_claim(t *testing.T) {
	_, _, _, err := convertContainerVolume(0, scoretypes.ContainerVolumesElem{
		Source: "volume.default#my-workload.thing",
	}, map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
		"volume.default#my-workload.thing": {
			Outputs: map[string]interface{}{
				"claimSpec": map[string]interface{}{
					"fruit": "banana",
				},
			},
		},
	}, noSubstitutesFunction)
	assert.EqualError(t, err, "failed to convert resource 'volume.default#my-workload.thing' outputs into a Kubernetes volume: json: unknown field \"fruit\"")
}

func Test_convertContainerVolume_nominal_source(t *testing.T) {
	mount, vol, claim, err := convertContainerVolume(0, scoretypes.ContainerVolumesElem{
		Target:   "/mount/path",
		Source:   "volume.default#my-workload.thing",
		ReadOnly: internal.Ref(true),
		Path:     internal.Ref("sub"),
	}, map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
		"volume.default#my-workload.thing": {
			Outputs: map[string]interface{}{
				"source": map[string]interface{}{
					"emptyDir": map[string]interface{}{
						"sizeLimit": "10Mi",
					},
				},
			},
		},
	}, noSubstitutesFunction)
	assert.Equal(t, coreV1.VolumeMount{
		Name:      "vol-0",
		ReadOnly:  true,
		MountPath: "/mount/path",
		SubPath:   "sub",
	}, mount)
	if assert.NotNil(t, vol) {
		assert.Equal(t, coreV1.Volume{
			Name: "vol-0",
			VolumeSource: coreV1.VolumeSource{
				EmptyDir: &coreV1.EmptyDirVolumeSource{
					SizeLimit: internal.Ref(resource.MustParse("10Mi")),
				},
			},
		}, *vol)
	}
	assert.Nil(t, claim)
	assert.NoError(t, err)
}

func Test_convertContainerVolume_nominal_claim(t *testing.T) {
	mount, vol, claim, err := convertContainerVolume(0, scoretypes.ContainerVolumesElem{
		Target:   "/mount/path",
		Source:   "volume.default#my-workload.thing",
		ReadOnly: internal.Ref(true),
		Path:     internal.Ref("sub"),
	}, map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
		"volume.default#my-workload.thing": {
			Outputs: map[string]interface{}{
				"claimSpec": map[string]interface{}{
					"storageClassName": "default",
				},
			},
		},
	}, noSubstitutesFunction)
	assert.Equal(t, coreV1.VolumeMount{
		Name:      "vol-0",
		ReadOnly:  true,
		MountPath: "/mount/path",
		SubPath:   "sub",
	}, mount)
	assert.Nil(t, vol)
	if assert.NotNil(t, claim) {
		assert.Equal(t, coreV1.PersistentVolumeClaim{
			ObjectMeta: v1.ObjectMeta{
				Name: "vol-0",
			},
			Spec: coreV1.PersistentVolumeClaimSpec{
				StorageClassName: internal.Ref("default"),
			},
		}, *claim)
	}
	assert.NoError(t, err)
}

func Test_collapseVolumeMounts_nominal(t *testing.T) {
	vols, mounts, err := collapseVolumeMounts(
		[]coreV1.Volume{
			{Name: "v1", VolumeSource: coreV1.VolumeSource{
				Secret: &coreV1.SecretVolumeSource{SecretName: "x", Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}}},
			},
			{Name: "v2", VolumeSource: coreV1.VolumeSource{
				Secret: &coreV1.SecretVolumeSource{SecretName: "y", Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}}},
			},
			{Name: "v3", VolumeSource: coreV1.VolumeSource{
				Secret: &coreV1.SecretVolumeSource{SecretName: "z", Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}}},
			},
			{Name: "v4", VolumeSource: coreV1.VolumeSource{
				ConfigMap: &coreV1.ConfigMapVolumeSource{LocalObjectReference: coreV1.LocalObjectReference{Name: "a"}, Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}},
			}},
			{Name: "v5", VolumeSource: coreV1.VolumeSource{
				ConfigMap: &coreV1.ConfigMapVolumeSource{LocalObjectReference: coreV1.LocalObjectReference{Name: "b"}, Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}},
			}},
		},
		[]coreV1.VolumeMount{
			{Name: "unknown", MountPath: "/thing"},
			{Name: "v1", MountPath: "/a"},
			{Name: "v2", MountPath: "/b"},
			{Name: "v3", MountPath: "/a"},
			{Name: "v4", MountPath: "/c"},
			{Name: "v5", MountPath: "/b"},
		},
	)
	assert.NoError(t, err)
	assert.Equal(t, []coreV1.Volume{
		{Name: "proj-vol-0", VolumeSource: coreV1.VolumeSource{
			Projected: &coreV1.ProjectedVolumeSource{
				Sources: []coreV1.VolumeProjection{
					{Secret: &coreV1.SecretProjection{LocalObjectReference: coreV1.LocalObjectReference{Name: "x"}, Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}}},
					{Secret: &coreV1.SecretProjection{LocalObjectReference: coreV1.LocalObjectReference{Name: "z"}, Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}}},
				},
			},
		}},
		{Name: "proj-vol-1", VolumeSource: coreV1.VolumeSource{
			Projected: &coreV1.ProjectedVolumeSource{
				Sources: []coreV1.VolumeProjection{
					{Secret: &coreV1.SecretProjection{LocalObjectReference: coreV1.LocalObjectReference{Name: "y"}, Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}}},
					{ConfigMap: &coreV1.ConfigMapProjection{LocalObjectReference: coreV1.LocalObjectReference{Name: "b"}, Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}}},
				},
			},
		}},
		{Name: "v4", VolumeSource: coreV1.VolumeSource{
			ConfigMap: &coreV1.ConfigMapVolumeSource{LocalObjectReference: coreV1.LocalObjectReference{Name: "a"}, Items: []coreV1.KeyToPath{{Key: "k", Path: "p"}}},
		}},
	}, vols)
	assert.Equal(t, []coreV1.VolumeMount{
		{Name: "unknown", MountPath: "/thing"},
		{Name: "proj-vol-0", MountPath: "/a", ReadOnly: true},
		{Name: "proj-vol-1", MountPath: "/b", ReadOnly: true},
		{Name: "v4", MountPath: "/c"},
	}, mounts)
}

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
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	machineryMeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/score-spec/score-k8s/internal"
	"github.com/score-spec/score-k8s/internal/project"
)

func TestMassive(t *testing.T) {
	var err error
	state := new(project.State)
	state, err = state.WithWorkload(&scoretypes.Workload{
		Metadata: map[string]interface{}{
			"name": "example",
			"annotations": map[string]interface{}{
				"my.custom.scope/annotation": "value",
			},
			"thing": "other",
		},
		Containers: map[string]scoretypes.Container{
			"c1": {
				Image:   "my-image",
				Command: []string{"do", "thing"},
				Args:    []string{"with", "${args}"},
				Variables: map[string]string{
					"VAR":  "RAW",
					"VAR2": "",
					"VAR3": "${metadata.name}",
					"VAR4": "${metadata.thing}",
					"VAR5": "${resources.foo.key}",
				},
				Files: map[string]scoretypes.ContainerFile{
					"/root.md": {
						Content: internal.Ref("my-content ${metadata.name}"),
					},
					"/binary": {
						BinaryContent: internal.Ref(base64.StdEncoding.EncodeToString([]byte("hello ${metadata.name} world"))),
					},
				},
				LivenessProbe: &scoretypes.ContainerProbe{Exec: &scoretypes.ExecProbe{
					Command: []string{"echo", "true"},
				}},
				ReadinessProbe: &scoretypes.ContainerProbe{HttpGet: &scoretypes.HttpProbe{
					Scheme: internal.Ref(scoretypes.HttpProbeSchemeHTTPS),
					Host:   internal.Ref("127.0.0.1"),
					Port:   3001,
				}},
				Resources: &scoretypes.ContainerResources{
					Requests: &scoretypes.ResourcesLimits{Cpu: internal.Ref("999m")},
					Limits:   &scoretypes.ResourcesLimits{Memory: internal.Ref("10Mi")},
				},
				Volumes: map[string]scoretypes.ContainerVolume{
					"/mount/thing": {
						Source: "${resources.vol}",
					},
				},
			},
			"c2": {
				Image: "other-image",
			},
		},
		Service: &scoretypes.WorkloadService{
			Ports: map[string]scoretypes.ServicePort{
				"web": {
					Port:       80,
					TargetPort: internal.Ref(8080),
					Protocol:   internal.Ref(scoretypes.ServicePortProtocolUDP),
				},
			},
		},
		Resources: map[string]scoretypes.Resource{
			"foo": {
				Type:  "thing",
				Class: internal.Ref("default"),
				Id:    internal.Ref("shared"),
			},
			"vol": {
				Type:  "vol",
				Class: internal.Ref("default"),
			},
		},
	}, nil, project.WorkloadExtras{InstanceSuffix: "-abcdef"})
	require.NoError(t, err)
	state.Resources = map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
		"thing.default#shared": {
			Type:  "thing",
			Class: "default",
			Id:    "shared",
			Outputs: map[string]interface{}{
				"key": "xxx",
			},
		},
		"vol.default#example.vol": {
			Type:  "vol",
			Class: "default",
			Id:    "",
			Outputs: map[string]interface{}{
				"source": map[string]interface{}{
					"emptyDir": map[string]interface{}{},
				},
			},
		},
	}
	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)
	out := new(bytes.Buffer)
	for _, manifest := range manifests {
		if assert.NoError(t, err) {
			assert.NoError(t, internal.YamlSerializerInfo.Serializer.Encode(manifest.(runtime.Object), out))
			out.WriteString("---\n")
		}
	}
	assert.Equal(t, `apiVersion: v1
binaryData:
  file: aGVsbG8gJHttZXRhZGF0YS5uYW1lfSB3b3JsZA==
kind: ConfigMap
metadata:
  name: example-c1-file-d0e9aff012
---
apiVersion: v1
binaryData:
  file: bXktY29udGVudCBleGFtcGxl
kind: ConfigMap
metadata:
  name: example-c1-file-7a1ae64977
---
apiVersion: v1
kind: Service
metadata:
  annotations:
    k8s.score.dev/workload-name: example
  labels:
    app.kubernetes.io/instance: example-abcdef
    app.kubernetes.io/managed-by: score-k8s
    app.kubernetes.io/name: example
  name: example
spec:
  ports:
  - name: web
    port: 80
    protocol: UDP
    targetPort: 8080
  selector:
    app.kubernetes.io/instance: example-abcdef
status:
  loadBalancer: {}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    k8s.score.dev/workload-name: example
  labels:
    app.kubernetes.io/instance: example-abcdef
    app.kubernetes.io/managed-by: score-k8s
    app.kubernetes.io/name: example
  name: example
spec:
  selector:
    matchLabels:
      app.kubernetes.io/instance: example-abcdef
  strategy: {}
  template:
    metadata:
      annotations:
        k8s.score.dev/workload-name: example
        my.custom.scope/annotation: value
      labels:
        app.kubernetes.io/instance: example-abcdef
        app.kubernetes.io/managed-by: score-k8s
        app.kubernetes.io/name: example
    spec:
      containers:
      - args:
        - with
        - ${args}
        command:
        - do
        - thing
        env:
        - name: VAR
          value: RAW
        - name: VAR2
        - name: VAR3
          value: example
        - name: VAR4
          value: other
        - name: VAR5
          value: xxx
        image: my-image
        livenessProbe:
          exec:
            command:
            - echo
            - "true"
        name: c1
        readinessProbe:
          httpGet:
            host: 127.0.0.1
            port: 3001
            scheme: HTTPS
        resources:
          limits:
            memory: 10Mi
          requests:
            cpu: 999m
        volumeMounts:
        - mountPath: /mount/thing
          name: vol-75c4252512
        - mountPath: /
          name: proj-vol-0
          readOnly: true
      - image: other-image
        name: c2
        resources: {}
      volumes:
      - emptyDir: {}
        name: vol-75c4252512
      - name: proj-vol-0
        projected:
          sources:
          - configMap:
              items:
              - key: file
                path: binary
              name: example-c1-file-d0e9aff012
          - configMap:
              items:
              - key: file
                path: root.md
              name: example-c1-file-7a1ae64977
status: {}
---
`, out.String())
}

// TestSharedVolumeAcrossContainers reproduces https://github.com/score-spec/score-k8s/issues/363:
// when two containers mount the same volume resource at the same path (e.g. an init container
// staging files into an emptyDir that the main container reads), the pod-level volume list must
// contain that volume only once, otherwise Kubernetes rejects the pod with a "Duplicate value" error.
func TestSharedVolumeAcrossContainers(t *testing.T) {
	state := new(project.State)
	state, err := state.WithWorkload(&scoretypes.Workload{
		Metadata: map[string]interface{}{"name": "example"},
		Containers: map[string]scoretypes.Container{
			"main": {
				Image: "main-image",
				Volumes: map[string]scoretypes.ContainerVolume{
					"/data": {Source: "${resources.vol}"},
				},
			},
			"init": {
				Image: "init-image",
				Volumes: map[string]scoretypes.ContainerVolume{
					"/data": {Source: "${resources.vol}"},
				},
			},
		},
		Resources: map[string]scoretypes.Resource{
			"vol": {Type: "vol", Class: internal.Ref("default")},
		},
	}, nil, project.WorkloadExtras{})
	require.NoError(t, err)
	state.Resources = map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
		"vol.default#example.vol": {
			Type:  "vol",
			Class: "default",
			Outputs: map[string]interface{}{
				"source": map[string]interface{}{"emptyDir": map[string]interface{}{}},
			},
		},
	}

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)

	var deployment *v1.Deployment
	for _, m := range manifests {
		if d, ok := m.(*v1.Deployment); ok {
			deployment = d
		}
	}
	require.NotNil(t, deployment)

	volumes := deployment.Spec.Template.Spec.Volumes
	require.Len(t, volumes, 1, "shared volume must be collapsed into a single pod-level volume")

	// Both containers must reference that single volume by name.
	names := make([]string, 0, len(volumes))
	for _, v := range volumes {
		names = append(names, v.Name)
	}
	for _, c := range deployment.Spec.Template.Spec.Containers {
		require.Len(t, c.VolumeMounts, 1)
		assert.Contains(t, names, c.VolumeMounts[0].Name)
	}
}

// buildVolumeWorkloadState builds a workload state with a `vol` resource per entry in resourceNames,
// each backed by an emptyDir, so the volume identity tests below can stay focused on the mounts.
func buildVolumeWorkloadState(t *testing.T, containers map[string]scoretypes.Container, resourceNames ...string) *project.State {
	t.Helper()
	resources := make(map[string]scoretypes.Resource, len(resourceNames))
	for _, name := range resourceNames {
		resources[name] = scoretypes.Resource{Type: "vol", Class: internal.Ref("default")}
	}
	state := new(project.State)
	state, err := state.WithWorkload(&scoretypes.Workload{
		Metadata:   map[string]interface{}{"name": "example"},
		Containers: containers,
		Resources:  resources,
	}, nil, project.WorkloadExtras{})
	require.NoError(t, err)

	state.Resources = map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{}
	for _, name := range resourceNames {
		state.Resources[framework.ResourceUid("vol.default#example."+name)] = framework.ScoreResourceState[project.ResourceExtras]{
			Type:  "vol",
			Class: "default",
			Outputs: map[string]interface{}{
				"source": map[string]interface{}{"emptyDir": map[string]interface{}{}},
			},
		}
	}
	return state
}

func deploymentFrom(t *testing.T, manifests []machineryMeta.Object) *v1.Deployment {
	t.Helper()
	for _, m := range manifests {
		if d, ok := m.(*v1.Deployment); ok {
			return d
		}
	}
	t.Fatal("no Deployment found in manifests")
	return nil
}

// TestSharedVolumeAcrossDifferentMountPaths covers the shared-emptydir-diff-mountpoint case from
// https://github.com/score-spec/score-k8s/issues/367: one volume resource mounted by two containers
// at two different paths is a single shared volume, not two independent ones. Volume names used to
// be derived from the mount path, so the two paths produced two separate emptyDirs.
func TestSharedVolumeAcrossDifferentMountPaths(t *testing.T) {
	state := buildVolumeWorkloadState(t, map[string]scoretypes.Container{
		"main": {
			Image:   "busybox",
			Volumes: map[string]scoretypes.ContainerVolume{"/one": {Source: "${resources.data}"}},
		},
		"sidecar": {
			Image:   "busybox",
			Volumes: map[string]scoretypes.ContainerVolume{"/two": {Source: "${resources.data}"}},
		},
	}, "data")

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)
	deployment := deploymentFrom(t, manifests)

	volumes := deployment.Spec.Template.Spec.Volumes
	require.Len(t, volumes, 1, "one source mounted at two paths must be a single pod-level volume")

	// Both containers mount that one volume, each keeping its own mount path.
	mountPaths := map[string]string{}
	for _, c := range deployment.Spec.Template.Spec.Containers {
		require.Len(t, c.VolumeMounts, 1)
		assert.Equal(t, volumes[0].Name, c.VolumeMounts[0].Name, "both containers must reference the shared volume")
		mountPaths[c.Name] = c.VolumeMounts[0].MountPath
	}
	assert.Equal(t, map[string]string{"main": "/one", "sidecar": "/two"}, mountPaths)
}

// TestDistinctVolumesAtSameMountPath covers the diff-emptydir-same-mountpoint case from the same
// issue: two different volume resources mounted at the same path in two containers are two distinct
// volumes. Deriving the name from the mount path silently collapsed them into one, so a container
// ended up reading a volume it never asked for.
func TestDistinctVolumesAtSameMountPath(t *testing.T) {
	state := buildVolumeWorkloadState(t, map[string]scoretypes.Container{
		"main": {
			Image:   "busybox",
			Volumes: map[string]scoretypes.ContainerVolume{"/data": {Source: "${resources.vol-01}"}},
		},
		"sidecar": {
			Image:   "busybox",
			Volumes: map[string]scoretypes.ContainerVolume{"/data": {Source: "${resources.vol-02}"}},
		},
	}, "vol-01", "vol-02")

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)
	deployment := deploymentFrom(t, manifests)

	require.Len(t, deployment.Spec.Template.Spec.Volumes, 2, "two different sources must stay two volumes")

	mounted := map[string]string{}
	for _, c := range deployment.Spec.Template.Spec.Containers {
		require.Len(t, c.VolumeMounts, 1)
		assert.Equal(t, "/data", c.VolumeMounts[0].MountPath)
		mounted[c.Name] = c.VolumeMounts[0].Name
	}
	assert.NotEqual(t, mounted["main"], mounted["sidecar"], "distinct sources must not share a volume name")

	names := []string{deployment.Spec.Template.Spec.Volumes[0].Name, deployment.Spec.Template.Spec.Volumes[1].Name}
	assert.Contains(t, names, mounted["main"])
	assert.Contains(t, names, mounted["sidecar"])
}

// TestSharedVolumeKeepsPerContainerMountOptions checks that collapsing to one pod-level volume does
// not flatten the mount options: subPath and readOnly belong to the mount, not to the volume.
func TestSharedVolumeKeepsPerContainerMountOptions(t *testing.T) {
	state := buildVolumeWorkloadState(t, map[string]scoretypes.Container{
		"writer": {
			Image: "busybox",
			Volumes: map[string]scoretypes.ContainerVolume{
				"/data": {Source: "${resources.data}", Path: internal.Ref("in")},
			},
		},
		"reader": {
			Image: "busybox",
			Volumes: map[string]scoretypes.ContainerVolume{
				"/data": {Source: "${resources.data}", Path: internal.Ref("out"), ReadOnly: internal.Ref(true)},
			},
		},
	}, "data")

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)
	deployment := deploymentFrom(t, manifests)

	require.Len(t, deployment.Spec.Template.Spec.Volumes, 1, "the same source must collapse to one volume")

	mounts := map[string]coreV1.VolumeMount{}
	for _, c := range deployment.Spec.Template.Spec.Containers {
		require.Len(t, c.VolumeMounts, 1)
		mounts[c.Name] = c.VolumeMounts[0]
	}
	assert.Equal(t, "in", mounts["writer"].SubPath)
	assert.False(t, mounts["writer"].ReadOnly)
	assert.Equal(t, "out", mounts["reader"].SubPath)
	assert.True(t, mounts["reader"].ReadOnly, "readOnly must stay per container")
	assert.Equal(t, mounts["writer"].Name, mounts["reader"].Name)
}

// TestSharedVolumeMountedTwiceInOneContainer checks the single container variant: mounting one
// source at two paths inside the same container is legal, and must still yield one pod volume.
func TestSharedVolumeMountedTwiceInOneContainer(t *testing.T) {
	state := buildVolumeWorkloadState(t, map[string]scoretypes.Container{
		"main": {
			Image: "busybox",
			Volumes: map[string]scoretypes.ContainerVolume{
				"/one": {Source: "${resources.data}"},
				"/two": {Source: "${resources.data}"},
			},
		},
	}, "data")

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)
	deployment := deploymentFrom(t, manifests)

	require.Len(t, deployment.Spec.Template.Spec.Volumes, 1, "one source is one pod volume even when mounted twice")
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	mounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
	require.Len(t, mounts, 2, "the container keeps both mount points")
	assert.Equal(t, mounts[0].Name, mounts[1].Name)
	assert.ElementsMatch(t, []string{"/one", "/two"}, []string{mounts[0].MountPath, mounts[1].MountPath})
}

// TestSharedVolumeClaimAcrossContainers is the claim-backed counterpart: now that claim names follow
// the volume source rather than the mount path, two containers mounting one claim produce the same
// template twice, and a stateful set listing a claim template twice is rejected by Kubernetes.
func TestSharedVolumeClaimAcrossContainers(t *testing.T) {
	state := new(project.State)
	state, err := state.WithWorkload(&scoretypes.Workload{
		Metadata: map[string]interface{}{
			"name":        "example",
			"annotations": map[string]interface{}{internal.WorkloadKindAnnotation: WorkloadKindStatefulSet},
		},
		Containers: map[string]scoretypes.Container{
			"main": {
				Image:   "busybox",
				Volumes: map[string]scoretypes.ContainerVolume{"/one": {Source: "${resources.data}"}},
			},
			"sidecar": {
				Image:   "busybox",
				Volumes: map[string]scoretypes.ContainerVolume{"/two": {Source: "${resources.data}"}},
			},
		},
		Resources: map[string]scoretypes.Resource{
			"data": {Type: "vol", Class: internal.Ref("default")},
		},
	}, nil, project.WorkloadExtras{})
	require.NoError(t, err)
	state.Resources = map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{
		"vol.default#example.data": {
			Type:  "vol",
			Class: "default",
			Outputs: map[string]interface{}{
				"claimSpec": map[string]interface{}{"storageClassName": "default"},
			},
		},
	}

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)

	var statefulSet *v1.StatefulSet
	for _, m := range manifests {
		if s, ok := m.(*v1.StatefulSet); ok {
			statefulSet = s
		}
	}
	require.NotNil(t, statefulSet)

	require.Len(t, statefulSet.Spec.VolumeClaimTemplates, 1, "a shared claim must produce a single volume claim template")
	claimName := statefulSet.Spec.VolumeClaimTemplates[0].Name
	for _, c := range statefulSet.Spec.Template.Spec.Containers {
		require.Len(t, c.VolumeMounts, 1)
		assert.Equal(t, claimName, c.VolumeMounts[0].Name, "both containers must reference the shared claim")
	}
}

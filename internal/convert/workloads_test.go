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
          name: vol-5e3859fe72
        - mountPath: /
          name: proj-vol-0
          readOnly: true
      - image: other-image
        name: c2
        resources: {}
      volumes:
      - emptyDir: {}
        name: vol-5e3859fe72
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

func TestConvertWorkload_BeforeComplete(t *testing.T) {
	// Container with before: {app: {ready: complete}} should go to initContainers
	var err error
	state := new(project.State)
	state, err = state.WithWorkload(&scoretypes.Workload{
		Metadata: map[string]interface{}{"name": "example"},
		Containers: map[string]scoretypes.Container{
			"migrate": {
				Image:   "my-app:latest",
				Command: []string{"migrate"},
				Before: scoretypes.ContainerBefore{
					"app": scoretypes.ContainerBeforeEntry{
						Ready: scoretypes.ContainerBeforeReadyComplete,
					},
				},
			},
			"app": {
				Image: "my-app:latest",
			},
		},
	}, nil, project.WorkloadExtras{InstanceSuffix: "-test"})
	require.NoError(t, err)

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)

	// Find the Deployment
	for _, m := range manifests {
		if dep, ok := m.(*v1.Deployment); ok {
			assert.Len(t, dep.Spec.Template.Spec.InitContainers, 1, "expected 1 init container")
			assert.Equal(t, "migrate", dep.Spec.Template.Spec.InitContainers[0].Name)
			assert.Nil(t, dep.Spec.Template.Spec.InitContainers[0].RestartPolicy, "complete init container should not have restartPolicy")

			assert.Len(t, dep.Spec.Template.Spec.Containers, 1, "expected 1 regular container")
			assert.Equal(t, "app", dep.Spec.Template.Spec.Containers[0].Name)
			return
		}
	}
	t.Fatal("no Deployment found in manifests")
}

func TestConvertWorkload_BeforeStarted(t *testing.T) {
	// Container with before: {app: {ready: started}} should go to initContainers with restartPolicy: Always
	var err error
	state := new(project.State)
	state, err = state.WithWorkload(&scoretypes.Workload{
		Metadata: map[string]interface{}{"name": "example"},
		Containers: map[string]scoretypes.Container{
			"sidecar": {
				Image: "sidecar:latest",
				Before: scoretypes.ContainerBefore{
					"app": scoretypes.ContainerBeforeEntry{
						Ready: scoretypes.ContainerBeforeReadyStarted,
					},
				},
			},
			"app": {
				Image: "my-app:latest",
			},
		},
	}, nil, project.WorkloadExtras{InstanceSuffix: "-test"})
	require.NoError(t, err)

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)

	for _, m := range manifests {
		if dep, ok := m.(*v1.Deployment); ok {
			assert.Len(t, dep.Spec.Template.Spec.InitContainers, 1, "expected 1 init container")
			assert.Equal(t, "sidecar", dep.Spec.Template.Spec.InitContainers[0].Name)
			require.NotNil(t, dep.Spec.Template.Spec.InitContainers[0].RestartPolicy, "sidecar should have restartPolicy")
			assert.Equal(t, coreV1.ContainerRestartPolicyAlways, *dep.Spec.Template.Spec.InitContainers[0].RestartPolicy)

			assert.Len(t, dep.Spec.Template.Spec.Containers, 1, "expected 1 regular container")
			assert.Equal(t, "app", dep.Spec.Template.Spec.Containers[0].Name)
			return
		}
	}
	t.Fatal("no Deployment found in manifests")
}

func TestConvertWorkload_NoBefore(t *testing.T) {
	// Containers without before should all go to regular containers (backward compatible)
	var err error
	state := new(project.State)
	state, err = state.WithWorkload(&scoretypes.Workload{
		Metadata: map[string]interface{}{"name": "example"},
		Containers: map[string]scoretypes.Container{
			"web": {
				Image: "nginx:latest",
			},
			"worker": {
				Image: "worker:latest",
			},
		},
	}, nil, project.WorkloadExtras{InstanceSuffix: "-test"})
	require.NoError(t, err)

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)

	for _, m := range manifests {
		if dep, ok := m.(*v1.Deployment); ok {
			assert.Len(t, dep.Spec.Template.Spec.InitContainers, 0, "expected no init containers")
			assert.Len(t, dep.Spec.Template.Spec.Containers, 2, "expected 2 regular containers")
			return
		}
	}
	t.Fatal("no Deployment found in manifests")
}

func TestConvertWorkload_MixedBefore(t *testing.T) {
	// Mixed: complete init + started sidecar + regular container
	var err error
	state := new(project.State)
	state, err = state.WithWorkload(&scoretypes.Workload{
		Metadata: map[string]interface{}{"name": "example"},
		Containers: map[string]scoretypes.Container{
			"migrate": {
				Image:   "my-app:latest",
				Command: []string{"migrate"},
				Before: scoretypes.ContainerBefore{
					"app": scoretypes.ContainerBeforeEntry{
						Ready: scoretypes.ContainerBeforeReadyComplete,
					},
				},
			},
			"sidecar": {
				Image: "sidecar:latest",
				Before: scoretypes.ContainerBefore{
					"app": scoretypes.ContainerBeforeEntry{
						Ready: scoretypes.ContainerBeforeReadyStarted,
					},
				},
			},
			"app": {
				Image: "my-app:latest",
			},
		},
	}, nil, project.WorkloadExtras{InstanceSuffix: "-test"})
	require.NoError(t, err)

	manifests, err := ConvertWorkload(state, "example")
	require.NoError(t, err)

	for _, m := range manifests {
		if dep, ok := m.(*v1.Deployment); ok {
			assert.Len(t, dep.Spec.Template.Spec.InitContainers, 2, "expected 2 init containers")
			assert.Len(t, dep.Spec.Template.Spec.Containers, 1, "expected 1 regular container")
			assert.Equal(t, "app", dep.Spec.Template.Spec.Containers[0].Name)

			// Check that migrate is init (no restartPolicy) and sidecar has restartPolicy
			for _, ic := range dep.Spec.Template.Spec.InitContainers {
				if ic.Name == "migrate" {
					assert.Nil(t, ic.RestartPolicy, "migrate should not have restartPolicy")
				} else if ic.Name == "sidecar" {
					require.NotNil(t, ic.RestartPolicy, "sidecar should have restartPolicy")
					assert.Equal(t, coreV1.ContainerRestartPolicyAlways, *ic.RestartPolicy)
				}
			}
			return
		}
	}
	t.Fatal("no Deployment found in manifests")
}

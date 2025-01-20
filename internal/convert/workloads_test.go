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
	"encoding/base64"
	"testing"

	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				Files: []scoretypes.ContainerFilesElem{
					{
						Target:  "/root.md",
						Content: internal.Ref("my-content ${metadata.name}"),
					},
					{
						Target:        "/binary",
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
				Volumes: []scoretypes.ContainerVolumesElem{
					{
						Target: "/mount/thing",
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
  file: bXktY29udGVudCBleGFtcGxl
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: example-c1-file-0
---
apiVersion: v1
binaryData:
  file: aGVsbG8gJHttZXRhZGF0YS5uYW1lfSB3b3JsZA==
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: example-c1-file-1
---
apiVersion: v1
kind: Service
metadata:
  annotations:
    k8s.score.dev/workload-name: example
  creationTimestamp: null
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
  creationTimestamp: null
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
      creationTimestamp: null
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
          name: vol-0
        - mountPath: /
          name: proj-vol-0
          readOnly: true
      - image: other-image
        name: c2
        resources: {}
      volumes:
      - emptyDir: {}
        name: vol-0
      - name: proj-vol-0
        projected:
          sources:
          - configMap:
              items:
              - key: file
                path: root.md
              name: example-c1-file-0
          - configMap:
              items:
              - key: file
                path: binary
              name: example-c1-file-1
status: {}
---
`, out.String())
}

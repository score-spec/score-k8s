package convert

import (
	"bytes"
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
						Content: internal.Ref("my-content"),
					},
				},
				LivenessProbe: &scoretypes.ContainerProbe{HttpGet: scoretypes.HttpProbe{
					Scheme: internal.Ref(scoretypes.HttpProbeSchemeHTTP),
					Host:   internal.Ref("hostname"),
					Port:   3000,
					Path:   "/something",
				}},
				ReadinessProbe: &scoretypes.ContainerProbe{HttpGet: scoretypes.HttpProbe{
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
	}, nil, framework.NoExtras{})
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
		}
	}
	assert.Equal(t, `apiVersion: v1
binaryData:
  file: bXktY29udGVudA==
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: example-c1-file-0
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    k8s.score.dev/workload-name: example
  creationTimestamp: null
  name: example
spec:
  replicas: 1
  selector:
    matchExpressions:
    - key: score-workload
      operator: In
      values:
      - example
  strategy: {}
  template:
    metadata:
      annotations:
        k8s.score.dev/workload-name: example
        my.custom.scope/annotation: value
      creationTimestamp: null
      labels:
        score-workload: example
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
          httpGet:
            host: hostname
            path: /something
            port: 3000
            scheme: HTTP
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
          name: file-0
      - image: other-image
        name: c2
        resources: {}
      volumes:
      - emptyDir: {}
        name: vol-0
      - configMap:
          items:
          - key: file
            path: root.md
          name: example-c1-file-0
        name: file-0
status: {}
`, out.String())
}

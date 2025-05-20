package patching

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/score-spec/score-k8s/internal/project"
)

func TestPatchServices(t *testing.T) {
	output, err := PatchServices(
		new(project.State),
		[]map[string]interface{}{
			{
				"apiVersion": "apps/V1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "x",
				},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []map[string]interface{}{
								{
									"name":  "main",
									"image": "some/image",
								},
							},
						},
					},
				},
			},
		},
		`
{{ range $i, $m := .Manifests }}
- op: set
  path: {{ $i }}.metadata.annotations.k8s\.score\.dev/workload-name
  value: {{ $m.metadata.name }}
  description: Do a thing
- op: delete
  path: {{ $i }}.spec.template.spec.containers.0.name
{{ end }}
`,
	)
	require.NoError(t, err)
	assert.Equal(t, []map[string]interface{}{
		{
			"apiVersion": "apps/V1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{"k8s.score.dev/workload-name": "x"},
				"name":        "x",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"image": "some/image",
							},
						},
					},
				},
			},
		},
	}, output)
}

func TestPatchServices_can_delete_manifest(t *testing.T) {
	output, err := PatchServices(
		new(project.State),
		[]map[string]interface{}{
			{
				"apiVersion": "apps/V1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "x",
				},
				"spec": map[string]interface{}{},
			},
			{
				"apiVersion": "apps/V1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "y",
				},
				"spec": map[string]interface{}{},
			},
		},
		`
{{ range $i, $m := (reverse .Manifests) }}
{{ $i := sub (len $.Manifests) (add $i 1) }}
- op: delete
  path: {{ $i }}
{{ end }}
`,
	)
	require.NoError(t, err)
	assert.Equal(t, []map[string]interface{}{}, output)
}

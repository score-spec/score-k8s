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

package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/score-spec/score-k8s/internal/project"
)

func changeToDir(t *testing.T, dir string) string {
	t.Helper()
	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	return dir
}

func changeToTempDir(t *testing.T) string {
	return changeToDir(t, t.TempDir())
}

func TestGenerateWithoutInit(t *testing.T) {
	_ = changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"generate"})
	assert.EqualError(t, err, "state directory does not exist, please run \"score-k8s init\" first")
	assert.Equal(t, "", stdout)
}

func TestGenerateWithoutScoreFiles(t *testing.T) {
	_ = changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{"generate"})
	assert.EqualError(t, err, "Project is empty, please add a score file")
	assert.Equal(t, "", stdout)
}

func TestInitAndGenerateWithBadFile(t *testing.T) {
	td := changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)

	assert.NoError(t, os.WriteFile(filepath.Join(td, "thing"), []byte(`"blah"`), 0644))

	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{"generate", "thing"})
	assert.EqualError(t, err, "failed to decode input score file: thing: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `blah` into map[string]interface {}")
	assert.Equal(t, "", stdout)
}

func TestInitAndGenerateWithBadScore(t *testing.T) {
	td := changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)

	assert.NoError(t, os.WriteFile(filepath.Join(td, "thing"), []byte(`{}`), 0644))

	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{"generate", "thing"})
	assert.EqualError(t, err, "invalid score file: thing: jsonschema: '' does not validate with https://score.dev/schemas/score#/required: missing properties: 'apiVersion', 'metadata', 'containers'")
	assert.Equal(t, "", stdout)
}

func TestInitAndGenerate_with_sample(t *testing.T) {
	td := changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	require.NoError(t, err)
	assert.Equal(t, "", stdout)

	// write overrides file
	assert.NoError(t, os.WriteFile(filepath.Join(td, "overrides.yaml"), []byte(`{"resources": {"foo": {"type": "example-provisioner-resource"}}}`), 0644))
	// generate
	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{
		"generate", "-o", "manifests.yaml",
		"--overrides-file", "overrides.yaml",
		"--override-property", "containers.main.variables.THING=${resources.foo.plaintext}",
		"--", "score.yaml",
	})
	require.NoError(t, err)
	assert.Equal(t, "", stdout)
	raw, err := os.ReadFile(filepath.Join(td, "manifests.yaml"))
	assert.NoError(t, err)
	assert.Contains(t, string(raw), "\nkind: ConfigMap\n")
	assert.Contains(t, string(raw), "\nkind: Service\n")
	assert.Contains(t, string(raw), "\nkind: Deployment\n")

	// check that state was persisted
	sd, ok, err := project.LoadStateDirectory(td)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "score.yaml", *sd.State.Workloads["example"].File)
	assert.Len(t, sd.State.Workloads, 1)
	assert.Len(t, sd.State.Resources, 1)
}

func TestInitAndGenerate_with_image_override(t *testing.T) {
	td := changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)

	// write new score file
	assert.NoError(t, os.WriteFile(filepath.Join(td, "score.yaml"), []byte(`
apiVersion: score.dev/v1b1
metadata:
  name: example
containers:
  example:
    image: .
`), 0644))

	t.Run("generate but fail due to missing override", func(t *testing.T) {
		stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{
			"generate", "-o", "manifests.yaml", "--", "score.yaml",
		})
		assert.EqualError(t, err, "failed to convert 'score.yaml' because container 'example' has no image and --image was not provided")
	})

	t.Run("generate with image", func(t *testing.T) {
		// generate with image
		stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{
			"generate", "-o", "manifests.yaml", "--image", "busybox:latest", "--", "score.yaml",
		})
		assert.NoError(t, err)
		assert.Equal(t, "", stdout)
		raw, err := os.ReadFile(filepath.Join(td, "manifests.yaml"))
		assert.NoError(t, err)
		assert.Contains(t, string(raw), "---\napiVersion: apps/v1\nkind: Deployment\n")
	})
}

func TestInitAndGenerate_with_default_provisioners(t *testing.T) {
	td := changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)

	// write new score file
	assert.NoError(t, os.WriteFile(filepath.Join(td, "score.yaml"), []byte(`
apiVersion: score.dev/v1b1
metadata:
  name: example
containers:
  example:
    image: nginx:latest
service:
  ports:
    web:
      port: 8080
resources:
  res-a:
    type: example-provisioner-resource
  res-b:
    type: volume
  res-c:
    type: dns
  res-d:
    type: route
    params:
      host: ${resources.res-c.host}
      path: /
      port: 8080
  res-e:
    type: postgres
  res-f:
    type: redis
`), 0644))

	// generate first
	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{
		"generate", "-o", "manifests.yaml", "--", "score.yaml",
	})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	raw, err := os.ReadFile(filepath.Join(td, "manifests.yaml"))
	assert.NoError(t, err)

	// generate second
	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{
		"generate", "-o", "manifests.yaml", "--", "score.yaml",
	})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	raw2, err := os.ReadFile(filepath.Join(td, "manifests.yaml"))
	assert.NoError(t, err)

	assert.Contains(t, string(raw), string(raw2))
}

func TestGenerateMultipleSpecsWithImage(t *testing.T) {
	td := changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	assert.NoError(t, os.WriteFile(filepath.Join(td, "scoreA.yaml"), []byte(`
apiVersion: score.dev/v1b1
metadata:
  name: example-a
containers:
  hello:
    image: foo
`), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(td, "scoreB.yaml"), []byte(`
apiVersion: score.dev/v1b1
metadata:
  name: example-b
containers:
  hello:
    image: foo
`), 0644))
	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{
		"generate", "--image", "nginx:latest", "scoreA.yaml", "scoreB.yaml",
	})
	assert.EqualError(t, err, "cannot use --override-property, --overrides-file, or --image when 0 or more than 1 score files are provided")
	assert.Equal(t, "", stdout)
}

func TestDeduplicateResourceManifests(t *testing.T) {
	td := changeToTempDir(t)
	assert.NoError(t, os.WriteFile(filepath.Join(td, "score.yaml"), []byte(`
apiVersion: score.dev/v1b1
metadata:
  name: example-a
containers:
  hello:
    image: foo
resources:
  d1:
    type: dummy
  d2:
    type: dummy
`), 0644))
	_, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	require.NoError(t, err)
	assert.NoError(t, os.WriteFile(filepath.Join(td, ".score-k8s", "00.provisioners.yaml"), []byte(`
- uri: template://dummy
  type: dummy
  manifests: |
    - apiVersion: v1
      kind: Secret
      metadata:
        name: my-secret
      data:
        fruit: {{ b64enc "banana" }}
`), 0644))
	_, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"generate", "score.yaml"})
	t.Log(string(stderr))
	require.NoError(t, err)
	rawManifests, err := os.ReadFile(filepath.Join(td, "manifests.yaml"))
	require.NoError(t, err)
	assert.Equal(t, strings.Count(string(rawManifests), "kind: Secret"), 1, "failed to find in", string(rawManifests))
}

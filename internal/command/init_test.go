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

package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/score-spec/score-go/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/score-spec/score-k8s/internal/project"
	"github.com/score-spec/score-k8s/internal/provisioners"
	"github.com/score-spec/score-k8s/internal/provisioners/loader"
)

func TestInitNominal(t *testing.T) {
	td := t.TempDir()

	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(td))
	defer func() {
		require.NoError(t, os.Chdir(wd))
	}()

	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	assert.NotEqual(t, "", strings.TrimSpace(stderr))

	stdout, stderr, err = executeAndResetCommand(context.Background(), rootCmd, []string{"generate", "score.yaml"})
	assert.NoError(t, err)
	assert.Equal(t, ``, stdout)
	assert.NotEqual(t, "", strings.TrimSpace(stderr))

	sd, ok, err := project.LoadStateDirectory(".")
	assert.NoError(t, err)
	if assert.True(t, ok) {
		assert.Equal(t, project.DefaultRelativeStateDirectory, sd.Path)
		assert.Len(t, sd.State.Workloads, 1)
		assert.Equal(t, map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{}, sd.State.Resources)
		assert.Equal(t, map[string]interface{}{}, sd.State.SharedState)
	}
}

func TestInitNoSample(t *testing.T) {
	td := t.TempDir()

	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(td))
	defer func() {
		require.NoError(t, os.Chdir(wd))
	}()

	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--no-sample"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	assert.NotEqual(t, "", strings.TrimSpace(stderr))

	_, err = os.Stat("score.yaml")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestInitNominal_run_twice(t *testing.T) {
	td := t.TempDir()

	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(td))
	defer func() {
		require.NoError(t, os.Chdir(wd))
	}()

	// first init
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--file", "score2.yaml"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	assert.NotEqual(t, "", strings.TrimSpace(stderr))

	// check default provisioners exists and overwrite it with an empty array
	dpf, err := os.Stat(filepath.Join(td, ".score-k8s", "zz-default.provisioners.yaml"))
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(filepath.Join(td, ".score-k8s", dpf.Name()), []byte("[]"), 0644))

	// init again
	stdout, stderr, err = executeAndResetCommand(context.Background(), rootCmd, []string{"init"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	assert.NotEqual(t, "", strings.TrimSpace(stderr))

	// verify that default provisioners was not overwritten again
	dpf, err = os.Stat(filepath.Join(td, ".score-k8s", dpf.Name()))
	assert.NoError(t, err)
	assert.Equal(t, 2, int(dpf.Size()))

	_, err = os.Stat("score.yaml")
	assert.NoError(t, err)
	_, err = os.Stat("score2.yaml")
	assert.NoError(t, err)

	sd, ok, err := project.LoadStateDirectory(".")
	assert.NoError(t, err)
	if assert.True(t, ok) {
		assert.Equal(t, project.DefaultRelativeStateDirectory, sd.Path)
		assert.Equal(t, map[string]framework.ScoreWorkloadState[project.WorkloadExtras]{}, sd.State.Workloads)
		assert.Equal(t, map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{}, sd.State.Resources)
		assert.Equal(t, map[string]interface{}{}, sd.State.SharedState)
	}
}

func TestInitWithProvisioners(t *testing.T) {
	td := t.TempDir()
	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(td))
	defer func() {
		require.NoError(t, os.Chdir(wd))
	}()

	td2 := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(td2, "one.provisioners.yaml"), []byte(`
- uri: template://one
  type: thing
  outputs: "{}"
`), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(td2, "two.provisioners.yaml"), []byte(`
- uri: template://two
  type: thing
  outputs: "{}"
`), 0644))

	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--provisioners", filepath.Join(td2, "one.provisioners.yaml"), "--provisioners", "file://" + filepath.Join(td2, "two.provisioners.yaml")})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	assert.NotEqual(t, "", strings.TrimSpace(stderr))

	provs, err := loader.LoadProvisionersFromDirectory(filepath.Join(td, ".score-k8s"), loader.DefaultSuffix)
	assert.NoError(t, err)
	expectedProvisionerUris := []string{"template://one", "template://two"}
	for _, expectedUri := range expectedProvisionerUris {
		assert.True(t, slices.ContainsFunc(provs, func(p provisioners.Provisioner) bool {
			return p.Uri() == expectedUri
		}), fmt.Sprintf("Expected provisioner '%s' not found", expectedUri))
	}
}

func TestInitWithPatchingFiles(t *testing.T) {
	td := t.TempDir()
	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(td))
	defer func() {
		require.NoError(t, os.Chdir(wd))
	}()
	assert.NoError(t, os.WriteFile(filepath.Join(td, "patch-templates-1"), []byte(`[]`), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(td, "patch-templates-2"), []byte(`[]`), 0644))

	t.Run("new", func(t *testing.T) {
		_, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--patch-templates", filepath.Join(td, "patch-templates-1"), "--patch-templates", filepath.Join(td, "patch-templates-2")})
		assert.NoError(t, err)
		t.Log(stderr)
		sd, ok, err := project.LoadStateDirectory(".")
		assert.NoError(t, err)
		if assert.True(t, ok) {
			assert.Len(t, sd.State.Extras.PatchingTemplates, 2)
		}
	})

	t.Run("update", func(t *testing.T) {
		_, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--patch-templates", filepath.Join(td, "patch-templates-2")})
		assert.NoError(t, err)
		t.Log(stderr)
		sd, ok, err := project.LoadStateDirectory(".")
		assert.NoError(t, err)
		if assert.True(t, ok) {
			assert.Len(t, sd.State.Extras.PatchingTemplates, 1)
		}
	})

	t.Run("bad patch", func(t *testing.T) {
		assert.NoError(t, os.WriteFile(filepath.Join(td, "patch-templates-3"), []byte(`{{ what is this }}`), 0644))
		_, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--patch-templates", filepath.Join(td, "patch-templates-3")})
		assert.Error(t, err, "failed to parse template: template: :1: function \"what\" not defined")
	})
}

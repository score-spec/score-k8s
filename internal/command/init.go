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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	"github.com/score-spec/score-go/uriget"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/score-spec/score-k8s/internal/project"
	_default "github.com/score-spec/score-k8s/internal/provisioners/default"
	"github.com/score-spec/score-k8s/internal/provisioners/loader"
)

const (
	initCmdFileFlag         = "file"
	initCmdFileNoSampleFlag = "no-sample"
	initCmdProvisionerFlag  = "provisioners"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Args:  cobra.NoArgs,
	Short: "Initialise a new score-k8s project with local state directory and sample score file",
	Long: `The init subcommand will prepare the current directory for working with score-k8s and write the initial
empty state and default provisioners file into the '.score-k8s' subdirectory.

The '.score-k8s' directory contains state that will be used to generate any Kubernetes resource manifests including
potentially sensitive data and raw secrets, so this should not be checked into generic source control.

Custom provisioners can be installed by uri using the --provisioners flag. The provisioners will be installed and take
precedence in the order they are defined over the default provisioners. If init has already been called with provisioners
the new provisioners will take precedence.
`,
	Example: `
  # Initialise a new score-k8s project
  score-k8s init

  # Or disable the default score file generation if you already have a score file
  score-k8s init --no-sample

  # Optionally loading in provisoners from a remote url
  score-k8s init --provisioners https://raw.githubusercontent.com/user/repo/main/example.yaml`,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		sd, ok, err := project.LoadStateDirectory(".")
		if err != nil {
			return errors.Wrap(err, "failed to load existing state directory")
		} else if ok {
			slog.Info("Found existing state directory", "dir", sd.Path)
		} else {
			slog.Info("Writing new state directory", "dir", project.DefaultRelativeStateDirectory)
			sd = &project.StateDirectory{
				Path: project.DefaultRelativeStateDirectory,
				State: project.State{
					Workloads:   map[string]framework.ScoreWorkloadState[project.WorkloadExtras]{},
					Resources:   map[framework.ResourceUid]framework.ScoreResourceState[project.ResourceExtras]{},
					SharedState: map[string]interface{}{},
				},
			}
			slog.Info("Writing new state directory", "dir", sd.Path)
			if err := sd.Persist(); err != nil {
				return errors.Wrap(err, "failed to persist new state directory")
			}
		}

		defaultProvisioners := filepath.Join(sd.Path, "zz-default.provisioners.yaml")
		if _, err := os.Stat(defaultProvisioners); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return errors.Wrapf(err, "failed to check for existing default provisioners file")
			}
			if err := os.WriteFile(defaultProvisioners, []byte(_default.DefaultProvisioners), 0644); err != nil {
				if errors.Is(err, os.ErrExist) {
					return errors.Errorf("default provisioners file '%s' already exists", defaultProvisioners)
				}
				return errors.Wrap(err, "failed to open default provisioners file")
			}
			slog.Info("Created default provisioners file", "file", defaultProvisioners)
		} else {
			slog.Info("Skipping creation of default provisioners file since it already exists", "file", defaultProvisioners)
		}

		initCmdScoreFile, _ := cmd.Flags().GetString(initCmdFileFlag)
		if _, err := os.Stat(initCmdScoreFile); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return errors.Wrap(err, "failed to check for existing Score file")
			}
			if v, _ := cmd.Flags().GetBool(initCmdFileNoSampleFlag); v {
				slog.Info("Initial Score file does not exist - and sample generation is disabled", "file", initCmdScoreFile)
			} else {
				workload := &scoretypes.Workload{
					ApiVersion: "score.dev/v1b1",
					Metadata: map[string]interface{}{
						"name": "example",
					},
					Containers: map[string]scoretypes.Container{
						"main": {
							Image: "stefanprodan/podinfo",
						},
					},
					Service: &scoretypes.WorkloadService{
						Ports: map[string]scoretypes.ServicePort{
							"web": {Port: 8080},
						},
					},
				}
				if f, err := os.OpenFile(initCmdScoreFile, os.O_CREATE|os.O_WRONLY, 0755); err != nil {
					return errors.Wrap(err, "failed to open empty Score file")
				} else {
					defer f.Close()
					if err := yaml.NewEncoder(f).Encode(workload); err != nil {
						return errors.Wrap(err, "failed to write Score file")
					}
					slog.Info("Created initial Score file", "file", initCmdScoreFile)
				}
			}
		} else {
			slog.Info("Skipping creation of initial Score file since it already exists", "file", initCmdScoreFile)
		}

		if v, _ := cmd.Flags().GetStringArray(initCmdProvisionerFlag); len(v) > 0 {
			for i, vi := range v {
				data, err := uriget.GetFile(cmd.Context(), vi)
				if err != nil {
					return fmt.Errorf("failed to load provisioner %d: %w", i+1, err)
				}
				if err := loader.SaveProvisionerToDirectory(sd.Path, vi, data); err != nil {
					return fmt.Errorf("failed to save provisioner %d: %w", i+1, err)
				}
			}
		}

		if provs, err := loader.LoadProvisionersFromDirectory(sd.Path, loader.DefaultSuffix); err != nil {
			return fmt.Errorf("failed to load existing provisioners: %w", err)
		} else {
			slog.Debug(fmt.Sprintf("Successfully loaded %d resource provisioners", len(provs)))
		}

		slog.Info("Read more about the Score specification at https://docs.score.dev/docs/")

		return nil
	},
}

func init() {
	initCmd.Flags().StringP(initCmdFileFlag, "f", "score.yaml", "The score file to initialize")
	initCmd.Flags().Bool(initCmdFileNoSampleFlag, false, "Disable generation of the sample score file")
	initCmd.Flags().StringArray(initCmdProvisionerFlag, nil, "Provisioner files to install. May be specified multiple times. Supports:\n"+
		"- HTTP        : http://host/file\n"+
		"- HTTPS       : https://host/file\n"+
		"- Git (SSH)   : git-ssh://git@host/repo.git/file\n"+
		"- Git (HTTPS) : git-https://host/repo.git/file\n"+
		"- OCI         : oci://[registry/][namespace/]repository[:tag|@digest]")
	rootCmd.AddCommand(initCmd)
}

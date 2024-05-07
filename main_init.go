package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/score-spec/score-k8s/internal/project"
	"github.com/score-spec/score-k8s/internal/provisioners/default"
)

var initCmd = &cobra.Command{
	Use:           "init",
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		sd, ok, err := project.LoadStateDirectory(".")
		if err != nil {
			return errors.Wrap(err, "failed to load existing state directory")
		} else if ok {
			slog.Info(fmt.Sprintf("Found existing state directory '%s'", sd.Path))
		} else {
			slog.Info(fmt.Sprintf("Writing new state directory '%s'", project.DefaultRelativeStateDirectory))
			sd = &project.StateDirectory{
				Path: project.DefaultRelativeStateDirectory,
				State: project.State{
					Workloads:   map[string]framework.ScoreWorkloadState[framework.NoExtras]{},
					Resources:   map[framework.ResourceUid]framework.ScoreResourceState{},
					SharedState: map[string]interface{}{},
					Extras:      project.StateExtras{},
				},
			}
			slog.Info(fmt.Sprintf("Writing new state directory '%s'", sd.Path))
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
			slog.Info("Skipping creation of default provisioners file since it already exists")
		}

		if _, err := os.Stat("score.yaml"); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return errors.Wrapf(err, "failed to check for existing score.yaml")
			}
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
			if f, err := os.OpenFile("score.yaml", os.O_CREATE|os.O_WRONLY, 0600); err != nil {
				return errors.Wrap(err, "failed to open empty score.yaml file")
			} else {
				defer f.Close()
				if err := yaml.NewEncoder(f).Encode(workload); err != nil {
					return errors.Wrap(err, "failed to write empty score.yaml file")
				}
				slog.Info("Created empty score.yaml file", "file", "score.yaml")
			}
		} else {
			slog.Info("Skipping creation of score.yaml file since it already exists")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

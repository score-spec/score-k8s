package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoreloader "github.com/score-spec/score-go/loader"
	scoreschema "github.com/score-spec/score-go/schema"
	scoretypes "github.com/score-spec/score-go/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/score-spec/score-k8s/internal/convert"
	"github.com/score-spec/score-k8s/internal/project"
)

var rootCmd = &cobra.Command{
	Use:           "score-k8s",
	SilenceErrors: true,
}

const projectDirectory = ".score-k8s"
const stateFileName = "state.yaml"
const manifestsDirectory = "manifests"

var initCmd = &cobra.Command{
	Use:           "init",
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		if err := os.MkdirAll(projectDirectory, 0700); err != nil {
			return errors.Wrap(err, "failed to ensure state directory")
		}

		state := new(framework.State[framework.NoExtras, framework.NoExtras])
		stateFile := filepath.Join(projectDirectory, stateFileName)
		if f, err := os.OpenFile(stateFile, os.O_CREATE|os.O_WRONLY, 0600); err != nil {
			if errors.Is(err, os.ErrExist) {
				return errors.Errorf("state file '%s' already exists", stateFile)
			}
			return errors.Wrap(err, "failed to open empty project state")
		} else {
			defer f.Close()
			if err := yaml.NewEncoder(f).Encode(state); err != nil {
				return errors.Wrap(err, "failed to write empty project state")
			}
			slog.Info("Created empty project state", "file", stateFile)
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
		}
		if f, err := os.OpenFile("score.yaml", os.O_CREATE|os.O_WRONLY, 0600); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return errors.Wrap(err, "failed to open empty score.yaml file")
			}
		} else {
			defer f.Close()
			if err := yaml.NewEncoder(f).Encode(workload); err != nil {
				return errors.Wrap(err, "failed to write empty score.yaml file")
			}
			slog.Info("Created empty score.yaml file", "file", "score.yaml")
		}

		return nil
	},
}

var generateCmd = &cobra.Command{
	Use:           "generate",
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		stateFile := filepath.Join(projectDirectory, stateFileName)
		var state *project.State

		if raw, err := os.ReadFile(stateFile); err != nil {
			return errors.Wrap(err, "failed to read existing state file")
		} else {
			enc := yaml.NewDecoder(bytes.NewReader(raw))
			enc.KnownFields(false)
			var rawState project.State
			if err = enc.Decode(&rawState); err != nil {
				return errors.Wrap(err, "failed to load existing state")
			}
			state = &rawState
			slog.Info("Loaded project state", "file", stateFile, "#workloads", len(state.Workloads), "#resources", len(state.Resources))
		}

		slices.Sort(args)
		for _, arg := range args {
			var rawWorkload map[string]interface{}
			var workload scoretypes.Workload
			if raw, err := os.ReadFile(arg); err != nil {
				return errors.Wrapf(err, "failed to read input score file: %s", arg)
			} else if err = yaml.Unmarshal(raw, &rawWorkload); err != nil {
				return errors.Wrapf(err, "failed to decode input score file: %s", arg)
			} else if err = scoreschema.Validate(rawWorkload); err != nil {
				return errors.Wrapf(err, "invalid score file: %s", arg)
			} else if err = scoreloader.MapSpec(&workload, rawWorkload); err != nil {
				return errors.Wrapf(err, "failed to decode input score file: %s", arg)
			} else if state, err = state.WithWorkload(&workload, &arg, framework.NoExtras{}); err != nil {
				return errors.Wrapf(err, "failed to add score file to project: %s", arg)
			}
			slog.Info("Added score file to project", "file", arg)
		}

		var err error
		if state, err = state.WithPrimedResources(); err != nil {
			return errors.Wrap(err, "failed to prime resources")
		}
		slog.Info("Primed resources")

		if len(state.Workloads) == 0 {
			return errors.New("Project is empty, please add a score file")
		}

		if items, err := os.ReadDir(manifestsDirectory); err == nil && len(items) > 0 {
			manifestsBackup := manifestsDirectory + ".backup." + time.Now().Format("20060102150405")
			if err := os.Rename(manifestsDirectory, manifestsBackup); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return errors.Wrap(err, "failed to backup previous manifests")
				}
				slog.Info("No previous manifests directory to backup")
			} else {
				slog.Info("Backed up manifests directory", "dir", manifestsBackup)
			}
		}

		if err := os.MkdirAll(manifestsDirectory, 0700); err != nil {
			return errors.Wrapf(err, "failed to create output manifests directory")
		}
		slog.Info("Created new manifests directory", "dir", manifestsDirectory)

		scheme := runtime.NewScheme()
		_ = coreV1.AddToScheme(scheme)
		_ = appsV1.AddToScheme(scheme)
		cf := serializer.NewCodecFactory(scheme)
		info, _ := runtime.SerializerInfoForMediaType(cf.SupportedMediaTypes(), runtime.ContentTypeYAML)

		resIds, err := state.GetSortedResourceUids()
		if err != nil {
			return errors.Wrap(err, "failed to determine resource sorting")
		}

		seenNames := make(map[string]bool)
		for i := len(resIds) - 1; i >= 0; i-- {
			resId := resIds[i]
			var manifests []v1.Object
			manifests, state, err = convert.ConvertResource(state, resId)
			if err != nil {
				return errors.Wrapf(err, "resources: %s: failed to convert", resId)
			}
			if len(manifests) > 0 {
				out := new(bytes.Buffer)
				for _, m := range manifests {
					name := fmt.Sprintf("%s/%s", m.(v1.Type).GetKind(), m.GetName())
					if !seenNames[name] {
						out.WriteString("---\n")
						if err = info.Serializer.Encode(m.(runtime.Object), out); err != nil {
							return errors.Wrapf(err, "resources: %s: failed to serialise manifest %s", resId, m.GetName())
						}
						out.WriteString("\n")
						seenNames[name] = true
					}
				}
				if err := os.WriteFile(filepath.Join(manifestsDirectory, fmt.Sprintf("resource-%s.yaml", resId)), out.Bytes(), 0600); err != nil {
					return errors.Wrapf(err, "resources: %s: failed to write manifests file", resId)
				}
				slog.Info("Wrote manifests file for resource", "resource", resId)
			}
		}

		for workloadName := range state.Workloads {
			manifests, err := convert.ConvertWorkload(state, workloadName)
			if err != nil {
				return errors.Wrapf(err, "workload: %s: failed to convert", workloadName)
			}
			if len(manifests) > 0 {
				out := new(bytes.Buffer)
				for _, m := range manifests {
					out.WriteString("---\n")
					if err = info.Serializer.Encode(m.(runtime.Object), out); err != nil {
						return errors.Wrapf(err, "workload: %s: failed to serialise manifest %s", workloadName, m.GetName())
					}
					out.WriteString("\n")
				}
				if err := os.WriteFile(filepath.Join(manifestsDirectory, fmt.Sprintf("workload-%s.yaml", workloadName)), out.Bytes(), 0600); err != nil {
					return errors.Wrapf(err, "workload: %s: failed to write manifests file", workloadName)
				}
				slog.Info("Wrote manifests file for workload", "workload", workloadName)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.CompletionOptions = cobra.CompletionOptions{HiddenDefaultCmd: true}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		os.Exit(1)
	}
}

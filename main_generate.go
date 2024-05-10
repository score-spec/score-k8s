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

package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoreloader "github.com/score-spec/score-go/loader"
	scoreschema "github.com/score-spec/score-go/schema"
	scoretypes "github.com/score-spec/score-go/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/score-spec/score-k8s/internal"
	"github.com/score-spec/score-k8s/internal/convert"
	"github.com/score-spec/score-k8s/internal/project"
	"github.com/score-spec/score-k8s/internal/provisioners"
	"github.com/score-spec/score-k8s/internal/provisioners/loader"
)

const (
	generateCmdOverridesFileFlag    = "overrides-file"
	generateCmdOverridePropertyFlag = "override-property"
	generateCmdImageFlag            = "image"
	generateCmdOutputFlag           = "output"
	generateCmdPatchManifestsFlag   = "patch-manifests"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Args:  cobra.ArbitraryArgs,
	Short: "Convert one or more Score files into a set of Kubernetes manifests",
	Long: `The generate command will convert Score files in the current Score state into a combined set of Kubernetes
manifests. All resources and links between Workloads will be resolved and provisioned as required.

"score-compose init" MUST be run first. An error will be thrown if the project directory is not present.
`,
	Example: `
  # Specify Score files
  score-k8s generate score.yaml *.score.yaml

  # Regenerate without adding new score files
  score-k8s generate

  # Provide a default container image for any containers with image=.
  score-k8s generate score.yaml --image=nginx:latest

  # Provide overrides when one score file is provided
  score-k8s generate score.yaml --override-file=./overrides.score.yaml --override-property=metadata.key=value

  # Patch resulting manifests
  score-k8s generate score.yaml --patch-manifests */*/metadata.annotations.key=value --patch-manifests Deployment/foo/spec.replicas=4`,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		sd, ok, err := project.LoadStateDirectory(".")
		if err != nil {
			return fmt.Errorf("failed to load existing state directory: %w", err)
		} else if !ok {
			return fmt.Errorf("state directory does not exist, please run \"score-k8s init\" first")
		}
		state := &sd.State

		if len(args) != 1 && (cmd.Flags().Lookup(generateCmdOverridesFileFlag).Changed || cmd.Flags().Lookup(generateCmdOverridePropertyFlag).Changed) {
			return errors.Errorf("cannot use --%s or --%s when 0 or more than 1 score files are provided", generateCmdOverridePropertyFlag, generateCmdOverridesFileFlag)
		}

		slices.Sort(args)
		for _, arg := range args {
			var rawWorkload map[string]interface{}
			if raw, err := os.ReadFile(arg); err != nil {
				return errors.Wrapf(err, "failed to read input score file: %s", arg)
			} else if err = yaml.Unmarshal(raw, &rawWorkload); err != nil {
				return errors.Wrapf(err, "failed to decode input score file: %s", arg)
			}

			// apply overrides

			if v, _ := cmd.Flags().GetString(generateCmdOverridesFileFlag); v != "" {
				if err := parseAndApplyOverrideFile(v, generateCmdOverridesFileFlag, rawWorkload); err != nil {
					return err
				}
			}

			// Now read, parse, and apply any override properties to the score files
			if v, _ := cmd.Flags().GetStringArray(generateCmdOverridePropertyFlag); len(v) > 0 {
				for _, overridePropertyEntry := range v {
					if rawWorkload, err = parseAndApplyOverrideProperty(overridePropertyEntry, generateCmdOverridePropertyFlag, rawWorkload); err != nil {
						return err
					}
				}
			}

			// Ensure transforms are applied (be a good citizen)
			if changes, err := scoreschema.ApplyCommonUpgradeTransforms(rawWorkload); err != nil {
				return fmt.Errorf("failed to upgrade spec: %w", err)
			} else if len(changes) > 0 {
				for _, change := range changes {
					slog.Info(fmt.Sprintf("Applying backwards compatible upgrade %s", change))
				}
			}

			var workload scoretypes.Workload
			if err = scoreschema.Validate(rawWorkload); err != nil {
				return errors.Wrapf(err, "invalid score file: %s", arg)
			} else if err = scoreloader.MapSpec(&workload, rawWorkload); err != nil {
				return errors.Wrapf(err, "failed to decode input score file: %s", arg)
			}

			// Apply image override
			for containerName, container := range workload.Containers {
				if container.Image == "." {
					if v, _ := cmd.Flags().GetString(generateCmdImageFlag); v != "" {
						container.Image = v
						slog.Info(fmt.Sprintf("Set container image for container '%s' to %s from --%s", containerName, v, generateCmdImageFlag))
						workload.Containers[containerName] = container
					} else {
						return errors.Errorf("failed to convert '%s' because container '%s' has no image and --image was not provided", arg, containerName)
					}
				}
			}

			if state, err = state.WithWorkload(&workload, &arg, framework.NoExtras{}); err != nil {
				return errors.Wrapf(err, "failed to add score file to project: %s", arg)
			}
			slog.Info("Added score file to project", "file", arg)
		}

		if len(state.Workloads) == 0 {
			return errors.New("Project is empty, please add a score file")
		}

		if state, err = state.WithPrimedResources(); err != nil {
			return errors.Wrap(err, "failed to prime resources")
		}

		slog.Info("Primed resources", "#workloads", len(state.Workloads), "#resources", len(state.Resources))

		localProvisioners, err := loader.LoadProvisionersFromDirectory(sd.Path, loader.DefaultSuffix)
		if err != nil {
			return errors.Wrapf(err, "failed to load provisioners")
		}
		slog.Info("Loaded provisioners", "#provisioners", len(localProvisioners))

		state, err = provisioners.ProvisionResources(context.Background(), state, localProvisioners)
		if err != nil {
			return errors.Wrap(err, "failed to provision resources")
		}

		sd.State = *state
		if err := sd.Persist(); err != nil {
			return errors.Wrap(err, "failed to persist state file")
		}
		slog.Info("Persisted state file")

		outputManifests := make([]map[string]interface{}, 0)
		resIds, _ := state.GetSortedResourceUids()
		for _, id := range resIds {
			res := state.Resources[id]
			if len(res.Extras.Manifests) > 0 {
				for _, manifest := range res.Extras.Manifests {
					if p, ok := internal.FindFirstUnresolvedSecretRef("", manifest); ok {
						return errors.Errorf("unresolved secret ref in manifest: %s", p)
					}
					outputManifests = append(outputManifests, manifest)
				}
				slog.Info(fmt.Sprintf("Wrote %d resource manifests to manifests buffer for resource '%s'", len(res.Extras.Manifests), id))
			}
		}

		for workloadName := range state.Workloads {
			manifests, err := convert.ConvertWorkload(state, workloadName)
			if err != nil {
				return errors.Wrapf(err, "workload: %s: failed to convert", workloadName)
			}
			for _, m := range manifests {
				subOut := new(bytes.Buffer)
				if err = internal.YamlSerializerInfo.Serializer.Encode(m.(runtime.Object), subOut); err != nil {
					return errors.Wrapf(err, "workload: %s: failed to serialise manifest %s", workloadName, m.GetName())
				}
				var intermediate map[string]interface{}
				_ = yaml.Unmarshal(subOut.Bytes(), &intermediate)
				if p, ok := internal.FindFirstUnresolvedSecretRef("", intermediate); ok {
					return errors.Errorf("unresolved secret ref in manifest: %s", p)
				}
				outputManifests = append(outputManifests, intermediate)
			}
			slog.Info(fmt.Sprintf("Wrote %d manifests to manifests buffer for workload '%s'", len(manifests), workloadName))
		}

		// patch manifests here
		if v, _ := cmd.Flags().GetStringArray(generateCmdPatchManifestsFlag); len(v) > 0 {
			for _, entry := range v {
				if outputManifests, err = parseAndApplyManifestPatches(entry, generateCmdPatchManifestsFlag, outputManifests); err != nil {
					return err
				}
			}
		}

		out := new(bytes.Buffer)
		for _, manifest := range outputManifests {
			out.WriteString("---\n")
			_ = yaml.NewEncoder(out).Encode(manifest)
		}
		v, _ := cmd.Flags().GetString(generateCmdOutputFlag)
		if v == "" {
			return fmt.Errorf("no output file specified")
		} else if v == "-" {
			_, _ = fmt.Fprint(cmd.OutOrStdout(), out.String())
		} else if err := os.WriteFile(v+".tmp", out.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		} else if err := os.Rename(v+".tmp", v); err != nil {
			return fmt.Errorf("failed to complete writing output file: %w", err)
		} else {
			slog.Info(fmt.Sprintf("Wrote manifests to '%s'", v))
		}
		return nil
	},
}

func parseAndApplyOverrideFile(entry string, flagName string, spec map[string]interface{}) error {
	if raw, err := os.ReadFile(entry); err != nil {
		return fmt.Errorf("--%s '%s' is invalid, failed to read file: %w", flagName, entry, err)
	} else {
		slog.Info(fmt.Sprintf("Applying overrides from %s to workload", entry))
		var out map[string]interface{}
		if err := yaml.Unmarshal(raw, &out); err != nil {
			return fmt.Errorf("--%s '%s' is invalid: failed to decode yaml: %w", flagName, entry, err)
		} else if err := mergo.Merge(&spec, out, mergo.WithOverride); err != nil {
			return fmt.Errorf("--%s '%s' failed to apply: %w", flagName, entry, err)
		}
	}
	return nil
}

func parseAndApplyOverrideProperty(entry string, flagName string, spec map[string]interface{}) (map[string]interface{}, error) {
	parts := strings.SplitN(entry, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("--%s '%s' is invalid, expected a =-separated path and value", flagName, entry)
	}
	if parts[1] == "" {
		slog.Info(fmt.Sprintf("Overriding '%s' in workload", parts[0]))
		after, err := framework.OverridePathInMap(spec, framework.ParseDotPathParts(parts[0]), true, nil)
		if err != nil {
			return nil, fmt.Errorf("--%s '%s' could not be applied: %w", flagName, entry, err)
		}
		return after, nil
	} else {
		var value interface{}
		if err := yaml.Unmarshal([]byte(parts[1]), &value); err != nil {
			return nil, fmt.Errorf("--%s '%s' is invalid, failed to unmarshal value as json: %w", flagName, entry, err)
		}
		slog.Info(fmt.Sprintf("Overriding '%s' in workload", parts[0]))
		after, err := framework.OverridePathInMap(spec, framework.ParseDotPathParts(parts[0]), false, value)
		if err != nil {
			return nil, fmt.Errorf("--%s '%s' could not be applied: %w", flagName, entry, err)
		}
		return after, nil
	}
}

func parseAndApplyManifestPatches(entry string, flagName string, manifests []map[string]interface{}) ([]map[string]interface{}, error) {
	parts := strings.SplitN(entry, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("--%s '%s' is invalid, expected a =-separated path and value", flagName, entry)
	}
	filter := strings.SplitN(parts[0], "/", 3)
	if len(filter) != 3 {
		return nil, fmt.Errorf("--%s '%s' is invalid, expected the patch path to have an initial <kind>/<name>/... prefix", flagName, entry)
	}
	kindFilter, nameFilter, path := filter[0], filter[1], filter[2]
	outManifests := slices.Clone(manifests)

	for i, manifest := range manifests {
		kind, kOk := manifest["kind"].(string)
		meta, _ := manifest["metadata"].(map[string]interface{})
		name, nOk := meta["name"].(string)
		if (kindFilter == "*" || (kOk && kind == kindFilter)) && (nameFilter == "*" || (nOk && name == nameFilter)) {
			if parts[1] == "" {
				slog.Info(fmt.Sprintf("Overriding '%s' in manifest %s/%s", path, kind, name))
				after, err := framework.OverridePathInMap(manifest, framework.ParseDotPathParts(path), true, nil)
				if err != nil {
					return nil, fmt.Errorf("--%s '%s' could not be applied to %s/%s: %w", flagName, entry, kind, name, err)
				}
				manifest = after
			} else {
				var value interface{}
				if err := yaml.Unmarshal([]byte(parts[1]), &value); err != nil {
					return nil, fmt.Errorf("--%s '%s' is invalid, failed to unmarshal value as yaml: %w", flagName, entry, err)
				}
				slog.Info(fmt.Sprintf("Overriding '%s' in manifest %s/%s", path, kind, name))
				after, err := framework.OverridePathInMap(manifest, framework.ParseDotPathParts(path), false, value)
				if err != nil {
					return nil, fmt.Errorf("--%s '%s' could not be applied to %s/%s: %w", flagName, entry, kind, name, err)
				}
				manifest = after
			}
		}
		outManifests[i] = manifest
	}
	return outManifests, nil
}

func init() {
	generateCmd.Flags().StringP(generateCmdOutputFlag, "o", "manifests.yaml", "The output manifests file to write the manifests to")
	generateCmd.Flags().String(generateCmdOverridesFileFlag, "", "An optional file of Score overrides to merge in")
	generateCmd.Flags().StringArray(generateCmdOverridePropertyFlag, []string{}, "An optional set of path=key overrides to set or remove")
	generateCmd.Flags().String(generateCmdImageFlag, "", "An optional container image to use for any container with image == '.'")
	generateCmd.Flags().StringArray(generateCmdPatchManifestsFlag, []string{}, "An optional set of <kind|*>/<name|*>/path=key operations for the output manifests")

	rootCmd.AddCommand(generateCmd)
}

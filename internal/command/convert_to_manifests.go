package command

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/score-spec/score-k8s/internal/convert"
)

var (
	convertToManifests = &cobra.Command{
		Use:    "convert-workload-to-manifests",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			rawInputs, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}
			rawManifests, err := convert.ConvertRawInputsToRawManifests(rawInputs)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(rawManifests)
			return err
		},
	}
)

func init() {
	rootCmd.AddCommand(convertToManifests)
}

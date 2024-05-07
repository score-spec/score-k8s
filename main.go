package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "score-k8s",
	SilenceErrors: true,
}

func init() {
	rootCmd.CompletionOptions = cobra.CompletionOptions{HiddenDefaultCmd: true}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		os.Exit(1)
	}
}

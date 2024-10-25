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
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

// executeAndResetCommand is a test helper that runs and then resets a command for executing in another test.
func executeAndResetCommand(ctx context.Context, cmd *cobra.Command, args []string) (string, string, error) {
	beforeOut, beforeErr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	defer func() {
		cmd.SetOut(beforeOut)
		cmd.SetErr(beforeErr)
		// also have to remove completion commands which get auto added and bound to an output buffer
		for _, command := range cmd.Commands() {
			if command.Name() == "completion" {
				cmd.RemoveCommand(command)
				break
			}
		}
	}()

	nowOut, nowErr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(nowOut)
	cmd.SetErr(nowErr)
	cmd.SetArgs(args)
	subCmd, err := cmd.ExecuteContextC(ctx)
	if subCmd != nil {
		subCmd.SetOut(nil)
		subCmd.SetErr(nil)
		subCmd.SetContext(nil)
		subCmd.SilenceUsage = false
		subCmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() == "stringArray" {
				_ = f.Value.(pflag.SliceValue).Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
		})
	}
	return nowOut.String(), nowErr.String(), err
}

func TestRootVersion(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"--version"})
	assert.NoError(t, err)
	pattern := regexp.MustCompile(`^score-k8s 0.0.0 \(build: \S+, sha: \S+\)\n$`)
	assert.Truef(t, pattern.MatchString(stdout), "%s does not match: '%s'", pattern.String(), stdout)
	assert.Equal(t, "", stderr)
}

func TestRootCompletion(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"completion"})
	assert.NoError(t, err)
	assert.Equal(t, `Generate the autocompletion script for score-k8s for the specified shell.
See each sub-command's help for details on how to use the generated script.

Usage:
  score-k8s completion [command]

Available Commands:
  bash        Generate the autocompletion script for bash
  fish        Generate the autocompletion script for fish
  powershell  Generate the autocompletion script for powershell
  zsh         Generate the autocompletion script for zsh

Flags:
  -h, --help   help for completion

Global Flags:
      --quiet           Mute any logging output
  -v, --verbose count   Increase log verbosity and detail by specifying this flag one or more times

Use "score-k8s completion [command] --help" for more information about a command.
`, stdout)
	assert.Equal(t, "", stderr)

	stdout2, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"help", "completion"})
	assert.NoError(t, err)
	assert.Equal(t, stdout, stdout2)
	assert.Equal(t, "", stderr)
}

func TestRootCompletionBashHelp(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"completion", "bash", "--help"})
	assert.NoError(t, err)
	assert.Equal(t, `Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(score-k8s completion bash)

To load completions for every new session, execute once:

#### Linux:

	score-k8s completion bash > /etc/bash_completion.d/score-k8s

#### macOS:

	score-k8s completion bash > $(brew --prefix)/etc/bash_completion.d/score-k8s

You will need to start a new shell for this setup to take effect.

Usage:
  score-k8s completion bash

Flags:
  -h, --help              help for bash
      --no-descriptions   disable completion descriptions

Global Flags:
      --quiet           Mute any logging output
  -v, --verbose count   Increase log verbosity and detail by specifying this flag one or more times
`, stdout)
	assert.Equal(t, "", stderr)
}

func TestRootCompletionBash(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"completion", "bash"})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "# bash completion V2 for score-k8s")
	assert.Equal(t, "", stderr)
}

func TestRootCompletionFishHelp(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"completion", "fish", "--help"})
	assert.NoError(t, err)
	assert.Equal(t, `Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	score-k8s completion fish | source

To load completions for every new session, execute once:

	score-k8s completion fish > ~/.config/fish/completions/score-k8s.fish

You will need to start a new shell for this setup to take effect.

Usage:
  score-k8s completion fish [flags]

Flags:
  -h, --help              help for fish
      --no-descriptions   disable completion descriptions

Global Flags:
      --quiet           Mute any logging output
  -v, --verbose count   Increase log verbosity and detail by specifying this flag one or more times
`, stdout)
	assert.Equal(t, "", stderr)
}

func TestRootCompletionFish(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"completion", "fish"})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "# fish completion for score-k8s")
	assert.Equal(t, "", stderr)
}

func TestRootCompletionZshHelp(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"completion", "zsh", "--help"})
	assert.NoError(t, err)
	assert.Equal(t, `Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(score-k8s completion zsh)

To load completions for every new session, execute once:

#### Linux:

	score-k8s completion zsh > "${fpath[1]}/_score-k8s"

#### macOS:

	score-k8s completion zsh > $(brew --prefix)/share/zsh/site-functions/_score-k8s

You will need to start a new shell for this setup to take effect.

Usage:
  score-k8s completion zsh [flags]

Flags:
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions

Global Flags:
      --quiet           Mute any logging output
  -v, --verbose count   Increase log verbosity and detail by specifying this flag one or more times
`, stdout)
	assert.Equal(t, "", stderr)
}

func TestRootCompletionZsh(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"completion", "zsh"})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "# zsh completion for score-k8s")
	assert.Equal(t, "", stderr)
}

func TestRootCompletionPowershellHelp(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"completion", "powershell", "--help"})
	assert.NoError(t, err)
	assert.Equal(t, `Generate the autocompletion script for powershell.

To load completions in your current shell session:

	score-k8s completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.

Usage:
  score-k8s completion powershell [flags]

Flags:
  -h, --help              help for powershell
      --no-descriptions   disable completion descriptions

Global Flags:
      --quiet           Mute any logging output
  -v, --verbose count   Increase log verbosity and detail by specifying this flag one or more times
`, stdout)
	assert.Equal(t, "", stderr)
}

func TestRootCompletionPowershell(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"completion", "powershell"})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "# powershell completion for score-k8s")
	assert.Equal(t, "", stderr)
}

func TestRootUnknown(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"unknown"})
	assert.EqualError(t, err, "unknown command \"unknown\" for \"score-k8s\"")
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

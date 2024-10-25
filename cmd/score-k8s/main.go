package score_k8s

import (
	"fmt"
	"os"

	"github.com/score-spec/score-k8s/internal/command"
)

func main() {
	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		os.Exit(1)
	}
}

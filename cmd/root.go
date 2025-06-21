package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	BuildTime string
	CommitID  string
)

var rootCmd = &cobra.Command{
	Use:     "distninja",
	Short:   "distributed build system",
	Version: BuildTime + "-" + CommitID,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

// nolint:gochecknoinits
func init() {
	cobra.OnInitialize()

	rootCmd.Root().CompletionOptions.DisableDefaultCmd = true
}

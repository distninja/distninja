package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/distninja/distninja/http"
	"github.com/distninja/distninja/rpc"
)

var (
	BuildTime string
	CommitID  string
)

var (
	grpcServe string
	httpServe string
)

var rootCmd = &cobra.Command{
	Use:     "distninja",
	Short:   "A distributed build system",
	Version: BuildTime + "-" + CommitID,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		if err := run(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	},
}

// nolint:gochecknoinits
func init() {
	cobra.OnInitialize()

	rootCmd.Flags().StringVarP(&grpcServe, "grpc-serve", "", "", "Run in grpc serve mode")
	rootCmd.Flags().StringVarP(&httpServe, "http-serve", "", "", "Run in http serve mode")

	rootCmd.MarkFlagsOneRequired("grpc-serve", "http-serve")
	rootCmd.MarkFlagsMutuallyExclusive("grpc-serve", "http-serve")

	rootCmd.Root().CompletionOptions.DisableDefaultCmd = true
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(_ context.Context) error {
	if grpcServe != "" {
		return rpc.StartServer(grpcServe)
	}

	if httpServe != "" {
		return http.StartServer(httpServe)
	}

	return errors.New("--grpc-serve or --http-serve is required")
}

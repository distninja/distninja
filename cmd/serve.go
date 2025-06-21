package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	grpcAddress string
	httpAddress string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run api server",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		if err := runServe(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	},
}

// nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.PersistentFlags().StringVarP(&grpcAddress, "grpc", "g", ":9090", "grpc address")
	serveCmd.PersistentFlags().StringVarP(&httpAddress, "http", "t", ":8080", "http address")

	serveCmd.MarkFlagsOneRequired("grpc", "http")
	serveCmd.MarkFlagsMutuallyExclusive("grpc", "http")
}

func runServe(ctx context.Context) error {
	return nil
}

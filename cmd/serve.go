package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/distninja/distninja/server"
	"github.com/distninja/distninja/utils"
)

var (
	grpcAddress string
	httpAddress string
	storePath   string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run api server",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		_path := utils.ExpandTilde(storePath)
		if _, err := os.Stat(_path); err == nil {
			if entries, err := os.ReadDir(_path); err == nil && len(entries) > 0 {
				_, _ = fmt.Fprintln(os.Stderr, "store path contains files:", storePath)
				os.Exit(1)
			}
		}
		if err := runServe(ctx, _path); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	},
}

// nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.PersistentFlags().StringVarP(&grpcAddress, "grpc", "g", "", "grpc address")
	serveCmd.PersistentFlags().StringVarP(&httpAddress, "http", "t", "", "http address")
	serveCmd.PersistentFlags().StringVarP(&storePath, "store", "s", "ninja.db", "store path")

	serveCmd.MarkFlagsOneRequired("grpc", "http")
	serveCmd.MarkFlagsMutuallyExclusive("grpc", "http")
}

func runServe(ctx context.Context, _path string) error {
	if grpcAddress != "" {
		fmt.Printf("Starting gRPC server on %s\n", grpcAddress)
		return server.StartGRPCServer(ctx, grpcAddress, _path)
	}

	if httpAddress != "" {
		fmt.Printf("Starting HTTP server on %s\n", httpAddress)
		return server.StartHTTPServer(ctx, httpAddress, _path)
	}

	fmt.Printf("Starting HTTP server on %s\n", httpAddress)

	return server.StartHTTPServer(ctx, httpAddress, _path)
}

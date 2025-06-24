package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

type DistNinjaService struct {
	// Add your service fields here
}

func StartGRPCServer(ctx context.Context, address, _store string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	server := grpc.NewServer(
		grpc.UnaryInterceptor(loggingInterceptor),
	)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	_ = &DistNinjaService{}

	// TODO: Register your proto service here
	// pb.RegisterDistNinjaServiceServer(server, distNinjaService)

	reflection.Register(server)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)

	go func() {
		if err := server.Serve(listener); err != nil {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
	case <-quit:
	case err := <-serverErr:
		return fmt.Errorf("gRPC server error: %w", err)
	}

	server.GracefulStop()

	return nil
}

func loggingInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	fmt.Printf("gRPC request: %s\n", info.FullMethod)

	resp, err := handler(ctx, req)
	if err != nil {
		fmt.Printf("gRPC error: %v\n", err)
	}

	return resp, err
}

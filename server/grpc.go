package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cayleygraph/quad"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/distninja/distninja/parser"
	"github.com/distninja/distninja/server/proto"
	"github.com/distninja/distninja/store"
)

type DistNinjaService struct {
	proto.UnimplementedDistNinjaServiceServer
	store *store.NinjaStore
}

func StartGRPCServer(ctx context.Context, address, storeDir string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	server := grpc.NewServer(
		grpc.UnaryInterceptor(loggingInterceptor),
	)

	// Initialize store
	ninjaStore, err := store.NewNinjaStore(storeDir)
	if err != nil {
		return fmt.Errorf("failed to initialize ninja store: %w", err)
	}

	// Register services
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	distNinjaService := &DistNinjaService{
		store: ninjaStore,
	}
	proto.RegisterDistNinjaServiceServer(server, distNinjaService)

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
	_ = ninjaStore.Close()

	return nil
}

// Admin methods
func (s *DistNinjaService) Health(ctx context.Context, req *proto.HealthRequest) (*proto.HealthResponse, error) {
	return &proto.HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

func (s *DistNinjaService) Status(ctx context.Context, req *proto.StatusRequest) (*proto.StatusResponse, error) {
	return &proto.StatusResponse{
		Service: "distninja",
		Uptime:  time.Since(time.Now()).String(), // This would normally be calculated from start time
	}, nil
}

// Build methods
func (s *DistNinjaService) CreateBuild(ctx context.Context, req *proto.CreateBuildRequest) (*proto.CreateBuildResponse, error) {
	build := &store.NinjaBuild{
		BuildID: req.BuildId,
		Pool:    req.Pool,
	}

	if req.Rule != "" {
		build.Rule = quad.IRI(fmt.Sprintf("rule:%s", req.Rule))
	}

	if err := build.SetVariables(req.Variables); err != nil {
		return nil, fmt.Errorf("failed to set variables: %w", err)
	}

	if err := s.store.AddBuild(build, req.Inputs, req.Outputs, req.ImplicitDeps, req.OrderDeps); err != nil {
		return nil, fmt.Errorf("failed to create build: %w", err)
	}

	return &proto.CreateBuildResponse{
		Status:  "created",
		BuildId: req.BuildId,
	}, nil
}

func (s *DistNinjaService) GetBuild(ctx context.Context, req *proto.GetBuildRequest) (*proto.NinjaBuild, error) {
	build, err := s.store.GetBuild(req.Id)
	if err != nil {
		return nil, fmt.Errorf("build not found: %w", err)
	}

	return &proto.NinjaBuild{
		Id:        string(build.ID),
		Type:      string(build.Type),
		BuildId:   build.BuildID,
		Rule:      string(build.Rule),
		Variables: build.Variables,
		Pool:      build.Pool,
	}, nil
}

func (s *DistNinjaService) GetBuildStats(ctx context.Context, req *proto.BuildStatsRequest) (*proto.BuildStatsResponse, error) {
	stats, err := s.store.GetBuildStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get build stats: %w", err)
	}

	// Convert map[string]interface{} to map[string]int64
	protoStats := make(map[string]int64)
	for k, v := range stats {
		if intVal, ok := v.(int); ok {
			protoStats[k] = int64(intVal)
		} else if int64Val, ok := v.(int64); ok {
			protoStats[k] = int64Val
		}
	}

	return &proto.BuildStatsResponse{
		Stats: protoStats,
	}, nil
}

func (s *DistNinjaService) GetBuildOrder(ctx context.Context, req *proto.BuildOrderRequest) (*proto.BuildOrderResponse, error) {
	order, err := s.store.GetBuildOrder()
	if err != nil {
		return nil, fmt.Errorf("failed to get build order: %w", err)
	}

	return &proto.BuildOrderResponse{
		BuildOrder: order,
	}, nil
}

// Rule methods
func (s *DistNinjaService) CreateRule(ctx context.Context, req *proto.CreateRuleRequest) (*proto.CreateRuleResponse, error) {
	rule := &store.NinjaRule{
		Name:        req.Name,
		Command:     req.Command,
		Description: req.Description,
	}

	if err := rule.SetVariables(req.Variables); err != nil {
		return nil, fmt.Errorf("failed to set variables: %w", err)
	}

	if _, err := s.store.AddRule(rule); err != nil {
		return nil, fmt.Errorf("failed to create rule: %w", err)
	}

	return &proto.CreateRuleResponse{
		Status: "created",
		Name:   req.Name,
	}, nil
}

func (s *DistNinjaService) GetRule(ctx context.Context, req *proto.GetRuleRequest) (*proto.NinjaRule, error) {
	rule, err := s.store.GetRule(req.Name)
	if err != nil {
		return nil, fmt.Errorf("rule not found: %w", err)
	}

	return &proto.NinjaRule{
		Id:          string(rule.ID),
		Type:        string(rule.Type),
		Name:        rule.Name,
		Command:     rule.Command,
		Description: rule.Description,
		Variables:   rule.Variables,
	}, nil
}

func (s *DistNinjaService) GetTargetsByRule(ctx context.Context, req *proto.GetTargetsByRuleRequest) (*proto.GetTargetsByRuleResponse, error) {
	targets, err := s.store.GetTargetsByRule(req.RuleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get targets by rule: %w", err)
	}

	var protoTargets []*proto.NinjaTarget
	for _, target := range targets {
		protoTargets = append(protoTargets, &proto.NinjaTarget{
			Id:     string(target.ID),
			Type:   string(target.Type),
			Path:   target.Path,
			Status: target.Status,
			Hash:   target.Hash,
			Build:  string(target.Build),
		})
	}

	return &proto.GetTargetsByRuleResponse{
		Targets: protoTargets,
	}, nil
}

// Target methods
func (s *DistNinjaService) GetAllTargets(ctx context.Context, req *proto.GetAllTargetsRequest) (*proto.GetAllTargetsResponse, error) {
	targets, err := s.store.GetAllTargets()
	if err != nil {
		return nil, fmt.Errorf("failed to get all targets: %w", err)
	}

	var protoTargets []*proto.NinjaTarget
	for _, target := range targets {
		protoTargets = append(protoTargets, &proto.NinjaTarget{
			Id:     string(target.ID),
			Type:   string(target.Type),
			Path:   target.Path,
			Status: target.Status,
			Hash:   target.Hash,
			Build:  string(target.Build),
		})
	}

	return &proto.GetAllTargetsResponse{
		Targets: protoTargets,
	}, nil
}

func (s *DistNinjaService) GetTarget(ctx context.Context, req *proto.GetTargetRequest) (*proto.NinjaTarget, error) {
	target, err := s.store.GetTarget(req.Path)
	if err != nil {
		return nil, fmt.Errorf("target not found: %w", err)
	}

	return &proto.NinjaTarget{
		Id:     string(target.ID),
		Type:   string(target.Type),
		Path:   target.Path,
		Status: target.Status,
		Hash:   target.Hash,
		Build:  string(target.Build),
	}, nil
}

func (s *DistNinjaService) GetTargetDependencies(ctx context.Context, req *proto.GetTargetDependenciesRequest) (*proto.GetTargetDependenciesResponse, error) {
	dependencies, err := s.store.GetBuildDependencies(req.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get target dependencies: %w", err)
	}

	var protoDeps []*proto.NinjaFile
	for _, dep := range dependencies {
		protoDeps = append(protoDeps, &proto.NinjaFile{
			Id:       string(dep.ID),
			Type:     string(dep.Type),
			Path:     dep.Path,
			FileType: dep.FileType,
		})
	}

	return &proto.GetTargetDependenciesResponse{
		Dependencies: protoDeps,
	}, nil
}

func (s *DistNinjaService) GetTargetReverseDependencies(ctx context.Context, req *proto.GetTargetReverseDependenciesRequest) (*proto.GetTargetReverseDependenciesResponse, error) {
	reverseDeps, err := s.store.GetReverseDependencies(req.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get reverse dependencies: %w", err)
	}

	var protoTargets []*proto.NinjaTarget
	for _, target := range reverseDeps {
		protoTargets = append(protoTargets, &proto.NinjaTarget{
			Id:     string(target.ID),
			Type:   string(target.Type),
			Path:   target.Path,
			Status: target.Status,
			Hash:   target.Hash,
			Build:  string(target.Build),
		})
	}

	return &proto.GetTargetReverseDependenciesResponse{
		ReverseDependencies: protoTargets,
	}, nil
}

func (s *DistNinjaService) UpdateTargetStatus(ctx context.Context, req *proto.UpdateTargetStatusRequest) (*proto.UpdateTargetStatusResponse, error) {
	if req.Status == "" {
		return nil, fmt.Errorf("status field is required")
	}

	// Check if target exists
	if _, err := s.store.GetTarget(req.Path); err != nil {
		return nil, fmt.Errorf("target not found: %w", err)
	}

	if err := s.store.UpdateTargetStatus(req.Path, req.Status); err != nil {
		return nil, fmt.Errorf("failed to update target status: %w", err)
	}

	return &proto.UpdateTargetStatusResponse{
		Status: "updated",
	}, nil
}

// Analysis methods
func (s *DistNinjaService) FindCycles(ctx context.Context, req *proto.FindCyclesRequest) (*proto.FindCyclesResponse, error) {
	cycles, err := s.store.FindCycles()
	if err != nil {
		return nil, fmt.Errorf("failed to find cycles: %w", err)
	}

	var protoCycles []*proto.Cycle
	for _, cycle := range cycles {
		protoCycles = append(protoCycles, &proto.Cycle{
			Nodes: cycle,
		})
	}

	return &proto.FindCyclesResponse{
		Cycles:     protoCycles,
		CycleCount: int32(len(cycles)),
	}, nil
}

// Debug methods
func (s *DistNinjaService) DebugQuads(ctx context.Context, req *proto.DebugQuadsRequest) (*proto.DebugQuadsResponse, error) {
	// Call the debug function which prints to stdout
	if err := s.store.DebugQuads(); err != nil {
		return nil, fmt.Errorf("failed to debug quads: %w", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	return &proto.DebugQuadsResponse{
		Message: "Debug endpoint - check server logs for quad dump",
		Limit:   limit,
	}, nil
}

// Load methods
func (s *DistNinjaService) LoadNinjaFile(ctx context.Context, req *proto.LoadNinjaFileRequest) (*proto.LoadNinjaFileResponse, error) {
	startTime := time.Now()

	// Check if neither file_path nor content field were provided
	if req.FilePath == "" && req.Content == "" {
		return nil, fmt.Errorf("either file_path or content must be provided")
	}

	var content string
	var err error

	// Read file content if file_path is provided
	if req.FilePath != "" {
		contentBytes, err := os.ReadFile(req.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", req.FilePath, err)
		}
		content = string(contentBytes)
	} else {
		content = req.Content
	}

	// Parse and load the Ninja file
	ninjaParser := parser.NewNinjaParser(s.store)
	err = ninjaParser.ParseAndLoad(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse and load Ninja file: %w", err)
	}

	// Get statistics after loading
	stats, err := s.store.GetBuildStats()
	if err != nil {
		// Log the error but don't fail the request
		fmt.Printf("Warning: Failed to get build stats: %v\n", err)
		stats = map[string]interface{}{"error": "stats unavailable"}
	}

	buildTime := time.Since(startTime)

	// Convert stats to protobuf format
	protoStats := make(map[string]int64)
	for k, v := range stats {
		if intVal, ok := v.(int); ok {
			protoStats[k] = int64(intVal)
		} else if int64Val, ok := v.(int64); ok {
			protoStats[k] = int64Val
		}
	}

	return &proto.LoadNinjaFileResponse{
		Status:    "success",
		Message:   "Ninja file loaded successfully",
		Stats:     protoStats,
		BuildTime: buildTime.String(),
	}, nil
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

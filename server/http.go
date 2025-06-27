package server

import (
	"context"
	"encoding/json"
	_errors "errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/cayleygraph/quad"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/distninja/distninja/parser"
	"github.com/distninja/distninja/store"
)

const (
	httpIdleTimeout  = 60 * time.Second
	httpReadTimeout  = 15 * time.Second
	httpWriteTimeout = 15 * time.Second
)

var (
	ninjaStore *store.NinjaStore
)

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

type LoadNinjaRequest struct {
	FilePath string  `json:"file_path"`
	Content  *string `json:"content,omitempty"`
}

type LoadNinjaResponse struct {
	Status    string                 `json:"status"`
	Message   string                 `json:"message"`
	Stats     map[string]interface{} `json:"stats,omitempty"`
	BuildTime string                 `json:"build_time"`
}

func StartHTTPServer(ctx context.Context, address, _store string) error {
	var err error

	ninjaStore, err = store.NewNinjaStore(_store)
	if err != nil {
		return errors.Wrap(err, "failed to open ninja store\n")
	}

	router := mux.NewRouter()

	// Admin endpoints
	router.HandleFunc("/health", healthHandler).Methods("GET")
	v1 := router.PathPrefix("/api/v1").Subrouter()
	v1.HandleFunc("/status", statusHandler).Methods("GET")

	// Build endpoints
	v1.HandleFunc("/builds", createBuildHandler).Methods("POST")
	v1.HandleFunc("/builds", optionsHandler).Methods("OPTIONS")
	v1.HandleFunc("/builds/stats", getBuildStatsHandler).Methods("GET")
	v1.HandleFunc("/builds/order", getBuildOrderHandler).Methods("GET")
	v1.HandleFunc("/builds/{id}", getBuildHandler).Methods("GET")

	// Rule endpoints
	v1.HandleFunc("/rules", createRuleHandler).Methods("POST")
	v1.HandleFunc("/rules", optionsHandler).Methods("OPTIONS")
	v1.HandleFunc("/rules/{name}/targets", getTargetsByRuleHandler).Methods("GET")
	v1.HandleFunc("/rules/{name}", getRuleHandler).Methods("GET")

	// Target endpoints
	v1.HandleFunc("/targets", getAllTargetsHandler).Methods("GET")
	v1.HandleFunc("/targets/{path:.*}/dependencies", getTargetDependenciesHandler).Methods("GET")
	v1.HandleFunc("/targets/{path:.*}/reverse_dependencies", getTargetReverseDependenciesHandler).Methods("GET")
	v1.HandleFunc("/targets/{path:.*}/status", updateTargetStatusHandler).Methods("PUT")
	v1.HandleFunc("/targets/{path:.*}/status", optionsHandler).Methods("OPTIONS")
	v1.HandleFunc("/targets/{path:.*}", getTargetHandler).Methods("GET")

	// Analysis endpoints
	v1.HandleFunc("/analysis/cycles", findCyclesHandler).Methods("GET")

	// Debug endpoints
	v1.HandleFunc("/debug/quads", debugQuadsHandler).Methods("GET")

	// Load endpoint
	v1.HandleFunc("/load", loadNinjaFileHandler).Methods("POST")
	v1.HandleFunc("/load", optionsHandler).Methods("OPTIONS")

	router.Use(corsMiddleware)

	server := &http.Server{
		Addr:         address,
		Handler:      router,
		ReadTimeout:  httpReadTimeout,
		WriteTimeout: httpWriteTimeout,
		IdleTimeout:  httpIdleTimeout,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)

	go func() {
		if err := server.ListenAndServe(); err != nil {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
	case <-quit:
	case err := <-serverErr:
		if !_errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}

	_ = server.Shutdown(ctx)

	return nil
}

func loadNinjaFileHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	var req LoadNinjaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}

	// Check if neither file_path nor content field were provided
	if req.FilePath == "" && req.Content == nil {
		writeError(w, "Either file_path or content must be provided", http.StatusBadRequest)
		return
	}

	var content string
	var err error

	// Read file content if file_path is provided
	if req.FilePath != "" {
		contentBytes, err := os.ReadFile(req.FilePath)
		if err != nil {
			writeError(w, fmt.Sprintf("Failed to read file %s: %v", req.FilePath, err), http.StatusBadRequest)
			return
		}
		content = string(contentBytes)
	} else if req.Content != nil {
		content = *req.Content
	}

	// Use the shared parser
	ninjaParser := parser.NewNinjaParser(ninjaStore)
	err = ninjaParser.ParseAndLoad(content)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to parse and load Ninja file: %v", err), http.StatusInternalServerError)
		return
	}

	// Get statistics after loading
	stats, err := ninjaStore.GetBuildStats()
	if err != nil {
		// Log the error but don't fail the request
		fmt.Printf("Warning: Failed to get build stats: %v\n", err)
		stats = map[string]interface{}{"error": "stats unavailable"}
	}

	buildTime := time.Since(startTime)

	response := LoadNinjaResponse{
		Status:    "success",
		Message:   "Ninja file loaded successfully",
		Stats:     stats,
		BuildTime: buildTime.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(response)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service": "distninja",
		"uptime":  time.Since(time.Now()).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(response)
}

func createBuildHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BuildID      string            `json:"build_id"`
		Rule         string            `json:"rule"`
		Variables    map[string]string `json:"variables,omitempty"`
		Pool         string            `json:"pool,omitempty"`
		Inputs       []string          `json:"inputs"`
		Outputs      []string          `json:"outputs"`
		ImplicitDeps []string          `json:"implicit_deps,omitempty"`
		OrderDeps    []string          `json:"order_deps,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	build := &store.NinjaBuild{
		BuildID: req.BuildID,
		Rule:    quad.IRI(fmt.Sprintf("rule:%s", req.Rule)),
		Pool:    req.Pool,
	}

	if err := build.SetVariables(req.Variables); err != nil {
		writeError(w, "Failed to set variables", http.StatusBadRequest)
		return
	}

	if err := ninjaStore.AddBuild(build, req.Inputs, req.Outputs, req.ImplicitDeps, req.OrderDeps); err != nil {
		writeError(w, fmt.Sprintf("Failed to create build: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "created", "build_id": req.BuildID})
}

func getBuildHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	buildID := vars["id"]

	build, err := ninjaStore.GetBuild(buildID)
	if err != nil {
		writeError(w, fmt.Sprintf("Build not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(build)
}

func getBuildStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := ninjaStore.GetBuildStats()
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to get stats: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func getBuildOrderHandler(w http.ResponseWriter, r *http.Request) {
	order, err := ninjaStore.GetBuildOrder()
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to get build order: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string][]string{"build_order": order})
}

func createRuleHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string            `json:"name"`
		Command     string            `json:"command"`
		Description string            `json:"description,omitempty"`
		Variables   map[string]string `json:"variables,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	rule := &store.NinjaRule{
		Name:        req.Name,
		Command:     req.Command,
		Description: req.Description,
	}

	if err := rule.SetVariables(req.Variables); err != nil {
		writeError(w, "Failed to set variables", http.StatusBadRequest)
		return
	}

	_, err := ninjaStore.AddRule(rule)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to create rule: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "created", "name": req.Name})
}

func getRuleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ruleName := vars["name"]

	rule, err := ninjaStore.GetRule(ruleName)
	if err != nil {
		writeError(w, fmt.Sprintf("Rule not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rule)
}

func getTargetsByRuleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ruleName := vars["name"]

	targets, err := ninjaStore.GetTargetsByRule(ruleName)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to get targets by rule: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(targets)
}

func getAllTargetsHandler(w http.ResponseWriter, r *http.Request) {
	targets, err := ninjaStore.GetAllTargets()
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to get targets: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(targets)
}

func getTargetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetPath := vars["path"]

	target, err := ninjaStore.GetTarget(targetPath)
	if err != nil {
		writeError(w, fmt.Sprintf("Target not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(target)
}

func getTargetDependenciesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetPath := vars["path"]

	dependencies, err := ninjaStore.GetBuildDependencies(targetPath)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to get dependencies: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dependencies)
}

func getTargetReverseDependenciesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetPath := vars["path"]

	reverseDependencies, err := ninjaStore.GetReverseDependencies(targetPath)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to get reverse dependencies: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reverseDependencies)
}

func updateTargetStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetPath := vars["path"]

	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Status == "" {
		writeError(w, "Status field is required", http.StatusBadRequest)
		return
	}

	if _, err := ninjaStore.GetTarget(targetPath); err != nil {
		writeError(w, "Target not found", http.StatusNotFound)
		return
	}

	if err := ninjaStore.UpdateTargetStatus(targetPath, req.Status); err != nil {
		writeError(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func findCyclesHandler(w http.ResponseWriter, r *http.Request) {
	cycles, err := ninjaStore.FindCycles()
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to find cycles: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"cycles":      cycles,
		"cycle_count": len(cycles),
	})
}

func debugQuadsHandler(w http.ResponseWriter, r *http.Request) {
	// Get limit parameter
	limitStr := r.URL.Query().Get("limit")
	limit := 100 // default limit
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// This would need to be implemented in the store to return quad data
	// For now, just return a placeholder
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Debug endpoint - check server logs for quad dump",
		"limit":   limit,
	})

	// Call the debug function which prints to stdout
	_ = ninjaStore.DebugQuads()
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func optionsHandler(w http.ResponseWriter, r *http.Request) {
	// CORS headers are already set by the corsMiddleware
	w.WriteHeader(http.StatusOK)
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: message,
		Code:  code,
	})
}

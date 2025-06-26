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
	"strings"
	"syscall"
	"time"

	"github.com/cayleygraph/quad"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

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

type ParsedBuild struct {
	Rule         string
	Outputs      []string
	Inputs       []string
	ImplicitDeps []string
	OrderDeps    []string
	Variables    map[string]string
	Pool         string // Add pool field
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

	// Parse and load the Ninja file (even if content is empty)
	err = parseAndLoadNinjaFile(content)
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

func parseAndLoadNinjaFile(content string) error {
	lines := strings.Split(content, "\n")

	var currentRule *store.NinjaRule
	var currentBuild *ParsedBuild

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle line continuations
		for strings.HasSuffix(line, "$") && i+1 < len(lines) {
			i++
			if i < len(lines) {
				line = line[:len(line)-1] + " " + strings.TrimSpace(lines[i])
			}
		}

		// Parse rule definitions
		if strings.HasPrefix(line, "rule ") {
			// Save previous rule if exists and it's complete
			if currentRule != nil {
				if currentRule.Command == "" {
					return fmt.Errorf("rule %s is missing required command", currentRule.Name)
				}
				if _, err := ninjaStore.AddRule(currentRule); err != nil {
					return fmt.Errorf("failed to add rule %s: %w", currentRule.Name, err)
				}
			}

			ruleName := strings.TrimSpace(line[5:])
			currentRule = &store.NinjaRule{
				Name:      ruleName,
				Variables: "{}",
			}
			continue
		}

		// Parse build statements
		if strings.HasPrefix(line, "build ") {
			// Save previous rule if exists and it's complete
			if currentRule != nil {
				if currentRule.Command == "" {
					return fmt.Errorf("rule %s is missing required command", currentRule.Name)
				}
				if _, err := ninjaStore.AddRule(currentRule); err != nil {
					return fmt.Errorf("failed to add rule %s: %w", currentRule.Name, err)
				}
				currentRule = nil
			}

			// Save previous build if exists
			if currentBuild != nil {
				if err := saveBuild(currentBuild); err != nil {
					return fmt.Errorf("failed to save build: %w", err)
				}
			}

			// Parse build line: build outputs: rule inputs | implicit_deps || order_deps
			buildLine := strings.TrimSpace(line[6:]) // Remove "build "

			// Split by colon to separate outputs and rest
			colonParts := strings.SplitN(buildLine, ":", 2)
			if len(colonParts) != 2 {
				continue // Skip invalid build lines
			}

			outputs := parseFilePaths(colonParts[0])
			rest := strings.TrimSpace(colonParts[1])

			// Parse rule and dependencies
			parts := strings.Fields(rest)
			if len(parts) == 0 {
				continue // Skip if no rule specified
			}

			rule := parts[0]
			var inputs, implicitDeps, orderDeps []string

			// Join remaining parts and split by dependency separators
			if len(parts) > 1 {
				depString := strings.Join(parts[1:], " ")

				// Split by || for order dependencies
				orderParts := strings.Split(depString, "||")
				if len(orderParts) > 1 {
					orderDeps = parseFilePaths(strings.TrimSpace(orderParts[1]))
					depString = strings.TrimSpace(orderParts[0])
				}

				// Split by | for implicit dependencies
				implicitParts := strings.Split(depString, "|")
				if len(implicitParts) > 1 {
					implicitDeps = parseFilePaths(strings.TrimSpace(implicitParts[1]))
					depString = strings.TrimSpace(implicitParts[0])
				}

				// Remaining are regular inputs
				if depString != "" {
					inputs = parseFilePaths(depString)
				}
			}

			currentBuild = &ParsedBuild{
				Rule:         rule,
				Outputs:      outputs,
				Inputs:       inputs,
				ImplicitDeps: implicitDeps,
				OrderDeps:    orderDeps,
				Variables:    make(map[string]string),
				Pool:         "default", // Default pool
			}
			continue
		}

		// Handle other constructs (pools, variables, etc.) - must come before indented line parsing
		if strings.HasPrefix(line, "pool ") || strings.HasPrefix(line, "variable ") {
			// Save current rule if we're switching contexts
			if currentRule != nil {
				if currentRule.Command == "" {
					return fmt.Errorf("rule %s is missing required command", currentRule.Name)
				}
				if _, err := ninjaStore.AddRule(currentRule); err != nil {
					return fmt.Errorf("failed to add rule %s: %w", currentRule.Name, err)
				}
				currentRule = nil
			}

			// Save current build if we're switching contexts
			if currentBuild != nil {
				if err := saveBuild(currentBuild); err != nil {
					return fmt.Errorf("failed to save build: %w", err)
				}
				currentBuild = nil
			}
			// Skip pools and variables for now - could be implemented later
			continue
		}

		// Check if this is an indented line
		originalLine := lines[i] // Get the original line to check indentation
		if strings.HasPrefix(originalLine, "  ") || strings.HasPrefix(originalLine, "\t") {
			// Parse rule properties (indented lines after rule declaration)
			if currentRule != nil {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])

					switch key {
					case "command":
						currentRule.Command = value
					case "description":
						currentRule.Description = value
					default:
						// Handle custom variables
						vars, _ := currentRule.GetVariables()
						if vars == nil {
							vars = make(map[string]string)
						}
						vars[key] = value
						_ = currentRule.SetVariables(vars)
					}
				}
				continue
			}

			// Parse build variables (indented lines after build statement)
			if currentBuild != nil {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])

					if key == "pool" {
						currentBuild.Pool = value
					} else {
						currentBuild.Variables[key] = value
					}
				}
				continue
			}
		}
	}

	// Save any remaining rule or build
	if currentRule != nil {
		if currentRule.Command == "" {
			return fmt.Errorf("rule %s is missing required command", currentRule.Name)
		}
		if _, err := ninjaStore.AddRule(currentRule); err != nil {
			return fmt.Errorf("failed to add final rule %s: %w", currentRule.Name, err)
		}
	}

	if currentBuild != nil {
		if err := saveBuild(currentBuild); err != nil {
			return fmt.Errorf("failed to save final build: %w", err)
		}
	}

	return nil
}

func saveBuild(pb *ParsedBuild) error {
	if len(pb.Outputs) == 0 {
		return fmt.Errorf("build must have at least one output")
	}

	// Generate a unique build ID based on outputs
	buildID := strings.Join(pb.Outputs, ",")

	build := &store.NinjaBuild{
		BuildID: buildID,
		Rule:    quad.IRI(fmt.Sprintf("rule:%s", pb.Rule)),
		Pool:    pb.Pool, // Use the parsed pool
	}

	if err := build.SetVariables(pb.Variables); err != nil {
		return fmt.Errorf("failed to set build variables: %w", err)
	}

	return ninjaStore.AddBuild(build, pb.Inputs, pb.Outputs, pb.ImplicitDeps, pb.OrderDeps)
}

func parseFilePaths(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}

	var paths []string
	parts := strings.Fields(input)

	for _, part := range parts {
		// Handle escaped spaces and other characters
		part = strings.ReplaceAll(part, `\ `, " ")
		if part != "" {
			paths = append(paths, part)
		}
	}

	return paths
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

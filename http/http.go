package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func StartServer(serve string) error {
	port := serve
	if !strings.Contains(port, ":") {
		port = ":" + port
	}

	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/api/tasks", handleTasks)

	fmt.Printf("Starting HTTP server on %s...\n", port)

	return http.ListenAndServe(serve, nil)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]string{
		"message": "distninja http server",
	}

	_ = json.NewEncoder(w).Encode(response)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		// GET /api/tasks - return all tasks
		// TBD
	case http.MethodPost:
		// POST /api/tasks - create new task
		// TBD
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

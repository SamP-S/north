package service

import (
	"encoding/json"
	"net/http"

	"github.com/mark3labs/mcp-go/server"
)

// NewHandler builds the HTTP handler: the MCP streamable-HTTP server at /mcp
// (optionally guarded by a bearer token) and GET /health.
func NewHandler(token string) http.Handler {
	mcpServer := BuildServer()
	streamable := server.NewStreamableHTTPServer(mcpServer,
		server.WithEndpointPath("/"),
		server.WithStateLess(true),
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp/", tokenGuard(http.StripPrefix("/mcp", streamable), token))
	mux.Handle("/mcp", tokenGuard(http.StripPrefix("/mcp", streamable), token))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return mux
}

// Serve runs the MCP HTTP server on addr (e.g. "127.0.0.1:8001") until error.
func Serve(addr, token string) error {
	return http.ListenAndServe(addr, NewHandler(token))
}

// tokenGuard requires "Authorization: Bearer <token>" when a token is set.
func tokenGuard(next http.Handler, token string) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

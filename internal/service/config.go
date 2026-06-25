// Package service is the optional North MCP server.
//
// A small net/http app whose only job is to expose the board over MCP at /mcp.
// Run it on demand with `north mcp start`. There is no REST API and no
// background work — the board is plain files on disk.
package service

import (
	"os"

	"github.com/joho/godotenv"
)

// LoadToken returns the optional bearer token (MCP_TOKEN). It loads a .env file
// if present (best-effort), then reads the environment. Empty means no auth.
func LoadToken() string {
	_ = godotenv.Load() // best-effort; ignore missing .env
	return os.Getenv("MCP_TOKEN")
}

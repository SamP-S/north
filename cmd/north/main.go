// Command north is an in-repo Markdown task board CLI with an optional MCP server.
package main

import (
	"os"

	"github.com/SamP-S/north/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}

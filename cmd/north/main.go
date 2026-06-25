// Command north is an in-repo Markdown task board CLI.
package main

import (
	"os"

	"github.com/SamP-S/north/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}

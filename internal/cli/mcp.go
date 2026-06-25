package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/service"
	"github.com/spf13/cobra"
)

const mcpHost = "127.0.0.1"

func northHome() string { return filepath.Join(mustHome(), ".north") }
func pidFile() string   { return filepath.Join(northHome(), "mcp.pid") }
func logFile() string   { return filepath.Join(northHome(), "mcp.log") }

func mustHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return home
}

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "manage the on-demand MCP server",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "start", Short: "start the MCP server (detached)", Args: cobra.NoArgs, RunE: mcpStart},
		&cobra.Command{Use: "stop", Short: "stop the MCP server", Args: cobra.NoArgs, RunE: mcpStop},
		&cobra.Command{Use: "status", Short: "show MCP server status", Args: cobra.NoArgs, RunE: mcpStatus},
		&cobra.Command{Use: "run", Short: "run the MCP server in the foreground", Args: cobra.NoArgs, RunE: mcpRun},
	)
	return cmd
}

// readPID returns the running server's PID, or 0 if not running.
func readPID() int {
	data, err := os.ReadFile(pidFile())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !alive(pid) {
		return 0
	}
	return pid
}

func alive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func mcpStart(cmd *cobra.Command, _ []string) error {
	boardDir, err := board.LocateBoard("")
	if err != nil {
		return err
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		return err
	}
	if pid := readPID(); pid != 0 {
		return nerrors.Conflict(fmt.Sprintf("MCP server already running (pid %d) — `north mcp stop` first", pid))
	}
	if err := os.MkdirAll(northHome(), 0o755); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(logFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	proc := exec.Command(exe, "mcp", "run")
	proc.Env = append(os.Environ(), "NORTH_BOARD="+boardDir)
	proc.Stdout = log
	proc.Stderr = log
	proc.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := proc.Start(); err != nil {
		return err
	}
	pid := proc.Process.Pid
	if err := os.WriteFile(pidFile(), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return err
	}
	_ = proc.Process.Release()
	cmd.Printf("MCP server started (pid %d) at http://%s:%d/mcp\n", pid, mcpHost, cfg.MCPPort)
	cmd.Printf("logs: %s\n", logFile())
	return nil
}

func mcpStop(cmd *cobra.Command, _ []string) error {
	pid := readPID()
	if pid == 0 {
		_ = os.Remove(pidFile())
		cmd.Println("MCP server is not running.")
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = proc.Signal(syscall.SIGTERM)
	_ = os.Remove(pidFile())
	cmd.Printf("Stopped MCP server (pid %d).\n", pid)
	return nil
}

func mcpStatus(cmd *cobra.Command, _ []string) error {
	pid := readPID()
	if pid == 0 {
		cmd.Println("MCP server: stopped")
		return nil
	}
	boardDir, err := board.LocateBoard("")
	if err != nil {
		return err
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		return err
	}
	cmd.Printf("MCP server: running (pid %d) at http://%s:%d/mcp\n", pid, mcpHost, cfg.MCPPort)
	return nil
}

func mcpRun(cmd *cobra.Command, _ []string) error {
	boardDir, err := board.LocateBoard("")
	if err != nil {
		return err
	}
	if os.Getenv("NORTH_BOARD") == "" {
		_ = os.Setenv("NORTH_BOARD", boardDir)
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", mcpHost, cfg.MCPPort)
	return service.Serve(addr, service.LoadToken())
}

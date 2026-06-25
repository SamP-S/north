package service

import (
	"context"
	"encoding/json"
	"os"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// getBoard resolves the board directory for the running server: NORTH_BOARD
// (set by `north mcp`) takes precedence, else walk up from the working dir.
func getBoard() (string, error) {
	if env := os.Getenv("NORTH_BOARD"); env != "" {
		return env, nil
	}
	return board.LocateBoard("")
}

// toolErr translates a BoardError into a plain MCP tool error.
func toolErr(err error) (*mcp.CallToolResult, error) {
	if be, ok := nerrors.As(err); ok {
		return mcp.NewToolResultError(be.Code() + ": " + be.Message()), nil
	}
	return mcp.NewToolResultError(err.Error()), nil
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func taskSummary(t *models.Task) map[string]any {
	m := t.ToMap()
	delete(m, "body")
	return m
}

// BuildServer builds the single MCPServer with all task tools registered.
func BuildServer() *server.MCPServer {
	s := server.NewMCPServer("north", "0.1.0", server.WithInstructions("North task board."))

	s.AddTool(
		mcp.NewTool("list_tasks",
			mcp.WithDescription("List tasks (without bodies). Filter by status; include archived ones."),
			mcp.WithString("status"),
			mcp.WithBoolean("archived"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			boardDir, err := getBoard()
			if err != nil {
				return toolErr(err)
			}
			ts, err := tasks.List(boardDir, req.GetString("status", ""), req.GetBool("archived", false))
			if err != nil {
				return toolErr(err)
			}
			out := make([]map[string]any, len(ts))
			for i, t := range ts {
				out[i] = taskSummary(t)
			}
			return jsonResult(out)
		},
	)

	s.AddTool(
		mcp.NewTool("get_task",
			mcp.WithDescription("Get one task by id, including its body."),
			mcp.WithString("task_id", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			boardDir, err := getBoard()
			if err != nil {
				return toolErr(err)
			}
			id, err := req.RequireString("task_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			t, err := tasks.Get(boardDir, id)
			if err != nil {
				return toolErr(err)
			}
			return jsonResult(t.ToMap())
		},
	)

	s.AddTool(
		mcp.NewTool("create_task",
			mcp.WithDescription("Create a task (lands in draft)."),
			mcp.WithString("title", mcp.Required()),
			mcp.WithString("agent"),
			mcp.WithArray("labels", mcp.WithStringItems()),
			mcp.WithArray("depends_on", mcp.WithStringItems()),
			mcp.WithString("body"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			boardDir, err := getBoard()
			if err != nil {
				return toolErr(err)
			}
			title, err := req.RequireString("title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			t, err := tasks.Create(boardDir, title,
				req.GetString("agent", ""),
				req.GetStringSlice("labels", nil),
				req.GetStringSlice("depends_on", nil),
				req.GetString("body", ""))
			if err != nil {
				return toolErr(err)
			}
			return jsonResult(t.ToMap())
		},
	)

	s.AddTool(
		mcp.NewTool("set_task_status",
			mcp.WithDescription("Change a task's status (validates the transition)."),
			mcp.WithString("task_id", mcp.Required()),
			mcp.WithString("status", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			boardDir, err := getBoard()
			if err != nil {
				return toolErr(err)
			}
			id, err := req.RequireString("task_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			status, err := req.RequireString("status")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			t, err := tasks.Move(boardDir, id, status)
			if err != nil {
				return toolErr(err)
			}
			return jsonResult(t.ToMap())
		},
	)

	s.AddTool(
		mcp.NewTool("edit_task",
			mcp.WithDescription("Edit a task's fields and/or body."),
			mcp.WithString("task_id", mcp.Required()),
			mcp.WithString("title"),
			mcp.WithString("agent"),
			mcp.WithArray("labels", mcp.WithStringItems()),
			mcp.WithArray("depends_on", mcp.WithStringItems()),
			mcp.WithString("body"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			boardDir, err := getBoard()
			if err != nil {
				return toolErr(err)
			}
			id, err := req.RequireString("task_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			args := req.GetArguments()
			t, err := tasks.Edit(boardDir, id,
				optString(args, "title"),
				optString(args, "agent"),
				optStringSlice(req, args, "labels"),
				optStringSlice(req, args, "depends_on"),
				optString(args, "body"))
			if err != nil {
				return toolErr(err)
			}
			return jsonResult(t.ToMap())
		},
	)

	return s
}

// optString returns a *string only when the key was supplied in the request.
func optString(args map[string]any, key string) *string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return &s
		}
	}
	return nil
}

// optStringSlice returns a *[]string only when the key was supplied.
func optStringSlice(req mcp.CallToolRequest, args map[string]any, key string) *[]string {
	if _, ok := args[key]; !ok {
		return nil
	}
	s := req.GetStringSlice(key, nil)
	if s == nil {
		s = []string{}
	}
	return &s
}

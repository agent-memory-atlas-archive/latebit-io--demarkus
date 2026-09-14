package answerproxy

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/latebit-io/demarkus/tools/internal/mcpstdio"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

var legacyReadTools = map[string]bool{
	"mark_fetch": true, "mark_lookup": true, "mark_explore": true, "mark_list": true,
	"mark_backlinks": true, "mark_graph": true, "mark_versions": true, "mark_discover": true,
}

var scopedReadTools = map[string]bool{
	"mark_fetch": true, "mark_lookup": true, "mark_list": true, "mark_versions": true,
}

var versionPath = regexp.MustCompile(`^(/.*\.md)/v([1-9]\d*)$`)

type location struct {
	path string
}

// Serve preserves production schemas and handlers while restricting reads to
// one frozen endpoint and scope. This package never imports scorer data.
func Serve(ctx context.Context, cfg mcpstdio.Config, host, scope string) (err error) {
	backend, err := mcpstdio.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, backend.Close()) }()
	tools, err := backend.ListTools(ctx)
	if err != nil {
		return err
	}
	tools, err = selectReaderTools(tools, host, scope)
	if err != nil {
		return err
	}
	server := mcpserver.NewMCPServer("frozen-demarkus", "1")
	for i := range tools {
		tool := &tools[i]
		server.AddTool(*tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			raw, err := readerURL(tool.Name, args, host)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			loc, err := parseLocation(raw, host)
			if err != nil || strings.Contains(loc.path, "..") || strings.Contains(loc.path, "\\") {
				return mcp.NewToolResultError("URL outside frozen fixture scope"), nil
			}
			if !withinScope(scope, loc.path) {
				return mcp.NewToolResultError("URL outside frozen fixture scope"), nil
			}
			result, err := backend.Call(ctx, tool.Name, args)
			if err != nil {
				return nil, fmt.Errorf("fixture %s: %w", tool.Name, err)
			}
			if result.IsError {
				return mcp.NewToolResultError(result.Text), nil
			}
			return mcp.NewToolResultText(result.Text), nil
		})
	}
	stdio := mcpserver.NewStdioServer(server)
	return stdio.Listen(ctx, os.Stdin, os.Stdout)
}

func readerURL(tool string, args map[string]any, host string) (string, error) {
	raw, exists := args["url"]
	if !exists && tool == "mark_discover" {
		return "mark://" + host + "/.well-known/agent-manifest.md", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", errors.New("url is required")
	}
	return value, nil
}

func selectReaderTools(tools []mcp.Tool, host, scope string) ([]mcp.Tool, error) {
	allowed := legacyReadTools
	if scope != "" {
		if !strings.HasPrefix(scope, "/") || path.Clean(scope) != scope || strings.Contains(scope, "\\") {
			return nil, fmt.Errorf("invalid fixture scope %q", scope)
		}
		allowed = scopedReadTools
	}
	want := make(map[string]bool, len(allowed))
	for name := range allowed {
		if name != "mark_graph" || strings.HasPrefix(host, "127.0.0.1:") {
			want[name] = true
		}
	}
	selected := make([]mcp.Tool, 0, len(want))
	seen := make(map[string]bool, len(tools))
	for i := range tools {
		tool := &tools[i]
		if seen[tool.Name] {
			return nil, fmt.Errorf("production MCP listed duplicate tool %q", tool.Name)
		}
		seen[tool.Name] = true
		if want[tool.Name] {
			selected = append(selected, *tool)
		}
	}
	for name := range want {
		if !seen[name] {
			return nil, fmt.Errorf("production MCP lacks required reader tool %q", name)
		}
	}
	return selected, nil
}

// ToolProfile identifies the exact reader-visible API surface.
func ToolProfile(host, scope string) (profile string, names []string) {
	allowed := legacyReadTools
	profile = "legacy-read-v1"
	if scope != "" {
		allowed = scopedReadTools
		profile = "scoped-direct-read-v1"
	}
	names = make([]string, 0, len(allowed))
	for name := range allowed {
		if name != "mark_graph" || strings.HasPrefix(host, "127.0.0.1:") {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return profile, names
}

func parseLocation(raw, host string) (location, error) {
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery != "" || u.User != nil || (u.Host != "" && strings.TrimSuffix(u.Host, ":6309") != strings.TrimSuffix(host, ":6309")) || (u.Scheme != "" && u.Scheme != "mark") || !strings.HasPrefix(u.Path, "/") {
		return location{}, fmt.Errorf("invalid fixture location %q", raw)
	}
	docPath := u.Path
	if parts := versionPath.FindStringSubmatch(u.Path); parts != nil {
		docPath = parts[1]
		if _, err := strconv.Atoi(parts[2]); err != nil {
			return location{}, fmt.Errorf("parse version: %w", err)
		}
	}
	return location{path: docPath}, nil
}

func withinScope(scope, docPath string) bool {
	if scope == "" || scope == "/" {
		return true
	}
	return docPath == scope || strings.HasPrefix(docPath, strings.TrimSuffix(scope, "/")+"/")
}

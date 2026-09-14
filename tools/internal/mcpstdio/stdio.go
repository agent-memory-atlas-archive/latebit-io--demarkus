// Package mcpstdio owns scorer-free MCP stdio transport.
package mcpstdio

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Config describes an MCP subprocess.
type Config struct {
	Command string
	Args    []string
	Env     []string
}

// Result separates tool errors from transport errors.
type Result struct {
	Text    string
	IsError bool
}

// Session owns one initialized MCP subprocess.
type Session struct {
	client *mcpclient.Client
}

// Open starts and initializes an MCP subprocess.
func Open(ctx context.Context, cfg Config) (*Session, error) {
	c, err := mcpclient.NewStdioMCPClient(cfg.Command, cfg.Env, cfg.Args...)
	if err != nil {
		return nil, fmt.Errorf("spawn %s: %w", cfg.Command, err)
	}
	req := mcp.InitializeRequest{}
	req.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcp.Implementation{Name: "demarkus-benchmark-proxy", Version: "dev"}
	if _, err := c.Initialize(ctx, req); err != nil {
		closeErr := c.Close()
		return nil, fmt.Errorf("initialize %s: %w (close: %v)", cfg.Command, err, closeErr)
	}
	return &Session{client: c}, nil
}

// Call invokes one MCP tool.
func (s *Session) Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := s.client.CallTool(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("call %s: %w", name, err)
	}
	var b strings.Builder
	for i, content := range res.Content {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(mcp.GetTextFromContent(content))
	}
	return Result{Text: b.String(), IsError: res.IsError}, nil
}

// Close stops the subprocess.
func (s *Session) Close() error { return s.client.Close() }

// ListTools returns the complete paginated tool catalog.
func (s *Session) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var tools []mcp.Tool
	req := mcp.ListToolsRequest{}
	seen := make(map[mcp.Cursor]bool)
	for range 64 {
		res, err := s.client.ListToolsByPage(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("list tools: %w", err)
		}
		if res == nil {
			return nil, fmt.Errorf("list tools: missing page")
		}
		if len(res.Tools) > 4096-len(tools) {
			return nil, fmt.Errorf("list tools: limit of 4096 tools exceeded")
		}
		tools = append(tools, res.Tools...)
		if res.NextCursor == "" {
			return tools, nil
		}
		if seen[res.NextCursor] {
			return nil, fmt.Errorf("list tools: repeated cursor %q", res.NextCursor)
		}
		seen[res.NextCursor] = true
		req.Params.Cursor = res.NextCursor
	}
	return nil, fmt.Errorf("list tools: limit of 64 pages exceeded")
}

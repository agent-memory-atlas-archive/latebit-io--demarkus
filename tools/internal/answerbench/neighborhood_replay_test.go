package answerbench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/latebit-io/demarkus/tools/internal/mcpstdio"
	"github.com/latebit-io/demarkus/tools/internal/retrievalbench"
	"github.com/mark3labs/mcp-go/mcp"
)

// Explicit binaries keep this compatibility replay separate from unit tests.
func TestNeighborhoodBinaryReplay(t *testing.T) {
	before, after, server := os.Getenv("DEMARKUS_BENCH_BEFORE_MCP"), os.Getenv("DEMARKUS_BENCH_AFTER_MCP"), os.Getenv("DEMARKUS_BENCH_SERVER")
	if before == "" && after == "" && server == "" {
		t.Skip("set DEMARKUS_BENCH_BEFORE_MCP, DEMARKUS_BENCH_AFTER_MCP, DEMARKUS_BENCH_SERVER")
	}
	for name, binary := range map[string]string{"before": before, "after": after, "server": server} {
		raw, err := os.ReadFile(binary)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s=%s", name, digest(raw))
	}
	fixture, err := LoadFixture()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	root := filepath.Join(work, "corpus")
	if err := seed(root, &fixture); err != nil {
		t.Fatal(err)
	}
	if err := verifyEndpointFree(16319); err != nil {
		t.Fatal(err)
	}
	token, policy, _, err := createRunCapability(work)
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(work, "server.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := log.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := child(ctx, server, "-root", root, "-port", "16319", "-tokens", policy, "-read-only")
	cmd.Env = append(cleanEnv(os.Environ()), "DEMARKUS_LOG_FORMAT=json", "DEMARKUS_RATE_LIMIT=0")
	cmd.Stdout, cmd.Stderr = log, log
	process, err := startManaged(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := process.stop(cancel); err != nil {
			t.Error(err)
		}
		if process.UnexpectedExit() {
			t.Error("fixture server exited before owned shutdown")
		}
	}()
	const host = "127.0.0.1:16319"
	if err := verifyServerIndex(ctx, host, token, root, log.Name(), process, &fixture); err != nil {
		t.Fatal(err)
	}
	counter, err := retrievalbench.NewO200kCounter()
	if err != nil {
		t.Fatal(err)
	}
	first := replayNeighborhoodBinary(ctx, t, before, host, token)
	second := replayNeighborhoodBinary(ctx, t, after, host, token)
	if len(first) != len(second) {
		t.Fatal("replay length changed")
	}
	total := 0
	for i, output := range first {
		if output != second[i] {
			t.Fatalf("replay step %d differs:\nbefore: %s\nafter: %s", i, output, second[i])
		}
		tokens, err := counter.Count(output)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("step=%d tokens=%d hash=%s", i, tokens, digest([]byte(output)))
		if i > 0 {
			total += tokens
		}
	}
	t.Logf("identical_schemas=true identical_results=%d result_tokens_per_arm=%d", len(first)-1, total)
}

func replayNeighborhoodBinary(ctx context.Context, t *testing.T, binary, host, token string) []string {
	t.Helper()
	env := make([]string, 0, len(os.Environ())+2)
	for _, value := range cleanEnv(os.Environ()) {
		if !strings.HasPrefix(value, "HOME=") {
			env = append(env, value)
		}
	}
	env = append(env, "HOME="+t.TempDir(), "DEMARKUS_AUTH="+token)
	session, err := mcpstdio.Open(ctx, mcpstdio.Config{Command: binary, Args: []string{"-host", "mark://" + host, "-insecure", "-no-cache"}, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	tools, err := session.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(tools, func(a, b mcp.Tool) int { return strings.Compare(a.Name, b.Name) })
	schemas, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	outputs := []string{string(schemas)}
	call := func(tool string, args map[string]any) string {
		result, err := session.Call(ctx, tool, args)
		if err != nil || result.IsError {
			t.Fatalf("%s: %v %s", tool, err, result.Text)
		}
		return result.Text
	}
	for range 2 {
		outputs = append(outputs, call("mark_explore", map[string]any{"url": "/ops/restore.md", "direction": "outgoing", "relations": []string{"depends-on"}}))
	}
	call("mark_graph", map[string]any{"url": "/index.md", "depth": 2})
	for _, args := range []map[string]any{
		{"url": "/ops/audit.md", "direction": "incoming", "relations": []string{"depends-on"}},
		{"url": "/decisions/adr-009.md", "direction": "incoming", "relations": []string{"supersedes"}},
		{"url": "/index.md", "direction": "outgoing", "relations": []string{""}, "page_size": 1},
	} {
		output := call("mark_explore", args)
		outputs = append(outputs, output)
		if strings.Contains(output, "next-cursor: ") {
			cursor, _, _ := strings.Cut(strings.SplitN(output, "next-cursor: ", 2)[1], "\n")
			args["cursor"] = cursor
			outputs = append(outputs, call("mark_explore", args))
		}
	}
	outputs = append(outputs, call("mark_backlinks", map[string]any{"url": "/ops/audit.md"}))
	if len(outputs) != 8 || !strings.Contains(outputs[1], "outgoing [depends-on]") ||
		!strings.Contains(outputs[3], "incoming [depends-on]") || !strings.Contains(outputs[4], "incoming [supersedes]") ||
		!strings.Contains(outputs[7], "/ops/restore.md") {
		t.Fatal("replay did not exercise relations, pagination, and backlinks")
	}
	for page, wantPath := range []string{"/decisions/adr-009.md", "/decisions/adr-014.md"} {
		_, relations, found := strings.Cut(outputs[5+page], "\n## Relations (")
		if !found {
			t.Fatalf("page %d has no relation section", page+1)
		}
		relations, _, _ = strings.Cut(relations, "\n## ")
		var urls []string
		for line := range strings.SplitSeq(relations, "\n") {
			if !strings.HasPrefix(line, "- [") {
				continue
			}
			_, target, found := strings.Cut(line, "](")
			if !found || !strings.HasSuffix(target, ")") {
				t.Fatalf("page %d has malformed relation row %q", page+1, line)
			}
			urls = append(urls, strings.TrimSuffix(target, ")"))
		}
		want := []string{"mark://" + host + wantPath}
		if !slices.Equal(urls, want) {
			t.Fatalf("page %d relation URLs = %v, want %v", page+1, urls, want)
		}
	}
	return outputs
}

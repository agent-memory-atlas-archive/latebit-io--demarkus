package answerproxy

import (
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestScopedToolProfileExcludesNestedReaders(t *testing.T) {
	catalog := make([]mcp.Tool, 0, len(legacyReadTools)+1)
	for name := range legacyReadTools {
		catalog = append(catalog, mcp.Tool{Name: name})
	}
	catalog = append(catalog, mcp.Tool{Name: "mark_publish"})
	selected, err := selectReaderTools(catalog, "world.example", "/team")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(selected))
	for _, tool := range selected {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if got, want := strings.Join(names, ","), "mark_fetch,mark_list,mark_lookup,mark_versions"; got != want {
		t.Fatalf("scoped tools=%s, want %s", got, want)
	}
	if withinScope("/team", "/other/answer-key.md") || !withinScope("/team", "/team/docs/a.md") {
		t.Fatal("scope boundary accepted escape or rejected child")
	}
}

func TestLegacyToolProfilePreservesGraphOnlyForLocalFixture(t *testing.T) {
	catalog := make([]mcp.Tool, 0, len(legacyReadTools))
	for name := range legacyReadTools {
		catalog = append(catalog, mcp.Tool{Name: name})
	}
	for _, tc := range []struct {
		host      string
		wantGraph bool
	}{{"127.0.0.1:16319", true}, {"soul.example", false}} {
		selected, err := selectReaderTools(catalog, tc.host, "")
		if err != nil {
			t.Fatal(err)
		}
		gotGraph := false
		for _, tool := range selected {
			gotGraph = gotGraph || tool.Name == "mark_graph"
		}
		if gotGraph != tc.wantGraph {
			t.Fatalf("host=%s graph=%t, want %t", tc.host, gotGraph, tc.wantGraph)
		}
	}
}

func TestToolProfileRejectsMissingDuplicateAndInvalidScope(t *testing.T) {
	complete := make([]mcp.Tool, 0, len(scopedReadTools))
	for name := range scopedReadTools {
		complete = append(complete, mcp.Tool{Name: name})
	}
	for _, tc := range []struct {
		name  string
		tools []mcp.Tool
		scope string
	}{
		{"missing", complete[:len(complete)-1], "/"},
		{"duplicate", append(complete, complete[0]), "/"},
		{"scope", complete, "/team/../private"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := selectReaderTools(tc.tools, "world.example", tc.scope); err == nil {
				t.Fatal("invalid tool profile accepted")
			}
		})
	}
}

func TestReaderURLDefaultsOnlyDiscover(t *testing.T) {
	got, err := readerURL("mark_discover", map[string]any{}, "world.example")
	if err != nil || got != "mark://world.example/.well-known/agent-manifest.md" {
		t.Fatalf("discover url=%q err=%v", got, err)
	}
	if _, err := readerURL("mark_fetch", map[string]any{}, "world.example"); err == nil {
		t.Fatal("fetch accepted missing URL")
	}
	if _, err := readerURL("mark_discover", map[string]any{"url": 42}, "world.example"); err == nil {
		t.Fatal("discover accepted invalid URL type")
	}
}

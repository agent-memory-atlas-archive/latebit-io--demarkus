package answerbench

import (
	"maps"
	"testing"
	"time"
)

func TestReportV2RequiresAttributedLifecycleForCompleteUsage(t *testing.T) {
	report := Report{
		Format:    reportFormatV2,
		Generated: time.Now(),
		Spec: RunSpec{Suite: "test", Repeats: 1, ExpectedAttempts: 1, PromptHash: "prompt", ConfigHash: "config", ScoringVersion: scoringVersion,
			Hashes: map[string]string{"corpus": "corpus", "tasks": "tasks", "rubric": "rubric"}},
		Binaries:  map[string]string{"server": "server", "mcp": "mcp", "runner": "runner", "proxy": "proxy"},
		Lifecycle: Lifecycle{Attribution: "run-read-capability-v1", Endpoint: "127.0.0.1:16319", ToolProfile: "legacy-read-v1", ToolNames: []string{"mark_fetch"}},
		Attempts:  []Attempt{{Task: "q1", Category: "direct", Repeat: 1, Phase: "cold", Trace: Trace{UsageComplete: true}}},
	}
	report.summarize()
	if err := validateReport(&report); err == nil {
		t.Fatal("complete report without lifecycle proof accepted")
	}
	report.Lifecycle.EndpointFree = true
	report.Lifecycle.StartupVerified = true
	report.Lifecycle.ServerStayedUp = true
	report.Lifecycle.CleanupVerified = true
	if err := validateReport(&report); err != nil {
		t.Fatalf("attributed report rejected: %v", err)
	}
}

func TestReportV2RejectsDerivedAndAttemptTampering(t *testing.T) {
	report := Report{
		Format: reportFormatV2, Generated: time.Now(),
		Spec:      RunSpec{Suite: "test", Repeats: 1, ExpectedAttempts: 1, PromptHash: "prompt", ConfigHash: "config", ScoringVersion: scoringVersion, Hashes: map[string]string{"corpus": "corpus", "tasks": "tasks", "rubric": "rubric"}},
		Binaries:  map[string]string{"server": "server", "mcp": "mcp", "runner": "runner", "proxy": "proxy"},
		Lifecycle: Lifecycle{Attribution: "run-read-capability-v1", Endpoint: "127.0.0.1:16319", EndpointFree: true, StartupVerified: true, ServerStayedUp: true, CleanupVerified: true, ToolProfile: "legacy-read-v1", ToolNames: []string{"mark_fetch"}},
		Attempts:  []Attempt{{Task: "q1", Category: "direct", Repeat: 1, Phase: "cold", Trace: Trace{Usage: Usage{Input: 10}, UsageComplete: true}}},
	}
	report.summarize()
	for _, tc := range []struct {
		name   string
		mutate func(*Report)
	}{
		{"summary", func(r *Report) { r.Summary.KnownTokens++ }},
		{"category", func(r *Report) { r.Categories["direct"] = Summary{} }},
		{"duplicate", func(r *Report) { r.Attempts = append(r.Attempts, r.Attempts[0]); r.Spec.ExpectedAttempts++ }},
		{"phase", func(r *Report) { r.Attempts[0].Phase = "warm" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := report
			changed.Attempts = append([]Attempt(nil), report.Attempts...)
			changed.Categories = make(map[string]Summary, len(report.Categories))
			maps.Copy(changed.Categories, report.Categories)
			tc.mutate(&changed)
			if err := validateReport(&changed); err == nil {
				t.Fatal("tampered report accepted")
			}
		})
	}
}

func TestReportFormatRejectsUnknownEmptyAndDowngraded(t *testing.T) {
	if err := validateReport(&Report{}); err == nil {
		t.Fatal("empty historical report accepted")
	}
	if err := validateReport(&Report{Format: "future"}); err == nil {
		t.Fatal("unknown report format accepted")
	}
	if err := validateReport(&Report{Lifecycle: Lifecycle{Attribution: "run-read-capability-v1"}}); err == nil {
		t.Fatal("v2 report accepted after format downgrade")
	}
}

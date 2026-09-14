package answerbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixtureRejectsReaderScorerContractMismatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Fixture)
		want   string
	}{
		{"empty-category", func(f *Fixture) { f.Tasks[0].Category = "" }, "invalid or duplicate task"},
		{"unknown-type", func(f *Fixture) { f.Tasks[0].Fields["days"] = "integer" }, "invalid field/type"},
		{"wrong-answer-type", func(f *Fixture) { f.Tasks[0].Fields["days"] = "boolean" }, "does not match"},
		{"invalid-scope", func(f *Fixture) {
			for i := range f.Tasks {
				f.Tasks[i].Scope = "/"
			}
			f.Tasks[0].Scope = "/ops/../private"
		}, "invalid scope"},
		{"mixed-contracts", func(f *Fixture) { f.Tasks[0].Scope = "/" }, "cannot mix"},
		{"dataset-without-scope", func(f *Fixture) { f.StoreRoot = t.TempDir(); f.Dataset = &DatasetManifest{} }, "requires scoped"},
		{"evidence-escape", func(f *Fixture) {
			complete := true
			for i := range f.Tasks {
				f.Tasks[i].Scope = "/"
				r := f.Rubrics[f.Tasks[i].ID]
				r.Outcome = "answered"
				r.Completion = []Completion{{Step: "scope", Tool: "mark_lookup", URL: "/", Query: "all", Match: "body", Status: "ok", Complete: &complete}}
				f.Rubrics[f.Tasks[i].ID] = r
			}
			f.Tasks[0].Scope = "/systems"
			r := f.Rubrics[f.Tasks[0].ID]
			r.Completion[0].URL = "/systems"
			f.Rubrics[f.Tasks[0].ID] = r
		}, "escapes scope"},
		{"empty-anchor", func(f *Fixture) {
			r := f.Rubrics["q1"]
			r.Evidence["days"][0].Anchor = ""
			f.Rubrics["q1"] = r
		}, "lacks an anchor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := testFixture(t)
			tc.mutate(&f)
			if err := f.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validation error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestCopiedFixtureRejectsScorerFilesUnderServedRoot(t *testing.T) {
	root := t.TempDir()
	questions := filepath.Join(root, "questions")
	if err := os.Mkdir(questions, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadStoreFixture(t.Context(), root, questions); err == nil || !strings.Contains(err.Error(), "outside served corpus") {
		t.Fatalf("reader-accessible scorer directory error=%v", err)
	}
}

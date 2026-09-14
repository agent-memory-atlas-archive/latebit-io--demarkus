package answerbench

import (
	"encoding/json"
	"testing"
)

func TestDatasetManifestPinsEveryIndependentInput(t *testing.T) {
	fixture := Fixture{
		Documents:      []Document{{Path: "/index.md"}},
		VersionCount:   2,
		Hashes:         map[string]string{"corpus": digest([]byte("corpus")), "tasks": digest([]byte("tasks")), "rubric": digest([]byte("rubric"))},
		storedVersions: map[string]map[int]bool{"/index.md": {1: true}, "/archived.md": {1: true}},
	}
	manifest := CorpusManifest{
		Format: corpusArchiveFormat, ID: "org-1", Source: "mark://org.example", Artifact: "org.jsonl.gz",
		SHA256: digest([]byte("archive")), CorpusSHA256: fixture.Hashes["corpus"],
		CompressedBytes: 10, UncompressedBytes: 20, Documents: 2, ActiveDocuments: 1, Versions: 2,
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	dataset := DatasetManifest{
		Format: datasetFormatV1, ID: "org-1", Source: manifest.Source,
		ArchiveManifestSHA256: digest(manifestRaw), CorpusSHA256: manifest.CorpusSHA256,
		TasksSHA256: fixture.Hashes["tasks"], RubricSHA256: fixture.Hashes["rubric"],
		ScoringVersion: independentScoringVersion, ToolProfile: "scoped-direct-read-v1",
		ReaderPolicy: "section-first", Authored: "2026-09-14",
	}
	prompt, err := readerContract(dataset.ReaderPolicy)
	if err != nil {
		t.Fatal(err)
	}
	dataset.ReaderContractSHA256 = digest([]byte(prompt))
	if err := validateDataset(&dataset, &manifest, &fixture, manifestRaw); err != nil {
		t.Fatalf("valid dataset rejected: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*DatasetManifest)
	}{
		{"tasks", func(d *DatasetManifest) { d.TasksSHA256 = digest([]byte("other")) }},
		{"corpus", func(d *DatasetManifest) { d.CorpusSHA256 = digest([]byte("other")) }},
		{"manifest", func(d *DatasetManifest) { d.ArchiveManifestSHA256 = digest([]byte("other")) }},
		{"scorer", func(d *DatasetManifest) { d.ScoringVersion = scoringVersion }},
		{"profile", func(d *DatasetManifest) { d.ToolProfile = "legacy-read-v1" }},
		{"id", func(d *DatasetManifest) { d.ID = "other" }},
		{"source", func(d *DatasetManifest) { d.Source = "https://example.com" }},
		{"reader", func(d *DatasetManifest) { d.ReaderContractSHA256 = digest([]byte("other")) }},
		{"authored", func(d *DatasetManifest) { d.Authored = "soon" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := dataset
			tc.mutate(&changed)
			if err := validateDataset(&changed, &manifest, &fixture, manifestRaw); err == nil {
				t.Fatal("changed dataset input accepted")
			}
		})
	}
}

package answerbench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/latebit-io/demarkus/protocol/store"
)

// LoadStoreFixture reads an existing versioned corpus without renumbering history.
// Tasks and rubric live outside the served root.
func LoadStoreFixture(ctx context.Context, root, questions string) (Fixture, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Fixture{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Fixture{}, err
	}
	questions, err = filepath.Abs(questions)
	if err != nil {
		return Fixture{}, err
	}
	questions, err = filepath.EvalSymlinks(questions)
	if err != nil {
		return Fixture{}, err
	}
	rel, err := filepath.Rel(root, questions)
	if err != nil {
		return Fixture{}, err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return Fixture{}, fmt.Errorf("questions directory must be outside served corpus root")
	}
	f := Fixture{StoreRoot: root, Hashes: make(map[string]string), storedVersions: make(map[string]map[int]bool)}
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	err = store.New(root).ExportDocs(ctx, func(docPath string, doc store.StoredDocument) error {
		if err := encoder.Encode(corpusEntry{Path: docPath, Document: doc}); err != nil {
			return err
		}
		f.VersionCount += len(doc.Versions)
		f.storedVersions[docPath] = make(map[int]bool, len(doc.Versions))
		for _, version := range doc.Versions {
			f.storedVersions[docPath][version.Version] = true
		}
		if doc.Archived {
			return nil
		}
		current := doc.Versions[len(doc.Versions)-1]
		f.Documents = append(f.Documents, Document{Path: docPath, Current: current.Version, Metadata: store.ExtractMetadata(current.Stored), Versions: []string{string(store.ExtractBody(current.Stored))}})
		return nil
	})
	if err != nil {
		return f, fmt.Errorf("inventory copied corpus: %w", err)
	}
	f.Hashes["corpus"] = fmt.Sprintf("sha256-%x", hash.Sum(nil))
	for _, item := range []struct {
		name   string
		target any
	}{{"tasks", &f.Tasks}, {"rubric", &f.Rubrics}} {
		raw, err := readScorerFile(root, filepath.Join(questions, item.name+".json"), 8<<20)
		if err != nil {
			return f, err
		}
		if err := decodeJSON(raw, item.target); err != nil {
			return f, fmt.Errorf("decode %s: %w", item.name, err)
		}
		f.Hashes[item.name] = digest(raw)
	}
	if err := loadDataset(questions, &f); err != nil {
		return f, err
	}
	err = f.Validate()
	return f, err
}

func readScorerFile(root, file string, limit int64) ([]byte, error) {
	physical, err := filepath.EvalSymlinks(file)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, physical)
	if err != nil {
		return nil, err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return nil, fmt.Errorf("scorer file %s resolves inside served corpus root", file)
	}
	return readBounded(physical, limit)
}

// InspectStore reports snapshot identity without sending any content to a model.
func InspectStore(ctx context.Context, root, questions string) (map[string]any, error) {
	f, err := LoadStoreFixture(ctx, root, questions)
	if err != nil {
		return nil, err
	}
	return map[string]any{"documents": len(f.Documents), "versions": f.VersionCount, "tasks": len(f.Tasks), "hashes": f.Hashes}, nil
}

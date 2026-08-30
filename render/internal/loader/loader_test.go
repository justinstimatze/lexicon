package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `id: lex-uz7g4
name: test-entry
type-in: claim
type-out: posture
_tier: atomic
lineage:
  - source: walton
    text: walton-reed-macagno-2008
    citation: ch.1
canonical-instances:
  - "an example claim"
status: under-review
`

func writeTmpYAML(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// Load parses required fields and round-trips the entry intact.
func TestLoadValidEntry(t *testing.T) {
	dir := writeTmpYAML(t, map[string]string{"lex-uz7g4.yaml": validYAML})
	e, err := Load(dir, "lex-uz7g4")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e.ID != "lex-uz7g4" || e.Name != "test-entry" || e.Tier != "atomic" {
		t.Fatalf("entry mismatch: %+v", e)
	}
	if len(e.Lineage) != 1 || e.Lineage[0].Source != "walton" {
		t.Fatalf("lineage wrong: %+v", e.Lineage)
	}
}

// Missing required field is the worst silent-drift failure: a typo in
// a field name vanishes into yaml-ignored. We catch it explicitly.
func TestLoadRejectsMissingRequiredField(t *testing.T) {
	yaml := strings.Replace(validYAML, "name: test-entry\n", "", 1)
	dir := writeTmpYAML(t, map[string]string{"lex-uz7g4.yaml": yaml})
	_, err := Load(dir, "lex-uz7g4")
	if err == nil || !strings.Contains(err.Error(), "missing required field 'name'") {
		t.Fatalf("expected missing-name error, got %v", err)
	}
}

// id-vs-filename mismatch: catches accidental copy-paste between
// entries; the filename is the canonical id by convention.
func TestLoadRejectsIDMismatch(t *testing.T) {
	yaml := strings.Replace(validYAML, "id: lex-uz7g4", "id: lex-ypecd", 1)
	dir := writeTmpYAML(t, map[string]string{"lex-uz7g4.yaml": yaml})
	_, err := Load(dir, "lex-uz7g4")
	if err == nil || !strings.Contains(err.Error(), "id field is lex-ypecd") {
		t.Fatalf("expected id-mismatch error, got %v", err)
	}
}

// id format must match /^lex-\d{4}$/ — catches a bad rename or
// hand-written id that breaks the elements' monotonic-numeric
// invariant.
func TestLoadRejectsBadIDFormat(t *testing.T) {
	yaml := strings.Replace(validYAML, "id: lex-uz7g4", "id: bad-id", 1)
	dir := writeTmpYAML(t, map[string]string{"bad-id.yaml": yaml})
	_, err := Load(dir, "bad-id")
	if err == nil || !strings.Contains(err.Error(), "id must match") {
		t.Fatalf("expected id-format error, got %v", err)
	}
}

// Empty lineage means the entry has no provenance at all —
// schema-required for SCHEMA.md compliance.
func TestLoadRejectsEmptyLineage(t *testing.T) {
	yaml := `id: lex-uz7g4
name: test-entry
type-in: claim
type-out: posture
_tier: atomic
lineage: []
canonical-instances: ["x"]
status: under-review
`
	dir := writeTmpYAML(t, map[string]string{"lex-uz7g4.yaml": yaml})
	_, err := Load(dir, "lex-uz7g4")
	if err == nil || !strings.Contains(err.Error(), "lineage must be a non-empty array") {
		t.Fatalf("expected empty-lineage error, got %v", err)
	}
}

// LoadAll must return every yaml file in the dir, keyed by id.
func TestLoadAllReadsAllEntries(t *testing.T) {
	second := strings.Replace(strings.Replace(validYAML, "lex-uz7g4", "lex-tf6br", 1), "test-entry", "second", 1)
	dir := writeTmpYAML(t, map[string]string{
		"lex-uz7g4.yaml": validYAML,
		"lex-tf6br.yaml": second,
	})
	entries, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries["lex-uz7g4"].Name != "test-entry" || entries["lex-tf6br"].Name != "second" {
		t.Fatalf("name lookup wrong: %+v", entries)
	}
}

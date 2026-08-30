// Package loader reads YAML lex entries from an elements directory and
// validates the minimum-required-fields contract from SCHEMA.md.
//
// Drift mitigation: the loader's job is to catch the worst silent-drift
// failures (typo'd field names that vanish into yaml-ignored,
// id-vs-filename mismatch, malformed id format, empty required arrays)
// before they propagate into render output. Full schema validation lives
// in `lexicon lint` and `lexicon db lint`.
package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// DefaultElementsDir is the elements/ directory at the repo root,
// reached as <renderDir>/../elements. v0 stays as on-disk YAML;
// elements eventually graduates to winze per BOOTSTRAP.
var DefaultElementsDir = filepath.Join("..", "elements")

// Migrated 2026-08-20 from sequential 4-digit ids to non-sequential
// 5-character opaque codes (see render/cmd/lexicon/cmd_renumber.go); the
// alphabet excludes 0/1/i/l/o for hand-typing safety.
var idPattern = regexp.MustCompile(`^lex-[23456789abcdefghjkmnpqrstuvwxyz]{5}$`)

// Load reads <elementsDir>/<id>.yaml and returns the validated entry.
func Load(elementsDir, id string) (*types.LexEntry, error) {
	path := filepath.Join(elementsDir, id+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("loader: read %s: %w", path, err)
	}
	entry, err := parseAndValidate(data, path)
	if err != nil {
		return nil, err
	}
	if entry.ID != id {
		return nil, fmt.Errorf(
			"loader: %s: id field is %s but filename is %s.yaml",
			path, entry.ID, id,
		)
	}
	return entry, nil
}

// LoadAll reads every *.yaml file in elementsDir. Map keys are the
// entry IDs. Returns first error if any file fails to parse/validate
// — elements consistency is more important than partial loads.
func LoadAll(elementsDir string) (map[string]*types.LexEntry, error) {
	dirEntries, err := os.ReadDir(elementsDir)
	if err != nil {
		return nil, fmt.Errorf("loader: readdir %s: %w", elementsDir, err)
	}
	out := map[string]*types.LexEntry{}
	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(elementsDir, de.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("loader: read %s: %w", path, err)
		}
		entry, err := parseAndValidate(data, path)
		if err != nil {
			return nil, err
		}
		expectedID := strings.TrimSuffix(de.Name(), ".yaml")
		if entry.ID != expectedID {
			return nil, fmt.Errorf(
				"loader: %s: id field is %s but filename is %s",
				path, entry.ID, de.Name(),
			)
		}
		out[entry.ID] = entry
	}
	return out, nil
}

func parseAndValidate(data []byte, path string) (*types.LexEntry, error) {
	var entry types.LexEntry
	if err := yaml.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("loader: parse %s: %w", path, err)
	}
	if err := validate(&entry, path); err != nil {
		return nil, err
	}
	return &entry, nil
}

func validate(e *types.LexEntry, path string) error {
	missing := []string{}
	if e.ID == "" {
		missing = append(missing, "id")
	}
	if e.Name == "" {
		missing = append(missing, "name")
	}
	if e.TypeIn == "" {
		missing = append(missing, "type-in")
	}
	if e.TypeOut == "" {
		missing = append(missing, "type-out")
	}
	if e.Tier == "" {
		missing = append(missing, "_tier")
	}
	if e.Status == "" {
		missing = append(missing, "status")
	}
	if len(missing) > 0 {
		return fmt.Errorf("loader: %s: missing required field '%s'", path, missing[0])
	}
	if !idPattern.MatchString(e.ID) {
		return fmt.Errorf("loader: %s: id must match /^lex-[2-9a-hj-km-np-z]{5}$/, got %s", path, e.ID)
	}
	if len(e.Lineage) == 0 {
		return fmt.Errorf("loader: %s: lineage must be a non-empty array", path)
	}
	if len(e.CanonicalInstances) == 0 {
		return fmt.Errorf("loader: %s: canonical-instances must be a non-empty array", path)
	}
	return nil
}

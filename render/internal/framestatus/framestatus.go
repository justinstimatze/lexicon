// Package framestatus operationalizes the oracle-risk register
// (docs/audits/oracle-risk-full-register.md) at render time.
//
// Every CQ-bearing atom was blind-coded for CQ-terminus and mapped to a
// frame-status (elements-design Principle 10):
//
//   - navigational — a CQ bottoms out in an external check; render as an
//     operative finding the user can verify.
//   - mixed        — carries a checkable toe; render operatively but LEAD
//     WITH THE NAMED HANDLE (the specific CQ that grounds externally).
//   - constitutive — grounds entirely in judgment; render as an OFFERED
//     LENS, never as a finding (surfacing-as-finding is the crypto-tarot
//     failure the whole register exists to prevent).
//
// The register markdown is the single source of truth; this package parses
// it rather than duplicating the classification into a generated file (no
// second copy to drift). Fail-soft by design: if the register is missing or
// unparseable, lookups return ok=false and the caller simply omits the
// frame annotation — the render still works, it just loses the honesty
// labels (with a one-time stderr note from the caller).
package framestatus

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Status is the frame-status of an atom: "navigational", "mixed", or
// "constitutive". Empty string means unclassified (not in the register —
// e.g. a newly-minted atom not yet swept, or a non-CQ-bearing atom).
type Status string

const (
	Navigational Status = "navigational"
	Mixed        Status = "mixed"
	Constitutive Status = "constitutive"
)

// Entry is one atom's frame-status plus, for mixed atoms, the checkable
// handle (the specific CQ text that grounds externally).
type Entry struct {
	Status Status
	Handle string // populated only for Mixed
}

// Map is a lex-id → Entry lookup parsed from the register.
type Map map[string]Entry

// RegisterRelPath is the register's location relative to renderDir
// (render/ is a sibling of docs/ at the repo root).
var RegisterRelPath = filepath.Join("..", "docs", "audits", "oracle-risk-full-register.md")

// Load parses the register at <renderDir>/../docs/audits/... into a Map.
// Returns an empty Map and a non-nil error if the file can't be read; a
// partial Map on a malformed table is acceptable (best-effort, fail-soft).
func Load(renderDir string) (Map, error) {
	path := filepath.Join(renderDir, RegisterRelPath)
	f, err := os.Open(path)
	if err != nil {
		return Map{}, err
	}
	defer f.Close()
	return parse(f), nil
}

// section tracks which register pool the scanner is currently inside.
type section int

const (
	secNone section = iota
	secConstitutive
	secMixed
	secNavigational
)

// parse scans the register markdown. Section is set by the "## A./B./C."
// headers; rows are table lines whose first cell starts with "lex-".
// Constitutive/Navigational tables are | id | name | prov |; the Mixed
// table is | id | name | handle | prov |.
func parse(r io.Reader) Map {
	m := Map{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	cur := secNone
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			low := strings.ToLower(line)
			switch {
			case strings.Contains(low, "constitutive pool"):
				cur = secConstitutive
			case strings.Contains(low, "mixed pool"):
				cur = secMixed
			case strings.Contains(low, "navigational"):
				cur = secNavigational
			}
			continue
		}
		if cur == secNone || !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitRow(line)
		if len(cells) == 0 || !strings.HasPrefix(cells[0], "lex-") {
			continue // header row, separator row, or non-data line
		}
		id := cells[0]
		switch cur {
		case secConstitutive:
			m[id] = Entry{Status: Constitutive}
		case secNavigational:
			m[id] = Entry{Status: Navigational}
		case secMixed:
			handle := ""
			if len(cells) >= 3 {
				handle = cells[2]
			}
			m[id] = Entry{Status: Mixed, Handle: handle}
		}
	}
	return m
}

// splitRow splits a markdown table row into trimmed cells, dropping the
// empty leading/trailing cells produced by the bounding pipes.
func splitRow(line string) []string {
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Lookup returns the Entry for an id and whether it was classified.
func (m Map) Lookup(id string) (Entry, bool) {
	e, ok := m[id]
	return e, ok
}

// Label is the short epistemic tag shown in the raw markdown frame line.
func (e Entry) Label() string {
	switch e.Status {
	case Navigational:
		return "operative — checkable"
	case Mixed:
		return "mixed — lead with the checkable handle; the rest is interpretive"
	case Constitutive:
		return "offered lens — not a finding; grounds in judgment, check it against your own case"
	default:
		return "unclassified"
	}
}

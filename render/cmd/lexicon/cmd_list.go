package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cmdList: emit a flat enumeration of every atom in the elements.
// Agent-tool first: JSON by default, --text for TSV.
//
// Usage:
//
//	lexicon list                       # JSON array of {id, name, tier, status}
//	lexicon list --text                # TSV: id\tname\ttier\tstatus
//	lexicon list --status active       # filter by status
//	lexicon list --tier primitive      # filter by _tier
type listEntry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Tier   string `json:"tier"`
	Status string `json:"status"`
}

func cmdList(renderDir string, args []string) {
	fl := flag.NewFlagSet("list", flag.ExitOnError)
	asText := fl.Bool("text", false, "human-readable TSV output (default: JSON)")
	tierFilter := fl.String("tier", "", "filter by _tier (e.g. primitive, molecule)")
	statusFilter := fl.String("status", "", "filter by status (e.g. active, under-review)")
	_ = fl.Parse(args)

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, "..", "elements")
		if _, err := os.Stat(elementsDir); err != nil {
			elementsDir = filepath.Join(renderDir, "elements")
		}
	}
	entries, err := os.ReadDir(elementsDir)
	if err != nil {
		fatal("list: %v", err)
	}

	out := make([]listEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if len(name) < 14 || name[:4] != "lex-" || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(elementsDir, name))
		if err != nil {
			continue
		}
		id, aname, tier, status := scanAtomMeta(data)
		if id == "" {
			continue
		}
		if *tierFilter != "" && tier != *tierFilter {
			continue
		}
		if *statusFilter != "" && status != *statusFilter {
			continue
		}
		out = append(out, listEntry{ID: id, Name: aname, Tier: tier, Status: status})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	if *asText {
		fmt.Fprintf(os.Stdout, "%d atoms\n\nid\tname\ttier\tstatus\n", len(out))
		for _, e := range out {
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\n", e.ID, e.Name, e.Tier, e.Status)
		}
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fatal("list: encode: %v", err)
	}
}

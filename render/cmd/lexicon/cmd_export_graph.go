package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/viz"
)

// cmdExportGraph emits the elements graph as raw JSON — the shared
// data contract (viz.Graph) every HTML renderer already inlines at
// build time, exposed standalone for the web/ frontend to consume.
func cmdExportGraph(renderDir string, args []string) {
	fl := flag.NewFlagSet("export-graph", flag.ExitOnError)
	out := fl.String("out", "", "output path for the graph JSON (default: stdout)")
	full := fl.Bool("full", false, "include canonical_instances, agent_instruction, critical_questions, and lineage text in the main graph output (default: trimmed — it's several MB raw across the whole catalog; use -details-dir instead of -full for a web frontend that wants those fields on demand per atom rather than all upfront)")
	detailsDir := fl.String("details-dir", "", "also write one JSON file per atom (<id>.json) under this directory, holding just canonical_instances/agent_instruction/critical_questions/lineage — for a frontend that fetches an atom's detail fields on open instead of shipping all 1000+ atoms' worth upfront. Independent of -full: the main -out graph stays trimmed either way unless -full is also passed.")
	if err := fl.Parse(args); err != nil {
		fatal("parse flags: %s", err)
	}

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, loader.DefaultElementsDir)
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("loader: %s", err)
	}

	graph := viz.ToGraph(pool, clusterContinuityPath(renderDir))
	graph.Layouts = viz.ComputeLayouts(graph.Nodes, graph.Edges)

	if *detailsDir != "" {
		if err := writeAtomDetails(*detailsDir, graph.Nodes); err != nil {
			fatal("write details-dir: %s", err)
		}
	}

	if !*full {
		for i := range graph.Nodes {
			graph.Nodes[i].CanonicalInstances = nil
			graph.Nodes[i].AgentInstruction = ""
			graph.Nodes[i].CriticalQuestions = nil
			graph.Nodes[i].Lineage = nil
		}
	}

	data, err := json.Marshal(graph)
	if err != nil {
		fatal("marshal: %s", err)
	}

	if *out == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			fatal("write stdout: %v", err)
		}
		fmt.Println()
		return
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fatal("mkdir: %s", err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fatal("write: %s", err)
	}
	fmt.Printf("wrote %s (%d atoms · %d edges · %d clusters)\n", *out, len(graph.Nodes), len(graph.Edges), len(graph.Clusters))
}

// atomDetail is the per-atom fields writeAtomDetails emits — the same
// text-heavy fields -full gates in the main graph, just split one file
// per atom instead of every atom in one payload.
type atomDetail struct {
	CanonicalInstances []string     `json:"canonical_instances,omitempty"`
	AgentInstruction   string       `json:"agent_instruction,omitempty"`
	CriticalQuestions  []string     `json:"critical_questions,omitempty"`
	Lineage            []viz.Lineage `json:"lineage,omitempty"`
}

func writeAtomDetails(dir string, nodes []viz.Node) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	for _, n := range nodes {
		d := atomDetail{
			CanonicalInstances: n.CanonicalInstances,
			AgentInstruction:   n.AgentInstruction,
			CriticalQuestions:  n.CriticalQuestions,
			Lineage:            n.Lineage,
		}
		data, err := json.Marshal(d)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", n.ID, err)
		}
		if err := os.WriteFile(filepath.Join(dir, n.ID+".json"), data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", n.ID, err)
		}
	}
	return nil
}

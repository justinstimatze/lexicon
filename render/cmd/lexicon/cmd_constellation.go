package main

// cmd_constellation.go — `lexicon constellation <id>`: N-hop neighborhood
// of a focal atom, emitted as structured JSON. Reads the elements
// in-memory and walks adjacencies; eventual DB-backed path will replace
// this when elements scales past ~thousands.
//
// Contract:
//
//	{
//	  "focal":    {id, name, tier, status, gloss, agent_instruction, type_in,
//	               type_out, premises, critical_questions},
//	  "outgoing": {related: [...], decomposes_into: [...], evokes: [...]},
//	  "incoming": {related_from: [...], decomposes_into_from: [...]},
//	  "hop2":     {via_lex-XXXX: [...]}   // only when --hops 2
//	}
//
// Each neighbor carries {id, name, tier, gloss, agent_instruction, type_in,
// type_out} so the caller can compose without a follow-up read -- type_in/
// type_out are exactly the schema's own answer to whether a "related" or
// "decomposes_into" neighbor is actually type-compatible, which is the
// premise those buckets are built on.
//
// premises/critical_questions live on "focal", not "outgoing": they're
// prose describing the FOCAL atom's own reasoning structure (Walton-style
// premise/defeater pairs), not references to other atoms. An earlier
// version ran them through the same atom-ID resolver as related/
// decomposes_into ("outgoing.premises"), which meant every premise string
// silently failed to resolve and the bucket was always empty -- confirmed
// via freshet's 2026-09-05 structured-fields-not-surfaced feedback.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

type constellationNeighbor struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Tier             string `json:"tier,omitempty"`
	Gloss            string `json:"gloss,omitempty"`
	AgentInstruction string `json:"agent_instruction,omitempty"`
	TypeIn           string `json:"type_in,omitempty"`
	TypeOut          string `json:"type_out,omitempty"`
}

// constellationFocal always carries the full molecule-assembly triple
// (Premises/CriticalQuestions) when the focal atom has one -- unlike
// read/distinctness, constellation has no detail flag to gate payload
// size against, because it's a single-atom deep-dive, not a many-result
// listing: one focal atom's full richness costs nothing like N results'
// worth would.
type constellationFocal struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Tier              string   `json:"tier,omitempty"`
	Status            string   `json:"status,omitempty"`
	Gloss             string   `json:"gloss,omitempty"`
	AgentInstruction  string   `json:"agent_instruction,omitempty"`
	TypeIn            string   `json:"type_in,omitempty"`
	TypeOut           string   `json:"type_out,omitempty"`
	Premises          []string `json:"premises,omitempty"`
	CriticalQuestions []string `json:"critical_questions,omitempty"`
}

type constellationDoc struct {
	Focal    constellationFocal                 `json:"focal"`
	Outgoing map[string][]constellationNeighbor `json:"outgoing,omitempty"`
	Incoming map[string][]constellationNeighbor `json:"incoming,omitempty"`
	Hop2     map[string][]constellationNeighbor `json:"hop2,omitempty"`
}

// reorderFlagsFirst moves all flag tokens (--name or --name=val) plus
// their value (for non-bool flags) to the front so the standard flag
// package's Parse() doesn't stop at the first positional. Best-effort:
// it preserves order within flag and positional groups separately.
func reorderFlagsFirst(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") || strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// If it's --flag value (no = and not a bool-styled --flag=true),
			// pull the next token too. This is a heuristic — bool flags don't
			// take a value so they break here.
			if !strings.Contains(a, "=") && i+1 < len(args) {
				next := args[i+1]
				if !strings.HasPrefix(next, "-") {
					// Only consume if the flag is plausibly value-taking. Without
					// reflection we can't know; we err toward consuming, which is
					// correct for --hops 2 and harmless for boolean flags written
					// with explicit values like --incoming=false.
					flags = append(flags, next)
					i++
				}
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

func cmdConstellation(renderDir string, args []string) {
	fl := flag.NewFlagSet("constellation", flag.ExitOnError)
	hops := fl.Int("hops", 1, "neighborhood depth (1 or 2)")
	includeIncoming := fl.Bool("incoming", true, "include backrefs (atoms pointing AT the focal)")
	// Reorder: pull positional args (lex-ids) to the END so flags after
	// the positional still get parsed. Without this, `lexicon constellation
	// lex-spm8x --hops 2` would treat --hops as a positional.
	args = reorderFlagsFirst(args)
	_ = fl.Parse(args)
	if fl.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: lexicon constellation <lex-id> [--hops 1|2] [--no-incoming]")
		os.Exit(2)
	}
	focal := fl.Arg(0)

	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("load elements: %v", err)
	}
	e, ok := pool[focal]
	if !ok {
		fatal("constellation: %s not in elements", focal)
	}

	doc := buildConstellation(e, pool, *hops, *includeIncoming)
	out, _ := json.MarshalIndent(doc, "", "  ")
	fmt.Println(string(out))
}

// buildConstellation assembles the focal atom's 1- or 2-hop neighborhood
// from the in-memory elements. Edge buckets are kept distinct so consumers
// can compose by edge type without reparsing.
func buildConstellation(focal *types.LexEntry, pool map[string]*types.LexEntry, hops int, incoming bool) constellationDoc {
	doc := constellationDoc{
		Focal: constellationFocal{
			ID:                focal.ID,
			Name:              focal.Name,
			Tier:              focal.Tier,
			Status:            focal.Status,
			Gloss:             patternGloss(focal),
			AgentInstruction:  focal.AgentInstruction,
			TypeIn:            focal.TypeIn,
			TypeOut:           focal.TypeOut,
			Premises:          focal.Premises,
			CriticalQuestions: focal.CriticalQuestions,
		},
		Outgoing: map[string][]constellationNeighbor{},
	}

	resolve := func(id string) (constellationNeighbor, bool) {
		n, ok := pool[id]
		if !ok || n == nil {
			return constellationNeighbor{}, false
		}
		return constellationNeighbor{
			ID:               n.ID,
			Name:             n.Name,
			Tier:             n.Tier,
			Gloss:            patternGloss(n),
			AgentInstruction: n.AgentInstruction,
			TypeIn:           n.TypeIn,
			TypeOut:          n.TypeOut,
		}, true
	}

	addBucket := func(bucket string, ids []string, store map[string][]constellationNeighbor) {
		seen := map[string]bool{}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			if n, ok := resolve(id); ok {
				store[bucket] = append(store[bucket], n)
			}
		}
	}

	addBucket("related", focal.Related, doc.Outgoing)
	addBucket("decomposes_into", focal.DecomposesInto, doc.Outgoing)
	if len(focal.Evokes) > 0 {
		doc.Outgoing["evokes"] = make([]constellationNeighbor, 0, len(focal.Evokes))
		for _, v := range focal.Evokes {
			doc.Outgoing["evokes"] = append(doc.Outgoing["evokes"], constellationNeighbor{
				Name: v,
			})
		}
	}

	if incoming {
		doc.Incoming = map[string][]constellationNeighbor{}
		ids := make([]string, 0, len(pool))
		for id := range pool {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if id == focal.ID {
				continue
			}
			candidate := pool[id]
			for _, rel := range candidate.Related {
				if rel == focal.ID {
					if n, ok := resolve(id); ok {
						doc.Incoming["related_from"] = append(doc.Incoming["related_from"], n)
					}
					break
				}
			}
			for _, d := range candidate.DecomposesInto {
				if strings.TrimSpace(d) == focal.ID {
					if n, ok := resolve(id); ok {
						doc.Incoming["decomposes_into_from"] = append(doc.Incoming["decomposes_into_from"], n)
					}
					break
				}
			}
		}
	}

	if hops >= 2 {
		doc.Hop2 = map[string][]constellationNeighbor{}
		visited := map[string]bool{focal.ID: true}
		for _, ns := range doc.Outgoing {
			for _, n := range ns {
				if n.ID != "" {
					visited[n.ID] = true
				}
			}
		}
		for _, hopOne := range doc.Outgoing["related"] {
			key := "via_" + hopOne.ID
			seen := map[string]bool{}
			via := pool[hopOne.ID]
			if via == nil {
				continue
			}
			for _, rel := range via.Related {
				if visited[rel] || seen[rel] {
					continue
				}
				seen[rel] = true
				if n, ok := resolve(rel); ok {
					doc.Hop2[key] = append(doc.Hop2[key], n)
				}
			}
		}
	}

	return doc
}

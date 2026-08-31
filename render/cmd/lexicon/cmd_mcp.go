package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/extrapolate"
	"github.com/justinstimatze/lexicon/render/internal/loader"
)

// MCP stdio server for lexicon's elements query surface. Speaks
// JSON-RPC 2.0 on newline-delimited stdio per the MCP spec.
//
// Pattern lifted from github.com/justinstimatze/be-my-geminis's
// cmd/bmg/cmd_mcp.go (which lifts from github.com/justinstimatze/hindcast's
// cmd/hindcast/cmd_mcp.go): hand-rolled scanner over stdin
// and json.Encoder over stdout, no mcp-go dep so the static binary
// stays small. Tools registered:
//
//	lexicon_read(text, top_k?, no_lens?, format?) → JSON pattern-id report (default)
//	lexicon_list()                                 → flat atom enumeration, JSON
//
// Each tool shells out to the same lexicon binary (resolved via
// os.Executable so an updated binary in $PATH is picked up the next
// time `claude mcp restart` is run). The subprocess isolation means
// a panic in `cmd_read.go` cannot kill the MCP server.

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func cmdMCP(renderDir string) {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(os.Stdout)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req mcpRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		handleMCP(enc, renderDir, req)
	}
}

func mcpEmit(enc *json.Encoder, resp any) {
	_ = enc.Encode(resp)
}

func errResp(id json.RawMessage, code int, msg string) mcpResponse {
	return mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcpError{Code: code, Message: msg},
	}
}

func handleMCP(enc *json.Encoder, renderDir string, req mcpRequest) {
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		mcpEmit(enc, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{
					"name":    "lexicon",
					"version": "0.1",
				},
				"instructions": mcpInstructionsFor(renderDir),
			},
		})

	case "notifications/initialized", "notifications/cancelled":
		// no response for notifications

	case "tools/list":
		mcpEmit(enc, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": []any{
					readToolDefinition(),
					listToolDefinition(),
					extrapolateToolDefinition(),
					constellationToolDefinition(),
					predictToolDefinition(),
					distinctnessToolDefinition(),
				},
			},
		})

	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			mcpEmit(enc, errResp(req.ID, -32602, "invalid params"))
			return
		}
		switch p.Name {
		case "lexicon_read":
			text, isErr := callRead(renderDir, p.Arguments)
			result := map[string]any{
				"content": []any{map[string]any{"type": "text", "text": text}},
			}
			if isErr {
				result["isError"] = true
			}
			mcpEmit(enc, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
		case "lexicon_list":
			text, isErr := callList(renderDir, p.Arguments)
			result := map[string]any{
				"content": []any{map[string]any{"type": "text", "text": text}},
			}
			if isErr {
				result["isError"] = true
			}
			mcpEmit(enc, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
		case "lexicon_extrapolate":
			text, isErr := callExtrapolate(renderDir, p.Arguments)
			result := map[string]any{
				"content": []any{map[string]any{"type": "text", "text": text}},
			}
			if isErr {
				result["isError"] = true
			}
			mcpEmit(enc, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
		case "lexicon_constellation":
			text, isErr := callConstellation(renderDir, p.Arguments)
			result := map[string]any{
				"content": []any{map[string]any{"type": "text", "text": text}},
			}
			if isErr {
				result["isError"] = true
			}
			mcpEmit(enc, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
		case "lexicon_predict":
			text, isErr := callPredict(renderDir, p.Arguments)
			result := map[string]any{
				"content": []any{map[string]any{"type": "text", "text": text}},
			}
			if isErr {
				result["isError"] = true
			}
			mcpEmit(enc, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
		case "lexicon_distinctness":
			text, isErr := callDistinctness(renderDir, p.Arguments)
			result := map[string]any{
				"content": []any{map[string]any{"type": "text", "text": text}},
			}
			if isErr {
				result["isError"] = true
			}
			mcpEmit(enc, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
		default:
			mcpEmit(enc, errResp(req.ID, -32601, "unknown tool: "+p.Name))
		}

	default:
		if !isNotification {
			mcpEmit(enc, errResp(req.ID, -32601, "method not found: "+req.Method))
		}
	}
}

// resolveElementsDir finds elements/ relative to renderDir, honouring an
// explicit LEXICON_ELEMENTS_DIR override.
func resolveElementsDir(renderDir string) string {
	if d := os.Getenv("LEXICON_ELEMENTS_DIR"); d != "" {
		return d
	}
	d := filepath.Join(renderDir, "..", "elements")
	if _, err := os.Stat(d); err != nil {
		d = filepath.Join(renderDir, "elements")
	}
	return d
}

// mcpInstructionsFor counts the elements at handshake time instead of baking
// a number into the string. The hardcoded count had drifted to less than half
// the real one (it read "~459 atoms" at 1028) and, because this text is
// injected into every session that registers the server, it was the single
// most-read wrong sentence in the project. A count is genuinely useful to a
// consuming model, so counting beats deleting; if the count fails, the clause
// is dropped rather than guessed.
func mcpInstructionsFor(renderDir string) string {
	size := "a catalog"
	if entries, err := os.ReadDir(resolveElementsDir(renderDir)); err == nil {
		n := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				n++
			}
		}
		if n > 0 {
			size = fmt.Sprintf("%d atoms", n)
		}
	}
	return fmt.Sprintf(mcpInstructionsTmpl, size)
}

const mcpInstructionsTmpl = `lexicon is Justin's pattern elements (%s covering reasoning,
strategy, ethics, narrative, scientific method, power dynamics, etc.).
Each atom names a recurring cognitive or structural move, with verbatim
lineage from primary sources.

lexicon_read: call whenever a passage of >2-3 sentences (a pasted news
story, transcript, doc, or your own reasoning/a stuck decision) would
benefit from named-pattern decomposition. Compact JSON by default; pass
detail=true only if you need critical_questions or the full 6-neighbor
adjacency list.

lexicon_list: rare — only when you need the atom inventory before
phrasing a query.

lexicon_extrapolate: given a constellation of atom IDs (e.g. from a prior
lexicon_read), returns the ADJACENCY FRONTIER — atoms the gestalt invokes
but doesn't contain, ranked by how many point at each. No LLM call.

See each tool's own description for its JSON shape.`

func readToolDefinition() map[string]any {
	return map[string]any{
		"name": "lexicon_read",
		"description": "Surface the top-K lexicon atoms firing on a passage of text. JSON: " +
			"{context, top_k, lens_used, patterns:[{id, name, tier, score, frame_status, " +
			"gloss, agent_instruction, adjacencies:[{id, name}]}]}. agent_instruction is " +
			"the 'when-you-see-this-do-this' rule. Compact by default (3 adjacencies, no " +
			"critical_questions); pass detail=true for the full 6-neighbor shape with " +
			"critical_questions. no_lens=true skips the LLM lens for a faster lexical-only " +
			"pass. format=\"markdown\"/\"plain\" for human-readable output.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "The passage to analyze.",
				},
				"top_k": map[string]any{
					"type":        "integer",
					"description": "How many atoms to surface (default 3, max ~10).",
				},
				"no_lens": map[string]any{
					"type":        "boolean",
					"description": "Skip the LLM-backed semantic lens; faster, less precise.",
				},
				"detail": map[string]any{
					"type":        "boolean",
					"description": "Include critical_questions and expand adjacencies to 6 (default: omitted / capped at 3 for a leaner response).",
				},
				"format": map[string]any{
					"type":        "string",
					"description": "\"json\" (default), \"markdown\" (structured), \"plain\" (LLM-translated narrative).",
					"enum":        []any{"json", "markdown", "plain"},
				},
				"no_explain": map[string]any{
					"type":        "boolean",
					"description": "DEPRECATED: use format=\"markdown\".",
				},
			},
			"required": []any{"text"},
		},
	}
}

func extrapolateToolDefinition() map[string]any {
	return map[string]any{
		"name": "lexicon_extrapolate",
		"description": "Adjacency-frontier read on a constellation of atom IDs: atoms NOT " +
			"in the input set that are pointed at by one or more of them, ranked by " +
			"adjacency-count. No LLM. JSON: {constellation, missing, candidates:[{id, name, " +
			"tier, status, adjacency_count, pointed_at_by}]}. Use to find the ontological " +
			"negative space of a frame implied by a constellation of atoms.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"atoms": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Constellation of atom IDs (e.g. ['lex-0001', 'lex-dm5te']).",
				},
				"top_k": map[string]any{
					"type":        "integer",
					"description": "Limit candidates returned (default: all).",
				},
			},
			"required": []any{"atoms"},
		},
	}
}

func distinctnessToolDefinition() map[string]any {
	return map[string]any{
		"name": "lexicon_distinctness",
		"description": "Pre-mint overlap check. Pass a candidate atom's name + brief " +
			"description; returns the corpus atoms most likely to overlap, each with its " +
			"own 'operationally distinct from' entries. JSON: {candidate, lens_used, " +
			"warning?, matches:[{id, name, tier, score, status, frame_status, gloss, " +
			"agent_instruction, operationally_distinct_from:[...]}]}. Scanned in concurrent " +
			"token-budgeted chunks so every atom gets a real look. If warning is set, " +
			"coverage was incomplete (some/all chunks failed, or no_lens=true) — matches " +
			"may lack content signal; retry when lens_used=false. Use before drafting a new " +
			"atom to verify it isn't already covered.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "The candidate atom's name + brief operational description.",
				},
				"top_k": map[string]any{
					"type":        "integer",
					"description": "How many overlap candidates to surface (default 5).",
				},
				"no_lens": map[string]any{
					"type":        "boolean",
					"description": "Skip the LLM-backed semantic lens. Default false.",
				},
			},
			"required": []any{"text"},
		},
	}
}

func callDistinctness(renderDir string, arguments json.RawMessage) (string, bool) {
	var args struct {
		Text   string `json:"text"`
		TopK   int    `json:"top_k"`
		NoLens bool   `json:"no_lens"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return fmt.Sprintf("lexicon_distinctness: invalid arguments: %s", err), true
	}
	if args.Text == "" {
		return "lexicon_distinctness: text is required", true
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Sprintf("lexicon_distinctness: resolve self: %s", err), true
	}
	cliArgs := []string{"distinctness"}
	if args.TopK > 0 {
		cliArgs = append(cliArgs, "--top-k", strconv.Itoa(args.TopK))
	}
	if args.NoLens {
		cliArgs = append(cliArgs, "--no-lens")
	}
	cliArgs = append(cliArgs, "-")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, cliArgs...)
	cmd.Stdin = bytes.NewBufferString(args.Text)
	cmd.Dir = renderDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("lexicon_distinctness failed: %s\n\nstderr:\n%s", err, stderr.String()), true
	}
	return stdout.String(), false
}

func predictToolDefinition() map[string]any {
	return map[string]any{
		"name": "lexicon_predict",
		"description": "Forecast downstream effects of a plan or situation via matched " +
			"reaction-tier atoms. JSON: {context, reactions:[{id, name, tier, frame_status, " +
			"mechanism, reactants, products, catalysts, inhibitors, conditions, " +
			"reversibility, gloss}], fallback_matches:[...]}. products = what's likely, " +
			"catalysts = what accelerates it, inhibitors = what blocks it. Falls back to " +
			"top-3 non-reaction matches when no reaction fires.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "The situation or planned action to forecast.",
				},
				"top_k": map[string]any{
					"type":        "integer",
					"description": "How many reactions to surface (default 3, max ~8).",
				},
				"no_lens": map[string]any{
					"type":        "boolean",
					"description": "Skip the LLM-backed semantic lens (lexical-only on full pool). Faster, less precise.",
				},
			},
			"required": []any{"text"},
		},
	}
}

func callPredict(renderDir string, arguments json.RawMessage) (string, bool) {
	var args struct {
		Text   string `json:"text"`
		TopK   int    `json:"top_k"`
		NoLens bool   `json:"no_lens"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return fmt.Sprintf("lexicon_predict: invalid arguments: %s", err), true
	}
	if args.Text == "" {
		return "lexicon_predict: text is required", true
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Sprintf("lexicon_predict: resolve self: %s", err), true
	}
	cliArgs := []string{"what-if", "--mode", "intervene", "--format", "json", "--context", "-"}
	if args.TopK > 0 {
		cliArgs = append(cliArgs, "--top-k", strconv.Itoa(args.TopK))
	}
	if args.NoLens {
		cliArgs = append(cliArgs, "--no-lens")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, cliArgs...)
	cmd.Stdin = bytes.NewBufferString(args.Text)
	cmd.Dir = renderDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("lexicon_predict failed: %s\n\nstderr:\n%s", err, stderr.String()), true
	}
	return stdout.String(), false
}

func constellationToolDefinition() map[string]any {
	return map[string]any{
		"name": "lexicon_constellation",
		"description": "N-hop neighborhood of one focal atom, structured JSON. Use after " +
			"lexicon_read identifies a high-relevance atom and you want its adjacencies " +
			"expanded — no LLM call, pure elements-graph walk. Returns " +
			"{focal:{id,name,tier,status,gloss,agent_instruction}, " +
			"outgoing:{related,decomposes_into,premises,evokes}, " +
			"incoming:{related_from,decomposes_into_from}, hop2:{via_lex-XXXX:[...]}}, each " +
			"neighbor carrying gloss + agent_instruction. Default hops=1.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"atom_id": map[string]any{
					"type":        "string",
					"description": "Focal atom id (e.g. 'lex-spm8x').",
				},
				"hops": map[string]any{
					"type":        "integer",
					"description": "Neighborhood depth (1 or 2; default 1).",
				},
				"incoming": map[string]any{
					"type":        "boolean",
					"description": "Include backrefs (atoms pointing AT the focal). Default true.",
				},
			},
			"required": []any{"atom_id"},
		},
	}
}

func callConstellation(renderDir string, arguments json.RawMessage) (string, bool) {
	var args struct {
		AtomID   string `json:"atom_id"`
		Hops     int    `json:"hops"`
		Incoming *bool  `json:"incoming"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return fmt.Sprintf("lexicon_constellation: invalid arguments: %s", err), true
	}
	if args.AtomID == "" {
		return "lexicon_constellation: atom_id is required", true
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Sprintf("lexicon_constellation: resolve self: %s", err), true
	}
	cliArgs := []string{"constellation"}
	if args.Hops > 0 {
		cliArgs = append(cliArgs, "--hops", strconv.Itoa(args.Hops))
	}
	if args.Incoming != nil && !*args.Incoming {
		cliArgs = append(cliArgs, "--incoming=false")
	}
	cliArgs = append(cliArgs, args.AtomID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, cliArgs...)
	cmd.Dir = renderDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("lexicon_constellation failed: %s\n\nstderr:\n%s", err, stderr.String()), true
	}
	return stdout.String(), false
}

func listToolDefinition() map[string]any {
	return map[string]any{
		"name":        "lexicon_list",
		"description": "List all atoms in the lexicon elements (ID + name + tier + status). Useful for elements-orientation before a targeted query. Returns a JSON array by default; pass format=\"text\" for the legacy TSV.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"format": map[string]any{
					"type":        "string",
					"description": "Output format: \"json\" (default, agent-consumable array of {id,name,tier,status}) or \"text\" (TSV).",
					"enum":        []any{"json", "text"},
				},
				"tier": map[string]any{
					"type":        "string",
					"description": "Optional filter by _tier (e.g. \"primitive\", \"molecule\").",
				},
				"status": map[string]any{
					"type":        "string",
					"description": "Optional filter by status (e.g. \"active\", \"under-review\").",
				},
			},
		},
	}
}

func callRead(renderDir string, arguments json.RawMessage) (string, bool) {
	var args struct {
		Text      string `json:"text"`
		TopK      int    `json:"top_k"`
		NoLens    bool   `json:"no_lens"`
		NoExplain bool   `json:"no_explain"`
		Format    string `json:"format"`
		Detail    bool   `json:"detail"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return fmt.Sprintf("lexicon_read: invalid arguments: %s", err), true
	}
	if args.Text == "" {
		return "lexicon_read: text is required", true
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Sprintf("lexicon_read: resolve self: %s", err), true
	}

	cliArgs := []string{"read"}
	if args.TopK > 0 {
		cliArgs = append(cliArgs, "--top-k", strconv.Itoa(args.TopK))
	}
	if args.NoLens {
		cliArgs = append(cliArgs, "--no-lens")
	}
	if args.NoExplain {
		cliArgs = append(cliArgs, "--no-explain")
	}
	if args.Format != "" {
		cliArgs = append(cliArgs, "--format", args.Format)
	}
	if args.Detail {
		cliArgs = append(cliArgs, "--detail")
	}
	cliArgs = append(cliArgs, "-")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, cliArgs...)
	cmd.Stdin = bytes.NewBufferString(args.Text)
	cmd.Dir = renderDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	out := stdout.String()
	if runErr != nil {
		return fmt.Sprintf("lexicon_read failed: %s\n\nstderr:\n%s\n\nstdout:\n%s", runErr, stderr.String(), out), true
	}
	if out == "" {
		return fmt.Sprintf("lexicon_read produced no output. stderr:\n%s", stderr.String()), true
	}
	return out, false
}

func callList(renderDir string, arguments json.RawMessage) (string, bool) {
	var args struct {
		Format string `json:"format"`
		Tier   string `json:"tier"`
		Status string `json:"status"`
	}
	if len(arguments) > 0 {
		_ = json.Unmarshal(arguments, &args)
	}
	if args.Format == "" {
		args.Format = "json"
	}
	elementsDir := resolveElementsDir(renderDir)
	entries, err := os.ReadDir(elementsDir)
	if err != nil {
		return fmt.Sprintf("lexicon_list: %s", err), true
	}
	type entry struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Tier   string `json:"tier"`
		Status string `json:"status"`
	}
	var rows []entry
	for _, e := range entries {
		name := e.Name()
		if len(name) < 14 || name[:4] != "lex-" || name[len(name)-5:] != ".yaml" {
			continue
		}
		path := filepath.Join(elementsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		id, aname, tier, status := scanAtomMeta(data)
		if id == "" {
			continue
		}
		if args.Tier != "" && tier != args.Tier {
			continue
		}
		if args.Status != "" && status != args.Status {
			continue
		}
		rows = append(rows, entry{ID: id, Name: aname, Tier: tier, Status: status})
	}
	if len(rows) == 0 {
		return "lexicon_list: no atoms found", true
	}
	if args.Format == "text" {
		var buf bytes.Buffer
		for _, r := range rows {
			fmt.Fprintf(&buf, "%s\t%s\t%s\t%s\n", r.ID, r.Name, r.Tier, r.Status)
		}
		return fmt.Sprintf("%d atoms\n\nid\tname\ttier\tstatus\n%s", len(rows), buf.String()), false
	}
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return fmt.Sprintf("lexicon_list: encode: %s", err), true
	}
	return string(b), false
}

// callExtrapolate runs the adjacency-frontier read on the constellation
// of atom IDs in arguments. In-process (mirrors callList pattern) —
// pure elements-graph walk, no need for subprocess isolation.
func callExtrapolate(renderDir string, arguments json.RawMessage) (string, bool) {
	var args struct {
		Atoms []string `json:"atoms"`
		TopK  int      `json:"top_k"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return fmt.Sprintf("lexicon_extrapolate: invalid arguments: %s", err), true
	}
	if len(args.Atoms) == 0 {
		return "lexicon_extrapolate: atoms array is required and must be non-empty", true
	}
	seen := map[string]struct{}{}
	cleaned := make([]string, 0, len(args.Atoms))
	for _, id := range args.Atoms {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, id)
	}
	elementsDir := resolveElementsDir(renderDir)
	atoms, err := loader.LoadAll(elementsDir)
	if err != nil {
		return fmt.Sprintf("lexicon_extrapolate: load elements: %s", err), true
	}
	result := extrapolate.Frontier(cleaned, atoms)
	if args.TopK > 0 && len(result.Candidates) > args.TopK {
		result.Candidates = result.Candidates[:args.TopK]
	}
	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("lexicon_extrapolate: encode: %s", err), true
	}
	return string(out), false
}

// scanAtomMeta is a tiny one-shot scanner over one atom YAML to
// pull out the four top-level fields we surface in lexicon_list.
// Avoids parsing the full YAML — we'd need yaml.v3 in this binary
// otherwise, and these four fields are always at top-level.
func scanAtomMeta(data []byte) (id, name, tier, status string) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case len(line) > 4 && line[:4] == "id: ":
			id = line[4:]
		case len(line) > 6 && line[:6] == "name: ":
			name = line[6:]
		case len(line) > 7 && line[:7] == "_tier: ":
			tier = line[7:]
		case len(line) > 8 && line[:8] == "status: ":
			status = line[8:]
		}
		if id != "" && name != "" && tier != "" && status != "" {
			return
		}
	}
	return
}

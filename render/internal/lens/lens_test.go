package lens

import (
	"fmt"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

func mkEntry(id string, tier, status string) *types.LexEntry {
	return &types.LexEntry{
		ID:      id,
		Name:    "some-atom-name-" + id,
		TypeIn:  "situation",
		TypeOut: "process",
		Tier:    tier,
		Status:  status,
		CanonicalInstances: []string{
			"A reasonably typical canonical instance sentence describing a concrete scenario for " + id + ".",
		},
	}
}

func TestChunkPoolSingleChunkWhenSmall(t *testing.T) {
	pool := []*types.LexEntry{mkEntry("lex-a", "atomic", "active"), mkEntry("lex-b", "atomic", "active")}
	chunks := ChunkPool(pool, 100_000)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if len(chunks[0]) != 2 {
		t.Fatalf("chunk 0 has %d entries, want 2", len(chunks[0]))
	}
}

func TestChunkPoolSplitsOversizedPool(t *testing.T) {
	pool := make([]*types.LexEntry, 0, 200)
	for i := 0; i < 200; i++ {
		pool = append(pool, mkEntry(fmt.Sprintf("lex-%03d", i), "atomic", "active"))
	}
	// Each line is roughly EstimateTokens(indexLine(e)) tokens; pick a
	// budget that forces multiple chunks without knowing the exact line
	// size in advance.
	oneLine := EstimateTokens(indexLine(pool[0]))
	maxTokens := oneLine * 20 // ~20 atoms per chunk
	chunks := ChunkPool(pool, maxTokens)
	if len(chunks) < 5 {
		t.Fatalf("chunks = %d, want several (budget forces splitting)", len(chunks))
	}
	total := 0
	seen := map[string]bool{}
	for _, c := range chunks {
		est := 0
		for _, e := range c {
			est += EstimateTokens(indexLine(e))
			if seen[e.ID] {
				t.Fatalf("atom %s appears in more than one chunk", e.ID)
			}
			seen[e.ID] = true
		}
		if est > maxTokens && len(c) > 1 {
			t.Fatalf("chunk of %d entries estimated at %d tokens, over budget %d", len(c), est, maxTokens)
		}
		total += len(c)
	}
	if total != len(pool) {
		t.Fatalf("chunked total = %d, want %d (no atom dropped)", total, len(pool))
	}
}

func TestChunkPoolOversizedSingleAtomGetsOwnChunk(t *testing.T) {
	e := mkEntry("lex-huge", "atomic", "active")
	line := indexLine(e)
	tinyBudget := EstimateTokens(line) - 5 // smaller than this one atom's own line
	chunks := ChunkPool([]*types.LexEntry{e}, tinyBudget)
	if len(chunks) != 1 || len(chunks[0]) != 1 || chunks[0][0].ID != "lex-huge" {
		t.Fatalf("chunks = %+v, want a single chunk containing the oversized atom rather than dropping it", chunks)
	}
}

func TestChunkPoolSkipsSubAtomicAndDeprecated(t *testing.T) {
	pool := []*types.LexEntry{
		mkEntry("lex-keep", "atomic", "active"),
		mkEntry("lex-sub", "sub-atomic", "active"),
		mkEntry("lex-dead", "atomic", "deprecated"),
	}
	chunks := ChunkPool(pool, 100_000)
	if len(chunks) != 1 || len(chunks[0]) != 1 || chunks[0][0].ID != "lex-keep" {
		t.Fatalf("chunks = %+v, want only lex-keep (sub-atomic/deprecated excluded, matching buildIndex)", chunks)
	}
}

func TestChunkPoolPreservesIDOrderWithinAndAcrossChunks(t *testing.T) {
	pool := []*types.LexEntry{
		mkEntry("lex-c", "atomic", "active"),
		mkEntry("lex-a", "atomic", "active"),
		mkEntry("lex-b", "atomic", "active"),
	}
	chunks := ChunkPool(pool, 100_000)
	var ids []string
	for _, c := range chunks {
		for _, e := range c {
			ids = append(ids, e.ID)
		}
	}
	want := []string{"lex-a", "lex-b", "lex-c"}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("ids = %v, want %v (ID-ascending, matching buildIndex's sort)", ids, want)
		}
	}
}

func TestParseLensResponseObjectShape(t *testing.T) {
	text := `{"picks":[{"id":"lex-0001","confidence":0.9,"suggested_mention":"foo"},{"id":"lex-0002","confidence":0.5}],"stuck_signal":true}`
	out, err := parseLensResponse(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ids) != 2 || out.ids[0] != "lex-0001" || out.ids[1] != "lex-0002" {
		t.Fatalf("ids = %v, want [lex-0001 lex-0002]", out.ids)
	}
	if out.confidences["lex-0001"] != 0.9 {
		t.Fatalf("confidence = %v, want 0.9", out.confidences["lex-0001"])
	}
	if out.topSuggestedMention != "foo" {
		t.Fatalf("topSuggestedMention = %q, want foo", out.topSuggestedMention)
	}
	if !out.stuckSignal {
		t.Fatal("stuckSignal = false, want true")
	}
}

func TestParseLensResponseArrayShape(t *testing.T) {
	text := `[{"id":"lex-0001","confidence":0.7}]`
	out, err := parseLensResponse(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ids) != 1 || out.ids[0] != "lex-0001" {
		t.Fatalf("ids = %v, want [lex-0001]", out.ids)
	}
}

func TestParseLensResponseStringArrayShape(t *testing.T) {
	text := `["lex-0001", "lex-0002"]`
	out, err := parseLensResponse(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ids) != 2 {
		t.Fatalf("ids = %v, want 2 entries", out.ids)
	}
}

func TestParseLensResponseWrappingProse(t *testing.T) {
	text := "Sure, here are the relevant entries:\n" +
		`{"picks":[{"id":"lex-0001","confidence":0.8}]}` +
		"\nLet me know if you need more."
	out, err := parseLensResponse(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ids) != 1 || out.ids[0] != "lex-0001" {
		t.Fatalf("ids = %v, want [lex-0001]", out.ids)
	}
}

// TestParseLensResponseStrayBracesInProse is the case the naive
// first-'{'-to-last-'}' approach mis-slices: wrapping prose that
// itself contains a brace/bracket character after the real JSON ends.
// findMatchingClose must stop at the JSON's own matching closer, not
// the last brace anywhere in the text.
func TestParseLensResponseStrayBracesInProse(t *testing.T) {
	text := `{"picks":[{"id":"lex-0001","confidence":0.8}]}` +
		"\n\nNote: this uses the {curly brace} notation for sets."
	out, err := parseLensResponse(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ids) != 1 || out.ids[0] != "lex-0001" {
		t.Fatalf("ids = %v, want [lex-0001]", out.ids)
	}
}

func TestParseLensResponseTrailingCommaRepair(t *testing.T) {
	text := `{"picks":[{"id":"lex-0001","confidence":0.8},{"id":"lex-0002","confidence":0.6},]}`
	out, err := parseLensResponse(text)
	if err != nil {
		t.Fatalf("unexpected error (trailing comma should be repaired): %v", err)
	}
	if len(out.ids) != 2 {
		t.Fatalf("ids = %v, want 2 entries", out.ids)
	}
}

func TestParseLensResponseTrailingCommaInArrayShape(t *testing.T) {
	text := `[{"id":"lex-0001","confidence":0.8},]`
	out, err := parseLensResponse(text)
	if err != nil {
		t.Fatalf("unexpected error (trailing comma should be repaired): %v", err)
	}
	if len(out.ids) != 1 {
		t.Fatalf("ids = %v, want 1 entry", out.ids)
	}
}

func TestParseLensResponseTruncated(t *testing.T) {
	text := `{"picks":[{"id":"lex-0001","confidence":0.8` // cut off mid-object
	if _, err := parseLensResponse(text); err == nil {
		t.Fatal("expected an error for truncated JSON, got nil")
	}
}

func TestParseLensResponseBraceInsideStringDoesNotConfuseDepth(t *testing.T) {
	text := `{"picks":[{"id":"lex-0001","confidence":0.8,"suggested_mention":"uses a } in prose"}]}`
	out, err := parseLensResponse(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ids) != 1 || out.ids[0] != "lex-0001" {
		t.Fatalf("ids = %v, want [lex-0001]", out.ids)
	}
	if out.topSuggestedMention != "uses a } in prose" {
		t.Fatalf("topSuggestedMention = %q, want the literal brace preserved", out.topSuggestedMention)
	}
}

func TestParseLensResponseNoJSON(t *testing.T) {
	if _, err := parseLensResponse("no json here at all"); err == nil {
		t.Fatal("expected an error for text with no JSON, got nil")
	}
}

func TestParseLensResponseMaxCandidatesCap(t *testing.T) {
	text := `["lex-0001","lex-0002","lex-0003","lex-0004","lex-75r77","lex-nahg9","lex-0007","lex-znaau","lex-jv983","lex-4yhqs"]`
	out, err := parseLensResponse(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ids) != MaxCandidates {
		t.Fatalf("ids has %d entries, want capped at MaxCandidates=%d", len(out.ids), MaxCandidates)
	}
}

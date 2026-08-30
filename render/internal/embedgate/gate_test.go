package embedgate

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

func TestPrototypeText(t *testing.T) {
	e := &types.LexEntry{
		Name:               "test-atom",
		CanonicalInstances: []string{"first instance describing X", "second instance"},
		Evokes:             []string{"alpha", "beta", "gamma"},
	}
	got := PrototypeText(e)
	want := "test-atom\nfirst instance describing X\nalpha, beta, gamma"
	if got != want {
		t.Errorf("PrototypeText = %q, want %q", got, want)
	}
}

func TestPrototypeText_NameOnly(t *testing.T) {
	e := &types.LexEntry{Name: "test-atom"}
	if got := PrototypeText(e); got != "test-atom" {
		t.Errorf("PrototypeText name-only = %q, want %q", got, "test-atom")
	}
}

// TestPrototypeTexts_OneTextPerInstance is the core regression guard for the
// multi-instance rewrite: an atom's overall matchability must not be capped
// by whichever canonical-instance happens to be listed first, so every
// instance needs its own embeddable text.
func TestPrototypeTexts_OneTextPerInstance(t *testing.T) {
	e := &types.LexEntry{
		Name:               "test-atom",
		CanonicalInstances: []string{"first instance describing X", "second instance", "third"},
		Evokes:             []string{"alpha", "beta"},
	}
	got := PrototypeTexts(e)
	want := []string{
		"test-atom\nfirst instance describing X\nalpha, beta",
		"test-atom\nsecond instance\nalpha, beta",
		"test-atom\nthird\nalpha, beta",
	}
	if len(got) != len(want) {
		t.Fatalf("PrototypeTexts returned %d texts, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("PrototypeTexts[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestPrototypeTexts_NoInstancesFallsBackToOne confirms an atom with zero
// canonical-instances (shouldn't happen per schema, but defended against)
// still gets one usable text rather than silently dropping out of the gate.
func TestPrototypeTexts_NoInstancesFallsBackToOne(t *testing.T) {
	e := &types.LexEntry{Name: "test-atom"}
	got := PrototypeTexts(e)
	if len(got) != 1 || got[0] != "test-atom" {
		t.Errorf("PrototypeTexts no-instances = %v, want [\"test-atom\"]", got)
	}
}

func TestCosine(t *testing.T) {
	// Already-normalized orthogonal vectors → 0.
	a := []float64{1, 0, 0}
	b := []float64{0, 1, 0}
	if got := Cosine(a, b); math.Abs(got) > 1e-9 {
		t.Errorf("Cosine(orthogonal) = %v, want ~0", got)
	}
	// Identical vectors → 1.
	if got := Cosine(a, a); math.Abs(got-1) > 1e-9 {
		t.Errorf("Cosine(identical) = %v, want 1", got)
	}
	// Mismatched lengths → -1 sentinel.
	if got := Cosine(a, []float64{1, 0}); got != -1 {
		t.Errorf("Cosine(mismatch) = %v, want -1", got)
	}
}

// TestBestCosine_PicksMaxRegardlessOfOrder is the core regression guard for
// the multi-instance rewrite: the atom's score must be the BEST-matching
// instance's similarity, not the first one's — otherwise an atom whose first
// canonical-instance is a poor embedding anchor stays capped exactly as
// before, and the multi-instance change buys nothing.
func TestBestCosine_PicksMaxRegardlessOfOrder(t *testing.T) {
	query := []float64{1, 0}
	weak := []float64{0, 1}   // orthogonal -> 0
	strong := []float64{1, 0} // identical -> 1
	vectors := [][]float64{weak, strong, weak}
	if got := BestCosine(query, vectors); math.Abs(got-1) > 1e-9 {
		t.Errorf("BestCosine = %v, want 1 (the strong instance, not the first)", got)
	}
}

func TestBestCosine_Empty(t *testing.T) {
	if got := BestCosine([]float64{1, 0}, nil); got >= -1 {
		t.Errorf("BestCosine(no vectors) = %v, want a sentinel below Cosine's [-1,1] range", got)
	}
}

func TestNormalize(t *testing.T) {
	v := []float64{3, 4, 0}
	normalize(v)
	want := []float64{0.6, 0.8, 0}
	for i := range v {
		if math.Abs(v[i]-want[i]) > 1e-9 {
			t.Errorf("normalize[%d] = %v, want %v", i, v[i], want[i])
		}
	}
	// Zero vector stays zero (no division).
	z := []float64{0, 0, 0}
	normalize(z)
	for i := range z {
		if z[i] != 0 {
			t.Errorf("normalize(zero)[%d] = %v, want 0", i, z[i])
		}
	}
}

func TestProtoHash_Stable(t *testing.T) {
	if protoHash("alpha") != protoHash("alpha") {
		t.Errorf("protoHash should be stable for identical input")
	}
}

func TestProtoHash_ChangesWithContent(t *testing.T) {
	if protoHash("alpha") == protoHash("alpha-renamed") {
		t.Errorf("protoHash should change when content changes")
	}
}

// TestPersistPrototypes_RoundTrip checks the atomic writer produces a file the
// loader path can read back for a matching atom ID + content hash.
func TestPersistPrototypes_RoundTrip(t *testing.T) {
	t.Setenv("LEXICON_CACHE_DIR", t.TempDir())
	a := &types.LexEntry{ID: "lex-0001", Name: "alpha"}
	persistCache(protoCache{
		Model: EmbedModel(),
		Entries: map[string]protoEntry{
			"lex-0001": {Hash: protoTextsHash(PrototypeTexts(a)), Vectors: [][]float64{{0.6, 0.8}}},
		},
	})

	// Complete cache + already-cancelled context: must return the cached vectors
	// without attempting any embed (no ollama in the test environment).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := LoadOrBuildPrototypes(ctx, []*types.LexEntry{a})
	if err != nil {
		t.Fatalf("LoadOrBuildPrototypes warm: unexpected error %v", err)
	}
	if vecs, ok := got["lex-0001"]; !ok || len(vecs) != 1 || len(vecs[0]) != 2 {
		t.Errorf("warm cache not returned: got %v", got)
	}
}

// TestPersistPrototypes_MultiInstanceRoundTrip confirms an atom with several
// canonical-instances round-trips ALL of its vectors through the cache, not
// just one — the actual shape change this rewrite makes.
func TestPersistPrototypes_MultiInstanceRoundTrip(t *testing.T) {
	t.Setenv("LEXICON_CACHE_DIR", t.TempDir())
	a := &types.LexEntry{
		ID:                 "lex-0001",
		Name:               "alpha",
		CanonicalInstances: []string{"instance one", "instance two", "instance three"},
	}
	seeded := [][]float64{{1, 0}, {0, 1}, {0.6, 0.8}}
	persistCache(protoCache{
		Model: EmbedModel(),
		Entries: map[string]protoEntry{
			"lex-0001": {Hash: protoTextsHash(PrototypeTexts(a)), Vectors: seeded},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := LoadOrBuildPrototypes(ctx, []*types.LexEntry{a})
	if err != nil {
		t.Fatalf("LoadOrBuildPrototypes warm: unexpected error %v", err)
	}
	vecs, ok := got["lex-0001"]
	if !ok || len(vecs) != 3 {
		t.Fatalf("warm multi-instance cache not returned in full: got %v", got)
	}
	for i, want := range seeded {
		if vecs[i][0] != want[0] || vecs[i][1] != want[1] {
			t.Errorf("vector[%d] = %v, want %v", i, vecs[i], want)
		}
	}
}

// TestLoadOrBuildPrototypes_PartialCacheBestEffort is the regression guard for
// the deadline-resumable build: a tight deadline that cuts a build short must
// still return whatever was already cached (best-effort) rather than erroring —
// otherwise the gate falls back to the full pool forever and silently disables
// itself. We simulate "build was cut short" with an already-cancelled context
// and a cache that holds one of the two atoms' vectors.
func TestLoadOrBuildPrototypes_PartialCacheBestEffort(t *testing.T) {
	t.Setenv("LEXICON_CACHE_DIR", t.TempDir())
	a := &types.LexEntry{ID: "lex-0001", Name: "alpha"}
	b := &types.LexEntry{ID: "lex-0002", Name: "beta"}
	atoms := []*types.LexEntry{a, b}
	// Seed only one atom's entry.
	persistCache(protoCache{
		Model: EmbedModel(),
		Entries: map[string]protoEntry{
			"lex-0001": {Hash: protoTextsHash(PrototypeTexts(a)), Vectors: [][]float64{{1, 0}}},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the missing atom's embed will fail immediately — no ollama needed
	got, err := LoadOrBuildPrototypes(ctx, atoms)
	if err != nil {
		t.Fatalf("partial cache should be best-effort, got error %v", err)
	}
	if _, ok := got["lex-0001"]; !ok {
		t.Errorf("seeded vector lost: got %v", got)
	}
	if _, ok := got["lex-0002"]; ok {
		t.Errorf("uncached atom should be absent when the build was cut short")
	}
}

// TestLoadCachedPrototypes_PartialValidityKeepsRest is the core regression
// guard for the per-atom cache redesign: an atom with a stale or absent entry
// must NOT invalidate the rest of the cache. This is the actual defect the
// whole-elements-hash design had — one new or edited atom threw away
// everything, forcing the hook onto the slow full-pool-lens fallback on every
// mining-pass commit (dispatch msg-0d5dd49e, 2026-07-14).
func TestLoadCachedPrototypes_PartialValidityKeepsRest(t *testing.T) {
	t.Setenv("LEXICON_CACHE_DIR", t.TempDir())
	valid := &types.LexEntry{ID: "lex-0001", Name: "alpha"}
	stale := &types.LexEntry{ID: "lex-0002", Name: "beta-edited"}
	fresh := &types.LexEntry{ID: "lex-0003", Name: "gamma"} // never cached
	persistCache(protoCache{
		Model: EmbedModel(),
		Entries: map[string]protoEntry{
			"lex-0001": {Hash: protoTextsHash(PrototypeTexts(valid)), Vectors: [][]float64{{1, 0}}},
			"lex-0002": {Hash: protoHash("beta-before-edit"), Vectors: [][]float64{{0, 1}}}, // stale hash
		},
	})

	got := loadCachedPrototypes([]*types.LexEntry{valid, stale, fresh})
	if _, ok := got["lex-0001"]; !ok {
		t.Errorf("valid entry should survive: got %v", got)
	}
	if _, ok := got["lex-0002"]; ok {
		t.Errorf("stale-hash entry should be excluded, not just wrong: got %v", got)
	}
	if _, ok := got["lex-0003"]; ok {
		t.Errorf("never-cached atom should be absent: got %v", got)
	}
}

// TestScore_ColdCacheNoBuild is the core guard for the hook-latency fix: Score
// must NEVER build prototypes (a minutes-long ollama job). On a cold cache it
// returns ErrColdCache immediately, before any embedding — so it cannot block
// the hook regardless of ollama state. We assert this with an empty cache dir
// and an un-cancelled context: if Score tried to build/embed it would hang or
// fail on the network; instead it must return ErrColdCache fast.
func TestScore_ColdCacheNoBuild(t *testing.T) {
	t.Setenv("LEXICON_CACHE_DIR", t.TempDir()) // empty → cold
	a := &types.LexEntry{ID: "lex-0001", Name: "alpha"}
	_, err := Score(context.Background(), "some prompt", []*types.LexEntry{a}, 5)
	if !errors.Is(err, ErrColdCache) {
		t.Errorf("cold cache: want ErrColdCache, got %v", err)
	}
}

// TestLoadCachedPrototypes_ModelMismatch confirms a cache built under a
// different embed model is treated as wholesale cold rather than scored
// against — embeddings from different models aren't comparable, so (unlike a
// per-atom content-hash miss) this is the one case that still invalidates
// everything.
func TestLoadCachedPrototypes_ModelMismatch(t *testing.T) {
	t.Setenv("LEXICON_CACHE_DIR", t.TempDir())
	a := &types.LexEntry{ID: "lex-0001", Name: "alpha"}
	persistCache(protoCache{
		Model: "some-other-model",
		Entries: map[string]protoEntry{
			"lex-0001": {Hash: protoTextsHash(PrototypeTexts(a)), Vectors: [][]float64{{1, 0}}},
		},
	})
	if got := loadCachedPrototypes([]*types.LexEntry{a}); got != nil {
		t.Errorf("model mismatch should read as cold, got %v", got)
	}
}

// TestLoadOrBuildPrototypes_NothingAvailableErrors confirms the gate-disabling
// path still surfaces an error when there is genuinely nothing to work with
// (empty cache + a build that can't run), so the caller falls back to the full
// pool rather than narrowing against an empty prototype set.
func TestLoadOrBuildPrototypes_NothingAvailableErrors(t *testing.T) {
	t.Setenv("LEXICON_CACHE_DIR", t.TempDir())
	a := &types.LexEntry{ID: "lex-0001", Name: "alpha"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadOrBuildPrototypes(ctx, []*types.LexEntry{a}); err == nil {
		t.Errorf("expected error when nothing cached and build cannot run")
	}
}

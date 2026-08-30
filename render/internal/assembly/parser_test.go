package assembly

import (
	"testing"
)

// TestParseRealFixtures parses every assembly: string observed in the
// elements today. Round-trip via String() is *not* expected to be
// byte-identical (whitespace/separator normalization), so the
// assertions are structural.
func TestParseRealFixtures(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // expected canonical String() output
	}{
		{
			name: "lex-kebfa sequential→defeasibility",
			src:  "sequential(lex-q9asc, lex-dm5te) → defeasibility-attach(lex-af9ax, defeaters=lex-th68b)",
			want: "sequential(sequential(lex-q9asc, lex-dm5te), defeasibility-attach(lex-af9ax, defeaters=lex-th68b))",
		},
		{
			name: "lex-z8m97 nested parallel in sequential",
			src:  "sequential(lex-9nj6a, parallel(lex-hnfrw, lex-gfxw3))",
			want: "sequential(lex-9nj6a, parallel(lex-hnfrw, lex-gfxw3))",
		},
		{
			name: "lex-a9wpd bare parallel",
			src:  "parallel(lex-gfxw3, lex-hnfrw)",
			want: "parallel(lex-gfxw3, lex-hnfrw)",
		},
		{
			name: "lex-bpr6b choice with selector predicate",
			src:  "sequential(lex-89rjr, choice(lex-ftke9, lex-gx8m2, selector=is-similar-known-problem-recallable))",
			want: "sequential(lex-89rjr, choice(lex-ftke9, lex-gx8m2, selector=is-similar-known-problem-recallable))",
		},
		{
			name: "lex-3ydmv variadic ellipsis + aggregator named-arg",
			src:  "sequential(decompose-into-multiplicative-factors, parallel(lex-axa6h, lex-axa6h, ...; aggregator=multiplicative-product), lex-ds73b)",
			want: "sequential(decompose-into-multiplicative-factors, parallel(lex-axa6h, lex-axa6h, ..., aggregator=multiplicative-product), lex-ds73b)",
		},
		{
			name: "lex-y6pqz bare-name atom + iteration with semicolon-separator",
			src:  "sequential(lex-kkr43, solve-toy-version, iteration(lex-x9pxs; until=corrections-small))",
			want: "sequential(lex-kkr43, solve-toy-version, iteration(lex-x9pxs, until=corrections-small))",
		},
		{
			name: "lex-sjsxx choice with multiple bare-name atoms",
			src:  "sequential(lex-hm8yx, choice(newtonian-framework, relativistic-framework, quantum-framework, quantum-field-theory-framework, selector=identified-scale-regime))",
			want: "sequential(lex-hm8yx, choice(newtonian-framework, relativistic-framework, quantum-framework, quantum-field-theory-framework, selector=identified-scale-regime))",
		},
		{
			name: "lex-mnxhs multiple bare-names + nested choice",
			src:  "sequential(lex-ds73b, identify-conflicting-parameters, lex-spk4s, choice(apply-40-principles; selector=contradiction-matrix-lookup))",
			want: "sequential(lex-ds73b, identify-conflicting-parameters, lex-spk4s, choice(apply-40-principles, selector=contradiction-matrix-lookup))",
		},
		{
			name: "lex-h7vet iteration with two positional args",
			src:  "sequential(lex-utuy6, iteration(lex-hng49, lex-zgmyw), lex-ppq72)",
			want: "sequential(lex-utuy6, iteration(lex-hng49, lex-zgmyw), lex-ppq72)",
		},
		{
			name: "lex-2rsad simple sequential of three atoms",
			src:  "sequential(lex-rcj6y, lex-vacbr, lex-t9fs7)",
			want: "sequential(lex-rcj6y, lex-vacbr, lex-t9fs7)",
		},
		{
			name: "lex-8c35v parallel→choice with named-arg only (no positional)",
			src:  "parallel(lex-hbgcb, lex-5bpee, lex-e98pd) → choice(selector=context_dependent)",
			want: "sequential(parallel(lex-hbgcb, lex-5bpee, lex-e98pd), choice(selector=context_dependent))",
		},
		{
			name: "single atom",
			src:  "lex-0001",
			want: "lex-0001",
		},
		{
			name: "single bare-name leaf",
			src:  "solve-toy-version",
			want: "solve-toy-version",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tc.src, err)
			}
			if got.String() != tc.want {
				t.Errorf("Parse(%q):\n  got:  %s\n  want: %s", tc.src, got.String(), tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"unclosed paren", "sequential(lex-0001"},
		{"empty string", ""},
		{"junk char", "sequential(lex-0001 @ lex-0002)"},
		{"two dots is not ellipsis", "parallel(lex-0001, ..)"},
		{"trailing arrow with nothing after", "lex-0001 →"},
		{"selector with sub-call (predicate must be ident)", "choice(lex-0001, selector=sequential(lex-0002))"},
		{"duplicate named-arg", "iteration(lex-0001; until=foo, until=bar)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.src); err == nil {
				t.Errorf("Parse(%q) expected error, got nil", tc.src)
			}
		})
	}
}

func TestArrowSugar(t *testing.T) {
	// "A → B → C" should normalize to a single sequential(A, B, C),
	// not nested. The grammar here is left-fold-flat per the n-ary
	// sequential semantics in composition-operations.md.
	got, err := Parse("lex-0001 → lex-0002 → lex-0003")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	op, ok := got.(*OpNode)
	if !ok {
		t.Fatalf("expected OpNode, got %T", got)
	}
	if op.Op != "sequential" {
		t.Errorf("expected sequential, got %s", op.Op)
	}
	if len(op.Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(op.Args))
	}
}

func TestCollectAtomIDs(t *testing.T) {
	src := "sequential(lex-q9asc, lex-dm5te) → defeasibility-attach(lex-af9ax, defeaters=lex-th68b)"
	n, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	CollectAtomIDs(n, got)
	want := []string{"lex-q9asc", "lex-dm5te", "lex-af9ax", "lex-th68b"}
	for _, id := range want {
		if !got[id] {
			t.Errorf("expected %s in collected atoms, got %v", id, got)
		}
	}
	if len(got) != 4 {
		t.Errorf("expected 4 atoms, got %d (%v)", len(got), got)
	}
}

func TestCollectMissingNames(t *testing.T) {
	src := "sequential(lex-kkr43, solve-toy-version, iteration(lex-x9pxs; until=corrections-small))"
	n, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	CollectMissingNames(n, got)
	// "corrections-small" is a Predicate (until=), not a MissingLeaf,
	// so only "solve-toy-version" should surface here.
	if !got["solve-toy-version"] {
		t.Errorf("expected solve-toy-version in missing names, got %v", got)
	}
	if got["corrections-small"] {
		t.Errorf("predicate corrections-small should not be in missing names: %v", got)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 missing name, got %d (%v)", len(got), got)
	}
}

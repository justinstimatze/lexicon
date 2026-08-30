package types

import "testing"

// Every case below is drawn from a convention actually present in the
// elements. The unstaked ones accreted across separate audits, and the
// renderers used to test for "MISSING" alone — so all but the first
// reported as VERIFIED on every human-facing surface.
func TestQuoteStaked(t *testing.T) {
	cases := []struct {
		name   string
		quote  string
		staked bool
	}{
		// --- unstaked: the placeholder conventions ---
		{"empty", "", false},
		{"whitespace only", "   ", false},
		{"MISSING placeholder", "[MISSING: verify before activation]", false},
		{"MISSING then NOT VERIFIED", "[MISSING — NOT VERIFIED. No copy in refs/.]", false},
		{"NOT VERIFIED at audit", "[NOT VERIFIED at 27 audit — quote stub (hint: Macquarrie & Robinson 1962)]", false},
		{"NOT VERIFIED REFS-GROUNDED", "[NOT VERIFIED REFS-GROUNDED at 29 audit: verify direct Foucault passage]", false},
		{"paraphrase not verbatim", "[paraphrase, not verbatim — in-copyright. Lakatos Ch.1 §4e (p33)]", false},
		{"MEMORY-LEVEL", "[MEMORY-LEVEL — verify before activation: Collins 1979 documents]", false},
		{"unbracketed sentinel", "MISSING — never staked", false},
		{"leading whitespace before sentinel", "  [NOT VERIFIED at 25 audit — Bloch]", false},

		// --- staked: real verbatim spans ---
		{"plain verbatim", "'If men define situations as real, they are real in their consequences.'", true},
		{"editorial page prefix", "[p.156:] 'difference of meaning correlates with difference of distribution.'", true},
		{"named-source prefix", "[Scott 1998 Introduction, on legibility:] 'The more I examined these efforts'", true},

		// --- regression guard: the bug pointed the other way ---
		// A substring test for "missing"/"not verbatim" would flag these
		// staked spans as unverified. Both are real elements entries
		// (lex-gbru2, lex-tfpzb).
		{"incidental 'missing' in editorial note", "[Plato mining: the asymmetry is what makes it information-bearing. Positive intuition's analog is missing from the elements.] 'the sign always turns me back'", true},
		{"incidental 'missing' in quoted body", "Survivorship bias is the logical error of concentrating on entities that passed a selection process, missing those that did not.", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LineageEntry{Quote: tc.quote}.QuoteStaked()
			if got != tc.staked {
				t.Fatalf("QuoteStaked(%q) = %v, want %v", tc.quote, got, tc.staked)
			}
		})
	}
}

// The sentinels are matched as a prefix of the leading bracketed segment,
// so a sentinel word appearing deep inside a long editorial prefix does
// NOT unstake the entry. This is deliberate: those long prefixes belong to
// other elements conventions (cross-domain consistency notes) whose
// staked-ness is a separate judgement, and silently reclassifying them
// here would be a second unreviewed behaviour change.
func TestQuoteStakedOnlyMatchesSentinelPrefix(t *testing.T) {
	q := "[Cross-domain consistency check, 2026-05-23: one instantiation is missing from the set.] 'the quoted span'"
	if !(LineageEntry{Quote: q}).QuoteStaked() {
		t.Fatal("sentinel word deep in an editorial prefix must not unstake the entry")
	}
}

package lens

import (
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

func TestParsePassageResponse(t *testing.T) {
	text := `Here you go:
{"picks":[{"id":"lex-aaaaa","confidence":0.82,"evidence":"the one, by removing its causes","why":"Splits a remedy into cause-removal and effect-control."},{"id":"lex-bbbbb","confidence":0.7,"evidence":"x","why":"y",},{"id":"lex-aaaaa","confidence":0.5}]}`
	picks, err := parsePassageResponse(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(picks) != 2 {
		t.Fatalf("picks = %d, want 2 (duplicate id dropped)", len(picks))
	}
	if picks[0].ID != "lex-aaaaa" || picks[0].Confidence != 0.82 || picks[0].Evidence != "the one, by removing its causes" {
		t.Fatalf("pick 0 = %+v", picks[0])
	}
	if picks[1].ID != "lex-bbbbb" {
		t.Fatalf("pick 1 = %+v", picks[1])
	}
}

func TestParsePassageResponseEmpty(t *testing.T) {
	picks, err := parsePassageResponse(`{"picks":[]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks = %d, want 0", len(picks))
	}
	if _, err := parsePassageResponse("no json here"); err == nil {
		t.Fatalf("expected error on prose-only response")
	}
}

// The source texts are hard-wrapped with CRLF and use typographic quotes;
// the model quotes them unwrapped with straight quotes. The located span
// must cover the original runes including the line break.
func TestLocateEvidenceAcrossHardWrap(t *testing.T) {
	passage := "It could never be more truly said than of the first remedy,\r\nthat it was worse than the disease. Liberty is to faction what\r\nair is to fire, an aliment without which it instantly expires."
	evidence := "Liberty is to faction what air is to fire"
	s, e, partial, ok := LocateEvidence(passage, evidence)
	if !ok || partial {
		t.Fatalf("ok=%v partial=%v", ok, partial)
	}
	got := string([]rune(passage)[s:e])
	if got != "Liberty is to faction what\r\nair is to fire" {
		t.Fatalf("span = %q", got)
	}
}

func TestLocateEvidenceIgnoresQuoteMarksAndCase(t *testing.T) {
	passage := "He said it was “worse than the disease” — and meant it."
	s, e, partial, ok := LocateEvidence(passage, `'WORSE than the disease' - and meant`)
	if !ok || partial {
		t.Fatalf("ok=%v partial=%v", ok, partial)
	}
	// The located span runs from the first matched letter to the last:
	// the passage's own quote marks fall outside it.
	if got := string([]rune(passage)[s:e]); got != "worse than the disease” — and meant" {
		t.Fatalf("span = %q", got)
	}
	// A straight-quoted source and a single-quoted model quote.
	src := `governments yet to form. "Render unto Cæsar the things which are` + "\r\n" + `Cæsar's" is the scripture doctrine of courts`
	s, e, partial, ok = LocateEvidence(src, `'Render unto Cæsar the things which are Cæsar's' is the scripture doctrine of courts`)
	if !ok || partial {
		t.Fatalf("ok=%v partial=%v", ok, partial)
	}
	if got := string([]rune(src)[s:e]); !strings.HasPrefix(got, "Render unto") || !strings.HasSuffix(got, "doctrine of courts") {
		t.Fatalf("span = %q", got)
	}
}

func TestLocateEvidenceVerseSlash(t *testing.T) {
	passage := "Man knows no Master save creating\r\nHeaven Or those whom choice and common good ordain."
	s, e, partial, ok := LocateEvidence(passage, "Man knows no Master save creating Heaven / Or those whom choice and common good ordain.")
	if !ok || partial {
		t.Fatalf("ok=%v partial=%v", ok, partial)
	}
	if got := string([]rune(passage)[s:e]); !strings.HasSuffix(got, "common good ordain.") {
		t.Fatalf("span = %q", got)
	}
}

func TestLocateEvidenceSuffixFallback(t *testing.T) {
	passage := "mankind; and is a\r\nformal and pompous method of offering up human sacrifices to the\r\npride of tyrants. But this general massacre"
	// Head mis-copied ("it is" for "and is"): the tail anchors.
	s, e, partial, ok := LocateEvidence(passage, "it is a formal and pompous method of offering up human sacrifices to the pride of tyrants")
	if !ok || !partial {
		t.Fatalf("ok=%v partial=%v", ok, partial)
	}
	if got := string([]rune(passage)[s:e]); !strings.HasSuffix(got, "pride of tyrants") {
		t.Fatalf("span = %q", got)
	}
}

func TestLocateEvidencePrefixFallback(t *testing.T) {
	passage := "There are two methods of curing the mischiefs of faction: the one, by removing its causes; the other, by controlling its effects."
	// Tail mis-copied: the head still anchors.
	evidence := "two methods of curing the mischiefs of faction: the one by REMOVING all of its many causes"
	s, e, partial, ok := LocateEvidence(passage, evidence)
	if !ok || !partial {
		t.Fatalf("ok=%v partial=%v", ok, partial)
	}
	got := string([]rune(passage)[s:e])
	if !strings.HasPrefix(got, "two methods of curing the mischiefs of faction") {
		t.Fatalf("span = %q", got)
	}
	if _, _, _, ok := LocateEvidence(passage, "nothing like this appears anywhere in the text at all"); ok {
		t.Fatalf("expected no match")
	}
}

func TestMechanismBriefCutsAtSentence(t *testing.T) {
	e := &types.LexEntry{
		AgentInstruction: "When pursuing a high-stakes objective, don't get attached to any single stratagem as though it were part of the goal itself -- hold only the aim as fixed. What makes this worth naming is how bizarre the individual stratagems look in isolation, and this second sentence runs on well past the cap so the cut must land at the first boundary.",
	}
	got := mechanismBrief(e)
	if !strings.HasSuffix(got, "hold only the aim as fixed.") {
		t.Fatalf("brief = %q", got)
	}
	e2 := &types.LexEntry{AgentInstruction: strings.Repeat("word ", 100)}
	if got := mechanismBrief(e2); len(got) > mechanismMaxCut+4 {
		t.Fatalf("brief not capped: %d chars", len(got))
	}
}

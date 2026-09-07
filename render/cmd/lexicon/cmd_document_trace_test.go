package main

import (
	"testing"
	"unicode/utf8"
)

// TestSplitParagraphsOffsetsAreRuneSafe guards the bug this file's rune
// conversion fixes: splitParagraphs itself tracks byte offsets (it needs
// them for strings.Index), but a frontend slices full_text with JS string
// indexing, which counts UTF-16 code units, not bytes. A caller that wrote
// span.start/span.end straight to JSON would silently misalign every chunk
// after the first multi-byte character -- confirmed live in Common Sense's
// text (æ, ¹, £) and Modest Proposal's (curly quotes).
func TestSplitParagraphsOffsetsAreRuneSafe(t *testing.T) {
	text := "Cæsar's æra.\n\nA very worthy person's plain reply, spoken in confidence, went on for rather a long while so the trailing padding comfortably outruns the byte/rune drift."
	spans := splitParagraphs(text, 1)
	if len(spans) != 2 {
		t.Fatalf("want 2 spans, got %d", len(spans))
	}

	second := spans[1]
	runes := []rune(text)

	// The bug this test guards: a frontend slices full_text with JS string
	// indexing (UTF-16 code units, same as Go's rune count for this BMP-only
	// corpus). Feeding it a raw Go BYTE offset instead is what a naive,
	// unconverted serialization would do -- simulated here by using
	// span.start/span.end directly as rune-array indices.
	buggy := string(runes[second.start:second.end])

	runeStart := utf8.RuneCountInString(text[:second.start])
	runeEnd := utf8.RuneCountInString(text[:second.end])
	fixed := string(runes[runeStart:runeEnd])

	want := "A very worthy person's plain reply, spoken in confidence, went on for rather a long while so the trailing padding comfortably outruns the byte/rune drift."
	if fixed != want {
		t.Fatalf("rune-converted slice = %q, want %q", fixed, want)
	}
	if buggy == fixed {
		t.Fatalf("fixture doesn't actually exercise the byte/rune gap -- unconverted and converted slices agree, so this test proves nothing")
	}
}

func TestExcerptDoesNotSplitAMultiByteRune(t *testing.T) {
	// "æ" is 2 bytes, 1 rune. A byte-slice truncation at length 1 would cut
	// it in half and produce invalid UTF-8; a rune-slice truncation must not.
	got := excerpt("æ", 1)
	if !utf8.ValidString(got) {
		t.Fatalf("excerpt produced invalid UTF-8: %q", got)
	}
	if got != "æ" {
		t.Fatalf("excerpt(%q, 1) = %q, want %q (single rune fits within a 1-rune budget)", "æ", got, "æ")
	}
}

#!/usr/bin/env bash
# OCR a scan whose pages are rotated, using tesseract's own orientation
# detection (--psm 1) rather than trusting the PDF's /Rotate.
#
# The case this was written for: a scan of Judgment Under Uncertainty that
# pdftotext reads as ZERO words, sideways, with handwritten annotations. The
# copy already on disk extracts 8,792 clean words in the WRONG ORDER, because
# it is a three-column Science page and the extractor interleaves the columns.
# An image scan has no reading order to inherit and no columns to confuse --
# OCR rebuilds it from the page geometry -- so a worse-looking file can be the
# better source. Verify that claim per book; do not assume it.
set -uo pipefail
cd "$(dirname "$0")/.."
export TMPDIR="$PWD/.cache/ocr-tmp"      # never inherited; see ocr-partials.sh
mkdir -p "$TMPDIR"
pdf="$1"; out="${pdf%.pdf}.ocr.txt"; d=$(mktemp -d "$TMPDIR/rot-XXXXXX")
pdftoppm -r 300 -png "$pdf" "$d/p" 2>/dev/null
: > "$out"
for f in "$d"/p-*.png; do tesseract "$f" - --psm 1 2>/dev/null >> "$out"; done
rm -rf "$d"
printf '%s: %s words\n' "$(basename "$out")" "$(wc -w < "$out")"

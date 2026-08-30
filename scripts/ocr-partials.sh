#!/usr/bin/env bash
# OCR the PDFs that have a text layer with HOLES in it.
#
# ocr-backlog.sh handles books with no text layer at all. This handles the
# harder case its own queue could not see when it was written: a book that
# extracts thousands of clean words and is still missing whole chapters,
# because some pages in the scan carry text and others are images. Every
# whole-document test -- word count, bytes per word, "has_text_layer" --
# passes such a file, so it is reported as fully readable and every quote
# from a missing page is reported as absent from the book.
#
# Levi-Strauss 1962 is the case that found this: 84,047 words at a healthy
# 174 bytes each, pages 11-18 empty, and those eight pages are the chapter
# that eighteen spans in lex-cf8kh cite. The words "the engineer" appear
# nowhere in the extraction of a book arguing bricoleur-versus-engineer.
#
# The sidecar is APPENDED to the embedded text by verify-quotes.extract, not
# substituted for it, so re-OCR-ing pages that already had good text cannot
# lose a match -- see the comment there.
set -uo pipefail
cd "$(dirname "$0")/.."

# NEVER honour an inherited TMPDIR here. The loop below empties this directory
# between books, so whatever it names gets deleted -- and under Claude Code
# TMPDIR is /home/<user>/.cache/tmp, the root of every session's scratchpad for
# every project. This script was written as a variant of ocr-backlog.sh and the
# one line changed was the line that made it safe; on the first run it wiped
# the scratchpad it was running in, including the audit logs it was producing
# evidence for. ocr-backlog.sh line 28 is the correct pattern: own directory,
# always.
export TMPDIR="$PWD/.cache/ocr-tmp"
mkdir -p "$TMPDIR"

# Delete by the literal path, not through a variable a caller can set. The
# ${VAR:?} guard only catches an EMPTY value; it is no protection at all
# against a value that is a real directory belonging to somebody else.
scratch_clear() { rm -rf "$PWD/.cache/ocr-tmp"/* 2>/dev/null || true; }

mapfile -t targets < <(python3 - <<'PY'
import collections, glob, importlib.util, os, re, subprocess, sys

_spec = importlib.util.spec_from_file_location("vq", "scripts/verify-quotes.py")
vq = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(vq)

LIMIT = 0.15

def blank_fraction(path):
    try:
        out = subprocess.run(["pdftotext", "-layout", path, "-"],
                             capture_output=True, timeout=180).stdout.decode("utf-8", "replace")
    except Exception:
        return 0.0
    pages = out.split("\f")
    if len(pages) < 20:          # too few pages for the ratio to mean anything;
        return 0.0               # one plate in a five-page paper is not a hole
    blank = sum(1 for p in pages if len(p.split()) < 20)
    return blank / len(pages)

STEM = re.compile(r"([A-Za-z]+)-(\d{4})")


def key(fn):
    m = STEM.match(fn)
    return (m.group(1).lower(), m.group(2)) if m else None


def clean_sibling(path, index):
    """Is there already a readable copy of this same book on disk?

    Four of the twenty-five books this script queued on its first run did not
    need OCR at all: Mises 1949 sits beside a 403k-word .azw3, Lukacs 1923
    beside a 164k-word .epub, and BOTH scanned Philosophical Investigations
    beside a 202k-word bilingual PDF. An hour of page rendering each, to
    reconstruct text that was already sitting in the same directory. A blank
    text layer says this FILE is unreadable, not that the BOOK is.
    """
    k = key(os.path.basename(path))
    if not k:
        return None
    for other in index.get(k, ()):
        if other == path or other.endswith(".ocr.txt"):
            continue
        # Use the project's own extractor rather than shelling out per format.
        # The first version called `ebook-convert other /dev/stdout`, which
        # calibre rejects because it infers the output format from the
        # extension and /dev/stdout has none -- so every .epub and .azw3
        # sibling silently failed and Mises and Lukacs stayed in the queue
        # beside the readable copies that were supposed to evict them.
        try:
            n = len(vq.extract(other).split())
        except Exception:
            continue
        if n > 5000:
            return f"{os.path.basename(other)} ({n:,}w)"
    return None


# How many failing quoted spans each source is currently blocking. This is the
# only ordering that matters -- the first run sorted by file size and spent its
# night on woodworking manuals while The Savage Mind, which alone accounts for
# twenty blocked spans, sat tenth in line.
def blocked_spans():
    n = collections.Counter()
    for fn in glob.glob("docs/audits/adj-*.txt"):
        for line in open(fn, encoding="utf-8", errors="replace"):
            m = re.match(r"^lex-\d+ \[.*?\]\s+(\S+)$", line.strip())
            if m:
                n[m.group(1)] += 1
    return n

index = {}
for root, _, files in os.walk("refs"):
    for fn in files:
        k = key(fn)
        if k:
            index.setdefault(k, []).append(os.path.join(root, fn))

blocking = blocked_spans()
rows = []
for root, _, files in os.walk("refs"):
    for fn in files:
        if not fn.lower().endswith(".pdf"):
            continue
        p = os.path.join(root, fn)
        if os.path.exists(os.path.splitext(p)[0] + ".ocr.txt"):
            continue             # ocr-backlog.sh already did this one
        fr = blank_fraction(p)
        if fr <= LIMIT:
            continue
        sib = clean_sibling(p, index)
        if sib:
            print(f"# SKIP {fn}  -- readable copy already on disk: {sib}",
                  file=sys.stderr)
            continue
        rows.append((blocking.get(fn, 0), fr, p))
        print(f"# {fr*100:3.0f}% blank  {blocking.get(fn,0):3d} blocked  {fn}",
              file=sys.stderr)

# Most-blocking first; among equals, smallest file first so the cheap ones land.
for _, _, p in sorted(rows, key=lambda r: (-r[0], os.path.getsize(r[2]))):
    print(p)
PY
)

echo "ocr-partials: ${#targets[@]} PDF(s) with a holed text layer"
ok=0; fail=0
for pdf in "${targets[@]}"; do
  out="${pdf%.pdf}.ocr.txt"
  printf '  %-72s ' "$(basename "$pdf")"
  if timeout 7200 python3 scripts/ocr-pdf.py "$pdf" >/dev/null 2>&1 && [[ -s "$out" ]]; then
    printf 'ok %sw\n' "$(wc -w < "$out")"; ok=$((ok+1))
    rm -f ".cache/quote-verify/$(basename "$pdf").txt"   # stale: pre-OCR text
  else
    printf 'FAILED\n'; fail=$((fail+1))
  fi
  scratch_clear
done
echo "ocr-partials: $ok ok, $fail failed"

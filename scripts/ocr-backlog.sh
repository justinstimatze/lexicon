#!/usr/bin/env bash
# OCR every PDF in refs/ without a usable text layer, leaving the text beside
# its source. "Usable" means BOTH a text layer and page coverage: a scan can
# extract 84,047 healthy-looking words and still be missing a third of its
# pages, and quotes from the missing third are indistinguishable from
# fabrications in any report that only counts mismatches.
#
# 35 of 468 PDFs have no usable text layer, and until they do, every quote
# drawn from them is unverifiable while looking exactly like a verified one
# in any report that counts only mismatches. Among them: Lakatos "Proofs and
# Refutations", Dennett "The Intentional Stance", Polya "How to Solve It",
# Levi-Strauss, Lukacs, Allport "The Psychology of Rumor".
#
# Output goes to refs/<stem>.ocr.txt -- ocr-pdf.py's own default. Polya was
# OCR'd once already and the text is gone, because that run passed --output
# to a path under /tmp. So this passes no --output.
#
# TMPDIR matters more than it looks. ocr-pdf.py renders every page to PNG in
# a tempfile.TemporaryDirectory(), which lands in /tmp, and /tmp on this host
# is a tmpfs -- RAM, not disk. Rendering a 118 MB scan at 300 dpi would put
# gigabytes of PNGs into memory on a 14 GB machine and push it into swap.
# Point TMPDIR at real disk.
#
# Smallest first, so the cheap books land while the expensive ones grind.
set -uo pipefail
cd "$(dirname "$0")/.."

export TMPDIR="${TMPDIR_OVERRIDE:-$PWD/.cache/ocr-tmp}"
mkdir -p "$TMPDIR"

mapfile -t targets < <(
  python3 - <<'PY'
import glob, os, re, subprocess

CAMEL = re.compile(r"([a-z])([A-Z])")
NON = re.compile(r"[^a-z0-9]+")


def toks(stem):
    s = NON.sub(" ", CAMEL.sub(r"\1 \2", stem).lower())
    return {t for t in s.split() if len(t) > 3 and not t.isdigit()}


def blank_fraction(text):
    """Fraction of pages that yielded almost nothing.

    A whole-document word count cannot see a PARTIAL text layer, and partial
    is the case that does real damage, because the document passes every
    has-text check while a third of it is missing. Levi-Strauss "The Savage
    Mind" extracts 84,047 words across 154 pages and looks entirely healthy;
    47 of those pages are blank, and every one of the 24 quotes this corpus
    draws from the missing third reads as though the words are not in the
    book. Wiener's "Cybernetics" is 24% blank and does not contain the word
    "steersman" -- the single most famous word in its introduction.
    """
    pages = text.split("\f")
    if len(pages) < 8:
        return 0.0
    return sum(1 for p in pages if len(p.split()) < 20) / len(pages)


BLANK_LIMIT = 0.15


def pdf_text(path):
    try:
        return subprocess.run(["pdftotext", path, "-"],
                              capture_output=True, timeout=120).stdout.decode("utf-8", "replace")
    except Exception:
        return ""


def readable(path):
    ext = os.path.splitext(path)[1].lower()
    if ext in (".epub", ".txt", ".html", ".mobi", ".azw3", ".djvu"):
        return True
    if ext != ".pdf":
        return False
    t = pdf_text(path)
    n = len(t.split())
    if n < 300 or os.path.getsize(path) / max(1, n) > 2000:
        return False
    return blank_fraction(t) <= BLANK_LIMIT


# Every ref that can already be read, so an image-only PDF with a twin can
# be skipped. This is not a micro-optimisation: Mises "Human Action" is a
# 118 MB scan and the single most expensive job in this batch, and refs/
# already holds Mises-1949-HumanAction.azw3. verify-quotes.py picks the
# longest extraction among a lineage's candidate files, so it has been
# reading the twin all along -- OCRing the scan would add nothing at all.
have = {}
for p in glob.glob("refs/*"):
    if os.path.isfile(p) and not p.endswith(".ocr.txt") and readable(p):
        have[p] = toks(os.path.splitext(os.path.basename(p))[0])

rows = []
for p in sorted(glob.glob("refs/*.pdf")):
    if os.path.exists(os.path.splitext(p)[0] + ".ocr.txt") or p in have:
        continue
    a = toks(os.path.splitext(os.path.basename(p))[0])
    twin = None
    for rp, rt in have.items():
        if len(a & rt) / max(1, min(len(a), len(rt))) >= 0.6:
            twin = rp
            break
    if twin:
        print(f"# skip {os.path.basename(p)} -- readable twin {os.path.basename(twin)}",
              file=__import__("sys").stderr)
        continue
    rows.append((os.path.getsize(p), p))

for _, p in sorted(rows):
    print(p)
PY
)

echo "ocr-backlog: ${#targets[@]} PDF(s) with no usable text layer (none, or >15% blank pages)"
ok=0; fail=0
for pdf in "${targets[@]}"; do
  out="${pdf%.pdf}.ocr.txt"
  printf '  %-72s ' "$(basename "$pdf")"
  if timeout 7200 python3 scripts/ocr-pdf.py "$pdf" >/dev/null 2>&1 && [[ -s "$out" ]]; then
    printf 'ok %sw\n' "$(wc -w < "$out")"; ok=$((ok+1))
  else
    printf 'FAILED\n'; fail=$((fail+1))
  fi
  rm -rf "${TMPDIR:?}"/* 2>/dev/null || true
done
echo "ocr-backlog: $ok ok, $fail failed"

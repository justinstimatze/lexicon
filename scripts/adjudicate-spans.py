#!/usr/bin/env python3
"""Print, for every unmatched span, the point where it leaves the source.

This is the procedure that found every fabrication so far, done by hand
fifteen times and then written down: take the failing span, find the longest
prefix of it that IS in the source, and read what the source says next. If
the source continues with different words, that is a substitution. If it
continues with the cited words plus something wedged between them -- a
footnote marker, a page number, a running head, a column boundary -- that is
an artifact and the citation is fine.

The verdict is not decidable from the numbers, only from those two strings
side by side, so this prints them and classifies nothing it cannot defend:

  DIVERGES   source continues with different words          <- read this
  WEDGE      cited text resumes after intervening characters <- artifact
  NO-ANCHOR  not even the first clause is present           <- read this

Filters exist because the hit rate varies enormously by source type. Quoted
spans against plain text found five defects in twenty-four spans; the same
audit read top to bottom was finding one per fifty. Cheapest first:

  scripts/adjudicate-spans.py --ext .txt          # no scan to blame
  scripts/adjudicate-spans.py --ext .epub,.mobi   # digital, but has markup
  scripts/adjudicate-spans.py --ext .pdf          # everything is an artifact

Usage:
  scripts/adjudicate-spans.py [--ext .epub,.txt] [--atoms lex-xxmeh,...]
                              [--include-unquoted] [-o FILE]
"""
import argparse
import glob
import importlib.util
import os
import re
import sys

_here = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location(
    "vq", os.path.join(_here, "verify-quotes.py"))
vq = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(vq)

CLAUSE = re.compile(r"(?<=[.;?!])\s+|\s*\.\.\.\s*|\s*\[\.\.\.\]\s*")


def present(text, sf):
    f = vq.despace(vq.norm(text))
    return len(f) >= 24 and f in sf


def longest_prefix(text, sf):
    """Longest prefix of text present in the source, by binary search."""
    lo, hi = 0, len(text)
    while lo < hi:
        mid = (lo + hi + 1) // 2
        if present(text[:mid], sf):
            lo = mid
        else:
            hi = mid - 1
    return lo


def source_after(anchor, sn, n=110):
    """What the source says starting at the end of `anchor`."""
    flat_anchor = vq.despace(vq.norm(anchor))
    if not flat_anchor:
        return ""
    # walk sn collecting alnum positions until the anchor is consumed
    keep, seen = [], 0
    for i, ch in enumerate(sn):
        if ch.isalnum():
            keep.append(i)
    flat = "".join(sn[i] for i in keep).lower()
    at = flat.find(flat_anchor)
    if at < 0:
        return ""
    end = keep[min(at + len(flat_anchor), len(keep) - 1)]
    return sn[end:end + n]


def adjudicate(span, sn, sf, out):
    clauses = [c.strip() for c in CLAUSE.split(span) if len(c.strip()) >= 25]
    first_bad = next((c for c in clauses if not present(c, sf)), None)
    if first_bad is None:
        first_bad = span
    n = longest_prefix(first_bad, sf)
    if n < 24:
        out("    NO-ANCHOR  nothing of the failing clause is in the source")
        out(f"      cited   {first_bad[:150]}")
        return "NO-ANCHOR"
    anchor = first_bad[:n]
    cited_next = first_bad[n:n + 110]
    src_next = source_after(anchor, sn)
    # A wedge is when the cited continuation reappears in the source shortly
    # after the divergence point -- something was inserted, nothing replaced.
    # The window has to be generous: what gets wedged in is sometimes a whole
    # footnote plus a running head. Freud's transference passage is broken by
    # "Marcinowski: 'Die erotischen Quellen der Minderwertigkeitsgefuhle',
    # Zeitschrift fur Sexualwissenschaft, 1918, IV. Page 37 22 Beyond the
    # Pleasure Principle" -- 140 characters -- and a short window called that
    # a substitution, which is the expensive direction to be wrong in.
    tail = vq.despace(vq.norm(cited_next[:60]))
    wide = vq.despace(vq.norm(source_after(anchor, sn, 700)))
    kind = "WEDGE" if tail and tail in wide else "DIVERGES"
    out(f"    {kind}")
    out(f"      after   …{anchor[-70:]}".replace("\x01", "'"))
    out(f"      cited   {cited_next}".replace("\x01", "'"))
    out(f"      source  {src_next}".replace("\x01", "'"))
    return kind


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--ext", default="")
    ap.add_argument("--atoms", default="")
    ap.add_argument("--include-unquoted", action="store_true")
    ap.add_argument("--no-ocr", action="store_true",
                    help="never OCR a scan that has no text layer. A full run "
                         "without this looks hung for hours on first pass and "
                         "the process shows no output while it works.")
    ap.add_argument("-o", "--out", default="")
    a = ap.parse_args()
    if a.no_ocr:
        vq.ALLOW_OCR = False
    exts = tuple(e.strip().lower() for e in a.ext.split(",") if e.strip())
    only = {x.strip() for x in a.atoms.split(",") if x.strip()}

    # --ext filters the WINNING ref, which is only known after every candidate
    # has been checked, so a .epub-only run still extracts the PDFs an atom
    # cites alongside them -- and extracting a PDF with no text layer means
    # OCR-ing it, minutes to hours per book. Filtering candidates before the
    # check instead would be wrong: a span that matches in the PDF is matched,
    # and dropping the PDF would report it as a miss. So keep checking
    # everything and just decline the OCR, which is exactly what the
    # pre-commit gate does for the same reason.
    if exts and not any(e.endswith(".pdf") for e in exts):
        vq.ALLOW_OCR = False

    idx = vq.build_ref_index()
    lines = []
    out = lines.append
    counts = {}
    paths = sorted(glob.glob(os.path.join(vq.ROOT, "elements", "lex-*.yaml")))
    for p in paths:
        aid = os.path.basename(p)[:-5]
        if only and aid not in only:
            continue
        try:
            d = vq.load_atom(p)
        except Exception:
            continue
        if not d:
            continue
        for e in d.get("lineage") or []:
            if not isinstance(e, dict) or e.get("source") not in (
                    "primary", "practitioner", "cross-attestation"):
                continue
            spans = vq.spans_of(e, with_kind=True)
            if not spans:
                continue
            # Ask verify-quotes which files the citation names. This used to be
            # a local copy of that logic and the copy went three fixes stale:
            # no platform filter, no citation-aware ranking, no widening. It
            # answered lex-dwycr's Gang-of-Four Strategy quotes with the
            # Wikipedia Falsifiability article and printed every content word
            # "absent" -- which reads as an invented quote and is a misresolved
            # one. Anything that adjudicates has to resolve the same way the
            # thing it adjudicates does.
            usable, why = vq.resolve_usable(e, idx)
            if why:
                continue
            for sp, kind in spans:
                if kind != "quoted" and not a.include_unquoted:
                    continue
                worst = None
                for c, csn, csf in usable:
                    bad = vq.check(sp, csn, csf)
                    if bad is None:
                        worst = None
                        break
                    if worst is None:
                        worst = (c, csn, csf, bad)
                if not worst:
                    continue
                cp, csn, csf, bad = worst
                if exts and not cp.lower().endswith(exts):
                    continue
                out(f"\n{aid} [{d.get('status')}]  {os.path.basename(cp)}")
                v = adjudicate(bad, csn, csf, out)
                counts[v] = counts.get(v, 0) + 1

    body = "\n".join(lines)
    head = (f"{sum(counts.values())} span(s): "
            + ", ".join(f"{k} {v}" for k, v in sorted(counts.items()))
            + "\nDIVERGES and NO-ANCHOR need reading; WEDGE is an artifact.\n")
    if a.out:
        open(a.out, "w", encoding="utf-8").write(head + body + "\n")
        print(head + f"wrote {a.out}")
    else:
        print(head + body)


if __name__ == "__main__":
    main()

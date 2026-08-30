#!/usr/bin/env python3
"""Split verify-quotes.py's misses into "the scan is dirty" and "the words differ".

verify-quotes.py deliberately refuses to guess: a span either matches the
source or it is reported. That is the right call for a gate, and it is useless
for triage, because a 21% miss rate could mean anything between "a few hundred
fabricated quotes" and "a few hundred bad OCR layers".

This separates them by measuring HOW MUCH differs, not whether anything does,
and writes every finding to a review file with the cited text and the source
text side by side. The verdict is triage; the review file is the actual work.

Two earlier versions of this were wrong, and both failures are worth keeping
written down because they were failures of measurement, not of judgement.

  v1 asked whether every 8-word window of a span appeared in the source. Any
     inline footnote marker defeats that -- Coase 1960 extracts as "in my
     previous article2 the case of a confectioner" -- because the marker
     breaks every window straddling it. It called 57% of misses divergent;
     hand-checking three found two false positives.

  v2 anchored with difflib and scored with SequenceMatcher.ratio(), which is
     symmetric: 2*matched/(len(a)+len(b)). The source window is padded by 80
     characters on purpose, so a perfect 1000-char span topped out near 0.96
     and a 200-char span near 0.83. The 0.97 threshold was unreachable. It
     reported ARTIFACT at 0%, piled every finding onto the 0.75 boundary, and
     printed empty diffs -- visibly degenerate output that still had to be
     read carefully to see was degenerate.

  v2 was also unusably slow. find_longest_match with autojunk=False against a
     despaced book is quadratic in practice: 2M characters over ~40 distinct
     symbols means every character indexes ~50,000 positions, so one span
     costs tens of millions of dict lookups. It ran 12 minutes of CPU without
     emitting a line.

So this version anchors by probing fixed-length substrings with str.find and
voting on the implied start, which is linear-ish and finishes; scores with
one-directional coverage (matched characters over CITED length), which the
padding cannot inflate and which reaches 1.0 at any span length; and reports:

  ARTIFACT    >= 0.97 -- a few characters differ, which is scan noise
  DIVERGENT   >= 0.75 -- words differ; a human has to look
  NO-OVERLAP  <  0.75 -- the anchor landed on the wrong occurrence, edition,
                         or work, or found nothing. Not evidence a sentence
                         was invented. Michels 1911 appears in its own
                         editorial introduction as well as its text, and the
                         first hit is the introduction.

Both bounds are judgement calls and neither is safe alone. ARTIFACT means "no
evidence of substitution," not "verified" -- a two-word swap inside a long
passage can score above 0.97. The counts are a claim about scan quality. The
review file is the claim about fidelity, and only after someone reads it.

Usage:
  scripts/classify-quote-misses.py            # whole elements -> review file
  scripts/classify-quote-misses.py lex-y93tk   # named atoms
"""
import collections
import functools
import glob
import importlib.util
import os
import re
import sys
from array import array

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
_spec = importlib.util.spec_from_file_location(
    "vq", os.path.join(os.path.dirname(os.path.abspath(__file__)), "verify-quotes.py"))
vq = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(vq)

REPORT = os.path.join(vq.ROOT, "docs", "audits", "quote-triage.txt")
KEEP = re.compile(r"[a-z0-9\x01]")


def despace_map(t):
    """despace(), plus the index of each surviving character in the original.

    vq.despace throws away where everything was, which is fine for matching
    and useless for a report: a despaced source window reads as an unbroken
    wall of letters that no one can compare against a quote by eye. The map
    lets an offset in despaced space be turned back into readable text.
    """
    low = t.lower()
    out, pos = [], array("i")
    for m in KEEP.finditer(low):
        out.append(m.group())
        pos.append(m.start())
    return "".join(out), pos


@functools.lru_cache(maxsize=6)
def source_map(path, sn):
    """Cached per source. A book's map is ~4 bytes per character, so this is
    tens of MB for a big scan -- small cache on purpose."""
    return despace_map(sn)


def anchor(span, src):
    """Where in src does span most likely sit? Probe and vote.

    Take fixed-length probes along the span, ask str.find (which is fast and
    written in C) where each occurs, and let each hit vote for the implied
    start of the span. A real match votes consistently; a probe that happens
    to be a common phrase votes somewhere unrelated and is outvoted. Falls
    back to shorter probes when the passage is short or the scan is dirty
    enough that no 48-character run survives intact.
    """
    n = len(span)
    for L in (48, 32, 24):
        if n < L:
            continue
        step = max(8, (n - L) // 12) or 1
        starts = []
        for off in range(0, n - L + 1, step):
            probe = span[off:off + L]
            at, hits = 0, 0
            while hits < 6:
                p = src.find(probe, at)
                if p < 0:
                    break
                starts.append(p - off)
                at, hits = p + 1, hits + 1
        if not starts:
            continue
        # densest cluster wins: insertions shift the implied start a little,
        # so exact agreement is the wrong test.
        starts.sort()
        best, bestn, j = starts[0], 0, 0
        for i, s in enumerate(starts):
            while starts[j] < s - 64:
                j += 1
            if i - j + 1 > bestn:
                best, bestn = s, i - j + 1
        return best, bestn
    return None, 0


def readable(flat_lo, flat_hi, pos, text):
    """A despaced range, as the text it came from."""
    if flat_hi <= flat_lo or not len(pos):
        return ""
    lo = pos[max(0, min(flat_lo, len(pos) - 1))]
    hi = pos[max(0, min(flat_hi - 1, len(pos) - 1))] + 1
    return text[lo:hi]


def align(span_norm, sn, path):
    """Verdict, coverage, and a readable cited/source pair for the report."""
    import difflib

    span_flat, span_pos = despace_map(span_norm)
    src_flat, src_pos = source_map(path, sn)
    if len(span_flat) < 30:
        return "NO-OVERLAP", 0.0, "", ""

    start, votes = anchor(span_flat, src_flat)
    if start is None:
        return "NO-OVERLAP", 0.0, "", ""

    lo = max(0, start - 40)
    hi = min(len(src_flat), start + len(span_flat) + 40)
    win_flat = src_flat[lo:hi]

    sm = difflib.SequenceMatcher(None, span_flat, win_flat, autojunk=False)
    blocks = sm.get_matching_blocks()
    # COVERAGE, not ratio: how much of the CITED span appears in the source.
    # Padding on the window cannot inflate it, and a perfect quote scores 1.0
    # at any length -- neither of which is true of SequenceMatcher.ratio().
    cov = sum(b.size for b in blocks) / max(1, len(span_flat))

    src_text = readable(lo, hi, src_pos, sn)
    if cov >= 0.97:
        return "ARTIFACT", cov, "", src_text
    if cov < 0.75:
        return "NO-OVERLAP", cov, "", src_text

    # Name the KIND of difference, which is what separates a dirty scan from a
    # rewritten quote and is not visible in the coverage number alone:
    #
    #   SPLICE-IN     the source has text the citation does not, at one spot.
    #                 A running head, footnote, or copyright line dropped into
    #                 the middle of a sentence by the extractor. Benign.
    #   CITED-EXTRA   the citation has text the source does not. Either the
    #                 extractor lost a page break, or words were supplied.
    #   SUBSTITUTION  both sides have text at the same position and they
    #                 differ. This is the shape of every fabrication found so
    #                 far -- Caillois's "dexterity", Mill's "white",
    #                 Nguyen's "grades" -- and the one to read first.
    #
    # The earlier version reported only the first opcode longer than six
    # characters, so a span differing by many small scattered edits (the OCR
    # signature) printed an empty diff and looked like a tooling failure.
    kind, best, detail = "SCATTERED", 0, ""
    for tag, i1, i2, j1, j2 in sm.get_opcodes():
        if tag == "equal":
            continue
        a, b = i2 - i1, j2 - j1
        if max(a, b) <= best:
            continue
        best = max(a, b)
        kind = ("SUBSTITUTION" if a >= 4 and b >= 4 else
                "CITED-EXTRA" if a > b else "SPLICE-IN")
        cited = readable(max(0, i1 - 30), i2 + 25, span_pos, span_norm)
        found = readable(lo + max(0, j1 - 30), lo + j2 + 25, src_pos, sn)
        detail = f"[{kind}] cited …{cited}…\n          source …{found}…"
    return f"DIVERGENT/{kind}", cov, detail, src_text


def main():
    only = {a for a in sys.argv[1:] if a.startswith("lex-")}
    if "--no-ocr" in sys.argv:
        # Same reason verify-quotes.py has the flag: extract() renders a
        # scanned PDF at 300 DPI when it finds no text layer, and the parent
        # sits at zero CPU for the duration, which is indistinguishable from
        # a hang. An existing sidecar is still read.
        vq.OCR_ENABLED = False
    idx = vq.build_ref_index()
    verdicts = collections.Counter()
    by_ref = collections.Counter()
    kinds = collections.Counter()
    rows = []

    paths = sorted(glob.glob(os.path.join(vq.ROOT, "elements", "*.yaml")))
    for p in paths:
        if only and os.path.basename(p)[:-5] not in only:
            continue
        try:
            d = vq.load_atom(p)
        except Exception as exc:
            print(f"  !! unparseable {os.path.basename(p)}: {exc}", file=sys.stderr)
            continue
        if not d:
            continue
        for e in d.get("lineage") or []:
            if not isinstance(e, dict) or e.get("source") not in (
                    "primary", "practitioner", "cross-attestation"):
                continue
            spans = vq.spans_of(e)
            if not spans:
                continue
            # resolve_usable, NOT a local author_year_keys + pick_ref. Its own
            # docstring says a copy of these rules three fixes behind is how a
            # verifier quietly stops verifying, and this file was carrying one:
            # it ignored .refs-pins.tsv entirely, so every Nimzowitsch span was
            # aligned against the 2016 translation while the pin says Hays 1991
            # -- 25 findings at the top of the non-artifact list, all of them
            # the wrong book rather than a wrong quotation. It also kept a
            # 300-word floor that verify-quotes.py lowered to 120, which
            # silently skipped every short abstract and xkcd transcript.
            usable, _why = vq.resolve_usable(e, idx)
            if not usable:
                continue
            for span in spans:
                # A miss is a miss against EVERY usable copy, and the one to
                # show is whichever got furthest -- the shortest unmatched
                # remainder -- not whichever happened to rank first.
                worst = None
                for path, sn_i, sf_i in usable:
                    bad_i = vq.check(span, sn_i, sf_i)
                    if not bad_i:
                        worst = None
                        break
                    if worst is None or len(bad_i) < len(worst[0]):
                        worst = (bad_i, path, sn_i)
                if worst is None:
                    continue
                bad, bestpath, sn = worst
                verdict, cov, detail, src = align(vq.norm(bad), sn, bestpath)
                verdicts[verdict.split("/")[0]] += 1
                if "/" in verdict:
                    kinds[verdict.split("/")[1]] += 1
                ref = os.path.basename(bestpath)
                if not verdict.startswith("ARTIFACT"):
                    by_ref[ref] += 1
                rows.append((verdict, cov, d["id"], d.get("status"), ref,
                             e.get("text") or "", bad, detail, src))

    total = sum(verdicts.values())
    print(f"{total} unmatched span(s) classified")
    for k in ("ARTIFACT", "DIVERGENT", "NO-OVERLAP"):
        n = verdicts[k]
        print(f"  {k:<11} {n:>4}  {100 * n / max(1, total):.0f}%")
    print("  divergent, by kind of difference:")
    for k, n in kinds.most_common():
        print(f"    {k:<14} {n:>4}")

    # SUBSTITUTION first within DIVERGENT: it is the shape every fabrication
    # found so far has had, and the only bucket where reading order matters.
    rank = {"DIVERGENT": 0, "NO-OVERLAP": 1, "ARTIFACT": 2}
    sub = {"SUBSTITUTION": 0, "CITED-EXTRA": 1, "SCATTERED": 2, "SPLICE-IN": 3}
    rows.sort(key=lambda r: (rank[r[0].split("/")[0]],
                             sub.get(r[0].split("/")[-1], 9), -r[1]))

    if only:
        # A named-atom run is a spot check and REPORT is the whole
        # elements'. Writing it here replaces the file with however few
        # rows the subset produced, and a truncated report reads exactly
        # like a clean one. Its sibling triage-no-overlap.py lost 399 lines
        # of tracked audit this way on 2026-08-06.
        print(f"\n(subset run of {len(only)} atom(s) -- {REPORT} left alone)")
        return
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    with open(REPORT, "w", encoding="utf-8") as fh:
        fh.write("quote triage -- every span verify-quotes.py could not match\n")
        fh.write("DIVERGENT first (words differ), then NO-OVERLAP (anchor missed),\n")
        fh.write("then ARTIFACT (scan noise). ARTIFACT is not a clearance.\n\n")
        for k in ("ARTIFACT", "DIVERGENT", "NO-OVERLAP"):
            fh.write(f"  {k:<11} {verdicts[k]}\n")
        for k, n in kinds.most_common():
            fh.write(f"    divergent/{k:<14} {n}\n")
        fh.write("\nrefs with the most non-artifact misses (a bad scan is ONE finding):\n")
        for ref, n in by_ref.most_common(25):
            fh.write(f"  {n:>4}  {ref}\n")
        fh.write("\n" + "=" * 78 + "\n")
        for verdict, cov, aid, status, ref, text, bad, detail, src in rows:
            fh.write(f"\n{aid} [{status}] {verdict} {cov:.2f}\n")
            fh.write(f"  lineage {text}\n  ref     {ref}\n")
            fh.write(f"  cited   {bad[:600]}\n".replace("\x01", "'"))
            if detail:
                fh.write(f"  diff    {detail[:600]}\n".replace("\x01", "'"))
            if src:
                fh.write(f"  source  {src[:600]}\n".replace("\x01", "'"))
    print(f"\nwrote {REPORT} ({len(rows)} findings)")
    print("\nrefs with the most non-artifact misses:")
    for ref, n in by_ref.most_common(12):
        print(f"  {n:>4}  {ref}")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Split the NO-OVERLAP bucket into "bad scan" and "not in this book".

classify-quote-misses.py anchors a cited span by probing 24-to-48-character
substrings and voting on the implied start. When no probe lands anywhere, it
returns NO-OVERLAP and stops, which is honest and uninformative: 231 spans
came back that way and NO-OVERLAP covers three completely different
situations that call for three different responses.

  the scan is shredded    The words ARE in the book. Column interleaving,
                          OCR damage, or hyphenation broke every run long
                          enough to probe with. Fix the extraction.
  the wrong copy          The words are not in THIS file but the passage is
                          real -- a different translation, edition, or
                          abridgement, or a citation pointing at the wrong
                          file. Fix the pointer or acquire the right copy.
  not in the book         The words are not there in any arrangement. Read
                          it by hand; this is where a fabrication would sit.

The discriminator is fragment density: what fraction of a span's short
shingles occur ANYWHERE in the source, regardless of order or position. A
shredded scan still contains almost every 16-character fragment of the
passage -- shredding relocates text, it does not delete it. A passage that
was never in the book contains almost none.

Sixteen characters is chosen to be long enough that a hit means something
(random English does not accidentally contain a given 16-character run) and
short enough to survive between two injected artifacts. The longest run of
consecutive hits approximates the longest common substring without paying
for one.

  DENSE   >= 0.70   the text is in there, badly arranged -- an extraction bug
  PARTIAL >= 0.25   some of it is; likely a different edition or translation
  ABSENT  <  0.25   the words are not in this file. READ THIS BY HAND.

Usage:
  scripts/triage-no-overlap.py            # whole elements
  scripts/triage-no-overlap.py lex-y93tk   # named atoms
"""
import collections
import glob
import importlib.util
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
_spec = importlib.util.spec_from_file_location(
    "vq", os.path.join(os.path.dirname(os.path.abspath(__file__)), "verify-quotes.py"))
vq = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(vq)

_cq = importlib.util.spec_from_file_location(
    "cq", os.path.join(os.path.dirname(os.path.abspath(__file__)), "classify-quote-misses.py"))
cq = importlib.util.module_from_spec(_cq)
_cq.loader.exec_module(cq)

REPORT = os.path.join(vq.ROOT, "docs", "audits", "quote-no-overlap.txt")
SHINGLE = 16
STEP = 6


def density(span_flat, src_flat):
    """Fraction of the span's shingles present anywhere in the source, and the
    longest run of consecutive ones."""
    if len(span_flat) < SHINGLE:
        return 0.0, 0, 0
    hits, total, run, best = [], 0, 0, 0
    for off in range(0, len(span_flat) - SHINGLE + 1, STEP):
        total += 1
        if src_flat.find(span_flat[off:off + SHINGLE]) >= 0:
            hits.append(off)
            run += 1
            best = max(best, run)
        else:
            run = 0
    return len(hits) / max(1, total), best * STEP + SHINGLE if best else 0, total


import re as _re

_CAMEL = _re.compile(r"([a-z])([A-Z])")
_NON = _re.compile(r"[^a-z0-9]+")
_STOP = {"the", "and", "of", "a", "an", "in", "on", "to", "for", "vol", "volume",
         "trans", "ed", "edition", "press", "university", "part", "book"}


def title_tokens(s):
    s = _NON.sub(" ", _CAMEL.sub(r"\1 \2", s).lower())
    return {t for t in s.split()
            if len(t) > 2 and not t.isdigit() and t not in _STOP}


def wrong_file(lineage_text, ref_name):
    """Did the checker resolve this citation to a different WORK?

    author_year_keys() matches on surname and year and nothing else, and
    picks whichever candidate extracts longest. Foucault published all three
    volumes of The History of Sexuality in 1984, and this corpus holds only
    volume 3. Every quote lex-ggyt6 draws from volume 2, The Use of Pleasure,
    was checked against The Care of the Self and reported as not found --
    thirteen findings that say nothing about the citation and everything
    about which book was on the shelf.

    "Not in this file" and "not in this work" need separate names, because
    the first is a citation defect and the second is a procurement gap.
    """
    # The surname is present in BOTH strings by construction -- it is what
    # matched them -- so it has to come out before asking whether the titles
    # agree, or nothing is ever a mismatch. Same for the file extension.
    stem = os.path.splitext(ref_name)[0]
    drop = set()
    for src in (lineage_text, stem):
        for k in vq.author_year_keys(src):
            drop.add(k.rsplit("-", 1)[0])
    a = title_tokens(lineage_text) - drop
    b = title_tokens(stem) - drop
    if not a or not b:
        return False          # one side carries no title at all; can't tell
    return not (a & b)


WORD = __import__("re").compile(r"[a-z]{8,}")


def cluster(span_norm, src_flat):
    """Localize a span by its distinctive words instead of its character runs.

    Shingle density answers the wrong question when the SOURCE ITSELF is OCR
    output. The Savage Mind's text layer renders "we can now see why" as "wo
    cnn now see why" and "to grasp the world" as "to grnep tho wol'ld" -- a
    character error every few words. No 16-character shingle survives that, so
    a real passage in a damaged scan scores the same zero as an invented one,
    and the whole discriminator collapses exactly where the risk is highest.

    Long words are the durable signal. OCR damages some of them and leaves
    most intact, and word presence does not care about order or spacing. But
    presence alone is not enough either -- "theoretical" and "phenomena" both
    occur all over a book of philosophy, so an invented passage assembled from
    the book's own vocabulary would score high on presence.

    So this asks where the words CO-OCCUR: the fraction of the span's distinct
    8+ character words that fall inside a single window three times the span's
    length. A real passage concentrates its rare vocabulary in one place. An
    invented one scatters it across the whole book.
    """
    words = {w for w in WORD.findall(span_norm.lower())}
    if len(words) < 4:
        return 0.0
    events = []
    for w in words:
        at, hits = 0, 0
        while hits < 40:
            i = src_flat.find(w, at)
            if i < 0:
                break
            events.append((i, w))
            at, hits = i + 1, hits + 1
    if not events:
        return 0.0
    events.sort()
    win = 3 * max(1, len(span_norm))
    seen, best, j = collections.Counter(), 0, 0
    for i, (p, w) in enumerate(events):
        seen[w] += 1
        while events[j][0] < p - win:
            seen[events[j][1]] -= 1
            if seen[events[j][1]] == 0:
                del seen[events[j][1]]
            j += 1
        best = max(best, len(seen))
    return best / len(words)


def main():
    only = {a for a in sys.argv[1:] if a.startswith("lex-")}
    if "--no-ocr" in sys.argv:
        # extract() renders a scanned PDF at 300 DPI when it finds no text
        # layer, and the parent sits at zero CPU throughout, which is
        # indistinguishable from a hang. An existing sidecar is still read.
        vq.OCR_ENABLED = False
    idx = vq.build_ref_index()
    verdicts = collections.Counter()
    by_ref = collections.Counter()
    rows = []

    for p in sorted(glob.glob(os.path.join(vq.ROOT, "elements", "*.yaml"))):
        if only and os.path.basename(p)[:-5] not in only:
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
            spans = vq.spans_of(e)
            if not spans:
                continue
            # resolve_usable, which reads .refs-pins.tsv. It keeps the property
            # this block was written for -- every source the lineage names, not
            # just pick_ref's guess, because lex-ukgke cites two Doctorow posts
            # and quotes the famous opening from the second, and checking only
            # the first reported four fabrications in an atom that cites both
            # correctly. What it adds is the pins: without them a span present
            # ONLY in a wrong edition counted as present, which is the quiet
            # direction to be wrong in. It also drops a 300-word floor that
            # verify-quotes.py lowered to 120, which had been skipping every
            # short abstract and transcript outright.
            usable, _why = vq.resolve_usable(e, idx)
            if not usable:
                continue
            for span in spans:
                miss = None
                for cp, csn, csf in usable:
                    bad = vq.check(span, csn, csf)
                    if bad is None:
                        miss = None
                        break
                    span_flat, _ = cq.despace_map(vq.norm(bad))
                    if len(span_flat) < 30 or cq.anchor(span_flat, csf)[0] is not None:
                        miss = None
                        break
                    d0 = density(span_flat, csf)
                    if miss is None or d0[0] > miss[0][0]:
                        miss = (d0, cp, csn, csf, bad, span_flat)
                if miss is None:
                    continue
                (dens, longest, total), bestpath, sn, sf, bad, span_flat = miss
                ref0 = os.path.basename(bestpath)
                clus = cluster(vq.norm(bad), sf) if dens < 0.70 else 1.0
                # Order matters: a clean scan answers on shingles, a damaged
                # one can only answer on word co-occurrence, and a span that
                # fails BOTH is the one to read by hand.
                v = ("DENSE" if dens >= 0.70 else
                     "OCR-DAMAGED" if clus >= 0.60 else
                     "PARTIAL" if clus >= 0.30 else
                     "WRONG-FILE" if wrong_file(e.get("text") or "", ref0) else
                     "ABSENT")
                verdicts[v] += 1
                ref = os.path.basename(bestpath)
                if v in ("ABSENT", "WRONG-FILE", "PARTIAL"):
                    by_ref[ref] += 1
                rows.append((v, dens, clus, longest, d["id"], d.get("status"), ref,
                             e.get("text") or "", bad))

    total = sum(verdicts.values())
    print(f"{total} NO-OVERLAP span(s)")
    for k in ("ABSENT", "WRONG-FILE", "PARTIAL", "OCR-DAMAGED", "DENSE"):
        n = verdicts[k]
        print(f"  {k:<8} {n:>4}  {100 * n / max(1, total):.0f}%")

    rank = {"ABSENT": 0, "WRONG-FILE": 1, "PARTIAL": 2, "OCR-DAMAGED": 3, "DENSE": 4}
    rows.sort(key=lambda r: (rank[r[0]], r[2]))
    if only:
        # A named-atom run is a spot check, and the report is the whole
        # elements'. Writing it here replaces 449 lines with however few
        # the subset produced, and the result reads like a clean sweep
        # rather than like a truncation. Print and stop.
        print(f"\n(subset run of {len(only)} atom(s) -- {REPORT} left alone)")
        return
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    with open(REPORT, "w", encoding="utf-8") as fh:
        fh.write("NO-OVERLAP triage -- spans whose anchor found nothing.\n"
                 "ABSENT means the words are not in this file in any arrangement,\n"
                 "which is where a fabrication would sit. Read those by hand.\n"
                 "DENSE means the text IS there and the extraction shredded it.\n\n")
        for k in ("ABSENT", "WRONG-FILE", "PARTIAL", "OCR-DAMAGED", "DENSE"):
            fh.write(f"  {k:<12} {verdicts[k]}\n")
        fh.write("\nrefs with the most non-DENSE spans:\n")
        for ref, n in by_ref.most_common(30):
            fh.write(f"  {n:>4}  {ref}\n")
        fh.write("\n" + "=" * 78 + "\n")
        for v, dens, clus, longest, aid, status, ref, text, bad in rows:
            fh.write(f"\n{aid} [{status}] {v} shingles={dens:.2f} word-cluster={clus:.2f} longest-run={longest}c\n")
            fh.write(f"  lineage {text}\n  ref     {ref}\n")
            fh.write(f"  cited   {bad[:600]}\n".replace("\x01", "'"))
    print(f"\nwrote {REPORT}")
    print("\nrefs with the most non-DENSE spans:")
    for ref, n in by_ref.most_common(12):
        print(f"  {n:>4}  {ref}")


if __name__ == "__main__":
    main()

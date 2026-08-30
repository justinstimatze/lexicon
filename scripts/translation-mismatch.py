#!/usr/bin/env python3
"""Find atoms whose quotes systematically disagree with the source they name.

Three atoms have now been found quoting a DIFFERENT translation than the one
their citation names, with the named translation sitting on disk the whole
time: lex-rxvus named Hubback 1922 and quoted Strachey, lex-ynqj9 named Cunnison
three times and quoted Halls 1990, lex-ykb5a named Haldane & Kemp and quoted
Payne. Each looked like a scatter of unrelated one-word substitutions, so
nothing named it as ONE defect, and each took a separate hand-investigation.

The signal is not the translator, it is the CONCENTRATION. A scan artifact
hits one span; a wrong translation hits every span in the entry at once. So
this ranks (atom, source) pairs by how many of their quoted spans fail, and
prints the translator the citation names when it names one, because that is
what the reader has to check the words against.

It is built to be able to come back empty. A high count is a place to look,
not a verdict -- a badly scanned book concentrates failures too, which is why
the report prints how many of the failures are WEDGE-shaped (something
inserted, citation fine) alongside the total.

Usage:
  scripts/translation-mismatch.py            # whole elements
  scripts/translation-mismatch.py --min 2    # threshold (default 2)
"""
import argparse
import glob
import importlib.util
import os
import re

_here = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location(
    "vq", os.path.join(_here, "verify-quotes.py"))
vq = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(vq)

_aspec = importlib.util.spec_from_file_location(
    "adj", os.path.join(_here, "adjudicate-spans.py"))

# "tr. Joan Stambaugh", "trans. Ian Cunnison", "translated by W. D. Halls",
# "Haldane & Kemp", "-Dryden1683" in a filename.
TRANSLATOR = re.compile(
    r"(?:tr(?:ans)?\.|translated by|translation(?:\s+by)?|,\s*tr\b)\s*"
    r"([A-Z][\w.'’-]+(?:\s+(?:&|and)\s+)?(?:\s+[A-Z][\w.'’-]+){0,3})")


def translators(citation):
    out = []
    for m in TRANSLATOR.finditer(citation or ""):
        name = m.group(1).strip(" .,")
        if name and name.lower() not in {"the", "by"}:
            out.append(name)
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--min", type=int, default=2)
    a = ap.parse_args()

    idx = vq.build_ref_index()
    rows = []
    for p in sorted(glob.glob(os.path.join(vq.ROOT, "elements", "lex-*.yaml"))):
        aid = os.path.basename(p)[:-5]
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
            spans = [(s, k) for s, k in vq.spans_of(e, with_kind=True) if k == "quoted"]
            if len(spans) < a.min:
                continue
            cands = []
            for k in vq.author_year_keys(e.get("text") or ""):
                cands += idx.get(k, [])
            usable = []
            for c in vq.rank_refs(e.get("text") or "", cands)[:4]:
                t = vq.extract(c)
                nw = len(t.split())
                if nw < 300 or os.path.getsize(c) / max(1, nw) > 2000:
                    continue
                sn = vq.norm(t, True)
                usable.append((c, sn, vq.despace(sn)))
            if not usable:
                continue
            fails, wedges, worst_ref = 0, 0, None
            for sp, _ in spans:
                bad = keep = None
                for c, sn, sf in usable:
                    b = vq.check(sp, sn, sf)
                    if b is None:
                        bad = None
                        break
                    if bad is None:
                        bad, keep = b, (c, sn, sf)
                if bad is None:
                    continue
                fails += 1
                worst_ref = worst_ref or os.path.basename(keep[0])
                # Is the cited text merely interrupted? Cheap version of
                # adjudicate-spans' WEDGE: does a later chunk of the failing
                # text reappear close by in the source?
                tail = vq.despace(vq.norm(bad[-70:]))
                if len(tail) >= 24 and tail in keep[2]:
                    wedges += 1
            if fails >= a.min:
                rows.append((fails - wedges, fails, wedges, len(spans), aid,
                             worst_ref, translators(e.get("citation") or "")))

    rows.sort(reverse=True)
    print(f"{len(rows)} (atom, source) pair(s) with {a.min}+ failing quoted spans\n")
    print(f"{'hard':>4} {'fail':>4} {'wedge':>5} {'of':>3}  atom       source / translator named")
    for hard, fails, wedges, total, aid, ref, tr in rows:
        who = ("  [names: " + "; ".join(tr) + "]") if tr else ""
        print(f"{hard:4d} {fails:4d} {wedges:5d} {total:3d}  {aid}  {ref}{who}")
    print("\nhard = failures that are NOT merely interrupted text. A pair with a high\n"
          "hard count and a translator named is where a wrong-translation atom looks\n"
          "like one defect instead of many.")


if __name__ == "__main__":
    main()

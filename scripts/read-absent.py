#!/usr/bin/env python3
"""Find WHERE a cited span stops matching its source, clause by clause.

The triage scripts sort spans into buckets. Adjudicating one still means
opening the book, and every fabrication found so far was caught the same way:
split the passage into clauses, check each on its own, and for the first
clause that fails, find the longest prefix that IS in the source and read
what the source says next. Mill's "The word white, when predicated of a
thing" gave way at the first clause and the source read "The name, therefore".
Woolf's "One sees that she is hampered and distracted" gave way after "one
sees that she" and the source read "will never get her genius expressed whole
and entire". Clance & Imes gave way at the sample description.

That procedure is mechanical, so this does it for every span that
verify-quotes.py cannot match:

  cited   the failing span
  clause  FOUND / MISS for each clause independently -- a span that fails as
          a whole but whose clauses all pass is a JOIN defect (reordering, a
          dropped sentence, an unmarked elision), not a substitution
  give    the longest prefix of the first failing clause that is present,
          then what the source actually continues with

The last line is the one that adjudicates. If the source continues with
different words, that is a substitution. If it continues with the cited words
after an injected running head, that is a scan artifact.

Usage:
  scripts/read-absent.py lex-brcgz lex-zeznb     # named atoms
  scripts/read-absent.py --from-report         # every atom with an ABSENT span
"""
import glob
import importlib.util
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
_spec = importlib.util.spec_from_file_location(
    "vq", os.path.join(os.path.dirname(os.path.abspath(__file__)), "verify-quotes.py"))
vq = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(vq)

NOOVERLAP = os.path.join(vq.ROOT, "docs", "audits", "quote-no-overlap.txt")
# sources already diagnosed as corpus problems rather than citation problems
DIAGNOSED = {
    "LeviStrauss-1962-TheSavageMind.pdf",                      # 31% blank pages
    "Foucault-1984-HistoryOfSexualityVol3-CareOfTheSelf.pdf",  # wrong volume on disk
    "Wiener-1948-CyberneticsControlCommunicationAnimal.pdf",   # 24% blank pages
}

CLAUSE = re.compile(r"(?<=[.;?!])\s+|\s*\.\.\.\s*|\s*\[\.\.\.\]\s*")


_MAPS = {}


def readable_gap(lo, hi, sn, sf):
    """Text between two despaced offsets, rendered as it reads in the source."""
    key = id(sf)
    if key not in _MAPS:
        _MAPS.clear()
        pos = []
        for m in re.finditer(r"[a-z0-9\x01]", sn.lower()):
            pos.append(m.start())
        _MAPS[key] = pos
    pos = _MAPS[key]
    if not pos or hi <= lo or lo >= len(pos):
        return ""
    a = pos[min(lo, len(pos) - 1)]
    b = pos[min(hi, len(pos) - 1)]
    return sn[a:b]


def present(text, sf):
    f = vq.despace(vq.norm(text))
    return len(f) >= 24 and f in sf


def longest_prefix(text, sf):
    """Longest prefix of text that appears in the source, by binary search."""
    lo, hi = 0, len(text)
    while lo < hi:
        mid = (lo + hi + 1) // 2
        if present(text[:mid], sf):
            lo = mid
        else:
            hi = mid - 1
    return lo


def report_atom(aid, idx, out):
    path = os.path.join(vq.ROOT, "elements", f"{aid}.yaml")
    try:
        d = vq.load_atom(path)
    except Exception as exc:
        out(f"{aid}: unparseable ({exc})")
        return 0
    if not d:
        return 0
    found = 0
    for e in d.get("lineage") or []:
        if not isinstance(e, dict) or e.get("source") not in (
                "primary", "practitioner", "cross-attestation"):
            continue
        spans = vq.spans_of(e)
        if not spans:
            continue
        cands = []
        for k in vq.author_year_keys(e.get("text") or ""):
            cands += idx.get(k, [])
        if not cands:
            continue
        bestpath = vq.pick_ref(e.get("text") or "", cands)
        best = vq.extract(bestpath) if bestpath else ""
        nw = len(best.split())
        if nw < 300 or (bestpath and os.path.getsize(bestpath) / max(1, nw) > 2000):
            continue
        ref = os.path.basename(bestpath)
        if ref in DIAGNOSED:
            continue
        sn = vq.norm(best, True)
        sf = vq.despace(sn)
        for span in spans:
            bad = vq.check(span, sn, sf)
            if not bad:
                continue
            found += 1
            out(f"\n{aid} [{d.get('status')}]  vs {ref}")
            out(f"  cited   {bad[:300]}".replace("\x01", "'"))
            clauses = [c.strip() for c in CLAUSE.split(bad) if len(c.strip()) >= 25]
            misses = []
            for c in clauses:
                ok = present(c, sf)
                if not ok:
                    misses.append(c)
                out(f"    {'ok  ' if ok else 'MISS'}  {c[:118]}".replace("\x01", "'"))
            if clauses and not misses:
                # Every clause is present but the passage is not, so something
                # sits between them in the source. WHAT sits there decides the
                # verdict and they need opposite responses: an injected page
                # header or running head is a scan artifact and the citation is
                # fine, while a real sentence the citation stepped over is an
                # elision that should carry an ellipsis. Print the gap.
                out("    >>> every clause matches alone -- JOIN defect; what the source"
                    " has between them:")
                spos = []
                for c in clauses:
                    f = vq.despace(vq.norm(c))
                    spos.append((sf.find(f), len(f)))
                for i in range(len(clauses) - 1):
                    a_end = spos[i][0] + spos[i][1]
                    b_start = spos[i + 1][0]
                    if spos[i][0] < 0 or b_start < 0:
                        continue
                    if b_start < a_end:
                        out(f"      [{i}->{i+1}] OUT OF ORDER in source "
                            f"(clause {i+1} begins {a_end - b_start} chars before "
                            f"clause {i} ends)")
                        continue
                    gap = readable_gap(a_end, b_start, sn, sf)
                    if not gap.strip():
                        continue
                    out(f"      [{i}->{i+1}] {len(gap)}c: {gap[:200]}".replace("\x01", "'"))
            for c in misses[:2]:
                n = longest_prefix(c, sf)
                if n < 24:
                    out(f"    give  nothing of this clause is in the source")
                    continue
                anchor = vq.despace(vq.norm(c[:n]))
                at = sf.find(anchor)
                # map back through a fresh despace of the source to read it
                tail = ""
                if at >= 0:
                    approx = sn
                    k = approx.lower().find(c[max(0, n - 40):n].lower().strip())
                    if k >= 0:
                        tail = approx[k:k + 200]
                out(f"    give  after …{c[max(0, n-60):n]}…".replace("\x01", "'"))
                out(f"    cited continues  {c[n:n+90]}".replace("\x01", "'"))
                if tail:
                    out(f"    source has      {tail[40:180]}".replace("\x01", "'"))
    return found


def main():
    args = [a for a in sys.argv[1:] if a.startswith("lex-")]
    if "--from-report" in sys.argv:
        seen = []
        for b in open(NOOVERLAP, encoding="utf-8").read().split("\n\n"):
            m = re.match(r"^(lex-\d+) \[.*\] ABSENT", b.strip())
            if m and m.group(1) not in seen:
                seen.append(m.group(1))
        args = seen
    if not args:
        print("usage: read-absent.py lex-NNNN ... | --from-report")
        return
    idx = vq.build_ref_index()
    total = 0
    lines = []
    for aid in args:
        total += report_atom(aid, idx, lines.append)
    print("\n".join(lines))
    print(f"\n--- {total} unmatched span(s) across {len(args)} atom(s) ---")


if __name__ == "__main__":
    main()

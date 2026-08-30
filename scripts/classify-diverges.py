#!/usr/bin/env python3
"""Separate scan damage from substitution among DIVERGES spans.

adjudicate-spans reports DIVERGES when a span anchors in the source and then
leaves it. That is where substitutions live -- and it is also where every
line-break, ligature and page-break defect lands, because those interrupt a
passage that is otherwise verbatim. Reading all of them by hand is how the
last pass spent its afternoon; 51 spans yielded mostly "the scan lost an fi".

Three mechanical shapes account for the artifacts, and each is decidable:

  CLIPPED   the source lost the head of a word at a line break, so the cited
            text matches from partway into its first word. Tractatus 4.0311
            reads "de" / "nite way" across a break, and the atom's "definite
            way, represents that the things are so combined" is verbatim.

  WEDGE-IN  the source has all the cited words in order, contiguously, with
            one run of debris pushed into the middle -- a running head, a page
            number, a footnote, a journal download stamp. Crenshaw's
            "antiracist" is split by an entire author affiliation line.

  JOINED    the cited text is present once whitespace is discounted, meaning
            the source ran two words together. `despace` in verify-quotes
            covers most of these already; anything reaching here is a
            different join.

What is left after those three is the residue worth a person's eyes: the
source has the words, in a different order or with a different word, and no
mechanical defect explains it.

Usage:
  scripts/classify-diverges.py --from <adjudication.txt>
  scripts/classify-diverges.py --from <adj.txt> --show RESIDUE
"""
import argparse
import importlib.util
import os
import re
import sys

_here = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location(
    "vq", os.path.join(_here, "verify-quotes.py"))
vq = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(vq)

REFS = os.path.join(os.path.dirname(_here), "refs")
ALNUM = re.compile(r"[^a-z0-9]+")


def flat(s):
    return ALNUM.sub("", (s or "").lower())


def clipped(cited_flat, src_flat):
    """Source lost the head of the cited text's first word."""
    # Anything shorter than this matches everywhere and proves nothing.
    for i in range(1, min(12, len(cited_flat) - 30)):
        if cited_flat[i:] in src_flat:
            return i
    return 0


def wedged(cited, src_flat, max_gap=400):
    """All cited words present in order with ONE run of debris inside."""
    words = [w for w in re.split(r"\s+", cited.strip()) if w]
    if len(words) < 4:
        return None
    for split in range(2, len(words) - 1):
        head = flat(" ".join(words[:split]))
        tail = flat(" ".join(words[split:]))
        if len(head) < 12 or len(tail) < 12:
            continue
        i = src_flat.find(head)
        while i >= 0:
            j = src_flat.find(tail, i + len(head))
            if j >= 0 and (j - (i + len(head))) <= max_gap:
                return (j - (i + len(head)), " ".join(words[:split])[-28:],
                        " ".join(words[split:])[:28])
            i = src_flat.find(head, i + 1)
    return None


def parse(path):
    """Yield (atom, ref, cited) for every DIVERGES block."""
    out, cur = [], None
    for line in open(path, encoding="utf-8", errors="replace"):
        m = re.match(r"^(lex-\d{4}) \[[a-z]*\]\s+(.+?)\s*$", line)
        if m:
            cur = [m.group(1), m.group(2), None]
            continue
        if cur and line.strip() == "DIVERGES":
            cur[2] = "PENDING"
            continue
        if cur and cur[2] == "PENDING" and line.strip().startswith("cited"):
            cur[2] = line.split("cited", 1)[1].strip()
            out.append(tuple(cur))
            cur = None
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--from", dest="src", required=True)
    ap.add_argument("--show", default="", help="print full detail for one verdict")
    a = ap.parse_args()

    rows = parse(a.src)
    if not rows:
        print("no DIVERGES blocks found", file=sys.stderr)
        return 2

    cache, tally, detail = {}, {}, []
    for atom, ref, cited in rows:
        path = os.path.join(REFS, ref)
        if not os.path.exists(path):
            verdict, note = "NO-FILE", ref
        else:
            if path not in cache:
                try:
                    cache[path] = flat(vq.extract(path))
                except Exception as e:                       # noqa: BLE001
                    cache[path] = ""
                    print(f"  !! {ref}: {e}", file=sys.stderr)
            sf = cache[path]
            cf = flat(cited)
            if not sf or len(cf) < 30:
                verdict, note = "TOO-SHORT", ""
            elif cf in sf:
                verdict, note = "JOINED", "present once whitespace is discounted"
            elif (n := clipped(cf, sf)):
                verdict, note = "CLIPPED", f"source lost {n} leading char(s)"
            elif (w := wedged(cited, sf)):
                verdict, note = "WEDGE-IN", f"{w[0]} chars of debris after ...{w[1]!r}"
            else:
                verdict, note = "RESIDUE", ""
        tally[verdict] = tally.get(verdict, 0) + 1
        detail.append((verdict, atom, ref, cited, note))

    print(f"{len(rows)} DIVERGES span(s)")
    for k in sorted(tally, key=lambda k: -tally[k]):
        print(f"  {tally[k]:>4}  {k}")
    print("\nCLIPPED / WEDGE-IN / JOINED are scan damage. RESIDUE needs reading.")

    want = a.show.upper() or "RESIDUE"
    print(f"\n===== {want} =====")
    for v, atom, ref, cited, note in detail:
        if v != want:
            continue
        print(f"\n{atom}  {ref}")
        if note:
            print(f"   {note}")
        print(f"   cited  {cited[:300]}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

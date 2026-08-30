#!/usr/bin/env python3
"""Group every file in refs/ by the WORK it is a copy of, and say which copy to read.

The problem this exists for, in the user's words: "you gotta stop missing books
on disk -- it happens over and over." Ten times in one session. Every instance
is one of two shapes, and only the second is interesting:

  I typed a filename from memory and it was wrong.
      Rolt-1920-...epub when the file is -archiveorg.txt; Fisher-1925-... when
      the file is -4thEd1934; a Beach-Pedersen path invented whole. Fixed on
      the other side -- verify-quotes.extract() now raises on a missing path
      instead of returning "", which used to read as "this book contains none
      of your words".

  ONE WORK, SEVERAL FILES. Find one, miss the others.
      Philosophical Investigations is on disk three times (-Anscombe,
      -Anscombe-v2, -Anscombe-v3-bilingual) and only the third is readable.
      Mises 1949 is a blank scan AND a 403k-word .azw3. Mauss 1925 is two
      different translations that word the same passage differently, which is
      how two correct atoms got rewritten into a translation they never cited.
      No amount of care with filenames fixes this, because the filename is not
      the unit anybody actually wants. The work is.

So: key on (surname, year), collapse every format and edition under it, and
print the word count of each so the readable copy is obvious. A file that
extracts to a few hundred words next to a sibling with two hundred thousand is
not a book that is missing -- it is a bad scan of a book that is present.

Usage:
  scripts/refs-manifest.py                  # write refs/MANIFEST.tsv
  scripts/refs-manifest.py wittgenstein     # show one work's whole group
  scripts/refs-manifest.py --multi          # only works with more than one copy
  scripts/refs-manifest.py --dead           # copies with no readable text at all
"""
import argparse
import collections
import importlib.util
import os
import re
import sys

_here = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location(
    "vq", os.path.join(_here, "verify-quotes.py"))
vq = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(vq)

ROOT = os.path.join(_here, "..")
MANIFEST = os.path.join(ROOT, "refs", "MANIFEST.tsv")

# Author-Year-TitleSlug is the house convention. Everything after the year is
# edition, translator, source, or format, and none of it identifies the work.
STEM = re.compile(r"^([A-Za-z][A-Za-z'’-]*(?:-[A-Z][A-Za-z'’-]*)?)-(\d{3,4})")

READABLE = 5000     # words below which a copy is a scan, not a book


def work_key(fn):
    m = STEM.match(fn)
    if not m:
        return None
    return (m.group(1).lower(), m.group(2))


def derived(path, sized):
    """Is every readable copy of this work just OCR output OF this file?

    `Foo.pdf` and `Foo.ocr.txt` are one artifact and its own transcription,
    not two copies. If the transcription is the only readable thing here, the
    scan underneath it is irreplaceable.
    """
    stem = os.path.splitext(path)[0]
    for n, other in sized:
        if n < READABLE or other == path:
            continue
        if os.path.splitext(other)[0].removesuffix(".ocr") != stem:
            return False        # some genuinely independent copy exists
    return True


def words(path):
    try:
        return len(vq.extract(path).split())
    except Exception:
        return -1


def scan():
    groups = collections.defaultdict(list)
    for root, _, files in os.walk(os.path.join(ROOT, "refs")):
        for fn in sorted(files):
            if fn == "MANIFEST.tsv" or fn.startswith("."):
                continue
            k = work_key(fn)
            groups[k or ("~unkeyed", "")].append(os.path.join(root, fn))
    return groups


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("query", nargs="?", default=None)
    ap.add_argument("--multi", action="store_true",
                    help="only works held in more than one file")
    ap.add_argument("--dead", action="store_true",
                    help="only copies that extract to nothing readable")
    a = ap.parse_args()

    groups = scan()
    rows = []
    for k in sorted(groups):
        copies = groups[k]
        if a.query:
            # Match per FILE, then keep that file's whole group -- the point of
            # the tool is to show the siblings, so a hit on one copy has to
            # pull in the others. Filtering the group as one blob made a query
            # for "wittgenstein" drag in the entire unkeyed bucket and extract
            # every file in it.
            if not any(a.query.lower() in os.path.basename(p).lower()
                       for p in copies):
                continue
            if k[0] == "~unkeyed":
                copies = [p for p in copies
                          if a.query.lower() in os.path.basename(p).lower()]
        # "~unkeyed" is not a work, it is everything whose filename does not
        # follow Author-Year-. Treating it as one group made --multi extract
        # every unkeyed file in refs/ looking for duplicates of a work that
        # does not exist.
        if k[0] == "~unkeyed" and (a.multi or a.dead) and not a.query:
            continue
        if a.multi and len(copies) < 2:
            continue
        sized = [(words(p), p) for p in copies]
        best = max((n for n, _ in sized), default=-1)
        for n, p in sorted(sized, reverse=True):
            flag = ("dead " if n < 300 else
                    "thin " if n < READABLE else "ok   ")
            if a.dead and n >= READABLE:
                continue
            # A thin copy is only a problem when an INDEPENDENT readable copy
            # exists. Its own .ocr.txt does not count and saying otherwise is
            # dangerous: it labels the scanned PDF redundant when that PDF is
            # the only image source the OCR was made from. Graham 1949 is the
            # case -- its sidecar has column-interleave damage, so the repair
            # is to re-OCR at better settings, which needs the pages. Deleting
            # the PDF on the strength of its own output would make the damage
            # permanent. Four of the eight files a first pass called redundant
            # were this shape.
            note = ""
            if n < READABLE <= best and not derived(p, sized):
                note = "  (superseded -- an independent readable copy is above)"
            rows.append((k, flag, n, os.path.relpath(p, ROOT), note))

    if a.query or a.multi or a.dead:
        shown = None
        for k, flag, n, p, note in rows:
            if k != shown:
                print(f"\n{k[0]} {k[1]}")
                shown = k
            print(f"  {flag} {n:>9,}w  {p}{note}")
        print(f"\n{len(rows)} file(s)")
        return

    os.makedirs(os.path.dirname(MANIFEST), exist_ok=True)
    with open(MANIFEST, "w", encoding="utf-8") as f:
        f.write("author\tyear\tstate\twords\tpath\n")
        for k, flag, n, p, _ in rows:
            f.write(f"{k[0]}\t{k[1]}\t{flag.strip()}\t{n}\t{p}\n")
    multi = sum(1 for k, c in collections.Counter(r[0] for r in rows).items()
                if c > 1)
    print(f"wrote {MANIFEST} -- {len(rows)} files, {multi} works held more "
          f"than once")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Write the slug -> file mapping down instead of re-deriving it every run.

The `text:` field of a lineage entry is a hand-written slug naming a work
("hayek-1945-individualism-true-and-false"). The files in refs/ are named
Author-Year-TitleSlug. Nothing connected the two, so verify-quotes grew a
matcher that guesses the connection from shared tokens and years.

That matcher is a bootstrap. It was the right way to produce a first mapping
across two thousand entries and a corpus nobody had cross-indexed; it is the
wrong thing to still be running on every pass, because it throws its answers
away and re-guesses. The same misses come back wearing different shapes:
"gamma-helm-johnson-vlissides-1994" reaches a blog post about Dwayne Johnson
on the token "johnson", and no run records that this was ever wrong.

So: pin it. One line per distinct slug, in a file the audit reads before it
reasons. A pin is permanent, correctable by hand, and can say the one thing
the matcher structurally cannot -- ABSENT, meaning somebody checked and the
shelf does not have this work. Design Patterns and the Dohare-Sutton
plasticity paper both need to say that, and today they instead resolve to
whatever shares a surname.

The pin file is gitignored. refs/ is a local corpus, not part of the
published repo, and the filenames carry formats (.epub, .azw3) that say how a
copy was obtained -- which is exactly what the ingest rule scrubs from
citations. The public artifact keeps the human-readable slug; only the local
dev tool gets the path.

    scripts/pin-refs.py            propose pins for unpinned slugs, merge in
    scripts/pin-refs.py --review   list the pins that need a human eye

Existing lines are never overwritten. Anything already marked `pinned` or
`absent` is a decision somebody made, and a regenerate must not silently
undo it.
"""
import importlib.util
import glob
import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PINS = os.path.join(ROOT, ".refs-pins.tsv")

spec = importlib.util.spec_from_file_location(
    "vq", os.path.join(ROOT, "scripts", "verify-quotes.py"))
vq = importlib.util.module_from_spec(spec)
spec.loader.exec_module(vq)

HEADER = """\
# slug<TAB>status<TAB>file[|file...]  -- relative to refs/
#
# status:
#   pinned   a human confirmed this file is the work the slug names
#   absent   a human checked and the work is not in refs/
#   none     the matcher found no candidate; nobody has looked yet
#   auto     proposed by the token matcher, NOT yet reviewed
#
# Only `auto` lines are rewritten by scripts/pin-refs.py. Edit a line and set
# its status to pinned or absent to make it permanent. Several files are
# allowed for one slug -- multi-volume works, a PDF beside its OCR sidecar,
# two editions the atoms quote from separately.
"""


def load_pins():
    pins = {}
    if not os.path.exists(PINS):
        return pins
    for line in open(PINS, encoding="utf-8"):
        line = line.rstrip("\n")
        if not line or line.startswith("#"):
            continue
        parts = line.split("\t")
        if len(parts) < 2:
            continue
        slug, status = parts[0], parts[1]
        files = [f for f in (parts[2].split("|") if len(parts) > 2 and parts[2] else [])]
        pins[slug] = (status, files)
    return pins


def save_pins(pins):
    with open(PINS, "w", encoding="utf-8") as fh:
        fh.write(HEADER)
        for slug in sorted(pins):
            status, files = pins[slug]
            fh.write(f"{slug}\t{status}\t{'|'.join(files)}\n")


def collect_slugs():
    """Every slug that has something to check, with the atoms that cite it.

    Keyed by slug rather than by atom, because the slug names the WORK and a
    correction to it should hold everywhere at once -- four atoms cite
    "kuhn-1962-structure-of-scientific-revolutions" and there is one right
    answer for all four. The citation prose is carried along because for a
    work held in several translations the translator is named only there.
    """
    slugs = {}
    for path in sorted(glob.glob(os.path.join(ROOT, "elements", "*.yaml"))):
        aid = os.path.basename(path)[:-5]
        d = vq.load_atom(path)
        if not d:
            continue
        for e in d.get("lineage") or []:
            if not isinstance(e, dict) or e.get("source") not in (
                    "primary", "practitioner", "cross-attestation"):
                continue
            if not [s for s, k in vq.spans_of(e, with_kind=True)
                    if k != "attribution"]:
                continue
            slug = (e.get("text") or "").strip()
            if not slug:
                continue
            rec = slugs.setdefault(slug, {"atoms": [], "cite": ""})
            rec["atoms"].append(aid)
            if len(e.get("citation") or "") > len(rec["cite"]):
                rec["cite"] = e.get("citation") or ""
    return slugs


def propose(slug, cite, idx):
    """The matcher's guess, as candidate paths -- no text extraction.

    resolve_usable extracts every candidate to decide which carry a text
    layer, which is why it takes minutes across the elements. Proposing a
    pin does not need that: a file with no text layer is still the right
    file, and verify-quotes reports "no text layer" for it either way.
    """
    cands = []
    for k in vq.author_year_keys(slug):
        cands += idx.get(k, [])
    if any(t in vq.PLATFORM for t in vq._title_tokens(slug)):
        cands = [c for c in cands if vq.author_only_admissible(slug, c)]
    if not cands:
        for k in vq.author_only_keys(slug):
            cands += idx.get(k, [])
        cands = [c for c in cands if vq.author_only_admissible(slug, c)]
    wider = [c for c in dict.fromkeys(
                 sum((idx.get("author:" + k.rsplit("-", 1)[0], [])
                      for k in vq.author_year_keys(slug)), []))
             if c not in cands]
    rank_text = slug + " " + cite
    ranked = vq.rank_refs(rank_text, list(dict.fromkeys(cands)))[:4]
    if wider:
        ranked += vq.rank_refs(rank_text, wider)[:2]
    return [os.path.relpath(c, vq.REFS) for c in dict.fromkeys(ranked)]


def author_field(stem):
    """Tokens occupying the filename's author position.

    Not the leading TOKEN: half the corpus is Author-Author-Year-Title with
    the surnames glued together (LakoffJohnson, AxelrodHamilton, BuenoDe
    MesquitaSmith), so a single leading token flags every co-authored file as
    suspicious. Two hyphen-segments, camel-split, covers those and covers
    Wikipedia-Title-Year, where the platform sits where an author would.
    """
    head = os.path.basename(stem)
    cut = vq.CITE_YEAR.search(head)
    head = head[:cut.start()] if cut else "-".join(head.split("-")[:2])
    return vq._title_tokens(head)


def author_matches(slug, stem):
    """Whether the citation's surname occupies the file's author position.

    Containment rather than equality, because the two sides split names
    differently and neither split is wrong. The filename glues co-authors
    together (GoldinMeadow, EkmanFriesen) and the camel-splitter cuts Scots
    prefixes off (McGilchrist -> gilchrist, mc dropped for length), while the
    slug keeps them whole. Requiring set equality flagged eleven correct pins
    for being spelled differently.
    """
    field = author_field(stem)
    for a in vq.author_tokens(slug):
        if any(a == f or (len(a) > 4 and a in f) or (len(f) > 4 and f in a)
               for f in field):
            return True
    return False


def confidence(slug, cite, rels, size):
    """Why a proposal deserves a human eye.

    Zero title overlap on its own is mostly innocent -- a chapter slug
    pointing at the volume containing it cannot share a title token, which is
    how five Kimmerer chapters correctly reach Braiding Sweetgrass and how
    Frost's poem reaches Mountain Interval. It only means something when the
    file is too small to be a containing volume, which is the shape every
    wrong pin found by hand has had: a 4K blog post or a 1K xkcd transcript
    answering a citation of a book.
    """
    if not rels:
        return "no candidate"
    reasons = []
    at = vq.author_tokens(slug)
    stem = os.path.splitext(os.path.basename(rels[0]))[0]
    if not author_matches(slug, stem):
        reasons.append("author absent from author position")
    want = vq._title_tokens(slug + " " + cite) - at
    if want and not (want & vq._title_tokens(stem)) and size < 150_000:
        reasons.append(f"no title overlap, {size // 1024}K file")
    sy, fy = vq._first_year(slug), vq._first_year(stem)
    if sy and fy and sy != fy:
        reasons.append(f"year {sy} vs {fy}")
    return ", ".join(reasons)


def main():
    review = "--review" in sys.argv
    pins = load_pins()
    slugs = collect_slugs()
    idx = vq.build_ref_index()

    added = 0
    for slug, rec in slugs.items():
        if slug in pins and pins[slug][0] in ("pinned", "absent"):
            # `none` is deliberately NOT sticky: it records that the matcher
            # came up empty, and a later acquisition should let it resolve.
            continue
        rels = propose(slug, rec["cite"], idx)
        # "none", not "absent". The matcher finding nothing is not the same
        # claim as a person having looked, and writing the stronger one here
        # would make the pin file lie about what has been checked -- which is
        # the failure it exists to fix. Only a hand edit promotes none ->
        # absent.
        pins[slug] = ("auto" if rels else "none", rels)
        added += 1
    for slug in list(pins):
        if slug not in slugs:
            del pins[slug]
    if not review:
        save_pins(pins)
        print(f"{len(pins)} slug(s) pinned; {added} proposed this run -> {PINS}")

    flagged = []
    for slug, (status, rels) in pins.items():
        if status != "auto":
            continue
        try:
            size = os.path.getsize(os.path.join(vq.REFS, rels[0])) if rels else 0
        except OSError:
            size = 0
        why = confidence(slug, slugs.get(slug, {}).get("cite", ""), rels, size)
        if why:
            flagged.append((len(slugs.get(slug, {}).get("atoms", [])), slug,
                            rels[0] if rels else "-", why))
    flagged.sort(reverse=True)
    print(f"\n{len(flagged)} auto pin(s) want a human eye "
          f"(of {sum(1 for s in pins.values() if s[0] == 'auto')} auto):\n")
    for n, slug, first, why in flagged:
        print(f"  {n:2d} atom(s)  {slug[:48]:48s} -> {first[:44]:44s}  [{why}]")


if __name__ == "__main__":
    main()

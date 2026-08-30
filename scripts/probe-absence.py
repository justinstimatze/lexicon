#!/usr/bin/env python3
"""Say WHY a span is absent, before anyone reads it.

adjudicate-spans prints where a span leaves the source. That is the right
question when the span has an anchor. When it has none -- NO-ANCHOR, not even
the first clause present -- the printout is two strings that share nothing and
tells you only that they differ, which you already knew. Nearly forty spans
came back that way and each needed a separate hand-investigation to land in one
of four buckets that a machine can tell apart:

  NOT-IN-BOOK    the span's rare words are absent from the source outright.
                 The passage is from a different work, or invented.
  SCRAMBLED      the rare words are all present, just never adjacent. Column
                 interleave, facing pages, a figure flattened into a line.
                 Usually the citation is fine and the scan is not -- but NOT
                 always, and the exception is the whole reason the damage
                 number is printed beside it. A sentence somebody WROTE in the
                 author's voice, out of the author's own vocabulary, has all
                 its rare words in the book too. That is how lex-8f8jk (James)
                 and lex-d234k (Weick) were built. So SCRAMBLED against a
                 source measuring near-zero damage is not an artifact; it is
                 the invented-quote signature and needs reading.
  VARIANT        all but one or two rare words present. An OCR letter-merge
                 ("seK-existence"), a spelling the edition does not share
                 ("Hebdidge"), a dropped diacritic. Check the missing word by
                 hand -- this bucket is where the real substitutions hide too.
  COMMENTARY     the span is the atom talking, not the author. Fragmentary
                 syntax, a leading dash, a trailing semicolon, and none of the
                 vocabulary of the book. Nothing to verify.

Rare words are the discriminator because common ones are in every book: a word
is rare if it is not in the 300 most frequent words of the source itself, which
needs no external list and adapts to the source's own register.

Usage:
  scripts/probe-absence.py --from docs/audits/adj-txtepub-v10.txt
  scripts/probe-absence.py --atoms lex-utuy6,lex-7gm7f
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

WORD = re.compile(r"[A-Za-z][A-Za-z'’-]{3,}")

# How far apart the span's rare words sit in the source, in characters.
#
# The first attempt here counted junk tokens per source and it separated
# nothing: clean epubs and wrecked OCR both scored 15-25%, because the rule
# was counting "of", "to" and "is" as fragments. The measurement that does
# work is per-span rather than per-source, and it asks the question directly.
#
# A column interleave, a facing page, a running head spliced mid-sentence --
# every reading-order defect is LOCAL. It displaces words by a line or a
# column, so a scrambled sentence's rare words still land within a few hundred
# characters of each other. A sentence somebody composed in the author's voice
# draws its vocabulary from wherever that vocabulary lives, which in a book is
# chapters apart. So: find the tightest window of source text containing all
# the span's rare words, and compare it to the span's own length.
LOCAL = 6      # window may be this many times the span's length and still be
               # one displaced passage rather than a book-wide gather


def ordered_in_window(span, src_lower, lo, hi, pad=1200):
    """Do the span's content words appear IN ORDER in this stretch of source?

    This is what finally decides a SCRAMBLED span, and locality alone cannot.
    Every reading-order defect INSERTS material -- the other column's words, a
    running head, a footnote, a page number -- while leaving the relative order
    of the original sentence's own words untouched. So a citation that is
    merely a victim of a bad scan is still an ordered subsequence of the source
    stream around it.

    A citation that reordered its source, or was composed from vocabulary lying
    nearby, is not. That is the difference between Graham's 'concept of |g
    safety We excitement and hazards...| margin and the principle' -- one
    sentence with a column shoved through it, order intact -- and Woolf
    lex-brcgz, where the sentences were resequenced under cover of an ellipsis.

    Returns (ok, first_word_out_of_order).

    It must try EVERY place the passage could be, not just the tightest window
    of rare words. The first version tested only that window and reported 36
    reordered spans in the PDF tier, nearly all of them wrong: for a short span
    the two or three rare words often cluster tightest somewhere unrelated -- a
    contents line, an index entry, a running head -- and the test then asked
    whether the sentence was in order in a place the sentence was never in.
    The spread numbers gave it away, x0 and x1 where a real sentence cannot fit.
    """
    # Only words the source actually HAS. Whether a word is missing is a
    # different question, and classify() has already answered it -- mixing the
    # two made every span carrying a compound the source hyphenates differently
    # ("process-tracing" vs "process tracing") report as reordered, because the
    # anchor below picks the rarest word and a word with zero occurrences is
    # the rarest of all.
    want = [w for w in WORD.findall(span.lower()) if w in src_lower]
    if not want:
        return True, None
    # Anchor on the span's least common word so there are few places to try.
    anchor = min(want, key=lambda w: src_lower.count(w))
    starts = [m.start() for m in re.finditer(re.escape(anchor), src_lower)]
    if not starts:
        return False, anchor
    reach = max(pad, 3 * len(span))
    worst = anchor
    for s in starts[:400]:
        stream = WORD.findall(src_lower[max(0, s - reach):s + reach])
        i, failed = 0, None
        for w in want:
            try:
                i = stream.index(w, i) + 1
                continue
            except ValueError:
                pass
            # Not after position i. Is it in this window AT ALL?
            #
            # If not, the word is simply not rendered here as a word -- a page
            # break split it ("the nature and de-|906 OCTOBER 1979 AMERICAN
            # PSYCHOLOGIST|velopment of metacognition", Flavell 1979) or OCR
            # merged it into a neighbour. That says nothing about order, so it
            # is skipped rather than counted against the citation.
            #
            # If it IS here but only EARLIER than the citation puts it, that is
            # the actual reordering signature and the only thing this function
            # should fire on.
            if w in stream:
                failed = w
                break
        if failed is None:
            return True, None
        worst = failed
    return False, worst


def tightest_window(rare, src_lower):
    """Smallest span of source containing every one of `rare`.

    Returns (width_in_chars, lo, hi) for the tightest such window, or None if
    any word is absent.
    """
    posts = []
    for w in rare:
        hits = [m.start() for m in re.finditer(re.escape(w), src_lower)]
        if not hits:
            return None
        posts.append(hits[:4000])
    # sweep: advance the pointer sitting at the window's left edge
    ptr = [0] * len(posts)
    best = None
    while True:
        cur = [p[i] for p, i in zip(posts, ptr)]
        lo, hi = min(cur), max(cur)
        if best is None or hi - lo < best[0]:
            best = (hi - lo, lo, hi)
        k = cur.index(lo)
        ptr[k] += 1
        if ptr[k] >= len(posts[k]):
            return best

# A span that is the atom's own voice rather than the author's. Each of these
# is a shape no quotation has: a bullet's dash, a claim-label the mining pass
# writes, a sentence that begins mid-clause with a verb.
#
# An earlier version also matched anything opening with "(" and it was wrong
# twice over: Legge starts a Tao Te Ching line with an editorial "(It is the
# way of the Tao) to act without..." and Wolfflin's five oppositions are
# quoted as "(1) The development from the linear to the painterly...". Both
# are the author. The rule stays deliberately narrow because of where it could
# end up -- this label is only ever REPORTED here, never used to drop a span
# from checking, and it must not become the reason a real quote goes unread.
COMMENTARY = re.compile(
    r"^\s*[-–—]\s|"
    r"^(Key claim|Claim|Mechanism|Note|Contrast|Corollary|Gloss)\b|"
    r"^(supplies|names|marks|tracks|generalises|generalizes|captures|"
    r"describes|the system produces)\b",
    re.I)


def rare_words(text, source_freq, k=300):
    common = {w for w, _ in source_freq.most_common(k)}
    out = []
    for w in WORD.findall(text.lower()):
        if w not in common and w not in out:
            out.append(w)
    return out


def classify(span, src_norm, src_lower, freq):
    if COMMENTARY.match(span.strip()):
        return "COMMENTARY", [], []
    rare = rare_words(span, freq)
    if not rare:
        return "COMMENTARY", [], []
    missing = [w for w in rare if w not in src_lower]
    present = [w for w in rare if w in src_lower]
    frac = len(missing) / len(rare)
    if frac >= 0.5:
        return "NOT-IN-BOOK", missing, present
    if missing:
        return "VARIANT", missing, present
    return "SCRAMBLED", missing, present


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--from", dest="src", default=None,
                    help="an adjudicate-spans output file to re-read")
    ap.add_argument("--atoms", default=None)
    ap.add_argument("--show", default="",
                    help="print the cited span AND the source window for one "
                         "bucket, e.g. --show SCRAMBLED. Without this, "
                         "SCRAMBLED and COMMENTARY print as a tally only, "
                         "which is fine for counting and useless for reading.")
    ap.add_argument("--ocr", action="store_true",
                    help="allow OCR of a scan with no text layer. OFF by "
                         "default: this reads sources adjudicate-spans has "
                         "already read, so anything needing OCR was skipped "
                         "upstream and OCR-ing it here only re-derives text "
                         "the run cannot use. Left on, a probe silently forks "
                         "pdftoppm at 300dpi on a 584-page book -- 15 minutes, "
                         "190 MB of page renders, and the probe itself sitting "
                         "at 0.0%% CPU looking like it hung.")
    a = ap.parse_args()
    if not a.ocr:
        vq.ALLOW_OCR = False

    want = set((a.atoms or "").split(",")) if a.atoms else None
    pairs = []
    if a.src:
        # blocks look like:  lex-utuy6 [active]  Ref.epub \n  VERDICT \n ... cited  TEXT
        for b in open(a.src, encoding="utf-8").read().split("\n\n"):
            L = [x.rstrip() for x in b.strip().split("\n")]
            if not L or not L[0].startswith("lex-"):
                continue
            head = L[0].split()
            aid, ref = head[0], head[-1]
            # Only NO-ANCHOR blocks. In a DIVERGES block the "cited" line is
            # the REMAINDER after the matched prefix, not a span -- probing it
            # measures the spread of a sentence fragment and calls a settled
            # OCR variant an invention.
            if not any(x.strip().startswith("NO-ANCHOR") for x in L):
                continue
            cited = [x.strip()[6:].strip() for x in L if x.strip().startswith("cited")]
            for c in cited:
                if not want or aid in want:
                    pairs.append((aid, ref, c))
    if not pairs:
        print("nothing to probe", file=sys.stderr)
        return

    idx = vq.build_ref_index()
    bypath = {}
    for v in idx.values():
        for p in v:
            bypath.setdefault(os.path.basename(p), p)

    cache = {}
    rows = []
    for aid, ref, span in pairs:
        p = bypath.get(ref)
        if not p:
            rows.append((aid, ref, "NO-REF", span, [], [], ""))
            continue
        if p not in cache:
            t = vq.extract(p)
            cache[p] = (vq.despace(vq.norm(t, True)), t.lower(),
                        collections.Counter(WORD.findall(t.lower())), t)
        sn, sl, freq, raw = cache[p]
        verdict, missing, present = classify(span, sn, sl, freq)
        spread, ctx = "", ""
        if verdict == "SCRAMBLED":
            rare = rare_words(span, freq)
            win = tightest_window(rare[:12], sl)
            if win is not None:
                width, lo, hi = win
                ratio = width / max(40, len(span))
                spread = f"x{ratio:.0f}"
                # The window the verdict was computed from, in the source's own
                # characters. SCRAMBLED is the one bucket whose verdict cannot
                # be checked from the numbers -- an invented sentence in the
                # author's voice scores exactly like a column interleave -- so
                # the only thing that settles it is this text beside the span.
                ctx = re.sub(r"\s+", " ", raw[max(0, lo - 200):hi + 200])
                # Displaced by a column, or gathered from across the book?
                if ratio > LOCAL:
                    verdict = "INVENTED?"
                else:
                    # Local, so the words are here. Are they in the cited
                    # ORDER? Insertion damage preserves order; a rewrite or a
                    # resequencing does not.
                    ok, culprit = ordered_in_window(span, sl, lo, hi)
                    if not ok:
                        verdict = "REORDERED?"
                        spread += f"  order breaks at '{culprit}'"
        rows.append((aid, ref, verdict, span, missing, present, spread, ctx))

    order = {"NOT-IN-BOOK": 0, "INVENTED?": 1, "REORDERED?": 2, "VARIANT": 3,
             "SCRAMBLED": 4, "COMMENTARY": 5, "NO-REF": 6}
    rows.sort(key=lambda r: (order.get(r[2], 9), r[0]))
    counts = collections.Counter(r[2] for r in rows)
    print(f"{len(rows)} span(s): " +
          ", ".join(f"{k} {v}" for k, v in counts.most_common()))
    print("NOT-IN-BOOK, INVENTED?, REORDERED? and VARIANT need reading.")
    print("SCRAMBLED is one displaced passage whose word ORDER still holds;")
    print(f"COMMENTARY was never a quote. x_ = span-lengths of spread; >x{LOCAL} is not local.\n")
    show = a.show.strip().upper()
    for aid, ref, verdict, span, missing, present, spread, ctx in rows:
        quiet = verdict in ("SCRAMBLED", "COMMENTARY") and verdict != show
        print(f"{verdict:12} {aid}  {ref[:40]:42} {spread}")
        if quiet:
            continue
        print(f"             cited   {span[:150]}")
        if missing:
            print(f"             absent  {', '.join(missing[:12])}")
        if present and verdict == "VARIANT":
            print(f"             present {', '.join(present[:8])}")
        if ctx and verdict == show:
            print(f"             window  …{ctx}…")


if __name__ == "__main__":
    main()

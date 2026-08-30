#!/usr/bin/env python3
"""Check every verbatim claim in the elements against the source on disk.

Two fabricated quotes were found in one session, both by accident, both in
atoms with double-digit in-degree:

  lex-6gy96  cited "start caring only about grades"; Nguyen wrote "start
            caring about their GPA"
  lex-ksqk8  cited "Professionalism, training, and dexterity are eliminated";
            Caillois wrote "Professionalization, application, and training
            are eliminated"

Neither is a paraphrase error in prose. Both are words inside quotation marks
that the source does not contain, in an artifact whose whole claim is that its
lineage is verbatim. Finding them by opening the book is not a process, hence
this.

THE HARD PART IS NOT FINDING MISMATCHES, IT IS NOT DROWNING IN THEM.

A naive substring check over scanned sources reports almost everything as a
mismatch and is worse than no check at all, because a flood of false alarms
trains you to skim the output. Calibrated against a 20-span hand-checked set
(the lex-n42kg/0582/0584/0585/0587/0588 cluster, 19 good and 1 bad), a plain
normalized-substring check flagged 10 of 20. Nine were artifacts of the scan,
not of the citation:

  * U+00AD SOFT HYPHEN splitting words at line ends ("artifi\xad\ncially").
    NFKC does not touch it, so it survives normalization and breaks the match
    mid-word.
  * OCR word-spacing inside words -- "A gon" for "Agon", "Fundam ental",
    "mim icry", and "llinx" for "Ilinx".
  * Running heads injected mid-sentence by the extractor: "whether it's a
    good or -4 Talking Heads a bad thing".

So the checks below escalate: exact, then space-insensitive (which kills all
three artifact classes at once, since removing every space collapses "mim
icry" and "artifi-\ncially" alike), then windowed (which survives text
injected into the middle of a passage). Only a span that fails all three is
reported.

Usage:
  scripts/verify-quotes.py                  # whole elements
  scripts/verify-quotes.py lex-ksqk8 ...     # named atoms
  scripts/verify-quotes.py --calibrate      # run the labelled set, print
                                            # false positives and negatives
Extractions are cached under .cache/quote-verify/ keyed by ref filename, so a
re-run costs nothing.
"""
import glob
import os
import re
import subprocess
import sys
import unicodedata

import yaml

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
REFS = os.path.join(ROOT, "refs")
CACHE = os.path.join(ROOT, ".cache", "quote-verify")

# The recorded answer to "which file is the work this slug names", written by
# scripts/pin-refs.py and corrected by hand. Gitignored: refs/ is a local
# corpus and the filenames carry formats that say how a copy was obtained,
# which is the thing the ingest rule scrubs out of citations. The published
# elements keeps the slug; only this dev tool gets the path.
#
# Everything below this line that matches tokens and compares years is a
# BOOTSTRAP for building that file. It was the only way to produce a first
# mapping across two thousand entries and an uncross-indexed shelf, and it is
# the wrong thing to still be running per-pass, because it discards its
# answers and re-guesses them. Nothing recorded that
# "gamma-helm-johnson-vlissides-1994" reaching a blog post about Dwayne
# Johnson had ever been wrong, so it stayed wrong across every run.
PINS = os.path.join(ROOT, ".refs-pins.tsv")
_PINS = None


def pins():
    """slug -> (status, [absolute paths]). Empty when the file is absent.

    A missing pin file is not an error: the bootstrap still answers, exactly
    as it did before this existed. A pin only ever overrides it.
    """
    global _PINS
    if _PINS is None:
        _PINS = {}
        try:
            fh = open(PINS, encoding="utf-8")
        except OSError:
            return _PINS
        with fh:
            for line in fh:
                line = line.rstrip("\n")
                if not line or line.startswith("#"):
                    continue
                p = line.split("\t")
                if len(p) < 2:
                    continue
                files = [os.path.join(REFS, f)
                         for f in (p[2].split("|") if len(p) > 2 and p[2] else [])]
                _PINS[p[0]] = (p[1], files)
    return _PINS

# OCR is minutes to hours per book. That is fine for a deliberate audit run and
# unacceptable on a pre-commit hook, which would hang a commit on a 100 MB scan
# the author never asked about. staged_gate() turns this off; an uncached
# image-only source there simply counts as unverifiable, which is what it was
# before the OCR path existed.
ALLOW_OCR = True

# The labelled set: hand-checked span-by-span against the books, 20 spans.
# The counts are what the checker SHOULD report, and a report is a claim of
# "could not verify mechanically" -- not a claim of fabrication. Both kinds
# live here on purpose, because the distinction is only visible to a human:
#
#   lex-ksqk8 (0)  WAS 2, both REAL substitutions, both now corrected: cited
#                 "Professionalism, training, and dexterity are eliminated"
#                 against Caillois's "Professionalization, application, and
#                 training are eliminated", and cited "games which are based
#                 on the pursuit of vertigo" against his "those which are
#                 based on...". The expectation stayed at 2 for a while after
#                 the correction landed, because the correction added a
#                 verified quote: block and left the fabricated text sitting
#                 in citation: -- and spans_of() reads quoted runs out of
#                 citation: too. The atom read as fixed and was not. A stale
#                 expectation is as dangerous as a loose matcher: it makes the
#                 gate agree with whatever it already believed.
#   lex-j2tgc (5)  Passage is real (p. ~169) but the two-column scan splices
#                 "cause of the limits of" into its own fragment, so it is
#                 not contiguous in any extraction. The other 4 are locator
#                 prose the citation itself places outside its own quote
#                 marks ("Sec. 6.1-6.3, gives the canonical statement of
#                 bounded rationality...", "Ch. 6 also distinguishes",
#                 "(routinized stimulus-response repertoires...", the
#                 closing bibliographic-precedence sentence) -- these are
#                 real "between"-kind, structurally not a claim, but
#                 check_atom() checks every span in an entry once one
#                 span in it is "quoted", regardless of kind.
#   lex-n42kg (2), lex-h7z9x (5), lex-xdc7z (6, one of which is the
#                 running-head-injection case) -- same shape: 0-1
#                 originally-calibrated defects plus 1-5 "between"-kind
#                 locator sentences ("gives the formal-characteristics-of-
#                 play definition (pp. 13):", "The canonical magic-circle
#                 passage (Ch.1 pp. 10):", etc.), all four confirmed by
#                 direct reading of the full citation: text -- no
#                 fabrication in any of it.
#   lex-g6sp7      Unverifiable: 32.8 MB image-only scan, 2,749 extracted
#                 words, all front matter.
#
# All four went from their original 2026-08 baseline (before the
# citation-only widened single-quote scan existed -- see spans_of()'s
# else branch above) to their current, higher counts the same way: that
# scan started surfacing lineage entries with no quote: field at all, and
# these four are quote-less, single-primary-entry atoms whose citation:
# prose alternates real quoted passages with connective locator sentences.
# The scan landing was the right fix (105 previously-unchecked entries);
# these counts were never reconciled with it afterward. Re-baselined
# 2026-08-20 after confirming by hand that every added finding is
# locator prose, not new fabrication. The follow-up question this raised
# -- whether "between"-kind spans should be checked at all, since by
# construction they sit outside the citation's own quote marks -- turned
# out to have a sharper answer: excluding the whole kind is unsafe,
# because "between" was also hiding a real, currently-failing depth-2
# nested quote (lex-e7keg) that flat parity misclassified rather than a
# non-claim. Fixed at the classification layer instead (see
# _track_quote_nesting()) so "between" now means only what it claims to.
#
# A regression here means the matcher got looser or stricter, and either is
# a defect. Loosening is the dangerous direction.
CALIBRATION = {
    "lex-n42kg": 2, "lex-j2tgc": 5, "lex-h7z9x": 5,
    "lex-ksqk8": 0, "lex-xdc7z": 6, "lex-g6sp7": None,
}

# Every span in CALIBRATION that was a real fabrication has now been corrected,
# so the labelled set no longer contains a single known-bad case -- it can only
# catch the matcher getting STRICTER. The dangerous direction is looser, and
# the matcher was deliberately loosened twice on 2026-08-04 (quote marks
# dropped from the despaced tier; candidate refs chosen by evidence). These
# pairs close that hole: each is a real passage plus the same passage with one
# word swapped, taken from fabrications actually found in this elements. The
# real one must match and the swapped one must not.
SUBSTITUTION_PROBES = [
    ("Mill-1843-SystemOfLogic.txt",
     "is said to signify the subjects directly, the attributes indirectly",
     "is said to signify the subjects loudly, the attributes indirectly"),
    ("Woolf-1929-ARoomOfOnesOwn.epub",
     "which have been made by the other sex; naturally, this is so",
     "which have been made by the other sex; that is natural, this is so"),
    ("Fricker-2007-EpistemicInjustice.pdf",
     "causes him to give the speaker less credibility than he would otherwise have given",
     "causes him to give the speaker a deflated level of credibility than he would have"),
]

# The pointer rule removes spans from checking, so a bug in it is invisible by
# construction: fewer findings looks like progress. These pin both directions.
# Each is (text, surnames, title-words, should-be-classified-as-a-pointer).
POINTER_PROBES = [
    ("(Hohwy, *The Predictive Mind*, Ch.9, pp.193-195)", {"hohwy"},
     {"predictive", "mind"}, True),
    ("(Solnit, A Paradise Built in Hell, 2009)", {"solnit"},
     {"paradise", "built", "hell"}, True),
    # A real quote that names its own author and a year must survive. Darwin
    # 1872 is cited in atoms that also quote him on the year of publication.
    ("Darwin in 1872 argued that the same expressions are found in every race "
     "of man, and that this uniformity is evidence of common descent rather "
     "than of shared culture", {"darwin"}, {"expression", "emotions"}, False),
    # Short, names the author, has a year -- but the content words are the
    # claim, not the pointer.
    ("Mill 1859 said the peculiar evil of silencing an opinion is robbing the "
     "human race", {"mill"}, {"liberty"}, False),
]

# A depth-2 case flat quote-mark parity gets wrong: a real double-quoted
# phrase nested as the SECOND (or later) sibling inside a still-open
# single-quoted block. lex-e7keg's Wierzbicka citation is the real case
# that found this -- see _track_quote_nesting()'s docstring for the
# mechanism. Each entry is (atom id, a substring that must appear in some
# span, the kind that span must carry). Doesn't slot into POINTER_PROBES/
# SUBSTITUTION_PROBES's loop shapes above -- those run against pure
# strings; this needs an actual atom loaded and spans_of() run on it, so
# it gets its own small check in main()'s --calibrate branch instead.
NESTING_PROBES = [
    ("lex-e7keg", "I want to know something, I want someone to tell me",
     "quoted"),
]

DASHES = [("—", "-"), ("–", "-"), ("‐", "-"), ("‑", "-")]
QUOTES = [("’", "'"), ("‘", "'"), ("“", '"'), ("”", '"')]


def norm(t, is_source=False):
    """Normalize for comparison. Source text additionally gets its line-split
    words rejoined -- citations never contain those, so doing it on both sides
    would be a way to make two different words compare equal."""
    t = unicodedata.normalize("NFKC", t)
    for a, b in DASHES + QUOTES + [(" ", " ")]:
        t = t.replace(a, b)
    if is_source:
        t = re.sub(r"­\s*", "", t)        # soft hyphen: always a line split
        t = re.sub(r"-\s*\n\s*", "", t)        # hard hyphen at a line end
    t = re.sub(r"[\"']", "\x01", t)            # quote style is not a fidelity claim
    return re.sub(r"\s+", " ", t).strip()


# NFKD splits the accent off a composed letter and dropping the combining
# marks leaves the base behind. These have no decomposition at all, because a
# stroke or a ligature is part of the letterform rather than a mark over it,
# so they have to be named.
LIGATURES = str.maketrans({
    "ø": "o", "ß": "ss", "æ": "ae", "œ": "oe",
    "đ": "d", "ł": "l", "þ": "th", "ð": "d",
})


def despace(t):
    """Everything that is not a letter or digit, removed. This is the check
    that survives a bad scan: it cannot tell "mim icry" from "mimicry", which
    is the point, and it cannot be fooled about which letters are present,
    which is the other point.

    Quotation marks go too, which they did not at first. norm() collapses every
    quote character to one marker because quote STYLE is not a fidelity claim,
    but that left quote PRESENCE significant in the despaced tier, and a
    citation that drops the source's internal scare quotes then reads as a
    mismatch. James's Principles is printed as "Let us call the resting-places
    the 'substantive parts,' and the places of flight the 'transitive parts,'
    of the stream of thought" and the citation drops both pairs. Nothing about
    a word is different. This tier already ignores every other mark; the
    exception was an oversight, not a safeguard, and it cannot hide a
    substitution -- the letters still have to be present and in order.

    Accents fold rather than vanish, for the same reason. "Everything that is
    not a letter or digit" used to include every non-ASCII LETTER, so this
    tier saw "mlis" for Melies and could never match it against a source
    printing Melies. Three spans were reported on that basis and in all three
    the ATOM was the more careful text: lex-kdt2v has Piraha where the Lent PDF
    lost the tilde, lex-cf8kh has Georges Melies where the Savage Mind epub has
    it plain, lex-s2pjj has herkommlich where Glock's epub splits the umlaut
    into a combining mark. Atoms reported for getting the diacritic right.

    Losing a diacritic is scan damage, which is the one thing this tier is for.
    Folding still cannot hide a substitution -- e is not a different letter
    from e, and every letter must still be present and in order."""
    t = unicodedata.normalize("NFKD", t.lower()).translate(LIGATURES)
    return re.sub(r"[^a-z0-9]", "", t)


BLANK_LIMIT = 0.15


def blank_fraction(text):
    """Share of pages with almost no words on them.

    Density and word count are both WHOLE-DOCUMENT measures and neither can
    see a PARTIALLY scanned book. Levi-Strauss 1962 extracts 84,047 words at
    174 bytes each -- a healthy text layer by both tests -- while pages 11-18
    come out empty, and those eight pages are the bricoleur chapter that
    eighteen spans in lex-cf8kh cite. The phrase "the engineer" does not occur
    anywhere in the extraction of a book whose argument is bricoleur-versus-
    engineer, which is the shape of the failure: not a bad file, a file with
    a hole in it, reported as fully readable.
    """
    pages = text.split("\f")
    if len(pages) < 5:
        return 0.0
    return sum(1 for p in pages if len(p.split()) < 20) / len(pages)


def has_text_layer(path, text):
    """Whether an extraction is usable. A word count alone is not the test --
    a short paper legitimately has few words. The tell is bytes per extracted
    word: Alexander 1979 is a 32.8 MB scan yielding 2,749 words, about 12,000
    bytes each, because the pages are images and only the front matter carries
    text. A real text layer runs in the low hundreds at most."""
    n = len(text.split())
    try:
        density = os.path.getsize(path) / max(1, n)
    except OSError:
        density = 0
    return n >= 300 and density <= 2000 and blank_fraction(text) <= BLANK_LIMIT


_SYSTEM_WORDS = None


def _system_words():
    """/usr/share/dict/words, loaded once. None if the host doesn't have it --
    callers must treat that as "cannot classify", not "zero matches"."""
    global _SYSTEM_WORDS
    if _SYSTEM_WORDS is None:
        try:
            with open("/usr/share/dict/words", encoding="utf-8", errors="replace") as f:
                _SYSTEM_WORDS = set(w.strip().lower() for w in f)
        except OSError:
            _SYSTEM_WORDS = set()
    return _SYSTEM_WORDS


_PB_NUM = re.compile(r"^-?\s*\d{1,4}\s*-?$")
_PB_TITLE = re.compile(r"^[A-Z][A-Za-z'’,.\- ]{2,58}$")


def _pb_candidate(line):
    line = line.strip()
    if not line:
        return None
    if _PB_NUM.match(line):
        return ("num", None)
    if _PB_TITLE.match(line):
        return ("title", line)
    return None


def repair_page_breaks(text):
    """pdftotext inserts \\x0c at every page boundary by default. When a
    scanned or print-typeset book carries a running header and a page
    number, they land stranded right around that \\x0c -- and a sentence
    that happened to break across the page turn gets a running header and
    a page number spliced into the middle of it. Confirmed first by hand
    on lex-sjyty (Rappaport, 'The ritual form' / '53') and lex-xdc7z (Dunbar,
    '-4 -' / 'Talking Heads'); a corpus-wide survey found the same
    structural shape at 58,785 of 101,008 total page breaks across 415
    cached refs. Not rare -- the two hand-found cases were typical, not
    exceptional.

    A number-shaped line (1-4 digits, optionally dashed) is trusted on
    shape alone -- real prose is never an isolated short numeral by
    coincidence at any rate worth worrying about. A title-shaped line is
    trusted ONLY if the exact same string recurs 3+ times elsewhere in
    this same document's page-break margins. A real running header repeats
    verbatim every page (or every page of a chapter); a real sentence that
    merely starts with a capital letter and fits under the length cap does
    not. The first version of this function, gated on shape alone, deleted
    "Third, the concepts necessity and law," out of a real Brandom sentence
    -- it fit the shape once and was never seen again. That's a strictly
    worse failure than under-fixing: leaving a genuine header unstripped
    (which happens when OCR renders it slightly differently page to page,
    splitting its count across near-duplicate strings) costs nothing new:
    the passage stays exactly as unverifiable as it already was. Deleting
    real prose invents a claim that isn't there. Conservative on purpose.
    """
    if "\x0c" not in text:
        return text, 0

    chunks = text.split("\x0c")
    title_counts = {}
    for i, chunk in enumerate(chunks):
        edge_lines = (chunks[i - 1].split("\n")[-3:] if i > 0 else []) + chunk.split("\n")[:3]
        for line in edge_lines:
            c = _pb_candidate(line)
            if c and c[0] == "title":
                title_counts[c[1]] = title_counts.get(c[1], 0) + 1

    def trusted(line):
        c = _pb_candidate(line)
        if not c:
            return False
        return True if c[0] == "num" else title_counts.get(c[1], 0) >= 3

    out_parts = [chunks[0]]
    joins = []
    repaired = 0
    for chunk in chunks[1:]:
        # Work on copies -- if nothing turns out to be boilerplate, prev_lines
        # and cur_lines must revert untouched. Trimming blank lines BEFORE
        # knowing whether anything will be removed was the first version's
        # bug: an ordinary page break with no header at all (a genuine
        # chapter start, no boilerplate present) got its surrounding blank
        # lines stripped and both sides glued together with no separator
        # regardless, silently collapsing a real paragraph break every time
        # \x0c appeared with nothing to repair around it.
        prev_lines = out_parts[-1].split("\n")
        cur_lines = chunk.split("\n")
        trial_prev = list(prev_lines)
        while trial_prev and trial_prev[-1] == "":
            trial_prev.pop()
        removed_before = 0
        for _ in range(2):
            if trial_prev and trusted(trial_prev[-1]):
                trial_prev.pop()
                removed_before += 1
                while trial_prev and trial_prev[-1] == "":
                    trial_prev.pop()
            else:
                break
        trial_cur = list(cur_lines)
        while trial_cur and trial_cur[0] == "":
            trial_cur.pop(0)
        removed_after = 0
        for _ in range(2):
            if trial_cur and trusted(trial_cur[0]):
                trial_cur.pop(0)
                removed_after += 1
                while trial_cur and trial_cur[0] == "":
                    trial_cur.pop(0)
            else:
                break
        found = bool(removed_before or removed_after)
        if found:
            repaired += 1
            out_parts[-1] = "\n".join(trial_prev)
            out_parts.append("\n".join(trial_cur))
        else:
            out_parts[-1] = "\n".join(prev_lines)
            out_parts.append("\n".join(cur_lines))
        joins.append(" " if found else "")

    result = out_parts[0]
    for j, part in zip(joins, out_parts[1:]):
        result += j + part
    return result, repaired


_LIGATURE_RE = re.compile(r"[A-Za-z]*®[A-Za-z]*")  # 'reg. mark' fill-in for a dropped glyph


def repair_fi_ligature(text):
    """pdftotext can extract the 'fi' ligature glyph as (R) instead of the two
    letters, silently zeroing out every word containing it -- Rappaport's
    Ritual and Religion reads 0 hits for 'significance', 'define', 'office',
    'classification' across 239,619 words because each is missing its fi.

    (R) is not exclusive to this bug -- other fonts drop 'ss' or an accented
    letter into the same glyph, and scanned/OCR'd pages produce it as plain
    noise. Gating on a single candidate risks 'fixing' garbage into different
    garbage, so this checks the whole file's candidates against a dictionary
    (Rappaport: 2283/2546 decode to real words; the noise cases: 0 of a
    handful) and only replaces globally once the file clears a wide margin.
    Individual misses on a true positive are expected -- 'signi(R)cata',
    'af(R)nes', 'Se(R)roth' are correct fi-decodes a general wordlist just
    doesn't carry -- which is why the threshold reads the file, not the word.
    """
    words = _system_words()
    if not words or "®" not in text:
        return text
    candidates = [m.group(0) for m in _LIGATURE_RE.finditer(text) if len(m.group(0)) >= 3]
    if len(candidates) < 5:
        return text
    hits = sum(1 for w in candidates if w.replace("®", "fi").lower() in words)
    if hits / len(candidates) >= 0.5:
        return text.replace("®", "fi")
    return text


OCR_ENABLED = True     # --no-ocr turns this off; see main()


def extract(path, ocr=True):
    """Text of one ref, cached.

    A PDF with no text layer is OCR'd rather than skipped. Doing that by hand
    was already the practice -- lex-kkr43's citation says "(OCR'd via
    scripts/ocr-pdf.py -- the scanned PDF has no native text layer)" -- but
    nothing invoked it automatically and the output was never kept, so the
    next thing that needed the same book paid the cost again or, worse,
    silently treated the book as unverifiable. An unverifiable source is
    indistinguishable from a verified one in any report that only counts
    mismatches, which is the failure this whole script exists to close.

    OCR output lands in the same cache as any other extraction and is marked
    with a header, so a reader of a quote sourced from it can tell that the
    text was reconstructed from pixels rather than read from a text layer.
    """
    # A path that is not there is a BUG in the caller, not an empty book.
    # Returning "" for it meant a mistyped filename read as "this source
    # contains none of the words you are looking for" -- during the 2026-08-05
    # audit that produced a confident "Fisher 1925 and Wittgenstein's
    # Investigations extract to ZERO WORDS, every atom citing them looks
    # fabricated", when both filenames were simply wrong (the real ones carry
    # -Anscombe and -Anscombe-v2 suffixes). Silent skips are how unverifiable
    # sources become indistinguishable from verified ones, which is the failure
    # this script exists to close. Always find refs with `lexicon refs <query>`.
    if not os.path.exists(path):
        raise FileNotFoundError(
            f"{path} does not exist -- find it with `lexicon refs <query>`")
    os.makedirs(CACHE, exist_ok=True)
    out = os.path.join(CACHE, os.path.basename(path) + ".txt")
    # A cache entry older than the file it caches is not a cache entry, it is a
    # memory of a different file. Without this check the entry is authoritative
    # forever, and the way that goes wrong is quiet: Alexander's Timeless Way
    # was OCR'd on 2026-08-04, the sidecar finished at 23:51 with 122,021 words
    # in it, and an extraction at 23:13 -- while the OCR was still writing --
    # had already cached the file as empty. Every run afterwards read zero
    # words from a book that was sitting there complete, and reported its
    # quotes absent. Re-OCR would not have helped; nothing invalidated.
    #
    # Also fires on the ordinary case: a ref replaced by a better copy under
    # the same name, which is exactly what a reacquisition does.
    if os.path.exists(out):
        try:
            fresh = os.path.getmtime(out) >= os.path.getmtime(path)
            side = os.path.splitext(path)[0] + ".ocr.txt"
            if fresh and os.path.exists(side):
                fresh = os.path.getmtime(out) >= os.path.getmtime(side)
        except OSError:
            fresh = True
        if fresh:
            return open(out, encoding="utf-8", errors="replace").read()
        os.remove(out)
    ocr = ocr and OCR_ENABLED
    ext = os.path.splitext(path)[1].lower()
    text = ""
    try:
        if ext == ".pdf":
            text = subprocess.run(["pdftotext", path, "-"], capture_output=True,
                                  timeout=180).stdout.decode("utf-8", "replace")
            text = repair_fi_ligature(text)
        elif ext in (".epub", ".mobi", ".azw3"):
            text = subprocess.run(["pandoc", "-f", "epub", "-t", "plain", path, "-o", "-"],
                                  capture_output=True, timeout=180).stdout.decode("utf-8", "replace")
            if len(text.split()) < 300 and ext == ".epub":
                text = epub_fallback(path) or text
            if len(text.split()) < 300 and ext == ".epub":
                # Dehaene-2011-NumberSense.epub identifies as an EPUB and is
                # not a valid zip, so both pandoc and the zip fallback return
                # nothing. Calibre reads it anyway: 0 words -> 139,234. Three
                # readers, three different failure modes, and a book is only
                # unreadable when all three decline.
                text = calibre_convert(path) or text
            if len(text.split()) < 300 and ext in (".mobi", ".azw3"):
                # pandoc does not read mobi or azw3 at all. It was being handed
                # them anyway and returning nothing, which read as "this book
                # has no quotes in it" rather than "this book was never opened".
                text = calibre_convert(path) or text
        elif ext == ".djvu":
            # .djvu was in the ref index but had no handler here at all, so
            # every djvu extracted to the empty string and every atom citing
            # one was reported as having no text layer -- Brandom's Making It
            # Explicit and all four volumes of Pareto's Mind and Society.
            text = subprocess.run(["djvutxt", path], capture_output=True,
                                  timeout=300).stdout.decode("utf-8", "replace")
        elif ext in (".txt", ".md"):
            text = open(path, encoding="utf-8", errors="replace").read()
        elif ext == ".zip":
            # An Internet Archive page dump: one OCR'd .txt per page, named by
            # page number inside a single item directory. It is a whole book,
            # and it was invisible -- .zip was not in the indexed extensions,
            # so Sperber and Wilson's Relevance could not be reached at all and
            # three atoms citing it were answered with the nearest surname
            # match instead, first E. O. Wilson's The Insect Societies and then
            # Nisbett and Wilson. Reassembling 292 page files in page order
            # gives 122,043 words and clears all 11 of their quoted findings.
            text = zip_pages(path)
        elif ext == ".html":
            # HTML was being read raw, markup and all, so any quote spanning an
            # element boundary could never match: Raymond's two sentences are
            # separated in the file by "</p></blockquote><p xmlns=...>", and
            # despacing turns that into 40-odd characters of tag soup wedged
            # into the middle of the passage. Strip to text first.
            text = strip_html(open(path, encoding="utf-8", errors="replace").read())
    except Exception:
        text = ""

    needs_ocr = ext == ".pdf" and not has_text_layer(path, text)
    # An EXISTING sidecar is read even under --no-ocr. The flag is there to
    # refuse an hour of page rendering, not to refuse a file already sitting
    # next to the PDF -- and conflating the two meant every --no-ocr run threw
    # away work a previous run had already paid for. It cost an hour here:
    # Lent was OCR'd, the sidecar written, and three separate --no-ocr checks
    # afterwards reported the same 169,511 words and the same missing
    # passages, so the OCR looked like it had failed when it had not been read.
    sidecar = os.path.splitext(path)[0] + ".ocr.txt"
    if ocr and needs_ocr and not (ALLOW_OCR) and os.path.exists(sidecar):
        ocr_text = open(sidecar, encoding="utf-8", errors="replace").read()
        if len(ocr_text.split()) > 300:
            text = (text + "\n\f[OCR: reconstructed from page images]\n"
                    + ocr_text)
    elif ocr and ALLOW_OCR and needs_ocr:
        ocr_text = run_ocr(path)
        if len(ocr_text.split()) > 300:
            # APPEND rather than replace. On a book with no text layer at all
            # the distinction is moot, but on a partial scan the embedded text
            # is the CLEANER of the two and replacing it would trade a page
            # that already matches for a re-OCR'd copy of the same page with
            # fresh character errors in it. A span only has to be found once,
            # so carrying both costs a longer haystack and cannot lose a match
            # that either half already had.
            text = (text + "\n\f[OCR: reconstructed from page images]\n"
                    + ocr_text)
        needs_ocr = False

    text, _ = repair_page_breaks(text)

    # Do NOT cache an extraction we deliberately declined to complete. Writing
    # the empty text layer of an image-only scan into the cache would make the
    # next full audit read that instead of OCR-ing, and the source would be
    # permanently and silently unverifiable -- the cache would remember a
    # decision made for a pre-commit hook's latency budget as if it were a
    # fact about the book.
    if not needs_ocr:
        open(out, "w", encoding="utf-8").write(text)
    return text


def strip_html(raw):
    """Markup out, text and entities in."""
    import html as _html
    raw = re.sub(r"(?is)<(script|style)[^>]*>.*?</\1>", " ", raw)
    raw = re.sub(r"(?is)<br\s*/?>|</(p|div|li|h[1-6]|blockquote|tr)>", "\n", raw)
    return re.sub(r"[ \t]+", " ", _html.unescape(re.sub(r"<[^>]+>", " ", raw)))


def zip_pages(path):
    """Reassemble an Internet Archive per-page text dump into one book.

    The archive holds one OCR'd .txt per page, named by page number under a
    single item directory. Sorted numerically and joined, that is the book.
    Sorted lexically it is not: page 100 would land between 10 and 11, which
    scrambles the reading order and turns every multi-page quote into a
    finding -- the exact defect this script spends its time telling apart from
    fabrication, so it is worth not manufacturing here.
    """
    import zipfile
    try:
        with zipfile.ZipFile(path) as zf:
            names = [n for n in zf.namelist() if n.lower().endswith(".txt")]
            if not names:
                return ""

            def page(n):
                m = re.search(r"(\d+)\.txt$", n, re.I)
                return (int(m.group(1)) if m else 0, n)

            parts = []
            for n in sorted(names, key=page):
                t = zf.read(n).decode("utf-8", "replace").strip()
                if t:
                    parts.append(t)
            return "\n".join(parts)
    except (zipfile.BadZipFile, OSError):
        return ""


def epub_fallback(path):
    """Read an epub as the zip of XHTML it is, when pandoc will not.

    pandoc dies on Mauss-1925-TheGift-Cunnison.epub with "parseSpine" -- a
    malformed spine -- and returned an empty string. Nothing noticed: an empty
    extraction is indistinguishable from a book with no quotes in it, so the
    atoms citing that translation were silently checked against the OTHER
    translation on disk and every passage read as not found. A silent-empty
    extraction is the worst failure this checker can have, because the report
    it produces looks exactly like a report about a book it actually read.
    """
    import zipfile
    try:
        with zipfile.ZipFile(path) as z:
            names = [n for n in z.namelist()
                     if n.lower().endswith((".xhtml", ".html", ".htm"))]
            parts = []
            for n in sorted(names):
                try:
                    raw = z.read(n).decode("utf-8", "replace")
                except Exception:
                    continue
                raw = re.sub(r"(?is)<(script|style)[^>]*>.*?</\1>", " ", raw)
                parts.append(re.sub(r"<[^>]+>", " ", raw))
            return re.sub(r"[ \t]+", " ", "\n".join(parts))
    except Exception:
        return ""


def calibre_convert(path):
    """mobi/azw3 via ebook-convert. Output lands in the cache dir, never /tmp,
    which is a tmpfs on this host."""
    os.makedirs(CACHE, exist_ok=True)
    out = os.path.join(CACHE, os.path.basename(path) + ".conv.txt")
    try:
        subprocess.run(["ebook-convert", path, out], capture_output=True, timeout=600)
        if os.path.exists(out):
            t = open(out, encoding="utf-8", errors="replace").read()
            os.remove(out)
            return t
    except Exception:
        pass
    return ""


def run_ocr(path, dpi=300):
    """Render pages and OCR them, leaving the text NEXT TO THE PDF.

    ocr-pdf.py already defaults its output to <pdf-stem>.ocr.txt beside the
    source, which is the right place: a sidecar in refs/ is where anyone --
    a person, a grep, a later script -- will look for it. Polya 1945 was
    OCR'd months ago and the text is gone, because that run passed --output
    at a temp path on a tmpfs. The default was correct and the invocation
    overrode it, so this deliberately does not pass --output at all.

    Slow, minutes per book, which is why it only fires when there is no text
    layer and why the result is kept.
    """
    ocr_script = os.path.join(ROOT, "scripts", "ocr-pdf.py")
    if not os.path.exists(ocr_script):
        return ""
    sidecar = os.path.splitext(path)[0] + ".ocr.txt"
    if os.path.exists(sidecar):
        return open(sidecar, encoding="utf-8", errors="replace").read()
    try:
        subprocess.run([sys.executable, ocr_script, path, "--dpi", str(dpi)],
                       capture_output=True, timeout=7200)
        if os.path.exists(sidecar):
            return open(sidecar, encoding="utf-8", errors="replace").read()
    except Exception:
        pass
    return ""


def load_atom(path):
    return load_atom_text(open(path, encoding="utf-8").read())


def load_atom_text(text):
    """Parse one elements file's text, tolerating the hybrid format.

    33 atoms are YAML front-matter, then a `---`, then a markdown body --
    lex-xrg6e and the lex-r3aug..0654 run. Go's loader reads the first document
    and ignores the tail, which is the intended design. PyYAML's safe_load
    raises "expected a single document in the stream" on all 33.

    The first version of this script wrapped safe_load in a bare
    `except Exception: continue`, so those 33 atoms were skipped in silence
    and counted as nothing at all -- not checked, not reported, not in any
    total. A verifier whose failure mode is quietly narrowing its own scope
    is the same defect it was written to catch.
    """
    try:
        d = yaml.safe_load(text)
        if isinstance(d, dict) and d.get("id"):
            return d
    except yaml.YAMLError:
        pass
    # Hybrid file. safe_load_all does not help -- the body after the separator
    # is markdown, not a second YAML document -- so cut at the separator and
    # parse only the head. Two shapes exist: a leading `---` with the front
    # matter between the first and second (lex-asu3y), and no leading `---`
    # with a single separator before the prose (lex-xrg6e).
    lines = text.split("\n")
    start = 1 if lines and lines[0].strip() == "---" else 0
    end = len(lines)
    for i in range(start, len(lines)):
        if lines[i].strip() == "---":
            end = i
            break
    d = yaml.safe_load("\n".join(lines[start:end]))
    return d if isinstance(d, dict) and d.get("id") else None


CAMEL = re.compile(r"([a-z])([A-Z])")
NONAL = re.compile(r"[^a-z0-9]+")


STOP = {
    "the", "and", "for", "von", "der", "des", "del", "vol", "complete",
    "works", "edition", "trans", "translated", "selected", "advanced",
    "revised", "with", "from", "his", "her", "its", "book", "books", "text",
    "full", "part", "new", "ed", "nd", "archiveorg", "pubmed", "abstract",
    "ocr", "pdf", "epub", "chapter", "chapters",
}


# A platform is not an author. These tokens name where something was published,
# so two files sharing one of them share nothing about authorship, and a
# citation resolved on one alone has not been resolved.
#
# lex-rn9ht cites "wikipedia-hypervigilance". No such article is on disk. The
# author-only fallback matched four files on the bare token "wikipedia" --
# three xkcd transcripts and a Wikipedia dump of Conway's Game of Life -- and
# file size broke the tie, so the report said the atom had been checked against
# a comic about caltrops and its quotes were absent. That is strictly worse
# than reporting no source: a missing source says "cannot check this", while a
# wrong source says "checked, and the quote is not there", and the only
# editable thing in reach is the atom. Eighty-five findings named a numbered
# transcript this way.
#
# Frequency cannot make this distinction -- "wikipedia" indexes 4 files and
# "aristotle" indexes 5. Only the semantics separate them.
PLATFORM = {
    "wikipedia", "wikiquote", "wiktionary", "wikisource", "zim", "xkcd",
    "youtube", "reddit", "arxiv", "jstor", "pubmed", "gutenberg", "archive",
    "imdb", "britannica", "medium", "substack", "stackoverflow",
    "stackexchange", "transcript", "subtitles", "podcast", "episode",
    "blog", "post", "article", "unknown", "untitled", "misc",
}


CITE_YEAR = re.compile(r"\b(1[6-9]\d\d|20\d\d)\b")


def _first_year(text):
    """The leftmost 1600-2099 year, or None.

    Leftmost is deliberate: the filename convention puts the WORK's year first
    and any edition year after it, so Peter-Hull-1969-ThePeterPrinciple-
    2009CollinsBusinessEd reads as 1969 and matches a citation of the 1969
    book rather than looking like a 2009 one.
    """
    m = CITE_YEAR.search(text or "")
    return m.group(1) if m else None


def _leading_token(text):
    """The first word of a citation or a filename stem, or None.

    Platform names are skipped for the same reason author_tokens skips them:
    the leading word of "Wikipedia-PeterPrinciple-2025-12" is not the author.
    """
    toks = [t for t in re.split(r"[^a-z0-9]+", (text or "").lower())
            if len(t) > 2]
    while toks and toks[0] in PLATFORM:
        toks = toks[1:]
    return toks[0] if toks else None


def author_tokens(text):
    """The tokens of a citation slug that name its AUTHOR, not its title.

    Citation slugs put the author first: "olson-1971-logic-of-collective-
    action", "shakespeare-othello-act-III-scene-iii". With a year present the
    author is whatever precedes it, capped at three tokens so a three-author
    paper still resolves. With no year there is nothing to delimit the author
    from the title, and taking three tokens then swallows the title whole --
    "genesis-tradition" would offer "tradition" as an author name, which is
    how an xkcd comic called Tradition came to be a source for Genesis.
    """
    toks = [t for t in re.split(r"[^a-z0-9]+", (text or "").lower()) if len(t) > 2]
    if not toks:
        return set()
    # A leading platform name is not the author, so the token after it is the
    # closest thing the citation has to one. "wikipedia-hypervigilance" names
    # an article, and its head is "hypervigilance"; treating "wikipedia" as the
    # head meant the rule that stops xkcd transcripts answering these also
    # stopped the actual article from answering them, once the articles were
    # finally saved. Both halves matter: a platform token still admits
    # nothing on its own.
    while len(toks) > 1 and toks[0] in PLATFORM:
        toks = toks[1:]
    m = CITE_YEAR.search(text or "")
    if not m:
        return {toks[0]}
    head = [t for t in re.split(r"[^a-z0-9]+", (text or "")[:m.start()].lower())
            if len(t) > 2]
    return set(head[:3]) if head else {toks[0]}


def author_only_admissible(lineage_text, cand):
    """Whether an author-only candidate is a resolution or a coincidence.

    The author-only keys are deliberately coarse (see author_only_keys), and
    coarseness was argued to be free on the grounds that a span is a finding
    only when it fails against EVERY candidate, so a wider pool can only
    reduce false alarms. That argument does not survive the cap: check_atom
    ranks candidates and keeps the top four, so a pool widened with junk can
    push the real source out of the list, and it can furnish four confident
    wrong answers where the honest answer is "not on disk". Eighty-five
    findings named a numbered xkcd transcript as their source this way.

    Admit on either of two grounds:

      the author matches -- a lone surname is enough, and has to be, because
        "shakespeare" alone is what reaches Shakespeare-Complete-Works.txt
        from a citation of Othello;

      or three or more title tokens match with no author agreement at all,
        which is what rescues "olson-1971-logic-of-collective-action" from a
        file the renamer clipped to JR-1971-TheLogicOfCollectiveAction.epub.

    Two matching title tokens are not enough:
    "mitchell-russo-pennington-1989-back-to-the-future" shares "back" and
    "future" with the xkcd strip 0102-Back_to_the_Future.txt and shares no
    author with anything, which is a title collision rather than a citation.

    A surname stops being enough the moment both sides name a year and the
    years disagree. "carroll-1871-through-the-looking-glass" was answered with
    Sean Carroll's 2016 The Big Picture and the Red Queen's line reported
    absent from it -- two authors, one surname, a hundred and forty-five years
    apart. The fallback exists for citations whose year is MISSING (antiquity,
    an undated reprint, a complete-works volume); a year on both sides that
    disagrees is evidence against the match, not the absence of evidence.

    On a conflict, three things can still rescue the pairing, and it took all
    three to avoid trading one wrong answer for another:

      agreement on something that is not a name, which admits a different
        edition of the same work: hobbes-1651-leviathan against
        Hobbes-1909-Leviathan agrees on "leviathan";

      or the same LEADING author on both sides, within twenty-five years.
        An essay is routinely cited by its own year and held in a later
        collection -- lorde-1979-the-masters-tools against
        Lorde-1984-SisterOutsider, which is where that essay is. A bare gap
        test was the first attempt and it is not enough on its own:
        gamma-helm-johnson-vlissides-1994 sits fourteen years from
        Lakoff-Johnson-1980, well inside any workable window, and matches on
        "johnson" -- third author on one side, second on the other. Requiring
        the LEADING author kills that pairing while keeping Lorde's.

    Twenty-five years is where a reprint or a collection stops being the
    likelier story than a coincidence of surname. It keeps Alexander 1977
    against Alexander 1979 -- companion volumes, so the report names the wrong
    one, but it names the right author's other book rather than a stranger's.
    It drops Jay Garfield on Nagarjuna reaching Eugene Garfield on citation
    indexes, forty years apart.
    """
    stem = os.path.splitext(os.path.basename(cand))[0]
    shared = (_title_tokens(lineage_text) & _title_tokens(stem)) - PLATFORM
    if not shared:
        return False
    cy, fy = _first_year(lineage_text), _first_year(stem)
    at = author_tokens(lineage_text)
    if cy and fy and cy != fy:
        # A year conflict is evidence AGAINST the pairing, so it has to raise
        # the bar, not lower it. The first version of this branch admitted on
        # any single shared non-author token -- a WEAKER test than the
        # no-conflict path below, which wants author agreement or three tokens.
        # "your" was enough to answer hartman-2007-lose-your-mother with
        # Pollan's How to Change Your Mind and report Saidiya Hartman's opening
        # page missing from it. Same line put Greene's 48 Laws against Anil
        # Seth on "you" and Schull's Addiction by Design against Jesse Schell's
        # Art of Game Design on "design".
        if shared & at:
            # Same author, different year: a reprint, a collection, a second
            # edition. A shared title word settles it -- hobbes-1651-leviathan
            # against Hobbes-1909-Leviathan agrees on "leviathan".
            if shared - at:
                return True
            if abs(int(cy) - int(fy)) > 25:
                return False
            return _leading_token(lineage_text) == _leading_token(stem)
        # No author agreement AND the years disagree. Nothing is left but a
        # title that matches on its own terms, at the same bar the clean path
        # uses -- which is what still rescues a citation whose author the
        # renamer mangled, without admitting a title collision.
        return len(shared - at) >= 3
    if shared & at:
        return True
    return len(shared) >= 3


def author_only_keys(stem):
    """Author keys for a stem whose year is missing or not a modern number.

    author_year_keys requires a 1600-2099 year and returns NOTHING without
    one, on both sides of the lookup -- so a citation with no usable year gets
    no candidates, and a FILE with no usable year is never indexed at all.
    The consequence measured 2026-08-05: 936 quoted spans across 309 atoms,
    32% of the elements' quoted text, reported as having no source on disk.
    Six of the eight worst were sitting in refs/ the whole time:

      lent-2017-...                  Lent-ND-ThePatterningInstinct...pdf
      marcus-aurelius-...-c170-180-ce   MarcusAurelius-C170-Meditations-Long.epub
      laozi-tao-te-ching-c6-c4-bce   Laozi-C5BCE-TaoTeChing-Legge.epub
      aristotle-nicomachean-ethics-c350-bce
                                     aristotle_nicomachean-ethics_f-h-peters...epub
      olson-1971-logic-of-collective-action
                                     JR-1971-TheLogicOfCollectiveAction...epub
      shakespeare-othello-act-III-scene-iii
                                     Shakespeare-Complete-Works.txt

    Antiquity, undated reprints, a surname the renamer clipped to "Jr", and a
    complete-works volume with no year at all. None of them is a procurement
    gap; all of them read as one.

    A first version took only the first two tokens of four-plus characters,
    and that was still too narrow because a filename need not lead with the
    surname: `henry-david-thoreau_walden_advanced.epub` keyed on "henry" and
    "david", and `sun-tzu_the-art-of-war_lionel-giles_advanced.epub` keyed on
    "lionel" and "giles" -- the translator. Both books were on disk and
    unreachable. Every token now becomes a key, stoplisted and capped, and
    rank_refs does the discriminating it already does.

    These keys are deliberately coarse and are a FALLBACK only -- check_atom
    reaches for them after author-year has produced nothing, exactly as it
    already does for the collected-essay case. Coarseness costs candidates,
    not correctness: a span is a finding only when it fails against EVERY
    candidate, so a wider pool can only reduce false alarms.
    """
    s = NONAL.sub("-", CAMEL.sub(r"\1-\2", stem).lower()).strip("-")
    out = []
    for t in s.split("-"):
        if len(t) < 3 or t.isdigit() or t in STOP:
            continue
        k = "author:" + t
        if k not in out:
            out.append(k)
        if len(out) >= 8:
            break
    return out


def author_year_keys(stem):
    s = NONAL.sub("-", CAMEL.sub(r"\1-\2", stem).lower()).strip("-")
    parts = s.split("-")
    yi, year = -1, None
    for i, p in enumerate(parts):
        if len(p) == 4 and re.fullmatch(r"(1[6-9]|20)\d\d", p):
            year, yi = p, i
            break
    if not year:
        return []
    ks = []
    for i in range(yi - 1, -1, -1):
        t = parts[i]
        if len(t) < 3 or t in ("the", "and", "von", "der", "des", "del", "vol"):
            continue
        ks.append(f"{t}-{year}")
        if len(ks) >= 3:
            break

    # A citation can name more than one source, and stopping at the first year
    # means the rest are never looked up at all. lex-ukgke cites
    # "doctorow-2022-11-28-pluralistic-amazon-coinage-and-2023-01-21-pluralistic-..."
    # -- both posts, correctly -- and resolved only to the 2022 one, so every
    # quote from the January 2023 post ("Here is how platforms die") was
    # checked against the November 2022 post and reported as not found. The
    # citation was right and the lookup was wrong.
    for j, p2 in enumerate(parts):
        if j <= yi or not re.fullmatch(r"(1[6-9]|20)\d\d", p2):
            continue
        for i in range(yi - 1, -1, -1):
            t = parts[i]
            if len(t) < 3 or t in ("the", "and", "von", "der", "des", "del", "vol"):
                continue
            k = f"{t}-{p2}"
            if k not in ks:
                ks.append(k)
            break
    return ks


def build_ref_index():
    """Refs keyed by author-year, and also by author alone.

    The author-only keys exist because an essay's publication year and the
    year of the volume that collects it are different numbers, and the
    citation naturally gives the first. lex-z775k cites Hayek's "Individualism:
    True and False" (1945); the text is in Hayek-1948-IndividualismAndEconomic
    Order.pdf, which is on disk. author_year_keys yields "hayek-1945", the
    file is keyed "hayek-1948", they never meet, and three verbatim quotes
    were reported as absent from a book nobody had opened. Author-only keys
    are a FALLBACK -- check_atom uses them only when every author-year
    candidate has already failed -- because on their own they would offer
    every Foucault to every Foucault citation.
    """
    idx = {}
    for dirpath, _, files in os.walk(REFS):
        for f in files:
            if os.path.splitext(f)[1].lower() not in (
                    ".pdf", ".epub", ".djvu", ".mobi", ".azw3", ".lit", ".txt",
                    ".html", ".md", ".zip"):
                continue
            p = os.path.join(dirpath, f)
            stem = os.path.splitext(f)[0]
            for k in author_year_keys(stem):
                idx.setdefault(k, []).append(p)
            # Index EVERY file by author alone, including the ones with no
            # modern year, so a citation of antiquity or of an undated reprint
            # has something to find.
            for k in author_only_keys(stem):
                # author_only_keys already returns "author:<token>" with no
                # year to strip, so the old second line here produced
                # "author:author:<token>" -- 6,228 keys nothing ever looked
                # up, doubling the index for no lookup.
                idx.setdefault(k, []).append(p)
    return idx


# An apostrophe inside a word is punctuation, not a quotation mark. Without
# this, every possessive and contraction in an atom's own commentary opens a
# "quoted run" that then fails to match any source, because it was never a
# claim about a source. lex-6qevt's gloss "...Cooper's transcription with
# editorial framing. The form attests Cooper..." was reported as an unmatched
# verbatim span reading "s transcription with editorial framing. The form
# attests Cooper" -- a fabrication-shaped finding manufactured entirely by the
# checker. Excluding these is a correctness fix and not a loosening: nothing
# here was ever offered as verbatim.
INWORD_APOS = re.compile(r"(?<=[A-Za-z])['’](?=[A-Za-z])")

# A plural possessive ends its word with the apostrophe, so INWORD_APOS -- which
# needs a letter on both sides -- leaves it standing as a lone quote character.
# One of those flips the parity of everything after it. lex-4snxu writes "the
# implementers' creative talents go into making the design work", the atom's own
# gloss, and it was reported as an unmatched verbatim quotation of Brooks.
#
# Over-masking is the safe direction here and is why this can be a plain rule.
# If it eats a real closing quote, the quotation simply runs longer than it
# should and still reports as quoted -- today's behaviour. Under-masking invents
# a quotation that was never claimed, which is what sends someone to rewrite an
# atom that was never wrong.
PLURAL_POSS = re.compile(r"(?<=s)['’](?=\s+[a-z])")
QUOTE_CH = "\"'‘’“”"


SINGLE_OPEN, SINGLE_CLOSE, SINGLE_AMBIG = "‘", "’", "'"
DOUBLE_OPEN, DOUBLE_CLOSE, DOUBLE_AMBIG = "“", "”", '"'


def _track_quote_nesting(text):
    """Nesting-aware replacement for flat QUOTE_CH parity counting.

    Flat counting treats every character in QUOTE_CH as one undifferentiated
    class, so it can only ever distinguish EVEN from ODD -- it conflates
    depth-0 (truly outside any quote) with depth-2 (inside a SECOND, or any
    even-numbered, sibling inner quote) as the same "even" state. A
    single-quoted block containing one self-contained nested double-quote
    (open-close, net +2, parity unchanged) reads fine; a block containing
    two or more SIBLING double-quoted sub-phrases does not -- by the time
    the second sibling's own opening quote is reached, the running count
    has ticked back to even, so that sibling's content reads as "between"
    even though the outer single-quote never closed. lex-e7keg's Wierzbicka
    citation hit this: a single-quoted paragraph containing three sibling
    double-quoted NSM formulas, the second and third of which -- "I want to
    know something, I want someone to tell me" among them -- read as
    unclaimed connective prose and silently stopped being checked, purely
    because they were the second and third sibling rather than the first.

    Track two independent one-level booleans, single_open and double_open,
    instead of one flat count. Curly quotes are unambiguous by glyph and
    always assert their direction; straight quotes are ambiguous and toggle
    their family's state, exactly like today's flat scan, just scoped to
    the right family. A plural-possessive-shaped straight single quote
    (PLURAL_POSS) is masked instead of toggled when single_open is
    currently FALSE -- outside any open quote, where this shape is
    overwhelmingly a real possessive in ordinary prose (lex-4snxu's own
    gloss, "the implementers' creative talents...", is the historical
    case). When single_open is already true, the same shape is left to
    toggle normally instead, because that position is almost always the
    scare-phrase's own close (lex-dwey5's "'free marketplace of ideas'"
    is that case) -- masking it there is what
    _mask_apostrophes_stateful (this function's single-family-only
    predecessor, added earlier this session) existed to prevent. A stray
    curly close with nothing open is masked too, same over-masking-is-safe
    direction as PLURAL_POSS itself -- most likely a possessive apostrophe
    INWORD_APOS's letter-on-both-sides requirement didn't catch (a leading
    elision, an OCR artifact), and treating it as noise instead of forcing
    a family closed from nothing avoids corrupting whatever comes after.

    Known residual imprecision, applying this to the quote: field for the
    first time: the "toggle when already open" rule assumes a possessive
    reached mid-quote is almost always the scare-phrase's own close, which
    holds for citation:'s short scare-phrases but not always for quote:'s
    long continuous passages -- lex-hrnuj's own "subjects' presence" and
    lex-v5fqf's "others' praise" are both genuine possessives deep inside
    otherwise-continuous verbatim quotes, and get toggled (treated as a
    close) rather than masked, splitting one span into two. Confirmed
    non-regressive by hand this session: both split pieces were already
    failing to match as ONE unsplit span before this function existed
    (unrelated OCR noise in both sources), so the split changes reported
    granularity, not what gets checked or why it fails. Left as a known
    limitation rather than chased further -- the fix would need to be
    speculative (nothing here distinguishes "long quote, real possessive"
    from "short scare-phrase, real close" except length, which is a weak
    signal), and this file's history is that a speculative widening here
    is exactly what makes things worse.

    Returns (masked_text, before_state): masked_text has \\x02 in place of
    every character this function decided not to count, and before_state
    is a per-index array where before_state[i] is True iff a quote (either
    family) was open strictly before position i was processed. Run capture
    regexes against masked_text, never against the raw input -- that is
    what closes off apostrophes and stray closes from splitting spans, the
    same job PLURAL_POSS/INWORD_APOS's blanket .sub() calls used to do.

    Today's "quoted" if (inside or not balanced) else "between" whole-part
    escape hatch (force every span in a part to quoted once ANY quote in
    that part is left unclosed, even spans positioned before the eventual
    unclosed quote) is not ported. Its own fail-safe property -- an
    unclosed quote leaves everything after it quoted -- falls out of this
    per-position tracking directly: an open single_open or double_open
    simply never closes for the rest of the text, so before_state stays
    True through to the end. The new version is strictly more precise
    (scoped to each span's own position rather than the whole part), which
    is expected to produce a small number of quoted-to-between corrections
    for spans that sit before an unrelated trailing unclosed quote in the
    same part -- not a regression.
    """
    text = INWORD_APOS.sub("\x02", text)
    chars = list(text)
    before_state = [False] * (len(text) + 1)
    single_open = False
    double_open = False
    for i, ch in enumerate(text):
        before_state[i] = single_open or double_open
        if ch == SINGLE_OPEN:
            single_open = True
        elif ch == SINGLE_CLOSE:
            if single_open:
                single_open = False
            else:
                chars[i] = "\x02"
        elif ch == SINGLE_AMBIG:
            if not single_open and PLURAL_POSS.match(text, i):
                chars[i] = "\x02"
            else:
                single_open = not single_open
        elif ch == DOUBLE_OPEN:
            double_open = True
        elif ch == DOUBLE_CLOSE:
            if double_open:
                double_open = False
            else:
                chars[i] = "\x02"
        elif ch == DOUBLE_AMBIG:
            double_open = not double_open
    before_state[len(text)] = single_open or double_open
    return "".join(chars), before_state


LOCATOR = re.compile(
    r"\b(ch(ap(ter)?)?\.?\s*\d|p{1,2}\.\s*\d|§|introduction|afterword|appendix|"
    r"preface|conclusion|commandment|epilogue|prologue|vol(ume)?\.?\s*\d|"
    r"book\s+[ivx\d]|part\s+[ivx\d]|\b1[6-9]\d\d\b|\b20\d\d\b)", re.I)

TRAILING_PAREN = re.compile(r"\s*\(([^()]{6,120})\)[\s.]*$")
# The same pointer with its closing paren lost. Quote characters delimit spans
# and are stripped before this runs, so an attribution containing a quoted
# chapter title gets cut at that quote: lex-m43kg's "(Eyal, Hooked, Ch.3
# 'Action.')" reaches here as "(Eyal, Hooked, Ch.3" with nothing to close it.
TRAILING_OPEN = re.compile(r"\s*\(([^()]{4,120})$")


def _is_pointer(text, surnames, title_words):
    """Whether a fragment only POINTS at the source rather than quoting it.

    "(Hohwy, *The Predictive Mind*, Ch.9, pp.193-195)" is not a claim about
    what Hohwy wrote, but the span scanner cannot tell it from one, so it was
    checked against the book and reported as absent -- correctly, since the
    book does not contain its own citation. The test is deliberately narrow:
    the cited author's surname must be present, a structural locator must be
    present, and once both are removed along with words already in the
    citation, almost nothing may be left. A fragment with real content words
    in it is a quote that happens to name its author, and stays a quote.
    """
    if len(text) > 130:
        return False
    low = text.lower()
    if not any(s and s in low for s in surnames):
        return False
    if not LOCATOR.search(text):
        return False
    rest = LOCATOR.sub(" ", text)
    for s in surnames:
        rest = re.sub(re.escape(s), " ", rest, flags=re.I)
    left = [w for w in re.findall(r"[A-Za-z]{3,}", rest)
            if w.lower() not in title_words and w.lower() not in {"and", "the", "see", "also"}]
    return len(left) <= 3


def spans_of(entry, with_kind=False):
    """Quoted spans an entry claims as verbatim: everything inside the quote:
    field that is not our own bracketed gloss, plus anything in double quotes
    inside the citation.

    With with_kind, each span comes back as (text, kind) where kind is
    "quoted" for anything delimited by quote marks and "unquoted" for a bare
    run of connective prose. All twelve fabrications found so far were
    "quoted"; the unquoted runs are overwhelmingly the atom's own commentary,
    so the distinction is the difference between a report worth reading by
    hand and one that is 19% the elements talking to itself.
    """
    out = []
    q = entry.get("quote") or ""
    # A bracketed ellipsis -- "[...]" -- is house style for "material elided
    # here" (116 atoms use it), and it reads as an ordinary editorial mark to
    # a person. But the gloss-split below treats ANY [bracket] as a gloss to
    # strip, with no exception for one that holds nothing but dots, and
    # stripping it SPLITS the surrounding text into two independent parts.
    # Parity is tracked per-part, so a single continuous quotation with a
    # bracketed ellipsis in the middle gets parity-reset at that point --
    # lex-nx7u7's Tversky-Kahneman quote and lex-4ckek's Lukacs quote were both
    # reported unmatched for exactly this reason, on text confirmed verbatim
    # by hand. Bare "..." (no brackets) already round-trips fine through
    # pieces_matched()/check() below, so collapsing the bracketed form to the
    # bare one before the gloss-split removes the false split without
    # touching real glosses -- "[...paraphrasing:]" (lex-bx8hz's convention)
    # has non-dot content before its closing bracket and is untouched.
    q = re.sub(r"\[\s*\.{3,}\s*\]", "...", q)
    own_years = {k.rsplit("-", 1)[1] for k in author_year_keys(entry.get("text") or "")}
    # our gloss convention is [square brackets]; strip it, keep the rest.
    # The gloss can also HAND OFF attribution: lex-tb3re quotes Goodhart's real
    # sentence and then writes "[Strathern 1997, the canonical generalisation
    # paraphrase:] 'When a measure becomes a target, it ceases to be a good
    # measure.'" -- correctly, because Goodhart did not write that sentence.
    # Checking it against Goodhart reports the atom for getting the
    # attribution right. So a gloss naming a different year hands the spans
    # after it to a different source, and they are not this entry's claim.
    parts = re.split(r"(\[[^\]]*\])", q)
    foreign = False
    para = False
    for part in parts:
        if part.startswith("[") and part.endswith("]"):
            yrs = set(re.findall(r"\b(1[6-9]\d\d|20\d\d)\b", part))
            if yrs:
                foreign = bool(own_years) and not (yrs & own_years)
            # A gloss can also say outright that what follows is NOT verbatim.
            # 79 atoms use the convention, and checking a self-declared
            # paraphrase against the book reports the atom for being honest:
            # lex-bx8hz writes "[...paraphrasing:] ... Continuous reinforcement
            # extinguishes fast when reinforcement stops", which is Pryor's
            # finding in the atom's words and appears nowhere in her book.
            #
            # "paraphrasing <Name>:" is the opposite case and must NOT match --
            # there the SOURCE is paraphrasing someone else, so the words that
            # follow are still the source's own and still checkable. Kuhn
            # paraphrasing Planck is Kuhn verbatim.
            para = bool(re.search(r"paraphrase\b|paraphrasing\s*:", part, re.I))
            continue
        if foreign or para:
            continue
        # Captures ALTERNATE between inside-quotes and between-quotes. norm()
        # collapses every quote style to one marker so the pattern cannot tell
        # an opening quote from a closing one, and it happily returns the
        # connective prose between two quotations as though it were a third
        # quotation. lex-7ra5h reads
        #
        #   'Old Money desires...' — the apparent-accident is itself the
        #   costly-signal (only those who own working-button suits can perform
        #   this) — 'Elites and cults often use...'
        #
        # and the middle clause was checked against Marx's book as a verbatim
        # claim. Scanning left to right from a part that starts outside a
        # quote, match 0 is inside, match 1 is between, match 2 is inside.
        # Only trust that when the quote characters are balanced -- an unpaired
        # one (a possessive plural the apostrophe mask did not catch) flips the
        # parity for the rest of the part, and a wrong "this is not a claim" is
        # the expensive direction.
        # Parity has to come from the span's POSITION, not from its index in
        # the match list. The pattern only captures runs of 40 characters or
        # more, so a short quoted word is skipped -- and skipping one shifts
        # every index after it, which lands the connective prose on an even
        # index and reports the atom's own voice as a verbatim claim. Three
        # atoms did exactly this, each around a single scare-quoted word:
        # lex-cgqta 'feminine', lex-ftu47 'openness-to-evidence', lex-vx867
        # 'psychopathic'. Counting the quote characters that precede a span
        # cannot be thrown off by a run the pattern declined to capture.
        masked, before = _track_quote_nesting(part)
        for m in re.finditer(r"[\"'‘’“”]([^\"'‘’“”]{40,})", masked):
            kind = "quoted" if before[m.start(1)] else "between"
            out.append((m.group(1).replace("\x02", "'"), kind))
        if not re.search(f"[{QUOTE_CH}]", masked) and len(masked.strip()) >= 60:
            # Unquoted connective prose between glosses, which in this house
            # style is the ATOM's editorial voice, not a claim about what the
            # source says. lex-we98d writes "Key claim: the change-cognition
            # path of dissonance-reduction is not merely attitude-shift..."
            # after a bracketed translation, and checking that against Scheler
            # reports the atom for having an opinion. Kept, because genuinely
            # verbatim material does sometimes land here unquoted and that is
            # worth seeing, but tagged so a fabrication hunt is not reading
            # 833 paragraphs of commentary looking for one. Tested against
            # `masked`, not raw `part` -- a part containing only a possessive
            # apostrophe and no real quote mark must still qualify as
            # unquoted, and only the masked form has that apostrophe gone.
            out.append((masked.strip().replace("\x02", "'"), "unquoted"))
    cit = entry.get("citation") or ""
    # Same parity problem as the quote field, and it bit here too. This pattern
    # wants an opening and a closing quote with 40+ characters between them, so
    # when a short quotation fails the length test the engine simply retries
    # with its CLOSING quote as an opener and captures the prose after it.
    # lex-xj7s3 writes: Mill names the move "merely social intolerance" and
    # treats it as more pernicious than legal persecution... -- the quoted
    # phrase is 25 characters, too short to capture, and the atom's own next
    # clause was reported as an unmatched quotation of Mill. It is not; "Our
    # merely social intolerance kills no one" is verbatim in On Liberty.
    #
    # Double straight quotes only -- EXCEPT when there is no quote: field at
    # all. A 2026-08 audit found the gap: this house style's own dominant
    # convention is single quotes for a verbatim span inside citation:
    # (matching the quote: field), and this pattern could not see any of
    # them. 105 lineage entries had a 40+ char single-quoted run in
    # citation:, no quote: field, and nothing checking them at all --
    # lex-k6p4z (Klein) and lex-j2tgc (March-Simon) were the ones that
    # surfaced it; lex-j2tgc's citation carried a fabricated opening clause
    # that had been sitting unchecked because of exactly this.
    #
    # Widening the pattern to single quotes UNCONDITIONALLY was the first
    # attempt and made things worse: citation: is long, apostrophe-heavy
    # prose (bookkeeping notes, ISBNs, "author's", "isn't"), and PLURAL_POSS/
    # INWORD_APOS -- adequate for the shorter, more disciplined quote: field
    # -- do not mask enough of it. One unmasked apostrophe anywhere upstream
    # flips the parity for everything after it, and a run of quoted-kind
    # false positives followed: a full-corpus run went from 397 to 1231
    # "in quote marks" unmatched, and reading a sample showed most of the
    # increase was the atom's OWN bookkeeping ("Verified verbatim 2026-05-20
    # ...", "Refs/ PDF (isbn13 ...)", "pp.15-16 (defeasible-attach via
    # CQ-list)") misread as a verbatim claim. That is the opposite of the
    # point: this report is supposed to stay short enough that a "quoted"
    # hit means something.
    #
    # So the widened pattern applies ONLY to the entries that actually had
    # the gap -- no quote: field at all, meaning citation: is the ONLY place
    # a verbatim claim could be hiding. Every entry that already has a
    # quote: field keeps the original double-quote-only scan on citation:,
    # unchanged, exactly as proven safe before this audit.
    if (entry.get("quote") or "").strip():
        cit_balanced = cit.count('"') % 2 == 0
        for m in re.finditer(r'"([^"]{40,})"', cit):
            inside = cit[:m.start(1)].count('"') % 2 == 1
            out.append((m.group(1).replace("\x02", "'"),
                        "quoted" if (inside or not cit_balanced) else "between"))
    else:
        masked, before = _track_quote_nesting(cit)
        for m in re.finditer(r"[\"'‘’“”]([^\"'‘’“”]{40,})", masked):
            kind = "quoted" if before[m.start(1)] else "between"
            out.append((m.group(1).replace("\x02", "'"), kind))

    # Attribution pointers are not verbatim claims. Two shapes: a span that is
    # ONLY a pointer, which becomes its own kind, and a real quote with a
    # pointer glued to its end, where the pointer is stripped and the quote
    # stays checked. The second matters more than it looks -- lex-qcv5z quotes
    # Nagoski at length and then writes "(Nagoski, Come As You Are, Ch.2.)"
    # inside the same quote marks, so the whole passage failed on account of
    # nine words that were never hers.
    surnames = {k.rsplit("-", 1)[0].lower()
                for k in author_year_keys(entry.get("text") or "")}
    title_words = set(re.findall(r"[a-z]{4,}", (entry.get("citation") or "").lower()))
    final = []
    for sp, kind in out:
        if _is_pointer(sp, surnames, title_words):
            final.append((sp, "attribution"))
            continue
        cut = None
        for rx in (TRAILING_PAREN, TRAILING_OPEN):
            m = rx.search(sp)
            if m and _is_pointer(m.group(1), surnames, title_words):
                cut = m.start()
                break
        if cut is not None and len(sp[:cut].rstrip()) >= 40:
            final.append((sp[:cut].rstrip(), kind))
            continue
        final.append((sp, kind))
    return final if with_kind else [s for s, _ in final]


def pieces_matched(span, source_norm, source_flat):
    """How many of the span's ellipsis-separated pieces this source has.

    Only ever used to decide WHICH failure to show a reader when several
    candidate sources all fail. It is not a verdict and must never gate one:
    a source can hold most of a span's pieces and still be the wrong book,
    because the pieces a fabricated passage borrows are real.
    """
    n = 0
    for piece in re.split(r"\s*\.\.\.\s*|\s*\[\.\.\.\]\s*", norm(span)):
        piece = piece.strip()
        if len(piece) < 40:
            continue
        if piece in source_norm or despace(piece) in source_flat:
            n += 1
    return n


def check(span, source_norm, source_flat):
    """Match a claimed-verbatim span against the source.

    Three tiers, and the tier is part of the answer -- borrowed from
    publicrecord, which records exact|normalized|not_found on the record
    itself rather than leaving it in a report nobody re-reads.

      exact       the span is in the source as written
      whitespace  matches once runs of whitespace are collapsed
      despaced    matches once ALL spacing and punctuation are removed

    "despaced" forgives a bad scan and nothing else. It cannot tell "mim
    icry" from "mimicry" or a soft-hyphen line split from a whole word,
    which is the point; it also cannot be fooled about which letters are
    present or their order, which is the other point. Caillois's
    "Professionalization, application, and training" does not despace-match
    a cited "Professionalism, training, and dexterity" -- different letters.

    There is deliberately no fuzzy or windowed tier. An earlier version
    accepted a span when 85% of its six-word windows were found, to tolerate
    a running head spliced mid-sentence by the extractor. That is precisely
    the shape a substituted phrase has: most of the passage is real and one
    clause is not. A tier that forgives 15% of a quote forgives exactly the
    thing this script exists to catch. Injected running heads are a source-
    quality problem, and the fix is a cleaner extraction, not a looser test.
    """
    for piece in re.split(r"\s*\.\.\.\s*|\s*\[\.\.\.\]\s*", norm(span)):
        piece = piece.strip()
        if len(piece) < 40:
            continue
        if piece in source_norm:
            continue
        flat = despace(piece)
        if len(flat) >= 30 and flat in source_flat:
            continue
        return piece
    return None


_STOPT = {"the", "and", "of", "a", "an", "in", "on", "to", "for", "trans", "ed",
          "edition", "press", "university", "part", "book", "pdf", "txt", "epub"}


def _title_tokens(s):
    s = NONAL.sub(" ", CAMEL.sub(r"\1 \2", s).lower())
    return {t for t in s.split() if len(t) > 2 and not t.isdigit() and t not in _STOPT}


def rank_refs(lineage_text, cands):
    """Candidates ordered by title-token overlap, then by size."""
    want = _title_tokens(lineage_text)
    scored = []
    for c in cands:
        stem = os.path.splitext(os.path.basename(c))[0]
        try:
            size = os.path.getsize(c)
        except OSError:
            size = 0
        scored.append((len(want & _title_tokens(stem)), size, c))
    scored.sort(reverse=True)
    return [c for _, _, c in scored]


def pick_ref(lineage_text, cands):
    """Choose which candidate file a citation actually refers to.

    Candidates are matched on surname and year alone, so a multi-volume work
    yields several and they are all equally "correct" by that test. Taking the
    longest extraction -- which is what this did -- silently picks whichever
    volume happens to be bigger. James's Principles of Psychology is on disk in
    both volumes, and the passages this elements quotes ("Let us call the
    resting-places the substantive parts", "make our nervous system our ally
    instead of our enemy") are Volume 1, Chapters IX and IV. Both were checked
    against Volume 2 and reported as not found. Locke's Essay is on disk in
    both volumes too, and its four-sorts-of-arguments passage is Book IV, in
    Volume 2, checked against Volume 1.

    So: score on title-token overlap with the citation first, and fall back to
    length only among candidates that tie. Seven findings in this audit were
    the checker reading a different book than the citation named.
    """
    want = _title_tokens(lineage_text)
    best, bestkey = None, None
    for c in cands:
        stem = os.path.splitext(os.path.basename(c))[0]
        overlap = len(want & _title_tokens(stem))
        try:
            size = os.path.getsize(c)
        except OSError:
            size = 0
        key = (overlap, size)
        if bestkey is None or key > bestkey:
            best, bestkey = c, key
    return best


def resolve_usable(e, idx):
    """The sources on disk that one lineage entry's citation actually names.

    Returns (usable, why): usable is [(path, normalized, despaced)] in ranked
    order, and why is "" or the reason it came back empty -- "unresolved" when
    no file on disk answers the citation, "unverifiable" when files answer it
    but none carries readable text.

    This is a function because adjudicate-spans.py needs the SAME answer
    check_atom gets, and the copy it carried was three fixes behind: no
    platform filter, no citation-aware ranking, no widening. That copy is what
    answered lex-dwycr's Gang-of-Four Strategy quotes with the Falsifiability
    article and printed the words absent from it -- a wrong source, which
    always reads as a bad atom rather than as a bad checker. check_atom's own
    docstring already says a copy of these rules is how a verifier quietly
    stops verifying; the copy was sitting in the next file over.
    """
    slug = (e.get("text") or "").strip()
    status, pinned = pins().get(slug, (None, None))
    if status == "none":
        # The matcher had nothing, and nobody has looked since. Same verdict
        # it gave before pins existed, and it must keep the same name --
        # folding it into not-on-shelf would inflate the count of works
        # somebody has actually ruled out.
        return [], "unresolved"
    if status == "absent":
        # The one answer the bootstrap structurally cannot give. It admits a
        # candidate on a surname alone, so when the shelf simply does not
        # hold the cited work it returns whatever shares a name rather than
        # saying so -- and a wrong source reads as a bad atom, while a
        # missing one reads as a gap. Design Patterns and the Dohare-Sutton
        # plasticity paper are both genuinely not here.
        return [], "not-on-shelf"
    if status in ("pinned", "auto") and pinned:
        return usable_of([p for p in pinned if os.path.exists(p)])

    cands = []
    for k in author_year_keys(e.get("text") or ""):
        cands += idx.get(k, [])
    # The author-year key is no safer than the author-only one when the
    # "author" is a platform. Saving 24 Wikipedia articles as
    # Wikipedia-<Title>-2025-12.txt gave every one of them the author-year
    # key "wikipedia-2025", so lex-ew7zx's citation of
    # wikipedia-state-pattern-2025-12 -- an article nobody has saved --
    # resolved to the Falsifiability article and reported the Gang of Four
    # State pattern absent from Popper. Same rule, both paths.
    if any(t in PLATFORM for t in _title_tokens(e.get("text") or "")):
        cands = [c for c in cands
                 if author_only_admissible(e.get("text") or "", c)]
    if not cands:
        # No usable year in the citation -- antiquity, an undated reprint,
        # a complete-works volume. Fall back to the author alone rather
        # than declaring the source absent. Before this, a third of the
        # elements' quoted text was never checked against anything and
        # was counted as "no ref on disk" when the book was right there.
        for k in author_only_keys(e.get("text") or ""):
            cands += idx.get(k, [])
        # A shared platform name is not a resolution. Without this the
        # fallback answers "wikipedia-hypervigilance" with three xkcd
        # transcripts and reports the quotes missing from a comic.
        cands = [c for c in cands
                 if author_only_admissible(e.get("text") or "", c)]
    if not cands:
        return [], "unresolved"
    # Try every plausible candidate and keep whichever the quotes actually
    # match. Title overlap alone cannot resolve a citation that names no
    # volume -- lex-rgr3t cites "james-1890-principles-of-psychology" and
    # the passages are Volume 1, but nothing in the citation says so, so
    # ranking ties and the larger file wins. Evidence breaks the tie: the
    # volume the passages are in is the volume they match.
    # A span is a finding only if it fails against EVERY source the citation
    # names. Picking one winning candidate and reporting its misses is
    # wrong for any lineage that cites more than one work: lex-ukgke cites
    # both Doctorow posts, correctly, and its passages are split across
    # them, so whichever post won the comparison reported the other post's
    # passages as missing. Multi-volume works have the same shape.
    # Same author, different year, when the exact year finds nothing that
    # matches. Ranked by title overlap and capped, so it widens by one or
    # two books rather than by an author's whole shelf. See
    # build_ref_index for why the year is the wrong key for a collected
    # essay.
    wider = [c for c in dict.fromkeys(
                 sum((idx.get("author:" + k.rsplit("-", 1)[0], [])
                      for k in author_year_keys(e.get("text") or "")), []))
             if c not in cands]
    # Rank on the citation prose as well as the slug. The slug names the
    # WORK ("mauss-1925-essai-sur-le-don"); only the prose names the
    # TRANSLATOR, and for a work held in several translations the
    # translator is the whole question. Acquiring Halls made this visible:
    # it outranked Guyer and Cunnison for three atoms that cite Guyer and
    # Cunnison, and all three passages were verbatim in the translation
    # each atom actually named. Ranking on the slug alone picked the right
    # translation for one of the four Mauss atoms; ranking on slug plus
    # citation picks it for all four.
    #
    # The prose feeds ranking ONLY. author_only_admissible still sees the
    # slug, because a paragraph of citation mentions enough names to make
    # any file look admissible.
    rank_text = (e.get("text") or "") + " " + (e.get("citation") or "")
    ranked = rank_refs(rank_text, cands)[:4]
    if wider:
        ranked += rank_refs(rank_text, wider)[:2]
    return usable_of(ranked)


def usable_of(ranked):
    """Extract the ranked candidates and drop the ones with no readable text.

    Split out so a pinned entry gets the same treatment as a guessed one. The
    text-layer test has to stay on this side of the pin: a pin says which file
    is the right work, which is a different question from whether that copy
    can be read, and a scan with no text layer is still correctly pinned.
    """
    usable = []
    for cand in ranked:
        text = extract(cand)
        n = len(text.split())
        try:
            density = os.path.getsize(cand) / max(1, n)
        except OSError:
            density = 0
        # 120, not 300. The floor rejects junk extractions, and 300 also
        # rejected things that are legitimately short: a PubMed abstract
        # (Egan-Santos-Bloom 2007 is 230 words) and every xkcd transcript
        # under 300, which are the actual sources for the atoms citing them.
        # Junk is caught by density instead -- the York 404 page that stood in
        # for Lange was 95 words, still below this, and a nav-chrome page has
        # nothing like a prose byte-per-word ratio anyway.
        if n < 120 or density > 2000:
            continue
        csn = norm(text, True)
        usable.append((cand, csn, despace(csn)))
    if not usable:
        return [], "unverifiable"
    return usable, ""


def check_atom(d, idx):
    """Findings for one parsed atom: (ref, unmatched-piece) per failing span.

    Also returns the tallies main() prints, so the counting lives in exactly
    one place. It used to live inline in main(), which meant --staged could
    not reuse it without copying the "is this source usable" rules, and a
    copy of those rules is how a verifier quietly stops verifying.
    """
    findings, unresolved, unverifiable, checked, offshelf = [], 0, 0, 0, 0
    for e in d.get("lineage") or []:
        if not isinstance(e, dict) or e.get("source") not in (
                "primary", "practitioner", "cross-attestation"):
            continue
        # An attribution pointer is not a verbatim claim, so it is not checked
        # at all rather than checked and forgiven -- a forgiven check still
        # counts in the denominator and still has to be read past.
        spans = [(s, k) for s, k in spans_of(e, with_kind=True) if k != "attribution"]
        # Bare prose inside a quote: field IS checked -- a fabrication hides
        # just as well without quotation marks around it, and that rule stays.
        # But it only means anything when the field offers SOMETHING as
        # verbatim. An entry with no quoted run anywhere is the atom
        # paraphrasing its source, and a paraphrase cannot match the source by
        # construction, so checking it manufactures a finding every time. 98
        # entries and 105 spans were failing this way, including every
        # "cross-domain-...-attestation" note and lex-q9asc's own type-check
        # reasoning, which names no source because it is not quoting one.
        if not any(k == "quoted" for _, k in spans):
            continue
        if not spans:
            continue
        usable, why = resolve_usable(e, idx)
        if why == "not-on-shelf":
            # Distinct from "unresolved" on purpose. Unresolved means the
            # matcher found nothing and might simply have failed; not-on-shelf
            # means a person looked and the work is not here. Rolling them
            # together would hide the difference between a gap in the audit
            # and a gap in the library, and only one of those is actionable.
            offshelf += 1
            continue
        if why == "unresolved":
            unresolved += 1
            continue
        if why:
            unverifiable += 1
            continue
        checked += len(spans)
        for sp, kind in spans:
            worst = None
            best = None          # (pieces matched, candidate, failing piece)
            for cand, csn, csf in usable:
                bad = check(sp, csn, csf)
                if bad is None:
                    worst = None
                    best = None
                    break
                # Report the candidate that got FURTHEST, not the first tried.
                # The first-tried rule showed lex-z775k's failure against
                # Hayek's 1945 article, where the passage is absent outright,
                # while the 1948 collection the citation actually names -- and
                # which was checked, and in which the passage is verbatim --
                # missed only on a page break splicing a running head into
                # "ex-tended". A reader given the 1945 miss has every reason to
                # think the quote invented. Furthest-progress puts the nearest
                # miss in front of them, which is the one worth reading.
                score = pieces_matched(sp, csn, csf)
                if best is None or score > best[0]:
                    best = (score, cand, bad)
            if best is not None:
                _, cand, bad = best
                # Name every source that was tried, not just the one that got
                # closest. A report naming a single file reads as "this is the
                # book", and for an atom citing a translation other than the
                # best-ranked file that is actively misleading: lex-urh87 cites
                # Guyer 2016, the report said Cunnison, and the passage was
                # rewritten into Cunnison's words to match a book the atom
                # never claimed to be quoting. Both files were on disk and both
                # were checked; only one was shown.
                others = [os.path.basename(c) for c, _, _ in usable
                          if c != cand]
                label = os.path.basename(cand)
                if others:
                    label += f"  (also checked: {', '.join(others)})"
                worst = (label, bad, kind)
            if worst:
                findings.append(worst)
    return findings, unresolved, unverifiable, checked, offshelf


def staged_gate():
    """Ratchet for the pre-commit hook: block only on a REGRESSION.

    Blocking on any unmatched span would block almost every commit -- the
    elements carries hundreds of them and about half are scan artifacts, not
    citation defects -- and a gate that always fires is a gate everyone learns
    to pass with --no-verify. So compare each staged atom against its own HEAD
    version and block only when the count goes UP. That has no false positives
    by construction: whatever the scan does to a passage, it does to both
    versions.

    A newly added atom has no HEAD version to ratchet against, so its
    unmatched spans are printed rather than blocked. Printing is the point --
    every fabrication found so far was invisible because nothing ever put the
    cited text and the source text in front of anyone.
    """
    global ALLOW_OCR
    ALLOW_OCR = False
    out = subprocess.run(["git", "diff", "--cached", "--name-only"],
                         capture_output=True, text=True, cwd=ROOT).stdout
    files = [f for f in out.split("\n")
             if f.startswith("elements/") and f.endswith(".yaml")]
    if not files:
        return 0

    def at(rev, path):
        r = subprocess.run(["git", "show", f"{rev}:{path}"],
                           capture_output=True, text=True, cwd=ROOT)
        if r.returncode != 0:
            return None
        try:
            return load_atom_text(r.stdout)
        except Exception:
            return None

    idx = build_ref_index()
    regressed, fresh = [], []
    for f in files:
        new = at("", f)
        if not new:
            continue
        now, *_ = check_atom(new, idx)
        old = at("HEAD", f)
        if old is None:
            if now:
                fresh.append((f, now))
            continue
        was, *_ = check_atom(old, idx)
        if len(now) > len(was):
            regressed.append((f, len(was), len(now), now))

    for f, was, now, findings in regressed:
        print(f"verify-quotes BLOCKED: {f} unmatched spans {was} -> {now}")
        for ref, bad, kind in findings:
            print(f"    vs {ref} [{kind}]\n      {bad[:220]}")
    for f, findings in fresh:
        print(f"verify-quotes WARN: {f} is new and has "
              f"{len(findings)} unmatched span(s) -- read them before trusting them")
        for ref, bad, kind in findings:
            print(f"    vs {ref} [{kind}]\n      {bad[:220]}")
    return 1 if regressed else 0


def main():
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    calibrate = "--calibrate" in sys.argv
    if "--no-ocr" in sys.argv:
        # A full sweep auto-OCRs every PDF with no text layer, one at a time,
        # which turns a 20-minute measurement into a day and leaves the parent
        # process at zero CPU the whole while. With --no-ocr those books
        # report as unverifiable instead, which is the honest thing to say
        # about a book nobody can read yet, and the sweep finishes.
        globals()["OCR_ENABLED"] = False
    quoted_only = "--quoted-only" in sys.argv
    if "--staged" in sys.argv:
        sys.exit(staged_gate())
    if calibrate:
        args = list(CALIBRATION)
    idx = build_ref_index()
    paths = ([os.path.join(ROOT, "elements", f"{a}.yaml") for a in args]
             if args else sorted(glob.glob(os.path.join(ROOT, "elements", "*.yaml"))))

    findings, unresolved, unverifiable, checked, offshelf = [], 0, 0, 0, 0
    for p in paths:
        try:
            d = load_atom(p)
        except Exception as exc:
            print(f"  !! unparseable: {os.path.basename(p)}: {exc}", file=sys.stderr)
            continue
        if not d:
            continue
        f, u, v, c, o = check_atom(d, idx)
        unresolved += u
        unverifiable += v
        checked += c
        offshelf += o
        for ref, bad, kind in f:
            findings.append((d["id"], d.get("status"), ref, bad, kind))

    nq = sum(1 for x in findings if x[4] == "quoted")
    print(f"checked {checked} span(s); {len(findings)} unmatched "
          f"({nq} in quote marks, {len(findings) - nq} unquoted atom prose); "
          f"{unresolved} entr(ies) with no ref on disk; {unverifiable} with no text layer; "
          f"{offshelf} pinned not-on-shelf")
    for aid, status, ref, bad, kind in findings:
        if quoted_only and kind != "quoted":
            continue
        print(f"\n{aid} [{status}] vs {ref} [{kind}]\n  {bad[:200]}")

    if calibrate:
        got = {}
        for aid, *_ in findings:
            got[aid] = got.get(aid, 0) + 1
        print("\n--- pointer probes (an attribution must be skipped, a quote must not) ---")
        for text, sur, tw, want in POINTER_PROBES:
            saw = _is_pointer(text, sur, tw)
            flag = "ok" if saw == want else "FAIL"
            print(f"  {'pointer' if want else 'quote  '}  got={str(saw):<5} {flag}   {text[:58]}")
        print("\n--- substitution probes (a real span must match, a swapped one must not) ---")
        for ref, real, fake in SUBSTITUTION_PROBES:
            cands = idx.get(next(iter(author_year_keys(os.path.splitext(ref)[0])), ""), [])
            path = next((c for c in cands if os.path.basename(c) == ref), None)
            if not path:
                print(f"  {ref}: NOT ON DISK -- probe skipped")
                continue
            sn = norm(extract(path), True)
            sf = despace(sn)
            ok_real = check(real, sn, sf) is None
            ok_fake = check(fake, sn, sf) is not None
            verdict = "ok" if (ok_real and ok_fake) else "REGRESSION"
            print(f"  {os.path.basename(ref)[:44]:<44} real={'match' if ok_real else 'MISS'} "
                  f"swapped={'caught' if ok_fake else 'MISSED'}   {verdict}")

        print("\n--- nesting probes (a depth-2 nested quote must classify as its own kind) ---")
        for aid, substring, want_kind in NESTING_PROBES:
            path = os.path.join(ROOT, "elements", f"{aid}.yaml")
            try:
                d = load_atom(path)
            except Exception as exc:
                print(f"  {aid}: unparseable ({exc}) -- probe skipped")
                continue
            saw_kind = None
            for e in (d.get("lineage") or []):
                if not isinstance(e, dict):
                    continue
                for sp, kind in spans_of(e, with_kind=True):
                    if substring in sp:
                        saw_kind = kind
                        break
                if saw_kind is not None:
                    break
            verdict = "ok" if saw_kind == want_kind else "REGRESSION"
            print(f"  {aid}: expected kind={want_kind}, got kind={saw_kind}   {verdict}   {substring[:50]}")

        print("\n--- calibration ---")
        for aid, want in CALIBRATION.items():
            have = got.get(aid, 0)
            verdict = "unverifiable" if want is None else ("ok" if have == want else "REGRESSION")
            print(f"  {aid}: expected {want}, got {have}   {verdict}")


if __name__ == "__main__":
    main()

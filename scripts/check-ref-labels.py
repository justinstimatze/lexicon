#!/usr/bin/env python3
"""Compare what a FILE declares about itself against what its NAME claims.

A mislabelled source is worse than a missing one. The audit's whole procedure
is to read the citation against the source, so if the source is wearing the
wrong label the procedure produces a confident wrong answer -- and the wrong
answer is always "rewrite the atom", because the atom is the thing we can edit.

Found 2026-08-05: seven atoms cite Lucretius in William Ellery Leonard's verse
translation, and the only copy on disk was named
`Lucretius-c55BCE-DeRerumNatura-Munro.txt`. Munro's translation is prose.
The file's own Gutenberg header says "Translator: William Ellery Leonard" and
the string "Munro" does not appear in it anywhere. Had the filename been
believed, seven correct atoms would have been rewritten into the words of a
translation nobody had -- which is exactly how lex-urh87 and lex-fy5k5 were
damaged, from the other direction.

Filenames claim three things -- author, year, translator -- and any of them
can be wrong. The author field matters most, because the resolver keys on it:
Olson's LOGIC OF COLLECTIVE ACTION is on disk as
`Jr-1971-TheLogicOfCollectiveAction...epub`, the renamer having taken "Olson
Jr" and kept the wrong half, so every citation of Olson missed it entirely.

Project Gutenberg texts declare author and translator outright, so those are
checkable for free. Nothing here guesses: a file with no declaration is
reported as unknown, not as agreeing.

Usage:
  scripts/check-ref-labels.py            # every declaring file in refs/
  scripts/check-ref-labels.py --quiet    # disagreements only
"""
import argparse
import os
import re
import sys
import zipfile

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
REFS = os.path.join(ROOT, "refs")

# Gutenberg's header, and the couple of other shapes that turn up in refs/.
DECLARED = re.compile(
    r"^\s*(?:Translator|Translated by)\s*[:\-]\s*(.+)$", re.M | re.I)
AUTHOR = re.compile(r"^\s*Author\s*[:\-]\s*(.+)$", re.M | re.I)
TITLE = re.compile(r"^\s*Title\s*[:\-]\s*(.+)$", re.M | re.I)
VOL = re.compile(r"vol(?:ume)?[\s._-]*([0-9]+|[ivx]+)\b", re.I)

ROMAN = {"i": "1", "ii": "2", "iii": "3", "iv": "4", "v": "5",
         "vi": "6", "vii": "7", "viii": "8", "ix": "9", "x": "10"}


def volume(s):
    m = VOL.search(s or "")
    if not m:
        return None
    v = m.group(1).lower()
    v = ROMAN.get(v, v)
    # Strip zero padding: a file titled "Volume 09" and a filename saying
    # "Vol9" are the same volume, and the first version of this check reported
    # them as a disagreement.
    return str(int(v)) if v.isdigit() else v

NAMEY = re.compile(r"[A-Z][\w'’-]{2,}")


DC = {"title": re.compile(r"<dc:title[^>]*>([^<]+)</dc:title>", re.I),
      "creator": re.compile(r"<dc:creator[^>]*>([^<]+)</dc:creator>", re.I)}


def epub_meta(path):
    """dc:title and dc:creator out of an epub's OPF.

    Worth the zip read: 382 epubs sit in refs/ against 74 files carrying a
    Gutenberg-style header, so this is five times the ground the header check
    covers, and the mislabel rate in the checked corner has been about one in
    ten.
    """
    try:
        with zipfile.ZipFile(path) as z:
            opf = next((n for n in z.namelist() if n.lower().endswith(".opf")),
                       None)
            if not opf:
                return None, None
            x = z.read(opf).decode("utf-8", "replace")
    except Exception:
        return None, None
    out = {}
    for k, rx in DC.items():
        m = rx.search(x)
        out[k] = re.sub(r"\s+", " ", m.group(1)).strip() if m else None
    return out.get("title"), out.get("creator")


def declared(path, pattern, head=8000):
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            head_text = f.read(head)
    except OSError:
        return None
    m = pattern.search(head_text)
    if not m:
        return None
    return re.sub(r"\s+", " ", m.group(1)).strip(" .,;")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--quiet", action="store_true")
    a = ap.parse_args()

    rows = []
    authors_wrong = []
    volumes_wrong = []
    titles_wrong = []
    for dirpath, _, files in os.walk(REFS):
        for fn in files:
            low = fn.lower()
            if not low.endswith((".txt", ".html", ".md", ".epub")):
                continue
            p = os.path.join(dirpath, fn)
            if low.endswith(".epub"):
                # epubs carry the same two claims in their OPF. The translator
                # is not reliably recorded there, so epubs feed the title and
                # author checks only -- reporting nothing rather than guessing.
                e_title, e_creator = epub_meta(p)
                who, auth, title_override = None, e_creator, e_title
            else:
                who = declared(p, DECLARED)
                auth = declared(p, AUTHOR)
                title_override = None
            # The AUTHOR check is separate and stricter: a filename is
            # supposed to LEAD with the surname, so if none of the declared
            # author's names appears anywhere in the stem, the file is filed
            # under the wrong person and no citation will ever reach it.
            if auth:
                # Split hyphenated surnames too, and match on a five-letter
                # prefix. Without both, "Wells-Barnett" misses a file named
                # Wells- and "Dostoyevsky" misses one named Dostoevsky -- a
                # double-barrelled name and a transliteration, neither a
                # misfiling.
                a_names = [x for s in NAMEY.findall(auth) for x in s.split("-")
                           if x.lower() not in ("jr", "sr", "iii", "the", "and")]
                a_stem = os.path.splitext(fn)[0].lower()
                a_flat = re.sub(r"[^a-z]", "", a_stem)
                if a_names and not any(
                        re.sub(r"[^a-z]", "", s.lower())[:5] in a_flat
                        for s in a_names if len(s) >= 4):
                    authors_wrong.append((fn, auth))
            # A filename claiming the wrong VOLUME is the quiet version of
            # claiming the wrong book: both volumes are the same author, the
            # same year and nearly the same title, so every ranking heuristic
            # ties and the quotes get checked against the wrong half. Recorded
            # in this audit's notes as a past failure, never checked until now.
            title = title_override or (
                None if low.endswith(".epub") else declared(p, TITLE))
            if title:
                # Does the filename carry ANY substantial word of the declared
                # title? Filenames abbreviate hard -- Vasari's "Lives of the
                # Most Eminent Painters, Sculptors and Architects" is filed as
                # LivesOfThePaintersVol9 -- so requiring a long prefix match
                # flags honest abbreviations. One shared word of five letters
                # or more is enough to say the file is the book its name
                # claims, and enough to have caught the Mencius/Analects case
                # on its own: nothing in "The Analects of Confucius" appears in
                # a filename reading Mencius-c300BCE-Mencius-Legge.
                # Five letters is the floor for a word carrying identity, and
                # four common conventions defeat a naive comparison, all of
                # them found on the first run: a Latin title against an English
                # one (DeRerumNatura / "On the Nature of Things",
                # DeArchitectura / "The Ten Books on Architecture"), Wade-Giles
                # against pinyin (Zhuangzi / "Chuang Tzu"), a title whose every
                # distinctive word is four letters (Bergson's "Time and Free
                # Will"), and an xkcd transcript whose Title: line reads
                # "text:". None is a mislabelled file; the check reports and
                # a human reads.
                t_words = [w for w in re.findall(r"[a-z]{5,}", title.lower())
                           if w not in ("their", "these", "those", "which",
                                        "other", "being", "first")]
                s_flat = re.sub(r"[^a-z]", "", os.path.splitext(fn)[0].lower())
                if t_words and not any(w in s_flat for w in t_words):
                    titles_wrong.append((fn, title))
                fv, tv = volume(os.path.splitext(fn)[0]), volume(title)
                if fv and tv and fv != tv:
                    volumes_wrong.append((fn, title, fv, tv))
            if not who:
                continue
            surnames = NAMEY.findall(who)
            # Compare with all spacing and punctuation removed, the same trick
            # the quote matcher uses. Without it "LEstrange" fails to match
            # "L'Estrange" and "AnnaKarenina" fails to match "Anna Karenina" --
            # both were reported as mislabelled files when the only difference
            # was an apostrophe and a space.
            flat = lambda s: re.sub(r"[^a-z]", "", s.lower())
            stem = os.path.splitext(fn)[0].lower()
            agrees = any(flat(s) in flat(stem) for s in surnames)
            # It only DISAGREES if its trailing token is a NAME the file never
            # mentions. A first version asked only whether the tail looked
            # name-shaped, and called eight files wrong for ending in a title
            # word -- AnnaKarenina, Laughter, Confessions, Vol1. The test that
            # works: a translator credited in a filename should appear
            # somewhere in the text that credits translators. If the tail is
            # nowhere in the file AND the file names someone else, the
            # filename is making a claim the file contradicts.
            tail = stem.rsplit("-", 1)[-1]
            try:
                with open(p, encoding="utf-8", errors="replace") as f:
                    head_text = f.read(8000).lower()
                head_flat = re.sub(r"[^a-z]", "", head_text)
            except OSError:
                head_text = head_flat = ""
            # A surname has no digits in it. This drops "vol1" and the
            # Gutenberg id tails like "academicapg29247" without a stoplist.
            # ...and it is SHORT. A concatenated title tail like
            # "deathofivanilyich" is alphabetic and absent from the file (which
            # spells it "Ilyitch"), so length is what separates a surname from
            # a run-together title. Crude, and it errs toward silence.
            claims = (3 < len(tail) <= 12 and tail.isalpha()
                      and tail not in ("archiveorg", "complete", "works",
                                       "abridged", "revised", "gutenberg")
                      and flat(tail) not in head_flat)
            rows.append((agrees, claims, fn, who))

    bad = [r for r in rows if not r[0] and r[1]]
    silent = [r for r in rows if not r[0] and not r[1]]
    print(f"{len(rows)} file(s) declare a translator: "
          f"{len(rows)-len(bad)-len(silent)} agree with the filename, "
          f"{len(bad)} DISAGREE, {len(silent)} filename says nothing\n")
    if bad:
        print("DISAGREE -- filename claims one translator, file declares another:")
        for _, _, fn, who in bad:
            print(f"  file says: {who}")
            print(f"  name says: {fn}\n")
    if authors_wrong:
        print(f"\nAUTHOR MISFILED -- the file names an author the filename "
              f"does not carry ({len(authors_wrong)}). Read each: one of "
              f"these\nis an editorial choice rather than an error -- Olive "
              f"Gilbert wrote down\nSojourner Truth's dictated narrative, and "
              f"filing it under Truth is right:")
        for fn, auth in authors_wrong:
            print(f"  file says: {auth}")
            print(f"  name says: {fn}\n")
    if titles_wrong:
        print(f"\nTITLE MISMATCH -- the filename shares no substantial word "
              f"with the title the file declares ({len(titles_wrong)}):")
        for fn, title in titles_wrong:
            print(f"  file says: {title[:66]}")
            print(f"  name says: {fn}\n")
    if volumes_wrong:
        print(f"\nWRONG VOLUME -- filename and file disagree on which volume "
              f"this is ({len(volumes_wrong)}):")
        for fn, title, fv, tv in volumes_wrong:
            print(f"  file says: vol {tv}   ({title[:60]})")
            print(f"  name says: vol {fv}   {fn}\n")
    if not a.quiet and silent:
        print("filename silent on translator (not a defect, just unchecked):")
        for _, _, fn, who in silent[:25]:
            print(f"  {who[:38]:40} {fn[:60]}")
    return 1 if (bad or authors_wrong or volumes_wrong or titles_wrong) else 0


if __name__ == "__main__":
    sys.exit(main())

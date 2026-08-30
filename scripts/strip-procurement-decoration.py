#!/usr/bin/env python3
"""Remove procurement bookkeeping and local file paths from elements citations.

A citation should name the source. It should not say whether the source is on
this machine, when it arrived, or what the file is called: a reader who does
not share the filesystem cannot use any of that, and a reader who does can
read off which atoms are grounded in a book and which are standing on a
summary. The design principle already says the citation cites the source; this
is the sweep that was never run against it.

Five shapes, in the order they are applied. Each is narrow on purpose --
anything a rule does not recognise is REPORTED rather than guessed at, because
a wrong edit here silently rewrites a citation, which is the same defect class
the quote audit exists to remove.

  1. "Tier-3 deepening source - X not yet procured" and friends: the
     procurement clause goes, the substance stays.
  2. "Procured 2026-05-22: (Project Gutenberg #34901)" -> "(Project Gutenberg
     #34901)". The Gutenberg number is a real edition locator and is kept; the
     date this machine acquired it is not.
  3. "refs/-grounded" / "Tier-8(a) refs/-grounded" -> dropped.
  4. "(Fisher 1925, refs/)" -> "(Fisher 1925)". A directory is not a locator.
  5. "refs/Grimms' Fairy Tales.epub" -> the work, by hand (see BY_HAND).

Both sides are written together. The elements YAML and the mining-pass MD
that mirrors it must stay byte-identical or check-drift fails, and the MDs are
gitignored so git cannot restore them if this goes wrong -- hence --dry-run,
which is the default.

Usage:
  scripts/strip-procurement-decoration.py            # dry run, prints diffs
  scripts/strip-procurement-decoration.py --apply
"""
import argparse
import glob
import io
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Filename citations are too few and too individual to pattern-match; each is
# replaced with the work it names. Anything not listed here is reported.
BY_HAND = [
    ("Verbatim verified from refs/Grimms' Fairy Tales.epub 2026-05-21 (extracted via unzip + Python HTML strip)",
     "Grimm, *Kinder- und Hausmärchen*, no. 11"),
    ("refs/Grimms' Fairy Tales.epub", "Grimm, *Kinder- und Hausmärchen*"),
    ("refs/Bakhtin-Dostoevsky ", "Bakhtin, *Problems of Dostoevsky's Poetics* "),
    ("refs/Bakhtin-Dostoevsky", "Bakhtin, *Problems of Dostoevsky's Poetics*"),
    ("refs/James-1902", "James, *The Varieties of Religious Experience* (1902)"),
    ("refs/James-1890", "James, *The Principles of Psychology* (1890)"),
    ("refs/Darwin-1859", "Darwin, *On the Origin of Species* (1859)"),
    ("refs/Lorde-2011", "Lorde, *Sister Outsider*"),
    ("refs/Antifragile", "Taleb, *Antifragile*"),
    ("refs/Jinek-2012", "Jinek et al. 2012"),
    ("refs/Heuer", "Heuer, *Psychology of Intelligence Analysis*"),

    # Mining-pass narrative. These say when a file arrived on this machine and
    # which atoms had not yet cited it -- process notes about the corpus, not
    # claims about the source. Written out literally because the sentences are
    # individual and a sentence-scoped regex mangled them (see RULES).
    ("was already acquired to refs/ but had zero elements citations",
     "had zero elements citations"),
    ("had been acquired to refs/ but had zero elements citations",
     "had zero elements citations"),
    ("had been acquired to refs/ with zero atoms citing it",
     "had zero atoms citing it"),
    ("had been acquired to refs/ but the", "was present but the"),
    ("was already acquired to refs/ (24pp, this single chapter) but had zero substrat",
     "(24pp, this single chapter) had zero substrat"),
    ("(already acquired to refs/, previously unmined)", "(previously unmined)"),
    ("the edited volume was acquired to refs/", "the edited volume was present"),
    ("now that the text has been acquired to refs/", "now that the text is available"),
    ("Available via Basic Books print; queued for future refs/ procurement.",
     "Available via Basic Books print."),
    ("Not in our refs/ corpus — cited", "Not held — cited"),
    ("and Confucius Analects refs/.", "and Confucius Analects."),
    ("(refs/ also holds a readable", "(a readable"),
    ("same refs/ file as the already-mined", "same edition as the already-mined"),
    # Cross-project path: cupel is a separate repository of the author's, and
    # naming its filesystem layout tells a reader nothing they can use.
    ("read directly from cupel's path; the file is physically in cupel's refs/, but the elements citation is to the underlying",
     "the elements citation is to the underlying"),
    # AI-conversation trace as well as a path.
    ("User then supplied the PDF via refs/. ", ""),
    ("[1976 foreword text not in lexicon refs/ as of mining-pass",
     "[1976 foreword text not held as of mining-pass"),
]

RULES = [
    # --- 3. refs/-grounded, in its several dressings -------------------------
    (re.compile(r"\s*\(principle 8\(a\)\s*refs/-grounded\)"), ""),
    (re.compile(r"\s*\(8\(a\)\s*refs/-grounded\)"), ""),
    (re.compile(r"\s*Tier-8\(a\)\s*refs/-grounded\.?"), ""),
    (re.compile(r",?\s*refs/-grounded(\s+(?:by|in|with))"), r" grounded\1"),
    (re.compile(r"\s*refs/-grounded\.?"), ""),
    (re.compile(r"\s*per Principle 8\(a\)[^.]*refs-grounded[^.]*\."), "."),

    # --- 2. Procured <date>: --------------------------------------------------
    (re.compile(r"\bProcured \d{4}-\d{2}-\d{2}:\s*"), ""),
    (re.compile(r"\bAcquired to refs/ \(\d{4}-\d{2}-\d{2}\)[^.]*\.\s*"), ""),
    (re.compile(r"\bAcquired to refs/\s*\(?\d{0,4}-?\d{0,2}-?\d{0,2}\)?,?\s*"), ""),

    # --- 1. not-yet-procured clauses -----------------------------------------
    # "X not yet procured, but <substance>"  -> keep the substance
    (re.compile(r"\s*[—-]\s*[A-Z][\w.' ]{0,40}\d{4}\s+(?:paper|book)?\s*not yet procured,\s*but\s+"), " — "),
    (re.compile(r"\s*[—-]\s*not yet procured,\s*but\s+"), " — "),
    (re.compile(r"\bnot yet procured,\s*but\s+"), ""),
    # standalone parenthetical / trailing clause
    (re.compile(r"\s*\((?:deepening source )?not yet procured[^)]*\)"), ""),
    (re.compile(r"\s*[—-]\s*[A-Z][\w.' ]{0,40}\d{4}\s+(?:paper|book)?\s*not yet procured\b\.?"), ""),
    (re.compile(r"\s*[—-]\s*not yet procured(?: at mint time)?\b\.?"), ""),
    (re.compile(r",?\s*which is not yet procured at mint time"), ""),
    (re.compile(r",?\s*\bnot yet procured(?: at mint time)?\b\.?"), ""),

    # --- 4. bare refs/ used as a locator --------------------------------------
    # NOTE the trailing \b trap: "\brefs/\b" only matches when a WORD character
    # follows the slash, so "refs/ " with a space -- by far the commonest form
    # -- silently never matched. Every rule here anchors on the left only.
    (re.compile(r",\s*refs/\)"), ")"),
    (re.compile(r"\(\s*refs/\s*\)"), ""),
    (re.compile(r";\s*refs/ contains ([^)]*)\)"), r"; \1)"),

    # --- 5. mining-pass bookkeeping ------------------------------------------
    # Sentence-scoped clause removal was tried here and withdrawn. "[^.;]*"
    # anchors on the previous period, and this prose is full of "Rosch et al."
    # and "Ch.X", so the match started mid-abbreviation and ate the rest of the
    # sentence: lex-4dest came out as "Rosch et al.Distinctness-audited before
    # minting", and lex-sp2sn lost its clause while KEEPING the cupel path the
    # rule existed to remove. Those atoms are reported as residue and edited by
    # hand instead. A rule that mangles prose is worse than no rule, because
    # the dry run reports it as a success.
    (re.compile(r"\blexicon/refs/\s*"), ""),

    # --- locators that point into a local file rather than the work ----------
    (re.compile(r"\s*\(lines \d+-\d+ in the refs/ text\)"), ""),
    (re.compile(r"\s*\(extracted from refs/[^)]*\)"), ""),
    (re.compile(r"\s*\(?refs/ (?:has|holds) ([^)]*)\)"), r" (\1)"),
    (re.compile(r"\s*which is the edition refs/ holds"), ""),
    (re.compile(r"\s*is what refs/ holds"), ""),
    (re.compile(r"\s*cited from refs/\s*"), " "),
    (re.compile(r",?\s*refs/ copy"), ""),
    (re.compile(r"\s*refs/ PDF via"), " via"),
    (re.compile(r"\s*\(Doyle refs/\)"), ""),
    (re.compile(r"\s+refs/\s*(?=[—-])"), " "),
    (re.compile(r"\bNot in our refs/ corpus"), "Not in the corpus"),
    # A blanket "refs/ " -> "" was tried and withdrawn. It produced grammatical
    # wreckage that no longer contained the marker, so the dry run reported
    # zero residue and the damage was invisible: "already acquired to refs/ but
    # had zero citations" became "already acquired to but had zero citations",
    # "physically in cupel's refs/, but" became "in cupel's, but", and "supplied
    # the PDF via refs/." became "supplied the PDF via.". Removing a path from
    # a sentence is not the same as removing the sentence's claim about where
    # the file lives. What is left goes to residue and is edited by hand.

]

# Tidy runs ONLY on lines the rules above actually changed, and never on the
# file as a whole. Applied globally, "  +" -> " " collapses YAML indentation on
# every line of every atom, which a dry run reports as 149 files changed and
# looks exactly like success. Caught by reading a diff instead of a count.
TIDY = [
    (re.compile(r"[ \t]+([,.;:)])"), r"\1"),
    (re.compile(r"\(\s*[,;]\s*"), "("),
    (re.compile(r"\(\s*\)"), ""),
    (re.compile(r"[ \t]*[—-][ \t]*([.;)])"), r"\1"),
    (re.compile(r"(?<=\S)[ \t][ \t]+"), " "),
    (re.compile(r"[ \t]+\."), "."),
    (re.compile(r"\.[ \t]*\."), "."),
    (re.compile(r"[ \t]+$"), ""),
]

MARKER = re.compile(r"refs/|not yet procured|Procured \d{4}|Acquired to refs")


def clean(text):
    out = text
    for old, new in BY_HAND:
        out = out.replace(old, new)
    for rx, rep in RULES:
        out = rx.sub(rep, out)
    # tidy only the lines that moved
    before, after = text.split("\n"), out.split("\n")
    if len(before) == len(after):
        for i, (a, b) in enumerate(zip(before, after)):
            if a == b:
                continue
            for rx, rep in TIDY:
                b = rx.sub(rep, b)
            after[i] = b
        out = "\n".join(after)
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--apply", action="store_true")
    a = ap.parse_args()

    mds = {p: io.open(p, encoding="utf-8").read()
           for p in glob.glob(os.path.join(ROOT, "docs", "passes", "*.md"))}
    changed_atoms, residue, md_writes = 0, [], {}

    for p in sorted(glob.glob(os.path.join(ROOT, "elements", "lex-*.yaml"))):
        src = io.open(p, encoding="utf-8").read()
        if not MARKER.search(src):
            continue
        out = clean(src)
        aid = os.path.basename(p)[:-5]
        if out != src:
            changed_atoms += 1
        # Residue must be computed for UNCHANGED atoms too. Reporting only on
        # files the rules touched hides every atom no rule matched at all,
        # which is precisely the set that needs a human -- twelve of them went
        # silent this way when a rule was withdrawn.
        if MARKER.search(out):
            for m in re.finditer(
                    r"[^.;]{0,70}(?:refs/|not yet procured|Procured \d{4}|Acquired to refs)[^.;]{0,50}", out):
                residue.append((aid, m.group(0).strip()[:112]))
        if out == src:
            continue
        # mirror: replace the exact old atom text wherever it appears in an MD
        for mp, mtext in mds.items():
            if src.strip() and src.strip() in mtext:
                md_writes.setdefault(mp, mds[mp])
                mds[mp] = mds[mp].replace(src.strip(), out.strip())
        if a.apply:
            io.open(p, "w", encoding="utf-8").write(out)

    if a.apply:
        for mp in md_writes:
            io.open(mp, "w", encoding="utf-8").write(mds[mp])

    print(f"{changed_atoms} atom(s) changed; {len(md_writes)} mirror MD(s) touched")
    if residue:
        print(f"\n{len(residue)} fragment(s) NO RULE MATCHED -- hand these:")
        for aid, frag in residue:
            print(f"  {aid}  {frag}")
    if not a.apply:
        print("\n(dry run -- pass --apply to write)")


if __name__ == "__main__":
    main()

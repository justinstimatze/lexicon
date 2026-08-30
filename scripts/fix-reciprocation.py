#!/usr/bin/env python3
"""Clean up reciprocation gaps in elements `related` arrays.

Two passes:
  1. Strip dangling references — entries in `related` pointing to lex-IDs that
     don't exist in the elements (typically aspirational IDs that were planned
     during pre-V17 work but never minted).
  2. Add back-references — for every (A, B) edge where A lists B but B does
     NOT list A, append A to B's `related` array (alphabetically positioned
     to keep diffs minimal: appended to end).

After both passes, `scripts/check-reciprocation.py` should report OK.

Run from anywhere.
"""
import re
import sys
from pathlib import Path

ELEMENTS = Path(__file__).resolve().parent.parent / "elements"

def read_related(path):
    text = path.read_text()
    m = re.search(r"^(related:\s*\[)([^\]]*)(\])", text, re.MULTILINE)
    if not m:
        return None, text, None
    body = m.group(2).strip()
    ids = [tok.strip() for tok in body.split(",") if tok.strip()]
    return ids, text, m

def write_related(path, text, m, new_ids):
    new_body = ", ".join(new_ids)
    new_text = text[: m.start(2)] + new_body + text[m.end(2) :]
    path.write_text(new_text)

def main():
    paths = sorted(ELEMENTS.glob("lex-*.yaml"))
    atoms = {p.stem: p for p in paths}

    # Load current state.
    state = {}
    for stem, p in atoms.items():
        ids, text, match = read_related(p)
        state[stem] = {"ids": ids or [], "text": text, "match": match}

    # Pass 1: strip dangling refs.
    dangling_removed = 0
    for stem, s in state.items():
        if s["match"] is None:
            continue
        new_ids = [b for b in s["ids"] if b in atoms]
        removed = set(s["ids"]) - set(new_ids)
        if removed:
            print(f"  {stem}: dropped dangling refs {sorted(removed)}")
            s["ids"] = new_ids
            dangling_removed += len(removed)

    # Pass 2: add missing back-references.
    backrefs_added = 0
    for a, sa in state.items():
        for b in sa["ids"]:
            sb = state[b]
            if a not in sb["ids"]:
                sb["ids"].append(a)
                backrefs_added += 1

    # Write back all changed files.
    for stem, s in state.items():
        if s["match"] is None:
            continue
        write_related(atoms[stem], s["text"], s["match"], s["ids"])

    print()
    print(f"dangling refs removed: {dangling_removed}")
    print(f"back-references added: {backrefs_added}")

if __name__ == "__main__":
    main()

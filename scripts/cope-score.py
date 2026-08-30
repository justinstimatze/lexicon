#!/usr/bin/env python3
"""Score an atom's prose fields against cope-gate's `lexicon` card.

Usage: cope-score.py <yaml-file>
       git show <ref>:<path> | cope-score.py -

Extracts agent-instruction + canonical-instances + critical-questions (the
same field set the 2026-08-06 backlog sizing scored), pipes them to
`cope-gate -check - -card lexicon`, and prints the violation count as a
bare integer on stdout. Exits 1 with "parse-error" on stderr if the input
doesn't parse as an atom (missing agent-instruction) -- callers must not
treat a parse failure as a real zero.
"""
import subprocess
import sys

import yaml


def load(source):
    text = sys.stdin.read() if source == "-" else open(source).read()
    return yaml.safe_load(text)


def prose(doc):
    parts = [doc.get("agent-instruction", "") or ""]
    parts += [str(x) for x in (doc.get("canonical-instances") or [])]
    parts += [str(x) for x in (doc.get("critical-questions") or [])]
    return "\n\n".join(p for p in parts if p)


def main():
    if len(sys.argv) != 2:
        print("usage: cope-score.py <yaml-file|->", file=sys.stderr)
        sys.exit(2)

    doc = load(sys.argv[1])
    if not doc or "agent-instruction" not in doc:
        print("parse-error", file=sys.stderr)
        sys.exit(1)

    text = prose(doc)
    if not text.strip():
        print(0)
        return

    result = subprocess.run(
        ["cope-gate", "-check", "-", "-card", "lexicon"],
        input=text,
        capture_output=True,
        text=True,
    )
    out = result.stdout.strip()
    first_line = out.splitlines()[0] if out else ""
    if "violation(s)" in first_line:
        n = first_line.split(":", 1)[1].strip().split(" violation")[0].strip()
        print(int(n))
    else:
        print(0)


if __name__ == "__main__":
    main()

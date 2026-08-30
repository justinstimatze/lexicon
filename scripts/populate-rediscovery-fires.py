#!/usr/bin/env python3
"""Populate `fired:` maps in rediscovery-set.yaml by running each excerpt through `lexicon gate`.

The recovery metric needs binary firing labels per excerpt to measure how well the
elements surfaces the should_fire truth set. This script populates those labels from
the current gate's actual output. Hand-coded labels would be the gold standard;
gate-derived labels measure 'how well does the current gate recover what humans
identified as the right atoms', which is the operational question.

Run from anywhere; outputs to elements-recovery/rediscovery-set.yaml.

Usage:
    python3 scripts/populate-rediscovery-fires.py
"""
import re
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
RENDER = REPO / "render"
RDSET = REPO / "elements-recovery" / "rediscovery-set.yaml"

def gate_fires(text):
    """Run `lexicon gate --context <text>` and return list of lex-IDs that surfaced."""
    result = subprocess.run(
        ["go", "run", "./cmd/lexicon", "gate", "--context", text],
        cwd=RENDER,
        capture_output=True,
        text=True,
    )
    ids = []
    for line in result.stdout.splitlines():
        m = re.match(r"^(lex-\d+)\s+", line)
        if m:
            ids.append(m.group(1))
    return ids

def main():
    text = RDSET.read_text()
    lines = text.splitlines(keepends=True)

    excerpts = []  # (line_idx_of_text, excerpt_id, excerpt_text)
    i = 0
    cur_excerpt_id = None
    while i < len(lines):
        line = lines[i]
        m_id = re.match(r"^\s+-\s+id:\s+(\S+)\s*$", line)
        if m_id and m_id.group(1).startswith("rm-"):
            cur_excerpt_id = m_id.group(1)
        m_text = re.match(r'^\s+text:\s+"(.*)"\s*$', line)
        if m_text and cur_excerpt_id and cur_excerpt_id.count("-") >= 2:
            excerpts.append((i, cur_excerpt_id, m_text.group(1)))
        i += 1

    print(f"found {len(excerpts)} excerpts")

    new_lines = list(lines)
    delta = 0  # cumulative line-index shift from in-place edits
    for idx, (orig_line_idx, excerpt_id, excerpt_text) in enumerate(excerpts):
        print(f"  [{idx+1}/{len(excerpts)}] {excerpt_id}: running gate...")
        fired = gate_fires(excerpt_text)
        if not fired:
            print(f"    (no fires)")
            continue
        print(f"    fired: {fired}")

        target_line = orig_line_idx + delta
        indent = "        "
        block_lines = [f"{indent}fired:\n"]
        for fid in fired:
            block_lines.append(f"{indent}  {fid}: true\n")

        # Replace any existing fired: block, or insert if none.
        ins_at = target_line + 1
        # If the next non-blank line is `fired:` (possibly `fired: {}`),
        # delete it plus any continuation lines at the deeper indent.
        existing_start = None
        existing_end = None
        if ins_at < len(new_lines) and re.match(r"^\s+fired:", new_lines[ins_at]):
            existing_start = ins_at
            existing_end = ins_at + 1
            # Collect deeper-indented continuation lines (the lex-XXXX: true rows).
            while existing_end < len(new_lines) and re.match(r"^\s{10,}\S", new_lines[existing_end]):
                existing_end += 1
        if existing_start is not None:
            old_n = existing_end - existing_start
            new_lines[existing_start:existing_end] = block_lines
            delta += len(block_lines) - old_n
        else:
            new_lines[ins_at:ins_at] = block_lines
            delta += len(block_lines)

    RDSET.write_text("".join(new_lines))
    print(f"wrote {RDSET}")

if __name__ == "__main__":
    main()

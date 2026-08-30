#!/usr/bin/env python3
"""Graduate Cat A scaffolds-on entries: source: practitioner -> primary
for the lineage block whose `text:` matches the audit's primary label.

Patches BOTH the elements YAML and the source mining-pass MD atomically.
Input: stdin lines of `lex-NNNN|text-label`. Output: report per atom.
"""
import re
import sys
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SUB = ROOT / "elements"
PASSES = ROOT / "docs" / "passes"


def find_md(atom_id: str) -> Path | None:
    r = subprocess.run(
        ["rg", "-l", f"^id: {atom_id}$", str(PASSES)],
        capture_output=True, text=True,
    )
    paths = [p for p in r.stdout.strip().splitlines() if p]
    if not paths:
        return None
    if len(paths) > 1:
        sys.stderr.write(f"WARN {atom_id}: multiple MDs: {paths}\n")
    return Path(paths[0])


def patch_file(path: Path, text_label: str, atom_id: str | None = None) -> tuple[bool, str]:
    """Flip practitioner->primary for the lineage block keyed by text_label.
    If atom_id is given AND the file contains `^id: <atom_id>$` line, scope the
    edit to that atom's YAML block (until the next `^id: ` line or EOF).
    Returns (changed, reason)."""
    src = path.read_text()
    pat = re.compile(
        r"(  - source: )practitioner(\n    text: "
        + re.escape(text_label)
        + r"\n)"
    )

    if atom_id:
        m = re.search(rf"^id: {re.escape(atom_id)}$", src, re.MULTILINE)
        if m:
            start = m.start()
            # find next `^id: ` (any atom) after start
            nxt = re.search(r"^id: ", src[m.end():], re.MULTILINE)
            end = m.end() + nxt.start() if nxt else len(src)
            block = src[start:end]
            new_block, n = pat.subn(r"\1primary\2", block)
            if n == 0:
                return False, "no match in atom-block (already graduated or text mismatch)"
            if n > 1:
                return False, f"multiple matches ({n}) in atom-block — refusing"
            new = src[:start] + new_block + src[end:]
            path.write_text(new)
            return True, "ok (atom-scoped)"

    new, n = pat.subn(r"\1primary\2", src)
    if n == 0:
        return False, "no match (already graduated or text mismatch)"
    if n > 1:
        return False, f"multiple matches ({n}) — refusing ambiguous patch"
    path.write_text(new)
    return True, "ok"


def main():
    failed = []
    ok = []
    for line in sys.stdin:
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        atom_id, text_label = line.split("|", 1)
        yaml = SUB / f"{atom_id}.yaml"
        md = find_md(atom_id)
        if not yaml.exists():
            failed.append((atom_id, "yaml missing"))
            print(f"FAIL  {atom_id}  yaml missing")
            continue
        if md is None:
            failed.append((atom_id, "MD not found"))
            print(f"FAIL  {atom_id}  MD not found")
            continue
        y_ok, y_msg = patch_file(yaml, text_label, atom_id)
        m_ok, m_msg = patch_file(md, text_label, atom_id)
        if y_ok and m_ok:
            ok.append(atom_id)
            print(f"OK    {atom_id}  yaml + {md.name}")
        else:
            failed.append((atom_id, f"yaml={y_msg} md={m_msg}"))
            print(f"FAIL  {atom_id}  yaml={y_msg} md={m_msg}")
    print(f"\n--- {len(ok)} ok, {len(failed)} failed", file=sys.stderr)


if __name__ == "__main__":
    main()

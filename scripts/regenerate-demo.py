#!/usr/bin/env python3
"""Regenerate the code-block outputs in demo.md by re-running their commands.

Markers in demo.md bracket each block:
    <!-- BEGIN: <name> [NON-DETERMINISTIC] -->
    ```
    ... block content (prompt + output) ...
    ```
    <!-- END: <name> -->

Each <name> maps to a generator function below. The generator produces both
the displayed `$ ...` prompt-line(s) AND the actual command output, exactly
as they should appear inside the fences.

Most blocks are deterministic given the current elements state. Marked
NON-DETERMINISTIC blocks (only gate-rto currently) call the LLM-backed
gate and will produce slightly different output run-to-run regardless;
commit those diffs only when you mean to.

Note: tarot-seed-21 is deterministic only relative to the CURRENT atom set
— the same --seed picks different cards when the elements grows or
shrinks. Regenerating after a mining pass is expected to refresh the draw.

Usage:
    python3 scripts/regenerate-demo.py            # regenerate all
    python3 scripts/regenerate-demo.py --check    # exit 1 if any block changed
    python3 scripts/regenerate-demo.py <name>...  # regenerate specific blocks

Run from anywhere; paths are repo-rooted via the script's own location.
"""

import argparse
import re
import shutil
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
DEMO = REPO / "demo.md"
RENDER = REPO / "render"


def sh(cmd: str, cwd: Path | None = None) -> str:
    """Run a shell pipeline and return its output, rstripped.

    stderr is merged into stdout so emit-order is preserved — important for
    commands like `lexicon gate` that write to both streams (the lens/signal
    lines vs the ranking table).
    """
    r = subprocess.run(
        cmd, shell=True, cwd=cwd or REPO,
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
    )
    return r.stdout.rstrip("\n")


# --- generators -----------------------------------------------------------
# Each returns a string that goes inside the ``` fences (no leading/trailing
# fence — markers add those). Generators bake the displayed `$ ...` prompt
# AND the actual run together, so what you see is exactly what was executed.

def the_grid() -> str:
    """Type-signature distribution + the top signature-cells (the aspirational
    periodic-table organization). Replaces the old coordinate-system block.
    """
    parts = []
    for direction in ("type-in", "type-out"):
        cmd = (
            f'grep -h "^{direction}:" elements/*.yaml '
            f'| sort | uniq -c | sort -rn'
        )
        parts.append(f"$ {cmd}")
        parts.append(sh(cmd))
        parts.append("")
    # Top signature-cells: paste the two grep results and count pairs.
    pair_cmd = (
        'paste -d">" '
        '<(grep -h "^type-in:" elements/*.yaml | sed "s/type-in: //") '
        '<(grep -h "^type-out:" elements/*.yaml | sed "s/type-out: //") '
        '| sort | uniq -c | sort -rn | head -8'
    )
    parts.append("$ # densest signature-cells:")
    parts.append("$ paste -d'>' <(grep -h '^type-in:' ... | sed ...) "
                 "<(grep -h '^type-out:' ... | sed ...) | sort | uniq -c | sort -rn | head -8")
    parts.append(sh(f"bash -c {pair_cmd!r}"))
    return "\n".join(parts).rstrip()


def useful_angle_renders() -> str:
    """One compact render per card the gate typically deals on the burnout
    problem. Hard-coded card list; if the gate drift takes it elsewhere, the
    prose narration above will still match these (they're the canonical
    pattern-id + useful-angle + frame + similar-pattern set).
    """
    cards = ["lex-4rx6s", "lex-mnxhs", "lex-gfb9k", "lex-rfxtb", "lex-u5869"]
    parts = []
    parts.append(
        '$ for atom in ' + ' '.join(cards) + '; do\n'
        '    cd render && go run ./cmd/lexicon render $atom 2>&1 \\\n'
        '      | grep -E "^# |Type signature:|Example:" | head -3\n'
        '    echo\n'
        '  done'
    )
    parts.append("")
    for atom in cards:
        out = sh(
            f'go run ./cmd/lexicon render {atom} 2>&1 '
            f'| grep -E "^# |Type signature:|Example:" | head -3',
            cwd=RENDER,
        )
        parts.append(out)
        parts.append("")
    return "\n".join(parts).rstrip()


def lint_forcing_functions() -> str:
    """Lint warnings for bare-name atoms (forcing functions / gap markers)."""
    display = '$ cd render && go run ./cmd/lexicon lint 2>&1 | grep "unresolvable-atom"'
    out = sh('go run ./cmd/lexicon lint 2>&1 | grep "unresolvable-atom"', cwd=RENDER)
    return f"{display}\n\n{out}"


def gate_rto() -> str:
    # NON-DETERMINISTIC. The displayed command (with line-continuation backslashes)
    # is reproduced literally; the actual run inlines the context onto one line.
    display = (
        '$ cd render && go run ./cmd/lexicon gate \\\n'
        '    --context "My company is mandating return-to-office three days a week.\n'
        '               Leadership says it\'s for collaboration; the data on remote-vs-\n'
        '               hybrid productivity is mixed. I worry the mandate itself will\n'
        '               cost us more than it gains." \\\n'
        '    --top-k 5'
    )
    ctx = (
        "My company is mandating return-to-office three days a week. "
        "Leadership says it's for collaboration; the data on remote-vs-hybrid "
        "productivity is mixed. I worry the mandate itself will cost us more "
        "than it gains."
    )
    out = sh(f'go run ./cmd/lexicon gate --context {ctx!r} --top-k 5', cwd=RENDER)
    return f"{display}\n\n{out}"


def render_lex_0182() -> str:
    display = (
        "$ cd render && go run ./cmd/lexicon render lex-4rx6s --mode introspection \\\n"
        "    | head -25"
    )
    out = sh(
        "go run ./cmd/lexicon render lex-4rx6s --mode introspection | head -25",
        cwd=RENDER,
    )
    return f"{display}\n\n{out}"


def tarot_seed_21() -> str:
    display = "$ cd render && go run ./cmd/lexicon tarot --n 3 --seed 21"
    out = sh("go run ./cmd/lexicon tarot --n 3 --seed 21", cwd=RENDER)
    return f"{display}\n\n{out}"


GENERATORS = {
    "gate-rto": gate_rto,
    "useful-angle-renders": useful_angle_renders,
    "render-lex-4rx6s": render_lex_0182,
    "lint-forcing-functions": lint_forcing_functions,
    "the-grid": the_grid,
    "tarot-seed-21": tarot_seed_21,
}

# Names that produce non-deterministic output.
#   gate-rto: LLM-backed, varies per call.
#
# Note: tarot-seed-21 was non-deterministic until an elements-tool fix in
# cmd_tarot.go (sort candidates by ID + use a local seeded *rand.Rand
# instead of the deprecated package-level rand.Seed which has been a no-op
# since Go 1.20). Same --seed now produces identical draws — determinism is
# subject to the active atom set being stable, not the OS RNG state.
NON_DETERMINISTIC = {"gate-rto"}


# --- block surgery --------------------------------------------------------

# Captures: <!-- BEGIN: name [opt-flags] -->\n```\n...\n```\n<!-- END: name -->
BLOCK_RE = re.compile(
    r"<!-- BEGIN: (?P<name>[\w-]+)(?P<flags>[^>]*) -->\n"
    r"```\n(?P<body>.*?)\n```\n"
    r"<!-- END: (?P=name) -->",
    re.DOTALL,
)


def regenerate(only: list[str] | None = None, check: bool = False) -> int:
    text = DEMO.read_text()
    found_names: list[str] = []
    changed_names: list[str] = []
    unknown: list[str] = []

    def replace(m: re.Match) -> str:
        name = m.group("name")
        flags = m.group("flags").strip()
        found_names.append(name)
        if only and name not in only:
            return m.group(0)
        gen = GENERATORS.get(name)
        if gen is None:
            unknown.append(name)
            return m.group(0)
        new_body = gen()
        if new_body != m.group("body"):
            changed_names.append(name)
        marker_flags = f" {flags}" if flags else ""
        return (
            f"<!-- BEGIN: {name}{marker_flags} -->\n"
            f"```\n{new_body}\n```\n"
            f"<!-- END: {name} -->"
        )

    new_text = BLOCK_RE.sub(replace, text)

    if unknown:
        print(f"warning: no generator for: {', '.join(unknown)}", file=sys.stderr)

    requested = set(only) if only else set()
    if only:
        missing = requested - set(found_names)
        if missing:
            print(f"warning: no marker for: {', '.join(sorted(missing))}", file=sys.stderr)

    if check:
        if changed_names:
            print(f"demo.md is stale; would update: {', '.join(changed_names)}")
            return 1
        print("demo.md is fresh.")
        return 0

    if new_text == text:
        print(f"demo.md unchanged ({len(found_names)} blocks checked).")
        return 0

    # Atomic write via temp + rename.
    tmp = DEMO.with_suffix(".md.tmp")
    tmp.write_text(new_text)
    shutil.move(str(tmp), str(DEMO))
    print(f"demo.md updated; regenerated blocks: {', '.join(changed_names)}")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description="Regenerate demo.md output blocks.")
    ap.add_argument("names", nargs="*", help="Specific block names to regenerate (default: all).")
    ap.add_argument("--check", action="store_true",
                    help="Exit 1 if any block would change; do not write.")
    ap.add_argument("--list", action="store_true",
                    help="List known block names and exit.")
    args = ap.parse_args()
    if args.list:
        for name in GENERATORS:
            tag = " (NON-DETERMINISTIC)" if name in NON_DETERMINISTIC else ""
            print(f"  {name}{tag}")
        return 0
    return regenerate(only=args.names or None, check=args.check)


if __name__ == "__main__":
    sys.exit(main())

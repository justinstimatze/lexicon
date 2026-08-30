#!/usr/bin/env python3
"""Save Wikipedia articles from the local ZIM straight to refs/.

The audit found 37 spans presented as verbatim quotes whose source is a
Wikipedia article that was read at mining time and never saved (see
docs/audits/wikipedia-cited-but-unsaved.md). The whole English Wikipedia is on
this box at /opt/zim, so these were always recoverable; what was missing was a
route from the archive to a file that does not push 150 KB of raw HTML per
article through an agent's context.

`gozimhttpd` is that route. It serves the archive over HTTP and articles live
under the **C** namespace:

    /<archive-slug>/zim/C/<Article_Name>

That path is the whole trick, and it cost an embarrassing detour to find: ZIM
used namespace A for content in the old layout, current files use C, and every
guess at A (plus the bare path the server's own redirect points at) returns
404. The link list at /<archive-slug>/browse/ shows the real form.

Usage:
  scripts/fetch-zim-article.py "Peter principle"
  scripts/fetch-zim-article.py --from-file topics.txt
  scripts/fetch-zim-article.py --list-only --from-file topics.txt
"""
import argparse
import html as _html
import os
import re
import subprocess
import sys
import time
import urllib.parse
import urllib.request

GOZIM = os.path.expanduser("~/go/bin/gozimhttpd")
ZIMDIR = "/opt/zim"
ARCHIVE = "wikipedia-en-all-nopic-2025-12"
SNAPSHOT = "2025-12"
PORT = 18937
REFS = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "refs")


def serve():
    """Start our own gozimhttpd and wait for it to answer."""
    p = subprocess.Popen([GOZIM, "-dir", ZIMDIR, "-port", str(PORT)],
                         stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(60):
        try:
            urllib.request.urlopen(f"http://127.0.0.1:{PORT}/{ARCHIVE}/", timeout=2).read()
            return p
        except Exception:                                    # noqa: BLE001
            time.sleep(1)
    p.kill()
    raise RuntimeError("gozimhttpd never came up")


def get(title):
    """Fetch one article's HTML by title, or None."""
    path = urllib.parse.quote(title.replace(" ", "_"), safe="")
    url = f"http://127.0.0.1:{PORT}/{ARCHIVE}/zim/C/{path}"
    try:
        with urllib.request.urlopen(url, timeout=60) as r:
            return r.read().decode("utf-8", "replace")
    except Exception:                                        # noqa: BLE001
        return None


def strip(raw):
    """HTML to readable text, dropping chrome and citation scaffolding.

    Reference superscripts and edit links are removed rather than spaced out:
    left in, they wedge themselves into the middle of otherwise verbatim
    sentences, which is the exact defect this audit spends its time telling
    apart from fabrication.
    """
    raw = re.sub(r"(?is)<(script|style|head)[^>]*>.*?</\1>", " ", raw)
    raw = re.sub(r"(?is)<sup[^>]*class=\"reference\"[^>]*>.*?</sup>", "", raw)
    raw = re.sub(r"(?is)<span[^>]*class=\"mw-editsection\"[^>]*>.*?</span>", "", raw)
    raw = re.sub(r"(?is)<br\s*/?>|</(p|div|li|h[1-6]|blockquote|tr|td)>", "\n", raw)
    txt = _html.unescape(re.sub(r"<[^>]+>", " ", raw))
    txt = re.sub(r"[ \t\xa0]+", " ", txt)
    return re.sub(r"\n\s*\n\s*\n+", "\n\n", txt).strip()


def slug(title):
    s = re.sub(r"[^A-Za-z0-9]+", " ", title).title().replace(" ", "")
    return s or "Article"


def save(title, text):
    out = os.path.join(REFS, f"Wikipedia-{slug(title)}-{SNAPSHOT}.txt")
    header = (f"{title} — Wikipedia\n\n"
              f"Citation: Wikipedia article \"{title}\", retrieved from\n"
              f"{ARCHIVE}.zim, a local offline snapshot dated {SNAPSHOT}.\n\n"
              + "-" * 70 + "\n\n")
    with open(out, "w", encoding="utf-8") as f:
        f.write(header + text + "\n")
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("topics", nargs="*")
    ap.add_argument("--from-file")
    ap.add_argument("--list-only", action="store_true",
                    help="report what would be fetched, write nothing")
    a = ap.parse_args()

    topics = list(a.topics)
    if a.from_file:
        topics += [l.strip() for l in open(a.from_file)
                   if l.strip() and not l.startswith("#")]
    if not topics:
        ap.error("give a topic or --from-file")
    if not os.path.exists(GOZIM):
        sys.exit(f"gozimhttpd not found at {GOZIM}")

    srv = serve()
    ok = miss = 0
    try:
        for t in topics:
            raw = get(t)
            if raw is None:
                print(f"  MISS     {t}")
                miss += 1
                continue
            text = strip(raw)
            if a.list_only:
                print(f"  {len(text.split()):>7,}w  {t}")
            else:
                print(f"  {len(text.split()):>7,}w  {os.path.basename(save(t, text))}")
            ok += 1
    finally:
        srv.kill()
    print(f"\n{ok} fetched, {miss} missing")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())

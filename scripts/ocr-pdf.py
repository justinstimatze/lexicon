#!/usr/bin/env python3
"""OCR a scanned PDF to text via pdftoppm + tesseract.

Usage:
    scripts/ocr-pdf.py <pdf-path> [--pages N-M] [--output PATH] [--dpi 300]

Renders one page at a time into a disk-backed tempdir next to the PDF (never
system /tmp, which is a RAM-backed tmpfs on this host — see the global
CLAUDE.md's tmpfs section), OCRs it, deletes the PNG, and moves to the next
page. No more than one rendered page exists on disk or in memory at once.
Emits a single text file with `\n\n--- page N ---\n\n` separators.
"""
import argparse
import subprocess
import sys
import tempfile
from pathlib import Path


def parse_pages(spec: str, total: int) -> tuple[int, int]:
    if "-" in spec:
        a, b = spec.split("-", 1)
        return int(a), int(b)
    n = int(spec)
    return n, n


def pdf_page_count(pdf: Path) -> int:
    out = subprocess.run(
        ["pdfinfo", str(pdf)], check=True, capture_output=True, text=True
    ).stdout
    for line in out.splitlines():
        if line.startswith("Pages:"):
            return int(line.split(":", 1)[1].strip())
    sys.exit(f"could not parse page count from pdfinfo for {pdf}")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("pdf", type=Path)
    ap.add_argument("--pages", default=None, help="e.g. 10-25 or 12 (1-indexed); default=all")
    ap.add_argument("--output", type=Path, default=None, help="output txt path; default=<pdf-stem>.ocr.txt")
    ap.add_argument("--dpi", type=int, default=300)
    args = ap.parse_args()

    if not args.pdf.is_file():
        sys.exit(f"not a file: {args.pdf}")

    total = pdf_page_count(args.pdf)
    if args.pages:
        first, last = parse_pages(args.pages, total)
    else:
        first, last = 1, total
    if not (1 <= first <= last <= total):
        sys.exit(f"page range {first}-{last} out of bounds (1-{total})")

    output = args.output or args.pdf.with_suffix("").with_name(args.pdf.stem + ".ocr.txt")

    # Disk-backed, next to the PDF -- never system /tmp (tmpfs=RAM on this host).
    tmp_base = args.pdf.resolve().parent / ".ocr-tmp"
    tmp_base.mkdir(exist_ok=True)

    with tempfile.TemporaryDirectory(prefix="ocr-pdf-", dir=tmp_base) as tmp:
        tmpdir = Path(tmp)
        prefix = tmpdir / "page"

        with output.open("w") as outfh:
            for page_num in range(first, last + 1):
                print(f"  rasterizing + OCR page {page_num}/{last} @ {args.dpi}dpi …", file=sys.stderr)
                subprocess.run(
                    [
                        "pdftoppm",
                        "-r", str(args.dpi),
                        "-f", str(page_num),
                        "-l", str(page_num),
                        "-png",
                        str(args.pdf),
                        str(prefix),
                    ],
                    check=True,
                )
                pngs = sorted(tmpdir.glob("page-*.png"))
                if not pngs:
                    sys.exit(f"pdftoppm produced no image for page {page_num}")
                png = pngs[0]
                txt = subprocess.run(
                    ["tesseract", str(png), "-", "--psm", "6"],
                    check=True,
                    capture_output=True,
                    text=True,
                ).stdout
                outfh.write(f"\n\n--- page {page_num} ---\n\n")
                outfh.write(txt)
                png.unlink()

    try:
        tmp_base.rmdir()
    except OSError:
        pass  # leftover siblings from a concurrent run; not this run's to remove

    print(f"wrote {output}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())

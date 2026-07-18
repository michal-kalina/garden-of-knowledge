"""Format-specific parsers producing the common block representation.

Design (ADR-0004): lightweight, deterministic parsing first. PyMuPDF gives us
text with font metrics, so headings are detected by font size relative to the
document's body text. Docling remains the documented upgrade path for scanned
documents (OCR) and complex table extraction — behind the same block contract,
so swapping it in later touches nothing outside this module.
"""

from dataclasses import dataclass
from statistics import median

import pymupdf


@dataclass
class Block:
    """Mirror of the API's ParsedBlock, kept dependency-free for testing."""

    type: str  # "heading" | "paragraph" | "table"
    text: str
    page: int | None = None


# --- Markdown ---------------------------------------------------------------


def parse_markdown(text: str) -> list[Block]:
    """Parse Markdown into blocks.

    Recognizes ATX headings (#, ##, …), pipe tables (consecutive lines
    starting with '|') and paragraphs separated by blank lines. Fenced code
    blocks are kept whole as paragraph blocks so a '#' inside code is never
    mistaken for a heading.
    """
    blocks: list[Block] = []
    para: list[str] = []
    in_fence = False

    def flush_para() -> None:
        if para:
            joined = "\n".join(para).strip()
            if joined:
                blocks.append(Block(type="paragraph", text=joined))
            para.clear()

    lines = text.splitlines()
    i = 0
    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        if stripped.startswith("```"):
            if not in_fence:
                flush_para()
                in_fence = True
                para.append(line)
            else:
                para.append(line)
                flush_para()
                in_fence = False
            i += 1
            continue

        if in_fence:
            para.append(line)
            i += 1
            continue

        if not stripped:
            flush_para()
            i += 1
            continue

        if stripped.startswith("#"):
            flush_para()
            heading_text = stripped.lstrip("#").strip()
            if heading_text:
                blocks.append(Block(type="heading", text=heading_text))
            i += 1
            continue

        if stripped.startswith("|"):
            flush_para()
            table_lines = []
            while i < len(lines) and lines[i].strip().startswith("|"):
                table_lines.append(lines[i].strip())
                i += 1
            blocks.append(Block(type="table", text="\n".join(table_lines)))
            continue

        para.append(stripped)
        i += 1

    flush_para()
    return blocks


# --- PDF --------------------------------------------------------------------

# A text block is considered a heading when its largest font clearly exceeds
# the document's body size AND it is short and single-line. Both conditions
# matter: large-font multi-line text is usually a callout, not a heading.
HEADING_SIZE_RATIO = 1.2
HEADING_MAX_CHARS = 120


def parse_pdf(data: bytes) -> list[Block]:
    """Extract text blocks from a PDF, classifying headings by font size.

    The body font size is the character-weighted median of all span sizes —
    robust against documents where headings or footnotes are frequent.
    Documents with a uniform font size simply yield paragraphs only.

    Table extraction is intentionally out of scope here (TODO: docling
    upgrade, ADR-0004); pipe tables in Markdown sources already exercise the
    chunker's table handling.
    """
    doc = pymupdf.open(stream=data, filetype="pdf")
    try:
        raw_blocks: list[tuple[int, str, float]] = []  # (page, text, max_span_size)
        span_sizes: list[tuple[float, int]] = []  # (size, char_count)

        for page_no, page in enumerate(doc, start=1):
            for block in page.get_text("dict")["blocks"]:
                if block.get("type") != 0:  # 0 = text; images are skipped
                    continue
                line_texts: list[str] = []
                max_size = 0.0
                for line in block.get("lines", []):
                    span_texts = []
                    for span in line.get("spans", []):
                        t = span.get("text", "")
                        if not t.strip():
                            continue
                        span_texts.append(t)
                        span_sizes.append((span["size"], len(t)))
                        max_size = max(max_size, span["size"])
                    if span_texts:
                        line_texts.append(" ".join(span_texts))
                text = "\n".join(line_texts).strip()
                if text:
                    raw_blocks.append((page_no, text, max_size))

        body_size = _weighted_median(span_sizes)
        threshold = body_size * HEADING_SIZE_RATIO

        blocks: list[Block] = []
        for page_no, text, max_size in raw_blocks:
            is_heading = (
                body_size > 0
                and max_size >= threshold
                and len(text) <= HEADING_MAX_CHARS
                and "\n" not in text
            )
            blocks.append(
                Block(
                    type="heading" if is_heading else "paragraph",
                    text=text,
                    page=page_no,
                )
            )
        return blocks
    finally:
        doc.close()


def _weighted_median(sizes: list[tuple[float, int]]) -> float:
    """Median font size weighted by character count."""
    if not sizes:
        return 0.0
    expanded: list[float] = []
    for size, chars in sizes:
        # Cap the expansion so pathological documents stay cheap.
        expanded.extend([size] * min(chars, 500))
    return float(median(expanded))

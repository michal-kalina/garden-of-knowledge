"""Unit tests for format parsers.

The PDF fixture is generated programmatically with PyMuPDF — no binary test
files in the repository, and the test controls font sizes precisely, which is
exactly what the heading heuristic keys on.
"""

import pymupdf

from app.parsers import parse_markdown, parse_pdf


class TestParseMarkdown:
    def test_headings_paragraphs_and_tables(self):
        md = (
            "# Contract\n"
            "\n"
            "First paragraph line one\ncontinues here.\n"
            "\n"
            "## Payments\n"
            "\n"
            "| item | price |\n"
            "| ---- | ----- |\n"
            "| a    | 1     |\n"
            "\n"
            "Closing remark.\n"
        )
        blocks = parse_markdown(md)
        types = [b.type for b in blocks]
        assert types == ["heading", "paragraph", "heading", "table", "paragraph"]
        assert blocks[0].text == "Contract"
        assert blocks[1].text == "First paragraph line one\ncontinues here."
        assert blocks[3].text.count("\n") == 2  # three table lines kept together

    def test_hash_inside_code_fence_is_not_a_heading(self):
        md = "Intro.\n\n```bash\n# just a comment\necho hi\n```\n\nAfter."
        blocks = parse_markdown(md)
        assert [b.type for b in blocks] == ["paragraph", "paragraph", "paragraph"]
        assert "# just a comment" in blocks[1].text

    def test_plain_text_yields_paragraphs_only(self):
        blocks = parse_markdown("Just some text.\n\nSecond paragraph.")
        assert [b.type for b in blocks] == ["paragraph", "paragraph"]

    def test_empty_input(self):
        assert parse_markdown("") == []
        assert parse_markdown("\n\n  \n") == []


def _make_pdf() -> bytes:
    doc = pymupdf.open()
    page = doc.new_page()
    page.insert_text((72, 80), "Contract Terms", fontsize=18)
    page.insert_text(
        (72, 120),
        "This agreement is made between the undersigned parties.",
        fontsize=11,
    )
    page.insert_text(
        (72, 140),
        "It remains in force for a period of twelve months.",
        fontsize=11,
    )
    page2 = doc.new_page()
    page2.insert_text((72, 80), "Payment is due within thirty days.", fontsize=11)
    data = doc.tobytes()
    doc.close()
    return data


class TestParsePdf:
    def test_detects_headings_by_font_size(self):
        blocks = parse_pdf(_make_pdf())
        headings = [b for b in blocks if b.type == "heading"]
        assert len(headings) == 1
        assert headings[0].text == "Contract Terms"
        assert headings[0].page == 1

    def test_tracks_page_numbers(self):
        blocks = parse_pdf(_make_pdf())
        page2_blocks = [b for b in blocks if b.page == 2]
        assert len(page2_blocks) == 1
        assert "thirty days" in page2_blocks[0].text
        assert page2_blocks[0].type == "paragraph"

    def test_uniform_font_size_yields_no_headings(self):
        doc = pymupdf.open()
        page = doc.new_page()
        page.insert_text((72, 80), "All the same size here.", fontsize=11)
        page.insert_text((72, 100), "And here as well, nothing stands out.", fontsize=11)
        data = doc.tobytes()
        doc.close()

        blocks = parse_pdf(data)
        assert blocks
        assert all(b.type == "paragraph" for b in blocks)

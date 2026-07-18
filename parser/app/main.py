"""Document parsing service.

Python exists in this project only where it has a real edge: the document
parsing ecosystem. Phase 1 step 3: real Markdown and PDF parsing behind the
same block contract established in Phase 0 (see parsers.py and ADR-0004).
"""

from fastapi import FastAPI, HTTPException, UploadFile
from pydantic import BaseModel

from app.parsers import parse_markdown, parse_pdf

app = FastAPI(title="GoK Parser", version="0.2.0")


class ParsedBlock(BaseModel):
    """A single content block extracted from a document."""

    type: str  # "heading" | "paragraph" | "table"
    text: str
    page: int | None = None


class ParseResponse(BaseModel):
    filename: str
    content_type: str
    blocks: list[ParsedBlock]


MARKDOWN_TYPES = {"text/markdown", "text/x-markdown", "text/plain"}


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/parse", response_model=ParseResponse)
async def parse(file: UploadFile) -> ParseResponse:
    """Accept a file and return its content as a list of typed blocks."""
    content_type = file.content_type or "application/octet-stream"
    raw = await file.read()

    if content_type == "application/pdf":
        try:
            blocks = parse_pdf(raw)
        except Exception as exc:  # pymupdf raises various types for broken files
            raise HTTPException(
                status_code=422, detail=f"failed to parse PDF: {exc}"
            ) from exc
    elif content_type in MARKDOWN_TYPES:
        blocks = parse_markdown(raw.decode("utf-8", errors="replace"))
    else:
        raise HTTPException(
            status_code=415, detail=f"unsupported content type: {content_type}"
        )

    return ParseResponse(
        filename=file.filename or "unknown",
        content_type=content_type,
        blocks=[ParsedBlock(type=b.type, text=b.text, page=b.page) for b in blocks],
    )

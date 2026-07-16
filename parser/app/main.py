"""Service parsing documents.

Python exists in this project only where it has a real advantage:
document parsing ecosystem (docling, unstructured, OCR).
Phase 0: health-check and stub /parse defining the API contract.
Phase 1: proper PDF and Markdown parsing.
"""

from fastapi import FastAPI, UploadFile
from pydantic import BaseModel

app = FastAPI(title="GoK Parser", version="0.1.0")


class ParsedBlock(BaseModel):
    """A single block of content extracted from a document (paragraph, heading, table cell)."""

    type: str  # e.g., "paragraph", "heading", "table"
    text: str
    page: int | None = None


class ParseResponse(BaseModel):
    filename: str
    content_type: str
    blocks: list[ParsedBlock]


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/parse", response_model=ParseResponse)
async def parse(file: UploadFile) -> ParseResponse:
    """Parses an uploaded file and returns a list of content blocks.

    Phase 0: stub — returns the content as a single block for text files,
    to establish the API contract between the worker (Go) and the parser (Python).
    """
    raw = await file.read()
    text = raw.decode("utf-8", errors="replace")
    return ParseResponse(
        filename=file.filename or "unknown",
        content_type=file.content_type or "application/octet-stream",
        blocks=[ParsedBlock(type="paragraph", text=text, page=None)],
    )

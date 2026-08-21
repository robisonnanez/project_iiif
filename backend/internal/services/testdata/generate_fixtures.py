"""Regenerate the small PDF fixtures used by the outline integration tests."""

from io import BytesIO
from pathlib import Path

from pypdf import PdfReader, PdfWriter
from reportlab.lib.pagesizes import A5
from reportlab.pdfgen.canvas import Canvas


OUTPUT_DIR = Path(__file__).resolve().parent


def base_pdf(page_labels: list[str]) -> bytes:
    buffer = BytesIO()
    canvas = Canvas(buffer, pagesize=A5)
    width, height = A5
    for page_number, label in enumerate(page_labels, start=1):
        canvas.setFont("Helvetica-Bold", 18)
        canvas.drawString(54, height - 72, label)
        canvas.setFont("Helvetica", 11)
        canvas.drawString(54, height - 100, f"Fixture page {page_number}")
        canvas.showPage()
    canvas.save()
    return buffer.getvalue()


def write_without_outline() -> None:
    (OUTPUT_DIR / "without_toc.pdf").write_bytes(
        base_pdf(["First page", "Second page"])
    )


def write_with_outline() -> None:
    reader = PdfReader(
        BytesIO(
            base_pdf(
                [
                    "Chapter 1",
                    "Introduction",
                    "Background",
                    "Methods",
                    "Chapter 2",
                    "Results",
                ]
            )
        )
    )
    writer = PdfWriter()
    writer.clone_document_from_reader(reader)
    chapter_one = writer.add_outline_item("Chapter 1", 0)
    writer.add_outline_item("Introduction", 1, parent=chapter_one)
    writer.add_outline_item("Background", 2, parent=chapter_one)
    writer.add_outline_item("Methods", 3, parent=chapter_one)
    chapter_two = writer.add_outline_item("Chapter 2", 4)
    writer.add_outline_item("Results", 5, parent=chapter_two)
    with (OUTPUT_DIR / "hierarchical_toc.pdf").open("wb") as stream:
        writer.write(stream)


if __name__ == "__main__":
    write_without_outline()
    write_with_outline()

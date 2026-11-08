"""Add the original publication framing to a one-page Graphviz PDF."""

import argparse
from datetime import date
from io import BytesIO
import os
from pathlib import Path
from tempfile import NamedTemporaryFile

from pypdf import PdfReader, PdfWriter
from reportlab.pdfgen import canvas
from reportlab.pdfbase.pdfmetrics import stringWidth


def frame_graph(source, destination, updated, subtitle):
    if source.resolve() == destination.resolve():
        raise ValueError("Use a separate output file to preserve the raw graph")
    reader = PdfReader(source)
    if len(reader.pages) != 1:
        raise ValueError("Expected a single-page Graphviz PDF")
    graph = reader.pages[0]
    if graph.rotation or graph.mediabox.lower_left != (0, 0) or graph.cropbox != graph.mediabox:
        raise ValueError("Expected an unrotated, uncropped Graphviz page")
    width, height = float(graph.mediabox.width), float(graph.mediabox.height)
    page_width, page_height = width * 1.04, height * 1.10
    overlay = BytesIO()
    drawing = canvas.Canvas(overlay, pagesize=(page_width, page_height))
    drawing.setLineWidth(max(1, int(min(width, height) * 0.001)))
    drawing.rect(width * 0.02, height * 0.02, width, height, stroke=1, fill=0)
    drawing.setFont("Times-Roman", height * 0.03)
    drawing.drawCentredString(page_width / 2, page_height - height * 0.032, "The Myst Graph")
    drawing.setFont("Times-Roman", height * 0.02)
    drawing.drawCentredString(page_width / 2, page_height - height * 0.062, subtitle)
    signature = f"Updated ({updated:%B %d, %Y}) | glthr.com"
    size, right, baseline = height * 0.01, width * 1.02, height * 0.01
    drawing.setFont("Times-Roman", size)
    drawing.drawRightString(right, baseline, signature)
    link_width, padding = stringWidth("glthr.com", "Times-Roman", size), size * 0.2
    drawing.linkURL("https://www.glthr.com", (right - link_width - padding,
                    baseline - padding, right + padding, baseline + size + padding), thickness=0)
    drawing.save()
    overlay.seek(0)
    writer = PdfWriter()
    framed = writer.add_blank_page(width=page_width, height=page_height)
    framed.merge_translated_page(graph, width * 0.02, height * 0.02)
    framed.merge_page(PdfReader(overlay).pages[0])
    writer.add_metadata({"/Title": "The Myst Graph", "/Author": "Guillaume Lethuillier"})
    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = None
    try:
        with NamedTemporaryFile(dir=destination.parent, suffix=".pdf", delete=False) as output:
            temporary = Path(output.name)
            writer.write(output)
        os.replace(temporary, destination)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--date", type=date.fromisoformat, default=date.today())
    parser.add_argument("--subtitle", default="Myst (Macintosh, 1993) as a Graph")
    args = parser.parse_args()
    frame_graph(args.input, args.output, args.date, args.subtitle)
    print(f"Framed graph saved to {args.output}")


if __name__ == "__main__":
    main()

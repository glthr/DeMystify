"""Tests for the optional publication framing script.

They need the pinned PDF libraries (scripts/requirements-pdf.txt) and are
skipped when those are not installed.
"""
from datetime import date
import importlib.util
import os
from pathlib import Path
import stat
import sys
import tempfile
import unittest

HAVE_PDF_LIBRARIES = all(importlib.util.find_spec(name) for name in ("pypdf", "reportlab"))
sys.path.insert(0, str(Path(__file__).parent))


@unittest.skipUnless(HAVE_PDF_LIBRARIES, "pypdf and reportlab are not installed")
class FrameGraphTests(unittest.TestCase):
    def raw_graph(self, directory, mode=0o644):
        from reportlab.pdfgen import canvas
        path = Path(directory) / "graph.pdf"
        drawing = canvas.Canvas(str(path), pagesize=(1000, 500))
        drawing.drawString(100, 250, "Example:1")
        drawing.save()
        path.chmod(mode)
        return path

    def test_frame_adds_margins_heading_and_dated_footer(self):
        from frame_graph_pdf import frame_graph
        from pypdf import PdfReader
        with tempfile.TemporaryDirectory() as directory:
            source = self.raw_graph(directory)
            original = source.read_bytes()
            framed = Path(directory) / "published" / "myst_graph.pdf"
            frame_graph(source, framed, date(2026, 11, 9), "Myst (Macintosh, 1993) as a Graph")
            self.assertEqual(source.read_bytes(), original, "the raw graph must stay unchanged")
            reader = PdfReader(framed)
            self.assertEqual(len(reader.pages), 1)
            page = reader.pages[0]
            self.assertAlmostEqual(float(page.mediabox.width), 1040, places=3)
            self.assertAlmostEqual(float(page.mediabox.height), 550, places=3)
            text = page.extract_text()
            for expected in ("The Myst Graph", "Myst (Macintosh, 1993) as a Graph", "Updated (November 09, 2026) | glthr.com", "Example:1"):
                self.assertIn(expected, text)
            self.assertEqual(reader.metadata.title, "The Myst Graph")
            self.assertEqual([path.name for path in framed.parent.iterdir()], ["myst_graph.pdf"], "no temporary file may remain")

    def test_frame_refuses_to_overwrite_its_source(self):
        from frame_graph_pdf import frame_graph
        with tempfile.TemporaryDirectory() as directory:
            source = self.raw_graph(directory)
            original = source.read_bytes()
            with self.assertRaises(ValueError):
                frame_graph(source, source, date(2026, 11, 9), "subtitle")
            self.assertEqual(source.read_bytes(), original)

    @unittest.skipIf(os.name == "nt", "POSIX permission bits")
    def test_framed_pdf_is_as_readable_as_the_graph_it_frames(self):
        from frame_graph_pdf import frame_graph
        with tempfile.TemporaryDirectory() as directory:
            for mode in (0o644, 0o640, 0o600, 0o444):
                source = self.raw_graph(directory, mode)
                framed = Path(directory) / "framed.pdf"
                frame_graph(source, framed, date(2026, 11, 9), "subtitle")
                self.assertEqual(stat.S_IMODE(framed.stat().st_mode), mode | stat.S_IWUSR, oct(mode))
                source.chmod(0o644)
                source.unlink()


if __name__ == "__main__":
    unittest.main()

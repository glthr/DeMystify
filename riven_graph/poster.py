"""Create a zoomable vector poster from an extracted Riven graph.

Run with ``uv run --no-project --with reportlab python -m riven_graph.poster``.
The extractor itself has no third-party Python requirements; ReportLab is only
needed for this optional printable output.
"""

from __future__ import annotations

import argparse
from collections import Counter
import datetime as dt
import json
import math
from pathlib import Path


COLORS = {"aspit": "#94a3b8", "bspit": "#eb8f54", "gspit": "#72bb8c",
          "jspit": "#d6bb58", "ospit": "#ba8ee2", "pspit": "#72a9db",
          "rspit": "#e88eab", "tspit": "#55c9cc"}
STACK_NAMES = {"aspit": "Menu and journals", "bspit": "Boiler Island",
               "gspit": "Garden Island", "jspit": "Jungle Island",
               "ospit": "Gehn's Age", "pspit": "Prison Island",
               "rspit": "Tay / Rebel Age", "tspit": "Temple Island"}


def render_poster(graph: dict, output: Path) -> None:
    try:
        from reportlab.pdfgen import canvas
        from reportlab.lib.colors import HexColor, Color
        from reportlab.pdfbase.pdfmetrics import stringWidth
    except ImportError as exc:
        raise SystemExit("ReportLab is required: uv run --no-project --with reportlab python -m riven_graph.poster") from exc

    nodes = graph["nodes"]
    real_nodes = [n for n in nodes if not n.get("virtual")]
    virtual_nodes = [n for n in nodes if n.get("virtual")]
    # Match the default browser projection: optional Zip navigation and the
    # engine's reset-to-menu transitions after endings are excluded.
    edges = [e for e in graph["edges"] if e["kind"] not in {"zip", "native-ending"}]
    if not nodes or any("x" not in n or "y" not in n for n in nodes):
        raise ValueError("Expected a nonempty graph with precomputed x/y coordinates")

    min_x, max_x = min(n["x"] for n in nodes), max(n["x"] for n in nodes)
    min_y, max_y = min(n["y"] for n in nodes), max(n["y"] for n in nodes)
    dx, dy = max(max_x - min_x, 1), max(max_y - min_y, 1)
    # A large landscape poster keeps all 2,307 card labels as genuine vector
    # text.  It may be zoomed or tiled by the PDF viewer without pixelation.
    page_width = 7200.0
    margin, header_height, footer_height = 120.0, 410.0, 205.0
    graph_width = page_width - 2 * margin
    scale = graph_width / dx
    graph_height = dy * scale
    page_height = graph_height + header_height + footer_height
    if page_height > 14000:
        scale *= (14000 - header_height - footer_height) / graph_height
        graph_width, graph_height = dx * scale, dy * scale
        page_width = graph_width + 2 * margin
        page_height = 14000

    points = {n["id"]: (margin + (n["x"] - min_x) * scale,
                             footer_height + (max_y - n["y"]) * scale) for n in nodes}

    # Derive card dimensions from local layout separation to prevent opaque
    # labels from touching each other in dense areas.
    nearest = []
    point_values = list(points.values())
    for i, (x, y) in enumerate(point_values):
        distance = min((math.hypot(x - xx, y - yy)
                        for j, (xx, yy) in enumerate(point_values) if i != j), default=70)
        nearest.append(distance)
    typical = sorted(nearest)[max(0, len(nearest) // 12)]
    font_size = max(4.3, min(8.3, typical / 8.0))
    node_height = font_size * 1.8
    widths = {n["id"]: max(21, stringWidth(n["label"], "Helvetica", font_size) + font_size * 1.15)
              for n in nodes}

    output.parent.mkdir(parents=True, exist_ok=True)
    pdf = canvas.Canvas(str(output), pagesize=(page_width, page_height), pageCompression=1)
    pdf.setTitle("Riven (1997) - The Card Graph")
    pdf.setAuthor("Riven graph extractor")
    pdf.setSubject("Directed structural references extracted from the supplied Riven CD images")
    paper, ink = HexColor("#faf9f6"), HexColor("#182b37")
    pdf.setFillColor(paper)
    pdf.rect(0, 0, page_width, page_height, fill=1, stroke=0)

    def text(x, y, value, size=24, bold=False, color=ink):
        pdf.setFillColor(color)
        pdf.setFont("Helvetica-Bold" if bold else "Helvetica", size)
        pdf.drawString(x, y, value)

    top = page_height - 105
    text(margin, top, "RIVEN", 100, True)
    text(margin + 405, top + 5, "THE CARD GRAPH", 53)
    text(margin, top - 65,
         f"1997 CD edition  |  {len(real_nodes):,} shipped cards  |  {len(virtual_nodes)} absent targets  |  {len(edges):,} typed directed connections  |  8 stacks", 30)
    text(margin, top - 110,
         "Solid boxes are shipped cards; hollow boxes preserve references to absent targets. Arrows preserve direction. A card can contain multiple render states.", 23)

    counts = Counter(n["stack"] for n in real_nodes)
    virtual_counts = Counter(n["stack"] for n in virtual_nodes)
    legend_y = top - 185
    slot = (page_width - 2 * margin) / 8
    for index, stack in enumerate(STACK_NAMES):
        x = margin + index * slot
        pdf.setFillColor(HexColor(COLORS[stack]))
        pdf.roundRect(x, legend_y - 6, 27, 27, 5, fill=1, stroke=0)
        text(x + 41, legend_y + 3, STACK_NAMES[stack], 20, True)
        count_label = f"{counts[stack]:,} cards / {stack}"
        if virtual_counts[stack]:
            count_label += f" / +{virtual_counts[stack]} absent"
        text(x + 41, legend_y - 29, count_label, 18)
    pdf.setStrokeColor(HexColor("#d4d9dc"))
    pdf.setLineWidth(1.4)
    pdf.line(margin, page_height - header_height + 40, page_width - margin, page_height - header_height + 40)

    def boundary(a, b, half_width, half_height):
        vx, vy = b[0] - a[0], b[1] - a[1]
        if abs(vx) + abs(vy) < 1e-10:
            return a
        ratio = min(half_width / abs(vx) if vx else math.inf,
                    half_height / abs(vy) if vy else math.inf)
        return a[0] + vx * ratio, a[1] + vy * ratio

    cross_count = sum(bool(e.get("cross_stack")) for e in edges)
    native_count = sum(e["kind"].startswith("native") for e in edges)
    # Draw intra-stack routes first, then native/cross-stack routes; node
    # backgrounds cover line crossings beneath their labels.
    for edge in sorted(edges, key=lambda e: bool(e.get("cross_stack"))):
        source, target = edge["source"], edge["target"]
        a, b = points[source], points[target]
        cross = bool(edge.get("cross_stack"))
        native = edge["kind"].startswith("native")
        pdf.setStrokeColor(HexColor("#aa6333") if cross else HexColor("#8f9da4"))
        pdf.setFillColor(HexColor("#aa6333") if cross else HexColor("#70828e"))
        pdf.setLineWidth(1.15 if cross else .48)
        pdf.setDash(3.5, 2.5) if native else pdf.setDash()
        if source == target:
            loop = max(node_height * .75, 7)
            pdf.circle(a[0] + widths[source] / 2, a[1] + node_height / 2, loop, stroke=1, fill=0)
            continue
        start = boundary(a, b, widths[source] / 2 + 1, node_height / 2 + 1)
        end = boundary(b, a, widths[target] / 2 + 1, node_height / 2 + 1)
        vx, vy = end[0] - start[0], end[1] - start[1]
        length = math.hypot(vx, vy)
        if length < 1:
            continue
        pdf.line(*start, *end)
        arrow_length = max(2.8, font_size * .45)
        ux, uy = vx / length, vy / length
        base_x, base_y = end[0] - ux * arrow_length, end[1] - uy * arrow_length
        arrow = pdf.beginPath()
        arrow.moveTo(*end)
        arrow.lineTo(base_x - uy * arrow_length * .4, base_y + ux * arrow_length * .4)
        arrow.lineTo(base_x + uy * arrow_length * .4, base_y - ux * arrow_length * .4)
        arrow.close()
        pdf.drawPath(arrow, fill=1, stroke=0)
    pdf.setDash()

    for node in nodes:
        x, y = points[node["id"]]
        width = widths[node["id"]]
        virtual = bool(node.get("virtual"))
        pdf.setFillColor(paper if virtual else HexColor(COLORS.get(node["stack"], "#d0d7dd")))
        pdf.setStrokeColor(ink if virtual else Color(.2, .3, .34, alpha=.55))
        pdf.setLineWidth(.8 if virtual else .35)
        pdf.setDash(2, 1.2) if virtual else pdf.setDash()
        pdf.roundRect(x - width / 2, y - node_height / 2, width, node_height,
                      min(2.4, node_height / 5), fill=1, stroke=1)
        pdf.setFillColor(ink)
        pdf.setFont("Helvetica", font_size)
        pdf.drawCentredString(x, y - font_size * .34, node["label"])
    pdf.setDash()

    # Mark the game's first playable card without moving it from the layout.
    start_id = graph.get("start_card")
    if start_id in points:
        x, y = points[start_id]
        pdf.setStrokeColor(HexColor("#172d39"))
        pdf.setLineWidth(2)
        pdf.roundRect(x - widths[start_id] / 2 - 3, y - node_height / 2 - 3,
                      widths[start_id] + 6, node_height + 6, 4, fill=0, stroke=1)

    # The footer is deliberately separate from the graph to keep the projection
    # limits visible at normal poster scale.
    pdf.setFillColor(paper)
    pdf.rect(0, 0, page_width, footer_height - 40, fill=1, stroke=0)
    pdf.setStrokeColor(HexColor("#d4d9dc"))
    pdf.setLineWidth(1.4)
    pdf.line(margin, footer_height - 40, page_width - margin, footer_height - 40)
    text(margin, 126,
         f"ARROWS  Gray: same stack  /  Brown: cross-stack ({cross_count})  /  Dashed: native navigation ({native_count})  /  Double border: first playable card  /  Hollow dashed box: absent target", 22)
    text(margin, 90,
         "Structural projection: all conditional branches are combined; puzzle state, enabled hotspots, script order and saved returns are not executed. Zip shortcuts and ending resets are excluded.", 21)
    stamp = str(graph.get("generated_at", graph.get("generated", dt.date.today().isoformat())))
    text(margin, 51, f"Extraction: {stamp}  |  Source data: the supplied five CD-ROM images  |  Vector layout: Graphviz sfdp, seed 42", 18)
    commit = graph.get("scummvm_commit", "aed0208e772b128d3f953207a297fc521c32833a")
    text(margin, 22,
         f"Engine semantics: github.com/scummvm/scummvm @ {commit} / engines/mohawk. Bytecode references and native transitions remain separately attributed in the JSON explorer.", 17)
    pdf.showPage()
    pdf.save()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("graph", nargs="?", type=Path, default=Path("generated/riven/graph.json"))
    parser.add_argument("--output", type=Path, default=Path("output/pdf/riven-graph.pdf"))
    args = parser.parse_args(argv)
    render_poster(json.loads(args.graph.read_text()), args.output)
    print(args.output)


if __name__ == "__main__":
    main()

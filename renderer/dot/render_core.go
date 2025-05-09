package dot

import (
	"bytes"
)

// buildDOT generates the DOT string representation
func (g *Generator) buildDOT() (string, error) {
	var buf bytes.Buffer

	buf.WriteString("/* Generated with DeMystify (github.com/glthr/DeMystify) */\n\n")

	buf.WriteString("digraph G {\n")

	g.writeGraphAttributes(&buf)
	g.writeComponentClusters(&buf)
	g.writeNodes(&buf)
	g.writeEdges(&buf)

	buf.WriteString("}\n")

	return buf.String(), nil
}

// writeGraphAttributes adds global graph attributes to the DOT output
func (g *Generator) writeGraphAttributes(buf *bytes.Buffer) {
	buf.WriteString(
		`
  graph [
    overlap=false,
    splines=true,
    ordering=out,
    nodesep=0.7,
    K=1.5,
    ranksep=1.0,
    fontname="Arial",
    pad=0.6,
    margin=0.6,
    compound=true,
    outputorder="edgesfirst",
    sep="+20",
    pack=true,
    packmode="graph",
    pack_component_margin=80,
    newrank=true,     // Better layer assignment
    concentrate=false // Required to be false, otherwise disabled arrows mask valid ones
  ];

  node [
        shape=box,
        style="filled,rounded",
        fillcolor="#E8E8E8",
        fontname="Arial", 
        fontsize=10,
        penwidth=1.0,
        width=0.4,
        height=0.4,
        fixedsize=false,
        margin="0.1,0.05"
  ];
			
  edge [
        color="#000000",
        arrowsize=0.6,
        penwidth=0.6, 
        minlen=1,
        weight=0.8
  ];

`)
}

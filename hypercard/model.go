// Package hypercard loads the stack data used for static HyperTalk analysis.
// It does not execute scripts or decode artwork and other media.
package hypercard

// Corpus is a collection of stacks, independent of their storage format.
type Corpus struct {
	Stacks []Stack
}

// Source identifies the input object. Offset is a byte offset for native stacks.
type Source struct {
	Path   string
	Offset int64
	Format string
}

// Script retains decoded text before the analysis trims individual lines.
type Script struct {
	Text   string
	Source Source
}

type Stack struct {
	Name                string
	Width               int
	Height              int
	Source              Source
	Script              Script
	Cards               []Card
	Backgrounds         []Background
	CardOrder           []int // Authoritative order, separate from graph enumeration.
	BackgroundsComplete bool  // False means that background bindings are unknown.
	Diagnostics         []Diagnostic
}

type Card struct {
	ID           int
	Name         string
	BackgroundID int
	Parts        []Part
	Contents     []Content
	Script       Script
	Source       Source
}

type Background struct {
	ID       int
	Name     string
	Parts    []Part
	Contents []Content
	Script   Script
	Source   Source
}

type Part struct {
	ID      int
	Type    string
	Name    string
	Rect    Rectangle
	Hidden  bool
	Enabled bool
	Script  Script
	Source  Source
}

// Rectangle stores the original QuickDraw coordinates in card pixels.
type Rectangle struct {
	Top    int `xml:"top"`
	Left   int `xml:"left"`
	Bottom int `xml:"bottom"`
	Right  int `xml:"right"`
}

type Content struct {
	Layer string
	ID    int
	Text  string
}

// Diagnostic records an input block that does not contribute to the graph.
type Diagnostic struct {
	BlockType string
	ID        int
	Offset    int64
	Message   string
}

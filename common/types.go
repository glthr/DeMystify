package common

import (
	"slices"
)

type NodeAttribute int

const (
	IsCard NodeAttribute = 1 << iota
	IsStack
	IsVirtual
	ContainsBluePage
	ContainsRedPage
	ContainsWhitePage
	IsIsolated // node with no incoming edges nor outgoing edges
	IsSource   // node with no incoming edges from other nodes (source)
	IsSink     // node with no outgoing edges (sink)
)

type Node struct {
	Attributes    []NodeAttribute
	GraphID       int64
	StackName     string
	Name          string
	OriginalName  *string
	SecondaryName *string
	Details       *CardDetails // Optional game-neutral card disassembly and stored geometry.
}

func (n Node) IsOfType(t NodeAttribute) bool {
	return slices.Contains(n.Attributes, t)
}

type EdgeAttribute int

const (
	IntraAge EdgeAttribute = 1 << iota
	CrossAge
	Disabled
	SelfReference
	NotImplemented
	Backtracking
	RestrictiveTransitivityTail
	RestrictiveTransitivityHead
	// SequenceContinuation is the step from a view that a handler has just
	// requested to the next one it requests, with nothing but display, sound
	// and wait commands or the end of a branch between the two requests. The
	// handler's own card keeps its reference to both. The continuation is
	// displayed and counts as an outgoing reference, but path search does not
	// follow it: one action is not several steps, and a view shared by callers
	// does not join their routes.
	SequenceContinuation
	// Conditional is a structural reference whose local branch guards are retained.
	Conditional
	// Native marks a reviewed transition implemented by the game interpreter.
	Native
	// Zip and EndingReset are displayed but excluded from ordinary path search.
	Zip
	EndingReset
)

type Edge struct {
	Attributes     []EdgeAttribute
	Source, Target *Node
	TransitivityID int64
	Kind           string
	Evidence       []map[string]any
}

func (e Edge) IsOfType(t EdgeAttribute) bool {
	return slices.Contains(e.Attributes, t)
}

func (e Edge) GetSourceAndTargetIDs() []int64 {
	return []int64{e.Source.GraphID, e.Target.GraphID}
}

type Metadata struct {
	Game          string // Empty retains the original Myst presentation.
	StartNodeID   int64
	StartNodeName string
	TotalCards    uint
	TotalStacks   uint
	TotalNodes    uint
	TotalEdges    uint
	Nodes         []*Node
	Edges         []*Edge
	Stats         GraphStats
}

// GraphStats contains analysis results for a graph
type GraphStats struct {
	// path and connectivity analysis
	// ShortestPaths is left nil by MystGraph.Process; callers that need every
	// route assign the result of MystGraph.ComputeAllShortestPaths themselves.
	ShortestPaths          map[int64]map[int64]ShortestPathInfo
	MostSeparatedNodes     NodePairInfo
	ConnectedComponents    [][]int64
	DisconnectedComponents [][]NodeInfo

	// node degree information
	MostIncomingNode    NodeDegreeInfo
	MostOutgoingNode    NodeDegreeInfo
	NodesWithNoIncoming []NodeInfo
	NodesWithNoOutgoing []NodeInfo
	IsolatedNodes       []NodeInfo
	NodesWithSelfLoops  []NodeInfo
}

// NodePairInfo contains information about a pair of nodes
type NodePairInfo struct {
	Source   NodeInfo
	Target   NodeInfo
	Distance float64
	Path     []int64
}

// ShortestPathInfo contains the shortest path information between two nodes
type ShortestPathInfo struct {
	From     int64
	To       int64
	Distance float64
	Path     []int64
}

// NodeDegreeInfo contains the degree information for a node
type NodeDegreeInfo struct {
	ID     int64
	Name   string
	Degree int
}

type NodeInfo struct {
	ID   int64
	Name string
}

type EdgeInfo struct {
	From NodeInfo
	To   NodeInfo
}

type TransitivityRank int

const (
	DefaultTransitivity TransitivityRank = iota
	RestrictiveTransitivity
	Tail // first part (tail) of restrictive transitivity edge
	Head // second part (head) of restrictive transitivity edge
)

// CardDetails contains optional source data for engines other than HyperCard.
// The shared graph and renderer preserve it without executing the game.
type CardDetails struct {
	SourceID       string
	GlobalID       string
	Script         string
	ViewWidth      int
	ViewHeight     int
	Zones          []InteractionZone
	Provenance     map[string]any
	Unresolved     []map[string]any
	PictureRecords []map[string]any
}

// InteractionZone is stored source geometry; Enabled/Hidden are not runtime guarantees.
type InteractionZone struct {
	ID                        int
	Name, Type, Layer, Script string
	X, Y, Width, Height       int
	Enabled, Hidden           bool
	RawFlags                  uint16
	// Availability is an optional static proof result, distinct from stored
	// HyperCard enabled flags and from the game's current runtime state.
	Availability, AvailabilityReason string
}

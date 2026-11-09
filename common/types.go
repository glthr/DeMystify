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
	// SequenceContinuation is the step from a view that a linear handler has
	// just requested to the next one it requests. The handler's own card keeps
	// its reference to both. The continuation is displayed and counts as an
	// outgoing reference, but path search does not follow it: one action is not
	// several steps, and a view shared by callers does not join their routes.
	SequenceContinuation
)

type Edge struct {
	Attributes     []EdgeAttribute
	Source, Target *Node
	TransitivityID int64
}

func (e Edge) IsOfType(t EdgeAttribute) bool {
	return slices.Contains(e.Attributes, t)
}

func (e Edge) GetSourceAndTargetIDs() []int64 {
	return []int64{e.Source.GraphID, e.Target.GraphID}
}

type Metadata struct {
	TotalCards  uint
	TotalStacks uint
	TotalNodes  uint
	TotalEdges  uint
	Nodes       []*Node
	Edges       []*Edge
	Stats       GraphStats
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

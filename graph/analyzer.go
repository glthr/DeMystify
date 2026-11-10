package graph

// Process analyzes the Myst Graph on three dimensions: nodes, components, and paths
func (g *MystGraph) Process() {
	// nodes
	g.Metadata.Stats.MostIncomingNode = g.FindNodeWithMostIncomingEdges()
	g.Metadata.Stats.MostOutgoingNode = g.FindNodeWithMostOutgoingEdges()
	g.Metadata.Stats.NodesWithNoIncoming = g.FindNodesWithNoIncomingEdges()
	g.Metadata.Stats.NodesWithNoOutgoing = g.FindNodesWithNoOutgoingEdges()
	g.Metadata.Stats.IsolatedNodes = g.FindIsolatedNodes()
	g.Metadata.Stats.NodesWithSelfLoops = g.FindNodesWithSelfLoops()

	// components
	g.Metadata.Stats.ConnectedComponents = g.FindConnectedComponents()

	// paths
	// Nothing reads the complete all-pairs table, which holds about 1.7 million
	// routes for the full Myst graph. ComputeAllShortestPaths builds it on demand.
	g.Metadata.Stats.MostSeparatedNodes = g.FindMostSeparatedNodes()
}

package graph

import (
	"fmt"
	"math"

	"github.com/glthr/DeMystify/common"

	"gonum.org/v1/gonum/graph/path"
)

// pathAnalyzer is a utility for path finding and analysis
type pathAnalyzer struct {
	g *MystGraph
}

func newPathAnalyzer(g *MystGraph) *pathAnalyzer {
	return &pathAnalyzer{g: g}
}

// FindMostSeparatedNodes identifies the pair of connected nodes with the longest shortest path
func (g *MystGraph) FindMostSeparatedNodes() common.NodePairInfo {
	result := g.pathAnalyzer.findMostSeparatedNodes()

	return result
}

// ComputeAllShortestPaths calculates the shortest paths between all pairs of nodes
func (g *MystGraph) ComputeAllShortestPaths() map[int64]map[int64]common.ShortestPathInfo {
	return g.pathAnalyzer.computeShortestPaths()
}

// ComputeShortestPath calculates the shortest path between two nodes
func (g *MystGraph) ComputeShortestPath(from, to int64, mandatoryNodes []common.Node) (*common.ShortestPathInfo, error) {
	return g.pathAnalyzer.computeShortestPath(from, to, mandatoryNodes)
}

// GetMostSeparatedNodePath returns the path between the most separated nodes
func (g *MystGraph) GetMostSeparatedNodePath() ([]int64, bool) {
	mostSeparated := g.FindMostSeparatedNodes()

	// if no path found (either nodes are disconnected or no valid path exists)
	if len(mostSeparated.Path) == 0 {
		return nil, false
	}

	// return the path — it is guaranteed to be valid now
	// ensure we are returning a copy of the path, not the original reference
	pathCopy := make([]int64, len(mostSeparated.Path))
	copy(pathCopy, mostSeparated.Path)

	return pathCopy, true
}

// computeShortestPaths calculates all shortest paths in the graph using Dijkstra
func (pa *pathAnalyzer) computeShortestPaths() map[int64]map[int64]common.ShortestPathInfo {
	result := make(map[int64]map[int64]common.ShortestPathInfo)

	nodeIDs := pa.g.traverser.getAllNodeIDs()

	for _, src := range nodeIDs {
		result[src] = make(map[int64]common.ShortestPathInfo)

		// use Dijkstra's algorithm from Gonum
		p := path.DijkstraFrom(pa.g.Node(src), orderedPathGraph{pa.g})

		// for each destination node...
		for _, dst := range nodeIDs {
			if src == dst {
				continue // Skip self-paths
			}

			// ... get the shortest path
			pathNodes, weight := p.To(dst)

			// check if there is a path
			if len(pathNodes) == 0 || math.IsInf(weight, 1) {
				// no path found, this could be due to disabled or backtracking edges
				continue
			}

			pathInfo := common.ShortestPathInfo{
				From:     src,
				To:       dst,
				Distance: weight,
				Path:     []int64{},
			}

			for _, node := range pathNodes {
				pathInfo.Path = append(pathInfo.Path, node.ID())
			}

			result[src][dst] = pathInfo
		}
	}

	return result
}

// computeShortestPath calculates the shortest path between two nodes
func (pa *pathAnalyzer) computeShortestPath(from, to int64, mandatoryNodes []common.Node) (*common.ShortestPathInfo, error) {
	if pa.g.Node(from) == nil {
		return nil, fmt.Errorf("source node %d does not exist", from)
	}
	if pa.g.Node(to) == nil {
		return nil, fmt.Errorf("target node %d does not exist", to)
	}

	// if mandatory nodes are provided, ensure they are included
	if len(mandatoryNodes) > 0 {
		shortestPath := common.ShortestPathInfo{
			From:     from,
			To:       to,
			Distance: 0,
			Path:     []int64{},
		}

		var path []int64
		curSource := from

		for _, node := range mandatoryNodes {
			if pa.g.Node(node.GraphID) == nil {
				return nil, fmt.Errorf("mandatory node %d does not exist", node.GraphID)
			}

			// compute the shortest path from the current source to the mandatory node
			tempPath, err := pa.computeShortestPath(curSource, node.GraphID, nil)
			if err != nil {
				return nil, fmt.Errorf("error in mandatory node path: %v", err)
			}

			// skip if the path uses backtracking
			if tempPath.Distance >= math.Inf(1) {
				return nil, fmt.Errorf("no valid path exists from %d to %d without backtracking", curSource, node.GraphID)
			}

			// append the path, excluding the last node to avoid duplication
			path = append(path, tempPath.Path[:len(tempPath.Path)-1]...)
			curSource = node.GraphID
		}

		// compute the shortest path from the last mandatory node to the destination
		finalPath, err := pa.computeShortestPath(curSource, to, nil)
		if err != nil {
			return nil, fmt.Errorf("error in final segment path: %v", err)
		}

		// skip if the path uses backtracking
		if finalPath.Distance >= math.Inf(1) {
			return nil, fmt.Errorf("no valid path exists from %d to %d without backtracking", curSource, to)
		}

		// append the final path
		path = append(path, finalPath.Path...)
		shortestPath.Path = path
		// Sum the complete route so mandatory segmentation does not change
		// floating-point rounding.
		for i := 1; i < len(path); i++ {
			weight, _ := pa.g.Weight(path[i-1], path[i])
			shortestPath.Distance += weight
		}
		if math.IsInf(shortestPath.Distance, 1) {
			return nil, fmt.Errorf("path distance overflows from %d to %d", from, to)
		}

		return &shortestPath, nil
	}

	return pa.computeSimpleShortestPath(from, to)
}

// findMostSeparatedNodes identifies the pair of connected nodes with the longest shortest path
// prioritizing peripheral nodes (nodes with fewer connections) at the ends of the path
func (pa *pathAnalyzer) findMostSeparatedNodes() common.NodePairInfo {
	nodeIDs := pa.g.traverser.getAllNodeIDs()

	// If we have less than 2 nodes, we cannot compute separation
	if len(nodeIDs) < 2 {
		return common.NodePairInfo{}
	}

	// equal distances favor peripheral nodes (fewer connections)
	nodeDegrees := make(map[int64]int, len(nodeIDs))
	for _, id := range nodeIDs {
		degree := 0
		for to := pa.g.To(id); to.Next(); {
			degree++
		}
		for from := pa.g.From(id); from.Next(); {
			degree++
		}
		nodeDegrees[id] = degree
	}

	found := false
	var source, target int64
	var distance float64
	var peripheralScore int

	// check all pairs of nodes
	for _, src := range nodeIDs {
		// calculate the shortest paths from this source
		p := path.DijkstraFrom(pa.g.Node(src), orderedPathGraph{pa.g})

		for _, dst := range nodeIDs {
			if src == dst {
				continue // skip self
			}

			// Distances rank every pair; only the selected route is materialized
			// below. Unreachable nodes and backtracking-only paths are infinite.
			weight := p.WeightTo(dst)
			if math.IsInf(weight, 1) {
				continue
			}

			// lower score means more peripheral; the first pair wins a complete tie
			score := nodeDegrees[src] + nodeDegrees[dst]
			if !found || weight > distance || weight == distance && score < peripheralScore {
				found = true
				source, target, distance, peripheralScore = src, dst, weight, score
			}
		}
	}

	if !found {
		return common.NodePairInfo{}
	}

	var route []int64
	if shortest, err := pa.computeShortestPath(source, target, nil); err == nil {
		route = shortest.Path
	} else {
		nodes, _ := path.DijkstraFrom(pa.g.Node(source), orderedPathGraph{pa.g}).To(target)
		for _, node := range nodes {
			route = append(route, node.ID())
		}
	}

	return common.NodePairInfo{
		Source:   common.NodeInfo{ID: source, Name: pa.g.GetNameForID(source)},
		Target:   common.NodeInfo{ID: target, Name: pa.g.GetNameForID(target)},
		Distance: distance,
		Path:     route,
	}
}

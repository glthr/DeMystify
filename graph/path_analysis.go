package graph

import (
	"fmt"
	"math"
	"sort"

	"github.com/glthr/DeMystify/common"

	gograph "gonum.org/v1/gonum/graph"
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
	if mostSeparated.Path == nil || len(mostSeparated.Path) == 0 || mostSeparated.Distance <= 0 {
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
		p := path.DijkstraFrom(pa.g.Node(src), pa.g)

		// for each destination node...
		for _, dst := range nodeIDs {
			if src == dst {
				continue // Skip self-paths
			}

			// ... get the shortest path
			pathNodes, weight := p.To(dst)

			// check if there is a path
			if len(pathNodes) == 0 {
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

			// validation: check that no backtracking or disabled edge exists in this path
			valid := true
			for i := 0; i < len(pathInfo.Path)-1; i++ {
				fromID := pathInfo.Path[i]
				toID := pathInfo.Path[i+1]

				// check if this edge is backtracking or disabled
				if edges, exists := pa.g.GetAllEdges(fromID, toID); exists {
					for _, edge := range edges {
						if edge.IsOfType(common.Backtracking) || edge.IsOfType(common.Disabled) {
							valid = false
							break
						}
					}
				}
			}

			if valid {
				result[src][dst] = pathInfo
			}
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
			shortestPath.Distance += tempPath.Distance
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
		shortestPath.Distance += finalPath.Distance
		shortestPath.Path = path

		return &shortestPath, nil
	}

	p := path.DijkstraFrom(pa.g.Node(from), pa.g)
	pathNodes, weight := p.To(to)

	pathInfo := common.ShortestPathInfo{
		From:     from,
		To:       to,
		Distance: weight,
		Path:     []int64{},
	}

	if len(pathNodes) > 0 {
		for _, node := range pathNodes {
			pathInfo.Path = append(pathInfo.Path, node.ID())
		}
	} else {
		return &pathInfo, fmt.Errorf("no path exists from %d to %d", from, to)
	}

	// check if the path uses backtracking
	if weight >= math.Inf(1) {
		return &pathInfo, fmt.Errorf("no valid path exists from %d to %d without backtracking", from, to)
	}

	// if there are multiple possible shortest paths with the same weight,
	// we need to ensure a deterministic selection.
	if len(pathInfo.Path) > 2 { // only needed for paths with intermediate nodes
		alternativePaths := pa.findAllShortestPaths(from, to, weight)
		if len(alternativePaths) > 1 {
			// sort the paths lexicographically to ensure deterministic selection
			sort.Slice(alternativePaths, func(i, j int) bool {
				// compare paths lexicographically
				for k := 0; k < len(alternativePaths[i]) && k < len(alternativePaths[j]); k++ {
					if alternativePaths[i][k] != alternativePaths[j][k] {
						return alternativePaths[i][k] < alternativePaths[j][k]
					}
				}
				// if one path is a prefix of the other, the shorter one comes first
				return len(alternativePaths[i]) < len(alternativePaths[j])
			})
			// replace with the lexicographically smallest path
			pathInfo.Path = alternativePaths[0]
		}
	}

	return &pathInfo, nil
}

// findAllShortestPaths finds all possible shortest paths between two nodes with a given target weight
func (pa *pathAnalyzer) findAllShortestPaths(from, to int64, targetWeight float64) [][]int64 {
	allPaths := [][]int64{}

	var explore func(current int64, visited map[int64]bool, path []int64, currentWeight float64)
	explore = func(current int64, visited map[int64]bool, path []int64, currentWeight float64) {
		// if we reached the target...
		if current == to {
			// ... and this is a shortest path (weight matches target)...
			if math.Abs(currentWeight-targetWeight) < 1e-9 {
				// ... make a copy of the path to avoid reference issues
				pathCopy := make([]int64, len(path))
				copy(pathCopy, path)
				allPaths = append(allPaths, pathCopy)
			}
			return
		}

		// if we have exceeded the target weight, stop exploring this path
		if currentWeight > targetWeight {
			return
		}

		// mark the current node as visited
		visited[current] = true

		// explore all neighbors
		neighbors := pa.g.From(current)

		var sortedNeighbors []struct {
			id     int64
			weight float64
		}

		for neighbors.Next() {
			neighbor := neighbors.Node()
			neighborID := neighbor.ID()

			if visited[neighborID] {
				continue
			}

			// get the edge weight
			edge := pa.g.Edge(current, neighborID)

			// skip backtracking and disabled edges
			if pa.g.IsBacktrackingEdge(current, neighborID) || pa.g.IsDisabledEdge(current, neighborID) {
				continue
			}

			// check if this edge would stay within the target weight
			edgeWeight := edge.(gograph.WeightedEdge).Weight()
			if currentWeight+edgeWeight <= targetWeight+1e-9 {
				sortedNeighbors = append(sortedNeighbors, struct {
					id     int64
					weight float64
				}{neighborID, edgeWeight})
			}
		}

		sort.Slice(sortedNeighbors, func(i, j int) bool {
			return sortedNeighbors[i].id < sortedNeighbors[j].id
		})

		for _, neighbor := range sortedNeighbors {
			newPath := append(path, neighbor.id)
			newVisited := make(map[int64]bool)
			for k, v := range visited {
				newVisited[k] = v
			}

			// continue exploration
			explore(neighbor.id, newVisited, newPath, currentWeight+neighbor.weight)
		}
	}

	// start exploration from the origin node
	initialPath := []int64{from}
	initialVisited := make(map[int64]bool)
	explore(from, initialVisited, initialPath, 0)

	return allPaths
}

// findMostSeparatedNodes identifies the pair of connected nodes with the longest shortest path
// prioritizing peripheral nodes (nodes with fewer connections) at the ends of the path
func (pa *pathAnalyzer) findMostSeparatedNodes() common.NodePairInfo {
	result := common.NodePairInfo{}
	maxFiniteDistance := -1.0

	nodeIDs := pa.g.traverser.getAllNodeIDs()

	// If we have less than 2 nodes, we cannot compute separation
	if len(nodeIDs) < 2 {
		return result
	}

	type pathCandidate struct {
		source   int64
		target   int64
		distance float64
		path     []int64
	}

	var candidates []pathCandidate

	// check all pairs of nodes
	for _, src := range nodeIDs {
		// calculate the shortest paths from this source
		p := path.DijkstraFrom(pa.g.Node(src), pa.g)

		for _, dst := range nodeIDs {
			if src == dst {
				continue // skip self
			}

			pathNodes, weight := p.To(dst)

			// skip unreachable nodes or paths using backtracking edges
			if len(pathNodes) == 0 || weight == math.Inf(1) {
				continue
			}

			// for reachable nodes with valid paths, track if this is a candidate
			if weight >= maxFiniteDistance {
				var pathIDs []int64
				for _, node := range pathNodes {
					pathIDs = append(pathIDs, node.ID())
				}

				if weight > maxFiniteDistance {
					maxFiniteDistance = weight
					candidates = []pathCandidate{}
				}

				candidates = append(candidates, pathCandidate{
					source:   src,
					target:   dst,
					distance: weight,
					path:     pathIDs,
				})
			}
		}
	}

	if len(candidates) == 0 {
		return result
	}

	if len(candidates) == 1 {
		candidate := candidates[0]
		return common.NodePairInfo{
			Source: common.NodeInfo{
				ID:   candidate.source,
				Name: pa.g.GetNameForID(candidate.source),
			},
			Target: common.NodeInfo{
				ID:   candidate.target,
				Name: pa.g.GetNameForID(candidate.target),
			},
			Distance: candidate.distance,
			Path:     candidate.path,
		}
	}

	// multiple candidates with the same distance: favor peripheral nodes
	nodeDegrees := make(map[int64]int)
	for _, id := range nodeIDs {
		inDegree := 0
		to := pa.g.To(id)
		for to.Next() {
			inDegree++
		}

		outDegree := 0
		from := pa.g.From(id)
		for from.Next() {
			outDegree++
		}

		nodeDegrees[id] = inDegree + outDegree
	}

	// find the candidate with the most peripheral end nodes
	bestCandidate := candidates[0]
	bestPeripheralScore := nodeDegrees[bestCandidate.source] + nodeDegrees[bestCandidate.target]

	for _, candidate := range candidates[1:] {
		currentScore := nodeDegrees[candidate.source] + nodeDegrees[candidate.target]

		// lower score means more peripheral (fewer connections)
		if currentScore < bestPeripheralScore {
			bestPeripheralScore = currentScore
			bestCandidate = candidate
		}
	}

	result = common.NodePairInfo{
		Source: common.NodeInfo{
			ID:   bestCandidate.source,
			Name: pa.g.GetNameForID(bestCandidate.source),
		},
		Target: common.NodeInfo{
			ID:   bestCandidate.target,
			Name: pa.g.GetNameForID(bestCandidate.target),
		},
		Distance: bestCandidate.distance,
		Path:     bestCandidate.path,
	}

	return result
}

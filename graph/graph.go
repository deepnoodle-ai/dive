package graph

import "fmt"

type Node interface {
	// Name of the node
	Name() string

	// Names of the nodes that must be executed before this node
	Dependencies() []string
}

// Graph can be used to create a topological sort of tasks to be executed.
type Graph struct {
	nodes  map[string]Node
	sorted []string
}

// New creates a new Graph from a list of nodes
func New(nodes []Node) *Graph {
	nodeMap := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		nodeMap[node.Name()] = node
	}
	return &Graph{nodes: nodeMap}
}

// TopologicalSort returns a topological sort of the nodes in the graph
func (g *Graph) TopologicalSort() ([]string, error) {

	if len(g.nodes) == 0 {
		return []string{}, nil
	}

	// Returns cached sort if available
	if g.sorted != nil {
		return g.sorted, nil
	}

	// Implementation of Kahn's algorithm for topological sort
	// Returns ordered list of node names respecting dependencies

	// Build in-degree counting for each node
	inDegree := make(map[string]int, len(g.nodes))
	for _, node := range g.nodes {
		inDegree[node.Name()] = len(node.Dependencies())
	}

	// Find all nodes with no dependencies
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	// If there are no nodes with no dependencies, return an error
	if len(queue) == 0 {
		return nil, fmt.Errorf("invalid node dependencies: no starting point")
	}

	var result []string
	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// For each task that depends on current task, reduce its in-degree
		for id, node := range g.nodes {
			for _, dep := range node.Dependencies() {
				if dep == current {
					inDegree[id]--
					if inDegree[id] == 0 {
						queue = append(queue, id)
					}
				}
			}
		}
	}

	// If there are any nodes left, there is a cycle
	if len(result) != len(g.nodes) {
		return nil, fmt.Errorf("invalid node dependencies: cycle detected")
	}
	g.sorted = result
	return result, nil
}

package graph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskTopologicalSort(t *testing.T) {
	tests := []struct {
		name          string
		nodes         []Node
		expectedOrder []string
		expectError   string
	}{
		{
			name: "linear dependency chain",
			nodes: []Node{
				NewSimpleNode("task1", []string{}),
				NewSimpleNode("task2", []string{"task1"}),
				NewSimpleNode("task3", []string{"task2"}),
			},
			expectedOrder: []string{"task1", "task2", "task3"},
		},
		{
			name: "multiple starting points",
			nodes: []Node{
				NewSimpleNode("task1", []string{}),
				NewSimpleNode("task2", []string{}),
				NewSimpleNode("task3", []string{"task1", "task2"}),
			},
			expectedOrder: []string{"task1", "task2", "task3"}, // Note: order of task1 and task2 could be swapped
		},
		{
			name: "diamond dependency pattern",
			nodes: []Node{
				NewSimpleNode("task1", []string{}),
				NewSimpleNode("task2", []string{"task1"}),
				NewSimpleNode("task3", []string{"task1"}),
				NewSimpleNode("task4", []string{"task2", "task3"}),
			},
			expectedOrder: []string{"task1", "task2", "task3", "task4"}, // Note: order of task2 and task3 could be swapped
		},
		{
			name: "complex dependency graph",
			nodes: []Node{
				NewSimpleNode("task1", []string{}),
				NewSimpleNode("task2", []string{}),
				NewSimpleNode("task3", []string{"task1"}),
				NewSimpleNode("task4", []string{"task2"}),
				NewSimpleNode("task5", []string{"task3", "task4"}),
				NewSimpleNode("task6", []string{"task5"}),
			},
			expectedOrder: []string{"task1", "task2", "task3", "task4", "task5", "task6"}, // Note: order may vary but must respect dependencies
		},
		{
			name: "no starting point with two nodes",
			nodes: []Node{
				NewSimpleNode("task1", []string{"task2"}),
				NewSimpleNode("task2", []string{"task1"}),
			},
			expectError: "invalid node dependencies: no starting point",
		},
		{
			name: "no starting point with three nodes",
			nodes: []Node{
				NewSimpleNode("task1", []string{"task3"}),
				NewSimpleNode("task2", []string{"task1"}),
				NewSimpleNode("task3", []string{"task2"}),
			},
			expectError: "invalid node dependencies: no starting point",
		},
		{
			name:          "empty task list",
			nodes:         []Node{},
			expectedOrder: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := New(tt.nodes)
			order, err := graph.TopologicalSort()

			// Check error expectation
			if tt.expectError != "" {
				require.EqualError(t, err, tt.expectError)
				return
			} else {
				require.NoError(t, err)
			}

			// For empty task list, we should get an empty result
			if len(tt.nodes) == 0 {
				require.Empty(t, order, "Expected empty result for empty task list")
				return
			}

			// Check if the order respects dependencies
			require.True(t, validateTopologicalOrder(tt.nodes, order),
				"TopologicalSort() returned invalid order %v", order)

			// For simple cases, check exact order
			if tt.name == "linear dependency chain" {
				require.Equal(t, tt.expectedOrder, order,
					"TopologicalSort() returned incorrect order")
			}
		})
	}
}

// validateTopologicalOrder checks if the given order respects all dependencies
func validateTopologicalOrder(nodes []Node, order []string) bool {
	// Build a map of task positions in the order
	positions := make(map[string]int)
	for i, nodeName := range order {
		positions[nodeName] = i
	}

	// Check if all tasks are in the order
	if len(nodes) != len(order) {
		return false
	}

	// Check if all dependencies come before their dependent tasks
	for _, node := range nodes {
		nodePos, exists := positions[node.Name()]
		if !exists {
			return false
		}

		for _, dep := range node.Dependencies() {
			depPos, exists := positions[dep]
			if !exists {
				return false
			}
			if depPos >= nodePos {
				return false
			}
		}
	}

	return true
}

package dive

import "fmt"

// taskGraph handles dependency resolution
type taskGraph struct {
	tasks  map[string]*Task
	sorted []string
}

func newTaskGraph(tasks []*Task) *taskGraph {
	taskMap := make(map[string]*Task, len(tasks))
	for _, task := range tasks {
		taskMap[task.Name()] = task
	}
	return &taskGraph{tasks: taskMap}
}

func (g *taskGraph) TopologicalSort() ([]string, error) {
	// Implementation of Kahn's algorithm for topological sort
	// Returns ordered list of task IDs respecting dependencies

	if g.sorted != nil {
		return g.sorted, nil
	}

	// Build in-degree counting for each task
	inDegree := make(map[string]int, len(g.tasks))
	for _, task := range g.tasks {
		inDegree[task.Name()] = len(task.Dependencies())
	}

	// Find all tasks with no dependencies
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	// If there are no tasks with no dependencies, return an error
	if len(queue) == 0 {
		return nil, fmt.Errorf("invalid task dependencies: no starting point")
	}

	var result []string
	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// For each task that depends on current task, reduce its in-degree
		for id, task := range g.tasks {
			for _, dep := range task.Dependencies() {
				if dep == current {
					inDegree[id]--
					if inDegree[id] == 0 {
						queue = append(queue, id)
					}
				}
			}
		}
	}

	// If there are any tasks left, there is a cycle
	if len(result) != len(g.tasks) {
		return nil, fmt.Errorf("invalid task dependencies: cycle detected")
	}
	g.sorted = result
	return result, nil
}

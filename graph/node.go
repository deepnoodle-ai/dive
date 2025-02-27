package graph

// SimpleNode is a sample implementation of the Node interface that can be used
// for testing or in situations where a different implementation doesn't exist.
type SimpleNode struct {
	name         string
	dependencies []string
}

func (n *SimpleNode) Name() string {
	return n.name
}

func (n *SimpleNode) Dependencies() []string {
	return n.dependencies
}

func NewSimpleNode(name string, dependencies []string) *SimpleNode {
	return &SimpleNode{name: name, dependencies: dependencies}
}

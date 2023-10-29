package ast

import "fmt"

var extendedSyntaxEnabled = true

func createCommaSeparatedNodes[T Node](
	leadingNodes []Node,
	nodes []T,
	commas []*RuneNode,
	trailingNodes []Node,
) []Node {
	for i, node := range leadingNodes {
		if node == nil {
			panic(fmt.Sprintf("leadingNodes[%d] is nil", i))
		}
	}
	for i, node := range trailingNodes {
		if node == nil {
			panic(fmt.Sprintf("trailingNodes[%d] is nil", i))
		}
	}
	if len(nodes) == 0 {
		panic("must have at least one node")
	}
	if !extendedSyntaxEnabled {
		if len(commas) != len(nodes)-1 {
			panic(fmt.Sprintf("%d nodes requires %d commas, not %d", len(nodes), len(nodes)-1, len(commas)))
		}
	} else {
		if len(commas) != len(nodes)-1 && len(commas) != len(nodes) {
			panic(fmt.Sprintf("%[1]d nodes requires %d or %[1]d commas, not %d", len(nodes), len(nodes)-1, len(commas)))
		}
	}

	children := make([]Node, 0, len(leadingNodes)+len(nodes)+len(commas)+len(trailingNodes))
	children = append(children, leadingNodes...)
	for i, node := range nodes {
		if i > 0 {
			if commas[i-1] == nil {
				panic(fmt.Sprintf("commas[%d] is nil", i-1))
			}
			children = append(children, commas[i-1])
		}
		if Node(node) == nil {
			panic(fmt.Sprintf("nodes[%d] is nil", i))
		}
		children = append(children, node)
	}
	if extendedSyntaxEnabled && len(commas) == len(nodes) {
		children = append(children, commas[len(commas)-1])
	}
	children = append(children, trailingNodes...)
	return children
}

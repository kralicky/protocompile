package ast

import (
	"fmt"
	"sort"
)

var ExtendedSyntaxEnabled = true

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
	if !ExtendedSyntaxEnabled {
		if len(commas) != len(nodes)-1 {
			panic(fmt.Sprintf("%d nodes requires %d commas, not %d", len(nodes), len(nodes)-1, len(commas)))
		}
	}

	children := make([]Node, 0, len(leadingNodes)+len(nodes)+len(commas)+len(trailingNodes))
	children = append(children, leadingNodes...)
	for _, node := range nodes {
		children = append(children, node)
	}
	for _, comma := range commas {
		children = append(children, comma)
	}
	off := len(leadingNodes)
	sort.Slice(children[off:], func(i, j int) bool {
		return children[off+i].Start() < children[off+j].Start()
	})
	children = append(children, trailingNodes...)
	return children
}

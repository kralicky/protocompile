package ast

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInspect(t *testing.T) {
	tree := &MessageNode{
		Decls: []MessageElement{
			&OptionNode{
				Name: &OptionNameNode{
					Parts: []*FieldReferenceNode{
						{
							Name: &CompoundIdentNode{
								Components: []*IdentNode{
									{
										TerminalNode: 1,
									},
									{
										TerminalNode: 2,
									},
									{
										TerminalNode: 3,
									},
								},
							},
						},
						{
							Open: &RuneNode{Rune: 'x'},
						},
					},
				},
			},
		},
	}
	var tracker AncestorTracker
	paths := [][]Node{}
	Inspect(tree, func(n Node) bool {
		paths = append(paths, slices.Clone(tracker.Path()))
		return true
	}, tracker.AsWalkOptions()...)

	expectedPaths := [][]Node{
		{tree},
		{tree, tree.Decls[0]},
		{tree, tree.Decls[0], tree.Decls[0].(*OptionNode).Name},
		{tree, tree.Decls[0], tree.Decls[0].(*OptionNode).Name, tree.Decls[0].(*OptionNode).Name.Parts[0]},
		{tree, tree.Decls[0], tree.Decls[0].(*OptionNode).Name, tree.Decls[0].(*OptionNode).Name.Parts[0], tree.Decls[0].(*OptionNode).Name.Parts[0].Name},
		{tree, tree.Decls[0], tree.Decls[0].(*OptionNode).Name, tree.Decls[0].(*OptionNode).Name.Parts[0], tree.Decls[0].(*OptionNode).Name.Parts[0].Name, tree.Decls[0].(*OptionNode).Name.Parts[0].Name.(*CompoundIdentNode).Components[0]},
		{tree, tree.Decls[0], tree.Decls[0].(*OptionNode).Name, tree.Decls[0].(*OptionNode).Name.Parts[0], tree.Decls[0].(*OptionNode).Name.Parts[0].Name, tree.Decls[0].(*OptionNode).Name.Parts[0].Name.(*CompoundIdentNode).Components[1]},
		{tree, tree.Decls[0], tree.Decls[0].(*OptionNode).Name, tree.Decls[0].(*OptionNode).Name.Parts[0], tree.Decls[0].(*OptionNode).Name.Parts[0].Name, tree.Decls[0].(*OptionNode).Name.Parts[0].Name.(*CompoundIdentNode).Components[2]},
		{tree, tree.Decls[0], tree.Decls[0].(*OptionNode).Name, tree.Decls[0].(*OptionNode).Name.Parts[1]},
		{tree, tree.Decls[0], tree.Decls[0].(*OptionNode).Name, tree.Decls[0].(*OptionNode).Name.Parts[1], tree.Decls[0].(*OptionNode).Name.Parts[1].Open},
	}
	assert.Equal(t, expectedPaths, paths)
}

func TestZipWalk(t *testing.T) {
	// Test case 1: a and b have the same length
	a := []Node{&IdentNode{TerminalNode: 1}, &IdentNode{TerminalNode: 3}, &IdentNode{TerminalNode: 5}}
	b := []Node{&RuneNode{TerminalNode: 2}, &RuneNode{TerminalNode: 4}, &RuneNode{TerminalNode: 6}}
	visitedNodes := make([]Node, 0)
	visitor := testVisitFn(func(n Node) {
		visitedNodes = append(visitedNodes, n)
	})
	zipWalk(visitor, a, b)
	expectedVisitedNodes := []Node{a[0], b[0], a[1], b[1], a[2], b[2]}
	assertVisitedNodes(t, visitedNodes, expectedVisitedNodes)

	// Test case 2: a is longer than b
	a = []Node{&IdentNode{TerminalNode: 1}, &IdentNode{TerminalNode: 3}, &IdentNode{TerminalNode: 5}}
	b = []Node{&RuneNode{TerminalNode: 2}, &RuneNode{TerminalNode: 4}}
	visitedNodes = make([]Node, 0)
	zipWalk(visitor, a, b)
	expectedVisitedNodes = []Node{a[0], b[0], a[1], b[1], a[2]}
	assertVisitedNodes(t, visitedNodes, expectedVisitedNodes)

	// Test case 3: b is longer than a
	a = []Node{&IdentNode{TerminalNode: 1}, &IdentNode{TerminalNode: 3}}
	b = []Node{&RuneNode{TerminalNode: 2}, &RuneNode{TerminalNode: 4}, &RuneNode{TerminalNode: 6}}
	visitedNodes = make([]Node, 0)
	zipWalk(visitor, a, b)
	expectedVisitedNodes = []Node{a[0], b[0], a[1], b[1], b[2]}
	assertVisitedNodes(t, visitedNodes, expectedVisitedNodes)

	// Test case 4: a and b are empty
	a = []Node{}
	b = []Node{}
	visitedNodes = make([]Node, 0)
	zipWalk(visitor, a, b)
	expectedVisitedNodes = []Node{}
	assertVisitedNodes(t, visitedNodes, expectedVisitedNodes)
}

type testVisitFn func(n Node)

func (f testVisitFn) Visit(n Node) Visitor {
	f(n)
	return f
}

func (f testVisitFn) Before(Node) bool { return true }
func (f testVisitFn) After(Node)       {}
func assertVisitedNodes(t *testing.T, visitedNodes, expectedVisitedNodes []Node) {
	if len(visitedNodes) != len(expectedVisitedNodes) {
		t.Errorf("Unexpected number of visited nodes. Expected: %d, Got: %d", len(expectedVisitedNodes), len(visitedNodes))
		return
	}
	for i := range visitedNodes {
		if visitedNodes[i] != expectedVisitedNodes[i] {
			t.Errorf("Unexpected visited node at index %d. Expected: %v, Got: %v", i, expectedVisitedNodes[i], visitedNodes[i])
		}
	}
}

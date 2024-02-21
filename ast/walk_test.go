package ast

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInspect(t *testing.T) {
	tree := &MessageNode{
		Decls: []*MessageElement{
			{
				Val: &MessageElement_Option{
					Option: &OptionNode{
						Name: &OptionNameNode{
							Parts: []*FieldReferenceNode{
								{
									Name: &IdentValueNode{
										Val: &IdentValueNode_CompoundIdent{
											CompoundIdent: &CompoundIdentNode{
												Components: []*IdentNode{
													{
														Token: 1,
													},
													{
														Token: 2,
													},
													{
														Token: 3,
													},
												},
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
		{tree, tree.Decls[0].Unwrap()},
		{tree, tree.Decls[0].Unwrap(), tree.Decls[0].GetOption().Name},
		{tree, tree.Decls[0].Unwrap(), tree.Decls[0].GetOption().Name, tree.Decls[0].GetOption().Name.Parts[0]},
		{tree, tree.Decls[0].Unwrap(), tree.Decls[0].GetOption().Name, tree.Decls[0].GetOption().Name.Parts[0], tree.Decls[0].GetOption().Name.Parts[0].Name.Unwrap()},
		{tree, tree.Decls[0].Unwrap(), tree.Decls[0].GetOption().Name, tree.Decls[0].GetOption().Name.Parts[0], tree.Decls[0].GetOption().Name.Parts[0].Name.Unwrap(), tree.Decls[0].GetOption().Name.Parts[0].Name.GetCompoundIdent().Components[0]},
		{tree, tree.Decls[0].Unwrap(), tree.Decls[0].GetOption().Name, tree.Decls[0].GetOption().Name.Parts[0], tree.Decls[0].GetOption().Name.Parts[0].Name.Unwrap(), tree.Decls[0].GetOption().Name.Parts[0].Name.GetCompoundIdent().Components[1]},
		{tree, tree.Decls[0].Unwrap(), tree.Decls[0].GetOption().Name, tree.Decls[0].GetOption().Name.Parts[0], tree.Decls[0].GetOption().Name.Parts[0].Name.Unwrap(), tree.Decls[0].GetOption().Name.Parts[0].Name.GetCompoundIdent().Components[2]},
		{tree, tree.Decls[0].Unwrap(), tree.Decls[0].GetOption().Name, tree.Decls[0].GetOption().Name.Parts[1]},
		{tree, tree.Decls[0].Unwrap(), tree.Decls[0].GetOption().Name, tree.Decls[0].GetOption().Name.Parts[1], tree.Decls[0].GetOption().Name.Parts[1].Open},
	}
	assert.Equal(t, expectedPaths, paths)
}

func TestZipWalk(t *testing.T) {
	// Test case 1: a and b have the same length
	a := []Node{&IdentNode{Token: 1}, &IdentNode{Token: 3}, &IdentNode{Token: 5}}
	b := []Node{&RuneNode{Token: 2}, &RuneNode{Token: 4}, &RuneNode{Token: 6}}
	visitedNodes := make([]Node, 0)
	visitor := testVisitFn(func(n Node) {
		visitedNodes = append(visitedNodes, n)
	})
	zipWalk(visitor, a, b)
	expectedVisitedNodes := []Node{a[0], b[0], a[1], b[1], a[2], b[2]}
	assertVisitedNodes(t, visitedNodes, expectedVisitedNodes)

	// Test case 2: a is longer than b
	a = []Node{&IdentNode{Token: 1}, &IdentNode{Token: 3}, &IdentNode{Token: 5}}
	b = []Node{&RuneNode{Token: 2}, &RuneNode{Token: 4}}
	visitedNodes = make([]Node, 0)
	zipWalk(visitor, a, b)
	expectedVisitedNodes = []Node{a[0], b[0], a[1], b[1], a[2]}
	assertVisitedNodes(t, visitedNodes, expectedVisitedNodes)

	// Test case 3: b is longer than a
	a = []Node{&IdentNode{Token: 1}, &IdentNode{Token: 3}}
	b = []Node{&RuneNode{Token: 2}, &RuneNode{Token: 4}, &RuneNode{Token: 6}}
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

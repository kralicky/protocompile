package ast_test

import (
	"os"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/parser"
	"github.com/kralicky/protocompile/reporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestInspect(t *testing.T) {
	tree := &MessageNode{
		Decls: []*MessageElement{
			{
				Val: &MessageElement_Option{
					Option: &OptionNode{
						Name: &OptionNameNode{
							Parts: []*ComplexIdentComponent{
								{
									Val: &ComplexIdentComponent_FieldRef{
										FieldRef: &FieldReferenceNode{
											Name: &IdentValueNode{
												Val: &IdentValueNode_CompoundIdent{
													CompoundIdent: &CompoundIdentNode{
														Components: []*ComplexIdentComponent{
															{
																Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 1}},
															},
															{
																Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 2}},
															},
															{
																Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 3}},
															},
															{
																Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 4}},
															},
															{
																Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 5}},
															},
														},
													},
												},
											},
										},
									},
								},
								{
									Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 6}},
								},
							},
						},
					},
				},
			},
		},
	}
	var tracker AncestorTracker
	nodePaths := [][]Node{}
	paths := []string{}

	Inspect(tree, func(n Node) bool {
		nodePaths = append(nodePaths, slices.Clone(tracker.Path()))
		paths = append(paths, tracker.ProtoPath().String())
		return true
	}, tracker.AsWalkOptions()...)

	root := tree
	option := root.Decls[0].GetOption()
	optionName := option.GetName()
	part0FieldRef := optionName.GetParts()[0].GetFieldRef()
	compoundIdent := part0FieldRef.GetName().GetCompoundIdent()

	expectedNodePaths := [][]Node{
		{root},
		{root, option},
		{root, option, optionName},
		{root, option, optionName, part0FieldRef},
		{root, option, optionName, part0FieldRef, compoundIdent},
		{root, option, optionName, part0FieldRef, compoundIdent, compoundIdent.GetComponents()[0].GetIdent()},
		{root, option, optionName, part0FieldRef, compoundIdent, compoundIdent.GetComponents()[1].GetDot()},
		{root, option, optionName, part0FieldRef, compoundIdent, compoundIdent.GetComponents()[2].GetIdent()},
		{root, option, optionName, part0FieldRef, compoundIdent, compoundIdent.GetComponents()[3].GetDot()},
		{root, option, optionName, part0FieldRef, compoundIdent, compoundIdent.GetComponents()[4].GetIdent()},
		{root, option, optionName, optionName.GetParts()[1].GetIdent()},
	}
	expectedPaths := []string{
		"(ast.MessageNode)",
		"(ast.MessageNode).decls[0].option",
		"(ast.MessageNode).decls[0].option.name",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[0].ident",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[1].dot",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[2].ident",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[3].dot",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[4].ident",
		"(ast.MessageNode).decls[0].option.name.parts[1].ident",
	}

	assert.Equal(t, len(expectedNodePaths), len(nodePaths))
	for i := range expectedNodePaths {
		for j := range expectedNodePaths[i] {
			if diff := cmp.Diff(expectedNodePaths[i][j], nodePaths[i][j], protocmp.Transform()); diff != "" {
				t.Errorf("unexpected node path at index (%d, %d) (-want +got):\n%s", i, j, diff)
			}
		}
	}

	assert.Equal(t, expectedPaths, paths)
}

func TestFullAST(t *testing.T) {
	f, err := os.Open("../internal/testdata/desc_test_complex.proto")
	require.NoError(t, err)
	res, err := parser.Parse("../internal/testdata/desc_test_complex.proto", f, reporter.NewHandler(nil), 0)
	require.NoError(t, err)
	var tracker AncestorTracker
	nodePaths := [][]Node{}
	paths := []string{}

	Inspect(res, func(n Node) bool {
		nodePaths = append(nodePaths, slices.Clone(tracker.Path()))
		paths = append(paths, tracker.ProtoPath().String())
		return true
	}, tracker.AsWalkOptions()...)

	require.NotNil(t, nodePaths)
}

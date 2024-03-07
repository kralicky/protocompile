package ast_test

import (
	"os"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/ast/paths"
	"github.com/kralicky/protocompile/parser"
	"github.com/kralicky/protocompile/reporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

// message Foo { option (a.b.c).d = "e"; }
var sampleTree1 = &MessageNode{
	Keyword:   &IdentNode{Token: 1, Val: "message"},
	Name:      &IdentNode{Token: 2, Val: "Foo"},
	OpenBrace: &RuneNode{Token: 3, Rune: '{'},
	Decls: []*MessageElement{
		{
			Val: &MessageElement_Option{
				Option: &OptionNode{
					Keyword: &IdentNode{Token: 4, Val: "option"},
					Name: &OptionNameNode{
						Parts: []*ComplexIdentComponent{
							{
								Val: &ComplexIdentComponent_FieldRef{
									FieldRef: &FieldReferenceNode{
										Open: &RuneNode{Token: 5, Rune: '('},
										Name: &IdentValueNode{
											Val: &IdentValueNode_CompoundIdent{
												CompoundIdent: &CompoundIdentNode{
													Components: []*ComplexIdentComponent{
														{
															Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 6, Val: "a"}},
														},
														{
															Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 7, Rune: '.'}},
														},
														{
															Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 8, Val: "b"}},
														},
														{
															Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 9, Rune: '.'}},
														},
														{
															Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 10, Val: "c"}},
														},
													},
												},
											},
										},
										Close: &RuneNode{Token: 11, Rune: ')'},
									},
								},
							},
							{
								Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 12, Rune: '.'}},
							},
							{
								Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 13, Val: "d"}},
							},
						},
					},
					Equals: &RuneNode{Token: 14, Rune: '='},
					Val: &ValueNode{
						Val: &ValueNode_StringLiteral{
							StringLiteral: &StringLiteralNode{Token: 15, Val: "e"},
						},
					},
					Semicolon: &RuneNode{Token: 16, Rune: ';'},
				},
			},
		},
	},
	CloseBrace: &RuneNode{Token: 17, Rune: '}'},
}

// syntax = "proto3"; message Foo { optional string foo = 1 [default = "bar"]; }
var sampleTree2 = &FileNode{
	Syntax: &SyntaxNode{
		Keyword:   &IdentNode{Token: 1, Val: "syntax"},
		Equals:    &RuneNode{Token: 2, Rune: '='},
		Syntax:    &StringValueNode{Val: &StringValueNode_StringLiteral{StringLiteral: &StringLiteralNode{Token: 3, Val: "proto3"}}},
		Semicolon: &RuneNode{Token: 4, Rune: ';'},
	},
	Decls: []*FileElement{
		{
			Val: &FileElement_Message{
				Message: &MessageNode{
					Keyword:   &IdentNode{Token: 5, Val: "message"},
					Name:      &IdentNode{Token: 6, Val: "Foo"},
					OpenBrace: &RuneNode{Token: 7, Rune: '{'},
					Decls: []*MessageElement{
						{
							Val: &MessageElement_Field{
								Field: &FieldNode{
									Label:     &IdentNode{Token: 8, Val: "optional"},
									FieldType: &IdentValueNode{Val: &IdentValueNode_Ident{Ident: &IdentNode{Token: 9, Val: "string"}}},
									Name:      &IdentNode{Token: 10, Val: "foo"},
									Equals:    &RuneNode{Token: 11, Rune: '='},
									Tag:       &UintLiteralNode{Token: 12, Val: 1},
									Options: &CompactOptionsNode{
										OpenBracket: &RuneNode{Token: 13, Rune: '['},
										Options: []*OptionNode{
											{
												Name: &OptionNameNode{
													Parts: []*ComplexIdentComponent{
														{
															Val: &ComplexIdentComponent_FieldRef{
																FieldRef: &FieldReferenceNode{
																	Name: &IdentValueNode{
																		Val: &IdentValueNode_Ident{
																			Ident: &IdentNode{Token: 14, Val: "default"},
																		},
																	},
																},
															},
														},
													},
												},
												Equals: &RuneNode{Token: 15, Rune: '='},
												Val: &ValueNode{
													Val: &ValueNode_StringLiteral{
														StringLiteral: &StringLiteralNode{Token: 16, Val: "bar"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Val: &FileElement_Enum{
				Enum: &EnumNode{
					Keyword: &IdentNode{Token: 5, Val: "enum"},
				},
			},
		},
	},
}

// optional X x = 1 [(.bufbuild.protocompile.test2.x).x.(y).x.(y).(y).x.x.(y).x = {x: {[bufbuild.protocompile.test2.y]: {x: {}}}}];
var sampleTree3 = &FieldNode{
	Label:     &IdentNode{Token: 1, Val: "optional"},
	FieldType: &IdentValueNode{Val: &IdentValueNode_Ident{Ident: &IdentNode{Token: 2, Val: "X"}}},
	Name:      &IdentNode{Token: 3, Val: "x"},
	Equals:    &RuneNode{Token: 4, Rune: '='},
	Tag:       &UintLiteralNode{Token: 5, Val: 1},
	Options: &CompactOptionsNode{
		OpenBracket: &RuneNode{Token: 6, Rune: '['},
		Options: []*OptionNode{
			{
				Name: &OptionNameNode{
					Parts: []*ComplexIdentComponent{
						{ // (.bufbuild.protocompile.test2.x)
							Val: &ComplexIdentComponent_FieldRef{
								FieldRef: &FieldReferenceNode{
									Open: &RuneNode{Token: 7, Rune: '('},
									Name: &IdentValueNode{
										Val: &IdentValueNode_CompoundIdent{
											CompoundIdent: &CompoundIdentNode{
												Components: []*ComplexIdentComponent{
													{Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 8, Rune: '.'}}},
													{Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 9, Val: "bufbuild"}}},
													{Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 10, Rune: '.'}}},
													{Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 11, Val: "protocompile"}}},
													{Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 12, Rune: '.'}}},
													{Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 13, Val: "test2"}}},
													{Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 14, Rune: '.'}}},
													{Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 15, Val: "x"}}},
												},
											},
										},
									},
									Close: &RuneNode{Token: 16, Rune: ')'},
								},
							},
						},
						{ // .
							Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 17, Rune: '.'}},
						},
						{ // x
							Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 18, Val: "x"}},
						},
						{ // .
							Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 19, Rune: '.'}},
						},
						{ // (y)
							Val: &ComplexIdentComponent_FieldRef{
								FieldRef: &FieldReferenceNode{
									Open: &RuneNode{Token: 20, Rune: '('},
									Name: &IdentValueNode{
										Val: &IdentValueNode_Ident{Ident: &IdentNode{Token: 21, Val: "y"}},
									},
									Close: &RuneNode{Token: 22, Rune: ')'},
								},
							},
						},
						{ // .
							Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 23, Rune: '.'}},
						},
						{ // x
							Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 24, Val: "x"}},
						},
						{ // .
							Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 25, Rune: '.'}},
						},
						{ // (y)
							Val: &ComplexIdentComponent_FieldRef{
								FieldRef: &FieldReferenceNode{
									Open: &RuneNode{Token: 26, Rune: '('},
									Name: &IdentValueNode{
										Val: &IdentValueNode_Ident{Ident: &IdentNode{Token: 27, Val: "y"}},
									},
									Close: &RuneNode{Token: 28, Rune: ')'},
								},
							},
						},
						{ // .
							Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 29, Rune: '.'}},
						},
						{ // (y)
							Val: &ComplexIdentComponent_FieldRef{
								FieldRef: &FieldReferenceNode{
									Open: &RuneNode{Token: 30, Rune: '('},
									Name: &IdentValueNode{
										Val: &IdentValueNode_Ident{Ident: &IdentNode{Token: 30, Val: "y"}},
									},
									Close: &RuneNode{Token: 31, Rune: ')'},
								},
							},
						},
						{ // .
							Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 32, Rune: '.'}},
						},
						{ // x
							Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 33, Val: "x"}},
						},
						{ // .
							Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 34, Rune: '.'}},
						},
						{ // x
							Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 35, Val: "x"}},
						},
						{ // .
							Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 36, Rune: '.'}},
						},
						{ // (y)
							Val: &ComplexIdentComponent_FieldRef{
								FieldRef: &FieldReferenceNode{
									Open: &RuneNode{Token: 37, Rune: '('},
									Name: &IdentValueNode{
										Val: &IdentValueNode_Ident{Ident: &IdentNode{Token: 38, Val: "y"}},
									},
									Close: &RuneNode{Token: 39, Rune: ')'},
								},
							},
						},
						{ // .
							Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 40, Rune: '.'}},
						},
						{ // x
							Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 41, Val: "x"}},
						},
					},
				},
				Equals: &RuneNode{Token: 42, Rune: '='},
				Val: &ValueNode{
					Val: &ValueNode_MessageLiteral{
						MessageLiteral: &MessageLiteralNode{
							Open: &RuneNode{Token: 43, Rune: '{'},
							Elements: []*MessageFieldNode{
								{ // x: {...}
									Name: &FieldReferenceNode{
										Name: &IdentValueNode{
											Val: &IdentValueNode_Ident{
												Ident: &IdentNode{Token: 44, Val: "x"},
											},
										},
									},
									Sep: &RuneNode{Token: 45, Rune: ':'},
									Val: &ValueNode{
										Val: &ValueNode_MessageLiteral{
											MessageLiteral: &MessageLiteralNode{
												Open: &RuneNode{Token: 46, Rune: '{'},
												Elements: []*MessageFieldNode{
													{ // [bufbuild.protocompile.test2.y]: {x: {}}
														Name: &FieldReferenceNode{
															Open: &RuneNode{Token: 47, Rune: '['},
															Name: &IdentValueNode{
																Val: &IdentValueNode_CompoundIdent{
																	CompoundIdent: &CompoundIdentNode{
																		Components: []*ComplexIdentComponent{
																			{ // bufbuild
																				Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 48, Val: "bufbuild"}},
																			},
																			{ // .
																				Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 49, Rune: '.'}},
																			},
																			{ // protocompile
																				Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 50, Val: "protocompile"}},
																			},
																			{ // .
																				Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 51, Rune: '.'}},
																			},
																			{ // test2
																				Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 52, Val: "test2"}},
																			},
																			{ // .
																				Val: &ComplexIdentComponent_Dot{Dot: &RuneNode{Token: 53, Rune: '.'}},
																			},
																			{ // y
																				Val: &ComplexIdentComponent_Ident{Ident: &IdentNode{Token: 54, Val: "y"}},
																			},
																		},
																	},
																},
															},
															Close: &RuneNode{Token: 55, Rune: ']'},
														},
														Sep: &RuneNode{Token: 56, Rune: ':'},
														Val: &ValueNode{
															Val: &ValueNode_MessageLiteral{
																MessageLiteral: &MessageLiteralNode{
																	Open:  &RuneNode{Token: 57, Rune: '{'},
																	Close: &RuneNode{Token: 58, Rune: '}'},
																},
															},
														},
													},
												},
												Close: &RuneNode{Token: 59, Rune: '}'},
											},
										},
									},
								},
							},
							Close: &RuneNode{Token: 60, Rune: '}'},
						},
					},
				},
			},
		},
		CloseBracket: &RuneNode{Token: 61, Rune: ']'},
		Semicolon:    &RuneNode{Token: 62, Rune: ';'},
	},
}

func TestInspect(t *testing.T) {
	var tracker paths.AncestorTracker
	nodePaths := [][]Node{}
	pathStrings := []string{}

	Inspect(sampleTree1, func(n Node) bool {
		values := tracker.Values()
		nodePaths = append(nodePaths, paths.ValuesToNodes(values))
		pathStrings = append(pathStrings, values.Path.String())
		return true
	}, tracker.AsWalkOptions()...)

	root := sampleTree1
	root_keyword := root.GetKeyword()
	root_name := root.GetName()
	root_open := root.GetOpenBrace()
	root_0_opt := root.Decls[0].GetOption()
	root_0_opt_keyword := root_0_opt.GetKeyword()
	root_0_opt_name := root_0_opt.GetName()
	root_0_opt_name_0_ref := root_0_opt_name.GetParts()[0].GetFieldRef()
	root_0_opt_name_0_ref_open := root_0_opt_name_0_ref.GetOpen()
	root_0_opt_name_0_ref_name := root_0_opt_name_0_ref.GetName().GetCompoundIdent()
	root_0_opt_name_0_ref_name_0_ident := root_0_opt_name_0_ref_name.GetComponents()[0].GetIdent()
	root_0_opt_name_0_ref_name_1_dot := root_0_opt_name_0_ref_name.GetComponents()[1].GetDot()
	root_0_opt_name_0_ref_name_2_ident := root_0_opt_name_0_ref_name.GetComponents()[2].GetIdent()
	root_0_opt_name_0_ref_name_3_dot := root_0_opt_name_0_ref_name.GetComponents()[3].GetDot()
	root_0_opt_name_0_ref_name_4_ident := root_0_opt_name_0_ref_name.GetComponents()[4].GetIdent()
	root_0_opt_name_0_ref_close := root_0_opt_name_0_ref.GetClose()
	root_0_opt_name_1_dot := root_0_opt_name.GetParts()[1].GetDot()
	root_0_opt_name_2_ident := root_0_opt_name.GetParts()[2].GetIdent()
	root_0_opt_equals := root_0_opt.GetEquals()
	root_0_opt_val_string := root_0_opt.GetVal().GetStringLiteral()
	root_0_opt_semicolon := root_0_opt.GetSemicolon()
	root_close := root.GetCloseBrace()

	expectedNodePaths := [][]Node{
		{root},
		{root, root_keyword},
		{root, root_name},
		{root, root_open},
		{root, root_0_opt},
		{root, root_0_opt, root_0_opt_keyword},
		{root, root_0_opt, root_0_opt_name},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_0_ref},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_0_ref, root_0_opt_name_0_ref_open},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_0_ref, root_0_opt_name_0_ref_name},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_0_ref, root_0_opt_name_0_ref_name, root_0_opt_name_0_ref_name_0_ident},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_0_ref, root_0_opt_name_0_ref_name, root_0_opt_name_0_ref_name_1_dot},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_0_ref, root_0_opt_name_0_ref_name, root_0_opt_name_0_ref_name_2_ident},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_0_ref, root_0_opt_name_0_ref_name, root_0_opt_name_0_ref_name_3_dot},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_0_ref, root_0_opt_name_0_ref_name, root_0_opt_name_0_ref_name_4_ident},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_0_ref, root_0_opt_name_0_ref_close},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_1_dot},
		{root, root_0_opt, root_0_opt_name, root_0_opt_name_2_ident},
		{root, root_0_opt, root_0_opt_equals},
		{root, root_0_opt, root_0_opt_val_string},
		{root, root_0_opt, root_0_opt_semicolon},
		{root, root_close},
	}
	expectedPaths := []string{
		"(ast.MessageNode)",
		"(ast.MessageNode).keyword",
		"(ast.MessageNode).name",
		"(ast.MessageNode).openBrace",
		"(ast.MessageNode).decls[0].option",
		"(ast.MessageNode).decls[0].option.keyword",
		"(ast.MessageNode).decls[0].option.name",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.open",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[0].ident",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[1].dot",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[2].ident",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[3].dot",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent.components[4].ident",
		"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.close",
		"(ast.MessageNode).decls[0].option.name.parts[1].dot",
		"(ast.MessageNode).decls[0].option.name.parts[2].ident",
		"(ast.MessageNode).decls[0].option.equals",
		"(ast.MessageNode).decls[0].option.val.stringLiteral",
		"(ast.MessageNode).decls[0].option.semicolon",
		"(ast.MessageNode).closeBrace",
	}

	assert.Equal(t, len(expectedNodePaths), len(nodePaths))
	for i := range expectedNodePaths {
		for j := range expectedNodePaths[i] {
			if diff := cmp.Diff(expectedNodePaths[i][j], nodePaths[i][j], protocmp.Transform()); diff != "" {
				t.Errorf("unexpected node path at index (%d, %d) (-want +got):\n%s", i, j, diff)
			}
		}
	}

	assert.Equal(t, expectedPaths, pathStrings)
}

func TestFullAST(t *testing.T) {
	f, err := os.Open("../internal/testdata/desc_test_complex.proto")
	require.NoError(t, err)
	res, err := parser.Parse("../internal/testdata/desc_test_complex.proto", f, reporter.NewHandler(nil), 0)
	require.NoError(t, err)
	var tracker paths.AncestorTracker
	nodePaths := [][]Node{}
	pathStrings := []string{}

	Inspect(res, func(n Node) bool {
		values := tracker.Values()
		nodePaths = append(nodePaths, paths.ValuesToNodes(values))
		pathStrings = append(pathStrings, tracker.Path().String())
		return true
	}, tracker.AsWalkOptions()...)

	require.NotNil(t, nodePaths)
}

func TestBreak(t *testing.T) {
	cases := []struct {
		tree   Node
		stopAt []string
		want   []string
	}{
		{
			tree:   sampleTree1,
			stopAt: []string{"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef"},
			want: []string{
				"(ast.MessageNode)",
				"(ast.MessageNode).keyword",
				"(ast.MessageNode).name",
				"(ast.MessageNode).openBrace",
				"(ast.MessageNode).decls[0].option",
				"(ast.MessageNode).decls[0].option.keyword",
				"(ast.MessageNode).decls[0].option.name",
				"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef",
				"(ast.MessageNode).decls[0].option.name.parts[1].dot",
				"(ast.MessageNode).decls[0].option.name.parts[2].ident",
				"(ast.MessageNode).decls[0].option.equals",
				"(ast.MessageNode).decls[0].option.val.stringLiteral",
				"(ast.MessageNode).decls[0].option.semicolon",
				"(ast.MessageNode).closeBrace",
			},
		},
		{
			tree: sampleTree1,
			stopAt: []string{
				"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent",
				"(ast.MessageNode).keyword",
			},
			want: []string{
				"(ast.MessageNode)",
				"(ast.MessageNode).keyword",
				"(ast.MessageNode).name",
				"(ast.MessageNode).openBrace",
				"(ast.MessageNode).decls[0].option",
				"(ast.MessageNode).decls[0].option.keyword",
				"(ast.MessageNode).decls[0].option.name",
				"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef",
				"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.open",
				"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.name.compoundIdent",
				"(ast.MessageNode).decls[0].option.name.parts[0].fieldRef.close",
				"(ast.MessageNode).decls[0].option.name.parts[1].dot",
				"(ast.MessageNode).decls[0].option.name.parts[2].ident",
				"(ast.MessageNode).decls[0].option.equals",
				"(ast.MessageNode).decls[0].option.val.stringLiteral",
				"(ast.MessageNode).decls[0].option.semicolon",
				"(ast.MessageNode).closeBrace",
			},
		},
		{
			tree: sampleTree2,
			stopAt: []string{
				"(ast.FileNode).syntax",
				"(ast.FileNode).decls[0].message.decls[0].field.options",
				"(ast.FileNode).decls[1].enum",
			},
			want: []string{
				"(ast.FileNode)",
				"(ast.FileNode).syntax",
				"(ast.FileNode).decls[0].message",
				"(ast.FileNode).decls[0].message.keyword",
				"(ast.FileNode).decls[0].message.name",
				"(ast.FileNode).decls[0].message.openBrace",
				"(ast.FileNode).decls[0].message.decls[0].field",
				"(ast.FileNode).decls[0].message.decls[0].field.label",
				"(ast.FileNode).decls[0].message.decls[0].field.fieldType.ident",
				"(ast.FileNode).decls[0].message.decls[0].field.name",
				"(ast.FileNode).decls[0].message.decls[0].field.equals",
				"(ast.FileNode).decls[0].message.decls[0].field.tag",
				"(ast.FileNode).decls[0].message.decls[0].field.options",
				"(ast.FileNode).decls[1].enum",
			},
		},
		{
			tree: sampleTree3,
			stopAt: []string{
				"(ast.FieldNode).options.options[0].name",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].name",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral.elements[0].name",
			},
			want: []string{
				"(ast.FieldNode)",
				"(ast.FieldNode).label",
				"(ast.FieldNode).fieldType.ident",
				"(ast.FieldNode).name",
				"(ast.FieldNode).equals",
				"(ast.FieldNode).tag",
				"(ast.FieldNode).options",
				"(ast.FieldNode).options.openBracket",
				"(ast.FieldNode).options.options[0]",
				"(ast.FieldNode).options.options[0].name",
				"(ast.FieldNode).options.options[0].equals",
				"(ast.FieldNode).options.options[0].val.messageLiteral",
				"(ast.FieldNode).options.options[0].val.messageLiteral.open",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0]",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].name",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].sep",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral.open",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral.elements[0]",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral.elements[0].name",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral.elements[0].sep",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral.elements[0].val.messageLiteral",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral.elements[0].val.messageLiteral.open",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral.elements[0].val.messageLiteral.close",
				"(ast.FieldNode).options.options[0].val.messageLiteral.elements[0].val.messageLiteral.close",
				"(ast.FieldNode).options.options[0].val.messageLiteral.close",
				"(ast.FieldNode).options.closeBracket",
				"(ast.FieldNode).options.semicolon",
			},
		},
	}

	for i, c := range cases {
		var tracker paths.AncestorTracker
		paths := []string{}

		Inspect(c.tree, func(n Node) bool {
			pathStr := tracker.Path().String()
			paths = append(paths, pathStr)
			return !slices.Contains(c.stopAt, pathStr)
		}, tracker.AsWalkOptions()...)

		assert.Equal(t, c.want, paths, "case %d", i)
	}
}

func TestSkipExtensions(t *testing.T) {
	root := &FileNode{
		Syntax: &SyntaxNode{Keyword: &IdentNode{Token: 1, Val: "syntax"}},
	}
	proto.SetExtension(root, E_FileInfo, &FileInfo{Comments: []*FileInfo_CommentInfo{{Index: 1}}})

	visited := []Node{}
	Inspect(root, func(n Node) bool {
		visited = append(visited, n)
		return true
	})

	assert.Equal(t, []Node{root, root.Syntax, root.Syntax.Keyword}, visited)
}

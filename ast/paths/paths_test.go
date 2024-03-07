package paths_test

import (
	"testing"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/ast/paths"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protopath"
)

var root = &ast.FileNode{
	Syntax: &ast.SyntaxNode{
		Syntax: &ast.StringValueNode{
			Val: &ast.StringValueNode_StringLiteral{
				StringLiteral: &ast.StringLiteralNode{Token: 1},
			},
		},
	},
	Decls: []*ast.FileElement{
		{
			Val: &ast.FileElement_Message{
				Message: &ast.MessageNode{
					Keyword: &ast.IdentNode{Token: 2},
					Decls: []*ast.MessageElement{
						{
							Val: &ast.MessageElement_Field{
								Field: &ast.FieldNode{
									Options: &ast.CompactOptionsNode{
										Options: []*ast.OptionNode{
											{}, // empty option node
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
}

var values []protopath.Values

func TestMain(m *testing.M) {
	var tracker paths.AncestorTracker
	ast.Inspect(root, func(n ast.Node) bool {
		values = append(values, tracker.Values())
		return true
	}, tracker.AsWalkOptions()...)
	// {root}
	// {root, syntax}
	// {root, syntax, string}
	// {root, message}
	// {root, message, keyword}
	// {root, message, field}
	// {root, message, field, options}
	// {root, message, field, options, option}
	if len(values) != 8 {
		panic("unexpected number of values")
	}
	m.Run()
}

func TestSuffix2(t *testing.T) {
	{
		out, ok := paths.Suffix2[*ast.FileNode, *ast.SyntaxNode](values[1])
		require.True(t, ok)
		require.Equal(t, root, out.T)
		require.Equal(t, root.Syntax, out.U)
	}

	{
		out, ok := paths.Suffix2[*ast.SyntaxNode, *ast.StringLiteralNode](values[2])
		require.True(t, ok)
		require.Equal(t, root.Syntax, out.T)
		require.Equal(t, root.Syntax.Syntax.GetStringLiteral(), out.U)
	}

	{
		out, ok := paths.Suffix2[*ast.StringValueNode, *ast.StringLiteralNode](values[2])
		require.True(t, ok)
		require.Equal(t, root.Syntax.Syntax, out.T)
		require.Equal(t, root.Syntax.Syntax.GetStringLiteral(), out.U)
	}

	{
		out, ok := paths.Suffix2[*ast.FileNode, *ast.MessageNode](values[3])
		require.True(t, ok)
		require.Equal(t, root, out.T)
		require.Equal(t, root.Decls[0].GetMessage(), out.U)
	}

	{
		out, ok := paths.Suffix2[*ast.MessageNode, *ast.IdentNode](values[4])
		require.True(t, ok)
		require.Equal(t, root.Decls[0].GetMessage(), out.T)
		require.Equal(t, root.Decls[0].GetMessage().Keyword, out.U)
	}

	{
		out, ok := paths.Suffix2[*ast.MessageNode, *ast.FieldNode](values[5])
		require.True(t, ok)
		require.Equal(t, root.Decls[0].GetMessage(), out.T)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField(), out.U)
	}

	{
		out, ok := paths.Suffix2[*ast.FieldNode, *ast.CompactOptionsNode](values[6])
		require.True(t, ok)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField(), out.T)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField().Options, out.U)
	}

	{
		out, ok := paths.Suffix2[*ast.CompactOptionsNode, *ast.OptionNode](values[7])
		require.True(t, ok)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField().Options, out.T)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField().Options.Options[0], out.U)
	}

	{
		_, ok := paths.Suffix2[*ast.FileNode, *ast.SyntaxNode](protopath.Values{})
		require.False(t, ok)
	}

	{
		_, ok := paths.Suffix2[*ast.FileNode, *ast.SyntaxNode](values[0])
		require.False(t, ok)
	}

	for _, v := range values {
		_, ok := paths.Suffix2[*ast.FileNode, *ast.StringLiteralNode](v)
		require.False(t, ok)
		_, ok = paths.Suffix2[*ast.SyntaxNode, *ast.FileNode](v)
		require.False(t, ok)
		_, ok = paths.Suffix2[*ast.FileNode, *ast.IdentNode](v)
		require.False(t, ok)
		_, ok = paths.Suffix2[*ast.FieldNode, *ast.OptionNode](v)
		require.False(t, ok)
	}

	require.Panics(t, func() {
		paths.Suffix2[*ast.FileNode, *ast.FileElement](paths.Slice(values[4], 1, 3))
	})
}

func TestSuffix3(t *testing.T) {
	{
		out, ok := paths.Suffix3[*ast.FileNode, *ast.SyntaxNode, *ast.StringLiteralNode](values[2])
		require.True(t, ok)
		require.Equal(t, root, out.T)
		require.Equal(t, root.Syntax, out.U)
		require.Equal(t, root.Syntax.Syntax.GetStringLiteral(), out.V)
	}

	{
		out, ok := paths.Suffix3[*ast.FileNode, *ast.MessageNode, *ast.IdentNode](values[4])
		require.True(t, ok)
		require.Equal(t, root, out.T)
		require.Equal(t, root.Decls[0].GetMessage(), out.U)
		require.Equal(t, root.Decls[0].GetMessage().Keyword, out.V)
	}

	{
		out, ok := paths.Suffix3[*ast.FileNode, *ast.MessageNode, *ast.FieldNode](values[5])
		require.True(t, ok)
		require.Equal(t, root, out.T)
		require.Equal(t, root.Decls[0].GetMessage(), out.U)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField(), out.V)
	}

	{
		out, ok := paths.Suffix3[*ast.MessageNode, *ast.FieldNode, *ast.CompactOptionsNode](values[6])
		require.True(t, ok)
		require.Equal(t, root.Decls[0].GetMessage(), out.T)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField(), out.U)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField().Options, out.V)
	}

	{
		out, ok := paths.Suffix3[*ast.FieldNode, *ast.CompactOptionsNode, *ast.OptionNode](values[7])
		require.True(t, ok)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField(), out.T)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField().Options, out.U)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField().Options.Options[0], out.V)
	}

	{
		_, ok := paths.Suffix3[*ast.FileNode, *ast.MessageNode, *ast.OptionNode](values[7])
		require.False(t, ok)
	}

	{
		_, ok := paths.Suffix3[*ast.FileNode, *ast.EnumNode, *ast.OptionNode](values[7])
		require.False(t, ok)
	}
}

func TestSuffix4(t *testing.T) {
	{
		out, ok := paths.Suffix4[*ast.FileNode, *ast.MessageNode, *ast.FieldNode, *ast.CompactOptionsNode](values[6])
		require.True(t, ok)
		require.Equal(t, root, out.T)
		require.Equal(t, root.Decls[0].GetMessage(), out.U)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField(), out.V)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField().Options, out.W)
	}

	{
		out, ok := paths.Suffix4[*ast.MessageNode, *ast.FieldNode, *ast.CompactOptionsNode, *ast.OptionNode](values[7])
		require.True(t, ok)
		require.Equal(t, root.Decls[0].GetMessage(), out.T)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField(), out.U)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField().Options, out.V)
		require.Equal(t, root.Decls[0].GetMessage().Decls[0].GetField().Options.Options[0], out.W)
	}
}

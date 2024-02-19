package ast_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/parser"
	"github.com/kralicky/protocompile/reporter"
	"github.com/stretchr/testify/require"
)

func TestClone(t *testing.T) {
	data, _ := os.ReadFile("../internal/testdata/desc_test_complex.proto")
	root, err := parser.Parse("desc_test_complex.proto", bytes.NewReader(data), reporter.NewHandler(nil), 0)
	require.NoError(t, err)
	for _, decl := range root.Decls {
		clone := ast.Clone(decl)
		// filter NaNs
		if !cmp.Equal(decl, clone, cmp.Comparer(floatCompare)) {
			t.Error(cmp.Diff(decl, clone))
		}
	}
}

func floatCompare(x, y float64) bool {
	return x == y || (x != x && y != y)
}

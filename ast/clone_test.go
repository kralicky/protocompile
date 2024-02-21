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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestClone(t *testing.T) {
	data, _ := os.ReadFile("../internal/testdata/desc_test_complex.proto")
	root, err := parser.Parse("desc_test_complex.proto", bytes.NewReader(data), reporter.NewHandler(nil), 0)
	require.NoError(t, err)
	clone := ast.Clone(root)
	// filter NaNs
	if !cmp.Equal(root, clone, protocmp.Transform(), cmp.Comparer(floatCompare)) {
		t.Error(cmp.Diff(root, clone))
	}
	fileInfo := proto.GetExtension(root, ast.E_FileInfo).(*ast.FileInfo)
	cloneFileInfo := proto.GetExtension(clone, ast.E_FileInfo).(*ast.FileInfo)
	if fileInfo != cloneFileInfo {
		t.Error("FileInfo pointers not equal")
	}
}

func floatCompare(x, y float64) bool {
	return x == y || (x != x && y != y)
}

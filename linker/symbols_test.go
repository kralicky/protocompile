// Copyright 2020-2023 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package linker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/reporter"
)

func TestSymbolsPackages(t *testing.T) {
	t.Parallel()

	var s Symbols
	// default/nameless package is the root
	assert.Equal(t, &s.pkgTrie, s.getPackage(""))

	h := reporter.NewHandler(nil)
	pos := ast.UnknownPos("foo.proto")
	pkg, err := s.importPackages(pos, "build.buf.foo.bar.baz", h)
	require.NoError(t, err)
	// new package has nothing in it
	assert.Empty(t, pkg.children)
	assert.Empty(t, pkg.files)
	assert.Empty(t, pkg.symbols)
	assert.Empty(t, pkg.exts)

	assert.Equal(t, pkg, s.getPackage("build.buf.foo.bar.baz"))

	// verify that trie was created correctly:
	//   each package has just one entry, which is its immediate sub-package
	cur := &s.pkgTrie
	pkgNames := []protoreflect.FullName{"build", "build.buf", "build.buf.foo", "build.buf.foo.bar", "build.buf.foo.bar.baz"}
	for _, pkgName := range pkgNames {
		assert.Equal(t, 1, len(cur.children))
		assert.Empty(t, cur.files)
		assert.Equal(t, 1, len(cur.symbols))
		assert.Empty(t, cur.exts)

		entry, ok := cur.symbols[pkgName]
		require.True(t, ok)
		assert.Equal(t, pos, entry.pos)
		assert.False(t, entry.isEnumValue)
		assert.True(t, entry.isPackage)

		next, ok := cur.children[pkgName]
		require.True(t, ok)
		require.NotNil(t, next)

		cur = next
	}
	assert.Equal(t, pkg, cur)
}

func TestSymbolsImport(t *testing.T) {
	t.Parallel()

	fileAsResult := parseAndLink(t, `
		syntax = "proto2";
		import "google/protobuf/descriptor.proto";
		package foo.bar;
		message Foo {
			optional string bar = 1;
			repeated int32 baz = 2;
			extensions 10 to 20;
		}
		extend Foo {
			optional float f = 10;
			optional string s = 11;
		}
		extend google.protobuf.FieldOptions {
			optional bytes xtra = 20000;
		}
		`)

	fileAsNonResult, err := protodesc.NewFile(fileAsResult.FileDescriptorProto(), protoregistry.GlobalFiles)
	require.NoError(t, err)

	h := reporter.NewHandler(nil)
	testCases := map[string]protoreflect.FileDescriptor{
		"linker.Result":               fileAsResult,
		"protoreflect.FileDescriptor": fileAsNonResult,
	}

	for name, fd := range testCases {
		fd := fd
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var s Symbols
			err := s.Import(fd, h)
			require.NoError(t, err)

			// verify contents of s

			pkg := s.getPackage("foo.bar")
			syms := pkg.symbols
			assert.Equal(t, 6, len(syms))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.Foo"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.Foo.bar"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.Foo.baz"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.f"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.s"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.xtra"))
			exts := pkg.exts
			assert.Equal(t, 2, len(exts))
			assert.Contains(t, exts, extNumber{"foo.bar.Foo", 10})
			assert.Contains(t, exts, extNumber{"foo.bar.Foo", 11})

			pkg = s.getPackage("google.protobuf")
			exts = pkg.exts
			assert.Equal(t, 1, len(exts))
			assert.Contains(t, exts, extNumber{"google.protobuf.FieldOptions", 20000})
		})
	}
}

func TestSymbolExtensions(t *testing.T) {
	t.Parallel()

	var s Symbols

	_, err := s.importPackages(ast.UnknownPos("foo.proto"), "foo.bar", reporter.NewHandler(nil))
	require.NoError(t, err)
	_, err = s.importPackages(ast.UnknownPos("google/protobuf/descriptor.proto"), "google.protobuf", reporter.NewHandler(nil))
	require.NoError(t, err)

	addExt := func(pkg, extendee protoreflect.FullName, num protoreflect.FieldNumber) error {
		return s.AddExtension(pkg, extendee, num, ast.UnknownPos("foo.proto"), reporter.NewHandler(nil))
	}

	t.Run("mismatch", func(t *testing.T) {
		t.Parallel()
		err := addExt("bar.baz", "foo.bar.Foo", 11)
		require.ErrorContains(t, err, "does not match package")
	})
	t.Run("missing package", func(t *testing.T) {
		t.Parallel()
		err := addExt("bar.baz", "bar.baz.Bar", 11)
		require.ErrorContains(t, err, "missing package symbols")
	})

	err = addExt("foo.bar", "foo.bar.Foo", 11)
	require.NoError(t, err)
	err = addExt("foo.bar", "foo.bar.Foo", 12)
	require.NoError(t, err)

	err = addExt("foo.bar", "foo.bar.Foo", 11)
	require.ErrorContains(t, err, "already defined")

	err = addExt("google.protobuf", "google.protobuf.FileOptions", 10101)
	require.NoError(t, err)
	err = addExt("google.protobuf", "google.protobuf.FieldOptions", 10101)
	require.NoError(t, err)
	err = addExt("google.protobuf", "google.protobuf.MessageOptions", 10101)
	require.NoError(t, err)

	// verify contents of s

	pkg := s.getPackage("foo.bar")
	exts := pkg.exts
	assert.Equal(t, 2, len(exts))
	assert.Contains(t, exts, extNumber{"foo.bar.Foo", 11})
	assert.Contains(t, exts, extNumber{"foo.bar.Foo", 12})

	pkg = s.getPackage("google.protobuf")
	exts = pkg.exts
	assert.Equal(t, 3, len(exts))
	assert.Contains(t, exts, extNumber{"google.protobuf.FileOptions", 10101})
	assert.Contains(t, exts, extNumber{"google.protobuf.FieldOptions", 10101})
	assert.Contains(t, exts, extNumber{"google.protobuf.MessageOptions", 10101})
}

func parseAndLink(t *testing.T, contents string) Result {
	t.Helper()
	h := reporter.NewHandler(nil)
	fileAst, err := parser.Parse("test.proto", strings.NewReader(contents), h)
	require.NoError(t, err)
	parseResult, err := parser.ResultFromAST(fileAst, true, h)
	require.NoError(t, err)
	dep, err := protoregistry.GlobalFiles.FindFileByPath("google/protobuf/descriptor.proto")
	require.NoError(t, err)
	depAsFile, err := NewFileRecursive(dep)
	require.NoError(t, err)
	depFiles := Files{depAsFile}
	linkResult, err := Link(parseResult, depFiles, nil, h)
	require.NoError(t, err)
	return linkResult
}

func TestDelete(t *testing.T) {
	// define the content for each of the test protobuf files
	files := []string{
		`
		syntax = "proto3";
		package test1;
		message Test1 {
			string field1 = 1;
			int32 field2 = 2;
			repeated string field3 = 3;
		}
		`,
		`
		syntax = "proto3";
		package test2.subtest;
		import "test1.proto";
		message Test2 {
			test1.Test1 field1 = 1;
			string field2 = 2;
			repeated string field3 = 3;
		}
		`,
		`
		syntax = "proto3";
		package test3.subtest.part3;
		import "test2.proto";
		message Test3 {
			test2.subtest.Test2 field1 = 1;
			string field2 = 2;
			repeated string field3 = 3;
		}
		`,
		`
		syntax = "proto3";
		package test4.part4;
		import "test3.proto";
		message Test4 {
			test3.subtest.part3.Test3 field1 = 1;
			string field2 = 2;
			repeated string field3 = 3;
		}
		`,
		`
		syntax = "proto3";
		package test5.sub5;
		import "test4.proto";
		message Test5 {
			test4.part4.Test4 field1 = 1;
			string field2 = 2;
			repeated string field3 = 3;
		}
		`,
	}
	h := reporter.NewHandler(nil)

	parseAndLinkNamed := func(name, contents string, prevDeps ...File) Result {
		t.Helper()
		fileAst, err := parser.Parse(name, strings.NewReader(contents), h)
		require.NoError(t, err)
		parseResult, err := parser.ResultFromAST(fileAst, true, h)
		require.NoError(t, err)
		dep, err := protoregistry.GlobalFiles.FindFileByPath("google/protobuf/descriptor.proto")
		require.NoError(t, err)
		depAsFile, err := NewFileRecursive(dep)
		require.NoError(t, err)
		depFiles := Files{depAsFile}
		depFiles = append(depFiles, prevDeps...)
		linkResult, err := Link(parseResult, depFiles, nil, h)
		require.NoError(t, err)
		return linkResult
	}

	fds := make(map[string]protoreflect.FileDescriptor, len(files))
	filenames := make([]string, 0, len(files))

	linkedFiles := make([]File, 0, len(files))
	for i, content := range files {
		name := fmt.Sprintf("test%d.proto", i+1)
		res := parseAndLinkNamed(name, content, linkedFiles...)
		linkedFiles = append(linkedFiles, res)
		fds[name] = res
		filenames = append(filenames, name)
	}

	var symtab Symbols

	// import each file and record the state of the symbol table
	states := make([]*Symbols, 0, len(files))
	for _, filename := range filenames {
		err := symtab.Import(fds[filename], h)
		require.NoError(t, err)
		states = append(states, symtab.Clone())
	}

	// delete each file and verify that the state of the symbol table is restored to the previous state
	for i := len(filenames) - 1; i >= 0; i-- {
		err := symtab.Delete(fds[filenames[i]], h)
		require.NoError(t, err)

		// if this is the last file, the symbol table should be empty
		if i == 0 {
			requireSymbolsEmpty(t, &symtab)
		} else {
			// otherwise, the symbol table should match the state recorded after the previous file was imported
			requireSymbolsEqual(t, &symtab, states[i-1])
		}
	}
}

func requireSymbolsEmpty(t *testing.T, s *Symbols) {
	t.Helper()
	require.Empty(t, s.pkgTrie.children)
	require.Empty(t, s.pkgTrie.exts)
	require.Empty(t, s.pkgTrie.files)
	require.Empty(t, s.pkgTrie.symbols)
}

func requireSymbolsEqual(t *testing.T, a, b *Symbols) {
	requirePackageSymbolsEqual(t, &a.pkgTrie, &b.pkgTrie)
}

func requirePackageSymbolsEqual(t *testing.T, a, b *packageSymbols) {
	for k, v := range a.children {
		requirePackageSymbolsEqual(t, v, b.children[k])
	}
	requireEquivalent(t, a.exts, b.exts)
	requireEquivalent(t, a.files, b.files)
	requireEquivalent(t, a.symbols, b.symbols)
}

func requireEquivalent(t *testing.T, expected, actual any) {
	t.Helper()
	diff := cmp.Diff(expected, actual, cmp.AllowUnexported(packageSymbols{}, symbolEntry{}), cmpopts.EquateEmpty(), protocmp.Transform())
	if diff != "" {
		t.Errorf(diff)
	}
}

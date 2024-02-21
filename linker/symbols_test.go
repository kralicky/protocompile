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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/parser"
	"github.com/kralicky/protocompile/reporter"
)

func TestSymbolsPackages(t *testing.T) {
	t.Parallel()

	s := NewSymbolTable()
	// default/nameless package is the root
	assert.Equal(t, &s.pkgTrie, s.getPackage(""))

	h := reporter.NewHandler(nil)
	span := ast.UnknownSpan("foo.proto")
	pkg, err := s.importPackages(span, "build.buf.foo.bar.baz", h)
	require.NoError(t, err)
	// new package has nothing in it
	assert.Empty(t, pkg.children)
	assert.Empty(t, pkg.symbols)
	assert.Empty(t, pkg.exts)

	assert.Equal(t, pkg, s.getPackage("build.buf.foo.bar.baz"))

	// verify that trie was created correctly:
	//   each package has just one entry, which is its immediate sub-package
	cur := &s.pkgTrie
	pkgNames := []protoreflect.FullName{"build", "build.buf", "build.buf.foo", "build.buf.foo.bar", "build.buf.foo.bar.baz"}
	for _, pkgName := range pkgNames {
		assert.Len(t, cur.children, 1)
		assert.Len(t, cur.symbols, 1)
		assert.Empty(t, cur.exts)

		entry, ok := cur.symbols[pkgName]
		require.True(t, ok)
		assert.Equal(t, span, entry.span)
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

			s := NewSymbolTable()
			err := s.Import(fd, h)
			require.NoError(t, err)

			// verify contents of s

			pkg := s.getPackage("foo.bar")
			syms := pkg.symbols
			assert.Len(t, syms, 6)
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.Foo"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.Foo.bar"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.Foo.baz"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.f"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.s"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.xtra"))
			exts := pkg.exts
			assert.Len(t, exts, 2)
			assert.Contains(t, exts, extNumber{"foo.bar.Foo", 10})
			assert.Contains(t, exts, extNumber{"foo.bar.Foo", 11})

			pkg = s.getPackage("google.protobuf")
			exts = pkg.exts
			assert.Len(t, exts, 1)
			assert.Contains(t, exts, extNumber{"google.protobuf.FieldOptions", 20000})
		})
	}
}

func TestSymbolExtensions(t *testing.T) {
	t.Parallel()

	s := NewSymbolTable()

	_, err := s.importPackages(ast.UnknownSpan("foo.proto"), "foo.bar", reporter.NewHandler(nil))
	require.NoError(t, err)
	_, err = s.importPackages(ast.UnknownSpan("google/protobuf/descriptor.proto"), "google.protobuf", reporter.NewHandler(nil))
	require.NoError(t, err)

	addExt := func(pkg, extendee protoreflect.FullName, num protoreflect.FieldNumber) error {
		return s.AddExtension(pkg, extendee, num, ast.UnknownSpan("foo.proto"), reporter.NewHandler(nil))
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
	assert.Len(t, exts, 2)
	assert.Contains(t, exts, extNumber{"foo.bar.Foo", 11})
	assert.Contains(t, exts, extNumber{"foo.bar.Foo", 12})

	pkg = s.getPackage("google.protobuf")
	exts = pkg.exts
	assert.Len(t, exts, 3)
	assert.Contains(t, exts, extNumber{"google.protobuf.FileOptions", 10101})
	assert.Contains(t, exts, extNumber{"google.protobuf.FieldOptions", 10101})
	assert.Contains(t, exts, extNumber{"google.protobuf.MessageOptions", 10101})
}

func parseAndLink(t *testing.T, contents string) Result {
	t.Helper()
	h := reporter.NewHandler(nil)
	fileAst, err := parser.Parse("test.proto", strings.NewReader(contents), h, 0)
	require.NoError(t, err)
	parseResult, err := parser.ResultFromAST(fileAst, true, h)
	require.NoError(t, err)
	dep, err := protoregistry.GlobalFiles.FindFileByPath("google/protobuf/descriptor.proto")
	require.NoError(t, err)
	depAsFile, err := NewFile(dep, nil)
	require.NoError(t, err)
	depFiles := Files{depAsFile}
	linkResult, err := Link(parseResult, depFiles, nil, h)
	require.NoError(t, err)
	return linkResult
}

func TestImportAndDelete(t *testing.T) {
	// define the content for each of the test protobuf files
	files := []string{
		0: `
		syntax = "proto3";
		package test1;
		message Test1 {
			string field1 = 1;
			int32 field2 = 2;
			repeated string field3 = 3;
		}
		`,

		1: `
		syntax = "proto3";
		package test2.subtest;
		import "test1.proto";
		import "google/protobuf/descriptor.proto";
		message Test2 {
			test1.Test1 field1 = 1;
			string field2 = 2;
			repeated string field3 = 3;
		}
		`,

		2: `
		syntax = "proto3";
		package test3.subtest.part3;
		import "test2.proto";
		import "google/protobuf/descriptor.proto";
		message Test3 {
			test2.subtest.Test2 field1 = 1;
			string field2 = 2;
			repeated string field3 = 3;
		}

		extend google.protobuf.MessageOptions {
			optional string example = 10001;
		}
		`,

		3: `
		syntax = "proto3";
		package test4.part4;
		import "test3.proto";
		message Test4 {
			test3.subtest.part3.Test3 field1 = 1;
			string field2 = 2;
			repeated string field3 = 3;
		}
		`,

		4: `
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

	ts := newTestSymtab(t)
	ts.parseAndLinkNamed(t, "test1.proto", files[0])
	ts.parseAndLinkNamed(t, "test2.proto", files[1])
	ts.parseAndLinkNamed(t, "test3.proto", files[2])
	ts.parseAndLinkNamed(t, "test4.proto", files[3])
	ts.parseAndLinkNamed(t, "test5.proto", files[4])

	ts.run()
}

func TestSymbolPackageCollision(t *testing.T) {
	files := []string{
		0: `
syntax = "proto2";
package foo.bar;
`,

		1: `
syntax = "proto2";

import "google/protobuf/descriptor.proto";
extend google.protobuf.FieldOptions {
	optional string foo = 10001;
}`,

		2: `
syntax = "proto2";

import "google/protobuf/descriptor.proto";
extend google.protobuf.FieldOptions {
	optional string bar = 10001;
}`,
	}

	ts := newTestSymtab(t)
	ts.parseAndLinkNamed(t, "f1.proto", files[0])
	ts.parseAndLinkNamed(t, "f2.proto", files[1], func(err error) bool {
		require.Error(t, err)
		require.Contains(t, err.Error(), "foo redeclared in this block")
		return true
	})
	ts.parseAndLinkNamed(t, "f3.proto", files[2])
	ts.run()
}

func TestPackageSymbolCollision(t *testing.T) {
	files := []string{
		0: `
syntax = "proto2";

import "google/protobuf/descriptor.proto";
extend google.protobuf.FieldOptions {
	optional string foo = 10001;
}`,
		1: `
syntax = "proto2";
package foo.bar;
`,
		2: `
syntax = "proto2";

import "google/protobuf/descriptor.proto";
extend google.protobuf.FieldOptions {
	optional string foo = 10001;
}`,
	}

	ts := newTestSymtab(t)
	ts.parseAndLinkNamed(t, "f1.proto", files[0])
	ts.parseAndLinkNamed(t, "f2.proto", files[1], func(err error) bool {
		require.Error(t, err)
		require.Contains(t, err.Error(), "foo redeclared in this block")
		return true
	})
	ts.parseAndLinkNamed(t, "f3.proto", files[2], func(err error) bool {
		require.Error(t, err)
		require.Contains(t, err.Error(), "foo redeclared in this block")
		return true
	})
	ts.run()
}

func TestSymbolCollision(t *testing.T) {
	files := []string{
		0: `
syntax = "proto2";
package testprotos;

message Test {
	optional string foo = 1;
}`,
		1: `
syntax = "proto2";
package testprotos;

import "test1.proto";
message Test2 {
	optional Test foo = 1;
}
`,
		2: `
syntax = "proto2";
package testprotos;

import "test1.proto";
message Test2 {
	optional Test foo = 1;
}

message Test{}
`,
	}

	ts := newTestSymtab(t)
	ts.parseAndLinkNamed(t, "test1.proto", files[0])
	ts.parseAndLinkNamed(t, "test2.proto", files[1])
	ts.runF(func(s *Symbols, h *reporter.Handler) {
		err := s.Delete(ts.fds["test2.proto"], h)
		require.NoError(t, err)
		ts.parseAndLinkNamed(t, "test2.proto", files[2], func(err error) bool {
			require.Error(t, err)
			require.Contains(t, err.Error(), "testprotos.Test redeclared in this block")
			return true
		})
		ts.parseAndLinkNamed(t, "test2.proto", files[1])
	})
}

type tempSymtab struct {
	sym       *Symbols
	t         *testing.T
	fds       map[string]File
	filenames []string
}

func newTestSymtab(t *testing.T) *tempSymtab {
	ts := &tempSymtab{
		sym: NewSymbolTable(),
		fds: make(map[string]File),
		t:   t,
	}

	h := reporter.NewHandler(nil)
	dep, err := protoregistry.GlobalFiles.FindFileByPath("google/protobuf/descriptor.proto")
	require.NoError(t, err)
	ts.sym.Import(dep, h)
	ts.fds["google/protobuf/descriptor.proto"], err = NewFile(dep, nil)
	require.NoError(t, err)

	return ts
}

func (ts *tempSymtab) parseAndLinkNamed(t *testing.T, name, contents string, handleLinkErr ...func(error) (cont bool)) {
	h := reporter.NewHandler(nil)
	fileAst, err := parser.Parse(name, strings.NewReader(contents), h, 0)
	require.NoError(t, err)
	parseResult, err := parser.ResultFromAST(fileAst, true, h)
	require.NoError(t, err)

	var depFiles Files
	for _, decl := range parseResult.AST().Decls {
		if imp := decl.GetImport(); imp != nil {
			f, ok := ts.fds[imp.Name.AsString()]
			require.True(t, ok)
			depFiles = append(depFiles, f)
		}
	}
	symClone := ts.sym.Clone()
	linkResult, err := Link(parseResult, depFiles, symClone, h)
	if err != nil {
		if len(handleLinkErr) > 0 {
			if handleLinkErr[0](err) {
				return
			}
		}
	}
	require.NoError(t, err)
	ts.sym = symClone
	ts.fds[name] = linkResult
	ts.filenames = append(ts.filenames, name)

	require.NoError(t, h.Error())
}

func (ts *tempSymtab) run() {
	h := reporter.NewHandler(nil)
	symtab := NewSymbolTable()

	for range 10 {
		states := make([]*Symbols, 0, len(ts.filenames))
		// import each file and record the state of the symbol table
		for _, filename := range ts.filenames {
			beforeImport := symtab.Clone()
			err := symtab.Import(ts.fds[filename], h)
			afterImport := symtab.Clone()
			require.NoError(ts.t, err)
			err = symtab.Delete(ts.fds[filename], h)
			require.NoError(ts.t, err)
			requireSymbolsEqual(ts.t, beforeImport, symtab)
			err = symtab.Import(ts.fds[filename], h)
			require.NoError(ts.t, err)
			requireSymbolsEqual(ts.t, afterImport, symtab)
			states = append(states, symtab.Clone())

			require.NoError(ts.t, h.Error())
		}

		// delete each file and verify that the state of the symbol table is restored to the previous state
		for i := len(ts.filenames) - 1; i >= 0; i-- {
			err := symtab.Delete(ts.fds[ts.filenames[i]], h)
			require.NoError(ts.t, err)

			// if this is the last file, the symbol table should be empty
			if i == 0 {
				requireSymbolsEmpty(ts.t, symtab)
			} else {
				// otherwise, the symbol table should match the state recorded after the previous file was imported
				requireSymbolsEqual(ts.t, states[i-1], symtab)
			}
		}
	}

	requireSymbolsEmpty(ts.t, symtab)
}

func (ts *tempSymtab) runF(fn func(s *Symbols, h *reporter.Handler)) {
	h := reporter.NewHandler(nil)
	fn(ts.sym, h)
	require.NoError(ts.t, h.Error())
}

func requireSymbolsEmpty(t *testing.T, s *Symbols) {
	t.Helper()
	require.Empty(t, s.pkgTrie.children)
	require.Empty(t, s.pkgTrie.exts)
	require.Empty(t, s.pkgTrie.symbols)
}

func requireSymbolsEqual(t *testing.T, expected, actual *Symbols) {
	requirePackageSymbolsEqual(t, &expected.pkgTrie, &actual.pkgTrie)
}

func requirePackageSymbolsEqual(t *testing.T, expected, actual *packageSymbols) {
	for k, v := range expected.children {
		if bc, ok := actual.children[k]; !ok {
			t.Fatalf("package %s not found in b\n----------\nexpected:\n%+v\n----------\nactual:\n%+v\n", k, expected.MarshalText(), actual.MarshalText()) // todo: handle transitive imports like google.protobuf.descriptor when deleting
			continue
		} else {
			requirePackageSymbolsEqual(t, v, bc)
		}
	}
	requireEquivalent(t, expected.exts, actual.exts)
	requireEquivalent(t, expected.symbols, actual.symbols)
}

func requireEquivalent(t *testing.T, expected, actual any) {
	t.Helper()
	if !assert.Equal(t, expected, actual) {
		t.Fatalf("expected:\n%s\nactual:\n%s\n", expected, actual)
	}
	// diff := cmp.Diff(expected, actual, cmp.AllowUnexported(packageSymbols{}, symbolEntry{}, extNumber{}, file{}, ast.FileInfo{}), cmpopts.EquateEmpty(), protocmp.Transform())
	// if diff != "" {
	// 	t.Errorf(diff)
	// }
}

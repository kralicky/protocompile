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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

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
	pos := ast.UnknownSpan("foo.proto")
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
		assert.Equal(t, pos, entry.span)
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
	depAsFile, err := NewFile(dep, nil)
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
		import "google/protobuf/descriptor.proto";
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
		`
	syntax = "proto2";

	package foo.bar;

	option go_package = "github.com/bufbuild/protocompile/internal/testprotos";

	import "google/protobuf/descriptor.proto";

	message Simple {
		optional string name = 1;
		optional uint64 id = 2;
		optional bytes _extra = 3; // default JSON name will be capitalized
		repeated bool _ = 4; // default JSON name will be empty(!)
	}

	extend . google. // identifier broken up strangely should still be accepted
		protobuf .
		ExtensionRangeOptions {
		optional string label = 20000;
	}

	message Test {
		optional string foo = 1 [json_name = "|foo|"];
		repeated int32 array = 2;
		optional Simple s = 3;
		repeated Simple r = 4;
		map<string, int32> m = 5;

		optional bytes b = 6 [default = "\0\1\2\3\4\5\6\7fubar!"];

		extensions 100 to 200;

		extensions 249, 300 to 350, 500 to 550, 20000 to max [(label) = "jazz"];

		message Nested {
			extend google.protobuf.MessageOptions {
				optional int32 fooblez = 20003;
			}
			message _NestedNested {
				enum EEE {
					OK = 0;
					V1 = 1;
					V2 = 2;
					V3 = 3;
					V4 = 4;
					V5 = 5;
					V6 = 6;
				}
				option (fooblez) = 10101;
				extend Test {
					optional string _garblez = 100;
				}
				option (rept) = { foo: "goo" [foo.bar.Test.Nested._NestedNested._garblez]: "boo" };
				message NestedNestedNested {
					option (rept) = { foo: "hoo" [Test.Nested._NestedNested._garblez]: "spoo" };

					optional Test Test = 1;
				}
			}
		}
	}

	enum EnumWithReservations {
		X = 2;
		Y = 3;
		Z = 4;
		reserved 1000 to max;
		reserved -2 to 1;
		reserved 5 to 10, 12 to 15, 18;
		reserved -5 to -3;
		reserved "C", "B", "A";
	}

	message MessageWithReservations {
		reserved 5 to 10, 12 to 15, 18;
		reserved 1000 to max;
		reserved "A", "B", "C";
	}

	message MessageWithMap {
		map<string, Simple> vals = 1;
	}

	extend google.protobuf.MessageOptions {
		repeated Test rept = 20002;
		optional Test.Nested._NestedNested.EEE eee = 20010;
		optional Another a = 20020;
		optional MessageWithMap map_vals = 20030;
	}

	message Another {
			option (.foo.bar.rept) = { foo: "abc" s < name: "foo", id: 123 >, array: [1, 2 ,3], r:[<name:"f">, {name:"s"}, {id:456} ], };
			option (foo.bar.rept) = { foo: "def" s { name: "bar", id: 321 }, array: [3, 2 ,1], r:{name:"g"} r:{name:"s"}};
			option (rept) = { foo: "def" };
			option (eee) = V1;
		option (a) = { fff: OK };
		option (a).test = { m { key: "foo" value: 100 } m { key: "bar" value: 200 }};
		option (a).test.foo = "m&m";
		option (a).test.s.name = "yolo";
			option (a).test.s.id = 98765;
			option (a).test.array = 1;
			option (a).test.array = 2;
			option (a).test.(.foo.bar.Test.Nested._NestedNested._garblez) = "whoah!";

		option (map_vals).vals = {}; // no key, no value
		option (map_vals).vals = {key: "foo"}; // no value
		option (map_vals).vals = {key: "bar", value: {name: "baz"}};

			optional Test test = 1;
			optional Test.Nested._NestedNested.EEE fff = 2 [default = V1];
	}

	message Validator {
		optional bool authenticated = 1;

		enum Action {
			LOGIN = 0;
			READ = 1;
			WRITE = 2;
		}
		message Permission {
			optional Action action = 1;
			optional string entity = 2;
		}

		repeated Permission permission = 2;
	}

	extend google.protobuf.MethodOptions {
		optional Validator validator = 12345;
	}

	service TestTestService {
		rpc UserAuth(Test) returns (Test) {
			option (validator) = {
				authenticated: true
				permission: {
					action: LOGIN
					entity: "client"
				}
			};
		}
		rpc Get(Test) returns (Test) {
			option (validator) = {
				authenticated: true
				permission: {
					action: READ
					entity: "user"
				}
			};
		}
	}

	message Rule {
		message StringRule {
			optional string pattern = 1;
			optional bool allow_empty = 2;
			optional int32 min_len = 3;
			optional int32 max_len = 4;
		}
		message IntRule {
			optional int64 min_val = 1;
			optional uint64 max_val = 2;
		}
		message RepeatedRule {
			optional bool allow_empty = 1;
			optional int32 min_items = 2;
			optional int32 max_items = 3;
			optional Rule items = 4;
		}
		oneof rule {
			StringRule string = 1;
			RepeatedRule repeated = 2;
			IntRule int = 3;
		group FloatRule = 4 {
			optional double min_val = 1;
			optional double max_val = 2;
		}
		}
	}

	extend google.protobuf.FieldOptions {
		optional Rule rules = 1234;
	}

	message IsAuthorizedReq {
			repeated string subjects = 1
				[(rules).repeated = {
					min_items: 1,
					items: { string: { pattern: "^(?:(?:team:(?:local|ldap))|user):[[:alnum:]_-]+$" } },
				}];
	}

	// tests cases where field names collide with keywords

	message KeywordCollisions {
		optional bool syntax = 1;
		optional bool import = 2;
		optional bool public = 3;
		optional bool weak = 4;
		optional bool package = 5;
		optional string string = 6;
		optional bytes bytes = 7;
		optional int32 int32 = 8;
		optional int64 int64 = 9;
		optional uint32 uint32 = 10;
		optional uint64 uint64 = 11;
		optional sint32 sint32 = 12;
		optional sint64 sint64 = 13;
		optional fixed32 fixed32 = 14;
		optional fixed64 fixed64 = 15;
		optional sfixed32 sfixed32 = 16;
		optional sfixed64 sfixed64 = 17;
		optional bool bool = 18;
		optional float float = 19;
		optional double double = 20;
		optional bool optional = 21;
		optional bool repeated = 22;
		optional bool required = 23;
		optional bool message = 24;
		optional bool enum = 25;
		optional bool service = 26;
		optional bool rpc = 27;
		optional bool option = 28;
		optional bool extend = 29;
		optional bool extensions = 30;
		optional bool reserved = 31;
		optional bool to = 32;
		optional int32 true = 33;
		optional int32 false = 34;
		optional int32 default = 35;
	}

	extend google.protobuf.FieldOptions {
		optional bool syntax = 20001;
		optional bool import = 20002;
		optional bool public = 20003;
		optional bool weak = 20004;
		optional bool package = 20005;
		optional string string = 20006;
		optional bytes bytes = 20007;
		optional int32 int32 = 20008;
		optional int64 int64 = 20009;
		optional uint32 uint32 = 20010;
		optional uint64 uint64 = 20011;
		optional sint32 sint32 = 20012;
		optional sint64 sint64 = 20013;
		optional fixed32 fixed32 = 20014;
		optional fixed64 fixed64 = 20015;
		optional sfixed32 sfixed32 = 20016;
		optional sfixed64 sfixed64 = 20017;
		optional bool bool = 20018;
		optional float float = 20019;
		optional double double = 20020;
		optional bool optional = 20021;
		optional bool repeated = 20022;
		optional bool required = 20023;
		optional bool message = 20024;
		optional bool enum = 20025;
		optional bool service = 20026;
		optional bool rpc = 20027;
		optional bool option = 20028;
		optional bool extend = 20029;
		optional bool extensions = 20030;
		optional bool reserved = 20031;
		optional bool to = 20032;
		optional int32 true = 20033;
		optional int32 false = 20034;
		optional int32 default = 20035;
		optional KeywordCollisions boom = 20036;
	}

	message KeywordCollisionOptions {
		optional uint64 id = 1 [
			(syntax) = true, (import) = true, (public) = true, (weak) = true, (package) = true,
			(string) = "string", (bytes) = "bytes", (bool) = true,
			(float) = 3.14, (double) = 3.14159,
			(int32) = 32, (int64) = 64, (uint32) = 3200, (uint64) = 6400, (sint32) = -32, (sint64) = -64,
			(fixed32) = 3232, (fixed64) = 6464, (sfixed32) = -3232, (sfixed64) = -6464,
			(optional) = true, (repeated) = true, (required) = true,
			(message) = true, (enum) = true, (service) = true, (rpc) = true,
			(option) = true, (extend) = true, (extensions) = true, (reserved) = true,
			(to) = true, (true) = 111, (false) = -111, (default) = 222
		];
		optional string name = 2 [
			(boom) = {
				syntax: true, import: true, public: true, weak: true, package: true,
				string: "string", bytes: "bytes", bool: true,
				float: 3.14, double: 3.14159,
				int32: 32, int64: 64, uint32: 3200, uint64: 6400, sint32: -32, sint64: -64,
				fixed32: 3232, fixed64: 6464, sfixed32: -3232, sfixed64: -6464,
				optional: true, repeated: true, required: true,
				message: true, enum: true, service: true, rpc: true,
				option: true, extend: true, extensions: true, reserved: true,
				to: true, true: 111, false: -111, default: 222
			}
		];
	}
	// comment for last element in file, KeywordCollisionOptions
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
		depAsFile, err := NewFile(dep, nil)
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

	symtab := NewSymbolTable()

	for z := 0; z < 100; z++ {
		states := make([]*Symbols, 0, len(files))
		// import each file and record the state of the symbol table
		for _, filename := range filenames {
			beforeImport := symtab.Clone()
			err := symtab.Import(fds[filename], h)
			afterImport := symtab.Clone()
			require.NoError(t, err)
			err = symtab.Delete(fds[filename], h)
			require.NoError(t, err)
			requireSymbolsEqual(t, beforeImport, symtab)
			err = symtab.Import(fds[filename], h)
			require.NoError(t, err)
			requireSymbolsEqual(t, afterImport, symtab)
			states = append(states, symtab.Clone())
		}

		// delete each file and verify that the state of the symbol table is restored to the previous state
		for i := len(filenames) - 1; i >= 0; i-- {
			err := symtab.Delete(fds[filenames[i]], h)
			require.NoError(t, err)

			// if this is the last file, the symbol table should be empty
			if i == 0 {
				requireSymbolsEmpty(t, symtab)
			} else {
				// otherwise, the symbol table should match the state recorded after the previous file was imported
				requireSymbolsEqual(t, states[i-1], symtab)
			}
		}
	}

	requireSymbolsEmpty(t, symtab)
}

func requireSymbolsEmpty(t *testing.T, s *Symbols) {
	t.Helper()
	require.Empty(t, s.pkgTrie.children)
	require.Empty(t, s.pkgTrie.exts)
	require.Empty(t, s.pkgTrie.files)
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
	requireEquivalent(t, expected.files, actual.files)
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

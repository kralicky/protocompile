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

package protocompile

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/internal/prototest"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/reporter"
)

func TestParseFilesMessageComments(t *testing.T) {
	t.Parallel()
	comp := Compiler{
		Resolver:       &SourceResolver{},
		SourceInfoMode: SourceInfoStandard,
	}
	ctx := context.Background()
	files, err := comp.Compile(ctx, "internal/testdata/desc_test1.proto")
	if !assert.Nil(t, err, "%v", err) {
		t.FailNow()
	}
	comments := ""
	expected := " Comment for TestMessage\n"
	for _, fd := range files.Files {
		msg := fd.Messages().ByName("TestMessage")
		if msg != nil {
			si := fd.SourceLocations().ByDescriptor(msg)
			if si.Path != nil {
				comments = si.LeadingComments
			}
			break
		}
	}
	assert.Equal(t, expected, comments)
}

func TestParseFilesWithImportsNoImportPath(t *testing.T) {
	t.Parallel()
	relFilePaths := []string{
		"a/b/b1.proto",
		"a/b/b2.proto",
		"c/c.proto",
	}

	comp := Compiler{
		Resolver: WithStandardImports(&SourceResolver{
			ImportPaths: []string{"internal/testdata/more"},
		}),
	}
	ctx := context.Background()
	protos, err := comp.Compile(ctx, relFilePaths...)
	if !assert.Nil(t, err, "%v", err) {
		t.FailNow()
	}
	assert.Equal(t, len(relFilePaths), len(protos.Files))
}

func TestParseFilesWithDependencies(t *testing.T) {
	t.Parallel()
	// Create some file contents that import a non-well-known proto.
	// (One of the protos in internal/testdata is fine.)
	contents := map[string]string{
		"test.proto": `
			syntax = "proto3";
			import "desc_test_wellknowntypes.proto";

			message TestImportedType {
				testprotos.TestWellKnownTypes imported_field = 1;
			}
		`,
	}
	baseResolver := ResolverFunc(func(f string) (SearchResult, error) {
		s, ok := contents[f]
		if !ok {
			return SearchResult{}, os.ErrNotExist
		}
		return SearchResult{Source: strings.NewReader(s)}, nil
	})

	fdset := prototest.LoadDescriptorSet(t, "./internal/testdata/all.protoset", nil)
	wktDesc, wktDescProto := findAndLink(t, "desc_test_wellknowntypes.proto", fdset, nil)

	ctx := context.Background()

	// Establish that we *can* parse the source file with a resolver that provides
	// the dependency, as either a full descriptor or as a descriptor proto.
	t.Run("DependencyIncluded", func(t *testing.T) {
		t.Parallel()
		// Create a dependency-aware compiler.
		compiler := Compiler{
			Resolver: ResolverFunc(func(f string) (SearchResult, error) {
				if f == "desc_test_wellknowntypes.proto" {
					return SearchResult{Desc: wktDesc}, nil
				}
				return baseResolver.FindFileByPath(f)
			}),
		}
		_, err := compiler.Compile(ctx, "test.proto")
		assert.Nil(t, err, "%v", err)
	})
	t.Run("DependencyIncludedProto", func(t *testing.T) {
		t.Parallel()
		// Create a dependency-aware compiler.
		compiler := Compiler{
			Resolver: WithStandardImports(ResolverFunc(func(f string) (SearchResult, error) {
				if f == "desc_test_wellknowntypes.proto" {
					return SearchResult{Proto: wktDescProto}, nil
				}
				return baseResolver.FindFileByPath(f)
			})),
		}
		_, err := compiler.Compile(ctx, "test.proto")
		assert.Nil(t, err, "%v", err)
	})

	// Establish that we *can not* parse the source file if the resolver
	// is not able to resolve the dependency.
	t.Run("DependencyExcluded", func(t *testing.T) {
		t.Parallel()
		// Create a dependency-UNaware parser.
		compiler := Compiler{Resolver: baseResolver}
		_, err := compiler.Compile(ctx, "test.proto")
		assert.NotNil(t, err, "expected parse to fail")
	})

	t.Run("NoDependencies", func(t *testing.T) {
		t.Parallel()
		// Create a dependency-aware parser that should never be called.
		compiler := Compiler{
			Resolver: ResolverFunc(func(f string) (SearchResult, error) {
				switch f {
				case "test.proto":
					return SearchResult{Source: strings.NewReader(`syntax = "proto3";`)}, nil
				case descriptorProtoPath:
					// used to see if resolver provides custom descriptor.proto
					return SearchResult{}, os.ErrNotExist
				default:
					// no other name should be passed to resolver
					t.Errorf("resolver was called for unexpected filename %q", f)
					return SearchResult{}, os.ErrNotExist
				}
			}),
		}
		_, err := compiler.Compile(ctx, "test.proto")
		assert.Nil(t, err)
	})
}

func findAndLink(t *testing.T, filename string, fdset *descriptorpb.FileDescriptorSet, soFar *protoregistry.Files) (protoreflect.FileDescriptor, *descriptorpb.FileDescriptorProto) {
	for _, fd := range fdset.File {
		if fd.GetName() == filename {
			if soFar == nil {
				soFar = &protoregistry.Files{}
			}
			for _, dep := range fd.GetDependency() {
				depDesc, _ := findAndLink(t, dep, fdset, soFar)
				err := soFar.RegisterFile(depDesc)
				if !assert.NoError(t, err) {
					t.FailNow()
				}
			}
			desc, err := protodesc.NewFile(fd, soFar)
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			return desc, fd
		}
	}
	assert.FailNow(t, "could not find dependency %q in proto set", filename)
	return nil, nil // make compiler happy
}

func TestParseCommentsBeforeDot(t *testing.T) {
	t.Parallel()
	accessor := SourceAccessorFromMap(map[string]string{
		"test.proto": `
syntax = "proto3";
message Foo {
  // leading comments
  .Foo foo = 1;
}
`,
	})

	compiler := Compiler{
		Resolver:       &SourceResolver{Accessor: accessor},
		SourceInfoMode: SourceInfoStandard,
	}
	ctx := context.Background()
	fds, err := compiler.Compile(ctx, "test.proto")
	assert.Nil(t, err)

	field := fds.Files[0].Messages().Get(0).Fields().Get(0)
	comment := fds.Files[0].SourceLocations().ByDescriptor(field).LeadingComments
	assert.Equal(t, " leading comments\n", comment)
}

func TestParseCustomOptions(t *testing.T) {
	t.Parallel()
	accessor := SourceAccessorFromMap(map[string]string{
		"test.proto": `
syntax = "proto3";
import "google/protobuf/descriptor.proto";
extend google.protobuf.MessageOptions {
    string foo = 30303;
    int64 bar = 30304;
}
message Foo {
  option (.foo) = "foo";
  option (bar) = 123;
}
`,
	})

	compiler := Compiler{
		Resolver:       WithStandardImports(&SourceResolver{Accessor: accessor}),
		SourceInfoMode: SourceInfoStandard,
	}
	ctx := context.Background()
	fds, err := compiler.Compile(ctx, "test.proto")
	if !assert.Nil(t, err, "%v", err) {
		t.FailNow()
	}

	ext := fds.Files[0].Extensions().ByName("foo")
	md := fds.Files[0].Messages().Get(0)
	fooVal := md.Options().ProtoReflect().Get(ext)
	assert.Equal(t, "foo", fooVal.String())

	ext = fds.Files[0].Extensions().ByName("bar")
	barVal := md.Options().ProtoReflect().Get(ext)
	assert.Equal(t, int64(123), barVal.Int())
}

func TestDataRace(t *testing.T) {
	t.Parallel()
	if !internal.IsRace {
		t.Skip("only useful when race detector enabled")
		return
	}

	data, err := os.ReadFile("./internal/testdata/desc_test_complex.proto")
	require.NoError(t, err)
	ast, err := parser.Parse("desc_test_complex.proto", bytes.NewReader(data), reporter.NewHandler(nil))
	require.NoError(t, err)
	parseResult, err := parser.ResultFromAST(ast, true, reporter.NewHandler(nil))
	require.NoError(t, err)
	// let's also produce a resolved proto
	files, err := (&Compiler{
		Resolver: WithStandardImports(&SourceResolver{
			ImportPaths: []string{"./internal/testdata"},
		}),
		SourceInfoMode: SourceInfoStandard,
	}).Compile(context.Background(), "desc_test_complex.proto")
	require.NoError(t, err)
	resolvedProto := files.Files[0].(linker.Result).FileDescriptorProto()

	descriptor, err := protoregistry.GlobalFiles.FindFileByPath(descriptorProtoPath)
	require.NoError(t, err)
	descriptorProto := protodesc.ToFileDescriptorProto(descriptor)

	// We will share this descriptor/parse result (which needs to be modified by the linker
	// to resolve all references) from multiple concurrent operations to make sure the race
	// detector is not triggered.
	testCases := []struct {
		name     string
		resolver Resolver
	}{
		{
			name: "share unresolved descriptor",
			resolver: WithStandardImports(ResolverFunc(func(name string) (SearchResult, error) {
				if name == "desc_test_complex.proto" {
					return SearchResult{
						Proto: parseResult.FileDescriptorProto(),
					}, nil
				}
				return SearchResult{}, os.ErrNotExist
			})),
		},
		{
			name: "share resolved descriptor",
			resolver: WithStandardImports(ResolverFunc(func(name string) (SearchResult, error) {
				if name == "desc_test_complex.proto" {
					return SearchResult{
						Proto: resolvedProto,
					}, nil
				}
				return SearchResult{}, os.ErrNotExist
			})),
		},
		{
			name: "share unresolved parse result",
			resolver: WithStandardImports(ResolverFunc(func(name string) (SearchResult, error) {
				if name == "desc_test_complex.proto" {
					return SearchResult{
						ParseResult: parseResult,
					}, nil
				}
				return SearchResult{}, os.ErrNotExist
			})),
		},
		{
			name: "share google/protobuf/descriptor.proto",
			resolver: WithStandardImports(ResolverFunc(func(name string) (SearchResult, error) {
				// we'll parse our test proto from source, but its implicit dep on
				// descriptor.proto will use a
				switch name {
				case "desc_test_complex.proto":
					return SearchResult{
						Source: bytes.NewReader(data),
					}, nil
				case "google/protobuf/descriptor.proto":
					return SearchResult{
						Proto: descriptorProto,
					}, nil
				default:
					return SearchResult{}, os.ErrNotExist
				}
			})),
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			compiler1 := &Compiler{
				Resolver:       testCase.resolver,
				SourceInfoMode: SourceInfoStandard,
			}
			compiler2 := &Compiler{
				Resolver:       testCase.resolver,
				SourceInfoMode: SourceInfoStandard,
			}
			grp, ctx := errgroup.WithContext(context.Background())
			grp.Go(func() error {
				_, err := compiler1.Compile(ctx, "desc_test_complex.proto")
				return err
			})
			grp.Go(func() error {
				_, err := compiler2.Compile(ctx, "desc_test_complex.proto")
				return err
			})
			err := grp.Wait()
			require.NoError(t, err)
		})
	}
}

func TestPanicHandling(t *testing.T) {
	t.Parallel()
	c := Compiler{
		Resolver: ResolverFunc(func(string) (SearchResult, error) {
			panic(errors.New("mui mui bad"))
		}),
	}
	_, err := c.Compile(context.Background(), "test.proto")
	panicErr, ok := err.(PanicError)
	require.True(t, ok)
	t.Logf("%v\n\n%v", panicErr, panicErr.Stack)
}

func TestDescriptorProtoPath(t *testing.T) {
	t.Parallel()
	// sanity check our constant
	path := (*descriptorpb.FileDescriptorProto)(nil).ProtoReflect().Descriptor().ParentFile().Path()
	require.Equal(t, descriptorProtoPath, path)
}

var baseContents = map[string]string{
	"a/b/b1.proto": `
syntax = "proto3";

package a.b;

message BeeOne {}
`,
	"a/b/b2.proto": `
syntax = "proto3";

package a.b;

import "a/b/b1.proto";

message BeeTwo {
  BeeOne bee_one = 1;
}
`,
	"c/c.proto": `
syntax = "proto3";

package c;

import "a/b/b1.proto";
import "a/b/b2.proto";
import "google/protobuf/timestamp.proto";

message See {
  a.b.BeeOne bee_one = 1;
  a.b.BeeTwo bee_two = 2;
  google.protobuf.Timestamp timestamp = 3;
}
`,
}

func mkResolver(contents map[string]string) Resolver {
	return ResolverFunc(func(name string) (SearchResult, error) {
		if s, ok := contents[name]; ok {
			return SearchResult{Source: strings.NewReader(s)}, nil
		}
		return SearchResult{}, os.ErrNotExist
	})
}
func TestIncrementalCompiler(t *testing.T) {
	baseResults := buildBaseDescriptors()

	overlay := map[string]string{}

	comp := Compiler{
		MaxParallelism: runtime.NumCPU(),
		Resolver: WithStandardImports(CompositeResolver{
			mkResolver(overlay),
			mkResolver(baseContents),
		}),
		SourceInfoMode: SourceInfoExtraComments | SourceInfoExtraOptionLocations,
		RetainASTs:     true,
		RetainResults:  true,
	}
	res, err := comp.Compile(context.Background(), "a/b/b1.proto", "a/b/b2.proto", "c/c.proto")
	require.NoError(t, err)

	requireASTsEqual(t, baseResults, res.Files, "a/b/b1.proto", "a/b/b2.proto", "c/c.proto")

	overlay["a/b/b1.proto"] = `
syntax = "proto3";

package a.b;

message BeeZero {}
`
	res, err = comp.Compile(context.Background(), "a/b/b1.proto")
	if !assert.ErrorContains(t, err, "a/b/b2.proto:9:3: field a.b.BeeTwo.bee_one: unknown type BeeOne") &&
		!assert.ErrorContains(t, err, "c/c.proto:11:3: field c.See.bee_one: unknown type a.b.BeeOne") {
		assert.FailNow(t, "expected error about unknown type BeeOne")
	}

	delete(overlay, "a/b/b1.proto")
	res, err = comp.Compile(context.Background(), "a/b/b1.proto")
	require.NoError(t, err)

	overlay["c/c.proto"] = `
syntax = "proto3";

package c;

import "a/b/b1.proto";
import "a/b/b2.proto";
import "google/protobuf/timestamp.proto";

message See {
  a.b.BeeOne bee_one = 1;
  a.b.BeeTwo bee_two = 2;
  google.protobuf.Timestamp timestamp = 3;
	string foo = 4;
}
`

	res, err = comp.Compile(context.Background(), "c/c.proto")
	assert.NoError(t, err)
}

func buildBaseDescriptors() linker.Files {
	comp := Compiler{
		Resolver:       WithStandardImports(mkResolver(baseContents)),
		SourceInfoMode: SourceInfoExtraComments | SourceInfoExtraOptionLocations,
		RetainASTs:     true,
	}

	results, err := comp.Compile(context.Background(), "a/b/b1.proto", "a/b/b2.proto", "c/c.proto")
	if err != nil {
		panic(err)
	}

	return results.Files
}

func requireASTsEqual(t *testing.T, a, b linker.Files, filenames ...string) {
	t.Helper()
	for _, filename := range filenames {
		aRes := a.FindFileByPath(filename)
		bRes := b.FindFileByPath(filename)
		require.Equal(t, aRes.(linker.Result).AST(), bRes.(linker.Result).AST())
	}
}

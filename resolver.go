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
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/parser"
)

// Resolver is used by the compiler to resolve a proto source file name
// into some unit that is usable by the compiler. The result could be source
// for a proto file or it could be an already-parsed AST or descriptor.
//
// Resolver implementations must be thread-safe as a single compilation
// operation could invoke FindFileByPath from multiple goroutines.
type Resolver interface {
	// FindFileByPath searches for information for the given file path. If no
	// result is available, it should return a non-nil error, such as
	// protoregistry.NotFound.
	FindFileByPath(path UnresolvedPath, whence ImportContext) (SearchResult, error)
}

// SearchResult represents information about a proto source file. Only one of
// the various fields must be set, based on what is available for a file. If
// multiple fields are set, the compiler prefers them in opposite order listed:
// so it uses a descriptor if present and only falls back to source if nothing
// else is available.
type SearchResult struct {
	// A unique path derived from the unresolved path and the import context.
	// If the import path was modified using information from the import context,
	// set this field to the path that was actually used to find the file.
	// This will ensure future lookups for this file will use the same path.
	// Required if the import context is non-empty, otherwise the path was
	// already known to be resolved.
	ResolvedPath ResolvedPath
	// Represents source code for the file. This should be nil if source code
	// is not available. If no field below is set, then the compiler will parse
	// the source code into an AST.
	Source io.Reader
	// Represents the abstract syntax tree for the file. If no field below is
	// set, then the compiler will convert the AST into a descriptor proto.
	AST *ast.FileNode
	// A descriptor proto that represents the file. If the field below is not
	// set, then the compiler will link this proto with its dependencies to
	// produce a linked descriptor.
	Proto *descriptorpb.FileDescriptorProto
	// A parse result for the file. This packages both an AST and a descriptor
	// proto in one. When a parser result is available, it is more efficient
	// than using an AST search result, since the descriptor proto need not be
	// re-created. And it provides better error messages than a descriptor proto
	// search result, since the AST has greater fidelity with regard to source
	// positions (even if the descriptor proto includes source code info).
	ParseResult parser.Result

	// Optional document version number. This will be attached to error and
	// warning reports, but is otherwise not used by the compiler.
	Version int32
}

// ResolverFunc is a simple function type that implements Resolver.
type ResolverFunc func(UnresolvedPath, ImportContext) (SearchResult, error)

var _ Resolver = ResolverFunc(nil)

func (f ResolverFunc) FindFileByPath(path UnresolvedPath, whence ImportContext) (SearchResult, error) {
	return f(path, whence)
}

// CompositeResolver is a slice of resolvers, which are consulted in order
// until one can supply a result. If none of the constituent resolvers can
// supply a result, the error returned by the first resolver is returned. If
// the slice of resolvers is empty, all operations return
// protoregistry.NotFound.
type CompositeResolver []Resolver

var _ Resolver = CompositeResolver(nil)

func (f CompositeResolver) FindFileByPath(path UnresolvedPath, whence ImportContext) (SearchResult, error) {
	if len(f) == 0 {
		return SearchResult{}, protoregistry.NotFound
	}
	var firstErr error
	for _, res := range f {
		r, err := res.FindFileByPath(path, whence)
		if err == nil {
			return r, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return SearchResult{}, firstErr
}

// SourceResolver can resolve file names by returning source code. It uses
// an optional list of import paths to search. By default, it searches the
// file system.
type SourceResolver struct {
	// Optional list of import paths. If present and not empty, then all
	// file paths to find are assumed to be relative to one of these paths.
	// If nil or empty, all file paths to find are assumed to be relative to
	// the current working directory.
	ImportPaths []string
	// Optional function for returning a file's contents. If nil, then
	// os.Open is used to open files on the file system.
	//
	// This function must be thread-safe as a single compilation operation
	// could result in concurrent invocations of this function from
	// multiple goroutines.
	Accessor func(path ResolvedPath) (io.ReadCloser, error)
}

var _ Resolver = (*SourceResolver)(nil)

func (r *SourceResolver) FindFileByPath(path UnresolvedPath, _ ImportContext) (SearchResult, error) {
	if len(r.ImportPaths) == 0 {
		reader, err := r.accessFile(ResolvedPath(path))
		if err != nil {
			return SearchResult{}, err
		}
		return SearchResult{
			ResolvedPath: ResolvedPath(path),
			Source:       reader,
		}, nil
	}

	var e error
	for _, importPath := range r.ImportPaths {
		// is the file fully-qualified with respect to the import path?
		if strings.HasPrefix(string(path), importPath) {
			reader, err := r.accessFile(ResolvedPath(path))
			if err == nil {
				return SearchResult{
					ResolvedPath: ResolvedPath(path),
					Source:       reader,
				}, nil
			}
		}
		resolved := ResolvedPath(filepath.Join(importPath, string(path)))
		reader, err := r.accessFile(resolved)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				e = err
				continue
			}
			return SearchResult{}, err
		}
		rel, err := filepath.Rel(importPath, string(resolved))
		if err != nil {
			return SearchResult{}, err
		}
		return SearchResult{
			ResolvedPath: ResolvedPath(rel),
			Source:       reader,
		}, nil
	}
	return SearchResult{}, e
}

func (r *SourceResolver) accessFile(path ResolvedPath) (io.ReadCloser, error) {
	if r.Accessor != nil {
		return r.Accessor(path)
	}
	return os.Open(string(path))
}

// SourceAccessorFromMap returns a function that can be used as the Accessor
// field of a SourceResolver that uses the given map to load source. The map
// keys are file names and the values are the corresponding file contents.
//
// The given map is used directly and not copied. Since accessor functions
// must be thread-safe, this means that the provided map must not be mutated
// once this accessor is provided to a compile operation.
func SourceAccessorFromMap(srcs map[string]string) func(ResolvedPath) (io.ReadCloser, error) {
	return func(path ResolvedPath) (io.ReadCloser, error) {
		src, ok := srcs[string(path)]
		if !ok {
			return nil, os.ErrNotExist
		}
		return io.NopCloser(strings.NewReader(src)), nil
	}
}

// WithStandardImports returns a new resolver that knows about the same standard
// imports that are included with protoc.
func WithStandardImports(r Resolver) Resolver {
	return ResolverFunc(func(name UnresolvedPath, whence ImportContext) (SearchResult, error) {
		res, err := r.FindFileByPath(name, whence)
		if err != nil {
			// error from given resolver? see if it's a known standard file
			if d, ok := standardImports[string(name)]; ok {
				return SearchResult{
					ResolvedPath: ResolvedPath(name),
					Proto:        d,
				}, nil
			}
		}
		return res, err
	})
}

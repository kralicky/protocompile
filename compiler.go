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
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/linker"
	"github.com/kralicky/protocompile/options"
	"github.com/kralicky/protocompile/parser"
	"github.com/kralicky/protocompile/reporter"
	"github.com/kralicky/protocompile/sourceinfo"
)

// Compiler handles compilation tasks, to turn protobuf source files, or other
// intermediate representations, into fully linked descriptors.
//
// The compilation process involves five steps for each protobuf source file:
//  1. Parsing the source into an AST (abstract syntax tree).
//  2. Converting the AST into descriptor protos.
//  3. Linking descriptor protos into fully linked descriptors.
//  4. Interpreting options.
//  5. Computing source code information.
//
// With fully linked descriptors, code generators and protoc plugins could be
// invoked (though that step is not implemented by this package and not a
// responsibility of this type).
type Compiler struct {
	// Resolves path/file names into source code or intermediate representations
	// for protobuf source files. This is how the compiler loads the files to
	// be compiled as well as all dependencies. This field is the only required
	// field.
	Resolver Resolver
	// The maximum parallelism to use when compiling. If unspecified or set to
	// a non-positive value, then min(runtime.NumCPU(), runtime.GOMAXPROCS(-1))
	// will be used.
	MaxParallelism int
	// A custom error and warning reporter. If unspecified a default reporter
	// is used. A default reporter fails the compilation after encountering any
	// errors and ignores all warnings.
	Reporter reporter.Reporter

	// If unspecified or set to SourceInfoNone, source code information will not
	// be included in the resulting descriptors. Source code information is
	// metadata in the file descriptor that provides position information (i.e.
	// the line and column where file elements were defined) as well as comments.
	//
	// If set to SourceInfoStandard, normal source code information will be
	// included in the resulting descriptors. This matches the output of protoc
	// (the reference compiler for Protocol Buffers). If set to
	// SourceInfoMoreComments, the resulting descriptor will attempt to preserve
	// as many comments as possible, for all elements in the file, not just for
	// complete declarations.
	//
	// If Resolver returns descriptors or descriptor protos for a file, then
	// those descriptors will not be modified. If they do not already include
	// source code info, they will be left that way when the compile operation
	// concludes. Similarly, if they already have source code info but this flag
	// is false, existing info will be left in place.
	SourceInfoMode SourceInfoMode

	// If true, ASTs are retained in compilation results for which an AST was
	// constructed. So any linker.Result value in the resulting compiled files
	// will have an AST, in addition to descriptors. If left false, the AST
	// will be removed as soon as it's no longer needed. This can help reduce
	// total memory usage for operations involving a large number of files.
	RetainASTs bool

	RetainResults bool

	// If true, all linked dependencies will be provided in the compiler results,
	// even if they were not explicitly requested to be compiled. Otherwise,
	// only the requested files will be included in the results.
	IncludeDependenciesInResults bool

	Hooks CompilerHooks

	exec *executor
}

type CompilerHooks struct {
	// If not nil, called before a file is invalidated.
	// Will be called before any dependencies have been invalidated.
	// This is called for all files, including those that contained errors
	// and were not fully linked (for which PostInvalidate will not be called).
	PreInvalidate func(path ResolvedPath, reason string)
	// If not nil, called after a file (and all its dependencies) have been
	// invalidated. This is only called for fully linked files without errors.
	// The previous result is guaranteed to be equal to a result that was
	// returned in the single most recent call to Compile; for all other purposes
	// it should be treated as opaque.
	// If the file is no longer resolvable (if it was deleted, for example),
	// willRecompile will be set to false. Otherwise, it will be true.
	PostInvalidate func(path ResolvedPath, previousResult linker.File, willRecompile bool)
	// If not nil, called before a file is compiled.
	PreCompile func(path ResolvedPath)
	// If not nil, called after a file has been compiled.
	PostCompile func(path ResolvedPath)
}

// SourceInfoMode indicates how source code info is generated by a Compiler.
type SourceInfoMode int

const (
	// SourceInfoNone indicates that no source code info is generated.
	SourceInfoNone = SourceInfoMode(0)
	// SourceInfoStandard indicates that the standard source code info is
	// generated, which includes comments only for complete declarations.
	SourceInfoStandard = SourceInfoMode(1)
	// SourceInfoExtraComments indicates that source code info is generated
	// and will include comments for all elements (more comments than would
	// be found in a descriptor produced by protoc).
	SourceInfoExtraComments = SourceInfoMode(2)
	// SourceInfoExtraOptionLocations indicates that source code info is
	// generated with additional locations for elements inside of message
	// literals in option values. This can be combined with the above by
	// bitwise-OR'ing it with SourceInfoExtraComments.
	SourceInfoExtraOptionLocations = SourceInfoMode(4)
)

type CompileResult struct {
	linker.Files
	PartialLinkResults    map[ResolvedPath]linker.Result
	UnlinkedParserResults map[ResolvedPath]parser.Result
}

// there are a variety of string identifiers used to refer to compiler results
// in different contexts, some of which cannot be interchanged. To avoid
// accidental misuse, these types are used to distinguish them.
type (
	// An import path as it appears in a file.
	UnresolvedPath string
	// A resolved path, uniquely identifying a file.
	ResolvedPath string
)

// Compile compiles the given unique paths into fully-linked descriptors. The
// compiler's resolver is used to locate source code (or intermediate artifacts
// such as parsed ASTs or descriptor protos) and then do what is necessary to
// transform that into descriptors (parsing, linking, etc).
//
// It is very important that the paths requested are known to the resolver
// to be unique. Because the same file can be resolved under different paths
// depending on the import context, these paths must be the ones that imports
// will be resolved *to*.
//
// Elements in the given returned files will implement [linker.Result] if the
// compiler had to link it (i.e. the resolver provided either a descriptor proto
// or source code). That result will contain a full AST for the file if the
// compiler had to parse it (i.e. the resolver provided source code for that
// file).
func (c *Compiler) Compile(ctx context.Context, paths ...ResolvedPath) (CompileResult, error) {
	if len(paths) == 0 {
		return CompileResult{}, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	par := c.MaxParallelism
	if par <= 0 {
		par = runtime.GOMAXPROCS(-1)
		cpus := runtime.NumCPU()
		if par > cpus {
			par = cpus
		}
	}

	h := reporter.NewHandler(c.Reporter)

	var e *executor
	if c.exec == nil {
		e = &executor{
			c:       c,
			h:       h,
			s:       semaphore.NewWeighted(int64(par)),
			cancel:  cancel,
			sym:     &linker.Symbols{},
			results: map[ResolvedPath]*result{},
			hooks:   c.Hooks,
		}
		if c.RetainResults {
			c.exec = e
		}
	} else {
		e = c.exec
		e.h = h // important: clear any previous errors
	}

	// We lock now and create all tasks under lock to make sure that no
	// async task can create a duplicate result. For example, if files
	// contains both "foo.proto" and "bar.proto", then there is a race
	// after we start compiling "foo.proto" between this loop and the
	// async compilation task to create the result for "bar.proto". But
	// we need to know if the file is directly requested for compilation,
	// so we need this loop to define the result. So this loop holds the
	// lock the whole time so async tasks can't create a result first.
	needsRecompile := e.invalidate(paths...)
	results := make([]*result, 0, len(needsRecompile))

	for _, f := range needsRecompile {
		results = append(results, e.resolveAndCompile(ctx, UnresolvedPath(f), true, nil))
	}

	descs := make(linker.Files, 0, len(results))
	unlinked := make(map[ResolvedPath]parser.Result)
	partiallyLinked := make(map[ResolvedPath]linker.Result)
	var firstError error
	for _, r := range results {
		select {
		case <-r.ready:
		case <-ctx.Done():
			return CompileResult{}, ctx.Err()
		}
		if r.err != nil {
			if firstError == nil {
				firstError = r.err
			}
		}
		if r.res != nil {
			descs = append(descs, r.res)
		} else if r.partialLinkRes != nil {
			partiallyLinked[r.resolvedPath] = r.partialLinkRes
		} else if r.parseRes != nil {
			unlinked[r.resolvedPath] = r.parseRes
		}
	}

	if c.IncludeDependenciesInResults {
		descs = linker.ComputeReflexiveTransitiveClosure(descs)
	}

	if err := h.Error(); err != nil {
		return CompileResult{
			Files:                 descs,
			PartialLinkResults:    partiallyLinked,
			UnlinkedParserResults: unlinked,
		}, err
	}
	// this should probably never happen; if any task returned an
	// error, h.Error() should be non-nil
	return CompileResult{
		Files:                 descs,
		PartialLinkResults:    partiallyLinked,
		UnlinkedParserResults: unlinked,
	}, firstError
}

type block struct {
	// The import path as it appears in the file
	ImportedAs UnresolvedPath
	// The effective path resolved from the import path, uniquely identifying
	// the file that will be imported.
	ResolvedPath ResolvedPath
	resolved     chan struct{}
}

type result struct {
	// The resolved path of the file. This can only be read after the ready
	// channel is closed and err==nil, otherwise its contents are undefined.
	resolvedPath ResolvedPath

	ready chan struct{}

	// true if this file was explicitly provided to the compiler; otherwise
	// this file is an import that is implicitly included
	explicitFile bool

	// produces a linker.File or error, only available when ready is closed
	res linker.Result
	// parser result, may be available if linking fails but the file is syntactically valid
	parseRes parser.Result
	// partial link result, may be available if linking fails
	partialLinkRes linker.Result

	err error

	mu sync.Mutex
	// the results that are dependencies of this result; this result is
	// blocked, waiting on these dependencies to complete.
	blockedOn []*block
}

func (r *result) cancel(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
	close(r.ready)
}

func (r *result) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.err = err
	r.res = nil
	r.parseRes = nil

	close(r.ready)
}

func (r *result) failPartial(parseRes parser.Result, partialLinkRes linker.Result, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.err = err
	r.res = nil
	r.parseRes = parseRes
	r.partialLinkRes = partialLinkRes

	close(r.ready)
}

func (r *result) complete(f linker.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.err = nil
	r.res = f
	if parseRes, ok := f.(parser.Result); ok {
		r.parseRes = parseRes
	}

	close(r.ready)
}

func (r *result) setBlockedOn(blocks []*block) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockedOn = blocks
}

func (r *result) getBlockedOn() []*block {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.blockedOn
}

type executor struct {
	c      *Compiler
	h      *reporter.Handler
	s      *semaphore.Weighted
	cancel context.CancelFunc

	symTxLock sync.Mutex
	sym       *linker.Symbols

	descriptorProtoCheck    sync.Once
	descriptorProtoIsCustom bool

	mu      sync.Mutex
	results map[ResolvedPath]*result

	hooks CompilerHooks
}

type ImportContext parser.Result

func (e *executor) invalidate(rpaths ...ResolvedPath) []ResolvedPath {
	// remove the result from the cache, along with any results that depend on it
	e.mu.Lock()
	defer e.mu.Unlock()

	invalidated := map[ResolvedPath]struct{}{}
	blocks := map[ResolvedPath][]*result{}
	for _, res := range e.results {
		for _, dep := range res.blockedOn {
			if dep.ResolvedPath == "" {
				// this dependency was imported under a different name that could not be resolved,
				// so the relationship was never recorded
				continue
			}
			blockedDep, ok := e.results[dep.ResolvedPath]
			if !ok {
				slog.Error("bug: detected an inconsistency in dependency graph", "file", res.resolvedPath, "res", res, "dep", dep, "results", e.results)
				panic("bug: detected an inconsistency in dependency graph")
			}
			blocks[blockedDep.resolvedPath] = append(blocks[blockedDep.resolvedPath], res)
		}
	}
	for _, rpath := range rpaths {
		r := e.results[rpath]
		if r == nil {
			invalidated[rpath] = struct{}{}
			continue
		}

		e.invalidateLocked(r, blocks, invalidated, "file was modified")
	}

	filenames := make([]ResolvedPath, 0, len(invalidated))
	for name := range invalidated {
		if _, err := e.c.Resolver.FindFileByPath(UnresolvedPath(name), nil); err != nil {
			// if the file doesn't exist anymore, we don't need to
			// recompile it
			// if e.hooks.PostInvalidate != nil {
			// 	e.hooks.PostInvalidate(name, invalidated[name])
			// }
			continue
		}
		filenames = append(filenames, name)
	}
	return filenames
}

func (e *executor) invalidateLocked(r *result, blocks map[ResolvedPath][]*result, seen map[ResolvedPath]struct{}, reason string) {
	if _, ok := seen[r.resolvedPath]; ok {
		return
	}
	seen[r.resolvedPath] = struct{}{}

	if e.hooks.PreInvalidate != nil {
		e.hooks.PreInvalidate(r.resolvedPath, reason)
	}

	for _, dep := range blocks[r.resolvedPath] {
		// fmt.Printf(" => invalidating %s (depends on %s)\n", dep.name, r.name)
		e.invalidateLocked(dep, blocks, seen, fmt.Sprintf("file depends on %s", r.resolvedPath))
	}

	if r.res != nil {
		if e.hooks.PostInvalidate != nil {
			defer func() {
				if err := recover(); err != nil {
					panic(err)
				}
				_, err := e.c.Resolver.FindFileByPath(UnresolvedPath(r.resolvedPath), nil)
				e.hooks.PostInvalidate(r.resolvedPath, r.res, err == nil)
			}()
		}
		// if an error occurred, r.res will be nil. we can skip deleting
		// the file in that case, since the link error should cause the
		// partially updated symbol table to be discarded.
		if err := e.sym.Delete(r.res, e.h); err != nil {
			panic(err)
		}
	}
	delete(e.results, r.resolvedPath)
}

var closedChannel = make(chan struct{})

func init() {
	close(closedChannel)
}

func (e *executor) resolveAndCompile(ctx context.Context, dep UnresolvedPath, explicitFile bool, whence ImportContext) *result {
	e.mu.Lock()
	defer e.mu.Unlock()

	sr, err := e.c.Resolver.FindFileByPath(UnresolvedPath(dep), whence)
	if err != nil {
		return &result{
			ready: closedChannel,
			err:   errFailedToResolve{err: err, path: dep},
		}
	}
	if sr.ResolvedPath == "" {
		panic("FindFileByPath: resolved path must be set")
	}

	if whence != nil && sr.ResolvedPath == ResolvedPath(whence.FileDescriptorProto().GetName()) {
		// doh! file imports itself
		span := findImportSpan(whence, dep)
		handleImportCycle(e.h, span, []ResolvedPath{sr.ResolvedPath}, dep)
		return &result{
			ready: closedChannel,
			err:   e.h.Error(),
		}
	}

	r := e.results[sr.ResolvedPath]
	if r != nil {
		return r
	}

	r = &result{
		resolvedPath: sr.ResolvedPath,
		ready:        make(chan struct{}),
		explicitFile: explicitFile,
	}
	e.results[sr.ResolvedPath] = r

	go e.doCompile(ctx, r, &sr)
	return r
}

// PanicError is an error value that represents a recovered panic. It includes
// the value returned by recover() as well as the stack trace.
//
// This should generally only be seen if a Resolver implementation panics.
//
// An error returned by a Compiler may wrap a PanicError, so you may need to
// use errors.As(...) to access panic details.
type PanicError struct {
	// The file that was being processed when the panic occurred
	File string
	// The value returned by recover()
	Value interface{}
	// A formatted stack trace
	Stack string
}

// Error implements the error interface. It does NOT include the stack trace.
// Use a type assertion and query the Stack field directly to access that.
func (p PanicError) Error() string {
	return fmt.Sprintf("panic handling %q: %v", p.File, p.Value)
}

type errFailedToResolve struct {
	err  error
	path UnresolvedPath
}

func (e errFailedToResolve) Error() string {
	errMsg := e.err.Error()
	if strings.Contains(errMsg, string(e.path)) {
		// underlying error already refers to path in question, so we don't need to add more context
		return errMsg
	}
	return fmt.Sprintf("could not resolve path %q: %s", e.path, e.err.Error())
}

func (e errFailedToResolve) Unwrap() error {
	return e.err
}

func (e *executor) hasOverrideDescriptorProto() bool {
	e.descriptorProtoCheck.Do(func() {
		defer func() {
			// ignore a panic here; just assume no custom descriptor.proto
			_ = recover()
		}()
		res, err := e.c.Resolver.FindFileByPath(descriptorProtoPath, nil)
		e.descriptorProtoIsCustom = err == nil && res.ResolvedPath != "google/protobuf/descriptor.proto"
	})
	return e.descriptorProtoIsCustom
}

func (e *executor) doCompile(ctx context.Context, r *result, sr *SearchResult) {
	t := task{e: e, h: e.h.SubHandler(), r: r}
	if err := e.s.Acquire(ctx, 1); err != nil {
		r.fail(err)
		return
	}
	defer t.release()

	if e.hooks.PreCompile != nil {
		e.hooks.PreCompile(sr.ResolvedPath)
	}

	defer func() {
		if e.hooks.PostCompile != nil {
			e.hooks.PostCompile(sr.ResolvedPath)
		}
		// if results included a result, don't leave it open if it can be closed
		if sr.Source == nil {
			return
		}
		if c, ok := sr.Source.(io.Closer); ok {
			_ = c.Close()
		}
	}()

	desc, err := t.asFile(ctx, sr)
	if err != nil {
		if desc != nil || sr.ParseResult != nil {
			r.failPartial(sr.ParseResult, desc, err)
			return
		}
		r.fail(err)
		return
	}
	r.complete(desc)
}

// A compilation task. The executor has a semaphore that limits the number
// of concurrent, running tasks.
type task struct {
	e *executor

	// handler for this task
	h *reporter.Handler

	// If true, this task needs to acquire a semaphore permit before running.
	// If false, this task needs to release its semaphore permit on completion.
	released bool

	// the result that is populated by this task
	r *result
}

func (t *task) release() {
	if !t.released {
		t.e.s.Release(1)
		t.released = true
	}
}

const descriptorProtoPath = "google/protobuf/descriptor.proto"

func (t *task) asFile(ctx context.Context, pr *SearchResult) (linker.Result, error) {
	// r := *pr
	// if r.Desc != nil {
	// 	if r.Desc.Path() != string(pr.ResolvedPath) {
	// 		return nil, fmt.Errorf("search result for %q returned descriptor for %q", pr.ResolvedPath, r.Desc.Path())
	// 	}
	// 	return linker.NewFileRecursive(r.Desc)
	// }

	parseRes, err := t.asParseResult(pr)
	if parseRes == nil {
		return nil, err
	}
	pr.ParseResult = parseRes

	if linkRes, ok := parseRes.(linker.Result); ok {
		// if resolver returned a parse result that was actually a link result,
		// use the link result directly (no other steps needed)
		return linkRes, nil
	}

	var deps linker.Files
	fileDescriptorProto := parseRes.FileDescriptorProto()
	var wantsDescriptorProto bool
	protoImports := fileDescriptorProto.Dependency

	if t.e.hasOverrideDescriptorProto() {
		// we only consider implicitly including descriptor.proto if it's overridden
		if pr.ResolvedPath != descriptorProtoPath {
			var includesDescriptorProto bool
			for _, dep := range fileDescriptorProto.Dependency {
				if dep == descriptorProtoPath {
					includesDescriptorProto = true
					break
				}
			}
			if !includesDescriptorProto {
				wantsDescriptorProto = true
				// make a defensive copy so we don't inadvertently mutate
				// slice's backing array when adding this implicit dep
				importsCopy := make([]string, len(protoImports)+1)
				copy(importsCopy, protoImports)
				importsCopy[len(protoImports)] = descriptorProtoPath
				protoImports = importsCopy
			}
		}
	}

	var overrideDescriptorProto linker.File
	if len(protoImports) > 0 {
		blocks := make([]*block, len(protoImports))
		for i, imp := range protoImports {
			blocks[i] = &block{
				ImportedAs: UnresolvedPath(imp),
				resolved:   make(chan struct{}),
			}
		}
		t.r.setBlockedOn(blocks)

		results := make([]*result, len(protoImports))
		for i, dep := range protoImports {
			res := t.e.resolveAndCompile(ctx, UnresolvedPath(dep), false, parseRes)
			blocks[i].ResolvedPath = res.resolvedPath
			close(blocks[i].resolved)
			results[i] = res
		}
		deps = make(linker.Files, len(results))
		var descriptorProtoRes *result
		if wantsDescriptorProto {
			descriptorProtoRes = t.e.resolveAndCompile(ctx, UnresolvedPath(descriptorProtoPath), false, parseRes)
		}

		// release our semaphore so dependencies can be processed w/out risk of deadlock
		t.e.s.Release(1)
		t.released = true

		checked := map[ResolvedPath]struct{}{}
		// now we wait for them all to be computed
		for i, res := range results {
			// check for dependency cycle to prevent deadlock
			span := findImportSpan(parseRes, UnresolvedPath(protoImports[i]))

			if err := t.e.checkForDependencyCycle(ctx, res, []ResolvedPath{pr.ResolvedPath, res.resolvedPath}, span, checked); err != nil {
				return nil, err
			}
			select {
			case <-res.ready:
				if res.err != nil {
					if rerr, ok := res.err.(errFailedToResolve); ok {
						// We don't report errors to get file from resolver to handler since
						// it's usually considered immediately fatal. However, if the reason
						// we were resolving is due to an import, turn this into an error with
						// source position that pinpoints the import statement and report it.
						if err := t.h.HandleErrorWithPos(findImportSpan(parseRes, rerr.path), rerr); err != nil {
							return nil, err
						}
						continue
					}
					if errors.Is(res.err, reporter.ErrInvalidSource) {
						// continue if the handler has suppressed all errors, to allow
						// link errors to be reported later
						continue
					}
					return nil, res.err
				}
				deps[i] = res.res
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if descriptorProtoRes != nil {
			select {
			case <-descriptorProtoRes.ready:
				// descriptor.proto wasn't explicitly imported, so we can ignore a failure
				if descriptorProtoRes.err == nil {
					overrideDescriptorProto = descriptorProtoRes.res
				}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		// all deps resolved
		// t.r.setBlockedOn(nil) // todo: logic moved to the complete() and fail() handlers, seems to work fine so far
		// reacquire semaphore so we can proceed
		if err := t.e.s.Acquire(ctx, 1); err != nil {
			return nil, err
		}
		t.released = false
	}

	return t.link(parseRes, deps, overrideDescriptorProto)
}

func (e *executor) checkForDependencyCycle(ctx context.Context, res *result, sequence []ResolvedPath, span ast.SourceSpan, checked map[ResolvedPath]struct{}) error {
	res.mu.Lock()
	defer res.mu.Unlock()

	if _, ok := checked[res.resolvedPath]; ok {
		// already checked this one
		return nil
	}
	checked[res.resolvedPath] = struct{}{}
	deps := res.blockedOn
	for _, dep := range deps {
		select {
		case <-dep.resolved:
		case <-ctx.Done():
			return ctx.Err()
		}

		// is this a cycle?
		for _, file := range sequence {
			if file == dep.ResolvedPath {
				handleImportCycle(e.h, span, sequence, dep.ImportedAs)
				return e.h.Error()
			}
		}
		e.mu.Lock()
		depRes := e.results[dep.ResolvedPath]
		e.mu.Unlock()
		if depRes == nil {
			continue
		}
		if err := e.checkForDependencyCycle(ctx, depRes, append(sequence, dep.ResolvedPath), span, checked); err != nil {
			return err
		}
	}
	return nil
}

func handleImportCycle(h *reporter.Handler, span ast.SourceSpan, importSequence []ResolvedPath, dep UnresolvedPath) {
	var buf bytes.Buffer
	buf.WriteString("cycle found in imports: ")
	for _, imp := range importSequence {
		_, _ = fmt.Fprintf(&buf, "%q -> ", imp)
	}
	_, _ = fmt.Fprintf(&buf, "%q", dep)
	// error is saved and returned in caller
	_ = h.HandleErrorf(span, buf.String())
}

func findImportSpan(res parser.Result, dep UnresolvedPath) ast.SourceSpan {
	root := res.AST()
	if root == nil {
		return ast.UnknownSpan(res.FileNode().Name())
	}
	for _, decl := range root.Decls {
		if imp, ok := decl.(*ast.ImportNode); ok {
			if imp.Name.AsString() == string(dep) {
				return root.NodeInfo(imp.Name)
			}
		}
	}
	// this should never happen...
	return ast.UnknownSpan(res.FileNode().Name())
}

func (t *task) link(parseRes parser.Result, deps linker.Files, overrideDescriptorProtoRes linker.File) (linker.Result, error) {
	t.e.symTxLock.Lock()
	pendingSymtab := t.e.sym.Clone()
	file, err := linker.Link(parseRes, deps, pendingSymtab, t.h)
	if err != nil {
		// If a link error occurs, do not commit the updated symbol table, as it may
		// be in an inconsistent state.
		t.e.symTxLock.Unlock()
		return file, err
	}
	// commit the updated symbol table
	t.e.sym = pendingSymtab
	t.e.symTxLock.Unlock()

	var interpretOpts []options.InterpreterOption
	if overrideDescriptorProtoRes != nil {
		interpretOpts = []options.InterpreterOption{options.WithOverrideDescriptorProto(overrideDescriptorProtoRes)}
	}

	optsIndex, descIndex, err := options.InterpretOptions(file, t.h, interpretOpts...)
	if err != nil {
		return file, err
	}
	// now that options are interpreted, we can do some additional checks
	if err := file.ValidateOptions(t.h); err != nil {
		return file, err
	}
	if t.r.explicitFile {
		file.CheckForUnusedImports(t.h)
	}

	if needsSourceInfo(parseRes, t.e.c.SourceInfoMode) {
		var srcInfoOpts []sourceinfo.GenerateOption
		if t.e.c.SourceInfoMode&SourceInfoExtraComments != 0 {
			srcInfoOpts = append(srcInfoOpts, sourceinfo.WithExtraComments())
		}
		if t.e.c.SourceInfoMode&SourceInfoExtraOptionLocations != 0 {
			srcInfoOpts = append(srcInfoOpts, sourceinfo.WithExtraOptionLocations())
		}
		parseRes.FileDescriptorProto().SourceCodeInfo = sourceinfo.GenerateSourceInfo(parseRes.AST(), optsIndex, srcInfoOpts...)
		file.PopulateSourceCodeInfo(optsIndex, descIndex)
	}

	if !t.e.c.RetainASTs {
		file.RemoveAST()
	}
	return file, nil
}

func needsSourceInfo(parseRes parser.Result, mode SourceInfoMode) bool {
	return mode != SourceInfoNone && parseRes.AST() != nil && parseRes.FileDescriptorProto().SourceCodeInfo == nil
}

func (t *task) asParseResult(r *SearchResult) (parser.Result, error) {
	if r.ParseResult != nil {
		if r.ParseResult.FileDescriptorProto().GetName() != string(r.ResolvedPath) {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", r.ResolvedPath, r.ParseResult.FileDescriptorProto().GetName())
		}
		// If the file descriptor needs linking, it will be mutated during the
		// next stage. So to make anu mutations thread-safe, we must make a
		// defensive copy.
		res := parser.Clone(r.ParseResult)
		return res, nil
	}

	if r.Proto != nil {
		if r.Proto.GetName() != string(r.ResolvedPath) {
			*r.Proto.Name = string(r.ResolvedPath)
			// return nil, fmt.Errorf("search result for %q returned descriptor for %q", r.ResolvedPath, r.Proto.GetName())
		}

		// If the file descriptor needs linking, it will be mutated during the
		// next stage. So to make any mutations thread-safe, we must make a
		// defensive copy.
		descProto := proto.Clone(r.Proto).(*descriptorpb.FileDescriptorProto) //nolint:errcheck
		return parser.ResultWithoutAST(descProto), nil
	}

	file, err := t.asAST(r)
	if err != nil {
		if !errors.Is(err, reporter.ErrInvalidSource) || file == nil {
			return nil, err
		}
	}

	return parser.ResultFromAST(file, true, t.h)
}

func (t *task) asAST(r *SearchResult) (_ *ast.FileNode, _err error) {
	if r.AST != nil {
		if r.AST.Name() != string(r.ResolvedPath) {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", r.ResolvedPath, r.AST.Name())
		}
		return r.AST, nil
	}

	// defer func() {
	// 	if r := recover(); r != nil {
	// 		_err = fmt.Errorf("unknown parse error: %v", r)
	// 	}
	// }()

	return parser.Parse(string(r.ResolvedPath), r.Source, t.h)
}

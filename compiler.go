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
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/options"
	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/reporter"
	"github.com/bufbuild/protocompile/sourceinfo"
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
	PreInvalidate func(path string, reason string)
	// If not nil, called after a file (and all its dependencies) have been
	// invalidated.
	// The previous result is guaranteed to be equal to a result that was
	// returned in the single most recent call to Compile; for all other purposes
	// it should be treated as opaque.
	// If the file is no longer resolvable (if it was deleted, for example),
	// willRecompile will be set to false. Otherwise, it will be true.
	PostInvalidate func(path string, previousResult linker.File, willRecompile bool)
	// If not nil, called before a file is compiled.
	PreCompile func(path string)
	// If not nil, called after a file has been compiled.
	PostCompile func(path string)
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
	linker.SortedFiles
	UnlinkedParserResults map[string]parser.Result
}

// Compile compiles the given file names into fully-linked descriptors. The
// compiler's resolver is used to locate source code (or intermediate artifacts
// such as parsed ASTs or descriptor protos) and then do what is necessary to
// transform that into descriptors (parsing, linking, etc).
//
// Elements in the given returned files will implement [linker.Result] if the
// compiler had to link it (i.e. the resolver provided either a descriptor proto
// or source code). That result will contain a full AST for the file if the
// compiler had to parse it (i.e. the resolver provided source code for that
// file).
func (c *Compiler) Compile(ctx context.Context, files ...string) (CompileResult, error) {
	if len(files) == 0 {
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
			results: map[string]*result{},
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
	needsRecompile := e.invalidate(files...)
	results := make([]*result, 0, len(needsRecompile))

	e.mu.Lock()
	for _, f := range needsRecompile {
		results = append(results, e.compileLocked(ctx, f, true))
	}
	e.mu.Unlock()

	descs := make(linker.Files, 0, len(results))
	unlinked := make(map[string]parser.Result)
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
		} else if r.parseRes != nil {
			unlinked[r.name] = r.parseRes
		}
	}

	if c.IncludeDependenciesInResults {
		descs = linker.ComputeReflexiveTransitiveClosure(descs)
	}

	if err := h.Error(); err != nil {
		return CompileResult{
			SortedFiles:           descs.Sort(),
			UnlinkedParserResults: unlinked,
		}, err
	}
	// this should probably never happen; if any task returned an
	// error, h.Error() should be non-nil
	return CompileResult{
		SortedFiles:           descs.Sort(),
		UnlinkedParserResults: unlinked,
	}, firstError
}

type result struct {
	name  string
	ready chan struct{}

	// true if this file was explicitly provided to the compiler; otherwise
	// this file is an import that is implicitly included
	explicitFile bool

	// produces a linker.File or error, only available when ready is closed
	res linker.Result
	// parser result, may be available if linking fails but the file is syntactically valid
	parseRes parser.Result
	err      error

	mu sync.Mutex
	// the results that are dependencies of this result; this result is
	// blocked, waiting on these dependencies to complete
	blockedOn     []string
	lastBlockedOn []string
}

func (r *result) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.err = err
	r.res = nil
	r.parseRes = nil

	r.lastBlockedOn, r.blockedOn = r.blockedOn, nil
	close(r.ready)
}

func (r *result) failPartial(parseRes parser.Result, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.err = err
	r.res = nil
	r.parseRes = parseRes

	r.lastBlockedOn, r.blockedOn = r.blockedOn, nil
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

	r.lastBlockedOn, r.blockedOn = r.blockedOn, nil
	close(r.ready)
}

func (r *result) setBlockedOn(deps []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockedOn = deps
}

func (r *result) getBlockedOn() []string {
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
	results map[string]*result

	hooks CompilerHooks
}

func (e *executor) compile(ctx context.Context, file string) *result {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.compileLocked(ctx, file, false)
}

func (e *executor) invalidate(files ...string) []string {
	// remove the result from the cache, along with any results that depend on it
	e.mu.Lock()
	defer e.mu.Unlock()

	invalidated := map[string]struct{}{}
	blocks := map[*result][]*result{}
	for _, res := range e.results {
		for _, dep := range res.lastBlockedOn {
			blocks[e.results[dep]] = append(blocks[e.results[dep]], res)
		}
	}
	for _, file := range files {
		r := e.results[file]
		if r == nil {
			invalidated[file] = struct{}{}
			continue
		}

		e.invalidateLocked(r, blocks, invalidated, "file was modified")
	}

	filenames := make([]string, 0, len(invalidated))
	for name := range invalidated {
		if _, err := e.c.Resolver.FindFileByPath(name); err != nil {
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

func (e *executor) invalidateLocked(r *result, blocks map[*result][]*result, seen map[string]struct{}, reason string) {
	if _, ok := seen[r.name]; ok {
		return
	}
	if e.hooks.PreInvalidate != nil {
		e.hooks.PreInvalidate(r.name, reason)
	}
	if e.hooks.PostInvalidate != nil {
		defer func() {
			if err := recover(); err != nil {
				panic(err)
			}
			_, err := e.c.Resolver.FindFileByPath(r.name)
			e.hooks.PostInvalidate(r.name, r.res, err == nil)
		}()
	}
	seen[r.name] = struct{}{}
	if r.res != nil {
		// if an error occurred, r.res will be nil. we can skip deleting
		// the file in that case, since the link error should cause the
		// partially updated symbol table to be discarded.
		if err := e.sym.Delete(r.res, e.h); err != nil {
			panic(err)
		}
	}
	delete(e.results, r.name)

	for _, dep := range blocks[r] {
		// fmt.Printf(" => invalidating %s (depends on %s)\n", dep.name, r.name)
		e.invalidateLocked(dep, blocks, seen, fmt.Sprintf("file depends on %s", r.name))
	}
}

func (e *executor) compileLocked(ctx context.Context, file string, explicitFile bool) *result {
	r := e.results[file]
	if r != nil {
		return r
	}

	// fmt.Printf("compiling %s\n", file)

	r = &result{
		name:         file,
		ready:        make(chan struct{}),
		explicitFile: explicitFile,
	}
	e.results[file] = r
	go func() {
		defer func() {
			// if p := recover(); p != nil {
			// 	if r.err == nil {
			// 		// TODO: strip top frames from stack trace so that the panic is
			// 		//  the top of the trace?
			// 		panicErr := PanicError{File: file, Value: p, Stack: string(debug.Stack())}
			// 		r.fail(panicErr)
			// 	}
			// 	// TODO: if r.err != nil, then this task has already
			// 	//  failed and there's nothing we can really do to
			// 	//  communicate this panic to parent goroutine. This
			// 	//  means the panic must have happened *after* the
			// 	//  failure was already recorded (or during?)
			// 	//  It would be nice to do something else here, like
			// 	//  send the compiler an out-of-band error? Or log?
			// }

			if e.hooks.PostCompile != nil {
				e.hooks.PostCompile(file)
			}
		}()
		e.doCompile(ctx, file, r)
	}()
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
	path string
}

func (e errFailedToResolve) Error() string {
	errMsg := e.err.Error()
	if strings.Contains(errMsg, e.path) {
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
		res, err := e.c.Resolver.FindFileByPath(descriptorProtoPath)
		e.descriptorProtoIsCustom = err == nil && res.Desc != standardImports[descriptorProtoPath]
	})
	return e.descriptorProtoIsCustom
}

func (e *executor) doCompile(ctx context.Context, file string, r *result) {
	t := task{e: e, h: e.h.SubHandler(), r: r}
	if err := e.s.Acquire(ctx, 1); err != nil {
		r.fail(err)
		return
	}
	defer t.release()

	if e.hooks.PreCompile != nil {
		e.hooks.PreCompile(file)
	}

	sr, err := e.c.Resolver.FindFileByPath(file)
	if err != nil {
		r.fail(errFailedToResolve{err: err, path: file})
		return
	}

	defer func() {
		// if results included a result, don't leave it open if it can be closed
		if sr.Source == nil {
			return
		}
		if c, ok := sr.Source.(io.Closer); ok {
			_ = c.Close()
		}
	}()

	desc, err := t.asFile(ctx, file, &sr)
	if err != nil {
		if sr.ParseResult != nil {
			r.failPartial(sr.ParseResult, err)
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

func (t *task) asFile(ctx context.Context, name string, pr *SearchResult) (linker.Result, error) {
	r := *pr
	if r.Desc != nil {
		if r.Desc.Path() != name {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", name, r.Desc.Path())
		}
		// return linker.NewFileRecursive(r.Desc)
	}

	parseRes, err := t.asParseResult(name, r)
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
	imports := fileDescriptorProto.Dependency

	if t.e.hasOverrideDescriptorProto() {
		// we only consider implicitly including descriptor.proto if it's overridden
		if name != descriptorProtoPath {
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
				importsCopy := make([]string, len(imports)+1)
				copy(importsCopy, imports)
				importsCopy[len(imports)] = descriptorProtoPath
				imports = importsCopy
			}
		}
	}

	var overrideDescriptorProto linker.File
	if len(imports) > 0 {
		t.r.setBlockedOn(imports)

		results := make([]*result, 0, len(fileDescriptorProto.Dependency))
		checked := map[string]struct{}{}
		for _, dep := range fileDescriptorProto.Dependency {
			pos := findImportPos(parseRes, dep)
			if name == dep {
				// doh! file imports itself
				handleImportCycle(t.h, pos, []string{name}, dep)
				return nil, t.h.Error()
			}

			res := t.e.compile(ctx, dep)
			// check for dependency cycle to prevent deadlock
			if err := t.e.checkForDependencyCycle(res, []string{name, dep}, pos, checked); err != nil {
				return nil, err
			}
			results = append(results, res)
		}
		deps = make(linker.Files, 0, len(results))
		var descriptorProtoRes *result
		if wantsDescriptorProto {
			descriptorProtoRes = t.e.compile(ctx, descriptorProtoPath)
		}

		// release our semaphore so dependencies can be processed w/out risk of deadlock
		t.e.s.Release(1)
		t.released = true

		// now we wait for them all to be computed
		for _, res := range results {
			select {
			case <-res.ready:
				if res.err != nil {
					if rerr, ok := res.err.(errFailedToResolve); ok {
						// We don't report errors to get file from resolver to handler since
						// it's usually considered immediately fatal. However, if the reason
						// we were resolving is due to an import, turn this into an error with
						// source position that pinpoints the import statement and report it.
						if err := t.h.HandleErrorWithPos(findImportPos(parseRes, res.name), rerr); err != nil {
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
				deps = append(deps, res.res)
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

func (e *executor) checkForDependencyCycle(res *result, sequence []string, pos ast.SourcePosInfo, checked map[string]struct{}) error {
	if _, ok := checked[res.name]; ok {
		// already checked this one
		return nil
	}
	checked[res.name] = struct{}{}
	deps := res.getBlockedOn()
	for _, dep := range deps {
		// is this a cycle?
		for _, file := range sequence {
			if file == dep {
				handleImportCycle(e.h, pos, sequence, dep)
				return e.h.Error()
			}
		}

		e.mu.Lock()
		depRes := e.results[dep]
		e.mu.Unlock()
		if depRes == nil {
			continue
		}
		if err := e.checkForDependencyCycle(depRes, append(sequence, dep), pos, checked); err != nil {
			return err
		}
	}
	return nil
}

func handleImportCycle(h *reporter.Handler, pos ast.SourcePosInfo, importSequence []string, dep string) {
	var buf bytes.Buffer
	buf.WriteString("cycle found in imports: ")
	for _, imp := range importSequence {
		_, _ = fmt.Fprintf(&buf, "%q -> ", imp)
	}
	_, _ = fmt.Fprintf(&buf, "%q", dep)
	// error is saved and returned in caller
	_ = h.HandleErrorf(pos, buf.String())
}

func findImportPos(res parser.Result, dep string) ast.SourcePosInfo {
	root := res.AST()
	if root == nil {
		return ast.UnknownPosInfo(res.FileNode().Name())
	}
	for _, decl := range root.Decls {
		if imp, ok := decl.(*ast.ImportNode); ok {
			if imp.Name.AsString() == dep {
				return root.NodeInfo(imp.Name)
			}
		}
	}
	// this should never happen...
	return ast.UnknownPosInfo(res.FileNode().Name())
}

func (t *task) link(parseRes parser.Result, deps linker.Files, overrideDescriptorProtoRes linker.File) (linker.Result, error) {
	t.e.symTxLock.Lock()
	pendingSymtab := t.e.sym.Clone()
	file, err := linker.Link(parseRes, deps, pendingSymtab, t.h)
	if err != nil {
		// If a link error occurs, do not commit the updated symbol table, as it may
		// be in an inconsistent state.
		t.e.symTxLock.Unlock()
		return nil, err
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
		return nil, err
	}
	// now that options are interpreted, we can do some additional checks
	if err := file.ValidateOptions(t.h); err != nil {
		return nil, err
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

func (t *task) asParseResult(name string, r SearchResult) (parser.Result, error) {
	if r.ParseResult != nil {
		if r.ParseResult.FileDescriptorProto().GetName() != name {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", name, r.ParseResult.FileDescriptorProto().GetName())
		}
		// If the file descriptor needs linking, it will be mutated during the
		// next stage. So to make anu mutations thread-safe, we must make a
		// defensive copy.
		res := parser.Clone(r.ParseResult)
		return res, nil
	}

	if r.Proto != nil {
		if r.Proto.GetName() != name {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", name, r.Proto.GetName())
		}
		// If the file descriptor needs linking, it will be mutated during the
		// next stage. So to make any mutations thread-safe, we must make a
		// defensive copy.
		descProto := proto.Clone(r.Proto).(*descriptorpb.FileDescriptorProto) //nolint:errcheck
		return parser.ResultWithoutAST(descProto), nil
	}

	file, err := t.asAST(name, r)
	if err != nil {
		if !errors.Is(err, reporter.ErrInvalidSource) || file == nil {
			return nil, err
		}
	}

	return parser.ResultFromAST(file, true, t.h)
}

func (t *task) asAST(name string, r SearchResult) (*ast.FileNode, error) {
	if r.AST != nil {
		if r.AST.Name() != name {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", name, r.AST.Name())
		}
		return r.AST, nil
	}

	return parser.Parse(name, r.Source, t.h)
}

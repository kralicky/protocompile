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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/protointernal"
	"github.com/kralicky/protocompile/protoutil"
	"github.com/kralicky/protocompile/reporter"
	"github.com/kralicky/protocompile/walk"
)

const unknownFilePath = "<unknown file>"

// Symbols is a symbol table that maps names for all program elements to their
// location in source. It also tracks extension tag numbers. This can be used
// to enforce uniqueness for symbol names and tag numbers across many files and
// many link operations.
//
// This type is thread-safe.
type Symbols struct {
	filesMu sync.RWMutex
	files   map[string]fileEntry
	pkgTrie packageSymbols
}

func NewSymbolTable() *Symbols {
	return &Symbols{
		files:   make(map[string]fileEntry),
		pkgTrie: *newPackageSymbols("", nil),
	}
}

type packageSymbols struct {
	fqn    protoreflect.FullName
	parent *packageSymbols

	mu       sync.RWMutex
	children map[protoreflect.FullName]*packageSymbols
	symbols  map[protoreflect.FullName]symbolEntry
	exts     map[extNumber]ast.SourceSpan
}

func (ps *packageSymbols) isEmpty() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.isEmptyLocked()
}

func (ps *packageSymbols) isEmptyLocked() bool {
	return len(ps.children) == 0 && len(ps.symbols) == 0 && len(ps.exts) == 0
}

func (ps *packageSymbols) cascadeDeleteEmptyLocked() {
	if ps.isEmptyLocked() {
		parent := ps.parent
		child := ps
		for parent != nil {
			delete(parent.children, child.fqn)
			delete(parent.symbols, child.fqn)
			if !parent.isEmptyLocked() {
				break
			}
			child = parent
			parent = parent.parent
		}
	}
}

func newPackageSymbols(fqn protoreflect.FullName, parent *packageSymbols) *packageSymbols {
	return &packageSymbols{
		fqn:      fqn,
		parent:   parent,
		children: make(map[protoreflect.FullName]*packageSymbols),
		symbols:  make(map[protoreflect.FullName]symbolEntry),
		exts:     make(map[extNumber]ast.SourceSpan),
	}
}

func (s *Symbols) MarshalText() string {
	s.pkgTrie.mu.RLock()
	defer s.pkgTrie.mu.RUnlock()
	return s.pkgTrie.MarshalText()
}

func (ps *packageSymbols) MarshalText() string {
	var buf strings.Builder
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	symbolCount := 0
	for symName, sym := range ps.symbols {
		if sym.isPackage {
			fmt.Fprintf(&buf, "symbol (package): %s [@%s]\n", symName, sym.span)
			continue
		}
		symbolCount++
	}
	fmt.Fprintf(&buf, "symbols (regular): %d\n", symbolCount)
	for ext, span := range ps.exts {
		fmt.Fprintf(&buf, "extension: %s [@%s]\n", ext.extendee.Name(), span.Start())
	}
	for pkgName, pkg := range ps.children {
		pkg.mu.RLock()
		fmt.Fprintf(&buf, " > package: %s\n", pkgName)
		pkg.mu.RUnlock()
		pkgText := pkg.MarshalText()
		pkgText = " > " + strings.Replace(pkgText, "\n", "\n > ", -1)
		buf.WriteString(pkgText)
	}

	return buf.String()
}

type extNumber struct {
	extendee protoreflect.FullName
	tag      protoreflect.FieldNumber
}

type symbolEntry struct {
	span        ast.SourceSpan
	isEnumValue bool
	isPackage   bool
}

type fileEntry struct {
	refcount int // number of times this file is imported
}

func (s *Symbols) Clone() *Symbols {
	if s == nil {
		return nil
	}
	s.pkgTrie.mu.RLock()
	defer s.pkgTrie.mu.RUnlock()
	s.filesMu.RLock()
	defer s.filesMu.RUnlock()
	return &Symbols{
		pkgTrie: *s.pkgTrie.clone(nil),
		files:   maps.Clone(s.files),
	}
}

func (ps *packageSymbols) clone(newParent *packageSymbols) *packageSymbols {
	if ps == nil {
		return nil
	}
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	clone := &packageSymbols{
		fqn:      ps.fqn,
		parent:   newParent,
		children: make(map[protoreflect.FullName]*packageSymbols, len(ps.children)),
		symbols:  make(map[protoreflect.FullName]symbolEntry, len(ps.symbols)),
		exts:     make(map[extNumber]ast.SourceSpan, len(ps.exts)),
	}
	clone.mu.Lock()
	defer clone.mu.Unlock()

	for k, v := range ps.children {
		clone.children[k] = v.clone(clone)
	}
	for k, v := range ps.symbols {
		clone.symbols[k] = v
	}
	for k, v := range ps.exts {
		clone.exts[k] = v
	}
	return clone
}

// Import populates the symbol table with all symbols/elements and extension
// tags present in the given file descriptor. If s is nil or if fd has already
// been imported into s, this returns immediately without doing anything. If any
// collisions in symbol names or extension tags are identified, an error will be
// returned and the symbol table will not be updated.
func (s *Symbols) Import(fd protoreflect.FileDescriptor, handler *reporter.Handler) error {
	if s == nil {
		return nil
	}

	if f, ok := fd.(*file); ok {
		// unwrap any file instance
		fd = f.FileDescriptor
	}

	var pkgSpan ast.SourceSpan
	if res, ok := fd.(*result); ok {
		pkgSpan = packageNameSpan(res)
	} else {
		pkgSpan = sourceSpanForPackage(fd)
	}
	pkg, err := s.importPackages(pkgSpan, fd.Package(), handler)
	if err != nil || pkg == nil {
		return err
	}

	s.filesMu.Lock()
	entry := s.files[fd.Path()]
	alreadyImported := entry.refcount > 0
	s.files[fd.Path()] = fileEntry{
		refcount: entry.refcount + 1,
	}
	s.filesMu.Unlock()

	for i := 0; i < fd.Imports().Len(); i++ {
		imp := fd.Imports().Get(i)
		if imp.IsPlaceholder() {
			continue
		}
		if err := s.Import(imp.FileDescriptor, handler); err != nil {
			return err
		}
	}

	if alreadyImported {
		return nil
	}

	if res, ok := fd.(*result); ok && res.hasSource() {
		return s.importResultWithExtensions(pkg, res, handler)
	}

	return s.importFileWithExtensions(pkg, fd, handler)
}

var (
	ErrFileStillInUse = errors.New("file still in use")
	ErrFileNotFound   = errors.New("file not found")
)

type SymbolCollisionError struct {
	error
	recoverable       bool
	entangledFiles    []string
	entangledPackages []string
}

func (u *SymbolCollisionError) Unwrap() error {
	return u.error
}

func (u *SymbolCollisionError) EntangledFiles() []string {
	return u.entangledFiles
}

func (u *SymbolCollisionError) EntangledPackages() []string {
	return u.entangledPackages
}

// Deletes all symbols associated with the given file descriptor.
func (s *Symbols) Delete(fd protoreflect.FileDescriptor, handler *reporter.Handler) error {
	if s == nil {
		return nil
	}

	if f, ok := fd.(*file); ok {
		// unwrap any file instance
		fd = f.FileDescriptor
	}

	for i := 0; i < fd.Imports().Len(); i++ {
		imp := fd.Imports().Get(i)
		if imp.IsPlaceholder() {
			continue
		}
		if err := s.Delete(imp.FileDescriptor, handler); err != nil {
			if !errors.Is(err, ErrFileStillInUse) {
				return err
			}
		}
	}

	if err := s.prepareDeleteFile(fd, handler); err != nil {
		return err
	}

	err := s.deleteFileLocked(fd, handler)
	if err != nil {
		return err
	}

	return nil
}

func (s *Symbols) prepareDeleteFile(fd protoreflect.FileDescriptor, handler *reporter.Handler) error {
	f, ok := s.files[fd.Path()]
	if !ok {
		return fmt.Errorf("%w: %s", ErrFileNotFound, fd.Path())
	}
	f.refcount--
	s.files[fd.Path()] = f
	if f.refcount > 0 {
		return ErrFileStillInUse
	}
	return nil
}

func (s *Symbols) importFileWithExtensions(pkg *packageSymbols, fd protoreflect.FileDescriptor, handler *reporter.Handler) error {
	imported, err := pkg.importFile(fd, handler)
	if err != nil {
		return err
	}
	if !imported {
		// nothing else to do
		return nil
	}

	return walk.Descriptors(fd, func(d protoreflect.Descriptor) error {
		fld, ok := d.(protoreflect.FieldDescriptor)
		if !ok || !fld.IsExtension() {
			return nil
		}
		span := sourceSpanForNumber(fld)
		extendee := fld.ContainingMessage()
		return s.AddExtension(packageFor(extendee), extendee.FullName(), fld.Number(), span, handler)
	})
}

func (ps *packageSymbols) importFile(fd protoreflect.FileDescriptor, handler *reporter.Handler) (bool, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// first pass: check for conflicts
	if err := ps.checkFileLocked(fd, handler); err != nil {
		return false, err
	}
	if err := handler.Error(); err != nil {
		if !errors.Is(err, reporter.ErrInvalidSource) {
			return false, err
		}
	}

	// second pass: commit all symbols
	ps.commitFileLocked(fd)

	return true, nil
}

func (ps *packageSymbols) deleteFile(fd protoreflect.FileDescriptor, handler *reporter.Handler) (deletedExtensions []extNumber) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	return ps.deleteFileLocked(fd)
}

func (s *Symbols) importPackages(pkgSpan ast.SourceSpan, pkg protoreflect.FullName, handler *reporter.Handler) (*packageSymbols, error) {
	cur := &s.pkgTrie
	enumerator := nameEnumerator{name: pkg}
	for {
		p, ok := enumerator.next()
		if !ok {
			return cur, nil
		}
		var err error
		cur, err = cur.importPackage(pkgSpan, p, handler)
		if err != nil {
			return nil, err
		}
		if cur == nil {
			return nil, nil
		}
	}
}

func (ps *packageSymbols) importPackage(pkgSpan ast.SourceSpan, pkg protoreflect.FullName, handler *reporter.Handler) (*packageSymbols, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	existing, ok := ps.symbols[pkg]
	var child *packageSymbols
	if ok && existing.isPackage {
		// package already exists
		child = ps.children[pkg]

		ps.symbols[pkg] = existing

		return child, nil
	} else if ok {
		return nil, reportSymbolCollision(symbolEntry{span: pkgSpan, isPackage: true}, pkg, false, existing, handler)
	}

	ps.symbols[pkg] = symbolEntry{span: pkgSpan, isPackage: true}
	child = newPackageSymbols(pkg, ps)
	ps.children[pkg] = child
	return child, nil
}

func (s *Symbols) getPackage(pkg protoreflect.FullName) *packageSymbols {
	if strings.HasPrefix(string(pkg), ".") {
		fmt.Printf("BUG: package name %q should not start with '.'\n", pkg)
		return nil
	}

	cur := &s.pkgTrie
	enumerator := nameEnumerator{name: pkg}
	for {
		p, ok := enumerator.next()
		if !ok {
			return cur
		}
		cur.mu.RLock()
		next := cur.children[p]
		cur.mu.RUnlock()

		if next == nil {
			return nil
		}
		cur = next
	}
}

func reportSymbolCollision(sym symbolEntry, fqn protoreflect.FullName, additionIsEnumVal bool, existing symbolEntry, handler *reporter.Handler) error {
	// because of weird scoping for enum values, provide more context in error message
	// if this conflict is with an enum value
	var suffix string

	recoverable := true
	if existing.isPackage != sym.isPackage {
		suffix = " (package name conflicts with symbol)"
		recoverable = false
	} else if additionIsEnumVal || existing.isEnumValue {
		suffix = "; protobuf uses C++ scoping rules for enum values, so they exist in the scope enclosing the enum"
	}

	handler.HandleErrorf(sym.span, "%w%s", reporter.SymbolRedeclared(string(fqn), existing.span), suffix)
	handler.HandleErrorf(existing.span, "%w%s", reporter.SymbolRedeclared(string(fqn), sym.span), suffix)

	ue := &SymbolCollisionError{
		recoverable: recoverable,
		error:       handler.Error(),
	}
	existingFilename := existing.span.Start().Filename
	if existingFilename != sym.span.Start().Filename {
		if !slices.Contains(ue.entangledFiles, existingFilename) {
			ue.entangledFiles = append(ue.entangledFiles, existingFilename)
		}
		if existing.isPackage {
			ue.entangledPackages = append(ue.entangledPackages, string(fqn))
		}
	}
	return ue
}

func posLess(a, b ast.SourcePos) bool {
	if a.Filename == b.Filename {
		if a.Line == b.Line {
			return a.Col < b.Col
		}
		return a.Line < b.Line
	}
	return false
}

func (ps *packageSymbols) checkFileLocked(f protoreflect.FileDescriptor, handler *reporter.Handler) error {
	return walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		span := sourceSpanFor(d)
		if existing, ok := ps.symbols[d.FullName()]; ok {
			_, isEnumVal := d.(protoreflect.EnumValueDescriptor)
			if err := reportSymbolCollision(symbolEntry{span: span}, d.FullName(), isEnumVal, existing, handler); err != nil {
				return err
			}
		}
		return nil
	})
}

func sourceSpanForPackage(fd protoreflect.FileDescriptor) ast.SourceSpan {
	loc := fd.SourceLocations().ByPath([]int32{protointernal.FilePackageTag})
	if protointernal.IsZeroSourceLocation(loc) {
		return ast.UnknownSpan(fd.Path())
	}
	return ast.NewSourceSpan(
		ast.SourcePos{
			Filename: fd.Path(),
			Line:     loc.StartLine,
			Col:      loc.StartColumn,
		},
		ast.SourcePos{
			Filename: fd.Path(),
			Line:     loc.EndLine,
			Col:      loc.EndColumn,
		},
	)
}

func sourceSpanFor(d protoreflect.Descriptor) ast.SourceSpan {
	file := d.ParentFile()
	if file == nil {
		return ast.UnknownSpan(unknownFilePath)
	}
	path, ok := protointernal.ComputeSourcePath(d)
	if !ok {
		return ast.UnknownSpan(file.Path())
	}
	if result, ok := file.(*result); ok {
		return nameSpan(result.FileNode(), result.Node(protoutil.ProtoFromDescriptor(d)))
	}

	namePath := path
	switch d.(type) {
	case protoreflect.FieldDescriptor:
		namePath = append(namePath, protointernal.FieldNameTag)
	case protoreflect.MessageDescriptor:
		namePath = append(namePath, protointernal.MessageNameTag)
	case protoreflect.OneofDescriptor:
		namePath = append(namePath, protointernal.OneofNameTag)
	case protoreflect.EnumDescriptor:
		namePath = append(namePath, protointernal.EnumNameTag)
	case protoreflect.EnumValueDescriptor:
		namePath = append(namePath, protointernal.EnumValNameTag)
	case protoreflect.ServiceDescriptor:
		namePath = append(namePath, protointernal.ServiceNameTag)
	case protoreflect.MethodDescriptor:
		namePath = append(namePath, protointernal.MethodNameTag)
	default:
		// NB: shouldn't really happen, but just in case fall back to path to
		// descriptor, sans name field
	}
	loc := file.SourceLocations().ByPath(namePath)
	if protointernal.IsZeroSourceLocation(loc) {
		loc = file.SourceLocations().ByPath(path)
		if protointernal.IsZeroSourceLocation(loc) {
			return ast.UnknownSpan(file.Path())
		}
	}
	return ast.NewSourceSpan(
		ast.SourcePos{
			Filename: file.Path(),
			Line:     loc.StartLine,
			Col:      loc.StartColumn,
		},
		ast.SourcePos{
			Filename: file.Path(),
			Line:     loc.EndLine,
			Col:      loc.EndColumn,
		},
	)
}

func sourceSpanForNumber(fd protoreflect.FieldDescriptor) ast.SourceSpan {
	file := fd.ParentFile()
	if file == nil {
		return ast.UnknownSpan(unknownFilePath)
	}
	path, ok := protointernal.ComputeSourcePath(fd)
	if !ok {
		return ast.UnknownSpan(file.Path())
	}
	numberPath := path
	numberPath = append(numberPath, protointernal.FieldNumberTag)
	loc := file.SourceLocations().ByPath(numberPath)
	if protointernal.IsZeroSourceLocation(loc) {
		loc = file.SourceLocations().ByPath(path)
		if protointernal.IsZeroSourceLocation(loc) {
			return ast.UnknownSpan(file.Path())
		}
	}
	start := ast.SourcePos{
		Filename: file.Path(),
		Line:     loc.StartLine,
		Col:      loc.StartColumn,
	}
	end := ast.SourcePos{
		Filename: file.Path(),
		Line:     loc.EndLine,
		Col:      loc.EndColumn,
	}
	return ast.NewSourceSpan(start, end)
}

func isZeroLoc(loc protoreflect.SourceLocation) bool {
	return loc.Path == nil &&
		loc.StartLine == 0 &&
		loc.StartColumn == 0 &&
		loc.EndLine == 0 &&
		loc.EndColumn == 0
}

func (ps *packageSymbols) commitFileLocked(f protoreflect.FileDescriptor) {
	_ = walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		span := sourceSpanFor(d)
		name := d.FullName()
		_, isEnumValue := d.(protoreflect.EnumValueDescriptor)
		ps.symbols[name] = symbolEntry{span: span, isEnumValue: isEnumValue}
		return nil
	})
}

func (ps *packageSymbols) deleteFileLocked(f protoreflect.FileDescriptor) (deletedExtensions []extNumber) {
	_ = walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		fqn := d.FullName()
		if fld, ok := d.(protoreflect.FieldDescriptor); ok && fld.IsExtension() {
			extNum := extNumber{extendee: fld.ContainingMessage().FullName(), tag: fld.Number()}
			deletedExtensions = append(deletedExtensions, extNum)
		} else if msg, ok := d.(protoreflect.MessageDescriptor); ok {
			ranges := msg.ExtensionRanges()
			for i := 0; i < ranges.Len(); i++ {
				rng := ranges.Get(i)
				for k, span := range ps.exts {
					if k.extendee == fqn && k.tag >= rng[0] && k.tag < rng[1] && span.Start().Filename == f.Path() {
						deletedExtensions = append(deletedExtensions, k)
					}
				}
			}
		}
		// only delete symbols if they were defined in the file being deleted
		if sym, ok := ps.symbols[fqn]; ok && sym.span.Start().Filename == f.Path() {
			delete(ps.symbols, fqn)
		}
		return nil
	})

	return
}

func (s *Symbols) importResultWithExtensions(pkg *packageSymbols, r *result, handler *reporter.Handler) error {
	imported, err := pkg.importResult(r, handler)
	if err != nil {
		return err
	}
	if !imported {
		// nothing else to do
		return nil
	}

	return walk.Descriptors(r, func(d protoreflect.Descriptor) error {
		fd, ok := d.(*extTypeDescriptor)
		if !ok {
			return nil
		}
		file := r.FileNode()
		node := r.FieldNode(fd.FieldDescriptorProto())
		info := file.NodeInfo(node.GetTag())
		extendee := fd.ContainingMessage()
		return s.AddExtension(packageFor(extendee), extendee.FullName(), fd.Number(), info, handler)
	})
}

func (s *Symbols) importResult(r *result, handler *reporter.Handler) error {
	s.filesMu.Lock()
	defer s.filesMu.Unlock()
	entry := s.files[r.Path()]
	alreadyImported := entry.refcount > 0
	s.files[r.Path()] = fileEntry{
		refcount: entry.refcount + 1,
	}
	if alreadyImported {
		return nil
	}
	pkg, err := s.importPackages(packageNameSpan(r), r.Package(), handler)
	if err != nil || pkg == nil {
		return err
	}
	_, err = pkg.importResult(r, handler)
	return err
}

func (s *Symbols) deleteFileLocked(fd protoreflect.FileDescriptor, handler *reporter.Handler) error {
	s.filesMu.Lock()
	defer s.filesMu.Unlock()

	pkgName := fd.Package()

	pkg := s.getPackage(pkgName)
	if pkg == nil {
		// Check for the case where the file exists and has a package name, but
		// no other symbols. If, for example, another file with the same package
		// imports this one and is invalidated, the package will become empty and
		// will have been cleaned up by the time we get here.
		if fd.Messages().Len() == 0 && fd.Enums().Len() == 0 && fd.Extensions().Len() == 0 && fd.Services().Len() == 0 {
			delete(s.files, fd.Path())
			return nil
		}
		return fmt.Errorf("%w: no such package %s", ErrFileNotFound, pkgName)
	}
	pkg.mu.Lock()
	deletedExtensions := pkg.deleteFileLocked(fd)
	if pkg.isEmptyLocked() {
		pkg.cascadeDeleteEmptyLocked()
	}
	pkg.mu.Unlock()

	// if any extensions were deleted, we need to delete them from the
	// extendee's symbols and from the global extension number map
	for _, ext := range deletedExtensions {
		extendeePkg := s.getPackage(ext.extendee.Parent())
		if extendeePkg == nil {
			continue
		}
		extendeePkg.mu.Lock()
		delete(extendeePkg.exts, ext)
		delete(s.pkgTrie.exts, ext) // delete from global map
		if extendeePkg.isEmptyLocked() {
			extendeePkg.cascadeDeleteEmptyLocked()
		}
		extendeePkg.mu.Unlock()
	}

	delete(s.files, fd.Path())

	return nil
}

func (ps *packageSymbols) importResult(r *result, handler *reporter.Handler) (bool, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// first pass: check for conflicts
	if err := ps.checkResultLocked(r, handler); err != nil {
		return false, err
	}
	if err := handler.Error(); err != nil {
		if !errors.Is(err, reporter.ErrInvalidSource) {
			return false, err
		}
	}

	// second pass: commit all symbols
	ps.commitFileLocked(r)

	return true, nil
}

func (ps *packageSymbols) checkResultLocked(r *result, handler *reporter.Handler) error {
	resultSyms := map[protoreflect.FullName]symbolEntry{}
	return walk.Descriptors(r, func(d protoreflect.Descriptor) error {
		_, isEnumVal := d.(protoreflect.EnumValueDescriptor)
		file := r.FileNode()
		fqn := d.FullName()
		node := r.Node(protoutil.ProtoFromDescriptor(d))
		span := nameSpan(file, node)
		// check symbols already in this symbol table
		if existing, ok := ps.symbols[fqn]; ok {
			return reportSymbolCollision(symbolEntry{span: span}, fqn, isEnumVal, existing, handler)
		}

		// also check symbols from this result (that are not yet in symbol table)
		if existing, ok := resultSyms[fqn]; ok {
			return reportSymbolCollision(symbolEntry{span: span}, fqn, isEnumVal, existing, handler)
		}
		resultSyms[fqn] = symbolEntry{
			span:        span,
			isEnumValue: isEnumVal,
		}

		return nil
	})
}

func packageNameSpan(r *result) ast.SourceSpan {
	if r == nil {
		return ast.UnknownSpan(unknownFilePath)
	}
	for _, decl := range r.FileNode().GetDecls() {
		if pkgNode := decl.GetPackage(); pkgNode != nil && !pkgNode.IsIncomplete() {
			return r.FileNode().NodeInfo(pkgNode.Name)
		}
	}

	return ast.UnknownSpan(r.Path())
}

func nameSpan(file *ast.FileNode, n ast.Node) ast.SourceSpan {
	if file == nil {
		return ast.UnknownSpan(unknownFilePath)
	}
	if n == nil {
		return ast.UnknownSpan(file.Name())
	}
	// TODO: maybe ast package needs a NamedNode interface to simplify this?
	switch n := n.(type) {
	case interface{ GetName() *ast.IdentNode }:
		return file.NodeInfo(n.GetName())
	default:
		return file.NodeInfo(n)
	}
}

// AddExtension records the given extension, which is used to ensure that no two files
// attempt to extend the same message using the same tag. The given pkg should be the
// package that defines extendee.
func (s *Symbols) AddExtension(pkg, extendee protoreflect.FullName, tag protoreflect.FieldNumber, span ast.SourceSpan, handler *reporter.Handler) error {
	if pkg != "" {
		if !strings.HasPrefix(string(extendee), string(pkg)+".") {
			return handler.HandleErrorf(span, "could not register extension: extendee %q does not match package %q", extendee, pkg)
		}
	}
	pkgSyms := s.getPackage(pkg)
	if pkgSyms == nil {
		return handler.HandleErrorf(span, "could not register extension: missing package symbols: %q", pkg)
	}

	pkgSyms.mu.Lock()
	defer pkgSyms.mu.Unlock()
	extNum := extNumber{extendee: extendee, tag: tag}
	if existing, ok := pkgSyms.exts[extNum]; ok {
		if err := handler.HandleErrorf(span, "extension with tag %d for message %s already defined at %v", tag, extendee, existing); err != nil {
			return err
		}
	} else if existing, ok := s.pkgTrie.exts[extNum]; ok {
		if err := handler.HandleErrorf(span, "extension with tag %d for message %s already defined at %v", tag, extendee, existing); err != nil {
			return err
		}
		// NB: even though this is an error, still keep track of the extension
		// within the package's symbol table
		pkgSyms.exts[extNum] = span
	} else {
		pkgSyms.exts[extNum] = span
		s.pkgTrie.exts[extNum] = span // also add to global map
	}
	return nil
}

// Lookup finds the registered location of the given name. If the given name has
// not been seen/registered, nil is returned.
func (s *Symbols) Lookup(name protoreflect.FullName) ast.SourceSpan {
	if pkgSyms := s.getPackage(name.Parent()); pkgSyms != nil {
		if entry, ok := pkgSyms.symbols[name]; ok {
			return entry.span
		}
	}
	return nil
}

// LookupExtension finds the registered location of the given extension. If the given
// extension has not been seen/registered, nil is returned.
func (s *Symbols) LookupExtension(messageName protoreflect.FullName, extensionNumber protoreflect.FieldNumber) ast.SourceSpan {
	if pkgSyms := s.getPackage(messageName.Parent()); pkgSyms != nil {
		if entry, ok := pkgSyms.exts[extNumber{messageName, extensionNumber}]; ok {
			return entry
		}
	}
	return nil
}

type nameEnumerator struct {
	name  protoreflect.FullName
	start int
}

func (e *nameEnumerator) next() (protoreflect.FullName, bool) {
	if e.start < 0 {
		return "", false
	}
	pos := strings.IndexByte(string(e.name[e.start:]), '.')
	if pos == -1 {
		e.start = -1
		return e.name, len(e.name) > 0 // note: changed from upstream `return e.name, true`, bug?
	}
	pos += e.start
	e.start = pos + 1
	return e.name[:pos], true
}

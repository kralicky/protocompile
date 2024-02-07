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
	"strings"
	"sync"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/internal"
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
	pkgTrie packageSymbols
}

func NewSymbolTable() *Symbols {
	return &Symbols{
		pkgTrie: *newPackageSymbols(),
	}
}

type packageSymbols struct {
	mu       sync.RWMutex
	children map[protoreflect.FullName]*packageSymbols
	files    map[string]struct{}
	symbols  map[protoreflect.FullName]symbolEntry
	exts     map[extNumber]ast.SourceSpan
}

func newPackageSymbols() *packageSymbols {
	return &packageSymbols{
		children: make(map[protoreflect.FullName]*packageSymbols),
		files:    make(map[string]struct{}),
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
	for file := range ps.files {
		fmt.Fprintf(&buf, "file: %q\n", file)
	}

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
	refcount    int // number of times this symbol has been imported
}

func (s *Symbols) Clone() *Symbols {
	if s == nil {
		return nil
	}
	s.pkgTrie.mu.RLock()
	defer s.pkgTrie.mu.RUnlock()
	return &Symbols{
		pkgTrie: *s.pkgTrie.clone(),
	}
}

func (ps *packageSymbols) clone() *packageSymbols {
	if ps == nil {
		return nil
	}
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	clone := newPackageSymbols()
	clone.mu.Lock()
	defer clone.mu.Unlock()

	for k, v := range ps.children {
		clone.children[k] = v.clone()
	}
	for k, v := range ps.files {
		clone.files[k] = v // this may or may not work, but i have no idea how to clone a protoreflect.FileDescriptor
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

	pkg.mu.RLock()
	_, alreadyImported := pkg.files[fd.Path()]
	pkg.mu.RUnlock()

	for i := 0; i < fd.Imports().Len(); i++ {
		if err := s.Import(fd.Imports().Get(i).FileDescriptor, handler); err != nil {
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
	ErrPackageStillInUse = errors.New("package still in use")
	ErrPackageNotFound   = errors.New("package not found")
	ErrFileNotFound      = errors.New("file not found")
)

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
		if err := s.Delete(fd.Imports().Get(i).FileDescriptor, handler); err != nil {
			return err
		}
	}

	err := s.prepareDeletePackages(fd.Package(), handler)
	if err != nil {
		switch {
		case errors.Is(err, ErrPackageStillInUse):
			// package still in use - can't delete
		case errors.Is(err, ErrPackageNotFound):
			// package doesn't exist - nothing to delete
			return nil
		default:
			// some other error
			return err
		}
	}

	if res, ok := fd.(*result); ok {
		err = s.deleteResultLocked(res, handler)
	} else {
		err = s.deleteFileLocked(fd, handler)
	}

	if err != nil {
		switch {
		case errors.Is(err, ErrPackageNotFound):
			// package doesn't exist - nothing to delete
			return nil
		case errors.Is(err, ErrFileNotFound):
			// file doesn't exist - nothing to delete
			return nil
		default:
			// some other error
			return err
		}
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

	if _, ok := ps.files[fd.Path()]; ok {
		// have to double-check if it's already imported, in case
		// it was added after above read-locked check
		return false, nil
	}

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

func (ps *packageSymbols) deleteFile(fd protoreflect.FileDescriptor, handler *reporter.Handler) (deletedExtensions []extNumber, _ error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, ok := ps.files[fd.Path()]; !ok {
		// already deleted
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, fd.Path())
	}

	// delete all symbols
	return ps.deleteFileLocked(fd), nil
}

func (s *Symbols) importPackages(pkgSpan ast.SourceSpan, pkg protoreflect.FullName, handler *reporter.Handler) (*packageSymbols, error) {
	if pkg == "" {
		return &s.pkgTrie, nil
	}

	parts := strings.Split(string(pkg), ".")
	for i := 1; i < len(parts); i++ {
		parts[i] = parts[i-1] + "." + parts[i]
	}

	cur := &s.pkgTrie
	for _, p := range parts {
		var err error
		cur, err = cur.importPackage(pkgSpan, protoreflect.FullName(p), handler)
		if err != nil {
			return nil, err
		}
		if cur == nil {
			return nil, nil
		}
	}

	return cur, nil
}

func (s *Symbols) prepareDeletePackages(pkg protoreflect.FullName, handler *reporter.Handler) error {
	parts := strings.Split(string(pkg), ".")
	curPkg := &s.pkgTrie
	pkgParts := []*packageSymbols{}
	if len(parts) > 1 {
		for i := 1; i < len(parts); i++ {
			parts[i] = parts[i-1] + "." + parts[i]
		}

		for i := 0; i < len(parts); i++ {
			next := curPkg.children[protoreflect.FullName(parts[i])]
			if next == nil {
				return ErrPackageNotFound
			}
			pkgParts = append(pkgParts, curPkg)
			curPkg = next
		}
	}

	var err error
	for i := len(pkgParts) - 1; i >= 0; i-- {
		p := pkgParts[i]
		err = p.prepareDeletePackage(protoreflect.FullName(parts[i]), handler)
		if err != nil {
			if !errors.Is(err, ErrPackageStillInUse) {
				return err
			}
		}
	}

	return err
}

func (ps *packageSymbols) importPackage(pkgSpan ast.SourceSpan, pkg protoreflect.FullName, handler *reporter.Handler) (*packageSymbols, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	existing, ok := ps.symbols[pkg]
	var child *packageSymbols
	if ok && existing.isPackage {
		// package already exists
		child = ps.children[pkg]

		// increase the reference count
		// fmt.Printf("[refcount] increasing %q from %d to %d\n", pkg, existing.refcount, existing.refcount+1)
		existing.refcount++
		ps.symbols[pkg] = existing

		return child, nil
	} else if ok {
		return nil, reportSymbolCollision(pkgSpan, pkg, false, existing, handler)
	}

	ps.symbols[pkg] = symbolEntry{span: pkgSpan, isPackage: true, refcount: 1}
	child = newPackageSymbols()
	ps.children[pkg] = child
	return child, nil
}

func (ps *packageSymbols) prepareDeletePackage(pkg protoreflect.FullName, handler *reporter.Handler) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	existing, ok := ps.symbols[pkg]
	if !ok || !existing.isPackage {
		// package doesn't exist
		return fmt.Errorf("bug(deletePackage): package %q does not exist", pkg)
	}

	// fmt.Printf("[refcount] decreasing %q from %d to %d\n", pkg, existing.refcount, existing.refcount-1)
	existing.refcount--
	ps.symbols[pkg] = existing
	if existing.refcount > 0 {
		// still in use
		return ErrPackageStillInUse
	}

	// refcount hit zero; this package is no longer imported anywhere, so it's ok to delete it
	return nil
}

func (s *Symbols) getPackage(pkg protoreflect.FullName) *packageSymbols {
	if pkg == "" {
		return &s.pkgTrie
	}

	if pkg[0] == '.' {
		fmt.Printf("BUG: package name %q should not start with '.'\n", pkg)
		return nil
	}

	parts := strings.Split(string(pkg), ".")
	for i := 1; i < len(parts); i++ {
		parts[i] = parts[i-1] + "." + parts[i]
	}

	cur := &s.pkgTrie
	for _, p := range parts {
		cur.mu.RLock()
		next := cur.children[protoreflect.FullName(p)]
		cur.mu.RUnlock()

		if next == nil {
			return nil
		}
		cur = next
	}

	return cur
}

func reportSymbolCollision(span ast.SourceSpan, fqn protoreflect.FullName, additionIsEnumVal bool, existing symbolEntry, handler *reporter.Handler) error {
	// because of weird scoping for enum values, provide more context in error message
	// if this conflict is with an enum value
	var suffix string
	if additionIsEnumVal || existing.isEnumValue {
		suffix = "; protobuf uses C++ scoping rules for enum values, so they exist in the scope enclosing the enum"
	}
	orig := existing.span
	conflict := span
	if posLess(conflict.Start(), orig.Start()) {
		orig, conflict = conflict, orig
	}
	var err error
	if existing.isPackage {
		err = reporter.AlreadyDefinedAsPkg(existing.span)
	} else {
		err = reporter.AlreadyDefined(existing.span)
	}
	return handler.HandleErrorf(conflict, "symbol %q %w%s", fqn, err, suffix)
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
			if err := reportSymbolCollision(span, d.FullName(), isEnumVal, existing, handler); err != nil {
				return err
			}
		}
		return nil
	})
}

func sourceSpanForPackage(fd protoreflect.FileDescriptor) ast.SourceSpan {
	loc := fd.SourceLocations().ByPath([]int32{internal.FilePackageTag})
	if internal.IsZeroLocation(loc) {
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
	path, ok := internal.ComputePath(d)
	if !ok {
		return ast.UnknownSpan(file.Path())
	}
	namePath := path
	switch d.(type) {
	case protoreflect.FieldDescriptor:
		namePath = append(namePath, internal.FieldNameTag)
	case protoreflect.MessageDescriptor:
		namePath = append(namePath, internal.MessageNameTag)
	case protoreflect.OneofDescriptor:
		namePath = append(namePath, internal.OneofNameTag)
	case protoreflect.EnumDescriptor:
		namePath = append(namePath, internal.EnumNameTag)
	case protoreflect.EnumValueDescriptor:
		namePath = append(namePath, internal.EnumValNameTag)
	case protoreflect.ServiceDescriptor:
		namePath = append(namePath, internal.ServiceNameTag)
	case protoreflect.MethodDescriptor:
		namePath = append(namePath, internal.MethodNameTag)
	default:
		// NB: shouldn't really happen, but just in case fall back to path to
		// descriptor, sans name field
	}
	loc := file.SourceLocations().ByPath(namePath)
	if internal.IsZeroLocation(loc) {
		loc = file.SourceLocations().ByPath(path)
		if internal.IsZeroLocation(loc) {
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
	path, ok := internal.ComputePath(fd)
	if !ok {
		return ast.UnknownSpan(file.Path())
	}
	numberPath := path
	numberPath = append(numberPath, internal.FieldNumberTag)
	loc := file.SourceLocations().ByPath(numberPath)
	if internal.IsZeroLocation(loc) {
		loc = file.SourceLocations().ByPath(path)
		if internal.IsZeroLocation(loc) {
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
	ps.files[f.Path()] = struct{}{}
}

func (ps *packageSymbols) deleteFileLocked(f protoreflect.FileDescriptor) (deletedExtensions []extNumber) {
	_ = walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		fqn := d.FullName()
		if fld, ok := d.(protoreflect.FieldDescriptor); ok && fld.IsExtension() {
			extNum := extNumber{extendee: packageFor(fld.ContainingMessage()), tag: fld.Number()}
			delete(ps.exts, extNum)
			deletedExtensions = append(deletedExtensions, extNum)
		} else if msg, ok := d.(protoreflect.MessageDescriptor); ok {
			ranges := msg.ExtensionRanges()
			for i := 0; i < ranges.Len(); i++ {
				rng := ranges.Get(i)
				for k := range ps.exts {
					if k.extendee == fqn && k.tag >= rng[0] && k.tag < rng[1] {
						delete(ps.exts, k)
						deletedExtensions = append(deletedExtensions, k)
					}
				}
			}
		}
		if sym, ok := ps.symbols[fqn]; ok {
			if sym.isPackage {
				delete(ps.children, fqn)
			}
			delete(ps.symbols, fqn)
		}
		return nil
	})

	delete(ps.files, f.Path())
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
		info := file.NodeInfo(node.FieldTag())
		extendee := fd.ContainingMessage()
		return s.AddExtension(packageFor(extendee), extendee.FullName(), fd.Number(), info, handler)
	})
}

func (s *Symbols) importResult(r *result, handler *reporter.Handler) error {
	pkg, err := s.importPackages(packageNameSpan(r), r.Package(), handler)
	if err != nil || pkg == nil {
		return err
	}
	_, err = pkg.importResult(r, handler)
	return err
}

func (s *Symbols) deleteResultLocked(r *result, handler *reporter.Handler) error {
	pkgName := r.Package()

	pkg := &s.pkgTrie
	pkgParts := []*packageSymbols{}
	var parts []string
	if pkgName != "" {
		pkgParts = append(pkgParts, pkg)
		parts = strings.Split(string(pkgName), ".")
		for i := 1; i < len(parts); i++ {
			parts[i] = parts[i-1] + "." + parts[i]
		}

		// walk the package tree to find the package
		for i := 0; i < len(parts); i++ {
			pkg = pkg.children[protoreflect.FullName(parts[i])]
			pkgParts = append(pkgParts, pkg)
			if pkg == nil {
				return fmt.Errorf("%w: %q", ErrPackageNotFound, pkgName)
			}
		}
	}

	deletedExtensions, err := pkg.deleteResult(r, handler)
	if err != nil {
		if !errors.Is(err, ErrFileNotFound) {
			return err
		}
	}

	// if any extensions were deleted, we need to delete them from the
	// extendee's symbols and from the global extension number map
	for _, ext := range deletedExtensions {
		// NB: only delete if the extension was defined in the file being deleted.
		// Otherwise, we can end up with duplicate extension numbers in files that
		// aren't related by imports, since they won't invalidate each other, but
		// they will invalidate the other's extension numbers in the global map.
		if extVal := s.pkgTrie.exts[ext]; extVal != nil && extVal.Start().Filename == r.Path() {
			delete(s.pkgTrie.exts, ext) // delete from global map
		}

		extendeePkg := s.getPackage(ext.extendee.Parent())
		if extendeePkg == nil {
			continue
		}
		extendeePkg.mu.Lock()
		delete(extendeePkg.exts, ext)
		extendeePkg.mu.Unlock()
	}

	// this might be the last result for this package, so recursively delete
	// empty packages
	for i := len(pkgParts) - 1; i > 0; i-- {
		pkg := pkgParts[i]
		if len(pkg.files) == 0 && len(pkg.children) == 0 && len(pkg.symbols) == 0 && len(pkg.exts) == 0 {
			key := protoreflect.FullName(parts[i-1])
			delete(pkgParts[i-1].children, key)
			delete(pkgParts[i-1].symbols, key)
		} else {
			break
		}
	}

	return nil
}

func (s *Symbols) deleteFileLocked(fd protoreflect.FileDescriptor, handler *reporter.Handler) error {
	pkgName := fd.Package()
	if pkgName == "" {
		return fmt.Errorf("no package name found")
	}

	pkg := &s.pkgTrie
	pkgParts := []*packageSymbols{}
	var parts []string
	if pkgName != "" {
		pkgParts = append(pkgParts, pkg)
		parts = strings.Split(string(pkgName), ".")
		for i := 1; i < len(parts); i++ {
			parts[i] = parts[i-1] + "." + parts[i]
		}

		// walk the package tree to find the package
		for i := 0; i < len(parts); i++ {
			pkg = pkg.children[protoreflect.FullName(parts[i])]
			pkgParts = append(pkgParts, pkg)
			if pkg == nil {
				return fmt.Errorf("%w: %q", ErrPackageNotFound, pkgName)
			}
		}
	}

	deletedExtensions, err := pkg.deleteFile(fd, handler)
	if err != nil {
		if !errors.Is(err, ErrFileNotFound) {
			return err
		}
	}

	// if any extensions were deleted, we need to delete them from the
	// extendee's symbols and from the global extension number map
	for _, ext := range deletedExtensions {
		if extVal := s.pkgTrie.exts[ext]; extVal != nil && extVal.Start().Filename == fd.Path() {
			delete(s.pkgTrie.exts, ext) // delete from global map
		}

		extendeePkg := s.getPackage(ext.extendee.Parent())
		if extendeePkg == nil {
			continue
		}
		extendeePkg.mu.Lock()
		delete(extendeePkg.exts, ext)
		extendeePkg.mu.Unlock()
	}

	// this might be the last result for this package, so recursively delete
	// empty packages
	for i := len(pkgParts) - 1; i > 0; i-- {
		pkg := pkgParts[i]
		if len(pkg.files) == 0 && len(pkg.children) == 0 && len(pkg.symbols) == 0 && len(pkg.exts) == 0 {
			key := protoreflect.FullName(parts[i-1])
			delete(pkgParts[i-1].children, key)
			delete(pkgParts[i-1].symbols, key)
		} else {
			break
		}
	}

	return nil
}

func (ps *packageSymbols) importResult(r *result, handler *reporter.Handler) (bool, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, ok := ps.files[r.Path()]; ok {
		// already imported
		return false, nil
	}

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
	ps.commitResultLocked(r)

	return true, nil
}

func (ps *packageSymbols) deleteResult(r *result, handler *reporter.Handler) ([]extNumber, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, ok := ps.files[r.Path()]; !ok {
		// already deleted
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, r.Path())
	}

	return ps.deleteResultLocked(r), nil
}

func (ps *packageSymbols) checkResultLocked(r *result, handler *reporter.Handler) error {
	resultSyms := map[protoreflect.FullName]symbolEntry{}
	return walk.DescriptorProtos(r.FileDescriptorProto(), func(fqn protoreflect.FullName, d proto.Message) error {
		_, isEnumVal := d.(*descriptorpb.EnumValueDescriptorProto)
		file := r.FileNode()
		node := r.Node(d)
		span := nameSpan(file, node)
		// check symbols already in this symbol table
		if existing, ok := ps.symbols[fqn]; ok {
			if err := reportSymbolCollision(span, fqn, isEnumVal, existing, handler); err != nil {
				return err
			}
		}

		// also check symbols from this result (that are not yet in symbol table)
		if existing, ok := resultSyms[fqn]; ok {
			if err := reportSymbolCollision(span, fqn, isEnumVal, existing, handler); err != nil {
				return err
			}
		}
		resultSyms[fqn] = symbolEntry{
			span:        span,
			isEnumValue: isEnumVal,
		}

		return nil
	})
}

func packageNameSpan(r *result) ast.SourceSpan {
	if node, ok := r.FileNode().(*ast.FileNode); ok {
		for _, decl := range node.Decls {
			if pkgNode, ok := decl.(*ast.PackageNode); ok && !pkgNode.IsIncomplete() {
				return r.FileNode().NodeInfo(pkgNode.Name)
			}
		}
	}
	return ast.UnknownSpan(r.Path())
}

func nameSpan(file ast.FileDeclNode, n ast.Node) ast.SourceSpan {
	// TODO: maybe ast package needs a NamedNode interface to simplify this?
	switch n := n.(type) {
	case ast.FieldDeclNode:
		return file.NodeInfo(n.FieldName())
	case ast.MessageDeclNode:
		return file.NodeInfo(n.MessageName())
	case ast.OneofDeclNode:
		return file.NodeInfo(n.OneofName())
	case ast.EnumValueDeclNode:
		return file.NodeInfo(n.GetName())
	case *ast.EnumNode:
		return file.NodeInfo(n.Name)
	case *ast.ServiceNode:
		return file.NodeInfo(n.Name)
	case ast.RPCDeclNode:
		return file.NodeInfo(n.GetName())
	default:
		return file.NodeInfo(n)
	}
}

func (ps *packageSymbols) commitResultLocked(r *result) {
	_ = walk.DescriptorProtos(r.FileDescriptorProto(), func(fqn protoreflect.FullName, d proto.Message) error {
		span := nameSpan(r.FileNode(), r.Node(d))
		_, isEnumValue := d.(protoreflect.EnumValueDescriptor)
		ps.symbols[fqn] = symbolEntry{span: span, isEnumValue: isEnumValue}
		return nil
	})
	ps.files[r.Path()] = struct{}{}
}

func (ps *packageSymbols) deleteResultLocked(r *result) (deletedExtensions []extNumber) {
	_ = walk.DescriptorProtos(r.FileDescriptorProto(), func(fqn protoreflect.FullName, d proto.Message) error {
		if ext, ok := d.(*descriptorpb.FieldDescriptorProto); ok && ext.GetExtendee() != "" {
			extendee := protoreflect.FullName(ext.GetExtendee())
			if extendee[0] == '.' {
				extendee = extendee[1:]
			}
			deletedExtensions = append(deletedExtensions, extNumber{extendee: extendee, tag: protowire.Number(ext.GetNumber())})
		} else if msg, ok := d.(protoreflect.MessageDescriptor); ok {
			ranges := msg.ExtensionRanges()
			for i := 0; i < ranges.Len(); i++ {
				rng := ranges.Get(i)
				for k := range ps.exts {
					if k.extendee == fqn && k.tag >= rng[0] && k.tag < rng[1] {
						delete(ps.exts, k)
					}
				}
			}
		}
		if sym, ok := ps.symbols[fqn]; ok {
			if sym.isPackage {
				delete(ps.children, fqn)
			}
			delete(ps.symbols, fqn)
		}
		return nil
	})

	delete(ps.files, r.Path())
	return
}

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

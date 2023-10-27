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

	"github.com/bufbuild/protocompile/walk"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// File is like a super-powered protoreflect.FileDescriptor. It includes helpful
// methods for looking up elements in the descriptor and can be used to create a
// resolver for all the file's transitive closure of dependencies. (See
// ResolverFromFile.)
type File interface {
	protoreflect.FileDescriptor
	Dependencies() Files

	// FindDescriptorByName returns the given named element that is defined in
	// this file. If no such element exists, nil is returned.
	FindDescriptorByName(name protoreflect.FullName) protoreflect.Descriptor
	// FindImportByPath returns the File corresponding to the given import path.
	// If this file does not import the given path, nil is returned.
	FindImportByPath(path string) File
	// FindExtensionByNumber returns the extension descriptor for the given tag
	// that extends the given message name. If no such extension is defined in this
	// file, nil is returned.
	FindExtensionByNumber(message protoreflect.FullName, tag protoreflect.FieldNumber) protoreflect.ExtensionTypeDescriptor
}

// NewFile converts a protoreflect.FileDescriptor to a File. The given deps must
// contain all dependencies/imports of f. Also see NewFileRecursive.
func NewFile(f protoreflect.FileDescriptor, deps Files) (File, error) {
	if asFile, ok := f.(File); ok {
		return asFile, nil
	}
	checkedDeps := make(Files, f.Imports().Len())
	for i := 0; i < f.Imports().Len(); i++ {
		imprt := f.Imports().Get(i)
		dep := deps.FindFileByPath(imprt.Path())
		if dep == nil {
			return nil, fmt.Errorf("cannot create File for %q: missing dependency for %q", f.Path(), imprt.Path())
		}
		checkedDeps[i] = dep
	}
	return newFile(f, checkedDeps)
}

func newFile(f protoreflect.FileDescriptor, deps Files) (*file, error) {
	descs := map[protoreflect.FullName]protoreflect.Descriptor{}
	err := walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		if _, ok := descs[d.FullName()]; ok {
			return fmt.Errorf("file %q contains multiple elements with the name %s", f.Path(), d.FullName())
		}
		descs[d.FullName()] = d
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &file{
		FileDescriptor: f,
		descs:          descs,
		deps:           deps,
	}, nil
}

// NewFileRecursive recursively converts a protoreflect.FileDescriptor to a File.
// If f has any dependencies/imports, they are converted, too, including any and
// all transitive dependencies.
//
// If f is an instance of File, it is returned unchanged.
func NewFileRecursive(f protoreflect.FileDescriptor) (File, error) {
	if fp, ok := f.(*file); ok {
		return fp, nil
	}
	file, err := newFileRecursive(f, map[protoreflect.FileDescriptor]File{})
	if err != nil {
		return nil, err
	}
	return file, nil
}

func newFileRecursive(fd protoreflect.FileDescriptor, seen map[protoreflect.FileDescriptor]File) (File, error) {
	if res, ok := seen[fd]; ok {
		if res == nil {
			return nil, fmt.Errorf("import cycle encountered: file %s transitively imports itself", fd.Path())
		}
		return res, nil
	}

	if f, ok := fd.(File); ok {
		seen[fd] = f
		return f, nil
	}

	seen[fd] = nil
	deps := make(Files, fd.Imports().Len())
	for i := 0; i < fd.Imports().Len(); i++ {
		imprt := fd.Imports().Get(i)
		dep, err := newFileRecursive(imprt, seen)
		if err != nil {
			return nil, err
		}
		deps[i] = dep
	}

	f, err := newFile(fd, deps)
	if err != nil {
		return nil, err
	}
	seen[fd] = f
	return f, nil
}

type file struct {
	protoreflect.FileDescriptor
	descs map[protoreflect.FullName]protoreflect.Descriptor
	deps  Files
}

func (f *file) Dependencies() Files {
	return f.deps
}

func (f *file) FindDescriptorByName(name protoreflect.FullName) protoreflect.Descriptor {
	return f.descs[name]
}

func (f *file) FindImportByPath(path string) File {
	return f.deps.FindFileByPath(path)
}

func (f *file) FindExtensionByNumber(msg protoreflect.FullName, tag protoreflect.FieldNumber) protoreflect.ExtensionTypeDescriptor {
	return findExtension(f, msg, tag)
}

var _ File = (*file)(nil)

// Files represents a set of protobuf files. It is a slice of File values, but
// also provides a method for easily looking up files by path and name.
type Files []File

// type SortedFiles []File

// func (f Files) Sort() SortedFiles {
// 	if len(f) < 2 {
// 		return (SortedFiles)(f)
// 	}
// 	slices.SortFunc(f, compareFiles)
// 	return (SortedFiles)(f)
// }

// // Efficiently merges two sorted Files lists. If 'a' has enough capacity to hold
// // the merged result, the merge is done in-place. Otherwise, a new slice is
// // allocated. The new slice is returned.
// func MergeFiles(a, b SortedFiles) SortedFiles {
// 	if cap(a) >= len(a)+len(b) {
// 		oldLen := len(a)
// 		a = append(a, b...)

// 		i, j, k := oldLen-1, len(b)-1, len(a)-1
// 		for i >= 0 && j >= 0 {
// 			switch compareFiles(a[i], b[j]) {
// 			case -1: // a[i] < b[j]
// 				a[k] = a[i]
// 				i--
// 			case 1: // a[i] > b[j]
// 				a[k] = b[j]
// 				j--
// 			case 0: // a[i] == b[j]
// 				// duplicate, overwrite the value in a with the value in b
// 				a[k] = b[j]
// 				i--
// 				j--
// 			}
// 			k--
// 		}
// 		for j >= 0 {
// 			a[k] = b[j]
// 			j--
// 			k--
// 		}
// 		return a
// 	}

// 	out := make(SortedFiles, len(a)+len(b))
// 	i, j, k := 0, 0, 0
// 	for i < len(a) && j < len(b) {
// 		switch compareFiles(a[i], b[j]) {
// 		case -1: // a[i] < b[j]
// 			out[k] = a[i]
// 			i++
// 		case 1: // a[i] > b[j]
// 			out[k] = b[j]
// 			j++
// 		case 0: // a[i] == b[j]
// 			// duplicate, overwrite the value in a with the value in b
// 			out[k] = b[j]
// 			i++
// 			j++
// 		}
// 		k++
// 	}
// 	for i < len(a) {
// 		out[k] = a[i]
// 		i++
// 		k++
// 	}
// 	for j < len(b) {
// 		out[k] = b[j]
// 		j++
// 		k++
// 	}
// 	return out[:k]
// }

// func compareFiles(a, b File) int {
// 	return strings.Compare(a.Path(), b.Path())
// }

// func (f *SortedFiles) Put(newFile File) bool {
// 	i, exists := slices.BinarySearchFunc(*f, newFile, compareFiles)
// 	if exists {
// 		(*f)[i] = newFile
// 	} else {
// 		*f = slices.Insert(*f, i, newFile)
// 	}
// 	return !exists
// }

// func (f *SortedFiles) Delete(file File) {
// 	i, exists := slices.BinarySearchFunc(*f, file, compareFiles)
// 	if exists {
// 		*f = slices.Delete(*f, i, i+1)
// 	}
// }

// // FindFileByPath finds a file in f that has the given path and name. If f
// // contains no such file, nil is returned.
// func (f SortedFiles) FindFileByPath(path string) File {
// 	idx, ok := slices.BinarySearchFunc(f, path, func(file File, path string) int {
// 		return strings.Compare(file.Path(), path)
// 	})
// 	if ok {
// 		return f[idx]
// 	}
// 	return nil
// }

// FindFileByPath finds a file in f that has the given path and name. If f
// contains no such file, nil is returned.
func (f Files) FindFileByPath(path string) File {
	for _, file := range f {
		if file == nil {
			continue
		}
		if file.Path() == path {
			return file
		}
	}
	return nil
}

// AsResolver returns a Resolver that uses f as the source of descriptors. If
// a given query cannot be answered with the files in f, the query will fail
// with a protoregistry.NotFound error. The implementation just delegates calls
// to each file until a result is found.
//
// Also see ResolverFromFile.
func (f Files) AsResolver() Resolver {
	return newFilesResolver(f)
}

// func (f SortedFiles) AsResolver() Resolver {
// 	return newFilesResolver(f)
// }

// Resolver is an interface that can resolve various kinds of queries about
// descriptors. It satisfies the resolver interfaces defined in protodesc
// and protoregistry packages.
type Resolver interface {
	protodesc.Resolver
	protoregistry.MessageTypeResolver
	protoregistry.ExtensionTypeResolver
}

// ResolverFromFile returns a Resolver that can resolve any element that is
// visible to the given file. It will search the given file, its imports, and
// any transitive public imports.
//
// Note that this function does not compute any additional indexes for efficient
// search, so queries generally take linear time, O(n) where n is the number of
// files whose elements are visible to the given file. Queries for an extension
// by number are linear with the number of messages and extensions defined across
// those files.
func ResolverFromFile(f File) Resolver {
	return fileResolver{f: f}
}

type fileResolver struct {
	f File
}

func (r fileResolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	return resolveInFile(r.f, false, nil, func(f File) (protoreflect.FileDescriptor, error) {
		if f.Path() == path {
			return f, nil
		}
		return nil, protoregistry.NotFound
	})
}

func (r fileResolver) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	return resolveInFile(r.f, false, nil, func(f File) (protoreflect.Descriptor, error) {
		if d := f.FindDescriptorByName(name); d != nil {
			return d, nil
		}
		return nil, protoregistry.NotFound
	})
}

func (r fileResolver) FindMessageByName(message protoreflect.FullName) (protoreflect.MessageType, error) {
	return resolveInFile(r.f, false, nil, func(f File) (protoreflect.MessageType, error) {
		d := f.FindDescriptorByName(message)
		if d != nil {
			md, ok := d.(protoreflect.MessageDescriptor)
			if !ok {
				return nil, fmt.Errorf("%q is %s, not a message", message, descriptorTypeWithArticle(d))
			}
			return dynamicpb.NewMessageType(md), nil
		}
		return nil, protoregistry.NotFound
	})
}

func (r fileResolver) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	fullName := messageNameFromURL(url)
	return r.FindMessageByName(protoreflect.FullName(fullName))
}

func messageNameFromURL(url string) string {
	lastSlash := strings.LastIndexByte(url, '/')
	return url[lastSlash+1:]
}

func (r fileResolver) FindExtensionByName(field protoreflect.FullName) (protoreflect.ExtensionType, error) {
	return resolveInFile(r.f, false, nil, func(f File) (protoreflect.ExtensionType, error) {
		d := f.FindDescriptorByName(field)
		if d != nil {
			fld, ok := d.(protoreflect.FieldDescriptor)
			if !ok || !fld.IsExtension() {
				return nil, fmt.Errorf("%q is %s, not an extension", field, descriptorTypeWithArticle(d))
			}
			if extd, ok := fld.(protoreflect.ExtensionTypeDescriptor); ok {
				return extd.Type(), nil
			}
			return dynamicpb.NewExtensionType(fld), nil
		}
		return nil, protoregistry.NotFound
	})
}

func (r fileResolver) FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	return resolveInFile(r.f, false, nil, func(f File) (protoreflect.ExtensionType, error) {
		ext := findExtension(f, message, field)
		if ext != nil {
			return ext.Type(), nil
		}
		return nil, protoregistry.NotFound
	})
}

type filesSliceType[T File] interface {
	~[]T
	FindFileByPath(string) T
}
type filesResolver[S filesSliceType[T], T File] []T

func newFilesResolver[S filesSliceType[T], T File](files S) filesResolver[S, T] {
	return filesResolver[S, T](files)
}

func (r filesResolver[S, T]) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	var f File = S(r).FindFileByPath(path)
	if f != nil {
		return f, nil
	}
	return nil, protoregistry.NotFound
}

func (r filesResolver[S, T]) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	for _, f := range r {
		result := f.FindDescriptorByName(name)
		if result != nil {
			return result, nil
		}
	}
	return nil, protoregistry.NotFound
}

func (r filesResolver[S, T]) FindMessageByName(message protoreflect.FullName) (protoreflect.MessageType, error) {
	for _, f := range r {
		d := f.FindDescriptorByName(message)
		if d != nil {
			if md, ok := d.(protoreflect.MessageDescriptor); ok {
				return dynamicpb.NewMessageType(md), nil
			}
			return nil, protoregistry.NotFound
		}
	}
	return nil, protoregistry.NotFound
}

func (r filesResolver[S, T]) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	name := messageNameFromURL(url)
	return r.FindMessageByName(protoreflect.FullName(name))
}

func (r filesResolver[S, T]) FindExtensionByName(field protoreflect.FullName) (protoreflect.ExtensionType, error) {
	for _, f := range r {
		d := f.FindDescriptorByName(field)
		if d != nil {
			if extd, ok := d.(protoreflect.ExtensionTypeDescriptor); ok {
				return extd.Type(), nil
			}
			if fld, ok := d.(protoreflect.FieldDescriptor); ok && fld.IsExtension() {
				return dynamicpb.NewExtensionType(fld), nil
			}
			return nil, protoregistry.NotFound
		}
	}
	return nil, protoregistry.NotFound
}

func (r filesResolver[S, T]) FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	for _, f := range r {
		ext := findExtension(f, message, field)
		if ext != nil {
			return ext.Type(), nil
		}
	}
	return nil, protoregistry.NotFound
}

type hasExtensionsAndMessages interface {
	Messages() protoreflect.MessageDescriptors
	Extensions() protoreflect.ExtensionDescriptors
}

func findExtension(d hasExtensionsAndMessages, message protoreflect.FullName, field protoreflect.FieldNumber) protoreflect.ExtensionTypeDescriptor {
	for i := 0; i < d.Extensions().Len(); i++ {
		if extType := isExtensionMatch(d.Extensions().Get(i), message, field); extType != nil {
			return extType
		}
	}

	for i := 0; i < d.Messages().Len(); i++ {
		if extType := findExtension(d.Messages().Get(i), message, field); extType != nil {
			return extType
		}
	}

	return nil // could not be found
}

func isExtensionMatch(ext protoreflect.ExtensionDescriptor, message protoreflect.FullName, field protoreflect.FieldNumber) protoreflect.ExtensionTypeDescriptor {
	if ext.Number() != field || ext.ContainingMessage().FullName() != message {
		return nil
	}
	if extType, ok := ext.(protoreflect.ExtensionTypeDescriptor); ok {
		return extType
	}
	return dynamicpb.NewExtensionType(ext).TypeDescriptor()
}

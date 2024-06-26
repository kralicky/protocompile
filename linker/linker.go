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
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	art "github.com/kralicky/go-adaptive-radix-tree"
	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/parser"
	"github.com/kralicky/protocompile/reporter"
	"github.com/kralicky/protocompile/sourceinfo"
)

// Link handles linking a parsed descriptor proto into a fully-linked descriptor.
// If the given parser.Result has imports, they must all be present in the given
// dependencies, in the exact order they are present in the parsed descriptor.
//
// The symbols value is optional and may be nil. If it is not nil, it must be the
// same instance used to create and link all of the given result's dependencies
// (or otherwise already have all dependencies imported). Otherwise, linking may
// fail with spurious errors resolving symbols.
//
// The handler value is used to report any link errors. If any such errors are
// reported, this function returns a non-nil error. The Result value returned
// also implements protoreflect.FileDescriptor.
//
// Note that linking does NOT interpret options. So options messages in the
// returned value have all values stored in UninterpretedOptions fields.
func Link(parsed parser.Result, dependencies Files, symbols *Symbols, handler *reporter.Handler) (Result, error) {
	if symbols == nil {
		symbols = NewSymbolTable()
	}
	fd := parsed.FileDescriptorProto()
	prefix := fd.GetPackage()
	if prefix != "" {
		prefix += "."
	}

	filteredDependencies := dependencies
	if len(fd.Dependency) != len(filteredDependencies) {
		if len(fd.Dependency) == len(filteredDependencies)-1 {
			// there may be an implicit dependency on google/protobuf/descriptor.proto
			for i, dep := range filteredDependencies {
				if dep.IsPlaceholder() {
					// the dependency didn't link
					continue
				}
				if dep.Path() == "google/protobuf/descriptor.proto" {
					filteredDependencies = append(filteredDependencies[:i], filteredDependencies[i+1:]...)
					goto dependencies_ok
				}
			}
		}
		panic(fmt.Sprintf("bug: dependencies length mismatch: descriptor has %v, want %v", fd.Dependency, filteredDependencies))
	}
dependencies_ok:

	for i, imp := range fd.Dependency {
		dep := filteredDependencies[i]
		fd.Dependency[i] = dep.Path()

		if dep.IsPlaceholder() {
			// handle unresolvable import paths
			// first, find the import node for this path
			var importNode *ast.ImportNode
			if parsedAst := parsed.AST(); parsedAst != nil {
				for _, node := range parsedAst.Decls {
					importNode = node.GetImport()
					if importNode == nil || importNode.IsIncomplete() {
						continue
					}
					if importNode.Name.AsString() == imp {
						break
					}
				}
				if importNode == nil {
					panic("bug: could not find import node for path: " + imp)
				}
				nodeInfo := parsed.AST().NodeInfo(importNode)
				if err := handler.HandleErrorf(nodeInfo, "could not resolve import %q", imp); err != nil {
					return nil, err
				}
			} else {
				// no ast, log an error with no source position
				if err := handler.HandleErrorf(ast.UnknownSpan(parsed.FileDescriptorProto().GetName()), "could not resolve import %q", imp); err != nil {
					return nil, err
				}
			}
		} else if err := symbols.Import(dep, handler); err != nil {
			return nil, err
		}
	}

	r := &result{
		FileDescriptor:       noOpFile,
		Result:               parsed,
		deps:                 dependencies, // the unfiltered dependencies
		descriptors:          art.New[protoreflect.Descriptor](),
		usedImports:          map[string]struct{}{},
		prefix:               prefix,
		optionQualifiedNames: map[*ast.IdentValueNode]string{},
		resolvedReferences:   map[protoreflect.Descriptor][]ast.NodeReference{},
		extensionsByMessage:  map[protoreflect.FullName][]protoreflect.ExtensionDescriptor{},
	}
	// First, we create the hierarchy of descendant descriptors.
	r.createDescendants()

	// Then we can put all symbols into a single pool, which lets us ensure there
	// are no duplicate symbols and will also let us resolve and revise all type
	// references in next step.
	var err error
	if err = symbols.importResult(r, handler); err != nil {
		if !IsRecoverable(err) {
			return nil, err
		}
		// let reference resolution continue so we can report more errors
	}

	// After we've populated the pool, we can now try to resolve all type
	// references. All references must be checked for correct type, any fields
	// with enum types must be corrected (since we parse them as if they are
	// message references since we don't actually know message or enum until
	// link time), and references will be re-written to be fully-qualified
	// references (e.g. start with a dot ".").
	if err := r.resolveReferences(handler, symbols); err != nil {
		return nil, err
	}

	if err == nil {
		err = handler.Error()
	}
	return r, err
}

func IsRecoverable(err error) bool {
	if err == nil {
		return true
	}
	var sce *SymbolCollisionError
	if errors.As(err, &sce) {
		return sce.recoverable
	}
	return errors.Is(err, reporter.ErrInvalidSource)
}

// Result is the result of linking. This is a protoreflect.FileDescriptor, but
// with some additional methods for exposing additional information, such as the
// for accessing the input AST or file descriptor.
//
// It also provides Resolve* methods, for looking up enums, messages, and
// extensions that are available to the protobuf source file this result
// represents. An element is "available" if it meets any of the following
// criteria:
//  1. The element is defined in this file itself.
//  2. The element is defined in a file that is directly imported by this file.
//  3. The element is "available" to a file that is directly imported by this
//     file as a public import.
//
// Other elements, even if in the transitive closure of this file, are not
// available and thus won't be returned by these methods.
type Result interface {
	File
	parser.Result

	// ResolveMessageLiteralExtensionName returns the fully qualified name for
	// an identifier for extension field names in message literals.
	ResolveMessageLiteralExtensionName(*ast.IdentValueNode) string
	// ValidateOptions runs some validation checks on the descriptor that can only
	// be done after options are interpreted. Any errors or warnings encountered
	// will be reported via the given handler. If any error is reported, this
	// function returns a non-nil error.
	ValidateOptions(handler *reporter.Handler, lenient bool) error
	// CheckForUnusedImports is used to report warnings for unused imports. This
	// should be called after options have been interpreted. Otherwise, the logic
	// could incorrectly report imports as unused if the only symbol used were a
	// custom option.
	CheckForUnusedImports(handler *reporter.Handler)
	// PopulateSourceCodeInfo is used to populate source code info for the file
	// descriptor. This step requires that the underlying descriptor proto have
	// its `source_code_info` field populated. This is typically a post-process
	// step separate from linking, because computing source code info requires
	// interpreting options (which is done after linking).
	PopulateSourceCodeInfo(sourceinfo.OptionIndex, sourceinfo.OptionDescriptorIndex)

	FindDescriptorsByPrefix(ctx context.Context, prefix string, filter ...func(protoreflect.Descriptor) bool) ([]protoreflect.Descriptor, error)
	RangeDescriptors(ctx context.Context, fn func(protoreflect.Descriptor) bool) error

	FindReferences(to protoreflect.Descriptor) []ast.NodeReference

	FindOptionSourceInfo(*ast.OptionNode) *sourceinfo.OptionSourceInfo
	FindOptionNameFieldDescriptor(name *descriptorpb.UninterpretedOption_NamePart) protoreflect.FieldDescriptor
	FindOptionFieldDescriptor(option *descriptorpb.UninterpretedOption) protoreflect.FieldDescriptor
	FindFieldDescriptorByFieldReferenceNode(node *ast.FieldReferenceNode) protoreflect.FieldDescriptor
	FindFieldDescriptorByMessageFieldNode(node *ast.MessageFieldNode) protoreflect.FieldDescriptor
	RangeFieldReferenceNodesWithDescriptors(func(node ast.Node, desc protoreflect.FieldDescriptor) bool)
	FindMessageDescriptorByTypeReferenceURLNode(node *ast.FieldReferenceNode) protoreflect.MessageDescriptor
	FindExtendeeDescriptorByName(fqn protoreflect.FullName) protoreflect.MessageDescriptor
	FindExtensionsByMessage(fqn protoreflect.FullName) []protoreflect.ExtensionDescriptor

	// RemoveAST drops the AST information from this result.
	RemoveAST()
}

// ErrorUnusedImport may be passed to a warning reporter when an unused
// import is detected. The error the reporter receives will be wrapped
// with source position that indicates the file and line where the import
// statement appeared.
type ErrorUnusedImport interface {
	error
	UnusedImport() string
}

type errUnusedImport string

func (e errUnusedImport) Error() string {
	return fmt.Sprintf("import %q not used", string(e))
}

func (e errUnusedImport) UnusedImport() string {
	return string(e)
}

type ErrorUndeclaredName interface {
	error
	UndeclaredName() string
	ParentFile() *ast.FileNode
	Hint() string
}

type errUndeclaredName struct {
	scope      string
	what       string
	name       string
	hint       string
	parentFile *ast.FileNode
}

func (e *errUndeclaredName) Error() string {
	hint := e.hint
	if e.hint != "" {
		hint = " (" + hint + ")"
	}
	return fmt.Sprintf("%s: unknown %s %s%s", e.scope, e.what, e.name, hint)
}

func (e *errUndeclaredName) UndeclaredName() string {
	return e.name
}

func (e *errUndeclaredName) ParentFile() *ast.FileNode {
	return e.parentFile
}

func (e *errUndeclaredName) Hint() string {
	return e.hint
}

func ComputeReflexiveTransitiveClosure(roots Files) Files {
	seen := map[File]struct{}{}
	var results Files
	for _, root := range roots {
		if root.IsPlaceholder() {
			continue
		}
		results = append(results, computeReflexiveTransitiveClosure(root, seen)...)
	}
	return results
}

func computeReflexiveTransitiveClosure(root File, seen map[File]struct{}) Files {
	if _, ok := seen[root]; ok {
		return nil
	}
	seen[root] = struct{}{}
	results := Files{}
	for _, dep := range root.Dependencies() {
		if dep.IsPlaceholder() {
			continue
		}
		results = append(results, computeReflexiveTransitiveClosure(dep, seen)...)
	}
	return append(results, root)
}

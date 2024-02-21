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

package parser

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/internal"
	"github.com/kralicky/protocompile/reporter"
)

var supportedEditions = map[string]descriptorpb.Edition{
	"2023": descriptorpb.Edition_EDITION_2023,
}

// NB: protoreflect.Syntax doesn't yet know about editions, so we have to use our own type.
type syntaxType int

const (
	syntaxProto2 = syntaxType(iota)
	syntaxProto3
	syntaxEditions
)

func (s syntaxType) String() string {
	switch s {
	case syntaxProto2:
		return "proto2"
	case syntaxProto3:
		return "proto3"
	case syntaxEditions:
		return "editions"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

type result struct {
	file  *ast.FileNode
	proto *descriptorpb.FileDescriptorProto

	nodes              map[proto.Message]ast.Node
	nodesInverse       map[ast.Node]proto.Message
	fieldExtendeeNodes map[ast.Node]*ast.ExtendNode

	// A position in the source file corresponding to the end of the last import
	// statement (the point just after the semicolon). This can be used as an
	// insertion point for new import statements.
	importInsertionPoint ast.SourcePos
}

// ResultWithoutAST returns a parse result that has no AST. All methods for
// looking up AST nodes return a placeholder node that contains only the filename
// in position information.
func ResultWithoutAST(proto *descriptorpb.FileDescriptorProto) Result {
	return &result{proto: proto}
}

// ResultFromAST constructs a descriptor proto from the given AST. The returned
// result includes the descriptor proto and also contains an index that can be
// used to lookup AST node information for elements in the descriptor proto
// hierarchy.
//
// If validate is true, some basic validation is performed, to make sure the
// resulting descriptor proto is valid per protobuf rules and semantics. Only
// some language elements can be validated since some rules and semantics can
// only be checked after all symbols are all resolved, which happens in the
// linking step.
//
// The given handler is used to report any errors or warnings encountered. If any
// errors are reported, this function returns a non-nil error.
func ResultFromAST(file *ast.FileNode, validate bool, handler *reporter.Handler) (Result, error) {
	filename := file.Name()
	r := &result{
		file:               file,
		nodes:              map[proto.Message]ast.Node{},
		nodesInverse:       map[ast.Node]proto.Message{},
		fieldExtendeeNodes: map[ast.Node]*ast.ExtendNode{},
	}
	r.createFileDescriptor(filename, file, handler)
	if validate {
		validateBasic(r, handler)
	}
	// Now that we're done validating, we can set any missing labels to optional
	// (we leave them absent in first pass if label was missing in source, so we
	// can do validation on presence of label, but final descriptors are expected
	// to always have them present).
	fillInMissingLabels(r.proto)
	var lastSeenImport *ast.ImportNode
DECLS:
	for _, decl := range file.Decls {
		switch decl := decl.Unwrap().(type) {
		case *ast.PackageNode:
			if lastSeenImport == nil {
				// as a backup in case there are no imports
				r.importInsertionPoint = file.NodeInfo(decl).End()
			}
		case *ast.ImportNode:
			if decl.IsIncomplete() {
				continue
			}
			lastSeenImport = decl
		default:
			if lastSeenImport != nil {
				r.importInsertionPoint = file.NodeInfo(lastSeenImport).End()
				break DECLS
			}
		}
	}

	return r, handler.Error()
}

func (r *result) ImportInsertionPoint() ast.SourcePos {
	if r.file == nil {
		return ast.SourcePos{}
	}

	if r.importInsertionPoint == (ast.SourcePos{}) {
		return r.file.NodeInfo(r.file.Syntax).End()
	}
	return r.importInsertionPoint
}

func (r *result) AST() *ast.FileNode {
	return r.file
}

func (r *result) FileDescriptorProto() *descriptorpb.FileDescriptorProto {
	return r.proto
}

func (r *result) createFileDescriptor(filename string, file *ast.FileNode, handler *reporter.Handler) {
	fd := &descriptorpb.FileDescriptorProto{Name: proto.String(filename)}
	r.proto = fd

	r.putFileNode(fd, file)

	var syntax syntaxType
	switch {
	case file.Syntax != nil:
		switch file.Syntax.Syntax.AsString() {
		case "proto3":
			syntax = syntaxProto3
		case "proto2":
			syntax = syntaxProto2
		default:
			nodeInfo := file.NodeInfo(file.Syntax.Syntax)
			if handler.HandleErrorf(nodeInfo, `syntax value must be "proto2" or "proto3"`) != nil {
				return
			}
		}

		// proto2 is the default, so no need to set for that value
		if syntax != syntaxProto2 {
			fd.Syntax = proto.String(file.Syntax.Syntax.AsString())
		}
	case file.Edition != nil:
		if !internal.AllowEditions {
			nodeInfo := file.NodeInfo(file.Edition.Edition)
			if handler.HandleErrorf(nodeInfo, `editions are not yet supported; use syntax proto2 or proto3 instead`) != nil {
				return
			}
		}
		edition := file.Edition.Edition.AsString()
		syntax = syntaxEditions

		fd.Syntax = proto.String("editions")
		editionEnum, ok := supportedEditions[edition]
		if !ok {
			nodeInfo := file.NodeInfo(file.Edition.Edition)
			editionStrs := make([]string, 0, len(supportedEditions))
			for supportedEdition := range supportedEditions {
				editionStrs = append(editionStrs, fmt.Sprintf("%q", supportedEdition))
			}
			sort.Strings(editionStrs)
			if handler.HandleErrorf(nodeInfo, `edition value %q not recognized; should be one of [%s]`, edition, strings.Join(editionStrs, ",")) != nil {
				return
			}
		}
		fd.Edition = editionEnum.Enum()
	default:
		nodeInfo := file.NodeInfo(file)
		handler.HandleWarningWithPos(nodeInfo, ErrNoSyntax)
	}

	for _, decl := range file.Decls {
		if handler.ReporterError() != nil {
			return
		}
		switch decl := decl.Unwrap().(type) {
		case *ast.EnumNode:
			fd.EnumType = append(fd.EnumType, r.asEnumDescriptor(decl, syntax, handler))
		case *ast.ExtendNode:
			if decl.IsIncomplete() {
				continue
			}
			r.addExtensions(decl, &fd.Extension, &fd.MessageType, syntax, handler, 0)
		case *ast.ImportNode:
			if decl.IsIncomplete() {
				continue
			}
			index := len(fd.Dependency)
			fd.Dependency = append(fd.Dependency, decl.Name.AsString())
			if decl.Public != nil {
				fd.PublicDependency = append(fd.PublicDependency, int32(index))
			} else if decl.Weak != nil {
				fd.WeakDependency = append(fd.WeakDependency, int32(index))
			}
		case *ast.MessageNode:
			fd.MessageType = append(fd.MessageType, r.asMessageDescriptor(decl, syntax, handler, 1))
		case *ast.OptionNode:
			if decl.IsIncomplete() {
				if decl.Name == nil || !ast.ExtendedSyntaxEnabled {
					continue
				}
			}
			if fd.Options == nil {
				fd.Options = &descriptorpb.FileOptions{}
			}
			fd.Options.UninterpretedOption = append(fd.Options.UninterpretedOption, r.asUninterpretedOption(decl))
		case *ast.ServiceNode:
			fd.Service = append(fd.Service, r.asServiceDescriptor(decl))
		case *ast.PackageNode:
			if decl.IsIncomplete() {
				continue
			}
			if fd.Package != nil {
				nodeInfo := file.NodeInfo(decl)
				if handler.HandleErrorf(nodeInfo, "files should have only one package declaration") != nil {
					return
				}
			}
			pkgName := string(decl.Name.AsIdentifier())
			if len(pkgName) >= 512 {
				nodeInfo := file.NodeInfo(decl.Name)
				if handler.HandleErrorf(nodeInfo, "package name (with whitespace removed) must be less than 512 characters long") != nil {
					return
				}
			}
			if strings.Count(pkgName, ".") > 100 {
				nodeInfo := file.NodeInfo(decl.Name)
				if handler.HandleErrorf(nodeInfo, "package name may not contain more than 100 periods") != nil {
					return
				}
			}
			fd.Package = proto.String(string(decl.Name.AsIdentifier()))
		}
	}
}

func (r *result) asUninterpretedOptions(nodes []*ast.OptionNode) []*descriptorpb.UninterpretedOption {
	if len(nodes) == 0 {
		return nil
	}
	opts := make([]*descriptorpb.UninterpretedOption, 0, len(nodes))
	for _, n := range nodes {
		if n.IsIncomplete() {
			if n.Name == nil || !ast.ExtendedSyntaxEnabled {
				continue
			}
		}
		opts = append(opts, r.asUninterpretedOption(n))
	}
	return opts
}

func (r *result) asUninterpretedOption(node *ast.OptionNode) *descriptorpb.UninterpretedOption {
	opt := &descriptorpb.UninterpretedOption{Name: r.asUninterpretedOptionName(node.Name.Parts)}
	r.putOptionNode(opt, node)

	if node.Val == nil && ast.ExtendedSyntaxEnabled {
		return opt
	}

	switch val := node.Val.Value().(type) {
	case bool:
		if val {
			opt.IdentifierValue = proto.String("true")
		} else {
			opt.IdentifierValue = proto.String("false")
		}
	case int64:
		opt.NegativeIntValue = proto.Int64(val)
	case uint64:
		opt.PositiveIntValue = proto.Uint64(val)
	case float64:
		opt.DoubleValue = proto.Float64(val)
	case string:
		opt.StringValue = []byte(val)
	case ast.Identifier:
		opt.IdentifierValue = proto.String(string(val))
	default:
		// the grammar does not allow arrays here, so the only possible case
		// left should be []*ast.MessageFieldNode, which corresponds to an
		// *ast.MessageLiteralNode
		if n := node.Val.GetMessageLiteral(); n != nil {
			var buf bytes.Buffer
			for i, el := range n.Elements {
				flattenNode(r.file, el, &buf)
				if len(n.Seps) > i && n.Seps[i] != nil && !n.Seps[i].Virtual {
					buf.WriteRune(' ')
					buf.WriteRune(n.Seps[i].Rune)
				}
			}
			aggStr := buf.String()
			opt.AggregateValue = proto.String(aggStr)
		}
		// TODO: else that reports an error or panics??
	}
	return opt
}

func flattenNode(f *ast.FileNode, n ast.Node, buf *bytes.Buffer) {
	ast.Inspect(n, func(node ast.Node) bool {
		if ast.IsTerminalNode(node) {
			if ast.IsVirtualNode(node) {
				return true
			}
			if buf.Len() > 0 {
				buf.WriteRune(' ')
			}
			str := f.NodeInfo(node).RawText()
			buf.WriteString(str)
		}
		return true
	})
}

func (r *result) asUninterpretedOptionName(parts []*ast.FieldReferenceNode) []*descriptorpb.UninterpretedOption_NamePart {
	ret := make([]*descriptorpb.UninterpretedOption_NamePart, 0, len(parts))
	for _, part := range parts {
		if part.IsIncomplete() {
			continue
		}
		np := &descriptorpb.UninterpretedOption_NamePart{
			NamePart:    proto.String(string(part.Name.AsIdentifier())),
			IsExtension: proto.Bool(part.IsExtension()),
		}
		r.putOptionNamePartNode(np, part)
		ret = append(ret, np)
	}
	return ret
}

func (r *result) addExtensions(ext *ast.ExtendNode, flds *[]*descriptorpb.FieldDescriptorProto, msgs *[]*descriptorpb.DescriptorProto, syntax syntaxType, handler *reporter.Handler, depth int) {
	extendee := string(ext.Extendee.AsIdentifier())
	count := 0
	for _, decl := range ext.Decls {
		switch decl := decl.Unwrap().(type) {
		case *ast.FieldNode:
			if decl.IsIncomplete() {
				continue
			}
			count++
			// use higher limit since we don't know yet whether extendee is messageset wire format
			fd := r.asFieldDescriptor(decl, internal.MaxTag, syntax, handler)
			fd.Extendee = proto.String(extendee)
			*flds = append(*flds, fd)
			r.putFieldNode(fd, decl)
			r.fieldExtendeeNodes[decl] = ext
		case *ast.GroupNode:
			count++
			// ditto: use higher limit right now
			fd, md := r.asGroupDescriptors(decl, syntax, internal.MaxTag, handler, depth+1)
			fd.Extendee = proto.String(extendee)
			r.fieldExtendeeNodes[decl] = ext
			*flds = append(*flds, fd)
			*msgs = append(*msgs, md)
		}
	}
	if count == 0 {
		nodeInfo := r.file.NodeInfo(ext.CloseBrace)
		if ast.ExtendedSyntaxEnabled {
			handler.HandleWarningWithPos(nodeInfo,
				NewExtendedSyntaxError(errors.New("extend sections must define at least one extension"), CategoryEmptyDecl))
		} else {
			_ = handler.HandleErrorf(nodeInfo, "extend sections must define at least one extension")
		}
	}
}

func asLabel(lbl *ast.IdentNode) *descriptorpb.FieldDescriptorProto_Label {
	if lbl == nil {
		return nil
	}
	switch lbl.Val {
	case "repeated":
		return descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	case "required":
		return descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()
	case "optional":
		return descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	default:
		return nil
	}
}

func (r *result) asFieldDescriptor(node *ast.FieldNode, maxTag int32, syntax syntaxType, handler *reporter.Handler) *descriptorpb.FieldDescriptorProto {
	tag := node.Tag.Val
	if err := r.checkTag(node.Tag, tag, maxTag); err != nil {
		_ = handler.HandleError(err)
	}
	fd := newFieldDescriptor(node.Name.Val, string(node.GetFieldType().AsIdentifier()), int32(tag), asLabel(node.Label))
	r.putFieldNode(fd, node)
	if opts := node.Options.GetElements(); len(opts) > 0 {
		fd.Options = &descriptorpb.FieldOptions{UninterpretedOption: r.asUninterpretedOptions(opts)}
	}
	if syntax == syntaxProto3 && fd.Label != nil && fd.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL {
		fd.Proto3Optional = proto.Bool(true)
	}
	return fd
}

var fieldTypes = map[string]descriptorpb.FieldDescriptorProto_Type{
	"double":   descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
	"float":    descriptorpb.FieldDescriptorProto_TYPE_FLOAT,
	"int32":    descriptorpb.FieldDescriptorProto_TYPE_INT32,
	"int64":    descriptorpb.FieldDescriptorProto_TYPE_INT64,
	"uint32":   descriptorpb.FieldDescriptorProto_TYPE_UINT32,
	"uint64":   descriptorpb.FieldDescriptorProto_TYPE_UINT64,
	"sint32":   descriptorpb.FieldDescriptorProto_TYPE_SINT32,
	"sint64":   descriptorpb.FieldDescriptorProto_TYPE_SINT64,
	"fixed32":  descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
	"fixed64":  descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
	"sfixed32": descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
	"sfixed64": descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,
	"bool":     descriptorpb.FieldDescriptorProto_TYPE_BOOL,
	"string":   descriptorpb.FieldDescriptorProto_TYPE_STRING,
	"bytes":    descriptorpb.FieldDescriptorProto_TYPE_BYTES,
}

func newFieldDescriptor(name string, fieldType string, tag int32, lbl *descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto {
	fd := &descriptorpb.FieldDescriptorProto{
		Name:     proto.String(name),
		JsonName: proto.String(internal.JSONName(name)),
		Number:   proto.Int32(tag),
		Label:    lbl,
	}
	t, ok := fieldTypes[fieldType]
	if ok {
		fd.Type = t.Enum()
	} else {
		// NB: we don't have enough info to determine whether this is an enum
		// or a message type, so we'll leave Type nil and set it later
		// (during linking)
		fd.TypeName = proto.String(fieldType)
	}
	return fd
}

func (r *result) asGroupDescriptors(group *ast.GroupNode, syntax syntaxType, maxTag int32, handler *reporter.Handler, depth int) (*descriptorpb.FieldDescriptorProto, *descriptorpb.DescriptorProto) {
	tag := group.Tag.Val
	if err := r.checkTag(group.Tag, tag, maxTag); err != nil {
		_ = handler.HandleError(err)
	}
	if !unicode.IsUpper(rune(group.Name.Val[0])) {
		nameNodeInfo := r.file.NodeInfo(group.Name)
		_ = handler.HandleErrorf(nameNodeInfo, "group %s should have a name that starts with a capital letter", group.Name.Val)
	}
	fieldName := strings.ToLower(group.Name.Val)
	fd := &descriptorpb.FieldDescriptorProto{
		Name:     proto.String(fieldName),
		JsonName: proto.String(internal.JSONName(fieldName)),
		Number:   proto.Int32(int32(tag)),
		Label:    asLabel(group.Label),
		Type:     descriptorpb.FieldDescriptorProto_TYPE_GROUP.Enum(),
		TypeName: proto.String(group.Name.Val),
	}
	if opts := group.Options.GetElements(); len(opts) > 0 {
		fd.Options = &descriptorpb.FieldOptions{UninterpretedOption: r.asUninterpretedOptions(opts)}
	}
	md := &descriptorpb.DescriptorProto{Name: proto.String(group.Name.Val)}
	r.putGroupNode(fd, md, group)
	// don't bother processing body if we've exceeded depth
	if r.checkDepth(depth, group, handler) {
		r.addMessageBody(md, group.Decls, syntax, handler, depth)
	}
	return fd, md
}

func (r *result) asMapDescriptors(mapField *ast.MapFieldNode, syntax syntaxType, maxTag int32, handler *reporter.Handler, depth int) (*descriptorpb.FieldDescriptorProto, *descriptorpb.DescriptorProto) {
	tag := mapField.Tag.Val
	if err := r.checkTag(mapField.Tag, tag, maxTag); err != nil {
		_ = handler.HandleError(err)
	}
	r.checkDepth(depth, mapField, handler)
	var lbl *descriptorpb.FieldDescriptorProto_Label
	if syntax == syntaxProto2 {
		lbl = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	}
	keyFd := newFieldDescriptor("key", mapField.MapType.KeyType.Val, 1, lbl)
	r.putSyntheticFieldNode(keyFd, mapField.KeyField())
	valFd := newFieldDescriptor("value", string(mapField.MapType.ValueType.AsIdentifier()), 2, lbl)
	r.putSyntheticFieldNode(valFd, mapField.ValueField())
	entryName := internal.InitCap(internal.JSONName(mapField.Name.Val)) + "Entry"
	fd := newFieldDescriptor(mapField.Name.Val, entryName, int32(tag), descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum())
	if opts := mapField.Options.GetElements(); len(opts) > 0 {
		fd.Options = &descriptorpb.FieldOptions{UninterpretedOption: r.asUninterpretedOptions(opts)}
	}
	md := &descriptorpb.DescriptorProto{
		Name:    proto.String(entryName),
		Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
		Field:   []*descriptorpb.FieldDescriptorProto{keyFd, valFd},
	}
	r.putMapFieldNode(fd, md, mapField)
	return fd, md
}

func (r *result) asExtensionRanges(node *ast.ExtensionRangeNode, maxTag int32, handler *reporter.Handler) []*descriptorpb.DescriptorProto_ExtensionRange {
	opts := r.asUninterpretedOptions(node.Options.GetElements())
	ers := make([]*descriptorpb.DescriptorProto_ExtensionRange, len(node.Ranges))
	for i, rng := range node.Ranges {
		start, end := r.getRangeBounds(rng, 1, maxTag, handler)
		er := &descriptorpb.DescriptorProto_ExtensionRange{
			Start: proto.Int32(start),
			End:   proto.Int32(end + 1),
		}
		if len(opts) > 0 {
			er.Options = &descriptorpb.ExtensionRangeOptions{UninterpretedOption: opts}
		}
		r.putExtensionRangeNode(er, rng)
		ers[i] = er
	}
	return ers
}

func (r *result) asEnumValue(ev *ast.EnumValueNode, handler *reporter.Handler) *descriptorpb.EnumValueDescriptorProto {
	num, ok := ast.AsInt32(ev.Number, math.MinInt32, math.MaxInt32)
	if !ok {
		numberNodeInfo := r.file.NodeInfo(ev.Number)
		_ = handler.HandleErrorf(numberNodeInfo, "value %d is out of range: should be between %d and %d", ev.Number.Value(), math.MinInt32, math.MaxInt32)
	}
	evd := &descriptorpb.EnumValueDescriptorProto{Name: proto.String(ev.Name.Val), Number: proto.Int32(num)}
	r.putEnumValueNode(evd, ev)
	if opts := ev.Options.GetElements(); len(opts) > 0 {
		evd.Options = &descriptorpb.EnumValueOptions{UninterpretedOption: r.asUninterpretedOptions(opts)}
	}
	return evd
}

func (r *result) asMethodDescriptor(node *ast.RPCNode) *descriptorpb.MethodDescriptorProto {
	var inputType, outputType string
	if !node.Input.IsIncomplete() {
		inputType = string(node.Input.MessageType.AsIdentifier())
	}
	if !node.Output.IsIncomplete() {
		outputType = string(node.Output.MessageType.AsIdentifier())
	}
	md := &descriptorpb.MethodDescriptorProto{
		Name:       proto.String(node.Name.Val),
		InputType:  &inputType,
		OutputType: &outputType,
	}
	r.putMethodNode(md, node)
	if node.Input.Stream != nil {
		md.ClientStreaming = proto.Bool(true)
	}
	if node.Output.Stream != nil {
		md.ServerStreaming = proto.Bool(true)
	}
	// protoc always adds a MethodOptions if there are brackets
	// We do the same to match protoc as closely as possible
	// https://github.com/protocolbuffers/protobuf/blob/0c3f43a6190b77f1f68b7425d1b7e1a8257a8d0c/src/google/protobuf/compiler/parser.cc#L2152
	if node.OpenBrace != nil {
		md.Options = &descriptorpb.MethodOptions{}
		for _, decl := range node.Decls {
			if option := decl.GetOption(); option != nil {
				if option.IsIncomplete() {
					if option.Name == nil || !ast.ExtendedSyntaxEnabled {
						continue
					}
				}
				md.Options.UninterpretedOption = append(md.Options.UninterpretedOption, r.asUninterpretedOption(option))
			}
		}
	}
	return md
}

func (r *result) asEnumDescriptor(en *ast.EnumNode, syntax syntaxType, handler *reporter.Handler) *descriptorpb.EnumDescriptorProto {
	ed := &descriptorpb.EnumDescriptorProto{Name: proto.String(en.Name.Val)}
	r.putEnumNode(ed, en)
	rsvdNames := map[string]ast.SourcePos{}
	for _, decl := range en.Decls {
		switch decl := decl.Unwrap().(type) {
		case *ast.OptionNode:
			if decl.IsIncomplete() {
				if decl.Name == nil || !ast.ExtendedSyntaxEnabled {
					continue
				}
			}
			if ed.Options == nil {
				ed.Options = &descriptorpb.EnumOptions{}
			}
			ed.Options.UninterpretedOption = append(ed.Options.UninterpretedOption, r.asUninterpretedOption(decl))
		case *ast.EnumValueNode:
			ed.Value = append(ed.Value, r.asEnumValue(decl, handler))
		case *ast.ReservedNode:
			r.addReservedNames(&ed.ReservedName, decl, syntax, handler, rsvdNames)
			for _, rng := range decl.Ranges {
				ed.ReservedRange = append(ed.ReservedRange, r.asEnumReservedRange(rng, handler))
			}
		}
	}
	return ed
}

func (r *result) asEnumReservedRange(rng *ast.RangeNode, handler *reporter.Handler) *descriptorpb.EnumDescriptorProto_EnumReservedRange {
	start, end := r.getRangeBounds(rng, math.MinInt32, math.MaxInt32, handler)
	rr := &descriptorpb.EnumDescriptorProto_EnumReservedRange{
		Start: proto.Int32(start),
		End:   proto.Int32(end),
	}
	r.putEnumReservedRangeNode(rr, rng)
	return rr
}

func (r *result) asMessageDescriptor(node *ast.MessageNode, syntax syntaxType, handler *reporter.Handler, depth int) *descriptorpb.DescriptorProto {
	msgd := &descriptorpb.DescriptorProto{Name: proto.String(node.Name.Val)}
	r.putMessageNode(msgd, node)
	// don't bother processing body if we've exceeded depth
	if r.checkDepth(depth, node, handler) {
		r.addMessageBody(msgd, node.Decls, syntax, handler, depth)
	}
	return msgd
}

func (r *result) addReservedNames(names *[]string, node *ast.ReservedNode, syntax syntaxType, handler *reporter.Handler, alreadyReserved map[string]ast.SourcePos) {
	if syntax == syntaxEditions {
		if len(node.Names) > 0 {
			nameNodeInfo := r.file.NodeInfo(node.Names[0])
			_ = handler.HandleErrorf(nameNodeInfo, `must use identifiers, not string literals, to reserved names with editions`)
		}
		for _, n := range node.Identifiers {
			name := string(n.AsIdentifier())
			nameNodeInfo := r.file.NodeInfo(n)
			if existing, ok := alreadyReserved[name]; ok {
				_ = handler.HandleErrorf(nameNodeInfo, "name %q is already reserved at %s", name, existing)
				continue
			}
			alreadyReserved[name] = nameNodeInfo.Start()
			*names = append(*names, name)
		}
		return
	}

	if len(node.Identifiers) > 0 {
		nameNodeInfo := r.file.NodeInfo(node.Identifiers[0])
		_ = handler.HandleErrorf(nameNodeInfo, `must use string literals, not identifiers, to reserved names with proto2 and proto3`)
	}
	for _, n := range node.Names {
		name := n.AsString()
		nameNodeInfo := r.file.NodeInfo(n)
		if existing, ok := alreadyReserved[name]; ok {
			_ = handler.HandleErrorf(nameNodeInfo, "name %q is already reserved at %s", name, existing)
			continue
		}
		alreadyReserved[name] = nameNodeInfo.Start()
		*names = append(*names, name)
	}
}

func (r *result) checkDepth(depth int, node ast.Node, handler *reporter.Handler) bool {
	if depth < 32 {
		return true
	}
	if grp, ok := node.(*ast.GroupNode); ok {
		// pinpoint the group keyword if the source is a group
		node = grp.Keyword
	}
	_ = handler.HandleErrorf(r.file.NodeInfo(node), "message nesting depth must be less than 32")
	return false
}

func (r *result) addMessageBody(msgd *descriptorpb.DescriptorProto, decls []*ast.MessageElement, syntax syntaxType, handler *reporter.Handler, depth int) {
	// first process any options
	for _, decl := range decls {
		if opt := decl.GetOption(); opt != nil {
			if opt.IsIncomplete() {
				if opt.Name == nil || !ast.ExtendedSyntaxEnabled {
					continue
				}
			}
			if msgd.Options == nil {
				msgd.Options = &descriptorpb.MessageOptions{}
			}
			msgd.Options.UninterpretedOption = append(msgd.Options.UninterpretedOption, r.asUninterpretedOption(opt))
		}
	}

	// now that we have options, we can see if this uses messageset wire format, which
	// impacts how we validate tag numbers in any fields in the message
	maxTag := int32(internal.MaxNormalTag)
	messageSetOpt, err := r.isMessageSetWireFormat("message "+msgd.GetName(), msgd, handler)
	if err != nil {
		return
	} else if messageSetOpt != nil {
		if syntax == syntaxProto3 {
			node := r.OptionNode(messageSetOpt)
			nodeInfo := r.file.NodeInfo(node)
			_ = handler.HandleErrorf(nodeInfo, "messages with message-set wire format are not allowed with proto3 syntax")
		}
		maxTag = internal.MaxTag // higher limit for messageset wire format
	}

	rsvdNames := map[string]ast.SourcePos{}

	// now we can process the rest
	for _, decl := range decls {
		switch decl := decl.Unwrap().(type) {
		case *ast.EnumNode:
			msgd.EnumType = append(msgd.EnumType, r.asEnumDescriptor(decl, syntax, handler))
		case *ast.ExtendNode:
			if decl.IsIncomplete() {
				continue
			}
			r.addExtensions(decl, &msgd.Extension, &msgd.NestedType, syntax, handler, depth)
		case *ast.ExtensionRangeNode:
			msgd.ExtensionRange = append(msgd.ExtensionRange, r.asExtensionRanges(decl, maxTag, handler)...)
		case *ast.FieldNode:
			if decl.IsIncomplete() {
				continue
			}
			fd := r.asFieldDescriptor(decl, maxTag, syntax, handler)
			msgd.Field = append(msgd.Field, fd)
		case *ast.MapFieldNode:
			fd, md := r.asMapDescriptors(decl, syntax, maxTag, handler, depth+1)
			msgd.Field = append(msgd.Field, fd)
			msgd.NestedType = append(msgd.NestedType, md)
		case *ast.GroupNode:
			fd, md := r.asGroupDescriptors(decl, syntax, maxTag, handler, depth+1)
			msgd.Field = append(msgd.Field, fd)
			msgd.NestedType = append(msgd.NestedType, md)
		case *ast.OneofNode:
			oodIndex := len(msgd.OneofDecl)
			ood := &descriptorpb.OneofDescriptorProto{Name: proto.String(decl.Name.Val)}
			r.putOneofNode(ood, decl)
			msgd.OneofDecl = append(msgd.OneofDecl, ood)
			ooFields := 0
			for _, oodecl := range decl.Decls {
				switch oodecl := oodecl.Unwrap().(type) {
				case *ast.OptionNode:
					if oodecl.IsIncomplete() {
						if oodecl.Name == nil || !ast.ExtendedSyntaxEnabled {
							continue
						}
					}
					if ood.Options == nil {
						ood.Options = &descriptorpb.OneofOptions{}
					}
					ood.Options.UninterpretedOption = append(ood.Options.UninterpretedOption, r.asUninterpretedOption(oodecl))
				case *ast.FieldNode:
					if oodecl.IsIncomplete() {
						continue
					}
					fd := r.asFieldDescriptor(oodecl, maxTag, syntax, handler)
					fd.OneofIndex = proto.Int32(int32(oodIndex))
					msgd.Field = append(msgd.Field, fd)
					ooFields++
				case *ast.GroupNode:
					fd, md := r.asGroupDescriptors(oodecl, syntax, maxTag, handler, depth+1)
					fd.OneofIndex = proto.Int32(int32(oodIndex))
					msgd.Field = append(msgd.Field, fd)
					msgd.NestedType = append(msgd.NestedType, md)
					ooFields++
				}
			}
			if ooFields == 0 {
				declNodeInfo := r.file.NodeInfo(decl)
				_ = handler.HandleErrorf(declNodeInfo, "oneof must contain at least one field")
			}
		case *ast.MessageNode:
			msgd.NestedType = append(msgd.NestedType, r.asMessageDescriptor(decl, syntax, handler, depth+1))
		case *ast.ReservedNode:
			r.addReservedNames(&msgd.ReservedName, decl, syntax, handler, rsvdNames)
			for _, rng := range decl.Ranges {
				msgd.ReservedRange = append(msgd.ReservedRange, r.asMessageReservedRange(rng, maxTag, handler))
			}
		}
	}

	if messageSetOpt != nil {
		if len(msgd.Field) > 0 {
			node := r.FieldNode(msgd.Field[0])
			nodeInfo := r.file.NodeInfo(node)
			_ = handler.HandleErrorf(nodeInfo, "messages with message-set wire format cannot contain non-extension fields")
		}
		if len(msgd.ExtensionRange) == 0 {
			node := r.OptionNode(messageSetOpt)
			nodeInfo := r.file.NodeInfo(node)
			_ = handler.HandleErrorf(nodeInfo, "messages with message-set wire format must contain at least one extension range")
		}
	}

	// process any proto3_optional fields
	if syntax == syntaxProto3 {
		r.processProto3OptionalFields(msgd)
	}
}

func (r *result) isMessageSetWireFormat(scope string, md *descriptorpb.DescriptorProto, handler *reporter.Handler) (*descriptorpb.UninterpretedOption, error) {
	uo := md.GetOptions().GetUninterpretedOption()
	index, err := internal.FindOption(r, handler, scope, uo, "message_set_wire_format")
	if err != nil {
		return nil, err
	}
	if index == -1 {
		// no such option
		return nil, nil
	}

	opt := uo[index]

	switch opt.GetIdentifierValue() {
	case "true":
		return opt, nil
	case "false":
		return nil, nil
	default:
		optNode := r.OptionNode(opt)
		optNodeInfo := r.file.NodeInfo(optNode.GetVal())
		return nil, handler.HandleErrorf(optNodeInfo, "%s: expecting bool value for message_set_wire_format option", scope)
	}
}

func (r *result) asMessageReservedRange(rng *ast.RangeNode, maxTag int32, handler *reporter.Handler) *descriptorpb.DescriptorProto_ReservedRange {
	start, end := r.getRangeBounds(rng, 1, maxTag, handler)
	rr := &descriptorpb.DescriptorProto_ReservedRange{
		Start: proto.Int32(start),
		End:   proto.Int32(end + 1),
	}
	r.putMessageReservedRangeNode(rr, rng)
	return rr
}

func (r *result) getRangeBounds(rng *ast.RangeNode, minVal, maxVal int32, handler *reporter.Handler) (int32, int32) {
	checkOrder := true
	start, ok := rng.StartValueAsInt32(minVal, maxVal)
	if !ok {
		checkOrder = false
		startValNodeInfo := r.file.NodeInfo(rng.StartVal)
		_ = handler.HandleErrorf(startValNodeInfo, "range start %d is out of range: should be between %d and %d", rng.StartValue(), minVal, maxVal)
	}

	end, ok := rng.EndValueAsInt32(minVal, maxVal)
	if !ok {
		checkOrder = false
		if rng.EndVal != nil {
			endValNodeInfo := r.file.NodeInfo(rng.EndVal)
			_ = handler.HandleErrorf(endValNodeInfo, "range end %d is out of range: should be between %d and %d", rng.EndValue(), minVal, maxVal)
		}
	}

	if checkOrder && start > end {
		rangeStartNodeInfo := r.file.NodeInfo(rng.RangeStart())
		_ = handler.HandleErrorf(rangeStartNodeInfo, "range, %d to %d, is invalid: start must be <= end", start, end)
	}

	return start, end
}

func (r *result) asServiceDescriptor(svc *ast.ServiceNode) *descriptorpb.ServiceDescriptorProto {
	sd := &descriptorpb.ServiceDescriptorProto{Name: proto.String(svc.Name.Val)}
	r.putServiceNode(sd, svc)
	for _, decl := range svc.Decls {
		switch decl := decl.Unwrap().(type) {
		case *ast.OptionNode:
			if decl.IsIncomplete() {
				if decl.Name == nil || !ast.ExtendedSyntaxEnabled {
					continue
				}
			}
			if sd.Options == nil {
				sd.Options = &descriptorpb.ServiceOptions{}
			}
			sd.Options.UninterpretedOption = append(sd.Options.UninterpretedOption, r.asUninterpretedOption(decl))
		case *ast.RPCNode:
			if decl.IsIncomplete() {
				continue
			}
			sd.Method = append(sd.Method, r.asMethodDescriptor(decl))
		}
	}
	return sd
}

func (r *result) checkTag(n ast.Node, v uint64, maxTag int32) error {
	switch {
	case v < 1:
		return reporter.Errorf(r.file.NodeInfo(n), "tag number %d must be greater than zero", v)
	case v > uint64(maxTag):
		return reporter.Errorf(r.file.NodeInfo(n), "tag number %d is higher than max allowed tag number (%d)", v, maxTag)
	case v >= internal.SpecialReservedStart && v <= internal.SpecialReservedEnd:
		return reporter.Errorf(r.file.NodeInfo(n), "tag number %d is in disallowed reserved range %d-%d", v, internal.SpecialReservedStart, internal.SpecialReservedEnd)
	default:
		return nil
	}
}

// processProto3OptionalFields adds synthetic oneofs to the given message descriptor
// for each proto3 optional field. It also updates the fields to have the correct
// oneof index reference.
func (r *result) processProto3OptionalFields(msgd *descriptorpb.DescriptorProto) {
	// add synthetic oneofs to the given message descriptor for each proto3
	// optional field, and update each field to have correct oneof index
	var allNames map[string]struct{}
	for _, fd := range msgd.Field {
		if fd.GetProto3Optional() {
			// lazy init the set of all names
			if allNames == nil {
				allNames = map[string]struct{}{}
				for _, fd := range msgd.Field {
					allNames[fd.GetName()] = struct{}{}
				}
				for _, od := range msgd.OneofDecl {
					allNames[od.GetName()] = struct{}{}
				}
				// NB: protoc only considers names of other fields and oneofs
				// when computing the synthetic oneof name. But that feels like
				// a bug, since it means it could generate a name that conflicts
				// with some other symbol defined in the message. If it's decided
				// that's NOT a bug and is desirable, then we should remove the
				// following four loops to mimic protoc's behavior.
				for _, fd := range msgd.Extension {
					allNames[fd.GetName()] = struct{}{}
				}
				for _, ed := range msgd.EnumType {
					allNames[ed.GetName()] = struct{}{}
					for _, evd := range ed.Value {
						allNames[evd.GetName()] = struct{}{}
					}
				}
				for _, fd := range msgd.NestedType {
					allNames[fd.GetName()] = struct{}{}
				}
			}

			// Compute a name for the synthetic oneof. This uses the same
			// algorithm as used in protoc:
			//  https://github.com/protocolbuffers/protobuf/blob/74ad62759e0a9b5a21094f3fb9bb4ebfaa0d1ab8/src/google/protobuf/compiler/parser.cc#L785-L803
			ooName := fd.GetName()
			if !strings.HasPrefix(ooName, "_") {
				ooName = "_" + ooName
			}
			for {
				_, ok := allNames[ooName]
				if !ok {
					// found a unique name
					allNames[ooName] = struct{}{}
					break
				}
				ooName = "X" + ooName
			}

			fd.OneofIndex = proto.Int32(int32(len(msgd.OneofDecl)))
			ood := &descriptorpb.OneofDescriptorProto{Name: proto.String(ooName)}
			msgd.OneofDecl = append(msgd.OneofDecl, ood)
			r.putOneofNode(ood, r.FieldNode(fd))
		}
	}
}

func (r *result) Node(m proto.Message) ast.Node {
	return r.nodes[m]
}

func (r *result) FileNode() *ast.FileNode {
	node, ok := r.nodes[r.proto].(*ast.FileNode)
	if !ok {
		return ast.NewEmptyFileNode(r.proto.GetName(), 0)
	}
	return node
}

func (r *result) OptionNode(o *descriptorpb.UninterpretedOption) *ast.OptionNode {
	node, _ := r.nodes[o].(*ast.OptionNode)
	return node
}

func (r *result) OptionNamePartNode(o *descriptorpb.UninterpretedOption_NamePart) ast.Node {
	return r.nodes[o]
}

func (r *result) MessageNode(m *descriptorpb.DescriptorProto) *ast.MessageDeclNode {
	switch n := r.nodes[m].(type) {
	case *ast.MessageDeclNode:
		return n
	case interface{ AsMessageDeclNode() *ast.MessageDeclNode }:
		return n.AsMessageDeclNode()
	}
	return nil
}

func (r *result) FieldNode(f *descriptorpb.FieldDescriptorProto) *ast.FieldDeclNode {
	switch n := r.nodes[f].(type) {
	case *ast.FieldDeclNode:
		return n
	case interface{ AsFieldDeclNode() *ast.FieldDeclNode }:
		return n.AsFieldDeclNode()
	}
	return nil
}

func (r *result) FieldExtendeeNode(f *descriptorpb.FieldDescriptorProto) *ast.ExtendNode {
	return r.fieldExtendeeNodes[r.FieldNode(f).Unwrap()]
}

func (r *result) OneofNode(o *descriptorpb.OneofDescriptorProto) ast.Node {
	node, _ := r.nodes[o].(*ast.OneofNode)
	return node
}

func (r *result) ExtensionRangeNode(e *descriptorpb.DescriptorProto_ExtensionRange) *ast.RangeNode {
	node, _ := r.nodes[e].(*ast.RangeNode)
	return node
}

func (r *result) MessageReservedRangeNode(rr *descriptorpb.DescriptorProto_ReservedRange) *ast.RangeNode {
	node, _ := r.nodes[rr].(*ast.RangeNode)
	return node
}

func (r *result) EnumNode(e *descriptorpb.EnumDescriptorProto) *ast.EnumNode {
	node, _ := r.nodes[e].(*ast.EnumNode)
	return node
}

func (r *result) EnumValueNode(e *descriptorpb.EnumValueDescriptorProto) *ast.EnumValueNode {
	node, _ := r.nodes[e].(*ast.EnumValueNode)
	return node
}

func (r *result) EnumReservedRangeNode(rr *descriptorpb.EnumDescriptorProto_EnumReservedRange) *ast.RangeNode {
	node, _ := r.nodes[rr].(*ast.RangeNode)
	return node
}

func (r *result) ServiceNode(s *descriptorpb.ServiceDescriptorProto) *ast.ServiceNode {
	node, _ := r.nodes[s].(*ast.ServiceNode)
	return node
}

func (r *result) MethodNode(m *descriptorpb.MethodDescriptorProto) *ast.RPCNode {
	node, _ := r.nodes[m].(*ast.RPCNode)
	return node
}

// EnumDescriptor implements Result.
func (r *result) EnumDescriptor(n *ast.EnumNode) *descriptorpb.EnumDescriptorProto {
	if d, ok := r.nodesInverse[n]; ok {
		if ed, ok := d.(*descriptorpb.EnumDescriptorProto); ok {
			return ed
		}
	}
	return nil
}

// EnumReservedRangeDescriptor implements Result.
func (r *result) EnumReservedRangeDescriptor(n *ast.RangeNode) *descriptorpb.EnumDescriptorProto_EnumReservedRange {
	if d, ok := r.nodesInverse[n]; ok {
		if erd, ok := d.(*descriptorpb.EnumDescriptorProto_EnumReservedRange); ok {
			return erd
		}
	}
	return nil
}

// EnumValueDescriptor implements Result.
func (r *result) EnumValueDescriptor(n *ast.EnumValueNode) *descriptorpb.EnumValueDescriptorProto {
	if d, ok := r.nodesInverse[n]; ok {
		if evd, ok := d.(*descriptorpb.EnumValueDescriptorProto); ok {
			return evd
		}
	}
	return nil
}

// ExtensionRangeDescriptor implements Result.
func (r *result) ExtensionRangeDescriptor(n *ast.RangeNode) *descriptorpb.DescriptorProto_ExtensionRange {
	if d, ok := r.nodesInverse[n]; ok {
		if erd, ok := d.(*descriptorpb.DescriptorProto_ExtensionRange); ok {
			return erd
		}
	}
	return nil
}

// FieldDescriptor implements Result.
func (r *result) FieldDescriptor(n *ast.FieldDeclNode) *descriptorpb.FieldDescriptorProto {
	if d, ok := r.nodesInverse[n]; ok {
		if fd, ok := d.(*descriptorpb.FieldDescriptorProto); ok {
			return fd
		}
	}
	return nil
}

// MessageDescriptor implements Result.
func (r *result) MessageDescriptor(n *ast.MessageDeclNode) *descriptorpb.DescriptorProto {
	if d, ok := r.nodesInverse[n]; ok {
		if md, ok := d.(*descriptorpb.DescriptorProto); ok {
			return md
		}
	}
	return nil
}

// MessageReservedRangeDescriptor implements Result.
func (r *result) MessageReservedRangeDescriptor(n *ast.RangeNode) *descriptorpb.DescriptorProto_ReservedRange {
	if d, ok := r.nodesInverse[n]; ok {
		if mrd, ok := d.(*descriptorpb.DescriptorProto_ReservedRange); ok {
			return mrd
		}
	}
	return nil
}

// MethodDescriptor implements Result.
func (r *result) MethodDescriptor(n *ast.RPCNode) *descriptorpb.MethodDescriptorProto {
	if d, ok := r.nodesInverse[n]; ok {
		if md, ok := d.(*descriptorpb.MethodDescriptorProto); ok {
			return md
		}
	}
	return nil
}

// OneofDescriptor implements Result.
func (r *result) OneofDescriptor(n ast.Node) *descriptorpb.OneofDescriptorProto {
	if d, ok := r.nodesInverse[n]; ok {
		if od, ok := d.(*descriptorpb.OneofDescriptorProto); ok {
			return od
		}
	}
	return nil
}

// OptionDescriptor implements Result.
func (r *result) OptionDescriptor(n *ast.OptionNode) *descriptorpb.UninterpretedOption {
	if d, ok := r.nodesInverse[n]; ok {
		if od, ok := d.(*descriptorpb.UninterpretedOption); ok {
			return od
		}
	}
	return nil
}

// OptionNamePartDescriptor implements Result.
func (r *result) OptionNamePartDescriptor(n ast.Node) *descriptorpb.UninterpretedOption_NamePart {
	if d, ok := r.nodesInverse[n]; ok {
		if od, ok := d.(*descriptorpb.UninterpretedOption_NamePart); ok {
			return od
		}
	}
	return nil
}

// ServiceDescriptor implements Result.
func (r *result) ServiceDescriptor(n *ast.ServiceNode) *descriptorpb.ServiceDescriptorProto {
	if d, ok := r.nodesInverse[n]; ok {
		if sd, ok := d.(*descriptorpb.ServiceDescriptorProto); ok {
			return sd
		}
	}
	return nil
}

func (r *result) Descriptor(n ast.Node) proto.Message {
	if d, ok := r.nodesInverse[n]; ok {
		return d
	}
	return nil
}

func (r *result) putFileNode(f *descriptorpb.FileDescriptorProto, n *ast.FileNode) {
	r.nodes[f] = n
	r.nodesInverse[n] = f
}

func (r *result) putOptionNode(o *descriptorpb.UninterpretedOption, n *ast.OptionNode) {
	r.nodes[o] = n
	r.nodesInverse[n] = o
}

func (r *result) putOptionNamePartNode(o *descriptorpb.UninterpretedOption_NamePart, n *ast.FieldReferenceNode) {
	r.nodes[o] = n
	r.nodesInverse[n] = o
}

func (r *result) putMessageNode(m *descriptorpb.DescriptorProto, n *ast.MessageNode) {
	r.nodes[m] = n
	r.nodesInverse[n] = m
}

func (r *result) putFieldNode(f *descriptorpb.FieldDescriptorProto, n *ast.FieldNode) {
	r.nodes[f] = n
	r.nodesInverse[n] = f
}

func (r *result) putMapFieldNode(f *descriptorpb.FieldDescriptorProto, m *descriptorpb.DescriptorProto, n *ast.MapFieldNode) {
	r.nodes[f] = n
	r.nodes[m] = n
	r.nodesInverse[n] = f
}

func (r *result) putSyntheticFieldNode(f *descriptorpb.FieldDescriptorProto, n *ast.SyntheticMapField) {
	r.nodes[f] = n
	r.nodesInverse[n] = f
}

func (r *result) putGroupNode(f *descriptorpb.FieldDescriptorProto, m *descriptorpb.DescriptorProto, n *ast.GroupNode) {
	r.nodes[f] = n
	r.nodes[m] = n
	r.nodesInverse[n] = f
}

func (r *result) putOneofNode(o *descriptorpb.OneofDescriptorProto, n ast.Node) {
	r.nodes[o] = n
	r.nodesInverse[n] = o
}

func (r *result) putExtensionRangeNode(e *descriptorpb.DescriptorProto_ExtensionRange, n *ast.RangeNode) {
	r.nodes[e] = n
	r.nodesInverse[n] = e
}

func (r *result) putMessageReservedRangeNode(rr *descriptorpb.DescriptorProto_ReservedRange, n *ast.RangeNode) {
	r.nodes[rr] = n
	r.nodesInverse[n] = rr
}

func (r *result) putEnumNode(e *descriptorpb.EnumDescriptorProto, n *ast.EnumNode) {
	r.nodes[e] = n
	r.nodesInverse[n] = e
}

func (r *result) putEnumValueNode(e *descriptorpb.EnumValueDescriptorProto, n *ast.EnumValueNode) {
	r.nodes[e] = n
	r.nodesInverse[n] = e
}

func (r *result) putEnumReservedRangeNode(rr *descriptorpb.EnumDescriptorProto_EnumReservedRange, n *ast.RangeNode) {
	r.nodes[rr] = n
	r.nodesInverse[n] = rr
}

func (r *result) putServiceNode(s *descriptorpb.ServiceDescriptorProto, n *ast.ServiceNode) {
	r.nodes[s] = n
	r.nodesInverse[n] = s
}

func (r *result) putMethodNode(m *descriptorpb.MethodDescriptorProto, n *ast.RPCNode) {
	r.nodes[m] = n
	r.nodesInverse[n] = m
}

// NB: If we ever add other put*Node methods, to index other kinds of elements in the descriptor
//     proto hierarchy, we need to update the index recreation logic in clone.go, too.

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

// Package options contains the logic for interpreting options. The parse step
// of compilation stores the options in uninterpreted form, which contains raw
// identifiers and literal values.
//
// The process of interpreting an option is to resolve identifiers, by examining
// descriptors for the google.protobuf.*Options types and their available
// extensions (custom options). As field names are resolved, the values can be
// type-checked against the types indicated in field descriptors.
//
// On success, the various fields and extensions of the options message are
// populated and the field holding the uninterpreted form is cleared.
package options

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/internal/messageset"
	"github.com/kralicky/protocompile/linker"
	"github.com/kralicky/protocompile/parser"
	"github.com/kralicky/protocompile/protointernal"
	"github.com/kralicky/protocompile/reporter"
	"github.com/kralicky/protocompile/sourceinfo"
)

type interpreter struct {
	file                    file
	resolver                linker.Resolver
	overrideDescriptorProto linker.File
	lenient                 bool
	handler                 *reporter.Handler
	index                   sourceinfo.OptionIndex
	pathBuffer              []int32
	descriptorIndex         sourceinfo.OptionDescriptorIndex
}

type file interface {
	parser.Result
	ResolveMessageLiteralExtensionName(*ast.IdentValueNode) string
}

type noResolveFile struct {
	parser.Result
}

func (n noResolveFile) ResolveMessageLiteralExtensionName(*ast.IdentValueNode) string {
	return ""
}

// InterpreterOption is an option that can be passed to InterpretOptions and
// its variants.
type InterpreterOption func(*interpreter)

// WithOverrideDescriptorProto returns an option that indicates that the given file
// should be consulted when looking up a definition for an option type. The given
// file should usually have the path "google/protobuf/descriptor.proto". The given
// file will only be consulted if the option type is otherwise not visible to the
// file whose options are being interpreted.
func WithOverrideDescriptorProto(f linker.File) InterpreterOption {
	return func(interp *interpreter) {
		interp.overrideDescriptorProto = f
	}
}

func WithInterpretLenient() InterpreterOption {
	return func(interp *interpreter) {
		interp.lenient = true
	}
}

// InterpretOptions interprets options in the given linked result, returning
// an index that can be used to generate source code info. This step mutates
// the linked result's underlying proto to move option elements out of the
// "uninterpreted_option" fields and into proper option fields and extensions.
//
// The given handler is used to report errors and warnings. If any errors are
// reported, this function returns a non-nil error.
func InterpretOptions(linked linker.Result, handler *reporter.Handler, opts ...InterpreterOption) (sourceinfo.OptionIndex, sourceinfo.OptionDescriptorIndex, error) {
	return interpretOptions(linked, linker.ResolverFromFile(linked), handler, opts)
}

// InterpretOptionsLenient interprets options in a lenient/best-effort way in
// the given linked result, returning an index that can be used to generate
// source code info. This step mutates the linked result's underlying proto to
// move option elements out of the "uninterpreted_option" fields and into proper
// option fields and extensions.
//
// In lenient more, errors resolving option names and type errors are ignored.
// Any options that are uninterpretable (due to such errors) will remain in the
// "uninterpreted_option" fields.
func InterpretOptionsLenient(linked linker.Result, opts ...InterpreterOption) (sourceinfo.OptionIndex, sourceinfo.OptionDescriptorIndex, error) {
	return interpretOptions(linked, linker.ResolverFromFile(linked), reporter.NewHandler(nil), append(opts, WithInterpretLenient()))
}

// InterpretUnlinkedOptions does a best-effort attempt to interpret options in
// the given parsed result, returning an index that can be used to generate
// source code info. This step mutates the parsed result's underlying proto to
// move option elements out of the "uninterpreted_option" fields and into proper
// option fields and extensions.
//
// This is the same as InterpretOptionsLenient except that it accepts an
// unlinked result. Because the file is unlinked, custom options cannot be
// interpreted. Other errors resolving option names or type errors will be
// effectively ignored. Any options that are uninterpretable (due to such
// errors) will remain in the "uninterpreted_option" fields.
func InterpretUnlinkedOptions(parsed parser.Result, opts ...InterpreterOption) (sourceinfo.OptionIndex, sourceinfo.OptionDescriptorIndex, error) {
	return interpretOptions(noResolveFile{parsed}, nil, reporter.NewHandler(nil), append(opts, WithInterpretLenient()))
}

func interpretOptions(file file, res linker.Resolver, handler *reporter.Handler, interpOpts []InterpreterOption) (sourceinfo.OptionIndex, sourceinfo.OptionDescriptorIndex, error) {
	interp := interpreter{
		file:            file,
		resolver:        res,
		handler:         handler,
		index:           sourceinfo.OptionIndex{},
		descriptorIndex: sourceinfo.NewOptionDescriptorIndex(),
		pathBuffer:      make([]int32, 0, 16),
	}
	for _, opt := range interpOpts {
		opt(&interp)
	}
	// We have to do this in two phases. First we interpret non-custom options.
	// This allows us to handle standard options and features that may needed to
	// correctly reference the custom options in the second phase.
	if err := interp.interpretFileOptions(file, false); err != nil {
		return nil, sourceinfo.OptionDescriptorIndex{}, err
	}
	// Now we can do custom options.
	if err := interp.interpretFileOptions(file, true); err != nil {
		return nil, sourceinfo.OptionDescriptorIndex{}, err
	}
	return interp.index, interp.descriptorIndex, nil
}

func (interp *interpreter) interpretFileOptions(file file, customOpts bool) error {
	fd := file.FileDescriptorProto()
	prefix := fd.GetPackage()
	if prefix != "" {
		prefix += "."
	}
	err := interpretElementOptions(interp, fd.GetName(), targetTypeFile, fd, customOpts)
	if err != nil {
		return err
	}
	for _, md := range fd.GetMessageType() {
		fqn := prefix + md.GetName()
		if err := interp.interpretMessageOptions(fqn, md, customOpts); err != nil {
			return err
		}
	}
	for _, fld := range fd.GetExtension() {
		fqn := prefix + fld.GetName()
		if err := interp.interpretFieldOptions(fqn, fld, customOpts); err != nil {
			return err
		}
	}
	for _, ed := range fd.GetEnumType() {
		fqn := prefix + ed.GetName()
		if err := interp.interpretEnumOptions(fqn, ed, customOpts); err != nil {
			return err
		}
	}
	for _, sd := range fd.GetService() {
		fqn := prefix + sd.GetName()
		err := interpretElementOptions(interp, fqn, targetTypeService, sd, customOpts)
		if err != nil {
			return err
		}
		for _, mtd := range sd.GetMethod() {
			mtdFqn := fqn + "." + mtd.GetName()
			err := interpretElementOptions(interp, mtdFqn, targetTypeMethod, mtd, customOpts)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveDescriptor[T protoreflect.Descriptor](res linker.Resolver, name protoreflect.FullName) T {
	var zero T
	if res == nil {
		return zero
	}
	if len(name) > 0 && name[0] == '.' {
		name = name[1:]
	}
	desc, _ := res.FindDescriptorByName(name)
	typedDesc, ok := desc.(T)
	if ok {
		return typedDesc
	}
	return zero
}

func (interp *interpreter) resolveExtensionType(name string) (protoreflect.ExtensionTypeDescriptor, error) {
	if interp.resolver == nil {
		return nil, protoregistry.NotFound
	}
	if len(name) > 0 && name[0] == '.' {
		name = name[1:]
	}
	ext, err := interp.resolver.FindExtensionByName(protoreflect.FullName(name))
	if err != nil {
		return nil, err
	}
	return ext.TypeDescriptor(), nil
}

func (interp *interpreter) resolveOptionsType(name protoreflect.FullName) protoreflect.MessageDescriptor {
	md := resolveDescriptor[protoreflect.MessageDescriptor](interp.resolver, name)
	if md != nil {
		return md
	}
	if interp.overrideDescriptorProto == nil {
		return nil
	}
	if len(name) > 0 && name[0] == '.' {
		name = name[1:]
	}
	desc := interp.overrideDescriptorProto.FindDescriptorByName(name)
	if md, ok := desc.(protoreflect.MessageDescriptor); ok {
		return md
	}
	return nil
}

func (interp *interpreter) nodeInfo(n ast.Node) ast.NodeInfo {
	return interp.file.FileNode().NodeInfo(n)
}

func (interp *interpreter) interpretMessageOptions(fqn string, md *descriptorpb.DescriptorProto, customOpts bool) error {
	err := interpretElementOptions(interp, fqn, targetTypeMessage, md, customOpts)
	if err != nil {
		return err
	}
	for _, fld := range md.GetField() {
		fldFqn := fqn + "." + fld.GetName()
		if err := interp.interpretFieldOptions(fldFqn, fld, customOpts); err != nil {
			return err
		}
	}
	for _, ood := range md.GetOneofDecl() {
		oodFqn := fqn + "." + ood.GetName()
		err := interpretElementOptions(interp, oodFqn, targetTypeOneof, ood, customOpts)
		if err != nil {
			return err
		}
	}
	for _, fld := range md.GetExtension() {
		fldFqn := fqn + "." + fld.GetName()
		if err := interp.interpretFieldOptions(fldFqn, fld, customOpts); err != nil {
			return err
		}
	}
	for _, er := range md.GetExtensionRange() {
		erFqn := fmt.Sprintf("%s.%d-%d", fqn, er.GetStart(), er.GetEnd())
		err := interpretElementOptions(interp, erFqn, targetTypeExtensionRange, er, customOpts)
		if err != nil {
			return err
		}
	}
	for _, nmd := range md.GetNestedType() {
		nmdFqn := fqn + "." + nmd.GetName()
		if err := interp.interpretMessageOptions(nmdFqn, nmd, customOpts); err != nil {
			return err
		}
	}
	for _, ed := range md.GetEnumType() {
		edFqn := fqn + "." + ed.GetName()
		if err := interp.interpretEnumOptions(edFqn, ed, customOpts); err != nil {
			return err
		}
	}

	// We also copy features for map fields down to their synthesized key and value fields.
	for _, fld := range md.GetField() {
		entryName := protointernal.InitCap(protointernal.JSONName(fld.GetName())) + "Entry"
		if fld.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED ||
			fld.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE &&
				fld.GetTypeName() != "."+fqn+"."+entryName {
			// can't be a map field
			continue
		}
		if fld.Options == nil || fld.Options.Features == nil {
			// no features to propagate
			continue
		}
		for _, nmd := range md.GetNestedType() {
			if nmd.GetName() == entryName {
				// found the entry message
				if !nmd.GetOptions().GetMapEntry() {
					break // not a map
				}
				for _, mapField := range nmd.Field {
					if mapField.Options == nil {
						mapField.Options = &descriptorpb.FieldOptions{}
					}
					features := proto.Clone(fld.Options.Features).(*descriptorpb.FeatureSet) //nolint:errcheck
					if mapField.Options.Features != nil {
						proto.Merge(features, mapField.Options.Features)
					}
					mapField.Options.Features = features
				}
				break
			}
		}
	}

	return nil
}

var emptyFieldOptions = &descriptorpb.FieldOptions{}

func (interp *interpreter) interpretFieldOptions(fqn string, fld *descriptorpb.FieldDescriptorProto, customOpts bool) error {
	opts := fld.GetOptions()
	emptyOptionsAlreadyPresent := opts != nil && len(opts.GetUninterpretedOption()) == 0

	// For non-custom phase, first process pseudo-options
	if len(opts.GetUninterpretedOption()) > 0 && !customOpts {
		if err := interp.interpretFieldPseudoOptions(fqn, fld, opts); err != nil {
			return err
		}
	}

	// Must re-check length of uninterpreted options since above step could remove some.
	if len(opts.GetUninterpretedOption()) == 0 {
		// If the message has no other interpreted options, we clear it out. But don't
		// do that if the descriptor came in with empty options or if it already has
		// interpreted option fields.
		if opts != nil && !emptyOptionsAlreadyPresent && proto.Equal(fld.Options, emptyFieldOptions) {
			fld.Options = nil
		}
		return nil
	}

	// Then process actual options.
	return interpretElementOptions(interp, fqn, targetTypeField, fld, customOpts)
}

func (interp *interpreter) interpretFieldPseudoOptions(fqn string, fld *descriptorpb.FieldDescriptorProto, opts *descriptorpb.FieldOptions) error {
	scope := fmt.Sprintf("field %s", fqn)
	uo := opts.UninterpretedOption

	// process json_name pseudo-option
	index, err := protointernal.FindOption(interp.file, interp.handler, scope, uo, "json_name")
	if err != nil && !interp.lenient {
		return err
	}
	if index >= 0 {
		opt := uo[index]
		optNode := interp.file.OptionNode(opt)
		if opt.StringValue == nil {
			return interp.HandleTypeMismatchErrorf(nil, optNode.GetVal(), "%s: expecting string value for json_name option", scope)
		}
		jsonName := string(opt.StringValue)
		// Extensions don't support custom json_name values.
		// If the value is already set (via the descriptor) and doesn't match the default value, return an error.
		if fld.GetExtendee() != "" && jsonName != "" && jsonName != protointernal.JSONName(fld.GetName()) {
			return interp.HandleOptionForbiddenErrorf(nil, optNode.GetName(), "%s: option json_name is not allowed on extensions", scope)
		}
		// attribute source code info
		interp.index[optNode] = &sourceinfo.OptionSourceInfo{Path: []int32{-1, protointernal.FieldJSONNameTag}}

		uo = protointernal.RemoveOption(uo, index)
		if strings.HasPrefix(jsonName, "[") && strings.HasSuffix(jsonName, "]") {
			return interp.HandleOptionValueErrorf(nil, optNode.GetVal(), "%s: expecting string value for json_name option", scope)
		}
		name := string(opt.StringValue)
		if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
			return interp.HandleOptionValueErrorf(nil, optNode.GetVal(), "%s: option json_name value cannot start with '[' and end with ']'; that is reserved for representing extensions", scope)
		}
		interp.descriptorIndex.OptionsToFieldDescriptors[opt] = resolveDescriptor[protoreflect.FieldDescriptor](interp.resolver, protoreflect.FullName(fqn))
		fld.JsonName = proto.String(jsonName)
	}

	// and process default pseudo-option
	if index, err := interp.processDefaultOption(scope, fqn, fld, uo); err != nil && !interp.lenient {
		return err
	} else if index >= 0 {
		fldDesc := resolveDescriptor[protoreflect.FieldDescriptor](interp.resolver, protoreflect.FullName(fqn))
		interp.descriptorIndex.OptionsToFieldDescriptors[uo[index]] = fldDesc
		nm := interp.file.OptionNamePartNode(uo[index].Name[0])
		interp.descriptorIndex.FieldReferenceNodesToFieldDescriptors[nm] = fldDesc
		// attribute source code info
		optNode := interp.file.OptionNode(uo[index])
		interp.index[optNode] = &sourceinfo.OptionSourceInfo{Path: []int32{-1, protointernal.FieldDefaultTag}}

		if optNode != nil && fldDesc != nil && fldDesc.Kind() == protoreflect.EnumKind {
			interp.indexEnumValueRef(fldDesc, optNode.Val)
		}
		uo = protointernal.RemoveOption(uo, index)
	}

	opts.UninterpretedOption = uo
	return nil
}

func (interp *interpreter) processDefaultOption(scope string, fqn string, fld *descriptorpb.FieldDescriptorProto, uos []*descriptorpb.UninterpretedOption) (defaultIndex int, err error) {
	found, err := protointernal.FindOption(interp.file, interp.handler, scope, uos, "default")
	if err != nil || found == -1 {
		return -1, err
	}
	opt := uos[found]
	optNode := interp.file.OptionNode(opt)
	if fld.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return -1, interp.HandleOptionForbiddenErrorf(nil, optNode.GetName(), "%s: default value cannot be set because field is repeated", scope)
	}
	if fld.GetType() == descriptorpb.FieldDescriptorProto_TYPE_GROUP || fld.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		return -1, interp.HandleOptionForbiddenErrorf(nil, optNode.GetName(), "%s: default value cannot be set because field is a message", scope)
	}
	mc := &protointernal.MessageContext{
		File:        interp.file,
		ElementName: fqn,
		ElementType: descriptorType(fld),
		Option:      opt,
	}

	val := optNode.GetVal()
	var v interface{}
	if val.Value() == nil {
		// no value in the AST, so we dig the value out of the uninterpreted option proto
		v, err = interp.defaultValueFromProto(mc, fld, opt, val)
	} else {
		// compute value from AST
		v, err = interp.defaultValue(mc, fld, val)
	}
	if err != nil {
		return -1, err
	}

	if str, ok := v.(string); ok {
		fld.DefaultValue = proto.String(str)
	} else if b, ok := v.([]byte); ok {
		fld.DefaultValue = proto.String(encodeDefaultBytes(b))
	} else {
		var flt float64
		var ok bool
		if flt, ok = v.(float64); !ok {
			var flt32 float32
			if flt32, ok = v.(float32); ok {
				flt = float64(flt32)
			}
		}
		if ok {
			switch {
			case math.IsInf(flt, 1):
				fld.DefaultValue = proto.String("inf")
			case math.IsInf(flt, -1):
				fld.DefaultValue = proto.String("-inf")
			case math.IsNaN(flt):
				fld.DefaultValue = proto.String("nan")
			default:
				fld.DefaultValue = proto.String(fmt.Sprintf("%v", v))
			}
		} else {
			fld.DefaultValue = proto.String(fmt.Sprintf("%v", v))
		}
	}
	return found, nil
}

func (interp *interpreter) defaultValue(mc *protointernal.MessageContext, fld *descriptorpb.FieldDescriptorProto, val *ast.ValueNode) (interface{}, error) {
	if msgLit := val.GetMessageLiteral(); msgLit != nil {
		return -1, interp.HandleOptionForbiddenErrorf(mc, val, "default value cannot be a message")
	}
	if fld.GetType() == descriptorpb.FieldDescriptorProto_TYPE_ENUM {
		ed := resolveDescriptor[protoreflect.EnumDescriptor](interp.resolver, protoreflect.FullName(fld.GetTypeName()))
		if ed == nil {
			return -1, interp.HandleOptionValueErrorf(mc, val, "unable to resolve enum type %q for field %q", fld.GetTypeName(), fld.GetName())
		}
		_, name, err := interp.enumFieldValue(mc, ed, val, false)
		if err != nil {
			return -1, err
		}
		return string(name), nil
	}
	return interp.scalarFieldValue(mc, fld.GetType(), val, false)
}

func (interp *interpreter) defaultValueFromProto(mc *protointernal.MessageContext, fld *descriptorpb.FieldDescriptorProto, opt *descriptorpb.UninterpretedOption, node ast.Node) (interface{}, error) {
	if opt.AggregateValue != nil {
		return -1, interp.HandleOptionValueErrorf(mc, node, "default value cannot be a message")
	}
	if fld.GetType() == descriptorpb.FieldDescriptorProto_TYPE_ENUM {
		ed := resolveDescriptor[protoreflect.EnumDescriptor](interp.resolver, protoreflect.FullName(fld.GetTypeName()))
		if ed == nil {
			return -1, interp.HandleOptionValueErrorf(mc, node, "unable to resolve enum type %q for field %q", fld.GetTypeName(), fld.GetName())
		}
		_, name, err := interp.enumFieldValueFromProto(mc, ed, opt, node)
		if err != nil {
			return nil, err
		}
		return string(name), nil
	}
	return interp.scalarFieldValueFromProto(mc, fld.GetType(), opt, node)
}

func encodeDefaultBytes(b []byte) string {
	var buf bytes.Buffer
	protointernal.WriteEscapedBytes(&buf, b)
	return buf.String()
}

func (interp *interpreter) interpretEnumOptions(fqn string, ed *descriptorpb.EnumDescriptorProto, customOpts bool) error {
	err := interpretElementOptions(interp, fqn, targetTypeEnum, ed, customOpts)
	if err != nil {
		return err
	}
	for _, evd := range ed.GetValue() {
		evdFqn := fqn + "." + evd.GetName()
		err := interpretElementOptions(interp, evdFqn, targetTypeEnumValue, evd, customOpts)
		if err != nil {
			return err
		}
	}
	return nil
}

func interpretElementOptions[Elem elementType[OptsStruct, Opts], OptsStruct any, Opts optionsType[OptsStruct]](
	interp *interpreter,
	fqn string,
	target *targetType[Elem, OptsStruct, Opts],
	elem Elem,
	customOpts bool,
) error {
	opts := elem.GetOptions()
	uninterpreted := opts.GetUninterpretedOption()
	if len(uninterpreted) > 0 {
		remain, err := interp.interpretOptions(fqn, target.t, elem, opts, uninterpreted, customOpts)
		if err != nil {
			return err
		}
		target.setUninterpretedOptions(opts, remain)
	} else if customOpts {
		// If customOpts is true, we are in second pass of interpreting.
		// For second pass, even if there are no options to interpret, we still
		// need to verify feature usage.
		features := opts.GetFeatures()
		var msg protoreflect.Message
		if len(features.ProtoReflect().GetUnknown()) > 0 {
			// We need to first convert to a message that uses the sources' definition
			// of FeatureSet.
			optsDesc := opts.ProtoReflect().Descriptor()
			optsFqn := optsDesc.FullName()
			if md := interp.resolveOptionsType(optsFqn); md != nil {
				dm := dynamicpb.NewMessage(md)
				if err := cloneInto(dm, opts, interp.resolver); err != nil {
					node := interp.file.Node(elem)
					return interp.handler.HandleError(reporter.Error(interp.nodeInfo(node), err))
				}
				msg = dm
			}
		}
		if msg == nil {
			msg = opts.ProtoReflect()
		}
		mc := &protointernal.MessageContext{
			File:        interp.file,
			ElementName: fqn,
			ElementType: target.t.String(),
			Option:      nil,
		}
		err := interp.validateRecursive(mc, false, msg, "", elem, nil, false, false, false)
		if err != nil {
			return err
		}
	}
	return nil
}

// interpretOptions processes the options in uninterpreted, which are interpreted as fields
// of the given opts message. The first return value is the features to use for child elements.
// On success, the latter two return values will usually be nil, nil. But if the current
// operation is lenient, it may return a non-nil slice of uninterpreted options on success.
// In such a case, the returned slice contains the options which could not be interpreted.
func (interp *interpreter) interpretOptions(
	fqn string,
	targetType descriptorpb.FieldOptions_OptionTargetType,
	element, opts proto.Message,
	uninterpreted []*descriptorpb.UninterpretedOption,
	customOpts bool,
) ([]*descriptorpb.UninterpretedOption, error) {
	optsDesc := opts.ProtoReflect().Descriptor()
	optsFqn := optsDesc.FullName()
	var msg protoreflect.Message
	// see if the parse included an override copy for these options
	if md := interp.resolveOptionsType(optsFqn); md != nil {
		dm := dynamicpb.NewMessage(md)
		if err := cloneInto(dm, opts, interp.resolver); err != nil {
			node := interp.file.Node(element)
			return nil, interp.HandleOptionNotFoundErrorf(nil, node, "failed to resolve option type: %w", err)
		}
		msg = dm
	} else {
		msg = proto.Clone(opts).ProtoReflect()
	}

	mc := &protointernal.MessageContext{
		File:        interp.file,
		ElementName: fqn,
		ElementType: descriptorType(element),
	}
	var remain []*descriptorpb.UninterpretedOption
	for _, uo := range uninterpreted {
		if len(uo.Name) == 0 {
			continue
		}
		isCustom := uo.Name[0].GetIsExtension()
		if isCustom != customOpts {
			// We're not looking at these this phase.
			remain = append(remain, uo)
			continue
		}
		firstName := uo.Name[0].GetNamePart()
		if targetType == descriptorpb.FieldOptions_TARGET_TYPE_FIELD &&
			!isCustom && (firstName == "default" || firstName == "json_name") {
			// Field pseudo-option that we can skip and is handled elsewhere.
			remain = append(remain, uo)
			continue
		}
		node := interp.file.OptionNode(uo)
		if !isCustom && firstName == "uninterpreted_option" {
			if interp.lenient {
				remain = append(remain, uo)
				continue
			}
			// uninterpreted_option might be found reflectively, but is not actually valid for use
			if err := interp.HandleOptionForbiddenErrorf(mc, node.GetName(), "invalid option 'uninterpreted_option'"); err != nil {
				return nil, err
			}
			continue
		}
		mc.Option = uo
		srcInfo, err := interp.interpretField(targetType, mc, msg, uo, 0, interp.pathBuffer)
		if err != nil {
			if interp.lenient {
				remain = append(remain, uo)
				continue
			}
			return nil, err
		}
		if srcInfo != nil {
			interp.index[node] = srcInfo
		}
	}

	// customOpts is true for the second pass, which is also when we want to validate feature usage.
	doValidation := customOpts
	if doValidation {
		validateRequiredFields := !interp.lenient
		err := interp.validateRecursive(mc, validateRequiredFields, msg, "", element, nil, false, false, false)
		if err != nil {
			return nil, err
		}
	}

	if interp.lenient {
		// If we're lenient, then we don't want to clobber the passed in message
		// and leave it partially populated. So we convert into a copy first
		optsClone := opts.ProtoReflect().New().Interface()
		if err := cloneInto(optsClone, msg.Interface(), interp.resolver); err != nil {
			// TODO: do this in a more granular way, so we can convert individual
			// fields and leave bad ones uninterpreted instead of skipping all of
			// the work we've done so far.
			return uninterpreted, nil
		}
		if doValidation {
			if err := proto.CheckInitialized(optsClone); err != nil {
				// Conversion from dynamic message failed to set some required fields.
				// TODO above applies here as well...
				return uninterpreted, nil
			}
		}
		// conversion from dynamic message above worked, so now
		// it is safe to overwrite the passed in message
		proto.Reset(opts)
		proto.Merge(opts, optsClone)

		return remain, nil
	}

	// now try to convert into the passed in message and fail if not successful
	if err := cloneInto(opts, msg.Interface(), interp.resolver); err != nil {
		node := interp.file.Node(element)
		return nil, interp.HandleOptionValueErrorf(nil, node, "error in %s options: %w", descriptorType(element), err)
	}

	return remain, nil
}

// checkFieldUsage verifies that the given option field can be used
// for the given target type. It reports an error if not and returns
// a non-nil error if the handler returned a non-nil error.
func (interp *interpreter) checkFieldUsage(
	targetType descriptorpb.FieldOptions_OptionTargetType,
	mc *protointernal.MessageContext,
	fld protoreflect.FieldDescriptor,
	node ast.Node,
) error {
	msgOpts, _ := fld.ContainingMessage().Options().(*descriptorpb.MessageOptions)
	if msgOpts.GetMessageSetWireFormat() && !messageset.CanSupportMessageSets() {
		err := interp.HandleOptionForbiddenErrorf(mc, node, "field %q may not be used in an option: it uses 'message set wire format' legacy proto1 feature which is not supported", fld.FullName())
		if err != nil {
			return err
		}
	}

	opts, ok := fld.Options().(*descriptorpb.FieldOptions)
	if !ok {
		return nil
	}
	targetTypes := opts.GetTargets()
	if len(targetTypes) == 0 {
		return nil
	}
	for _, allowedType := range targetTypes {
		if allowedType == targetType {
			return nil
		}
	}
	allowedTypes := make([]string, len(targetTypes))
	for i, t := range targetTypes {
		allowedTypes[i] = targetTypeString(t)
	}
	if len(targetTypes) == 1 && targetTypes[0] == descriptorpb.FieldOptions_TARGET_TYPE_UNKNOWN {
		return interp.HandleOptionForbiddenErrorf(mc, node, "field %q may not be used in an option (it declares no allowed target types)", fld.FullName())
	}
	return interp.HandleOptionForbiddenErrorf(mc, node, "field %q is allowed on [%s], not on %s", fld.FullName(), strings.Join(allowedTypes, ","), targetTypeString(targetType))
}

func targetTypeString(t descriptorpb.FieldOptions_OptionTargetType) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(t.String(), "TARGET_TYPE_"), "_", " "))
}

func editionString(t descriptorpb.Edition) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(t.String(), "EDITION_"), "_", "-"))
}

func cloneInto(dest proto.Message, src proto.Message, res linker.Resolver) error {
	if dest.ProtoReflect().Descriptor() == src.ProtoReflect().Descriptor() {
		proto.Reset(dest)
		proto.Merge(dest, src)
		return nil
	}

	// If descriptors are not the same, we could have field descriptors in src that
	// don't match the ones in dest. There's no easy/sane way to handle that. So we
	// just marshal to bytes and back to do this
	marshaler := proto.MarshalOptions{
		// We've already validated required fields before this point,
		// so we can allow partial here.
		AllowPartial: true,
	}
	data, err := marshaler.Marshal(src)
	if err != nil {
		return err
	}
	unmarshaler := proto.UnmarshalOptions{AllowPartial: true}
	if res != nil {
		unmarshaler.Resolver = res
	} else {
		// Use a typed nil, which returns "not found" to all queries
		// and prevents fallback to protoregistry.GlobalTypes.
		unmarshaler.Resolver = (*protoregistry.Types)(nil)
	}
	return unmarshaler.Unmarshal(data, dest)
}

func (interp *interpreter) validateRecursive(
	mc *protointernal.MessageContext,
	validateRequiredFields bool,
	msg protoreflect.Message,
	prefix string,
	element proto.Message,
	path []int32,
	isFeatures bool,
	inFeatures bool,
	inMap bool,
) error {
	if validateRequiredFields {
		flds := msg.Descriptor().Fields()
		var missingFields []string
		for i := 0; i < flds.Len(); i++ {
			fld := flds.Get(i)
			if fld.Cardinality() == protoreflect.Required && !msg.Has(fld) {
				missingFields = append(missingFields, fmt.Sprintf("%s%s", prefix, fld.Name()))
			}
		}
		if len(missingFields) > 0 {
			node := interp.findOptionNode(path, element)
			err := interp.HandleOptionValueErrorf(mc, node, "error in %s options: some required fields missing: %v", descriptorType(element), strings.Join(missingFields, ", "))
			if err != nil {
				return err
			}
		}
	}

	var err error
	msg.Range(func(fld protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		chpath := path
		if !inMap {
			chpath = append(chpath, int32(fld.Number()))
		}
		chInFeatures := isFeatures || inFeatures
		chIsFeatures := !chInFeatures && len(path) == 0 && fld.Name() == "features"

		if (isFeatures || (inFeatures && fld.IsExtension())) &&
			interp.file.FileNode().Name() == fld.ParentFile().Path() {
			var what, name string
			if fld.IsExtension() {
				what = "custom feature"
				name = "(" + string(fld.FullName()) + ")"
			} else {
				what = "feature"
				name = string(fld.Name())
			}
			node := interp.findOptionNode(path, element)
			err = interp.HandleOptionForbiddenErrorf(mc, node, "%s %s cannot be used from the same file in which it is defined", what, name)
			if err != nil {
				return false
			}
		}

		if chInFeatures {
			// Validate feature usage against feature settings.

			// First, check the feature support settings of the field.
			opts, _ := fld.Options().(*descriptorpb.FieldOptions)
			edition := interp.file.FileDescriptorProto().GetEdition()
			if opts != nil && opts.FeatureSupport != nil {
				err = interp.validateFeatureSupport(mc, edition, opts.FeatureSupport, "field", string(fld.FullName()), chpath, element)
				if err != nil {
					return false
				}
			}
			// Then, if it's an enum or has an enum, check the feature support settings of the enum values.
			var enum protoreflect.EnumDescriptor
			if fld.Enum() != nil {
				enum = fld.Enum()
			} else if fld.IsMap() && fld.MapValue().Enum() != nil {
				enum = fld.MapValue().Enum()
			}
			if enum != nil {
				switch {
				case fld.IsMap():
					val.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
						// Can't construct path to particular map entry since we don't this entry's index.
						// So we leave chpath alone, and it will have to point to the whole map value (or
						// the first entry if the map is de-structured across multiple option statements).
						err = interp.validateEnumValueFeatureSupport(mc, edition, enum, v.Enum(), chpath, element)
						return err == nil
					})
					if err != nil {
						return false
					}
				case fld.IsList():
					sl := val.List()
					for i := 0; i < sl.Len(); i++ {
						v := sl.Get(i)
						err = interp.validateEnumValueFeatureSupport(mc, edition, enum, v.Enum(), append(chpath, int32(i)), element)
						if err != nil {
							return false
						}
					}
				default:
					err = interp.validateEnumValueFeatureSupport(mc, edition, enum, val.Enum(), chpath, element)
					if err != nil {
						return false
					}
				}
			}
		}

		// If it's a message or contains a message, recursively validate fields in those messages.
		switch {
		case fld.IsMap() && fld.MapValue().Message() != nil:
			val.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
				chprefix := fmt.Sprintf("%s%s[%v].", prefix, fieldName(fld), k)
				err = interp.validateRecursive(mc, validateRequiredFields, v.Message(), chprefix, element, chpath, chIsFeatures, chInFeatures, true)
				return err == nil
			})
			if err != nil {
				return false
			}
		case fld.IsList() && fld.Message() != nil:
			sl := val.List()
			for i := 0; i < sl.Len(); i++ {
				v := sl.Get(i)
				chprefix := fmt.Sprintf("%s%s[%d].", prefix, fieldName(fld), i)
				if !inMap {
					chpath = append(chpath, int32(i))
				}
				err = interp.validateRecursive(mc, validateRequiredFields, v.Message(), chprefix, element, chpath, chIsFeatures, chInFeatures, inMap)
				if err != nil {
					return false
				}
			}
		case !fld.IsMap() && fld.Message() != nil:
			chprefix := fmt.Sprintf("%s%s.", prefix, fieldName(fld))
			err = interp.validateRecursive(mc, validateRequiredFields, val.Message(), chprefix, element, chpath, chIsFeatures, chInFeatures, inMap)
			if err != nil {
				return false
			}
		}
		return true
	})
	return err
}

func (interp *interpreter) validateEnumValueFeatureSupport(
	mc *protointernal.MessageContext,
	edition descriptorpb.Edition,
	enum protoreflect.EnumDescriptor,
	number protoreflect.EnumNumber,
	path []int32,
	element proto.Message,
) error {
	enumVal := enum.Values().ByNumber(number)
	if enumVal == nil {
		return nil
	}
	enumValOpts, _ := enumVal.Options().(*descriptorpb.EnumValueOptions)
	if enumValOpts == nil || enumValOpts.FeatureSupport == nil {
		return nil
	}
	return interp.validateFeatureSupport(mc, edition, enumValOpts.FeatureSupport, "enum value", string(enumVal.Name()), path, element)
}

func (interp *interpreter) validateFeatureSupport(
	mc *protointernal.MessageContext,
	edition descriptorpb.Edition,
	featureSupport *descriptorpb.FieldOptions_FeatureSupport,
	what string,
	name string,
	path []int32,
	element proto.Message,
) error {
	if featureSupport.EditionIntroduced != nil && edition < featureSupport.GetEditionIntroduced() {
		node := interp.findOptionNode(path, element)
		err := interp.HandleOptionForbiddenErrorf(mc, node, "%s %q was not introduced until edition %s", what, name, editionString(featureSupport.GetEditionIntroduced()))
		if err != nil {
			return err
		}
	}
	if featureSupport.EditionRemoved != nil && edition >= featureSupport.GetEditionRemoved() {
		node := interp.findOptionNode(path, element)
		err := interp.HandleOptionForbiddenErrorf(mc, node, "%s %q was removed in edition %s", what, name, editionString(featureSupport.GetEditionRemoved()))
		if err != nil {
			return err
		}
	}
	if featureSupport.EditionDeprecated != nil && edition >= featureSupport.GetEditionDeprecated() {
		node := interp.findOptionNode(path, element)
		var suffix string
		if featureSupport.GetDeprecationWarning() != "" {
			suffix = ": " + featureSupport.GetDeprecationWarning()
		}
		interp.handler.HandleWarningf(interp.nodeInfo(node), "%s %q is deprecated as of edition %s%s", what, name, editionString(featureSupport.GetEditionDeprecated()), suffix)
	}
	return nil
}

func (interp *interpreter) findOptionNode(
	path []int32,
	element proto.Message,
) ast.Node {
	elementNode := interp.file.Node(element)
	node, _ := findOptionNode[*ast.OptionNode](
		path,
		optionsRanger{elementNode},
		func(n *ast.OptionNode) *sourceinfo.OptionSourceInfo {
			return interp.index[n]
		},
	)
	if node != nil {
		return node
	}
	return elementNode
}

func findOptionNode[N ast.Node](
	path []int32,
	nodes interface {
		Range(func(N, *ast.ValueNode) bool)
	},
	srcInfoAccessor func(N) *sourceinfo.OptionSourceInfo,
) (ast.Node, int) {
	var bestMatch ast.Node
	var bestMatchLen int
	nodes.Range(func(node N, val *ast.ValueNode) bool {
		srcInfo := srcInfoAccessor(node)
		if srcInfo.Path[0] < 0 {
			// negative first value means it's a field pseudo-option; skip
			return true
		}
		match, matchLen := findOptionValueNode(path, node, val, srcInfo)
		if matchLen > bestMatchLen {
			bestMatch = match
			bestMatchLen = matchLen
			if matchLen >= len(path) {
				// not going to find a better one
				return false
			}
		}
		return true
	})
	return bestMatch, bestMatchLen
}

type optionsRanger struct {
	node ast.Node
}

func (r optionsRanger) Range(f func(*ast.OptionNode, *ast.ValueNode) bool) {
	ast.Inspect(r.node, func(optNode ast.Node) bool {
		if optNode, ok := optNode.(*ast.OptionNode); ok {
			return f(optNode, optNode.Val)
		}
		return true
	})
}

type valueRanger []*ast.ValueNode

func (r valueRanger) Range(f func(*ast.ValueNode, *ast.ValueNode) bool) {
	for _, elem := range r {
		if !f(elem, elem) {
			return
		}
	}
}

type fieldRanger map[*ast.MessageFieldNode]*sourceinfo.OptionSourceInfo

func (r fieldRanger) Range(f func(*ast.MessageFieldNode, *ast.ValueNode) bool) {
	for elem := range r {
		if !f(elem, elem.GetVal()) {
			return
		}
	}
}

func isPathMatch(a, b []int32) bool {
	length := len(a)
	if len(b) < length {
		length = len(b)
	}
	for i := 0; i < length; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func findOptionValueNode(
	path []int32,
	node ast.Node,
	value *ast.ValueNode,
	srcInfo *sourceinfo.OptionSourceInfo,
) (ast.Node, int) {
	srcInfoPath := srcInfo.Path
	if _, ok := srcInfo.Children.(*sourceinfo.ArrayLiteralSourceInfo); ok {
		// Last path element for array source info is the index of the
		// first element. So exclude in the comparison, since path could
		// indicate a later index, which is present in the array.
		srcInfoPath = srcInfo.Path[:len(srcInfo.Path)-1]
	}

	if !isPathMatch(path, srcInfoPath) {
		return nil, 0
	}
	if len(srcInfoPath) >= len(path) {
		return node, len(path)
	}

	switch children := srcInfo.Children.(type) {
	case *sourceinfo.ArrayLiteralSourceInfo:
		array := value.GetArrayLiteral()
		if array == nil {
			break // should never happen
		}
		var i int
		match, matchLen := findOptionNode[*ast.ValueNode](
			path,
			valueRanger(array.FilterValues()),
			func(_ *ast.ValueNode) *sourceinfo.OptionSourceInfo {
				val := &children.Elements[i]
				i++
				return val
			},
		)
		if match != nil {
			return match, matchLen
		}

	case *sourceinfo.MessageLiteralSourceInfo:
		match, matchLen := findOptionNode[*ast.MessageFieldNode](
			path,
			fieldRanger(children.Fields),
			func(n *ast.MessageFieldNode) *sourceinfo.OptionSourceInfo {
				return children.Fields[n]
			},
		)
		if match != nil {
			return match, matchLen
		}
	}

	return node, len(srcInfoPath)
}

// interpretField interprets the option described by opt, as a field inside the given msg. This
// interprets components of the option name starting at nameIndex. When nameIndex == 0, then
// msg must be an options message. For nameIndex > 0, msg is a nested message inside of the
// options message. The given pathPrefix is the path (sequence of field numbers and indices
// with a FileDescriptorProto as the start) up to but not including the given nameIndex.
//
// Any errors encountered will be handled, so the returned error will only be non-nil if
// the handler returned non-nil. Callers must check that the source info is non-nil before
// using it since it can be nil (in the event of a problem) even if the error is nil.
func (interp *interpreter) interpretField(
	targetType descriptorpb.FieldOptions_OptionTargetType,
	mc *protointernal.MessageContext,
	msg protoreflect.Message,
	opt *descriptorpb.UninterpretedOption,
	nameIndex int,
	pathPrefix []int32,
) (*sourceinfo.OptionSourceInfo, error) {
	var fld protoreflect.FieldDescriptor
	nm := opt.GetName()[nameIndex]
	node := interp.file.OptionNamePartNode(nm)
	if nm.GetIsExtension() {
		extName := nm.GetNamePart()
		if extName[0] == '.' {
			extName = extName[1:] /* skip leading dot */
		}
		var err error
		fld, err = interp.resolveExtensionType(extName)
		if err != nil {
			return nil, interp.HandleOptionNotFoundErrorf(mc, node, "unrecognized extension %s of %s", extName, msg.Descriptor().FullName())
		}
		if fld.ContainingMessage().FullName() != msg.Descriptor().FullName() {
			return nil, interp.HandleOptionForbiddenErrorf(mc, node, "extension %s should extend %s but instead extends %s", extName, msg.Descriptor().FullName(), fld.ContainingMessage().FullName())
		}
	} else {
		fld = msg.Descriptor().Fields().ByName(protoreflect.Name(nm.GetNamePart()))
		if fld == nil {
			return nil, interp.HandleOptionNotFoundErrorf(mc, node, "field %s of %s does not exist", nm.GetNamePart(), msg.Descriptor().FullName())
		}
	}
	interp.descriptorIndex.UninterpretedNameDescriptorsToFieldDescriptors[nm] = fld
	interp.descriptorIndex.FieldReferenceNodesToFieldDescriptors[node] = fld
	interp.descriptorIndex.OptionsToFieldDescriptors[opt] = fld
	pathPrefix = append(pathPrefix, int32(fld.Number()))

	if err := interp.checkFieldUsage(targetType, mc, fld, node); err != nil {
		return nil, err
	}

	if len(opt.GetName()) > nameIndex+1 {
		nextnm := opt.GetName()[nameIndex+1]
		nextnode := interp.file.OptionNamePartNode(nextnm)
		k := fld.Kind()
		if k != protoreflect.MessageKind && k != protoreflect.GroupKind {
			return nil, interp.HandleOptionForbiddenErrorf(mc, nextnode, "cannot set field %s because %s is not a message", nextnm.GetNamePart(), nm.GetNamePart())
		}
		if fld.Cardinality() == protoreflect.Repeated {
			return nil, interp.HandleOptionForbiddenErrorf(mc, nextnode, "cannot set field %s because %s is repeated (must use an aggregate)", nextnm.GetNamePart(), nm.GetNamePart())
		}
		var fdm protoreflect.Message
		if msg.Has(fld) {
			v := msg.Mutable(fld)
			fdm = v.Message()
		} else {
			if ood := fld.ContainingOneof(); ood != nil {
				existingFld := msg.WhichOneof(ood)
				if existingFld != nil && existingFld.Number() != fld.Number() {
					return nil, interp.HandleOptionForbiddenErrorf(mc, node, "oneof %q already has field %q set", ood.Name(), fieldName(existingFld))
				}
			}
			fldVal := msg.NewField(fld)
			fdm = fldVal.Message()
			msg.Set(fld, fldVal)
		}
		// recurse to set next part of name
		return interp.interpretField(targetType, mc, fdm, opt, nameIndex+1, pathPrefix)
	}

	optNode := interp.file.OptionNode(opt)
	optValNode := optNode.GetVal()

	var srcInfo *sourceinfo.OptionSourceInfo
	var err error
	if optValNode.Value() == nil {
		err = interp.setOptionFieldFromProto(targetType, mc, msg, fld, node, opt, optValNode)
		srcInfoVal := newSrcInfo(pathPrefix, nil)
		srcInfo = &srcInfoVal
	} else {
		srcInfo, err = interp.setOptionField(targetType, mc, msg, fld, node, optValNode, false, pathPrefix)
	}

	if err != nil {
		return nil, interp.handler.HandleError(err)
	}
	return srcInfo, nil
}

// setOptionField sets the value for field fld in the given message msg to the value represented
// by AST node val. The given name is the AST node that corresponds to the name of fld. On success,
// it returns additional metadata about the field that was set.
func (interp *interpreter) setOptionField(
	targetType descriptorpb.FieldOptions_OptionTargetType,
	mc *protointernal.MessageContext,
	msg protoreflect.Message,
	fld protoreflect.FieldDescriptor,
	name ast.Node,
	val *ast.ValueNode,
	insideMsgLiteral bool,
	pathPrefix []int32,
) (*sourceinfo.OptionSourceInfo, error) {
	v := val.Value()
	if sl, ok := v.([]*ast.ValueNode); ok {
		// handle slices a little differently than the others
		if fld.Cardinality() != protoreflect.Repeated {
			return nil, interp.HandleOptionForbiddenErrorf(mc, name, "value is an array but field is not repeated")
		}
		origPath := mc.OptAggPath
		defer func() {
			mc.OptAggPath = origPath
		}()
		childVals := make([]sourceinfo.OptionSourceInfo, len(sl))
		var firstIndex int
		if fld.IsMap() {
			firstIndex = msg.Get(fld).Map().Len()
		} else {
			firstIndex = msg.Get(fld).List().Len()
		}
		for index, item := range sl {
			mc.OptAggPath = fmt.Sprintf("%s[%d]", origPath, index)
			value, srcInfo, err := interp.fieldValue(targetType, mc, msg, fld, item, insideMsgLiteral, append(pathPrefix, int32(firstIndex+index)))
			if err != nil || !value.IsValid() {
				return nil, err
			}
			if fld.IsMap() {
				mv := msg.Mutable(fld).Map()
				setMapEntry(fld, msg, mv, value.Message())
			} else {
				lv := msg.Mutable(fld).List()
				lv.Append(value)
			}
			childVals[index] = srcInfo
		}
		srcInfo := newSrcInfo(append(pathPrefix, int32(firstIndex)), &sourceinfo.ArrayLiteralSourceInfo{Elements: childVals})
		return &srcInfo, nil
	}

	if fld.IsMap() {
		pathPrefix = append(pathPrefix, int32(msg.Get(fld).Map().Len()))
	} else if fld.IsList() {
		pathPrefix = append(pathPrefix, int32(msg.Get(fld).List().Len()))
	}

	value, srcInfo, err := interp.fieldValue(targetType, mc, msg, fld, val, insideMsgLiteral, pathPrefix)
	if err != nil || !value.IsValid() {
		return nil, err
	}
	if !value.IsValid() {
		return nil, interp.HandleOptionValueErrorf(mc, val, "invalid value")
	}

	if ood := fld.ContainingOneof(); ood != nil {
		existingFld := msg.WhichOneof(ood)
		if existingFld != nil && existingFld.Number() != fld.Number() {
			return nil, interp.HandleOptionForbiddenErrorf(mc, name, "oneof %q already has field %q set", ood.Name(), fieldName(existingFld))
		}
	}

	switch {
	case fld.IsMap():
		mv := msg.Mutable(fld).Map()
		setMapEntry(fld, msg, mv, value.Message())
	case fld.IsList():
		lv := msg.Mutable(fld).List()
		lv.Append(value)
	default:
		if msg.Has(fld) {
			return nil, interp.HandleOptionForbiddenErrorf(mc, name, "non-repeated option field %s already set", fieldName(fld))
		}
		msg.Set(fld, value)
	}
	return &srcInfo, nil
}

// setOptionFieldFromProto sets the value for field fld in the given message msg to the value
// represented by the given uninterpreted option. The given ast.Node, if non-nil, will be used
// to report source positions in error messages. On success, it returns additional metadata
// about the field that was set.
func (interp *interpreter) setOptionFieldFromProto(
	targetType descriptorpb.FieldOptions_OptionTargetType,
	mc *protointernal.MessageContext,
	msg protoreflect.Message,
	fld protoreflect.FieldDescriptor,
	name ast.Node,
	opt *descriptorpb.UninterpretedOption,
	node ast.Node,
) error {
	k := fld.Kind()
	var value protoreflect.Value
	switch k {
	case protoreflect.EnumKind:
		num, _, err := interp.enumFieldValueFromProto(mc, fld.Enum(), opt, node)
		if err != nil {
			return interp.HandleOptionValueErrorf(mc, node, "%w", err)
		}
		value = protoreflect.ValueOfEnum(num)

	case protoreflect.MessageKind, protoreflect.GroupKind:
		if opt.AggregateValue == nil {
			return interp.HandleOptionValueErrorf(mc, node, "expecting message, got %s", optionValueKind(opt))
		}
		// We must parse the text format from the aggregate value string
		var elem protoreflect.Message
		switch {
		case fld.IsMap():
			elem = dynamicpb.NewMessage(fld.Message())
		case fld.IsList():
			elem = msg.Get(fld).List().NewElement().Message()
		default:
			elem = msg.NewField(fld).Message()
		}
		err := prototext.UnmarshalOptions{
			Resolver:     &msgLiteralResolver{interp: interp, pkg: fld.ParentFile().Package()},
			AllowPartial: true,
		}.Unmarshal([]byte(opt.GetAggregateValue()), elem.Interface())
		if err != nil {
			return interp.HandleOptionValueErrorf(mc, node, "failed to parse message literal: %w", err)
		}
		if err := interp.checkFieldUsagesInMessage(targetType, mc, elem, node); err != nil {
			return err
		}
		value = protoreflect.ValueOfMessage(elem)
	default:
		v, err := interp.scalarFieldValueFromProto(mc, descriptorpb.FieldDescriptorProto_Type(k), opt, node)
		if err != nil {
			return interp.HandleOptionValueErrorf(mc, node, "%w", err)
		}
		value = protoreflect.ValueOf(v)
	}

	if ood := fld.ContainingOneof(); ood != nil {
		existingFld := msg.WhichOneof(ood)
		if existingFld != nil && existingFld.Number() != fld.Number() {
			return interp.HandleOptionForbiddenErrorf(mc, node, "oneof %q already has field %q set", ood.Name(), fieldName(existingFld))
		}
	}

	switch {
	case fld.IsMap():
		mv := msg.Mutable(fld).Map()
		setMapEntry(fld, msg, mv, value.Message())
	case fld.IsList():
		msg.Mutable(fld).List().Append(value)
	default:
		if msg.Has(fld) {
			// return interp.reporter.HandleErrorf(interp.nodeInfo(name), "%vnon-repeated option field %s already set", mc, fieldName(fld))
			return interp.HandleOptionForbiddenErrorf(mc, node, "non-repeated option field %s already set", fieldName(fld))
		}
		msg.Set(fld, value)
	}
	return nil
}

// checkFieldUsagesInMessage verifies that all fields present in the given
// message can be used for the given target type. When an AST is
// present, we validate each field as it is processed. But without
// an AST, we unmarshal a message from an uninterpreted option's
// aggregate value string, and then must make sure that all fields
// set in that message are valid. This reports an error for each
// invalid field it encounters and returns a non-nil error if/when
// the handler returns a non-nil error.
func (interp *interpreter) checkFieldUsagesInMessage(
	targetType descriptorpb.FieldOptions_OptionTargetType,
	mc *protointernal.MessageContext,
	msg protoreflect.Message,
	node ast.Node,
) error {
	var err error
	msg.Range(func(fld protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		err = interp.checkFieldUsage(targetType, mc, fld, node)
		if err != nil {
			return false
		}
		switch {
		case fld.IsList() && fld.Message() != nil:
			listVal := val.List()
			for i, length := 0, listVal.Len(); i < length; i++ {
				err = interp.checkFieldUsagesInMessage(targetType, mc, listVal.Get(i).Message(), node)
				if err != nil {
					return false
				}
			}
		case fld.IsMap() && fld.MapValue().Message() != nil:
			mapVal := val.Map()
			mapVal.Range(func(_ protoreflect.MapKey, val protoreflect.Value) bool {
				err = interp.checkFieldUsagesInMessage(targetType, mc, val.Message(), node)
				return err == nil
			})
		case !fld.IsMap() && fld.Message() != nil:
			err = interp.checkFieldUsagesInMessage(targetType, mc, val.Message(), node)
		}
		return err == nil
	})
	return err
}

func setMapEntry(
	fld protoreflect.FieldDescriptor,
	msg protoreflect.Message,
	mapVal protoreflect.Map,
	entry protoreflect.Message,
) {
	keyFld, valFld := fld.MapKey(), fld.MapValue()
	key := entry.Get(keyFld)
	val := entry.Get(valFld)
	if fld.MapValue().Kind() == protoreflect.MessageKind {
		// Replace any nil/invalid values with an empty message
		dm, valIsDynamic := val.Interface().(*dynamicpb.Message)
		if (valIsDynamic && dm == nil) || !val.Message().IsValid() {
			val = protoreflect.ValueOfMessage(dynamicpb.NewMessage(valFld.Message()))
		}
		_, containerIsDynamic := msg.Interface().(*dynamicpb.Message)
		if valIsDynamic && !containerIsDynamic {
			// This happens because we create dynamic messages to represent map entries,
			// but the container of the map may expect a non-dynamic, generated type.
			dest := mapVal.NewValue()
			_, destIsDynamic := dest.Message().Interface().(*dynamicpb.Message)
			if !destIsDynamic {
				// reflection Set methods do not support cases where destination is
				// generated but source is dynamic (or vice versa). But proto.Merge
				// *DOES* support that, as long as dest and source use the same
				// descriptor.
				proto.Merge(dest.Message().Interface(), val.Message().Interface())
				val = dest
			}
		}
	}
	// TODO: error if key is already present
	mapVal.Set(key.MapKey(), val)
}

type msgLiteralResolver struct {
	interp *interpreter
	pkg    protoreflect.FullName
}

func (r *msgLiteralResolver) FindMessageByName(message protoreflect.FullName) (protoreflect.MessageType, error) {
	if r.interp.resolver == nil {
		return nil, protoregistry.NotFound
	}
	return r.interp.resolver.FindMessageByName(message)
}

func (r *msgLiteralResolver) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	// In a message literal, we don't allow arbitrary URL prefixes
	pos := strings.LastIndexByte(url, '/')
	var urlPrefix string
	if pos > 0 {
		urlPrefix = url[:pos]
	}
	if urlPrefix != "type.googleapis.com" && urlPrefix != "type.googleprod.com" {
		return nil, fmt.Errorf("could not resolve type reference %s", url)
	}
	return r.FindMessageByName(protoreflect.FullName(url[pos+1:]))
}

func (r *msgLiteralResolver) FindExtensionByName(field protoreflect.FullName) (protoreflect.ExtensionType, error) {
	if r.interp.resolver == nil {
		return nil, protoregistry.NotFound
	}
	// In a message literal, extension name may be partially qualified, relative to package.
	// So we have to search through package scopes.
	pkg := r.pkg
	for {
		// TODO: This does not *fully* implement the insane logic of protoc with regards
		//       to resolving relative references.
		//       https://protobuf.com/docs/language-spec#reference-resolution
		name := pkg.Append(protoreflect.Name(field))
		ext, err := r.interp.resolver.FindExtensionByName(name)
		if err == nil {
			return ext, nil
		}
		if pkg == "" {
			// no more namespaces to check
			return nil, err
		}
		pkg = pkg.Parent()
	}
}

func (r *msgLiteralResolver) FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	if r.interp.resolver == nil {
		return nil, protoregistry.NotFound
	}
	return r.interp.resolver.FindExtensionByNumber(message, field)
}

func fieldName(fld protoreflect.FieldDescriptor) string {
	if fld.IsExtension() {
		return fmt.Sprintf("(%s)", fld.FullName())
	}
	return string(fld.Name())
}

func valueKind(val interface{}) string {
	switch val := val.(type) {
	case ast.Identifier:
		return "identifier"
	case bool:
		return "bool"
	case int64:
		if val < 0 {
			return "negative integer"
		}
		return "integer"
	case uint64:
		return "integer"
	case float64:
		return "double"
	case string, []byte:
		return "string"
	case []*ast.MessageFieldNode:
		return "message"
	case []*ast.ValueNode:
		return "array"
	default:
		return fmt.Sprintf("%T", val)
	}
}

func optionValueKind(opt *descriptorpb.UninterpretedOption) string {
	switch {
	case opt.IdentifierValue != nil:
		return "identifier"
	case opt.PositiveIntValue != nil:
		return "integer"
	case opt.NegativeIntValue != nil:
		return "negative integer"
	case opt.DoubleValue != nil:
		return "double"
	case opt.StringValue != nil:
		return "string"
	case opt.AggregateValue != nil:
		return "message"
	default:
		// should not be possible
		return "<nil>"
	}
}

// fieldValue computes a compile-time value (constant or list or message literal) for the given
// AST node val. The value in val must be assignable to the field fld.
//
// If the returned value is not valid, then an error occurred during processing.
// The returned err may be nil, however, as any errors will already have been
// handled (so the resulting error could be nil if the handler returned nil).
func (interp *interpreter) fieldValue(
	targetType descriptorpb.FieldOptions_OptionTargetType,
	mc *protointernal.MessageContext,
	msg protoreflect.Message,
	fld protoreflect.FieldDescriptor,
	val *ast.ValueNode,
	insideMsgLiteral bool,
	pathPrefix []int32,
) (protoreflect.Value, sourceinfo.OptionSourceInfo, error) {
	k := fld.Kind()
	switch k {
	case protoreflect.EnumKind:
		num, _, err := interp.enumFieldValue(mc, fld.Enum(), val, insideMsgLiteral)
		if err != nil {
			return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, interp.handler.HandleError(err)
		}
		interp.indexEnumValueRef(fld, val)
		return protoreflect.ValueOfEnum(num), newSrcInfo(pathPrefix, nil), nil

	case protoreflect.MessageKind, protoreflect.GroupKind:
		v := val.Value()
		if aggs, ok := v.([]*ast.MessageFieldNode); ok {
			var childMsg protoreflect.Message
			switch {
			case fld.IsList():
				// List of messages
				val := msg.NewField(fld)
				childMsg = val.List().NewElement().Message()
			case fld.IsMap():
				// No generated type for map entries, so we use a dynamic type
				childMsg = dynamicpb.NewMessage(fld.Message())
			default:
				// Normal message field
				childMsg = msg.NewField(fld).Message()
			}
			return interp.messageLiteralValue(targetType, mc, aggs, childMsg, pathPrefix)
		}
		return protoreflect.Value{}, sourceinfo.OptionSourceInfo{},
			interp.HandleOptionValueErrorf(mc, val, "expecting message, got %s", valueKind(v))

	default:
		v, err := interp.scalarFieldValue(mc, descriptorpb.FieldDescriptorProto_Type(k), val, insideMsgLiteral)
		if err != nil {
			return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, interp.handler.HandleError(err)
		}
		return protoreflect.ValueOf(v), newSrcInfo(pathPrefix, nil), nil
	}
}

// enumFieldValue resolves the given AST node val as an enum value descriptor. If the given
// value is not a valid identifier (or number if allowed), an error is returned instead.
func (interp *interpreter) enumFieldValue(
	mc *protointernal.MessageContext,
	ed protoreflect.EnumDescriptor,
	val *ast.ValueNode,
	allowNumber bool,
) (protoreflect.EnumNumber, protoreflect.Name, error) {
	v := val.Value()
	var num protoreflect.EnumNumber
	switch v := v.(type) {
	case ast.Identifier:
		name := protoreflect.Name(v)
		ev := ed.Values().ByName(name)
		if ev == nil {
			return 0, "", interp.HandleOptionValueErrorf(mc, val, "enum %s has no value named %s", ed.FullName(), v)
		}
		return ev.Number(), name, nil
	case int64:
		if !allowNumber {
			return 0, "", interp.HandleOptionValueErrorf(mc, val, "expecting enum name, got %s", valueKind(v))
		}
		if v > math.MaxInt32 || v < math.MinInt32 {
			return 0, "", interp.HandleOptionValueErrorf(mc, val, "value %d is out of range for an enum", v)
		}
		num = protoreflect.EnumNumber(v)
	case uint64:
		if !allowNumber {
			return 0, "", interp.HandleOptionValueErrorf(mc, val, "expecting enum name, got %s", valueKind(v))
		}
		if v > math.MaxInt32 {
			return 0, "", interp.HandleOptionValueErrorf(mc, val, "value %d is out of range for an enum", v)
		}
		num = protoreflect.EnumNumber(v)
	default:
		return 0, "", interp.HandleOptionValueErrorf(mc, val, "expecting enum, got %s", valueKind(v))
	}
	ev := ed.Values().ByNumber(num)
	if ev != nil {
		return num, ev.Name(), nil
	}
	if ed.IsClosed() {
		return num, "", reporter.Errorf(interp.nodeInfo(val), "%vclosed enum %s has no value with number %d", mc, ed.FullName(), num)
	}
	// unknown value, but enum is open, so we allow it and return blank name
	return num, "", nil
}

// enumFieldValueFromProto resolves the given uninterpreted option value as an enum value descriptor.
// If the given value is not a valid identifier, an error is returned instead.
func (interp *interpreter) enumFieldValueFromProto(
	mc *protointernal.MessageContext,
	ed protoreflect.EnumDescriptor,
	opt *descriptorpb.UninterpretedOption,
	node ast.Node,
) (protoreflect.EnumNumber, protoreflect.Name, error) {
	// We don't have to worry about allowing numbers because numbers are never allowed
	// in uninterpreted values; they are only allowed inside aggregate values (i.e.
	// message literals).
	switch {
	case opt.IdentifierValue != nil:
		name := protoreflect.Name(opt.GetIdentifierValue())
		ev := ed.Values().ByName(name)
		if ev == nil {
			return 0, "", interp.HandleOptionValueErrorf(mc, node, "enum %s has no value named %s", ed.FullName(), name)
		}
		return ev.Number(), name, nil
	default:
		return 0, "", interp.HandleOptionValueErrorf(mc, node, "expecting enum, got %s", optionValueKind(opt))
	}
}

func (interp *interpreter) indexEnumValueRef(fld protoreflect.FieldDescriptor, optValNode *ast.ValueNode) {
	enumDesc := fld.Enum()
	switch v := optValNode.Unwrap().(type) {
	case *ast.IdentNode:
		interp.descriptorIndex.EnumValueIdentNodesToEnumValueDescriptors[v] = enumDesc.Values().ByName(protoreflect.Name(v.AsIdentifier()))
	}
}

// scalarFieldValue resolves the given AST node val as a value whose type is assignable to a
// field with the given fldType.
func (interp *interpreter) scalarFieldValue(
	mc *protointernal.MessageContext,
	fldType descriptorpb.FieldDescriptorProto_Type,
	val *ast.ValueNode,
	insideMsgLiteral bool,
) (interface{}, error) {
	v := val.Value()
	switch fldType {
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		if b, ok := v.(bool); ok {
			return b, nil
		}
		if id, ok := v.(ast.Identifier); ok {
			if insideMsgLiteral {
				// inside a message literal, values use the protobuf text format,
				// which is lenient in that it accepts "t" and "f" or "True" and "False"
				switch id {
				case "t", "true", "True":
					return true, nil
				case "f", "false", "False":
					return false, nil
				}
			} else {
				// options with simple scalar values (no message literal) are stricter
				switch id {
				case "true":
					return true, nil
				case "false":
					return false, nil
				}
			}
		}
		return nil, interp.HandleOptionValueErrorf(mc, val, "expecting bool, got %s", valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		if str, ok := v.(string); ok {
			return []byte(str), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, val, "expecting bytes, got %s", valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		if str, ok := v.(string); ok {
			return str, nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, val, "expecting string, got %s", valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_SINT32, descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		if i, ok := v.(int64); ok {
			if i > math.MaxInt32 || i < math.MinInt32 {
				return nil, interp.HandleOptionValueErrorf(mc, val, "value %d is out of range for int32", i)
			}
			return int32(i), nil
		}
		if ui, ok := v.(uint64); ok {
			if ui > math.MaxInt32 {
				return nil, interp.HandleOptionValueErrorf(mc, val, "value %d is out of range for int32", ui)
			}
			return int32(ui), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, val, "expecting int32, got %s", valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32, descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		if i, ok := v.(int64); ok {
			if i > math.MaxUint32 || i < 0 {
				return nil, interp.HandleOptionValueErrorf(mc, val, "value %d is out of range for uint32", i)
			}
			return uint32(i), nil
		}
		if ui, ok := v.(uint64); ok {
			if ui > math.MaxUint32 {
				return nil, interp.HandleOptionValueErrorf(mc, val, "value %d is out of range for uint32", ui)
			}
			return uint32(ui), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, val, "expecting uint32, got %s", valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_SINT64, descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		if i, ok := v.(int64); ok {
			return i, nil
		}
		if ui, ok := v.(uint64); ok {
			if ui > math.MaxInt64 {
				return nil, interp.HandleOptionValueErrorf(mc, val, "value %d is out of range for int64", ui)
			}
			return int64(ui), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, val, "expecting int64, got %s", valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		if i, ok := v.(int64); ok {
			if i < 0 {
				return nil, interp.HandleOptionValueErrorf(mc, val, "value %d is out of range for uint64", i)
			}
			return uint64(i), nil
		}
		if ui, ok := v.(uint64); ok {
			return ui, nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, val, "expecting uint64, got %s", valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		if id, ok := v.(ast.Identifier); ok {
			switch id {
			case "inf", "infinity":
				return math.Inf(1), nil
			case "nan":
				return math.NaN(), nil
			}
		}
		if d, ok := v.(float64); ok {
			return d, nil
		}
		if i, ok := v.(int64); ok {
			return float64(i), nil
		}
		if u, ok := v.(uint64); ok {
			return float64(u), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, val, "expecting double, got %s", valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		if id, ok := v.(ast.Identifier); ok {
			switch id {
			case "inf", "infinity":
				return float32(math.Inf(1)), nil
			case "nan":
				return float32(math.NaN()), nil
			}
		}
		if d, ok := v.(float64); ok {
			return float32(d), nil
		}
		if i, ok := v.(int64); ok {
			return float32(i), nil
		}
		if u, ok := v.(uint64); ok {
			return float32(u), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, val, "expecting float, got %s", valueKind(v))
	default:
		return nil, interp.HandleOptionValueErrorf(mc, val, "unrecognized field type: %s", fldType)
	}
}

// scalarFieldValue resolves the given uninterpreted option value as a value whose type is
// assignable to a field with the given fldType.
func (interp *interpreter) scalarFieldValueFromProto(
	mc *protointernal.MessageContext,
	fldType descriptorpb.FieldDescriptorProto_Type,
	opt *descriptorpb.UninterpretedOption,
	node ast.Node,
) (interface{}, error) {
	switch fldType {
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		if opt.IdentifierValue != nil {
			switch opt.GetIdentifierValue() {
			case "true":
				return true, nil
			case "false":
				return false, nil
			}
		}
		return nil, interp.HandleOptionValueErrorf(mc, node, "expecting bool, got %s", optionValueKind(opt))
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		if opt.StringValue != nil {
			return opt.GetStringValue(), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, node, "expecting bytes, got %s", optionValueKind(opt))
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		if opt.StringValue != nil {
			return string(opt.GetStringValue()), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, node, "expecting string, got %s", optionValueKind(opt))
	case descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_SINT32, descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		if opt.NegativeIntValue != nil {
			i := opt.GetNegativeIntValue()
			if i > math.MaxInt32 || i < math.MinInt32 {
				return nil, interp.HandleOptionValueErrorf(mc, node, "value %d is out of range for int32", i)
			}
			return int32(i), nil
		}
		if opt.PositiveIntValue != nil {
			ui := opt.GetPositiveIntValue()
			if ui > math.MaxInt32 {
				return nil, interp.HandleOptionValueErrorf(mc, node, "value %d is out of range for int32", ui)
			}
			return int32(ui), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, node, "expecting int32, got %s", optionValueKind(opt))
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32, descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		if opt.NegativeIntValue != nil {
			i := opt.GetNegativeIntValue()
			if i > math.MaxUint32 || i < 0 {
				return nil, interp.HandleOptionValueErrorf(mc, node, "value %d is out of range for uint32", i)
			}
			return uint32(i), nil
		}
		if opt.PositiveIntValue != nil {
			ui := opt.GetPositiveIntValue()
			if ui > math.MaxUint32 {
				return nil, interp.HandleOptionValueErrorf(mc, node, "value %d is out of range for uint32", ui)
			}
			return uint32(ui), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, node, "expecting uint32, got %s", optionValueKind(opt))
	case descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_SINT64, descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		if opt.NegativeIntValue != nil {
			return opt.GetNegativeIntValue(), nil
		}
		if opt.PositiveIntValue != nil {
			ui := opt.GetPositiveIntValue()
			if ui > math.MaxInt64 {
				return nil, interp.HandleOptionValueErrorf(mc, node, "value %d is out of range for int64", ui)
			}
			return int64(ui), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, node, "expecting int64, got %s", optionValueKind(opt))
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		if opt.NegativeIntValue != nil {
			i := opt.GetNegativeIntValue()
			if i < 0 {
				return nil, interp.HandleOptionValueErrorf(mc, node, "value %d is out of range for uint64", i)
			}
			// should not be possible since i should always be negative...
			return uint64(i), nil
		}
		if opt.PositiveIntValue != nil {
			return opt.GetPositiveIntValue(), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, node, "expecting uint64, got %s", optionValueKind(opt))
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		if opt.IdentifierValue != nil {
			switch opt.GetIdentifierValue() {
			case "inf", "infinity":
				return math.Inf(1), nil
			case "nan":
				return math.NaN(), nil
			}
		}
		if opt.DoubleValue != nil {
			return opt.GetDoubleValue(), nil
		}
		if opt.NegativeIntValue != nil {
			return float64(opt.GetNegativeIntValue()), nil
		}
		if opt.PositiveIntValue != nil {
			return float64(opt.GetPositiveIntValue()), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, node, "expecting double, got %s", optionValueKind(opt))
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		if opt.IdentifierValue != nil {
			switch opt.GetIdentifierValue() {
			case "inf", "infinity":
				return float32(math.Inf(1)), nil
			case "nan":
				return float32(math.NaN()), nil
			}
		}
		if opt.DoubleValue != nil {
			return float32(opt.GetDoubleValue()), nil
		}
		if opt.NegativeIntValue != nil {
			return float32(opt.GetNegativeIntValue()), nil
		}
		if opt.PositiveIntValue != nil {
			return float32(opt.GetPositiveIntValue()), nil
		}
		return nil, interp.HandleOptionValueErrorf(mc, node, "expecting float, got %s", optionValueKind(opt))
	default:
		return nil, interp.HandleOptionValueErrorf(mc, node, "unrecognized field type: %s", fldType)
	}
}

func descriptorType(m proto.Message) string {
	switch m := m.(type) {
	case *descriptorpb.DescriptorProto:
		return "message"
	case *descriptorpb.DescriptorProto_ExtensionRange:
		return "extension range"
	case *descriptorpb.FieldDescriptorProto:
		if m.GetExtendee() == "" {
			return "field"
		}
		return "extension"
	case *descriptorpb.EnumDescriptorProto:
		return "enum"
	case *descriptorpb.EnumValueDescriptorProto:
		return "enum value"
	case *descriptorpb.ServiceDescriptorProto:
		return "service"
	case *descriptorpb.MethodDescriptorProto:
		return "method"
	case *descriptorpb.FileDescriptorProto:
		return "file"
	default:
		// shouldn't be possible
		return fmt.Sprintf("%T", m)
	}
}

// messageLiteralValue processes a message literal value.
//
// If the returned value is not valid, then an error occurred during processing.
// The returned err may be nil, however, as any errors will already have been
// handled (so the resulting error could be nil if the handler returned nil).
func (interp *interpreter) messageLiteralValue(
	targetType descriptorpb.FieldOptions_OptionTargetType,
	mc *protointernal.MessageContext,
	fieldNodes []*ast.MessageFieldNode,
	msg protoreflect.Message,
	pathPrefix []int32,
) (protoreflect.Value, sourceinfo.OptionSourceInfo, error) {
	fmd := msg.Descriptor()
	origPath := mc.OptAggPath
	defer func() {
		mc.OptAggPath = origPath
	}()
	flds := make(map[*ast.MessageFieldNode]*sourceinfo.OptionSourceInfo, len(fieldNodes))
	var hadError bool
	for _, fieldNode := range fieldNodes {
		if origPath == "" {
			mc.OptAggPath = fieldNode.Name.Value()
		} else {
			mc.OptAggPath = origPath + "." + fieldNode.Name.Value()
		}
		if fieldNode.Name.IsAnyTypeReference() {
			if len(fieldNodes) > 1 {
				err := interp.HandleOptionForbiddenErrorf(mc, fieldNode.Name.UrlPrefix, "any type references cannot be repeated or mixed with other fields")
				if err != nil {
					return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
				}
				hadError = true
			}

			if fmd.FullName() != "google.protobuf.Any" {
				err := interp.HandleOptionForbiddenErrorf(mc, fieldNode.Name.UrlPrefix, "type references are only allowed for google.protobuf.Any, but this type is %s", fmd.FullName())
				if err != nil {
					return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
				}
				hadError = true
				continue
			}
			typeURLDescriptor := fmd.Fields().ByNumber(protointernal.AnyTypeURLTag)
			var err error
			switch {
			case typeURLDescriptor == nil:
				err = fmt.Errorf("message schema is missing type_url field (number %d)", protointernal.AnyTypeURLTag)
			case typeURLDescriptor.IsList():
				err = fmt.Errorf("message schema has type_url field (number %d) that is a list but should be singular", protointernal.AnyTypeURLTag)
			case typeURLDescriptor.Kind() != protoreflect.StringKind:
				err = fmt.Errorf("message schema has type_url field (number %d) that is %s but should be string", protointernal.AnyTypeURLTag, typeURLDescriptor.Kind())
			}
			if err != nil {
				err := interp.HandleOptionValueErrorf(mc, fieldNode.Name, "%w", err)
				if err != nil {
					return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
				}
				hadError = true
				continue
			}
			valueDescriptor := fmd.Fields().ByNumber(protointernal.AnyValueTag)
			switch {
			case valueDescriptor == nil:
				err = fmt.Errorf("message schema is missing value field (number %d)", protointernal.AnyValueTag)
			case valueDescriptor.IsList():
				err = fmt.Errorf("message schema has value field (number %d) that is a list but should be singular", protointernal.AnyValueTag)
			case valueDescriptor.Kind() != protoreflect.BytesKind:
				err = fmt.Errorf("message schema has value field (number %d) that is %s but should be bytes", protointernal.AnyValueTag, valueDescriptor.Kind())
			}
			if err != nil {
				err := interp.HandleOptionValueErrorf(mc, fieldNode.Name, "%w", err)
				if err != nil {
					return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
				}
				hadError = true
				continue
			}

			urlPrefix := fieldNode.Name.UrlPrefix.AsIdentifier()
			msgName := fieldNode.Name.Name.AsIdentifier()
			fullURL := fmt.Sprintf("%s/%s", urlPrefix, msgName)
			// TODO: Support other URLs dynamically -- the caller of protocompile
			// should be able to provide a custom resolver that can resolve type
			// URLs into message descriptors. The default resolver would be
			// implemented as below, only accepting "type.googleapis.com" and
			// "type.googleprod.com" as hosts/prefixes and using the compiled
			// file's transitive closure to find the named message, since that
			// is what protoc does.
			if urlPrefix != "type.googleapis.com" && urlPrefix != "type.googleprod.com" {
				err := interp.HandleOptionNotFoundErrorf(mc, fieldNode.Name.UrlPrefix, "could not resolve type reference %s", fullURL)
				if err != nil {
					return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
				}
				hadError = true
				continue
			}
			anyFields, ok := fieldNode.Val.Value().([]*ast.MessageFieldNode)
			if !ok {
				err := interp.HandleOptionForbiddenErrorf(mc, fieldNode.Val, "type references for google.protobuf.Any must have message literal value")
				if err != nil {
					return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
				}
				hadError = true
				continue
			}

			anyMd := resolveDescriptor[protoreflect.MessageDescriptor](interp.resolver, protoreflect.FullName(msgName))
			if anyMd == nil {
				err := interp.HandleOptionNotFoundErrorf(mc, fieldNode.Name.UrlPrefix, "could not resolve type reference %s", fullURL)
				if err != nil {
					return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
				}
				hadError = true
				continue
			}
			// parse the message value
			msgVal, valueSrcInfo, err := interp.messageLiteralValue(targetType, mc, anyFields, dynamicpb.NewMessage(anyMd), append(pathPrefix, protointernal.AnyValueTag))
			if err != nil {
				return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
			} else if !msgVal.IsValid() {
				hadError = true
				continue
			}

			b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(msgVal.Message().Interface())
			if err != nil {
				err := interp.HandleOptionValueErrorf(mc, fieldNode.Val, "failed to serialize message value: %w", err)
				if err != nil {
					return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
				}
				hadError = true
				continue
			}

			// Success!
			if !hadError {
				interp.descriptorIndex.TypeReferenceURLsToMessageDescriptors[fieldNode.Name] = anyMd
				msg.Set(typeURLDescriptor, protoreflect.ValueOfString(fullURL))
				msg.Set(valueDescriptor, protoreflect.ValueOfBytes(b))
				flds[fieldNode] = &valueSrcInfo
			}
			continue
		}

		// Not expanded Any syntax; handle normal field.
		var ffld protoreflect.FieldDescriptor
		var err error
		if fieldNode.Name.IsExtension() {
			n := interp.file.ResolveMessageLiteralExtensionName(fieldNode.Name.Name)
			if n == "" {
				// this should not be possible!
				n = string(fieldNode.Name.Name.AsIdentifier())
			}
			ffld, err = interp.resolveExtensionType(n)
			if errors.Is(err, protoregistry.NotFound) {
				// may need to qualify with package name
				// (this should not be necessary!)
				pkg := mc.File.FileDescriptorProto().GetPackage()
				if pkg != "" {
					ffld, err = interp.resolveExtensionType(pkg + "." + n)
				}
			}
		} else {
			ffld = fmd.Fields().ByName(protoreflect.Name(fieldNode.Name.Value()))
			if ffld == nil {
				err = protoregistry.NotFound
				// It could be a proto2 group, where the text format refers to the group type
				// name, and the field name is the lower-cased form of that.
				ffld = fmd.Fields().ByName(protoreflect.Name(strings.ToLower(fieldNode.Name.Value())))
				if ffld != nil {
					// In editions, we support using the group type name only for fields that
					// "look like" proto2 groups.
					if protoreflect.Name(fieldNode.Name.Value()) == ffld.Message().Name() && // text format uses type name
						ffld.Message().FullName().Parent() == ffld.FullName().Parent() && // message and field declared in same scope
						ffld.Kind() == protoreflect.GroupKind /* uses delimited encoding */ {
						// This one looks like a proto2 group, so it's a keeper.
						err = nil
					} else {
						// It doesn't look like a proto2 group, so this is not a match.
						ffld = nil
					}
				}
			}
		}
		if errors.Is(err, protoregistry.NotFound) {
			err := interp.HandleOptionNotFoundErrorf(mc, fieldNode.Name, "field %s not found", string(fieldNode.Name.Name.AsIdentifier()))
			if err != nil {
				return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
			}
			hadError = true
			continue
		} else if err != nil {
			err := interp.HandleOptionNotFoundErrorf(mc, fieldNode.Name, "%v", err)
			if err != nil {
				return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
			}
			hadError = true
			continue
		}
		if err := interp.checkFieldUsage(targetType, mc, ffld, fieldNode.Name); err != nil {
			return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
		}
		if fieldNode.Sep == nil && ffld.Message() == nil {
			// If there is no separator, the field type should be a message.
			// Otherwise, it is an error in the text format.
			err := interp.HandleOptionValueErrorf(mc, fieldNode.Val, "unexpected value, expecting ':'")
			if err != nil {
				return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
			}
			hadError = true
			continue
		}
		srcInfo, err := interp.setOptionField(targetType, mc, msg, ffld, fieldNode.Name, fieldNode.Val, true, append(pathPrefix, int32(ffld.Number())))
		if err != nil {
			return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, err
		}
		if srcInfo != nil {
			interp.descriptorIndex.FieldReferenceNodesToFieldDescriptors[fieldNode] = ffld
			flds[fieldNode] = srcInfo
		}
	}
	if hadError {
		return protoreflect.Value{}, sourceinfo.OptionSourceInfo{}, nil
	}
	return protoreflect.ValueOfMessage(msg),
		newSrcInfo(pathPrefix, &sourceinfo.MessageLiteralSourceInfo{Fields: flds}),
		nil
}

func newSrcInfo(path []int32, children sourceinfo.OptionChildrenSourceInfo) sourceinfo.OptionSourceInfo {
	return sourceinfo.OptionSourceInfo{
		Path:     protointernal.ClonePath(path),
		Children: children,
	}
}

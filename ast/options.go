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

package ast

import (
	"fmt"
	"sort"
)

// OptionDeclNode is a placeholder interface for AST nodes that represent
// options. This allows NoSourceNode to be used in place of *OptionNode
// for some usages.
type OptionDeclNode interface {
	Node
	GetName() Node
	GetValue() ValueNode
}

var (
	_ OptionDeclNode = (*OptionNode)(nil)
	_ OptionDeclNode = NoSourceNode{}
)

// OptionNode represents the declaration of a single option for an element.
// It is used both for normal option declarations (start with "option" keyword
// and end with semicolon) and for compact options found in fields, enum values,
// and extension ranges. Example:
//
//	option (custom.option) = "foo";
type OptionNode struct {
	compositeNode
	Keyword   *KeywordNode // absent for compact options
	Name      *OptionNameNode
	Equals    *RuneNode
	Val       ValueNode
	Semicolon *RuneNode // absent for compact options

	incomplete bool
}

func (n *OptionNode) fileElement()    {}
func (n *OptionNode) msgElement()     {}
func (n *OptionNode) oneofElement()   {}
func (n *OptionNode) enumElement()    {}
func (n *OptionNode) serviceElement() {}
func (n *OptionNode) methodElement()  {}

func (m *OptionNode) AddSemicolon(semi *RuneNode) {
	m.Semicolon = semi
	m.children = append(m.children, semi)
}

// NewOptionNode creates a new *OptionNode for a full option declaration (as
// used in files, messages, oneofs, enums, services, and methods). All arguments
// must be non-nil. (Also see NewCompactOptionNode.)
//   - keyword: The token corresponding to the "option" keyword.
//   - name: The token corresponding to the name of the option.
//   - equals: The token corresponding to the "=" rune after the name.
//   - val: The token corresponding to the option value.
func NewOptionNode(keyword *KeywordNode, name *OptionNameNode, equals *RuneNode, val ValueNode) *OptionNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	if name == nil {
		panic("name is nil")
	}
	if equals == nil {
		panic("equals is nil")
	}
	if val == nil {
		panic("val is nil")
	}
	children := []Node{keyword, name, equals, val}
	return &OptionNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
		Name:    name,
		Equals:  equals,
		Val:     val,
	}
}

func NewIncompleteOptionNode(keyword *KeywordNode, name *OptionNameNode, equals *RuneNode, val ValueNode) *OptionNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	children := []Node{keyword}
	if name != nil {
		children = append(children, name)
	}
	if equals != nil {
		children = append(children, equals)
	}
	if val != nil {
		children = append(children, val)
	} else {
		val = NoSourceNode{}
	}
	return &OptionNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword:    keyword,
		Name:       name,
		Equals:     equals,
		Val:        val,
		incomplete: true,
	}
}

// NewCompactOptionNode creates a new *OptionNode for a full compact declaration
// (as used in fields, enum values, and extension ranges). All arguments must be
// non-nil.
//   - name: The token corresponding to the name of the option.
//   - equals: The token corresponding to the "=" rune after the name.
//   - val: The token corresponding to the option value.
func NewCompactOptionNode(name *OptionNameNode, equals *RuneNode, val ValueNode) *OptionNode {
	if name == nil {
		panic("name is nil")
	}
	if equals == nil {
		panic("equals is nil")
	}
	if val == nil {
		panic("val is nil")
	}
	children := []Node{name, equals, val}
	return &OptionNode{
		compositeNode: compositeNode{
			children: children,
		},
		Name:   name,
		Equals: equals,
		Val:    val,
	}
}

func (n *OptionNode) GetName() Node {
	if n.Name == nil {
		return NoSourceNode{}
	}
	return n.Name
}

func (n *OptionNode) GetValue() ValueNode {
	if n.Val == nil {
		return NoSourceNode{}
	}
	return n.Val
}

func (n *OptionNode) IsIncomplete() bool {
	return n.incomplete || n.Name.IsIncomplete()
}

func NewIncompleteCompactOptionNode(name *OptionNameNode, equals *RuneNode, val ValueNode) *OptionNode {
	var children []Node
	if name != nil {
		children = append(children, name)
	}
	if equals != nil {
		children = append(children, equals)
	}
	if val != nil {
		children = append(children, val)
	} else {
		val = NoSourceNode{}
	}

	return &OptionNode{
		compositeNode: compositeNode{
			children: children,
		},
		Name:       name,
		Equals:     equals,
		Val:        val,
		incomplete: true,
	}
}

// OptionNameNode represents an option name or even a traversal through message
// types to name a nested option field. Example:
//
//	(foo.bar).baz.(bob)
type OptionNameNode struct {
	compositeNode
	Parts []*FieldReferenceNode
	// Dots represent the separating '.' characters between name parts. The
	// length of this slice must be exactly len(Parts)-1, each item in Parts
	// having a corresponding item in this slice *except the last* (since a
	// trailing dot is not allowed).
	//
	// These do *not* include dots that are inside of an extension name. For
	// example: (foo.bar).baz.(bob) has three parts:
	//    1. (foo.bar)  - an extension name
	//    2. baz        - a regular field in foo.bar
	//    3. (bob)      - an extension field in baz
	// Note that the dot in foo.bar will thus not be present in Dots but is
	// instead in Parts[0].
	Dots []*RuneNode
}

// NewOptionNameNode creates a new *OptionNameNode. The dots arg must have a
// length that is one less than the length of parts. The parts arg must not be
// empty.
func NewOptionNameNode(parts []*FieldReferenceNode, dots []*RuneNode) *OptionNameNode {
	children := make([]Node, 0, len(parts)+len(dots))
	for _, part := range parts {
		children = append(children, part)
	}
	for _, dot := range dots {
		children = append(children, dot)
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Start() < children[j].Start()
	})
	return &OptionNameNode{
		compositeNode: compositeNode{
			children: children,
		},
		Parts: parts,
		Dots:  dots,
	}
}

func OptionNameNodeFromIdentValue(ident IdentValueNode) *OptionNameNode {
	switch ident := ident.(type) {
	case *IdentNode:
		return NewOptionNameNode([]*FieldReferenceNode{NewFieldReferenceNode(ident)}, nil)
	case *CompoundIdentNode:
		parts := make([]*FieldReferenceNode, len(ident.Components))
		for i, comp := range ident.Components {
			parts[i] = NewFieldReferenceNode(comp)
		}
		return NewOptionNameNode(parts, ident.Dots)
	default:
		panic(fmt.Sprintf("unknown ident type: %T", ident))
	}
}

func (n *OptionNameNode) IsIncomplete() bool {
	for _, part := range n.Parts {
		if part.IsIncomplete() {
			return true
		}
	}
	return false
}

// FieldReferenceNode is a reference to a field name. It can indicate a regular
// field (simple unqualified name), an extension field (possibly-qualified name
// that is enclosed either in brackets or parentheses), or an "any" type
// reference (a type URL in the form "server.host/fully.qualified.Name" that is
// enclosed in brackets).
//
// Extension names are used in options to refer to custom options (which are
// actually extensions), in which case the name is enclosed in parentheses "("
// and ")". They can also be used to refer to extension fields of options.
//
// Extension names are also used in message literals to set extension fields,
// in which case the name is enclosed in square brackets "[" and "]".
//
// "Any" type references can only be used in message literals, and are not
// allowed in option names. They are always enclosed in square brackets. An
// "any" type reference is distinguished from an extension name by the presence
// of a slash, which must be present in an "any" type reference and must be
// absent in an extension name.
//
// Examples:
//
//	foobar
//	(foo.bar)
//	[foo.bar]
//	[type.googleapis.com/foo.bar]
type FieldReferenceNode struct {
	compositeNode
	Open *RuneNode // only present for extension names and "any" type references

	// only present for "any" type references
	URLPrefix IdentValueNode
	Slash     *RuneNode

	Name IdentValueNode

	Close *RuneNode // only present for extension names and "any" type references

	incomplete bool
}

// NewFieldReferenceNode creates a new *FieldReferenceNode for a regular field.
// The name arg must not be nil.
func NewFieldReferenceNode(name *IdentNode) *FieldReferenceNode {
	if name == nil {
		panic("name is nil")
	}
	children := []Node{name}
	return &FieldReferenceNode{
		compositeNode: compositeNode{
			children: children,
		},
		Name: name,
	}
}

// NewExtensionFieldReferenceNode creates a new *FieldReferenceNode for an
// extension field. All args must be non-nil. The openSym and closeSym runes
// should be "(" and ")" or "[" and "]".
func NewExtensionFieldReferenceNode(openSym *RuneNode, name IdentValueNode, closeSym *RuneNode) *FieldReferenceNode {
	if name == nil {
		panic("name is nil")
	}
	if openSym == nil {
		panic("openSym is nil")
	}
	if closeSym == nil {
		panic("closeSym is nil")
	}
	children := []Node{openSym, name, closeSym}
	return &FieldReferenceNode{
		compositeNode: compositeNode{
			children: children,
		},
		Open:  openSym,
		Name:  name,
		Close: closeSym,
	}
}

// NewAnyTypeReferenceNode creates a new *FieldReferenceNode for an "any"
// type reference. All args must be non-nil. The openSym and closeSym runes
// should be "[" and "]". The slashSym run should be "/".
func NewAnyTypeReferenceNode(openSym *RuneNode, urlPrefix IdentValueNode, slashSym *RuneNode, name IdentValueNode, closeSym *RuneNode) *FieldReferenceNode {
	if name == nil {
		panic("name is nil")
	}
	if openSym == nil {
		panic("openSym is nil")
	}
	if closeSym == nil {
		panic("closeSym is nil")
	}
	if urlPrefix == nil {
		panic("urlPrefix is nil")
	}
	if slashSym == nil {
		panic("slashSym is nil")
	}
	children := []Node{openSym, urlPrefix, slashSym, name, closeSym}
	return &FieldReferenceNode{
		compositeNode: compositeNode{
			children: children,
		},
		Open:      openSym,
		URLPrefix: urlPrefix,
		Slash:     slashSym,
		Name:      name,
		Close:     closeSym,
	}
}

func NewIncompleteExtensionFieldReferenceNode(openSym *RuneNode, name IdentValueNode, closeSym *RuneNode) *FieldReferenceNode {
	if openSym == nil {
		panic("openSym is nil")
	}
	var nameToken Token
	children := []Node{openSym}
	if name != nil {
		children = append(children, name)
		nameToken = name.Start()
	} else {
		nameToken = openSym.Token()
	}
	if closeSym != nil {
		children = append(children, closeSym)
	}
	return &FieldReferenceNode{
		compositeNode: compositeNode{
			children: children,
		},
		Open:       openSym,
		Name:       NewIncompleteIdentNode(name, nameToken),
		Close:      closeSym,
		incomplete: true,
	}
}

func NewIncompleteFieldReferenceNode(name *IdentNode) *FieldReferenceNode {
	if name == nil {
		panic("name is nil")
	}
	return &FieldReferenceNode{
		compositeNode: compositeNode{
			children: []Node{name},
		},
		Name: NewIncompleteIdentNode(name, name.Token()),
	}
}

// IsExtension reports if this is an extension name or not (e.g. enclosed in
// punctuation, such as parentheses or brackets).
func (a *FieldReferenceNode) IsExtension() bool {
	return a.Open != nil && a.Slash == nil
}

// IsAnyTypeReference reports if this is an Any type reference.
func (a *FieldReferenceNode) IsAnyTypeReference() bool {
	return a.Slash != nil
}

func (a *FieldReferenceNode) IsIncomplete() bool {
	return a.incomplete
}

func (a *FieldReferenceNode) Value() string {
	if a.Open != nil {
		var closeRune string
		if a.Close != nil {
			// extended syntax rule: account for possible missing close rune
			closeRune = string(a.Close.Rune)
		}
		if a.Slash != nil {
			return string(a.Open.Rune) + string(a.URLPrefix.AsIdentifier()) + string(a.Slash.Rune) + string(a.Name.AsIdentifier()) + closeRune
		}
		return string(a.Open.Rune) + string(a.Name.AsIdentifier()) + closeRune
	}
	return string(a.Name.AsIdentifier())
}

// CompactOptionsNode represents a compact options declaration, as used with
// fields, enum values, and extension ranges. Example:
//
//	[deprecated = true, json_name = "foo_bar"]
type CompactOptionsNode struct {
	compositeNode
	OpenBracket *RuneNode
	Options     []*OptionNode
	// Commas represent the separating ',' characters between options. The
	// length of this slice must be exactly len(Options)-1, with each item
	// in Options having a corresponding item in this slice *except the last*
	// (since a trailing comma is not allowed).
	Commas       []*RuneNode
	CloseBracket *RuneNode
}

// NewCompactOptionsNode creates a *CompactOptionsNode. All args must be
// non-nil. The commas arg must have a length that is one less than the
// length of opts. The opts arg must not be empty.
func NewCompactOptionsNode(openBracket *RuneNode, opts []*OptionNode, commas []*RuneNode, closeBracket *RuneNode) *CompactOptionsNode {
	children := createCommaSeparatedNodes(
		[]Node{openBracket},
		opts,
		commas,
		[]Node{closeBracket},
	)

	return &CompactOptionsNode{
		compositeNode: compositeNode{
			children: children,
		},
		OpenBracket:  openBracket,
		Options:      opts,
		Commas:       commas,
		CloseBracket: closeBracket,
	}
}

func (e *CompactOptionsNode) GetElements() []*OptionNode {
	if e == nil {
		return nil
	}
	return e.Options
}

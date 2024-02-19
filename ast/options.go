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
	"cmp"
	"fmt"
	"slices"
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
	Keyword   *KeywordNode // absent for compact options
	Name      *OptionNameNode
	Equals    *RuneNode
	Val       ValueNode
	Semicolon *RuneNode // for compact options, this is actually a comma
}

func (n *OptionNode) Start() Token { return startToken(n.Keyword, n.Name, n.Equals, n.Val) }
func (n *OptionNode) End() Token   { return endToken(n.Semicolon, n.Val) }

func (n *OptionNode) fileElement()    {}
func (n *OptionNode) msgElement()     {}
func (n *OptionNode) oneofElement()   {}
func (n *OptionNode) enumElement()    {}
func (n *OptionNode) serviceElement() {}
func (n *OptionNode) methodElement()  {}

func (n *OptionNode) GetName() Node {
	return n.Name
}

func (n *OptionNode) GetValue() ValueNode {
	if IsNil(n.Val) {
		return nil
	}
	return n.Val
}

func (n *OptionNode) IsIncomplete() bool {
	return n.Name == nil || n.Name.IsIncomplete() || n.Equals == nil || IsNil(n.Val)
}

// OptionNameNode represents an option name or even a traversal through message
// types to name a nested option field. Example:
//
//	(foo.bar).baz.(bob)
type OptionNameNode struct {
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

func (n *OptionNameNode) Start() Token {
	if len(n.Parts) > 0 {
		if len(n.Dots) > 0 {
			return min(n.Parts[0].Start(), n.Dots[0].Start())
		}
		return n.Parts[0].Start()
	} else if len(n.Dots) > 0 {
		return n.Dots[0].Start()
	}
	return TokenError
}

func (n *OptionNameNode) End() Token {
	if len(n.Parts) > 0 {
		if len(n.Dots) > 0 {
			return max(n.Parts[len(n.Parts)-1].End(), n.Dots[len(n.Dots)-1].End())
		}
		return n.Parts[len(n.Parts)-1].End()
	} else if len(n.Dots) > 0 {
		return n.Dots[len(n.Dots)-1].End()
	}
	return TokenError
}

func (n *OptionNameNode) OrderedNodes() []Node {
	nodes := make([]Node, 0, len(n.Parts)+len(n.Dots))
	for _, comp := range n.Parts {
		nodes = append(nodes, comp)
	}
	for _, dot := range n.Dots {
		nodes = append(nodes, dot)
	}
	slices.SortFunc(nodes, func(i, j Node) int {
		return cmp.Compare(i.Start(), j.Start())
	})
	return nodes
}

func OptionNameNodeFromIdentValue(ident IdentValueNode) *OptionNameNode {
	switch ident := ident.(type) {
	case *IdentNode:
		return &OptionNameNode{
			Parts: []*FieldReferenceNode{{Name: ident}},
		}
	case *CompoundIdentNode:
		parts := make([]*FieldReferenceNode, len(ident.Components))
		for i, comp := range ident.Components {
			parts[i] = &FieldReferenceNode{Name: comp}
		}
		return &OptionNameNode{
			Parts: parts,
			Dots:  ident.Dots,
		}
	default:
		panic(fmt.Sprintf("unknown ident type: %T", ident))
	}
}

func (n *OptionNameNode) IsIncomplete() bool {
	if n == nil {
		return true
	}
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
	Open *RuneNode // only present for extension names and "any" type references

	// only present for "any" type references
	URLPrefix IdentValueNode
	Slash     *RuneNode

	Name IdentValueNode

	Comma     *RuneNode // only present for extension names and "any" type references
	Close     *RuneNode // only present for extension names and "any" type references
	Semicolon *RuneNode // only present for extension names and "any" type references
}

func (a *FieldReferenceNode) Start() Token {
	return startToken(a.Open, a.URLPrefix, a.Slash, a.Name, a.Comma, a.Close, a.Semicolon)
}

func (a *FieldReferenceNode) End() Token {
	return endToken(a.Semicolon, a.Close, a.Comma, a.Name, a.Slash, a.URLPrefix, a.Open)
}

// IsExtension reports if this is an extension name or not (e.g. enclosed in
// punctuation, such as parentheses or brackets).
func (a *FieldReferenceNode) IsExtension() bool {
	return a.Open != nil && a.Slash == nil && !IsNil(a.Name)
}

// IsAnyTypeReference reports if this is an Any type reference.
func (a *FieldReferenceNode) IsAnyTypeReference() bool {
	return !IsNil(a.URLPrefix) && a.Slash != nil && !IsNil(a.Name)
}

func (a *FieldReferenceNode) IsIncomplete() bool {
	switch {
	case a.Open != nil && a.Open.Rune == '(' && (IsNil(a.Name) || a.Close == nil):
		return true
	case a.Open != nil && a.Open.Rune == '[' && (IsNil(a.URLPrefix) || a.Slash == nil || IsNil(a.Name) || a.Close == nil):
		return true
	default:
		return IsNil(a.Name)
	}
}

func (a *FieldReferenceNode) Value() string {
	var name string
	if !IsNil(a.Name) {
		name = string(a.Name.AsIdentifier())
	}
	if a.Open != nil {
		var closeRune string
		if a.Close != nil {
			// extended syntax rule: account for possible missing close rune
			closeRune = string(a.Close.Rune)
		}
		if a.IsAnyTypeReference() {
			return string(a.Open.Rune) + string(a.URLPrefix.AsIdentifier()) + string(a.Slash.Rune) + name + closeRune
		}
		return string(a.Open.Rune) + name + closeRune
	}
	return name
}

// CompactOptionsNode represents a compact options declaration, as used with
// fields, enum values, and extension ranges. Example:
//
//	[deprecated = true, json_name = "foo_bar"]
type CompactOptionsNode struct {
	OpenBracket  *RuneNode
	Options      []*OptionNode
	CloseBracket *RuneNode
	Semicolon    *RuneNode
}

func (e *CompactOptionsNode) Start() Token { return e.OpenBracket.Start() }
func (e *CompactOptionsNode) End() Token   { return endToken(e.Semicolon, e.CloseBracket) }

func (e *CompactOptionsNode) GetElements() []*OptionNode {
	if e == nil {
		return nil
	}
	return e.Options
}

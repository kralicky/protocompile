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

import "reflect"

// Node is the interface implemented by all nodes in the AST. It
// provides information about the span of this AST node in terms
// of location in the source file. It also provides information
// about all prior comments (attached as leading comments) and
// optional subsequent comments (attached as trailing comments).
type Node interface {
	Start() Token
	End() Token
}

// TerminalNodeInterface represents a leaf in the AST. These represent
// the items/lexemes in the protobuf language. Comments and
// whitespace are accumulated by the lexer and associated with
// the following lexed token.
type TerminalNodeInterface interface {
	Node
	Token() Token
}

var (
	_ TerminalNodeInterface = (*StringLiteralNode)(nil)
	_ TerminalNodeInterface = (*UintLiteralNode)(nil)
	_ TerminalNodeInterface = (*FloatLiteralNode)(nil)
	_ TerminalNodeInterface = (*IdentNode)(nil)
	_ TerminalNodeInterface = (*SpecialFloatLiteralNode)(nil)
	_ TerminalNodeInterface = (*KeywordNode)(nil)
	_ TerminalNodeInterface = (*RuneNode)(nil)
)

// TerminalNode contains bookkeeping shared by all TerminalNode
// implementations. It is embedded in all such node types in this
// package. It provides the implementation of the TerminalNodeInterface
// interface.
type TerminalNode Token

func (n TerminalNode) Start() Token {
	return Token(n)
}

func (n TerminalNode) End() Token {
	return Token(n)
}

func (n TerminalNode) Token() Token {
	return Token(n)
}

func IsCompositeNode(n Node) bool {
	_, ok := n.(TerminalNodeInterface)
	return !ok
}

func IsTerminalNode(n Node) bool {
	_, ok := n.(TerminalNodeInterface)
	return ok
}

func IsVirtualNode(n Node) bool {
	rn, ok := n.(*RuneNode)
	return ok && rn.Virtual
}

func IsNil(n Node) bool {
	return n == nil || reflect.ValueOf(n).IsNil()
}

// RuneNode represents a single rune in protobuf source. Runes
// are typically collected into items, but some runes stand on
// their own, such as punctuation/symbols like commas, semicolons,
// equals signs, open and close symbols (braces, brackets, angles,
// and parentheses), and periods/dots.
// TODO: make this more compact; if runes don't have attributed comments
// then we don't need a Token to represent them and only need an offset
// into the file's contents.
type RuneNode struct {
	TerminalNode
	Rune rune

	// Virtual is true if this rune is not actually present in the source file,
	// but is instead injected by the lexer to satisfy certain grammar rules.
	Virtual bool
}

// EmptyDeclNode represents an empty declaration in protobuf source.
// These amount to extra semicolons, with no actual content preceding
// the semicolon.
type EmptyDeclNode struct {
	Semicolon *RuneNode
}

func (e *EmptyDeclNode) Start() Token { return e.Semicolon.Start() }
func (e *EmptyDeclNode) End() Token   { return e.Semicolon.End() }

// NewEmptyDeclNode creates a new *EmptyDeclNode. The one argument must
// be non-nil.
func NewEmptyDeclNode(semicolon *RuneNode) *EmptyDeclNode {
	if semicolon == nil {
		panic("semicolon is nil")
	}
	return &EmptyDeclNode{
		Semicolon: semicolon,
	}
}

func (e *EmptyDeclNode) fileElement()    {}
func (e *EmptyDeclNode) msgElement()     {}
func (e *EmptyDeclNode) extendElement()  {}
func (e *EmptyDeclNode) oneofElement()   {}
func (e *EmptyDeclNode) enumElement()    {}
func (e *EmptyDeclNode) serviceElement() {}
func (e *EmptyDeclNode) methodElement()  {}

type ErrorNode struct {
	E Node
}

func (e *ErrorNode) Start() Token { return e.E.Start() }
func (e *ErrorNode) End() Token   { return e.E.End() }

func NewErrorNode(node Node) *ErrorNode {
	return &ErrorNode{
		E: node,
	}
}

func (e *ErrorNode) fileElement() {}

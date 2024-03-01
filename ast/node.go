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
	"reflect"

	"google.golang.org/protobuf/proto"
)

// Node is the interface implemented by all nodes in the AST. It
// provides information about the span of this AST node in terms
// of location in the source file. It also provides information
// about all prior comments (attached as leading comments) and
// optional subsequent comments (attached as trailing comments).
type Node interface {
	proto.Message

	Start() Token
	End() Token
}

// TerminalNode represents a leaf in the AST. These represent
// the items/lexemes in the protobuf language. Comments and
// whitespace are accumulated by the lexer and associated with
// the following lexed token.
type TerminalNode interface {
	Node
	GetToken() Token
}

type NamedNode interface {
	Node
	GetName() *IdentNode
}

var (
	_ TerminalNode = (*StringLiteralNode)(nil)
	_ TerminalNode = (*UintLiteralNode)(nil)
	_ TerminalNode = (*FloatLiteralNode)(nil)
	_ TerminalNode = (*IdentNode)(nil)
	_ TerminalNode = (*RuneNode)(nil)
)

func (n *StringLiteralNode) Start() Token { return n.GetToken() }
func (n *StringLiteralNode) End() Token   { return n.GetToken() }

func (n *UintLiteralNode) Start() Token { return n.GetToken() }
func (n *UintLiteralNode) End() Token   { return n.GetToken() }

func (n *FloatLiteralNode) Start() Token { return n.GetToken() }
func (n *FloatLiteralNode) End() Token   { return n.GetToken() }

func (n *IdentNode) Start() Token { return n.GetToken() }
func (n *IdentNode) End() Token   { return n.GetToken() }

func (n *SpecialFloatLiteralNode) Start() Token { return n.Keyword.GetToken() }
func (n *SpecialFloatLiteralNode) End() Token   { return n.Keyword.GetToken() }

func (n *RuneNode) Start() Token { return n.GetToken() }
func (n *RuneNode) End() Token   { return n.GetToken() }

func IsCompositeNode(n Node) bool {
	_, ok := n.(TerminalNode)
	return !ok
}

func IsTerminalNode(n Node) bool {
	_, ok := n.(TerminalNode)
	return ok
}

func IsVirtualNode(n Node) bool {
	rn, ok := n.(*RuneNode)
	return ok && rn.Virtual
}

func IsNil(n Node) bool {
	return n == nil || reflect.ValueOf(n).IsNil()
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

func (e *ErrorNode) Start() Token { return e.Err.Start() }
func (e *ErrorNode) End() Token   { return e.Err.End() }

func (e *ErrorNode) fileElement() {}

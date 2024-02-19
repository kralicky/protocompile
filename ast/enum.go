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

// EnumDeclNode is a node in the AST that defines an enum type. This can be
// either an *EnumNode or a NoSourceNode.
type EnumDeclNode interface {
	Node
	GetName() Node
}

var (
	_ EnumDeclNode = (*EnumNode)(nil)
	_ EnumDeclNode = NoSourceNode{}
)

// EnumNode represents an enum declaration. Example:
//
//	enum Foo { BAR = 0; BAZ = 1 }
type EnumNode struct {
	Keyword    *KeywordNode
	Name       *IdentNode
	OpenBrace  *RuneNode
	Decls      []EnumElement
	CloseBrace *RuneNode
	Semicolon  *RuneNode
}

func (e *EnumNode) Start() Token { return e.Keyword.Start() }
func (e *EnumNode) End() Token   { return e.Semicolon.Token() }

func (*EnumNode) fileElement() {}
func (*EnumNode) msgElement()  {}

func (e *EnumNode) GetName() Node {
	return e.Name
}

func (e *EnumNode) GetElements() []EnumElement {
	return e.Decls
}

// EnumElement is an interface implemented by all AST nodes that can
// appear in the body of an enum declaration.
type EnumElement interface {
	Node
	enumElement()
}

var (
	_ EnumElement = (*OptionNode)(nil)
	_ EnumElement = (*EnumValueNode)(nil)
	_ EnumElement = (*ReservedNode)(nil)
	_ EnumElement = (*EmptyDeclNode)(nil)
)

// EnumValueDeclNode is a placeholder interface for AST nodes that represent
// enum values. This allows NoSourceNode to be used in place of *EnumValueNode
// for some usages.
type EnumValueDeclNode interface {
	Node
	GetName() Node
	GetNumber() Node
}

var (
	_ EnumValueDeclNode = (*EnumValueNode)(nil)
	_ EnumValueDeclNode = NoSourceNode{}
)

// EnumValueNode represents an enum declaration. Example:
//
//	UNSET = 0 [deprecated = true];
type EnumValueNode struct {
	Name      *IdentNode
	Equals    *RuneNode
	Number    IntValueNode
	Options   *CompactOptionsNode
	Semicolon *RuneNode
}

func (*EnumValueNode) enumElement() {}

func (e *EnumValueNode) Start() Token { return e.Name.Start() }
func (e *EnumValueNode) End() Token   { return e.Semicolon.Token() }

func (e *EnumValueNode) GetName() Node {
	return e.Name
}

func (e *EnumValueNode) GetNumber() Node {
	if IsNil(e.Number) {
		return nil
	}
	return e.Number
}

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

// MessageDeclNode is a node in the AST that defines a message type. This
// includes normal message fields as well as implicit messages:
//   - *MessageNode
//   - *GroupNode (the group is a field and inline message type)
//   - *MapFieldNode (map fields implicitly define a MapEntry message type)
//
// This also allows NoSourceNode to be used in place of one of the above
// for some usages.
type MessageDeclNode interface {
	Node
	MessageName() Node
}

var (
	_ MessageDeclNode = (*MessageNode)(nil)
	_ MessageDeclNode = (*GroupNode)(nil)
	_ MessageDeclNode = (*MapFieldNode)(nil)
	_ MessageDeclNode = NoSourceNode{}
)

// MessageNode represents a message declaration. Example:
//
//	message Foo {
//	  string name = 1;
//	  repeated string labels = 2;
//	  bytes extra = 3;
//	}
type MessageNode struct {
	Keyword    *KeywordNode
	Name       *IdentNode
	OpenBrace  *RuneNode
	Decls      []MessageElement
	CloseBrace *RuneNode
	Semicolon  *RuneNode
}

func (m *MessageNode) Start() Token { return startToken(m.Keyword) }
func (m *MessageNode) End() Token   { return endToken(m.Semicolon, m.CloseBrace) }

func (*MessageNode) fileElement() {}
func (*MessageNode) msgElement()  {}

func (e *MessageNode) GetElements() []MessageElement {
	return e.Decls
}

func (n *MessageNode) MessageName() Node {
	return n.Name
}

// MessageElement is an interface implemented by all AST nodes that can
// appear in a message body.
type MessageElement interface {
	Node
	msgElement()
}

var (
	_ MessageElement = (*OptionNode)(nil)
	_ MessageElement = (*FieldNode)(nil)
	_ MessageElement = (*MapFieldNode)(nil)
	_ MessageElement = (*OneofNode)(nil)
	_ MessageElement = (*GroupNode)(nil)
	_ MessageElement = (*MessageNode)(nil)
	_ MessageElement = (*EnumNode)(nil)
	_ MessageElement = (*ExtendNode)(nil)
	_ MessageElement = (*ExtensionRangeNode)(nil)
	_ MessageElement = (*ReservedNode)(nil)
	_ MessageElement = (*EmptyDeclNode)(nil)
)

// ExtendNode represents a declaration of extension fields. Example:
//
//	extend google.protobuf.FieldOptions {
//	  bool redacted = 33333;
//	}
type ExtendNode struct {
	Keyword    *KeywordNode
	Extendee   IdentValueNode
	OpenBrace  *RuneNode
	Decls      []ExtendElement
	CloseBrace *RuneNode
	Semicolon  *RuneNode
}

func (e *ExtendNode) Start() Token { return startToken(e.Keyword) }
func (e *ExtendNode) End() Token   { return endToken(e.Semicolon, e.CloseBrace) }

func (e *ExtendNode) GetElements() []ExtendElement {
	return e.Decls
}

func (*ExtendNode) fileElement() {}
func (*ExtendNode) msgElement()  {}

func (e *ExtendNode) IsIncomplete() bool {
	return IsNil(e.Extendee) || e.OpenBrace == nil || e.CloseBrace == nil
}

// ExtendElement is an interface implemented by all AST nodes that can
// appear in the body of an extends declaration.
type ExtendElement interface {
	Node
	extendElement()
}

var (
	_ ExtendElement = (*FieldNode)(nil)
	_ ExtendElement = (*GroupNode)(nil)
	_ ExtendElement = (*EmptyDeclNode)(nil)
)

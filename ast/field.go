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

// FieldDeclNode is a node in the AST that defines a field. This includes
// normal message fields as well as extensions. There are multiple types
// of AST nodes that declare fields:
//   - *FieldNode
//   - *GroupNode
//   - *MapFieldNode
//   - *SyntheticMapField
//
// This also allows NoSourceNode and SyntheticMapField to be used in place of
// one of the above for some usages.
type FieldDeclNode interface {
	Node
	FieldLabel() Node
	FieldName() Node
	FieldType() Node
	FieldTag() Node
	GetGroupKeyword() Node
	GetOptions() *CompactOptionsNode
}

var (
	_ FieldDeclNode = (*FieldNode)(nil)
	_ FieldDeclNode = (*GroupNode)(nil)
	_ FieldDeclNode = (*MapFieldNode)(nil)
	_ FieldDeclNode = (*SyntheticMapField)(nil)
	_ FieldDeclNode = NoSourceNode{}
)

// FieldNode represents a normal field declaration (not groups or maps). It
// can represent extension fields as well as non-extension fields (both inside
// of messages and inside of one-ofs). Example:
//
//	optional string foo = 1;
type FieldNode struct {
	Label     *KeywordNode
	FldType   IdentValueNode
	Name      *IdentNode
	Equals    *RuneNode
	Tag       *UintLiteralNode
	Options   *CompactOptionsNode
	Semicolon *RuneNode
}

func (n *FieldNode) Start() Token {
	return startToken(n.Label, n.FldType, n.Name)
}

func (n *FieldNode) End() Token {
	return endToken(n.Semicolon, n.Options, n.Tag, n.Equals, n.Name, n.FldType, n.Label)
}

func (*FieldNode) msgElement()    {}
func (*FieldNode) oneofElement()  {}
func (*FieldNode) extendElement() {}

func (n *FieldNode) FieldLabel() Node {
	if n.Label == nil {
		return nil
	}
	return n.Label
}

func (n *FieldNode) FieldName() Node {
	if IsNil(n.Name) {
		return nil
	}
	return n.Name
}

func (n *FieldNode) FieldType() Node {
	if IsNil(n.FldType) {
		return nil
	}
	return n.FldType
}

func (n *FieldNode) FieldTag() Node {
	if IsNil(n.Tag) {
		return nil
	}
	return n.Tag
}

func (n *FieldNode) GetGroupKeyword() Node {
	return nil
}

func (n *FieldNode) GetOptions() *CompactOptionsNode {
	return n.Options
}

func (n *FieldNode) IsIncomplete() bool {
	return n.Tag == nil || n.Equals == nil || n.Name == nil
}

// GroupNode represents a group declaration, which doubles as a field and inline
// message declaration. It can represent extension fields as well as
// non-extension fields (both inside of messages and inside of one-ofs).
// Example:
//
//	optional group Key = 4 {
//	  optional uint64 id = 1;
//	  optional string name = 2;
//	}
type GroupNode struct {
	Label      *KeywordNode
	Keyword    *KeywordNode
	Name       *IdentNode
	Equals     *RuneNode
	Tag        *UintLiteralNode
	Options    *CompactOptionsNode
	OpenBrace  *RuneNode
	Decls      []MessageElement
	CloseBrace *RuneNode
	Semicolon  *RuneNode
}

func (n *GroupNode) Start() Token {
	return startToken(n.Label, n.Keyword, n.Name, n.Equals, n.Tag)
}

func (n *GroupNode) End() Token { return endToken(n.Semicolon, n.CloseBrace) }

func (*GroupNode) msgElement()    {}
func (*GroupNode) oneofElement()  {}
func (*GroupNode) extendElement() {}

func (n *GroupNode) FieldLabel() Node {
	if n.Label == nil {
		return nil
	}
	return n.Label
}

func (n *GroupNode) FieldName() Node {
	if IsNil(n.Name) {
		return nil
	}
	return n.Name
}

func (n *GroupNode) FieldType() Node {
	if IsNil(n.Keyword) {
		return nil
	}
	return n.Keyword
}

func (n *GroupNode) FieldTag() Node {
	if IsNil(n.Tag) {
		return nil
	}
	return n.Tag
}

func (n *GroupNode) GetGroupKeyword() Node {
	if IsNil(n.Keyword) {
		return nil
	}
	return n.Keyword
}

func (n *GroupNode) GetOptions() *CompactOptionsNode {
	return n.Options
}

func (n *GroupNode) MessageName() Node {
	if IsNil(n.Name) {
		return nil
	}
	return n.Name
}

// OneofDeclNode is a node in the AST that defines a oneof. There are
// multiple types of AST nodes that declare oneofs:
//   - *OneofNode
//   - *SyntheticOneof
//
// This also allows NoSourceNode to be used in place of one of the above
// for some usages.
type OneofDeclNode interface {
	Node
	OneofName() Node
}

// OneofNode represents a one-of declaration. Example:
//
//	oneof query {
//	  string by_name = 2;
//	  Type by_type = 3;
//	  Address by_address = 4;
//	  Labels by_label = 5;
//	}
type OneofNode struct {
	Keyword    *KeywordNode
	Name       *IdentNode
	OpenBrace  *RuneNode
	Decls      []OneofElement
	CloseBrace *RuneNode
	Semicolon  *RuneNode
}

func (n *OneofNode) Start() Token {
	return startToken(n.Keyword, n.Name, n.OpenBrace)
}

func (n *OneofNode) End() Token {
	return endToken(n.CloseBrace, n.Semicolon)
}

func (n *OneofNode) GetElements() []OneofElement {
	return n.Decls
}

func (*OneofNode) msgElement() {}

func (n *OneofNode) OneofName() Node {
	return n.Name
}

// OneofElement is an interface implemented by all AST nodes that can
// appear in the body of a oneof declaration.
type OneofElement interface {
	Node
	oneofElement()
}

var (
	_ OneofElement = (*OptionNode)(nil)
	_ OneofElement = (*FieldNode)(nil)
	_ OneofElement = (*GroupNode)(nil)
	_ OneofElement = (*EmptyDeclNode)(nil)
)

// SyntheticOneof is not an actual node in the AST but a synthetic node
// that represents the oneof implied by a proto3 optional field.
type SyntheticOneof struct {
	Field *FieldNode
}

var _ Node = (*SyntheticOneof)(nil)

// NewSyntheticOneof creates a new *SyntheticOneof that corresponds to the
// given proto3 optional field.
func NewSyntheticOneof(field *FieldNode) *SyntheticOneof {
	return &SyntheticOneof{Field: field}
}

func (n *SyntheticOneof) Start() Token {
	return n.Field.Start()
}

func (n *SyntheticOneof) End() Token {
	return n.Field.End()
}

func (n *SyntheticOneof) LeadingComments() []Comment {
	return nil
}

func (n *SyntheticOneof) TrailingComments() []Comment {
	return nil
}

func (n *SyntheticOneof) OneofName() Node {
	return n.Field.FieldName()
}

// MapTypeNode represents the type declaration for a map field. It defines
// both the key and value types for the map. Example:
//
//	map<string, Values>
type MapTypeNode struct {
	Keyword    *KeywordNode
	OpenAngle  *RuneNode
	KeyType    *IdentNode
	Comma      *RuneNode
	ValueType  IdentValueNode
	CloseAngle *RuneNode
	Semicolon  *RuneNode
}

func (n *MapTypeNode) Start() Token { return n.Keyword.Token() }
func (n *MapTypeNode) End() Token   { return n.Semicolon.Token() }

// MapFieldNode represents a map field declaration. Example:
//
//	map<string,string> replacements = 3 [deprecated = true];
type MapFieldNode struct {
	MapType   *MapTypeNode
	Name      *IdentNode
	Equals    *RuneNode
	Tag       *UintLiteralNode
	Options   *CompactOptionsNode
	Semicolon *RuneNode
}

func (n *MapFieldNode) Start() Token { return n.MapType.Start() }
func (n *MapFieldNode) End() Token {
	return endToken(n.Semicolon, n.Options, n.Tag, n.Equals, n.Name, n.MapType)
}

func (*MapFieldNode) msgElement() {}

func (n *MapFieldNode) FieldLabel() Node {
	return nil
}

func (n *MapFieldNode) FieldName() Node {
	return n.Name
}

func (n *MapFieldNode) FieldType() Node {
	return n.MapType
}

func (n *MapFieldNode) FieldTag() Node {
	return n.Tag
}

func (n *MapFieldNode) FieldExtendee() Node {
	return nil
}

func (n *MapFieldNode) GetGroupKeyword() Node {
	return nil
}

func (n *MapFieldNode) GetOptions() *CompactOptionsNode {
	return n.Options
}

func (n *MapFieldNode) MessageName() Node {
	return n.Name
}

func (n *MapFieldNode) KeyField() *SyntheticMapField {
	return NewSyntheticMapField(n.MapType.KeyType, 1)
}

func (n *MapFieldNode) ValueField() *SyntheticMapField {
	return NewSyntheticMapField(n.MapType.ValueType, 2)
}

// SyntheticMapField is not an actual node in the AST but a synthetic node
// that implements FieldDeclNode. These are used to represent the implicit
// field declarations of the "key" and "value" fields in a map entry.
type SyntheticMapField struct {
	Ident IdentValueNode
	Tag   *UintLiteralNode
}

// NewSyntheticMapField creates a new *SyntheticMapField for the given
// identifier (either a key or value type in a map declaration) and tag
// number (1 for key, 2 for value).
func NewSyntheticMapField(ident IdentValueNode, tagNum uint64) *SyntheticMapField {
	tag := &UintLiteralNode{
		TerminalNode: ident.Start().AsTerminalNode(),
		Val:          tagNum,
	}
	return &SyntheticMapField{Ident: ident, Tag: tag}
}

func (n *SyntheticMapField) Start() Token {
	return n.Ident.Start()
}

func (n *SyntheticMapField) End() Token {
	return n.Ident.End()
}

func (n *SyntheticMapField) LeadingComments() []Comment {
	return nil
}

func (n *SyntheticMapField) TrailingComments() []Comment {
	return nil
}

func (n *SyntheticMapField) FieldLabel() Node {
	if IsNil(n.Ident) {
		return nil
	}
	return n.Ident
}

func (n *SyntheticMapField) FieldName() Node {
	if IsNil(n.Ident) {
		return nil
	}
	return n.Ident
}

func (n *SyntheticMapField) FieldType() Node {
	if IsNil(n.Ident) {
		return nil
	}
	return n.Ident
}

func (n *SyntheticMapField) FieldTag() Node {
	return n.Tag
}

func (n *SyntheticMapField) FieldExtendee() Node {
	return nil
}

func (n *SyntheticMapField) GetGroupKeyword() Node {
	return nil
}

func (n *SyntheticMapField) GetOptions() *CompactOptionsNode {
	return nil
}

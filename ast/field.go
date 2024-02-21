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

func (f *FieldDeclNode) Start() Token {
	if u := f.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (f *FieldDeclNode) End() Token {
	if u := f.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (f *FieldDeclNode) GetLabel() *IdentNode {
	if u := f.Unwrap(); u != nil {
		return u.GetLabel()
	}
	return nil
}

func (f *FieldDeclNode) GetName() *IdentNode {
	if u := f.Unwrap(); u != nil {
		return u.GetName()
	}
	return nil
}

func (f *FieldDeclNode) GetFieldType() *IdentValueNode {
	if u := f.Unwrap(); u != nil {
		return u.GetFieldType()
	}
	return nil
}

func (f *FieldDeclNode) GetTag() *UintLiteralNode {
	if u := f.Unwrap(); u != nil {
		return u.GetTag()
	}
	return nil
}

func (f *FieldDeclNode) GetGroupKeyword() *IdentNode {
	if u := f.Unwrap(); u != nil {
		return u.GetGroupKeyword()
	}
	return nil
}

func (f *FieldDeclNode) GetOptions() *CompactOptionsNode {
	if u := f.Unwrap(); u != nil {
		return u.GetOptions()
	}
	return nil
}

func (n *FieldNode) Start() Token {
	return startToken(n.Label, n.FieldType, n.Name)
}

func (n *FieldNode) End() Token {
	return n.Semicolon.Token
}

func (n *FieldNode) IsIncomplete() bool {
	return n.Tag == nil || n.Equals == nil || n.Name == nil
}

func (n *FieldNode) GetGroupKeyword() *IdentNode {
	return nil
}

func (n *GroupNode) Start() Token {
	return startToken(n.Label, n.Keyword, n.Name, n.Equals, n.Tag)
}

func (n *GroupNode) End() Token { return n.Semicolon.Token }

func (n *MapFieldNode) GetFieldType() *IdentValueNode {
	return n.GetMapType().GetKeyType().AsIdentValue()
}

func (n *GroupNode) GetFieldType() *IdentValueNode {
	return n.GetKeyword().AsIdentValue()
}

func (n *GroupNode) GetGroupKeyword() *IdentNode {
	return n.GetKeyword()
}

func (n *OneofElement) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *OneofElement) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (n *OneofNode) Start() Token {
	return startToken(n.Keyword, n.Name, n.OpenBrace)
}

func (n *OneofNode) End() Token {
	return endToken(n.CloseBrace, n.Semicolon)
}

func (*OneofNode) msgElement() {}

func (n *MapTypeNode) Start() Token { return n.GetKeyword().GetToken() }
func (n *MapTypeNode) End() Token   { return n.GetSemicolon().GetToken() }

func (n *MapFieldNode) Start() Token { return n.GetMapType().Start() }
func (n *MapFieldNode) End() Token   { return n.Semicolon.Token }

func (*MapFieldNode) msgElement() {}

func (n *MapFieldNode) GetGroupKeyword() *IdentNode {
	return nil
}

func (n *MapFieldNode) GetLabel() *IdentNode {
	return nil
}

func (n *MapFieldNode) KeyField() *SyntheticMapField {
	return &SyntheticMapField{
		Name: &IdentNode{
			Val:   "key",
			Token: n.MapType.KeyType.Token,
		},
		FieldType: n.GetFieldType(),
		Tag: &UintLiteralNode{
			Token: n.GetMapType().GetKeyType().GetToken(),
			Val:   2,
		},
	}
}

func (n *MapFieldNode) ValueField() *SyntheticMapField {
	return &SyntheticMapField{
		Name: &IdentNode{
			Val:   "value",
			Token: n.MapType.ValueType.Start(),
		},
		FieldType: n.GetMapType().GetValueType(),
		Tag: &UintLiteralNode{
			Token: n.GetMapType().GetValueType().Start(),
			Val:   2,
		},
	}
}

func (n *SyntheticMapField) Start() Token {
	return n.Name.Token
}

func (n *SyntheticMapField) End() Token {
	return n.Tag.Token
}

func (n *SyntheticMapField) GetOptions() *CompactOptionsNode {
	return nil
}

func (n *SyntheticMapField) GetGroupKeyword() *IdentNode {
	return nil
}

func (n *SyntheticMapField) GetLabel() *IdentNode {
	return nil
}

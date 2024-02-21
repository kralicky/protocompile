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

func (n *MessageDeclNode) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *MessageDeclNode) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (n *MessageDeclNode) GetName() *IdentNode {
	if u := n.Unwrap(); u != nil {
		return u.GetName()
	}
	return nil
}

func (m *MessageNode) Start() Token { return startToken(m.Keyword) }
func (m *MessageNode) End() Token   { return endToken(m.Semicolon, m.CloseBrace) }

func (*MessageNode) fileElement() {}
func (*MessageNode) msgElement()  {}

func (e *MessageNode) GetElements() []*MessageElement {
	return e.Decls
}

func (n *MessageNode) MessageName() Node {
	return n.Name
}

func (e *ExtendNode) Start() Token { return startToken(e.Keyword) }
func (e *ExtendNode) End() Token   { return endToken(e.Semicolon, e.CloseBrace) }

func (e *ExtendNode) GetElements() []*ExtendElement {
	return e.Decls
}

func (*ExtendNode) fileElement() {}
func (*ExtendNode) msgElement()  {}

func (e *ExtendNode) IsIncomplete() bool {
	return IsNil(e.Extendee) || e.OpenBrace == nil || e.CloseBrace == nil
}

func (n *MessageElement) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *MessageElement) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (n *ExtendElement) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *ExtendElement) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

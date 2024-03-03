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
)

func (n *OptionNode) Start() Token { return startToken(n.Keyword, n.Name, n.Equals, n.Val) }
func (n *OptionNode) End() Token   { return n.Semicolon.GetToken() }
func (n *OptionNode) SourceInfoEnd() Token {
	if n.Keyword != nil {
		return n.End()
	}
	// For source info purposes, the trailing comma is not considered part of the
	// option node.
	return n.Val.End()
}

func (n *OptionNode) fileElement()    {}
func (n *OptionNode) msgElement()     {}
func (n *OptionNode) oneofElement()   {}
func (n *OptionNode) enumElement()    {}
func (n *OptionNode) serviceElement() {}
func (n *OptionNode) methodElement()  {}

func (n *OptionNode) IsIncomplete() bool {
	return n.Name == nil || n.Name.IsIncomplete() || n.Equals == nil || IsNil(n.Val)
}

func (n *OptionNameNode) Start() Token {
	if len(n.Parts) > 0 {
		return n.Parts[0].Start()
	}
	return TokenError
}

func (n *OptionNameNode) End() Token {
	if len(n.Parts) > 0 {
		return n.Parts[len(n.Parts)-1].End()
	}
	return TokenError
}

func OptionNameNodeFromIdentValue(ident *IdentValueNode) *OptionNameNode {
	switch ident := ident.GetVal().(type) {
	case *IdentValueNode_Ident:
		return &OptionNameNode{
			Parts: []*ComplexIdentComponent{ident.Ident.AsComplexIdentComponent()},
		}
	case *IdentValueNode_CompoundIdent:
		return &OptionNameNode{
			Parts: ident.CompoundIdent.GetComponents(),
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
		if frn := part.GetFieldRef(); frn != nil && frn.IsIncomplete() {
			return true
		}
	}
	return false
}

func (a *FieldReferenceNode) Start() Token {
	return startToken(a.Open, a.UrlPrefix, a.Slash, a.Name, a.Comma, a.Close, a.Semicolon)
}

func (a *FieldReferenceNode) End() Token {
	return endToken(a.Semicolon, a.Close, a.Comma, a.Name, a.Slash, a.UrlPrefix, a.Open)
}

// IsExtension reports if this is an extension name or not (e.g. enclosed in
// punctuation, such as parentheses or brackets).
func (a *FieldReferenceNode) IsExtension() bool {
	return a.Open != nil && a.Slash == nil && !IsNil(a.Name)
}

// IsAnyTypeReference reports if this is an Any type reference.
func (a *FieldReferenceNode) IsAnyTypeReference() bool {
	return !IsNil(a.UrlPrefix) && a.Slash != nil && !IsNil(a.Name)
}

func (a *FieldReferenceNode) IsIncomplete() bool {
	switch {
	case a.Open != nil && a.Open.Rune == '(' && (IsNil(a.Name) || a.Close == nil):
		return true
	case a.Open != nil && a.Open.Rune == '[' && (IsNil(a.UrlPrefix) || a.Slash == nil || IsNil(a.Name) || a.Close == nil):
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
			return string(a.Open.Rune) + string(a.UrlPrefix.AsIdentifier()) + string(a.Slash.Rune) + name + closeRune
		}
		return string(a.Open.Rune) + name + closeRune
	}
	return name
}

func (e *CompactOptionsNode) Start() Token { return e.OpenBracket.Start() }
func (e *CompactOptionsNode) End() Token   { return endToken(e.Semicolon, e.CloseBracket) }

func (e *CompactOptionsNode) GetElements() []*OptionNode {
	if e == nil {
		return nil
	}
	return e.Options
}

func (e *OptionNameNode) FilterFieldReferences() []*FieldReferenceNode {
	var fieldRefs []*FieldReferenceNode
	for _, part := range e.Parts {
		if fieldRef := part.GetFieldRef(); fieldRef != nil {
			fieldRefs = append(fieldRefs, fieldRef)
		}
	}
	return fieldRefs
}

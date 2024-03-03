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
	"strings"

	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

// Identifier is a possibly-qualified name. This is used to distinguish
// ValueNode values that are references/identifiers vs. those that are
// string literals.
type Identifier = protoreflect.Name

func (n *IdentNode) Value() interface{} {
	return n.AsIdentifier()
}

func (n *IdentNode) AsIdentifier() Identifier {
	if n == nil {
		return Identifier("")
	}
	return Identifier(n.Val)
}

func (n *CompoundIdentNode) Start() Token {
	if len(n.Components) > 0 {
		return n.Components[0].Start()
	}
	return TokenError
}

func (n *CompoundIdentNode) End() Token {
	if len(n.Components) > 0 {
		return n.Components[len(n.Components)-1].End()
	}
	return TokenError
}

func (n *CompoundIdentNode) Value() interface{} {
	return n.AsIdentifier()
}

func (n *CompoundIdentNode) AsIdentifier() Identifier {
	b := strings.Builder{}

	for _, node := range n.GetComponents() {
		switch node := node.Unwrap().(type) {
		case *IdentNode:
			b.WriteString(node.Val)
		case *RuneNode:
			b.WriteRune(node.Rune)
		}
	}
	return Identifier(b.String())
}

func (n *IdentValueNode) AsIdentifier() Identifier {
	if u := n.Unwrap(); u != nil {
		return u.AsIdentifier()
	}
	return Identifier("")
}

func (n *IdentValueNode) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *IdentValueNode) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (n *IdentNode) ToKeyword() *IdentNode {
	n.IsKeyword = true
	return n
}

func (n *CompoundIdentNode) FilterIdents() []*IdentNode {
	var idents []*IdentNode
	for _, component := range n.Components {
		if ident, ok := component.Val.(*ComplexIdentComponent_Ident); ok {
			idents = append(idents, ident.Ident)
		}
	}
	return idents
}

func (n *CompoundIdentNode) Split() (idents []*IdentNode, dots []*RuneNode) {
	for _, component := range n.Components {
		switch component := component.Val.(type) {
		case *ComplexIdentComponent_Ident:
			idents = append(idents, component.Ident)
		case *ComplexIdentComponent_Dot:
			dots = append(dots, component.Dot)
		}
	}
	return
}

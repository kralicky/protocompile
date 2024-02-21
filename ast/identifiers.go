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
	"slices"
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

func (n *IdentNode) AsIdentValue() *IdentValueNode {
	if n == nil {
		return nil
	}
	return &IdentValueNode{
		Val: &IdentValueNode_Ident{
			Ident: n,
		},
	}
}

func (n *CompoundIdentNode) Start() Token {
	if len(n.GetComponents()) > 0 {
		if len(n.GetDots()) > 0 {
			return min(n.Components[0].Start(), n.Dots[0].Start())
		}
		return n.Components[0].Start()
	} else if len(n.GetDots()) > 0 {
		return n.Dots[0].Start()
	}
	return TokenError
}

func (n *CompoundIdentNode) End() Token {
	if len(n.GetComponents()) > 0 {
		if len(n.GetDots()) > 0 {
			return max(n.Components[len(n.Components)-1].End(), n.Dots[len(n.Dots)-1].End())
		}
		return n.Components[len(n.Components)-1].End()
	} else if len(n.GetDots()) > 0 {
		return n.Dots[len(n.Dots)-1].End()
	}
	return TokenError
}

func (n *CompoundIdentNode) Value() interface{} {
	return n.AsIdentifier()
}

func (n *CompoundIdentNode) AsIdentifier() Identifier {
	b := strings.Builder{}
	nodes := make([]Node, 0, len(n.GetComponents())+len(n.GetDots()))
	for _, comp := range n.GetComponents() {
		nodes = append(nodes, comp)
	}
	for _, dot := range n.GetDots() {
		nodes = append(nodes, dot)
	}
	slices.SortFunc(nodes, func(i, j Node) int {
		return cmp.Compare(i.Start(), j.Start())
	})
	for _, node := range nodes {
		if ident, ok := node.(*IdentNode); ok {
			b.WriteString(ident.Val)
		} else if dot, ok := node.(*RuneNode); ok {
			b.WriteRune(dot.Rune)
		}
	}
	return Identifier(b.String())
}

func (n *CompoundIdentNode) LeadingDot() (dot *RuneNode, ok bool) {
	if len(n.Dots) > 0 {
		if len(n.Components) == 0 {
			return n.Dots[0], true
		}
		if n.Dots[0].GetToken() < n.Components[0].GetToken() {
			return n.Dots[0], true
		}
		return nil, false
	}
	return nil, false
}

func (n *CompoundIdentNode) OrderedNodes() []Node {
	nodes := make([]Node, 0, len(n.GetComponents())+len(n.GetDots()))
	for _, comp := range n.GetComponents() {
		nodes = append(nodes, comp)
	}
	for _, dot := range n.GetDots() {
		nodes = append(nodes, dot)
	}
	slices.SortFunc(nodes, func(i, j Node) int {
		return cmp.Compare(i.Start(), j.Start())
	})
	return nodes
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

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
)

// Identifier is a possibly-qualified name. This is used to distinguish
// ValueNode values that are references/identifiers vs. those that are
// string literals.
type Identifier string

// IdentValueNode is an AST node that represents an identifier.
type IdentValueNode interface {
	ValueNode
	AsIdentifier() Identifier
}

var (
	_ IdentValueNode = (*IdentNode)(nil)
	_ IdentValueNode = (*CompoundIdentNode)(nil)
)

// IdentNode represents a simple, unqualified identifier. These are used to name
// elements declared in a protobuf file or to refer to elements. Example:
//
//	foobar
type IdentNode struct {
	TerminalNode
	Val string
}

func (n *IdentNode) Value() interface{} {
	return n.AsIdentifier()
}

func (n *IdentNode) AsIdentifier() Identifier {
	if n == nil {
		// extended syntax: fields with missing names
		return Identifier("")
	}
	return Identifier(n.Val)
}

// ToKeyword is used to convert identifiers to keywords. Since keywords are not
// reserved in the protobuf language, they are initially lexed as identifiers
// and then converted to keywords based on context.
func (n *IdentNode) ToKeyword() *KeywordNode {
	return (*KeywordNode)(n)
}

// CompoundIdentNode represents a qualified identifier. A qualified identifier
// has at least one dot and possibly multiple identifier names (all separated by
// dots). If the identifier has a leading dot, then it is a *fully* qualified
// identifier. Example:
//
//	.com.foobar.Baz
type CompoundIdentNode struct {
	Components []*IdentNode
	Dots       []*RuneNode
}

func (n *CompoundIdentNode) Start() Token {
	if len(n.Components) > 0 {
		if len(n.Dots) > 0 {
			return min(n.Components[0].Start(), n.Dots[0].Start())
		}
		return n.Components[0].Start()
	} else if len(n.Dots) > 0 {
		return n.Dots[0].Start()
	}
	return TokenError
}

func (n *CompoundIdentNode) End() Token {
	if len(n.Components) > 0 {
		if len(n.Dots) > 0 {
			return max(n.Components[len(n.Components)-1].End(), n.Dots[len(n.Dots)-1].End())
		}
		return n.Components[len(n.Components)-1].End()
	} else if len(n.Dots) > 0 {
		return n.Dots[len(n.Dots)-1].End()
	}
	return TokenError
}

func (n *CompoundIdentNode) Value() interface{} {
	return n.AsIdentifier()
}

func (n *CompoundIdentNode) AsIdentifier() Identifier {
	b := strings.Builder{}
	nodes := make([]Node, 0, len(n.Components)+len(n.Dots))
	for _, comp := range n.Components {
		nodes = append(nodes, comp)
	}
	for _, dot := range n.Dots {
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
		if n.Dots[0].Token() < n.Components[0].Token() {
			return n.Dots[0], true
		}
		return nil, false
	}
	return nil, false
}

func (n *CompoundIdentNode) OrderedNodes() []Node {
	nodes := make([]Node, 0, len(n.Components)+len(n.Dots))
	for _, comp := range n.Components {
		nodes = append(nodes, comp)
	}
	for _, dot := range n.Dots {
		nodes = append(nodes, dot)
	}
	slices.SortFunc(nodes, func(i, j Node) int {
		return cmp.Compare(i.Start(), j.Start())
	})
	return nodes
}

// KeywordNode is an AST node that represents a keyword. Keywords are
// like identifiers, but they have special meaning in particular contexts.
// Example:
//
//	message
type KeywordNode IdentNode

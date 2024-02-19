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

// ExtensionRangeNode represents an extension range declaration in an extendable
// message. Example:
//
//	extensions 100 to max;
type ExtensionRangeNode struct {
	Keyword *KeywordNode
	Ranges  []*RangeNode
	// Commas represent the separating ',' characters between ranges. The
	// length of this slice must be exactly len(Ranges)-1, each item in Ranges
	// having a corresponding item in this slice *except the last* (since a
	// trailing comma is not allowed).
	Commas    []*RuneNode
	Options   *CompactOptionsNode
	Semicolon *RuneNode
}

func (e *ExtensionRangeNode) Start() Token { return e.Keyword.Start() }
func (e *ExtensionRangeNode) End() Token   { return e.Semicolon.End() }

func (e *ExtensionRangeNode) msgElement() {}

// RangeDeclNode is a placeholder interface for AST nodes that represent
// numeric values. This allows NoSourceNode to be used in place of *RangeNode
// for some usages.
type RangeDeclNode interface {
	Node
	RangeStart() Node
	RangeEnd() Node
}

var (
	_ RangeDeclNode = (*RangeNode)(nil)
	_ RangeDeclNode = NoSourceNode{}
)

// RangeNode represents a range expression, used in both extension ranges and
// reserved ranges. Example:
//
//	1000 to max
type RangeNode struct {
	StartVal IntValueNode
	// if To is non-nil, then exactly one of EndVal or Max must also be non-nil
	To *KeywordNode
	// EndVal and Max are mutually exclusive
	EndVal IntValueNode
	Max    *KeywordNode
}

func (n *RangeNode) Start() Token { return n.RangeStart().Start() }
func (n *RangeNode) End() Token   { return n.RangeEnd().End() }

func (n *RangeNode) RangeStart() Node {
	return n.StartVal
}

func (n *RangeNode) RangeEnd() Node {
	if n.Max != nil {
		return n.Max
	}
	if !IsNil(n.EndVal) {
		return n.EndVal
	}
	return n.StartVal
}

func (n *RangeNode) StartValue() any {
	return n.StartVal.Value()
}

func (n *RangeNode) StartValueAsInt32(min, max int32) (int32, bool) {
	return AsInt32(n.StartVal, min, max)
}

func (n *RangeNode) EndValue() any {
	if IsNil(n.EndVal) {
		return nil
	}
	return n.EndVal.Value()
}

func (n *RangeNode) EndValueAsInt32(min, max int32) (int32, bool) {
	if n.Max != nil {
		return max, true
	}
	if IsNil(n.EndVal) {
		return n.StartValueAsInt32(min, max)
	}
	return AsInt32(n.EndVal, min, max)
}

// ReservedNode represents reserved declaration, which can be used to reserve
// either names or numbers. Examples:
//
//	reserved 1, 10-12, 15;
//	reserved "foo", "bar", "baz";
//	reserved foo, bar, baz;
type ReservedNode struct {
	Keyword *KeywordNode
	// If non-empty, this node represents reserved ranges, and Names and Identifiers
	// will be empty.
	Ranges []*RangeNode
	// If non-empty, this node represents reserved names as string literals, and
	// Ranges and Identifiers will be empty. String literals are used for reserved
	// names in proto2 and proto3 syntax.
	Names []StringValueNode
	// If non-empty, this node represents reserved names as identifiers, and Ranges
	// and Names will be empty. Identifiers are used for reserved names in editions.
	Identifiers []*IdentNode
	// Commas represent the separating ',' characters between options. The
	// length of this slice must be exactly len(Ranges)-1 or len(Names)-1, depending
	// on whether this node represents reserved ranges or reserved names. Each item
	// in Ranges or Names has a corresponding item in this slice *except the last*
	// (since a trailing comma is not allowed).
	Commas    []*RuneNode
	Semicolon *RuneNode
}

func (n *ReservedNode) Start() Token { return n.Keyword.Start() }
func (n *ReservedNode) End() Token   { return n.Semicolon.End() }

func (*ReservedNode) msgElement()  {}
func (*ReservedNode) enumElement() {}

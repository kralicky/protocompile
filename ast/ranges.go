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

import "fmt"

// ExtensionRangeNode represents an extension range declaration in an extendable
// message. Example:
//
//	extensions 100 to max;
type ExtensionRangeNode struct {
	compositeNode
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

func (e *ExtensionRangeNode) msgElement() {}

func (m *ExtensionRangeNode) AddSemicolon(semi *RuneNode) {
	m.Semicolon = semi
	m.children = append(m.children, semi)
}

// NewExtensionRangeNode creates a new *ExtensionRangeNode. All args must be
// non-nil except opts, which may be nil.
//   - keyword: The token corresponding to the "extends" keyword.
//   - ranges: One or more range expressions.
//   - commas: Tokens that represent the "," runes that delimit the range expressions.
//     The length of commas must be one less than the length of ranges.
//   - opts: The node corresponding to options that apply to each of the ranges.
func NewExtensionRangeNode(keyword *KeywordNode, ranges []*RangeNode, commas []*RuneNode, opts *CompactOptionsNode) *ExtensionRangeNode {
	if len(ranges) == 0 {
		panic("must have at least one range")
	}
	var trailingNodes []Node
	if opts != nil {
		trailingNodes = []Node{opts}
	}
	children := createCommaSeparatedNodes(
		[]Node{keyword},
		ranges,
		commas,
		trailingNodes,
	)
	return &ExtensionRangeNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
		Ranges:  ranges,
		Commas:  commas,
		Options: opts,
	}
}

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
	compositeNode
	StartVal IntValueNode
	// if To is non-nil, then exactly one of EndVal or Max must also be non-nil
	To *KeywordNode
	// EndVal and Max are mutually exclusive
	EndVal IntValueNode
	Max    *KeywordNode
}

// NewRangeNode creates a new *RangeNode. The start argument must be non-nil.
// The to argument represents the "to" keyword. If present (i.e. if it is non-nil),
// then so must be exactly one of end or max. If max is non-nil, it indicates a
// "100 to max" style range. But if end is non-nil, the end of the range is a
// literal, such as "100 to 200".
func NewRangeNode(start IntValueNode, to *KeywordNode, end IntValueNode, max *KeywordNode) *RangeNode {
	if start == nil {
		panic("start is nil")
	}
	numChildren := 1
	if to != nil {
		if end == nil && max == nil {
			panic("to is not nil, but end and max both are")
		}
		if end != nil && max != nil {
			panic("end and max cannot be both non-nil")
		}
		numChildren = 3
	} else {
		if end != nil {
			panic("to is nil, but end is not")
		}
		if max != nil {
			panic("to is nil, but max is not")
		}
	}
	children := make([]Node, 0, numChildren)
	children = append(children, start)
	if to != nil {
		children = append(children, to)
		if end != nil {
			children = append(children, end)
		} else {
			children = append(children, max)
		}
	}
	return &RangeNode{
		compositeNode: compositeNode{
			children: children,
		},
		StartVal: start,
		To:       to,
		EndVal:   end,
		Max:      max,
	}
}

func (n *RangeNode) RangeStart() Node {
	return n.StartVal
}

func (n *RangeNode) RangeEnd() Node {
	if n.Max != nil {
		return n.Max
	}
	if n.EndVal != nil {
		return n.EndVal
	}
	return n.StartVal
}

func (n *RangeNode) StartValue() interface{} {
	return n.StartVal.Value()
}

func (n *RangeNode) StartValueAsInt32(min, max int32) (int32, bool) {
	return AsInt32(n.StartVal, min, max)
}

func (n *RangeNode) EndValue() interface{} {
	if n.EndVal == nil {
		return nil
	}
	return n.EndVal.Value()
}

func (n *RangeNode) EndValueAsInt32(min, max int32) (int32, bool) {
	if n.Max != nil {
		return max, true
	}
	if n.EndVal == nil {
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
	compositeNode
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

func (*ReservedNode) msgElement()  {}
func (*ReservedNode) enumElement() {}

func (m *ReservedNode) AddSemicolon(semi *RuneNode) {
	m.Semicolon = semi
	m.children = append(m.children, semi)
}

// NewReservedRangesNode creates a new *ReservedNode that represents reserved
// numeric ranges. All args must be non-nil.
//   - keyword: The token corresponding to the "reserved" keyword.
//   - ranges: One or more range expressions.
//   - commas: Tokens that represent the "," runes that delimit the range expressions.
//     The length of commas must be one less than the length of ranges.
func NewReservedRangesNode(keyword *KeywordNode, ranges []*RangeNode, commas []*RuneNode) *ReservedNode {
	if len(ranges) == 0 {
		panic("must have at least one range")
	}
	children := createCommaSeparatedNodes(
		[]Node{keyword},
		ranges,
		commas,
		nil,
	)
	return &ReservedNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
		Ranges:  ranges,
		Commas:  commas,
	}
}

// NewReservedNamesNode creates a new *ReservedNode that represents reserved
// names. All args must be non-nil.
//   - keyword: The token corresponding to the "reserved" keyword.
//   - names: One or more names.
//   - commas: Tokens that represent the "," runes that delimit the names.
//     The length of commas must be one less than the length of names.
//   - semicolon The token corresponding to the ";" rune that ends the declaration.
func NewReservedNamesNode(keyword *KeywordNode, names []StringValueNode, commas []*RuneNode) *ReservedNode {
	if len(names) == 0 {
		panic("must have at least one name")
	}
	children := createCommaSeparatedNodes(
		[]Node{keyword},
		names,
		commas,
		nil,
	)
	return &ReservedNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
		Names:   names,
		Commas:  commas,
	}
}

// NewReservedIdentifiersNode creates a new *ReservedNode that represents reserved
// names. All args must be non-nil.
//   - keyword: The token corresponding to the "reserved" keyword.
//   - names: One or more names.
//   - commas: Tokens that represent the "," runes that delimit the names.
//     The length of commas must be one less than the length of names.
//   - semicolon The token corresponding to the ";" rune that ends the declaration.
func NewReservedIdentifiersNode(keyword *KeywordNode, names []*IdentNode, commas []*RuneNode) *ReservedNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	if len(names) == 0 {
		panic("must have at least one name")
	}
	if len(commas) != len(names)-1 {
		panic(fmt.Sprintf("%d names requires %d commas, not %d", len(names), len(names)-1, len(commas)))
	}
	children := make([]Node, 0, len(names)*2)
	children = append(children, keyword)
	for i, name := range names {
		if i > 0 {
			if commas[i-1] == nil {
				panic(fmt.Sprintf("commas[%d] is nil", i-1))
			}
			children = append(children, commas[i-1])
		}
		if name == nil {
			panic(fmt.Sprintf("names[%d] is nil", i))
		}
		children = append(children, name)
	}
	return &ReservedNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword:     keyword,
		Identifiers: names,
		Commas:      commas,
	}
}

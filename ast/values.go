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
	"math"
	"strings"
)

// ValueNode is an AST node that represents a literal value.
//
// It also includes references (e.g. IdentifierValueNode), which can be
// used as values in some contexts, such as describing the default value
// for a field, which can refer to an enum value.
//
// This also allows NoSourceNode to be used in place of a real value node
// for some usages.
type ValueNode interface {
	Node
	// Value returns a Go representation of the value. For scalars, this
	// will be a string, int64, uint64, float64, or bool. This could also
	// be an Identifier (e.g. IdentValueNodes). It can also be a composite
	// literal:
	//   * For array literals, the type returned will be []ValueNode
	//   * For message literals, the type returned will be []*MessageFieldNode
	//
	// If the ValueNode is a NoSourceNode, indicating that there is no actual
	// source code (and thus not AST information), then this method always
	// returns nil.
	Value() interface{}
}

var (
	_ ValueNode = (*IdentNode)(nil)
	_ ValueNode = (*CompoundIdentNode)(nil)
	_ ValueNode = (*StringLiteralNode)(nil)
	_ ValueNode = (*CompoundStringLiteralNode)(nil)
	_ ValueNode = (*UintLiteralNode)(nil)
	_ ValueNode = (*NegativeIntLiteralNode)(nil)
	_ ValueNode = (*FloatLiteralNode)(nil)
	_ ValueNode = (*SpecialFloatLiteralNode)(nil)
	_ ValueNode = (*SignedFloatLiteralNode)(nil)
	_ ValueNode = (*ArrayLiteralNode)(nil)
	_ ValueNode = (*MessageLiteralNode)(nil)
	_ ValueNode = NoSourceNode{}
)

// StringValueNode is an AST node that represents a string literal.
// Such a node can be a single literal (*StringLiteralNode) or a
// concatenation of multiple literals (*CompoundStringLiteralNode).
type StringValueNode interface {
	ValueNode
	AsString() string
}

var (
	_ StringValueNode = (*StringLiteralNode)(nil)
	_ StringValueNode = (*CompoundStringLiteralNode)(nil)
)

// StringLiteralNode represents a simple string literal. Example:
//
//	"proto2"
type StringLiteralNode struct {
	TerminalNode
	// Val is the actual string value that the literal indicates.
	Val string
}

func (n *StringLiteralNode) Value() interface{} {
	return n.AsString()
}

func (n *StringLiteralNode) AsString() string {
	return n.Val
}

// CompoundStringLiteralNode represents a compound string literal, which is
// the concatenaton of adjacent string literals. Example:
//
//	"this "  "is"   " all one "   "string"
type CompoundStringLiteralNode struct {
	Elements []StringValueNode
}

func (n *CompoundStringLiteralNode) Start() Token {
	if len(n.Elements) == 0 {
		return TokenError
	}
	return n.Elements[0].Start()
}

func (n *CompoundStringLiteralNode) End() Token {
	if len(n.Elements) == 0 {
		return TokenError
	}
	return n.Elements[len(n.Elements)-1].End()
}

func (n *CompoundStringLiteralNode) Value() interface{} {
	return n.AsString()
}

func (n *CompoundStringLiteralNode) AsString() string {
	var sb strings.Builder
	for _, elem := range n.Elements {
		sb.WriteString(elem.AsString())
	}
	return sb.String()
}

// IntValueNode is an AST node that represents an integer literal. If
// an integer literal is too large for an int64 (or uint64 for
// positive literals), it is represented instead by a FloatValueNode.
type IntValueNode interface {
	ValueNode
	AsInt64() (int64, bool)
	AsUint64() (uint64, bool)
}

// AsInt32 range checks the given int value and returns its value is
// in the range or 0, false if it is outside the range.
func AsInt32(n IntValueNode, min, max int32) (int32, bool) {
	i, ok := n.AsInt64()
	if !ok {
		return 0, false
	}
	if i < int64(min) || i > int64(max) {
		return 0, false
	}
	return int32(i), true
}

var (
	_ IntValueNode = (*UintLiteralNode)(nil)
	_ IntValueNode = (*NegativeIntLiteralNode)(nil)
)

// UintLiteralNode represents a simple integer literal with no sign character.
type UintLiteralNode struct {
	TerminalNode
	// Val is the numeric value indicated by the literal
	Val uint64
	// Raw is the original string representation of the literal
	Raw string
}

func (n *UintLiteralNode) Value() interface{} {
	return n.Val
}

func (n *UintLiteralNode) AsInt64() (int64, bool) {
	if n.Val > math.MaxInt64 {
		return 0, false
	}
	return int64(n.Val), true
}

func (n *UintLiteralNode) AsUint64() (uint64, bool) {
	return n.Val, true
}

func (n *UintLiteralNode) AsFloat() float64 {
	return float64(n.Val)
}

// NegativeIntLiteralNode represents an integer literal with a negative (-) sign.
type NegativeIntLiteralNode struct {
	Minus *RuneNode
	Uint  *UintLiteralNode
}

func (n *NegativeIntLiteralNode) Start() Token {
	return n.Minus.Token()
}

func (n *NegativeIntLiteralNode) End() Token {
	return n.Uint.End()
}

func (n *NegativeIntLiteralNode) Value() interface{} {
	return -int64(n.Uint.Val)
}

func (n *NegativeIntLiteralNode) AsInt64() (int64, bool) {
	return -int64(n.Uint.Val), true
}

func (n *NegativeIntLiteralNode) AsUint64() (uint64, bool) {
	i64, _ := n.AsInt64()
	if i64 < 0 {
		return 0, false
	}
	return uint64(i64), true
}

// FloatValueNode is an AST node that represents a numeric literal with
// a floating point, in scientific notation, or too large to fit in an
// int64 or uint64.
type FloatValueNode interface {
	ValueNode
	AsFloat() float64
}

var (
	_ FloatValueNode = (*FloatLiteralNode)(nil)
	_ FloatValueNode = (*SpecialFloatLiteralNode)(nil)
	_ FloatValueNode = (*UintLiteralNode)(nil)
)

// FloatLiteralNode represents a floating point numeric literal.
type FloatLiteralNode struct {
	TerminalNode
	// Val is the numeric value indicated by the literal
	Val float64
	// Raw is the original string representation of the literal
	Raw string
}

func (n *FloatLiteralNode) Value() interface{} {
	return n.AsFloat()
}

func (n *FloatLiteralNode) AsFloat() float64 {
	return n.Val
}

// SpecialFloatLiteralNode represents a special floating point numeric literal
// for "inf" and "nan" values.
type SpecialFloatLiteralNode struct {
	*KeywordNode
	Val float64
}

// NewSpecialFloatLiteralNode returns a new *SpecialFloatLiteralNode for the
// given keyword. The given keyword should be "inf", "infinity", or "nan"
// in any case.
func NewSpecialFloatLiteralNode(name *KeywordNode) *SpecialFloatLiteralNode {
	var f float64
	switch strings.ToLower(name.Val) {
	case "inf", "infinity":
		f = math.Inf(1)
	case "nan":
		f = math.NaN()
	default:
		panic(fmt.Sprintf("invalid special float literal: %q", name.Val))
	}
	return &SpecialFloatLiteralNode{
		KeywordNode: name,
		Val:         f,
	}
}

func (n *SpecialFloatLiteralNode) Value() interface{} {
	return n.AsFloat()
}

func (n *SpecialFloatLiteralNode) AsFloat() float64 {
	return n.Val
}

// SignedFloatLiteralNode represents a signed floating point number.
type SignedFloatLiteralNode struct {
	Sign  *RuneNode
	Float FloatValueNode
}

func (n *SignedFloatLiteralNode) Start() Token {
	return startToken(n.Sign, n.Float)
}

func (n *SignedFloatLiteralNode) End() Token {
	return endToken(n.Float, n.Sign)
}

func (n *SignedFloatLiteralNode) Value() interface{} {
	return n.AsFloat()
}

func (n *SignedFloatLiteralNode) AsFloat() float64 {
	val := n.Float.AsFloat()
	if n.Sign != nil {
		if n.Sign.Rune == '-' {
			val = -val
		}
	}
	return val
}

// ArrayLiteralNode represents an array literal, which is only allowed inside of
// a MessageLiteralNode, to indicate values for a repeated field. Example:
//
//	["foo", "bar", "baz"]
type ArrayLiteralNode struct {
	OpenBracket  *RuneNode
	Elements     []ValueNode
	Commas       []*RuneNode
	CloseBracket *RuneNode
	Semicolon    *RuneNode
}

func (n *ArrayLiteralNode) Start() Token {
	return n.OpenBracket.Token()
}

func (n *ArrayLiteralNode) End() Token {
	return endToken(n.Semicolon, n.CloseBracket)
}

func (n *ArrayLiteralNode) Value() interface{} {
	return n.Elements
}

// MessageLiteralNode represents a message literal, which is compatible with the
// protobuf text format and can be used for custom options with message types.
// Example:
//
//	{ foo:1 foo:2 foo:3 bar:<name:"abc" id:123> }
type MessageLiteralNode struct {
	Open     *RuneNode // should be '{' or '<'
	Elements []*MessageFieldNode
	// Separator characters between elements, which can be either ','
	// or ';' if present. This slice must be exactly len(Elements) in
	// length, with each item in Elements having one corresponding item
	// in Seps. Separators in message literals are optional, so a given
	// item in this slice may be nil to indicate absence of a separator.
	Seps      []*RuneNode
	Close     *RuneNode // should be '}' or '>', depending on Open
	Semicolon *RuneNode
}

func (n *MessageLiteralNode) Start() Token {
	return n.Open.Token()
}

func (n *MessageLiteralNode) End() Token {
	return endToken(n.Semicolon, n.Close)
}

func (n *MessageLiteralNode) GetElements() []*MessageFieldNode {
	return n.Elements
}

func (n *MessageLiteralNode) Value() interface{} {
	return n.Elements
}

// MessageFieldNode represents a single field (name and value) inside of a
// message literal. Example:
//
//	foo:"bar"
type MessageFieldNode struct {
	Name *FieldReferenceNode
	// Sep represents the ':' separator between the name and value. If
	// the value is a message or list literal (and thus starts with '<',
	// '{', or '['), then the separator may be omitted and this field may
	// be nil.
	Sep       *RuneNode
	Val       ValueNode
	Semicolon *RuneNode
}

func (n *MessageFieldNode) Start() Token {
	return n.Name.Start()
}

func (n *MessageFieldNode) End() Token {
	return endToken(n.Semicolon, n.Val)
}

func (n *MessageFieldNode) IsIncomplete() bool {
	if IsNil(n.Val) {
		return true
	}
	if n.Sep == nil {
		switch n.Val.(type) {
		case *MessageLiteralNode, *ArrayLiteralNode:
			return false
		default:
			return true
		}
	}
	return false
}

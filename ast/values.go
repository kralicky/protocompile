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

// Value returns a Go representation of the value. For scalars, this
// will be a string, int64, uint64, float64, or bool. This could also
// be an Identifier (e.g. IdentValueNodes). It can also be a composite
// literal:
//   - For array literals, the type returned will be []ValueNode
//   - For message literals, the type returned will be []*MessageFieldNode
//
// If the ValueNode is a NoSourceNode, indicating that there is no actual
// source code (and thus not AST information), then this method always
// returns nil.
func (v *ValueNode) Value() any {
	if u := v.Unwrap(); u != nil {
		return u.Value()
	}
	return nil
}

func (v *ValueNode) Start() Token {
	if u := v.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (v *ValueNode) End() Token {
	if u := v.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (s *StringValueNode) AsString() string {
	if u := s.Unwrap(); u != nil {
		return u.AsString()
	}
	return ""
}

func (s *StringValueNode) Start() Token {
	if u := s.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (s *StringValueNode) End() Token {
	if u := s.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (n *StringLiteralNode) Value() interface{} {
	return n.AsString()
}

func (n *StringLiteralNode) AsString() string {
	return n.Val
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

func (n *IntValueNode) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *IntValueNode) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (n *IntValueNode) AsInt64() (int64, bool) {
	if u := n.Unwrap(); u != nil {
		return u.AsInt64()
	}
	return 0, false
}

func (n *IntValueNode) AsUint64() (uint64, bool) {
	if u := n.Unwrap(); u != nil {
		return u.AsUint64()
	}
	return 0, false
}

func (n *IntValueNode) Value() any {
	if u := n.Unwrap(); u != nil {
		return u.Value()
	}
	return nil
}

// AsInt32 range checks the given int value and returns its value is
// in the range or 0, false if it is outside the range.
func AsInt32[T interface{ AsInt64() (int64, bool) }](n T, min, max int32) (int32, bool) {
	i, ok := n.AsInt64()
	if !ok {
		return 0, false
	}
	if i < int64(min) || i > int64(max) {
		return 0, false
	}
	return int32(i), true
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

func (n *NegativeIntLiteralNode) Start() Token {
	return n.Minus.GetToken()
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

func (n *FloatValueNode) AsFloat() float64 {
	if u := n.Unwrap(); u != nil {
		return u.AsFloat()
	}
	return 0
}

func (n *FloatValueNode) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *FloatValueNode) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (n *FloatLiteralNode) Value() interface{} {
	return n.AsFloat()
}

func (n *FloatLiteralNode) AsFloat() float64 {
	return n.Val
}

// NewSpecialFloatLiteralNode returns a new *SpecialFloatLiteralNode for the
// given keyword. The given keyword should be "inf", "infinity", or "nan"
// in any case.
func NewSpecialFloatLiteralNode(name *IdentNode) *SpecialFloatLiteralNode {
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
		Keyword: name,
		Val:     f,
	}
}

func (n *SpecialFloatLiteralNode) Value() interface{} {
	return n.AsFloat()
}

func (n *SpecialFloatLiteralNode) AsFloat() float64 {
	return n.Val
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

func (n *ArrayLiteralNode) Start() Token {
	return n.OpenBracket.GetToken()
}

func (n *ArrayLiteralNode) End() Token {
	return endToken(n.Semicolon, n.CloseBracket)
}

func (n *ArrayLiteralNode) Value() interface{} {
	return n.Elements
}

func (n *ArrayLiteralNode) FilterValues() []*ValueNode {
	var s []*ValueNode
	for _, r := range n.GetElements() {
		if v := r.GetValue(); v != nil {
			s = append(s, v)
		}
	}
	return s
}

func (n *ArrayLiteralNode) Split() ([]*ValueNode, []*RuneNode) {
	var values []*ValueNode
	var commas []*RuneNode
	for _, elem := range n.Elements {
		switch elem := elem.Val.(type) {
		case *ArrayLiteralElement_Value:
			values = append(values, elem.Value)
		case *ArrayLiteralElement_Comma:
			commas = append(commas, elem.Comma)
		}
	}
	return values, commas
}

func (n *MessageLiteralNode) Start() Token {
	return n.Open.GetToken()
}

func (n *MessageLiteralNode) End() Token {
	return endToken(n.Semicolon, n.Close)
}

func (n *MessageLiteralNode) Value() interface{} {
	return n.Elements
}

func (n *MessageFieldNode) Start() Token {
	return n.Name.Start()
}

func (n *MessageFieldNode) End() Token {
	return endToken(n.Semicolon, n.Val)
}

func (n *MessageFieldNode) IsIncomplete() bool {
	if n.Val == nil {
		return true
	}
	if n.Sep == nil {
		switch n.Val.GetVal().(type) {
		case *ValueNode_MessageLiteral, *ValueNode_ArrayLiteral:
			return false
		default:
			return true
		}
	}
	return false
}

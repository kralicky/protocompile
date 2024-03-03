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

func (e *ExtensionRangeNode) Start() Token { return e.Keyword.Start() }
func (e *ExtensionRangeNode) End() Token   { return e.Semicolon.End() }

func (e *ExtensionRangeNode) msgElement() {}

func (n *RangeNode) Start() Token { return n.RangeStart().Start() }
func (n *RangeNode) End() Token   { return n.RangeEnd().End() }

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

func (n *RangeNode) StartValueAsInt32(min, max int32) (int32, bool) {
	return AsInt32(n.StartVal, min, max)
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

func (n *RangeNode) StartValue() any {
	return n.StartVal.Value()
}

func (n *RangeNode) EndValue() any {
	return n.EndVal.Value()
}

func (n *ReservedNode) Start() Token { return n.Keyword.Start() }
func (n *ReservedNode) End() Token   { return n.Semicolon.End() }

func (*ReservedNode) msgElement()  {}
func (*ReservedNode) enumElement() {}

func (r *ReservedNode) FilterNames() []*StringValueNode {
	s := make([]*StringValueNode, 0, len(r.Elements))
	for _, e := range r.Elements {
		if n := e.GetName(); n != nil {
			s = append(s, n)
		}
	}
	return s
}

func (r *ReservedNode) FilterRanges() []*RangeNode {
	s := make([]*RangeNode, 0, len(r.Elements))
	for _, e := range r.Elements {
		if n := e.GetRange(); n != nil {
			s = append(s, n)
		}
	}
	return s
}

func (r *ReservedNode) FilterIdentifiers() []*IdentNode {
	s := make([]*IdentNode, 0, len(r.Elements))
	for _, e := range r.Elements {
		if n := e.GetIdentifier(); n != nil {
			s = append(s, n)
		}
	}
	return s
}

func (r *ExtensionRangeNode) FilterRanges() []*RangeNode {
	s := make([]*RangeNode, 0, len(r.Elements))
	for _, e := range r.Elements {
		if n := e.GetRange(); n != nil {
			s = append(s, n)
		}
	}
	return s
}

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

func (e *EnumNode) Start() Token { return e.Keyword.Start() }
func (e *EnumNode) End() Token   { return e.Semicolon.GetToken() }

func (*EnumNode) fileElement() {}
func (*EnumNode) msgElement()  {}

func (*EnumValueNode) enumElement() {}

func (e *EnumValueNode) Start() Token { return e.Name.GetToken() }
func (e *EnumValueNode) End() Token   { return e.Semicolon.GetToken() }

func (n *EnumElement) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *EnumElement) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

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

func (s *ServiceNode) Start() Token { return s.Keyword.Start() }
func (s *ServiceNode) End() Token   { return endToken(s.Semicolon, s.CloseBrace) }

func (*ServiceNode) fileElement() {}

func (n *RPCNode) Start() Token { return n.Keyword.Start() }
func (n *RPCNode) End() Token   { return n.Semicolon.GetToken() }

func (n *RPCNode) serviceElement() {}

func (n *RPCNode) IsIncomplete() bool {
	return n.Input.IsIncomplete() || n.Output.IsIncomplete()
}

func (n *RPCTypeNode) Start() Token { return n.OpenParen.GetToken() }
func (n *RPCTypeNode) End() Token   { return n.CloseParen.GetToken() }

func (n *RPCTypeNode) IsIncomplete() bool {
	return n.MessageType == nil
}

func (n *RPCElement) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *RPCElement) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (n *ServiceElement) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *ServiceElement) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

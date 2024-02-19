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

// ServiceDeclNode is a node in the AST that defines a service type. This
// can be either a *ServiceNode or a NoSourceNode.
type ServiceDeclNode interface {
	Node
	GetName() Node
}

var (
	_ ServiceDeclNode = (*ServiceNode)(nil)
	_ ServiceDeclNode = NoSourceNode{}
)

// ServiceNode represents a service declaration. Example:
//
//	service Foo {
//	  rpc Bar (Baz) returns (Bob);
//	  rpc Frobnitz (stream Parts) returns (Gyzmeaux);
//	}
type ServiceNode struct {
	Keyword    *KeywordNode
	Name       *IdentNode
	OpenBrace  *RuneNode
	Decls      []ServiceElement
	CloseBrace *RuneNode
	Semicolon  *RuneNode
}

func (s *ServiceNode) Start() Token { return s.Keyword.Start() }
func (s *ServiceNode) End() Token   { return endToken(s.Semicolon, s.CloseBrace) }

func (*ServiceNode) fileElement() {}

func (s *ServiceNode) GetName() Node {
	return s.Name
}

// ServiceElement is an interface implemented by all AST nodes that can
// appear in the body of a service declaration.
type ServiceElement interface {
	Node
	serviceElement()
}

var (
	_ ServiceElement = (*OptionNode)(nil)
	_ ServiceElement = (*RPCNode)(nil)
	_ ServiceElement = (*EmptyDeclNode)(nil)
)

// RPCDeclNode is a placeholder interface for AST nodes that represent RPC
// declarations. This allows NoSourceNode to be used in place of *RPCNode
// for some usages.
type RPCDeclNode interface {
	Node
	GetName() Node
	GetInputType() Node
	GetOutputType() Node
}

var (
	_ RPCDeclNode = (*RPCNode)(nil)
	_ RPCDeclNode = NoSourceNode{}
)

// RPCNode represents an RPC declaration. Example:
//
//	rpc Foo (Bar) returns (Baz);
type RPCNode struct {
	Keyword    *KeywordNode
	Name       *IdentNode
	Input      *RPCTypeNode
	Returns    *KeywordNode
	Output     *RPCTypeNode
	OpenBrace  *RuneNode
	Decls      []RPCElement
	CloseBrace *RuneNode
	Semicolon  *RuneNode
}

func (n *RPCNode) Start() Token { return n.Keyword.Start() }
func (n *RPCNode) End() Token   { return n.Semicolon.Token() }

func (n *RPCNode) serviceElement() {}

func (n *RPCNode) GetName() Node {
	return n.Name
}

func (n *RPCNode) GetInputType() Node {
	if n.Input.MessageType == nil {
		return NoSourceNode{}
	}
	return n.Input.MessageType
}

func (n *RPCNode) GetOutputType() Node {
	if n.Output.MessageType == nil {
		return NoSourceNode{}
	}
	return n.Output.MessageType
}

func (n *RPCNode) IsIncomplete() bool {
	return n.Input.IsIncomplete() || n.Output.IsIncomplete()
}

// RPCElement is an interface implemented by all AST nodes that can
// appear in the body of an rpc declaration (aka method).
type RPCElement interface {
	Node
	methodElement()
}

var (
	_ RPCElement = (*OptionNode)(nil)
	_ RPCElement = (*EmptyDeclNode)(nil)
)

// RPCTypeNode represents the declaration of a request or response type for an
// RPC. Example:
//
//	(stream foo.Bar)
type RPCTypeNode struct {
	OpenParen   *RuneNode
	Stream      *KeywordNode
	MessageType IdentValueNode
	CloseParen  *RuneNode
}

func (n *RPCTypeNode) Start() Token { return n.OpenParen.Token() }
func (n *RPCTypeNode) End() Token   { return n.CloseParen.Token() }

func (n *RPCTypeNode) IsIncomplete() bool {
	return IsNil(n.MessageType)
}

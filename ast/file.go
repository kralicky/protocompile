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

import "google.golang.org/protobuf/proto"

// NewFileNode creates a new *FileNode. The syntax parameter is optional. If it
// is absent, it means the file had no syntax declaration.
//
// This function panics if the concrete type of any element of decls is not
// from this package.
func NewFileNode(info *FileInfo, syntax *SyntaxNode, decls []*FileElement, eof Token) *FileNode {
	return newFileNode(info, syntax, nil, decls, eof)
}

// NewFileNodeWithEdition creates a new *FileNode. The edition parameter is required. If a file
// has no edition declaration, use NewFileNode instead.
//
// This function panics if the concrete type of any element of decls is not
// from this package.
func NewFileNodeWithEdition(info *FileInfo, edition *EditionNode, decls []*FileElement, eof Token) *FileNode {
	if edition == nil {
		panic("edition is nil")
	}
	return newFileNode(info, nil, edition, decls, eof)
}

func newFileNode(info *FileInfo, syntax *SyntaxNode, edition *EditionNode, decls []*FileElement, eof Token) *FileNode {
	var pragmas map[string]string
	if syntax != nil {
		pragmas = parsePragmas(info.NodeInfo(syntax).LeadingComments())
	} else if edition != nil {
		pragmas = parsePragmas(info.NodeInfo(edition).LeadingComments())
	}

	eofNode := &RuneNode{Token: eof, Rune: 0}

	node := &FileNode{
		Syntax:  syntax,
		Edition: edition,
		Decls:   decls,
		EOF:     eofNode,
	}
	proto.SetExtension(node, E_FileInfo, info)
	if pragmas != nil {
		proto.SetExtension(node, E_ExtendedAttributes, &ExtendedAttributes{Pragmas: pragmas})
	}
	return node
}

func (n *FileNode) fileInfo() *FileInfo {
	return proto.GetExtension(n, E_FileInfo).(*FileInfo)
}

func (n *FileElement) Start() Token {
	if u := n.Unwrap(); u != nil {
		return u.Start()
	}
	return TokenUnknown
}

func (n *FileElement) End() Token {
	if u := n.Unwrap(); u != nil {
		return u.End()
	}
	return TokenUnknown
}

func (f *FileNode) Start() Token {
	if f.Syntax != nil {
		return f.Syntax.Start()
	} else if f.Edition != nil {
		return f.Edition.Start()
	} else if len(f.Decls) > 0 {
		return f.Decls[0].Start()
	}
	return f.EOF.GetToken()
}

func (f *FileNode) End() Token {
	return f.EOF.GetToken()
}

// Returns the last non-EOF token, or TokenError if EOF is the only token.
func (f *FileNode) EndExclusive() Token {
	if len(f.Decls) > 0 {
		return f.Decls[len(f.Decls)-1].End()
	}
	if f.Syntax != nil {
		return f.Syntax.End()
	} else if f.Edition != nil {
		return f.Edition.End()
	}
	return TokenError
}

// NewEmptyFileNode returns an empty AST for a file with the given name.
func NewEmptyFileNode(filename string, version int32) *FileNode {
	fileInfo := NewFileInfo(filename, []byte{}, version)
	return NewFileNode(fileInfo, nil, nil, fileInfo.AddToken(0, 0))
}

func (f *FileNode) Name() string {
	return f.fileInfo().GetName()
}

func (f *FileNode) Version() int32 {
	return f.fileInfo().GetVersion()
}

func (f *FileNode) NodeInfo(n Node) NodeInfo {
	return f.fileInfo().NodeInfo(n)
}

func (f *FileNode) TokenInfo(t Token) NodeInfo {
	return f.fileInfo().TokenInfo(t)
}

func (f *FileNode) ItemInfo(i Item) ItemInfo {
	return f.fileInfo().ItemInfo(i)
}

func (f *FileNode) GetItem(i Item) (Token, Comment) {
	return f.fileInfo().GetItem(i)
}

func (f *FileNode) Items() Sequence[Item] {
	return f.fileInfo().Items()
}

func (f *FileNode) Tokens() Sequence[Token] {
	return f.fileInfo().Tokens()
}

func (f *FileNode) SourcePos(offset int) SourcePos {
	return f.fileInfo().SourcePos(offset)
}

// ItemAtOffset returns the token or comment at the given offset. Only one of
// the return values will be valid. If the item is a token then the returned
// comment will be a zero value and thus invalid (i.e. comment.IsValid() returns
// false). If the item is a comment then the returned token will be TokenError.
func (f *FileNode) ItemAtOffset(offset int) (Token, Comment) {
	return f.fileInfo().GetItem(f.fileInfo().TokenAtOffset(offset).AsItem())
}

func (f *FileNode) TokenAtOffset(offset int) Token {
	return f.fileInfo().TokenAtOffset(offset)
}

func (f *FileNode) Pragma(key string) (string, bool) {
	if proto.HasExtension(f, E_ExtendedAttributes) {
		xattr := proto.GetExtension(f, E_ExtendedAttributes).(*ExtendedAttributes)
		if xattr.Pragmas == nil {
			return "", false
		}
		val, ok := xattr.Pragmas[key]
		return val, ok
	}
	return "", false
}

func (m *SyntaxNode) Start() Token { return m.Keyword.Start() }
func (m *SyntaxNode) End() Token   { return endToken(m.Semicolon, m.Syntax) }

func (m *EditionNode) Start() Token { return m.Keyword.Start() }
func (m *EditionNode) End() Token   { return endToken(m.Semicolon, m.Edition) }

func (m *ImportNode) Start() Token { return m.Keyword.Start() }
func (m *ImportNode) End() Token   { return endToken(m.Semicolon, m.Name) }

func (*ImportNode) fileElement() {}

func (m *ImportNode) IsIncomplete() bool {
	return m.Name == nil
}

func (m *PackageNode) Start() Token { return m.Keyword.Start() }
func (m *PackageNode) End() Token   { return endToken(m.Semicolon, m.Name) }

func (*PackageNode) fileElement() {}

func (p *PackageNode) IsIncomplete() bool {
	return p.Name == nil
}

func (f *FileNode) DebugAnnotated() string {
	return f.fileInfo().DebugAnnotated()
}

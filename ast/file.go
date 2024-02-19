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

// FileDeclNode is a placeholder interface for AST nodes that represent files.
// This allows NoSourceNode to be used in place of *FileNode for some usages.
type FileDeclNode interface {
	Node
	Name() string
	NodeInfo(n Node) NodeInfo
}

var (
	_ FileDeclNode = (*FileNode)(nil)
	_ FileDeclNode = NoSourceNode{}
)

// FileNode is the root of the AST hierarchy. It represents an entire
// protobuf source file.
type FileNode struct {
	fileInfo *FileInfo

	// A map of implementation-specific key-value pairs parsed from comments on
	// the syntax or edition declaration. These work like the //go: comments in
	// Go source files.
	Pragmas map[string]string

	// A file has either a Syntax or Edition node, never both.
	// If both are nil, neither declaration is present and the
	// file is assumed to use "proto2" syntax.
	Syntax  *SyntaxNode
	Edition *EditionNode

	Decls []FileElement

	// This synthetic node allows access to final comments and whitespace
	EOF *RuneNode
}

// NewFileNode creates a new *FileNode. The syntax parameter is optional. If it
// is absent, it means the file had no syntax declaration.
//
// This function panics if the concrete type of any element of decls is not
// from this package.
func NewFileNode(info *FileInfo, syntax *SyntaxNode, decls []FileElement, eof Token) *FileNode {
	return newFileNode(info, syntax, nil, decls, eof)
}

// NewFileNodeWithEdition creates a new *FileNode. The edition parameter is required. If a file
// has no edition declaration, use NewFileNode instead.
//
// This function panics if the concrete type of any element of decls is not
// from this package.
func NewFileNodeWithEdition(info *FileInfo, edition *EditionNode, decls []FileElement, eof Token) *FileNode {
	if edition == nil {
		panic("edition is nil")
	}
	return newFileNode(info, nil, edition, decls, eof)
}

func newFileNode(info *FileInfo, syntax *SyntaxNode, edition *EditionNode, decls []FileElement, eof Token) *FileNode {
	var pragmas map[string]string
	if syntax != nil {
		pragmas = parsePragmas(info.NodeInfo(syntax).LeadingComments())
	} else if edition != nil {
		pragmas = parsePragmas(info.NodeInfo(edition).LeadingComments())
	}

	eofNode := &RuneNode{TerminalNode: eof.AsTerminalNode(), Rune: 0}

	return &FileNode{
		fileInfo: info,
		Pragmas:  pragmas,
		Syntax:   syntax,
		Edition:  edition,
		Decls:    decls,
		EOF:      eofNode,
	}
}

func (f *FileNode) Start() Token {
	if f.Syntax != nil {
		return f.Syntax.Start()
	} else if f.Edition != nil {
		return f.Edition.Start()
	} else if len(f.Decls) > 0 {
		return f.Decls[0].Start()
	}
	return f.EOF.Token()
}

func (f *FileNode) End() Token {
	return f.EOF.Token()
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
	return f.fileInfo.Name()
}

func (f *FileNode) Version() int32 {
	return f.fileInfo.Version()
}

func (f *FileNode) NodeInfo(n Node) NodeInfo {
	return f.fileInfo.NodeInfo(n)
}

func (f *FileNode) TokenInfo(t Token) NodeInfo {
	return f.fileInfo.TokenInfo(t)
}

func (f *FileNode) ItemInfo(i Item) ItemInfo {
	return f.fileInfo.ItemInfo(i)
}

func (f *FileNode) GetItem(i Item) (Token, Comment) {
	return f.fileInfo.GetItem(i)
}

func (f *FileNode) Items() Sequence[Item] {
	return f.fileInfo.Items()
}

func (f *FileNode) Tokens() Sequence[Token] {
	return f.fileInfo.Tokens()
}

func (f *FileNode) SourcePos(offset int) SourcePos {
	return f.fileInfo.SourcePos(offset)
}

func (f *FileNode) GetSyntaxNode() *SyntaxNode {
	return f.Syntax
}

func (f *FileNode) GetEditionNode() *EditionNode {
	return f.Edition
}

func (f *FileNode) GetDecls() []FileElement {
	return f.Decls
}

func (f *FileNode) GetEOF() *RuneNode {
	return f.EOF
}

// ItemAtOffset returns the token or comment at the given offset. Only one of
// the return values will be valid. If the item is a token then the returned
// comment will be a zero value and thus invalid (i.e. comment.IsValid() returns
// false). If the item is a comment then the returned token will be TokenError.
func (f *FileNode) ItemAtOffset(offset int) (Token, Comment) {
	return f.fileInfo.GetItem(f.fileInfo.TokenAtOffset(offset).AsItem())
}

func (f *FileNode) TokenAtOffset(offset int) Token {
	return f.fileInfo.TokenAtOffset(offset)
}

func (f *FileNode) Pragma(key string) (string, bool) {
	if f.Pragmas == nil {
		return "", false
	}
	val, ok := f.Pragmas[key]
	return val, ok
}

// FileElement is an interface implemented by all AST nodes that are
// allowed as top-level declarations in the file.
type FileElement interface {
	Node
	fileElement()
}

var (
	_ FileElement = (*ImportNode)(nil)
	_ FileElement = (*PackageNode)(nil)
	_ FileElement = (*OptionNode)(nil)
	_ FileElement = (*MessageNode)(nil)
	_ FileElement = (*EnumNode)(nil)
	_ FileElement = (*ExtendNode)(nil)
	_ FileElement = (*ServiceNode)(nil)
	_ FileElement = (*EmptyDeclNode)(nil)
	_ FileElement = (*ErrorNode)(nil)
)

// SyntaxNode represents a syntax declaration, which if present must be
// the first non-comment content. Example:
//
//	syntax = "proto2";
//
// Files that don't have a syntax node are assumed to use proto2 syntax.
type SyntaxNode struct {
	Keyword   *KeywordNode
	Equals    *RuneNode
	Syntax    StringValueNode
	Semicolon *RuneNode
}

func (m *SyntaxNode) Start() Token { return m.Keyword.Start() }
func (m *SyntaxNode) End() Token   { return endToken(m.Semicolon, m.Syntax) }

// EditionNode represents an edition declaration, which if present must be
// the first non-comment content. Example:
//
//	edition = "2023";
//
// Files may include either an edition node or a syntax node, but not both.
// If neither are present, the file is assumed to use proto2 syntax.
type EditionNode struct {
	Keyword   *KeywordNode
	Equals    *RuneNode
	Edition   StringValueNode
	Semicolon *RuneNode
}

func (m *EditionNode) Start() Token { return m.Keyword.Start() }
func (m *EditionNode) End() Token   { return endToken(m.Semicolon, m.Edition) }

// ImportNode represents an import statement. Example:
//
//	import "google/protobuf/empty.proto";
type ImportNode struct {
	Keyword *KeywordNode
	// Optional; if present indicates this is a public import
	Public *KeywordNode
	// Optional; if present indicates this is a weak import
	Weak      *KeywordNode
	Name      StringValueNode
	Semicolon *RuneNode
}

func (m *ImportNode) Start() Token { return m.Keyword.Start() }
func (m *ImportNode) End() Token   { return endToken(m.Semicolon, m.Name) }

func (*ImportNode) fileElement() {}

func (m *ImportNode) IsIncomplete() bool {
	return m.Name == nil
}

// PackageNode represents a package declaration. Example:
//
//	package foobar.com;
type PackageNode struct {
	Keyword   *KeywordNode
	Name      IdentValueNode
	Semicolon *RuneNode
}

func (m *PackageNode) Start() Token { return m.Keyword.Start() }
func (m *PackageNode) End() Token   { return endToken(m.Semicolon, m.Name) }

func (*PackageNode) fileElement() {}

func (p *PackageNode) IsIncomplete() bool {
	return p.Name == nil
}

func (f *FileNode) DebugAnnotated() string {
	return f.fileInfo.DebugAnnotated()
}

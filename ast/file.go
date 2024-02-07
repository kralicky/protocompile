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
)

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
	compositeNode
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
	numChildren := len(decls) + 1
	if syntax != nil || edition != nil {
		numChildren++
	}
	var pragmas map[string]string
	children := make([]Node, 0, numChildren)
	if syntax != nil {
		children = append(children, syntax)
		pragmas = parsePragmas(info.NodeInfo(syntax).LeadingComments())
	} else if edition != nil {
		children = append(children, edition)
		pragmas = parsePragmas(info.NodeInfo(edition).LeadingComments())
	}
	for _, decl := range decls {
		switch decl := decl.(type) {
		case *PackageNode, *ImportNode, *OptionNode, *MessageNode,
			*EnumNode, *ExtendNode, *ServiceNode, *EmptyDeclNode, *ErrorNode:
		default:
			panic(fmt.Sprintf("invalid FileElement type: %T", decl))
		}
		children = append(children, decl)
	}

	eofNode := NewRuneNode(0, eof)
	children = append(children, eofNode)

	return &FileNode{
		compositeNode: compositeNode{
			children: children,
		},
		fileInfo: info,
		Pragmas:  pragmas,
		Syntax:   syntax,
		Edition:  edition,
		Decls:    decls,
		EOF:      eofNode,
	}
}

// NewEmptyFileNode returns an empty AST for a file with the given name.
func NewEmptyFileNode(filename string) *FileNode {
	fileInfo := NewFileInfo(filename, []byte{})
	return NewFileNode(fileInfo, nil, nil, fileInfo.AddToken(0, 0))
}

func (f *FileNode) Name() string {
	return f.fileInfo.Name()
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

func (f *FileNode) ItemAtOffset(offset int) Token {
	return f.fileInfo.TokenAtOffset(offset, true)
}

func (f *FileNode) TokenAtOffset(offset int) Token {
	return f.fileInfo.TokenAtOffset(offset, false)
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
	compositeNode
	Keyword   *KeywordNode
	Equals    *RuneNode
	Syntax    StringValueNode
	Semicolon *RuneNode
}

func (m *SyntaxNode) AddSemicolon(semi *RuneNode) {
	m.Semicolon = semi
	m.children = append(m.children, semi)
}

// NewSyntaxNode creates a new *SyntaxNode. All four arguments must be non-nil:
//   - keyword: The token corresponding to the "syntax" keyword.
//   - equals: The token corresponding to the "=" rune.
//   - syntax: The actual syntax value, e.g. "proto2" or "proto3".
//   - semicolon: The token corresponding to the ";" rune that ends the declaration.
func NewSyntaxNode(keyword *KeywordNode, equals *RuneNode, syntax StringValueNode) *SyntaxNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	if equals == nil {
		panic("equals is nil")
	}
	if syntax == nil {
		panic("syntax is nil")
	}
	children := []Node{keyword, equals, syntax}
	return &SyntaxNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
		Equals:  equals,
		Syntax:  syntax,
	}
}

// EditionNode represents an edition declaration, which if present must be
// the first non-comment content. Example:
//
//	edition = "2023";
//
// Files may include either an edition node or a syntax node, but not both.
// If neither are present, the file is assumed to use proto2 syntax.
type EditionNode struct {
	compositeNode
	Keyword   *KeywordNode
	Equals    *RuneNode
	Edition   StringValueNode
	Semicolon *RuneNode
}

func (m *EditionNode) AddSemicolon(semi *RuneNode) {
	m.Semicolon = semi
	m.children = append(m.children, semi)
}

// NewEditionNode creates a new *EditionNode. All four arguments must be non-nil:
//   - keyword: The token corresponding to the "edition" keyword.
//   - equals: The token corresponding to the "=" rune.
//   - edition: The actual edition value, e.g. "2023".
//   - semicolon: The token corresponding to the ";" rune that ends the declaration.
func NewEditionNode(keyword *KeywordNode, equals *RuneNode, edition StringValueNode) *EditionNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	if equals == nil {
		panic("equals is nil")
	}
	if edition == nil {
		panic("edition is nil")
	}
	children := []Node{keyword, equals, edition}
	return &EditionNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
		Equals:  equals,
		Edition: edition,
	}
}

// ImportNode represents an import statement. Example:
//
//	import "google/protobuf/empty.proto";
type ImportNode struct {
	compositeNode
	Keyword *KeywordNode
	// Optional; if present indicates this is a public import
	Public *KeywordNode
	// Optional; if present indicates this is a weak import
	Weak      *KeywordNode
	Name      StringValueNode
	Semicolon *RuneNode
}

// NewImportNode creates a new *ImportNode. The public and weak arguments are optional
// and only one or the other (or neither) may be specified, not both. When public is
// non-nil, it indicates the "public" keyword in the import statement and means this is
// a public import. When weak is non-nil, it indicates the "weak" keyword in the import
// statement and means this is a weak import. When both are nil, this is a normal import.
// The other arguments must be non-nil:
//   - keyword: The token corresponding to the "import" keyword.
//   - public: The token corresponding to the optional "public" keyword.
//   - weak: The token corresponding to the optional "weak" keyword.
//   - name: The actual imported file name.
func NewImportNode(keyword *KeywordNode, public *KeywordNode, weak *KeywordNode, name StringValueNode) *ImportNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	if name == nil {
		panic("name is nil")
	}
	children := []Node{keyword}
	if public != nil {
		children = append(children, public)
	} else if weak != nil {
		children = append(children, weak)
	}
	children = append(children, name)

	return &ImportNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
		Public:  public,
		Weak:    weak,
		Name:    name,
	}
}

func (*ImportNode) fileElement() {}

func (m *ImportNode) AddSemicolon(semi *RuneNode) {
	m.Semicolon = semi
	m.children = append(m.children, semi)
}

func NewIncompleteImportNode(keyword *KeywordNode, public *KeywordNode, weak *KeywordNode) *ImportNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	children := []Node{keyword}
	if public != nil {
		children = append(children, public)
	} else if weak != nil {
		children = append(children, weak)
	}
	return &ImportNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
		Public:  public,
		Weak:    weak,
	}
}

func (m *ImportNode) IsIncomplete() bool {
	return m.Name == nil
}

// PackageNode represents a package declaration. Example:
//
//	package foobar.com;
type PackageNode struct {
	compositeNode
	Keyword   *KeywordNode
	Name      IdentValueNode
	Semicolon *RuneNode
}

func (*PackageNode) fileElement() {}

func (p *PackageNode) AddSemicolon(semi *RuneNode) {
	p.Semicolon = semi
	p.children = append(p.children, semi)
}

// NewPackageNode creates a new *PackageNode. All three arguments must be non-nil:
//   - keyword: The token corresponding to the "package" keyword.
//   - name: The package name declared for the file.
//   - semicolon: The token corresponding to the ";" rune that ends the declaration.
func NewPackageNode(keyword *KeywordNode, name IdentValueNode) *PackageNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	if name == nil {
		panic("name is nil")
	}
	children := []Node{keyword, name}
	return &PackageNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
		Name:    name,
	}
}

func NewIncompletePackageNode(keyword *KeywordNode) *PackageNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	children := []Node{keyword}
	return &PackageNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword: keyword,
	}
}

func (p *PackageNode) IsIncomplete() bool {
	return p.Name == nil
}

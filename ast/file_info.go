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
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// FileInfo contains information about the contents of a source file, including
// details about comments and items. A lexer accumulates these details as it
// scans the file contents. This allows efficient representation of things like
// source positions.
type FileInfo struct {
	// The name of the source file.
	name string
	// The raw contents of the source file.
	data []byte
	// The offsets for each line in the file. The value is the zero-based byte
	// offset for a given line. The line is given by its index. So the value at
	// index 0 is the offset for the first line (which is always zero). The
	// value at index 1 is the offset at which the second line begins. Etc.
	lines []int
	// The info for every comment in the file. This is empty if the file has no
	// comments. The first entry corresponds to the first comment in the file,
	// and so on.
	comments []commentInfo
	// The info for every lexed item in the file. The last item in the slice
	// corresponds to the EOF, so every file (even an empty one) should have at
	// least one entry. This includes all terminal symbols (tokens) in the AST
	// as well as all comments.
	items []itemSpan
	// Document version, if provided by the resolver. The value is not used for
	// any purpose other than to allow the caller to attach version information
	// to the file for use in external tooling.
	version int32
	// zero-length-token counts, used for validation.
	zltTotalCount       int
	zltConsecutiveCount int
}

type commentInfo struct {
	// the index of the item, in the file's items slice, that represents this
	// comment
	index int
	// the index of the token to which this comment is attributed.
	attributedToIndex int
	// if > 0, the comment is attributed to the token in attributedToIndex for
	// display and interaction purposes, but only because the token it should be
	// attributed to is a virtual token - this has implications when formatting
	// extended syntax virtual tokens.
	virtualIndex int
}

type itemSpan struct {
	// the offset into the file of the first character of an item.
	offset int
	// the length of the item
	length int
}

// NewFileInfo creates a new instance for the given file.
func NewFileInfo(filename string, contents []byte, version int32) *FileInfo {
	return &FileInfo{
		name:    filename,
		data:    contents,
		lines:   []int{0},
		version: version,
	}
}

func (f *FileInfo) Name() string {
	return f.name
}

func (f *FileInfo) Version() int32 {
	return f.version
}

// AddLine adds the offset representing the beginning of the "next" line in the file.
// The first line always starts at offset 0, the second line starts at offset-of-newline-char+1.
func (f *FileInfo) AddLine(offset int) {
	if offset < 0 {
		panic(fmt.Sprintf("invalid offset: %d must not be negative", offset))
	}
	if offset > len(f.data) {
		panic(fmt.Sprintf("invalid offset: %d is greater than file size %d", offset, len(f.data)))
	}

	if len(f.lines) > 0 {
		lastOffset := f.lines[len(f.lines)-1]
		if offset <= lastOffset {
			panic(fmt.Sprintf("invalid offset: %d is not greater than previously observed line offset %d", offset, lastOffset))
		}
	}

	f.lines = append(f.lines, offset)
}

// AddToken adds info about a token at the given location to this file. It
// returns a value that allows access to all of the token's details.
func (f *FileInfo) AddToken(offset, length int) Token {
	if offset < 0 {
		panic(fmt.Sprintf("invalid offset: %d must not be negative", offset))
	}
	if length < 0 {
		panic(fmt.Sprintf("invalid length: %d must not be negative", length))
	}
	if offset+length > len(f.data) {
		panic(fmt.Sprintf("invalid offset+length: %d is greater than file size %d", offset+length, len(f.data)))
	}

	tokenID := len(f.items)
	if len(f.items) > 0 {
		lastToken := f.items[tokenID-1]
		lastEnd := lastToken.offset + lastToken.length - 1
		if offset <= lastEnd {
			panic(fmt.Sprintf("invalid offset: %d is not greater than previously observed token end %d", offset, lastEnd))
		}
	}

	// run some sanity checks, bugs in the lexer can cause memory to grow
	// forever until oom-killed, which is not an ideal user experience
	if len(f.items)-f.zltTotalCount > len(f.data)+1 {
		// if we have more non-zero-length tokens than bytes in the file, something
		// has gone horribly wrong
		f.abort(`
FATAL: More tokens have been created than could possibly exist in the file.
Stopping execution to prevent unbounded memory growth.`[1:])
	} else if f.zltConsecutiveCount > 10 {
		// too many consecutive zero-length tokens is definitely a bug; at most,
		// there could be two or three in sequence under certain circumstances.
		f.abort(`
FATAL: More than 10 consecutive zero-length tokens have been created.
Stopping execution to prevent unbounded memory growth.`[1:])
	}

	f.items = append(f.items, itemSpan{offset: offset, length: length})
	if length == 0 {
		f.zltConsecutiveCount++
		f.zltTotalCount++
	} else {
		f.zltConsecutiveCount = 0
	}
	return Token(tokenID)
}

func (f *FileInfo) abort(message string) {
	var tokenList strings.Builder
	for i := range f.items {
		if i > 0 {
			tokenList.WriteString("\n")
		}
		info := f.TokenInfo(Token(i))
		start, end := info.Start(), info.End()
		pos := fmt.Sprintf("%4d:%02d-%02d", start.Line, start.Col, end.Col)
		tokenText := info.RawText()
		if len(tokenText) == 0 {
			tokenText = "(empty)"
		} else {
			tokenText = fmt.Sprintf("%q", tokenText)
		}
		tokenList.WriteString(fmt.Sprintf("%s | %s", pos, tokenText))
	}
	panic(fmt.Sprintf(`
================================================================================
%s

This is a bug! If you see this message, please consider reporting it at
https://github.com/kralicky/protols/issues/new

Accumulated tokens (%d):
%s

line:col   | text
-----------+-----------
%s
================================================================================
`, message, len(f.items), f.name, tokenList.String()))
}

// AddComment adds info about a comment to this file. Comments must first be
// added as items via f.AddToken(). The given comment argument is the Token
// from that step. The given attributedTo argument indicates another token in the
// file with which the comment is associated. If comment's offset is before that
// of attributedTo, then this is a leading comment. Otherwise, it is a trailing
// comment.
func (f *FileInfo) AddComment(comment, attributedTo Token) {
	if len(f.comments) > 0 {
		lastComment := f.comments[len(f.comments)-1]
		if int(comment) <= lastComment.index {
			panic(fmt.Sprintf("invalid index: %d is not greater than previously observed comment index %d", comment, lastComment.index))
		}
		if int(attributedTo) < lastComment.attributedToIndex {
			panic(fmt.Sprintf("invalid attribution: %d is not greater than previously observed comment attribution index %d", attributedTo, lastComment.attributedToIndex))
		}
	}

	f.comments = append(f.comments, commentInfo{
		index:             int(comment),
		attributedToIndex: int(attributedTo),
		virtualIndex:      -1,
	})
}

func (f *FileInfo) AddVirtualComment(comment Token, attributedTo Token, virtualToken Token) {
	if len(f.comments) > 0 {
		lastComment := f.comments[len(f.comments)-1]
		if int(comment) <= lastComment.index {
			panic(fmt.Sprintf("invalid index: %d is not greater than previously observed comment index %d", comment, lastComment.index))
		}
		if int(attributedTo) < lastComment.attributedToIndex {
			panic(fmt.Sprintf("invalid attribution: %d is not greater than previously observed comment attribution index %d", attributedTo, lastComment.attributedToIndex))
		}
	}

	f.comments = append(f.comments, commentInfo{
		index:             int(comment),
		attributedToIndex: int(attributedTo),
		virtualIndex:      int(virtualToken),
	})
}

// NodeInfo returns details from the original source for the given AST node.
//
// If the given n is out of range, this returns an invalid NodeInfo (i.e.
// nodeInfo.IsValid() returns false). If the given n is not out of range but
// also from a different file than f, then the result is undefined.
func (f *FileInfo) NodeInfo(n Node) NodeInfo {
	return f.nodeInfo(int(n.Start()), int(n.End()))
}

// TokenInfo returns details from the original source for the given token.
//
// If the given t is out of range, this returns an invalid NodeInfo (i.e.
// nodeInfo.IsValid() returns false). If the given t is not out of range but
// also from a different file than f, then the result is undefined.
func (f *FileInfo) TokenInfo(t Token) NodeInfo {
	return f.nodeInfo(int(t), int(t))
}

func (f *FileInfo) nodeInfo(start, end int) NodeInfo {
	if start < 0 || start >= len(f.items) {
		return NodeInfo{
			fileInfo:   f,
			startIndex: -1,
			endIndex:   -1,
		}
	}
	if end < 0 || end >= len(f.items) {
		return NodeInfo{fileInfo: f}
	}
	return NodeInfo{fileInfo: f, startIndex: start, endIndex: end}
}

// ItemInfo returns details from the original source for the given item.
//
// If the given i is out of range, this returns nil. If the given i is not
// out of range but also from a different file than f, then the result is
// undefined.
func (f *FileInfo) ItemInfo(i Item) ItemInfo {
	tok, cmt := f.GetItem(i)
	if tok != TokenError {
		return f.TokenInfo(tok)
	}
	if cmt.IsValid() {
		return cmt
	}
	return nil
}

// GetItem returns the token or comment represented by the given item. Only one
// of the return values will be valid. If the item is a token then the returned
// comment will be a zero value and thus invalid (i.e. comment.IsValid() returns
// false). If the item is a comment then the returned token will be TokenError.
//
// If the given i is out of range, this returns (TokenError, Comment{}). If the
// given i is not out of range but also from a different file than f, then
// the result is undefined.
func (f *FileInfo) GetItem(i Item) (Token, Comment) {
	if i < 0 || int(i) >= len(f.items) {
		return TokenError, Comment{}
	}
	if !f.isComment(i) {
		return Token(i), Comment{}
	}
	// It's a comment, so find its location in f.comments
	c := sort.Search(len(f.comments), func(c int) bool {
		return f.comments[c].index >= int(i)
	})
	if c < len(f.comments) && f.comments[c].index == int(i) {
		return TokenError, Comment{fileInfo: f, info: f.comments[c]}
	}
	// f.isComment(i) returned true, but we couldn't find it
	// in f.comments? Uh oh... that shouldn't be possible.
	return TokenError, Comment{}
}

func (f *FileInfo) isDummyFile() bool {
	return f == nil || f.lines == nil
}

// Sequence represents a navigable sequence of elements.
type Sequence[T any] interface {
	// First returns the first element in the sequence. The bool return
	// is false if this sequence contains no elements. For example, an
	// empty file has no items or tokens.
	First() (T, bool)
	// Next returns the next element in the sequence that comes after
	// the given element. The bool returns is false if there is no next
	// item (i.e. the given element is the last one). It also returns
	// false if the given element is invalid.
	Next(T) (T, bool)
	// Last returns the last element in the sequence. The bool return
	// is false if this sequence contains no elements. For example, an
	// empty file has no items or tokens.
	Last() (T, bool)
	// Previous returns the previous element in the sequence that comes
	// before the given element. The bool returns is false if there is no
	// previous item (i.e. the given element is the first one). It also
	// returns false if the given element is invalid.
	Previous(T) (T, bool)
}

func (f *FileInfo) Items() Sequence[Item] {
	return items{fileInfo: f}
}

func (f *FileInfo) Tokens() Sequence[Token] {
	return tokens{fileInfo: f}
}

type items struct {
	fileInfo *FileInfo
}

func (i items) First() (Item, bool) {
	if len(i.fileInfo.items) == 0 {
		return 0, false
	}
	return 0, true
}

func (i items) Next(item Item) (Item, bool) {
	if item < 0 || int(item) >= len(i.fileInfo.items)-1 {
		return 0, false
	}
	return i.fileInfo.itemForward(item+1, true)
}

func (i items) Last() (Item, bool) {
	if len(i.fileInfo.items) == 0 {
		return 0, false
	}
	return Item(len(i.fileInfo.items) - 1), true
}

func (i items) Previous(item Item) (Item, bool) {
	if item <= 0 || int(item) >= len(i.fileInfo.items) {
		return 0, false
	}
	return i.fileInfo.itemBackward(item-1, true)
}

type tokens struct {
	fileInfo *FileInfo
}

func (t tokens) First() (Token, bool) {
	i, ok := t.fileInfo.itemForward(0, false)
	return Token(i), ok
}

func (t tokens) Next(tok Token) (Token, bool) {
	if tok < 0 || int(tok) >= len(t.fileInfo.items)-1 {
		return 0, false
	}
	i, ok := t.fileInfo.itemForward(Item(tok+1), false)
	return Token(i), ok
}

func (t tokens) Last() (Token, bool) {
	i, ok := t.fileInfo.itemBackward(Item(len(t.fileInfo.items))-1, false)
	return Token(i), ok
}

func (t tokens) Previous(tok Token) (Token, bool) {
	if tok <= 0 || int(tok) >= len(t.fileInfo.items) {
		return 0, false
	}
	i, ok := t.fileInfo.itemBackward(Item(tok-1), false)
	return Token(i), ok
}

func (f *FileInfo) itemForward(i Item, allowComment bool) (Item, bool) {
	end := Item(len(f.items))
	for i < end {
		if allowComment || !f.isComment(i) {
			return i, true
		}
		i++
	}
	return 0, false
}

func (f *FileInfo) itemBackward(i Item, allowComment bool) (Item, bool) {
	for i >= 0 {
		if allowComment || !f.isComment(i) {
			return i, true
		}
		i--
	}
	return 0, false
}

// isComment is comment returns true if i refers to a comment.
// (If it returns false, i refers to a token.)
func (f *FileInfo) isComment(i Item) bool {
	item := f.items[i]
	if item.length < 2 {
		return false
	}
	// see if item text starts with "//" or "/*"
	if f.data[item.offset] != '/' {
		return false
	}
	c := f.data[item.offset+1]
	return c == '/' || c == '*'
}

func (f *FileInfo) SourcePos(offset int) SourcePos {
	lineNumber := sort.Search(len(f.lines), func(n int) bool {
		return f.lines[n] > offset
	})

	col := offset
	if lineNumber > 0 {
		col -= f.lines[lineNumber-1]
	}

	return SourcePos{
		Filename: f.name,
		Offset:   offset,
		Line:     lineNumber,
		// Columns are 1-indexed in this AST
		Col: col + 1,
	}
}

func (f *FileInfo) TokenAtOffset(offset int) Token {
	if offset < 0 || offset > len(f.data) || len(f.items) == 0 {
		return TokenError
	}

	// search for the token that contains the given offset, or if there is no
	// such token, the closest token on the same line as the given offset.
	// If there are no tokens on the same line, then TokenError is returned.
	targetLine, found := sort.Find(len(f.lines), func(n int) int {
		lineOffset := f.lines[n]
		if lineOffset > offset {
			// went past the target line
			return -1
		} else if n < len(f.lines)-1 && f.lines[n+1] <= offset {
			// not yet at the target line
			return 1
		}
		return 0
	})
	if !found {
		return TokenError
	}
	offsetMin := f.lines[targetLine]
	offsetMax := len(f.data)
	if targetLine < len(f.lines)-1 {
		offsetMax = f.lines[targetLine+1] - 1
	}
	targetIdx := sort.Search(len(f.items), func(n int) bool {
		item := f.items[n]
		return item.offset >= offsetMin &&
			(item.offset == offset || item.offset+item.length > offset)
	})
	if targetIdx == len(f.items) {
		// if the cursor is at EOF, then the last token is the target
		if offset == len(f.data) {
			targetIdx--
		} else {
			return TokenError
		}
	}

	// if the target token has a length of 0, or the next token is 1 or more
	// characters away from the target offset, check to see if the previous token
	// would be a better match.
	if f.items[targetIdx].length == 0 || f.items[targetIdx].offset-offset > 0 {
		i := targetIdx - 1
		for i > 0 && f.items[i].length == 0 {
			i--
		}
		if i < 0 {
			return TokenError
		}
		if item := f.items[i]; item.offset+item.length == offset {
			// only use the previous token if the position is directly after it
			targetIdx = i
		}
	}

	target := Token(targetIdx)
	if f.items[target].offset > offsetMax {
		// no tokens on the target line
		return TokenError
	}

	return target
}

// Token represents a single lexed token.
type Token int

// TokenError indicates an invalid token. It is returned from query
// functions when no valid token satisfies the request.
const TokenError = Token(-1)

// AsItem returns the Item that corresponds to t.
func (t Token) AsItem() Item {
	return Item(t)
}

func (t Token) AsTerminalNode() TerminalNode {
	return TerminalNode(t)
}

// Item represents an item lexed from source. It represents either
// a Token or a Comment.
type Item int

func (i Item) IsValid() bool {
	return i != -1
}

// ItemInfo provides details about an item's location in the source file and
// its contents.
type ItemInfo interface {
	SourceSpan
	LeadingWhitespace() string
	RawText() string
}

type SourceSpan interface {
	fmt.Stringer
	Start() SourcePos
	End() SourcePos
}

func NewSourceSpan(start SourcePos, end SourcePos) SourceSpan {
	return sourceSpan{StartPos: start, EndPos: end}
}

type sourceSpan struct {
	StartPos SourcePos
	EndPos   SourcePos
}

func (s sourceSpan) Start() SourcePos {
	return s.StartPos
}

func (s sourceSpan) End() SourcePos {
	return s.EndPos
}

func (s sourceSpan) String() string {
	if s.StartPos.Col == s.EndPos.Col {
		return fmt.Sprintf("%s:%d:%d", s.StartPos.Filename, s.StartPos.Line, s.StartPos.Col)
	} else {
		return fmt.Sprintf("%s:%d:%d-%d", s.StartPos.Filename, s.StartPos.Line, s.StartPos.Col, s.EndPos.Col)
	}
}

var _ SourceSpan = sourceSpan{}

// NodeInfo represents the details for a node or token in the source file's AST.
// It provides access to information about the node's location in the source
// file. It also provides access to the original text in the source file (with
// all the original formatting intact) and also provides access to surrounding
// comments.
type NodeInfo struct {
	fileInfo             *FileInfo
	startIndex, endIndex int
}

var _ ItemInfo = NodeInfo{}

// IsValid returns true if this node info is valid. If n is a zero-value struct,
// it is not valid.
func (n NodeInfo) IsValid() bool {
	return n.fileInfo != nil && n.startIndex >= 0 && n.endIndex >= 0
}

// Start returns the starting position of the element. This is the first
// character of the node or token.
func (n NodeInfo) Start() SourcePos {
	if n.fileInfo == nil {
		return SourcePos{}
	}
	if n.fileInfo.isDummyFile() || !n.IsValid() {
		return UnknownPos(n.fileInfo.name)
	}
	tok := n.fileInfo.items[n.startIndex]
	return n.fileInfo.SourcePos(tok.offset)
}

// End returns the ending position of the element, exclusive. This is the
// location after the last character of the node or token. If n returns
// the same position for Start() and End(), the element in source had a
// length of zero (which should only happen for the special EOF token
// that designates the end of the file).
func (n NodeInfo) End() SourcePos {
	if n.fileInfo == nil {
		return SourcePos{}
	}
	if n.fileInfo.isDummyFile() || !n.IsValid() {
		return UnknownPos(n.fileInfo.name)
	}

	tok := n.fileInfo.items[n.endIndex]
	// find offset of last character in the span
	offset := tok.offset
	if tok.length > 0 {
		offset += tok.length - 1
	}
	pos := n.fileInfo.SourcePos(offset)
	if tok.length > 0 {
		// We return "open range", so end is the position *after* the
		// last character in the span. So we adjust
		pos.Col++
	}
	return pos
}

// LeadingWhitespace returns any whitespace prior to the element. If there
// were comments in between this element and the previous one, this will
// return the whitespace between the last such comment in the element. If
// there were no such comments, this returns the whitespace between the
// previous element and the current one.
func (n NodeInfo) LeadingWhitespace() string {
	if n.fileInfo.isDummyFile() {
		return ""
	}

	tok := n.fileInfo.items[n.startIndex]
	if tok.length == 0 && n.startIndex < len(n.fileInfo.items)-1 {
		// leading whitespace is attributed to tokens that follow one or more
		// zero-length tokens (except eof, since it is always the last token)
		return ""
	}
	var prevEnd int
	if n.startIndex > 0 {
		var prevTok itemSpan
		for offset := -1; n.startIndex+offset >= 0; offset-- {
			prevTok = n.fileInfo.items[n.startIndex+offset]
			if prevTok.length > 0 {
				break
			}
		}
		prevEnd = prevTok.offset + prevTok.length
	}
	return string(n.fileInfo.data[prevEnd:tok.offset])
}

// LeadingComments returns all comments in the source that exist between the
// element and the previous element, except for any trailing comment on the
// previous element.
func (n NodeInfo) LeadingComments() Comments {
	if n.fileInfo.isDummyFile() {
		return EmptyComments
	}

	start := sort.Search(len(n.fileInfo.comments), func(i int) bool {
		return n.fileInfo.comments[i].attributedToIndex >= n.startIndex
	})

	if start == len(n.fileInfo.comments) || n.fileInfo.comments[start].attributedToIndex != n.startIndex {
		// no comments associated with this token
		return EmptyComments
	}

	numComments := 0
	for i := start; i < len(n.fileInfo.comments); i++ {
		comment := n.fileInfo.comments[i]
		if comment.attributedToIndex == n.startIndex &&
			comment.index < n.startIndex {
			numComments++
		} else {
			break
		}
	}

	return Comments{
		fileInfo: n.fileInfo,
		first:    start,
		num:      numComments,
	}
}

func (n NodeInfo) String() string {
	start, end := n.Start(), n.End()
	return fmt.Sprintf("%s:%d:%d-%d", start.Filename, start.Line, start.Col, end.Col)
}

// TrailingComments returns the trailing comment for the element, if any.
// An element will have a trailing comment only if it is the last token
// on a line and is followed by a comment on the same line. Typically, the
// following comment is a line-style comment (starting with "//").
//
// If the following comment is a block-style comment that spans multiple
// lines, and the next token is on the same line as the end of the comment,
// the comment is NOT considered a trailing comment.
//
// Examples:
//
//	foo // this is a trailing comment for foo
//
//	bar /* this is a trailing comment for bar */
//
//	baz /* this is a trailing
//	       comment for baz */
//
//	fizz /* this is NOT a trailing
//	        comment for fizz because
//	        its on the same line as the
//	        following token buzz */       buzz
func (n NodeInfo) TrailingComments() Comments {
	if n.fileInfo.isDummyFile() {
		return EmptyComments
	}

	start := sort.Search(len(n.fileInfo.comments), func(i int) bool {
		comment := n.fileInfo.comments[i]
		return (comment.virtualIndex >= n.endIndex) ||
			(comment.attributedToIndex >= n.endIndex && comment.index > n.endIndex)
	})

	numComments := 0
	var virtual []int
	for i := start; i < len(n.fileInfo.comments); i++ {
		comment := n.fileInfo.comments[i]
		if comment.attributedToIndex == n.endIndex {
			if comment.virtualIndex > 0 {
				virtual = append(virtual, numComments)
			}
			numComments++
		} else if comment.virtualIndex == n.endIndex {
			numComments++
		} else {
			break
		}
	}

	if numComments == 0 {
		return EmptyComments
	}

	return Comments{
		fileInfo: n.fileInfo,
		first:    start,
		num:      numComments,
		virtual:  virtual,
	}
}

// RawText returns the actual text in the source file that corresponds to the
// element. If the element is a node in the AST that encompasses multiple
// items (like an entire declaration), the full text of all items is returned
// including any interior whitespace and comments.
func (n NodeInfo) RawText() string {
	startTok := n.fileInfo.items[n.startIndex]
	endTok := n.fileInfo.items[n.endIndex]
	return string(n.fileInfo.data[startTok.offset : endTok.offset+endTok.length])
}

type FileInfoInterface interface {
	Name() string
	Version() int32
	SourcePos(int) SourcePos
	NodeInfo(Node) NodeInfo
	TokenInfo(Token) NodeInfo
	ItemInfo(Item) ItemInfo
	GetItem(Item) (Token, Comment)
}

type nodeInfoInternal interface {
	// ParentFile returns a limited interface which can be used to look up
	// node information for other nodes within the same file as this node.
	ParentFile() FileInfoInterface
}

// Returns an interface that allows limited access to internal node information.
// This should only be used for optimization purposes in places where this info
// would otherwise still be obtainable, but with a performance cost.
// This should not be considered part of the public NodeInfo API.
func (n NodeInfo) Internal() nodeInfoInternal {
	return nodeInfoInternalImpl{n}
}

type nodeInfoInternalImpl struct {
	NodeInfo
}

func (n nodeInfoInternalImpl) ParentFile() FileInfoInterface {
	return n.fileInfo
}

// SourcePos identifies a location in a proto source file.
type SourcePos struct {
	Filename string
	// The line and column numbers for this position. These are
	// one-based, so the first line and column is 1 (not zero). If
	// either is zero, then the line and column are unknown and
	// only the file name is known.
	Line, Col int
	// The offset, in bytes, from the beginning of the file. This
	// is zero-based: the first character in the file is offset zero.
	Offset int
}

func (pos SourcePos) String() string {
	if pos.Line <= 0 || pos.Col <= 0 {
		return pos.Filename
	}
	return fmt.Sprintf("%s:%d:%d", pos.Filename, pos.Line, pos.Col)
}

// Comments represents a range of sequential comments in a source file
// (e.g. no interleaving items or AST nodes).
type Comments struct {
	fileInfo   *FileInfo
	first, num int
	virtual    []int // indices of virtual comments
}

// EmptyComments is an empty set of comments.
var EmptyComments = Comments{}

// Len returns the number of comments in c.
func (c Comments) Len() int {
	return c.num
}

func (c Comments) Index(i int) Comment {
	if i < 0 || i >= c.num {
		panic(fmt.Sprintf("index %d out of range (len = %d)", i, c.num))
	}
	virtual := false
	if len(c.virtual) > 0 {
		virtual = slices.Contains(c.virtual, i)
	}
	return Comment{
		fileInfo: c.fileInfo,
		info:     c.fileInfo.comments[c.first+i],
		virtual:  virtual,
	}
}

// Comment represents a single comment in a source file. It indicates
// the position of the comment and its contents. A single comment means
// one line-style comment ("//" to end of line) or one block comment
// ("/*" through "*/"). If a longer comment uses multiple line comments,
// each line is considered to be a separate comment. For example:
//
//	// This is a single comment, and
//	// this is a separate comment.
type Comment struct {
	fileInfo *FileInfo
	info     commentInfo
	virtual  bool
}

var _ ItemInfo = Comment{}

// IsValid returns true if this comment is valid. If this comment is
// a zero-value struct, it is not valid.
func (c Comment) IsValid() bool {
	return c.fileInfo != nil && c.info.index >= 0
}

// AsItem returns the Item that corresponds to c.
func (c Comment) AsItem() Item {
	return Item(c.info.index)
}

func (c Comment) Start() SourcePos {
	span := c.fileInfo.items[c.AsItem()]
	return c.fileInfo.SourcePos(span.offset)
}

// TODO: for some reason, this returns the position of the last character in the
// comment, not the character after the last one, as is the case for tokens.
// Unsure why this is the case, possibly unintentional?
func (c Comment) End() SourcePos {
	span := c.fileInfo.items[c.AsItem()]
	return c.fileInfo.SourcePos(span.offset + span.length - 1)
}

func (c Comment) IsVirtual() bool {
	return c.virtual
}

func (c Comment) VirtualItem() Item {
	return Item(c.info.virtualIndex)
}

func (c Comment) AttributedTo() Item {
	return Item(c.info.attributedToIndex)
}

func (c Comment) LeadingWhitespace() string {
	item := c.AsItem()
	span := c.fileInfo.items[item]
	var prevEnd int
	if item > 0 {
		var prevItem itemSpan
		for offset := -1; int(item)+offset >= 0; offset-- {
			prevItem = c.fileInfo.items[int(item)+offset]
			if prevItem.length > 0 {
				break
			}
		}
		prevEnd = prevItem.offset + prevItem.length
	}
	return string(c.fileInfo.data[prevEnd:span.offset])
}

func (c Comment) RawText() string {
	span := c.fileInfo.items[c.AsItem()]
	return string(c.fileInfo.data[span.offset : span.offset+span.length])
}

func (c Comment) String() string {
	if !c.IsValid() {
		return ""
	}

	start, end := c.Start(), c.End()
	return fmt.Sprintf("%s:%d:%d-%d:%d: %s", start.Filename, start.Line, start.Col, end.Line, end.Col, c.RawText())
}

type NodeReference struct {
	Node
	NodeInfo
}

func NewNodeReference[F interface{ NodeInfo(Node) NodeInfo }](f F, node Node) NodeReference {
	return NodeReference{
		Node:     node,
		NodeInfo: f.NodeInfo(node),
	}
}

func (f *FileInfo) DebugAnnotated() string {
	var buf bytes.Buffer

	zltCount := 0
	for i, item := range f.items {
		info := f.ItemInfo(Item(i))
		start, end := item.offset, item.offset+item.length
		data := f.data[start:end]
		tokenLen := end - start
		if tokenLen == 0 {
			zltCount++
			continue
		} else if zltCount > 0 {
			// for zero-length tokens, print a bold black-on-white number
			// for the count of consecutive zero-length tokens
			buf.WriteString("\x1B[1;30;47m")
			buf.WriteString(fmt.Sprintf("%d", zltCount))
			buf.WriteString("\x1B[0m")
			zltCount = 0
		}
		// background-color tokens blue, comments green (ansi codes)
		// odd-numbered tokens are dimmed
		var bgcolor string
		var fgcolor string
		if f.isComment(Item(i)) {
			if i%2 == 0 {
				bgcolor = "\x1B[48;5;2m"
			} else {
				bgcolor = "\x1B[48;5;22m"
			}
		} else {
			if i%2 == 0 {
				bgcolor = "\x1B[48;5;4m"
			} else {
				bgcolor = "\x1B[48;5;24m"
			}
		}
		buf.WriteString(info.LeadingWhitespace())
		buf.WriteString(bgcolor)
		buf.WriteString(fgcolor)
		buf.WriteString(string(data))
		buf.WriteString("\x1B[0m")
	}

	return buf.String()
}

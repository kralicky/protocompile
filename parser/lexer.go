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

package parser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/reporter"
)

type runeReader struct {
	data []byte
	pos  int
	err  error
	mark int
	// Enable this check to make input required to be valid UTF-8.
	// For now, since protoc allows invalid UTF-8, default to false.
	utf8Strict bool

	savedPos int
}

func (rr *runeReader) save() {
	rr.savedPos = rr.pos
}

func (rr *runeReader) restore() {
	rr.pos = rr.savedPos
}

func (rr *runeReader) readRune() (r rune, size int, err error) {
	if rr.err != nil {
		return 0, 0, rr.err
	}
	if rr.pos == len(rr.data) {
		rr.err = io.EOF
		return 0, 0, rr.err
	}
	r, sz := utf8.DecodeRune(rr.data[rr.pos:])
	if rr.utf8Strict && r == utf8.RuneError {
		rr.err = fmt.Errorf("invalid UTF8 at offset %d: %x", rr.pos, rr.data[rr.pos])
		return 0, 0, rr.err
	}
	rr.pos += sz
	return r, sz, nil
}

func (rr *runeReader) offset() int {
	return rr.pos
}

func (rr *runeReader) unreadRune(sz int) {
	newPos := rr.pos - sz
	if newPos < rr.mark {
		if rr.err == io.EOF {
			rr.err = nil
			// un-reading EOF should not move the position
			return
		}
		panic("unread past mark")
	}
	rr.pos = newPos
}

func (rr *runeReader) setMark() {
	rr.mark = rr.pos
}

func (rr *runeReader) getMark() string {
	return string(rr.data[rr.mark:rr.pos])
}

type insertSemiMode int

const (
	atNextNewline         = 1
	immediate             = 2 | atNextNewline
	onlyIfMissing         = 4
	always                = 8 | onlyIfMissing
	onlyIfLastTokenOnLine = 16
)

type protoLex struct {
	input   *runeReader
	info    *ast.FileInfo
	handler *reporter.Handler
	res     *ast.FileNode

	prevSym    ast.TerminalNode
	prevOffset int
	eof        ast.Token

	// if true, the lexer will insert a semicolon and set insertSemi to false
	insertSemi              insertSemiMode
	inCompoundStringLiteral bool

	comments []ast.Token
}

var utf8Bom = []byte{0xEF, 0xBB, 0xBF}

func newLexer(in io.Reader, filename string, handler *reporter.Handler) (*protoLex, error) {
	br := bufio.NewReader(in)

	// if file has UTF8 byte order marker preface, consume it
	marker, err := br.Peek(3)
	if err == nil && bytes.Equal(marker, utf8Bom) {
		_, _ = br.Discard(3)
	}

	contents, err := io.ReadAll(br)
	if err != nil {
		return nil, err
	}
	return &protoLex{
		input:   &runeReader{data: contents},
		info:    ast.NewFileInfo(filename, contents),
		handler: handler,
	}, nil
}

var keywords = map[string]int{
	"bytes":      _BYTES,
	"bool":       _BOOL,
	"double":     _DOUBLE,
	"edition":    _EDITION,
	"enum":       _ENUM,
	"extend":     _EXTEND,
	"extensions": _EXTENSIONS,
	"false":      _FALSE,
	"fixed32":    _FIXED32,
	"fixed64":    _FIXED64,
	"float":      _FLOAT,
	"group":      _GROUP,
	"import":     _IMPORT,
	"inf":        _INF,
	"int32":      _INT32,
	"int64":      _INT64,
	"map":        _MAP,
	"max":        _MAX,
	"message":    _MESSAGE,
	"nan":        _NAN,
	"oneof":      _ONEOF,
	"optional":   _OPTIONAL,
	"option":     _OPTION,
	"package":    _PACKAGE,
	"public":     _PUBLIC,
	"repeated":   _REPEATED,
	"required":   _REQUIRED,
	"reserved":   _RESERVED,
	"returns":    _RETURNS,
	"rpc":        _RPC,
	"service":    _SERVICE,
	"sfixed32":   _SFIXED32,
	"sfixed64":   _SFIXED64,
	"sint32":     _SINT32,
	"sint64":     _SINT64,
	"stream":     _STREAM,
	"string":     _STRING,
	"syntax":     _SYNTAX,
	"to":         _TO,
	"true":       _TRUE,
	"uint32":     _UINT32,
	"uint64":     _UINT64,
	"weak":       _WEAK,
}

func (l *protoLex) maybeNewLine(r rune) {
	if r == '\n' {
		l.info.AddLine(l.input.offset())
	}
}

func (l *protoLex) prev() ast.SourcePos {
	return l.info.SourcePos(l.prevOffset)
}

func (l *protoLex) writeVirtualSemi(lval *protoSymType) int {
	l.input.unreadRune(1)
	l.setVirtualRune(lval, ';')
	return ';'
}

func (l *protoLex) Lex(lval *protoSymType) int {
	if l.handler.ReporterError() != nil {
		// if error reporter already returned non-nil error,
		// we can skip the rest of the input
		return 0
	}

	l.comments = nil

	for {
		l.input.setMark()

		l.prevOffset = l.input.offset()
		c, _, err := l.input.readRune()
		if err == io.EOF {
			// insert a semicolon if the last token was the '}' rune (i.e. end of message)
			if l.insertSemi != 0 {
				l.insertSemi = 0
				return l.writeVirtualSemi(lval)
			}

			// we're not actually returning a rune, but this will associate
			// accumulated comments as a trailing comment on last symbol
			// (if appropriate)
			l.setRune(lval, 0)
			l.eof = lval.b.Token()
			return 0
		}
		if err != nil {
			l.setError(lval, err)
			return _ERROR
		}

		if strings.ContainsRune("\r\t\f\v ", c) {
			// skip whitespace
			continue
		}

		if l.insertSemi&immediate == immediate {
			if (l.insertSemi&always == always) || c != ';' {
				l.insertSemi = 0
				return l.writeVirtualSemi(lval)
			}
			l.insertSemi = 0
		}

		switch c {
		case '}':
			l.insertSemi = immediate | always
		case '>':
			l.insertSemi = immediate | always
		case ']':
			l.insertSemi = immediate | onlyIfMissing
		case '\n':
			if l.insertSemi&atNextNewline != 0 {
				prev := l.prevSym
				prevRuneIsSemi := false
				switch prev := prev.(type) {
				case *ast.RuneNode:
					if prev.Rune == ';' {
						prevRuneIsSemi = true
					}
				}
				if (l.insertSemi&always == always) || !prevRuneIsSemi {
					l.insertSemi = 0
					return l.writeVirtualSemi(lval)
				}
				l.insertSemi = 0
			}
			l.info.AddLine(l.input.offset())
			continue
		default:
			if l.insertSemi&onlyIfLastTokenOnLine == onlyIfLastTokenOnLine {
				if c != '/' {
					l.insertSemi = 0
				}
			}
		}

		if c == '.' {
			// decimal literals could start with a dot
			cn, szn, err := l.input.readRune()
			if err != nil {
				l.setRune(lval, c)
				return int(c)
			}
			if cn >= '0' && cn <= '9' {
				l.readNumber()
				token := l.input.getMark()
				f, err := parseFloat(token)
				if err != nil {
					l.setError(lval, numError(err, "float", token))
					return _ERROR
				}
				l.setFloat(lval, f)
				return _FLOAT_LIT
			}
			l.input.unreadRune(szn)
			l.setRune(lval, c)
			return int(c)
		}

		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			// identifier
			l.readIdentifier()
			str := l.input.getMark()
			l.maybeProcessPartialField(str)
			if t, ok := keywords[str]; ok {
				l.setIdent(lval, str)
				return t
			}
			if lval.id != nil {
				switch lval.id.Val {
				case "package":
					// package expressions are a bit ambiguous when no semicolon is present
					l.insertSemi |= atNextNewline | onlyIfMissing | onlyIfLastTokenOnLine
				}
			}
			l.setIdent(lval, str)
			return _NAME
		}

		if c >= '0' && c <= '9' {
			// integer or float literal
			l.readNumber()
			token := l.input.getMark()
			if strings.HasPrefix(token, "0x") || strings.HasPrefix(token, "0X") {
				// hexadecimal
				ui, err := strconv.ParseUint(token[2:], 16, 64)
				if err != nil {
					l.setError(lval, numError(err, "hexadecimal integer", token[2:]))
					return _ERROR
				}
				l.setInt(lval, ui)
				return _INT_LIT
			}
			if strings.ContainsAny(token, ".eE") {
				// floating point!
				f, err := parseFloat(token)
				if err != nil {
					l.setError(lval, numError(err, "float", token))
					return _ERROR
				}
				l.setFloat(lval, f)
				return _FLOAT_LIT
			}
			// integer! (decimal or octal)
			base := 10
			if token[0] == '0' {
				base = 8
			}
			ui, err := strconv.ParseUint(token, base, 64)
			if err != nil {
				kind := "integer"
				if base == 8 {
					kind = "octal integer"
				} else if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
					// if it's too big to be an int, parse it as a float
					var f float64
					kind = "float"
					f, err = parseFloat(token)
					if err == nil {
						l.setFloat(lval, f)
						return _FLOAT_LIT
					}
				}
				l.setError(lval, numError(err, kind, token))
				return _ERROR
			}
			if !l.matchNextRune('[') {
				l.insertSemi |= atNextNewline | onlyIfMissing | onlyIfLastTokenOnLine
			}
			l.setInt(lval, ui)
			return _INT_LIT
		}

		if c == '\'' || c == '"' {
			// string literal
			str, err := l.readStringLiteral(c)
			if err != nil {
				l.setError(lval, err)
				return _ERROR
			}
			l.setString(lval, str)
			// check if this is a compound string literal
			if l.matchNextRune('"', '\'') {
				l.inCompoundStringLiteral = true
				// continue reading
				continue
			}
			l.inCompoundStringLiteral = false
			l.insertSemi |= atNextNewline | onlyIfMissing | onlyIfLastTokenOnLine
			return _STRING_LIT
		}

		if c == '/' {
			// comment
			cn, szn, err := l.input.readRune()
			if err != nil {
				l.setRune(lval, '/')
				return int(c)
			}
			if cn == '/' {
				if hasErr := l.skipToEndOfLineComment(lval); hasErr {
					return _ERROR
				}
				l.comments = append(l.comments, l.newToken())
				continue
			}
			if cn == '*' {
				ok, hasErr := l.skipToEndOfBlockComment(lval)
				if hasErr {
					return _ERROR
				}
				if !ok {
					l.setError(lval, errors.New("block comment never terminates, unexpected EOF"))
					return _ERROR
				}
				l.comments = append(l.comments, l.newToken())
				continue
			}
			l.input.unreadRune(szn)
		}

		if c < 32 || c == 127 {
			l.setError(lval, errors.New("invalid control character"))
			return _ERROR
		}
		if !strings.ContainsRune(";,.:=-+(){}[]<>/", c) {
			l.setError(lval, errors.New("invalid character"))
			return _ERROR
		}
		l.setRune(lval, c)
		return int(c)
	}
}

func parseFloat(token string) (float64, error) {
	// strconv.ParseFloat allows _ to separate digits, but protobuf does not
	if strings.ContainsRune(token, '_') {
		return 0, &strconv.NumError{
			Func: "parseFloat",
			Num:  token,
			Err:  strconv.ErrSyntax,
		}
	}
	f, err := strconv.ParseFloat(token, 64)
	if err == nil {
		return f, nil
	}
	if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange && math.IsInf(f, 1) {
		// protoc doesn't complain about float overflow and instead just uses "infinity"
		// so we mirror that behavior by just returning infinity and ignoring the error
		return f, nil
	}
	return f, err
}

func (l *protoLex) newToken() ast.Token {
	offset := l.input.mark
	length := l.input.pos - l.input.mark
	return l.info.AddToken(offset, length)
}

func (l *protoLex) setPrevAndAddComments(n ast.TerminalNode) {
	comments := l.comments
	l.comments = nil
	var prevTrailingComments []ast.Token
	if l.prevSym != nil && len(comments) > 0 {
		prevEnd := l.info.NodeInfo(l.prevSym).End().Line
		info := l.info.NodeInfo(n)
		nStart := info.Start().Line
		if nStart == prevEnd {
			if rn, ok := n.(*ast.RuneNode); ok && rn.Rune == 0 {
				// if current token is EOF, pretend its on separate line
				// so that the logic below can attribute a final trailing
				// comment to the previous token
				nStart++
			}
		}
		c := comments[0]
		commentInfo := l.info.TokenInfo(c)
		commentStart := commentInfo.Start().Line
		if nStart > prevEnd && commentStart == prevEnd {
			// Comment starts right after the previous token. If it's a
			// line comment, we record that as a trailing comment.
			//
			// But if it's a block comment, it is only a trailing comment
			// if there are multiple comments or if the block comment ends
			// on a line before n.
			canDonate := strings.HasPrefix(commentInfo.RawText(), "//") ||
				len(comments) > 1 || commentInfo.End().Line < nStart

			if canDonate {
				prevTrailingComments = comments[:1]
				comments = comments[1:]
			}
		}
	}

	// now we can associate comments
	for _, c := range prevTrailingComments {
		l.info.AddComment(c, l.prevSym.Token())
	}

	if rn, ok := n.(*ast.RuneNode); ok && rn.Virtual {
		for _, c := range comments {
			l.info.AddVirtualComment(c, l.prevSym.Token(), n.Token())
		}
	} else {
		for _, c := range comments {
			l.info.AddComment(c, n.Token())
		}
	}

	l.prevSym = n
}

func (l *protoLex) setString(lval *protoSymType, val string) {
	node := ast.NewStringLiteralNode(val, l.newToken())
	if l.inCompoundStringLiteral && lval.sv != nil {
		lval.sv = ast.NewCompoundStringLiteralNode(lval.sv, node)
	} else {
		lval.sv = node
	}
	l.setPrevAndAddComments(node)
}

func (l *protoLex) setIdent(lval *protoSymType, val string) {
	lval.id = ast.NewIdentNode(val, l.newToken())
	l.setPrevAndAddComments(lval.id)
}

func (l *protoLex) setInt(lval *protoSymType, val uint64) {
	lval.i = ast.NewUintLiteralNode(val, l.newToken())
	l.setPrevAndAddComments(lval.i)
}

func (l *protoLex) setFloat(lval *protoSymType, val float64) {
	lval.f = ast.NewFloatLiteralNode(val, l.newToken())
	l.setPrevAndAddComments(lval.f)
}

func (l *protoLex) setRune(lval *protoSymType, val rune) {
	lval.b = ast.NewRuneNode(val, l.newToken())
	l.setPrevAndAddComments(lval.b)
}

func (l *protoLex) setVirtualRune(lval *protoSymType, val rune) {
	lval.b = ast.NewVirtualRuneNode(val, l.newToken())
	l.setPrevAndAddComments(lval.b)
}

func (l *protoLex) setError(lval *protoSymType, err error) {
	lval.err, _ = l.addSourceError(err)
}

func (l *protoLex) readNumber() {
	allowExpSign := false
	for {
		c, sz, err := l.input.readRune()
		if err != nil {
			break
		}
		if (c == '-' || c == '+') && !allowExpSign {
			l.input.unreadRune(sz)
			break
		}
		allowExpSign = false
		if c != '.' && c != '_' && (c < '0' || c > '9') &&
			(c < 'a' || c > 'z') && (c < 'A' || c > 'Z') &&
			c != '-' && c != '+' {
			// no more chars in the number token
			l.input.unreadRune(sz)
			break
		}
		if c == 'e' || c == 'E' {
			// scientific notation char can be followed by
			// an exponent sign
			allowExpSign = true
		}
	}
}

func numError(err error, kind, s string) error {
	ne, ok := err.(*strconv.NumError)
	if !ok {
		return err
	}
	if ne.Err == strconv.ErrRange {
		return fmt.Errorf("value out of range for %s: %s", kind, s)
	}
	// syntax error
	return fmt.Errorf("invalid syntax in %s value: %s", kind, s)
}

func (l *protoLex) readIdentifier() {
	for {
		c, sz, err := l.input.readRune()
		if err != nil {
			break
		}
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			l.input.unreadRune(sz)
			break
		}
	}
}

func (l *protoLex) readStringLiteral(quote rune) (string, error) {
	var buf bytes.Buffer
	var escapeError reporter.ErrorWithPos
	var noMoreErrors bool
	var errCount int
	reportErr := func(msg, badEscape string) {
		errCount++
		if noMoreErrors {
			return
		}
		if escapeError != nil {
			// report previous one
			_, ok := l.addSourceError(escapeError)
			if !ok {
				noMoreErrors = true
			}
		}
		var err error
		if strings.HasSuffix(msg, "%s") {
			err = fmt.Errorf(msg, badEscape)
		} else {
			err = errors.New(msg)
		}
		// we've now consumed the bad escape and lexer position is after it, so we need
		// to back up to the beginning of the escape to report the correct position
		escapeError = l.errWithCurrentPos(err, -len(badEscape))

		if errCount > 10 {
			noMoreErrors = true
		}
	}
	defer func() {
		if noMoreErrors && errCount > 10 {
			l.addSourceError(fmt.Errorf("too many errors (%d) encountered while parsing string literal", errCount))
		}
	}()
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return "", err
		}
		if c == '\n' {
			return "", errors.New("encountered end-of-line before end of string literal")
		}
		if c == quote {
			break
		}
		if c == 0 {
			reportErr("null character ('\\0') not allowed in string literal", string(rune(0)))
			continue
		}
		if c == '\\' {
			// escape sequence
			c, _, err = l.input.readRune()
			if err != nil {
				return "", err
			}
			switch {
			case c == 'x' || c == 'X':
				// hex escape
				c1, sz1, err := l.input.readRune()
				if err != nil {
					return "", err
				}
				if c1 == quote || c1 == '\\' {
					l.input.unreadRune(sz1)
					reportErr("invalid hex escape: %s", "\\"+string(c))
					continue
				}
				c2, sz2, err := l.input.readRune()
				if err != nil {
					return "", err
				}
				var hex string
				if (c2 < '0' || c2 > '9') && (c2 < 'a' || c2 > 'f') && (c2 < 'A' || c2 > 'F') {
					l.input.unreadRune(sz2)
					hex = string(c1)
				} else {
					hex = string([]rune{c1, c2})
				}
				i, err := strconv.ParseInt(hex, 16, 32)
				if err != nil {
					reportErr("invalid hex escape: %s", "\\"+string(c)+hex)
					continue
				}
				buf.WriteByte(byte(i))
			case c >= '0' && c <= '7':
				// octal escape
				c2, sz2, err := l.input.readRune()
				if err != nil {
					return "", err
				}
				var octal string
				if c2 < '0' || c2 > '7' {
					l.input.unreadRune(sz2)
					octal = string(c)
				} else {
					c3, sz3, err := l.input.readRune()
					if err != nil {
						return "", err
					}
					if c3 < '0' || c3 > '7' {
						l.input.unreadRune(sz3)
						octal = string([]rune{c, c2})
					} else {
						octal = string([]rune{c, c2, c3})
					}
				}
				i, err := strconv.ParseInt(octal, 8, 32)
				if err != nil {
					reportErr("invalid octal escape: %s", "\\"+octal)
					continue
				}
				if i > 0xff {
					reportErr("octal escape is out range, must be between 0 and 377: %s", "\\"+octal)
					continue
				}
				buf.WriteByte(byte(i))
			case c == 'u':
				// short unicode escape
				u := make([]rune, 4)
				for i := range u {
					c2, sz2, err := l.input.readRune()
					if err != nil {
						return "", err
					}
					if c2 == quote || c2 == '\\' {
						l.input.unreadRune(sz2)
						u = u[:i]
						break
					}
					u[i] = c2
				}
				codepointStr := string(u)
				if len(u) < 4 {
					reportErr("invalid unicode escape: %s", "\\u"+codepointStr)
					continue
				}
				i, err := strconv.ParseInt(codepointStr, 16, 32)
				if err != nil {
					reportErr("invalid unicode escape: %s", "\\u"+codepointStr)
					continue
				}
				buf.WriteRune(rune(i))
			case c == 'U':
				// long unicode escape
				u := make([]rune, 8)
				for i := range u {
					c2, sz2, err := l.input.readRune()
					if err != nil {
						return "", err
					}
					if c2 == quote || c2 == '\\' {
						l.input.unreadRune(sz2)
						u = u[:i]
						break
					}
					u[i] = c2
				}
				codepointStr := string(u)
				if len(u) < 8 {
					reportErr("invalid unicode escape: %s", "\\U"+codepointStr)
					continue
				}
				i, err := strconv.ParseInt(string(u), 16, 32)
				if err != nil {
					reportErr("invalid unicode escape: %s", "\\U"+codepointStr)
					continue
				}
				if i > 0x10ffff || i < 0 {
					reportErr("unicode escape is out of range, must be between 0 and 0x10ffff: %s", "\\U"+codepointStr)
					continue
				}
				buf.WriteRune(rune(i))
			case c == 'a':
				buf.WriteByte('\a')
			case c == 'b':
				buf.WriteByte('\b')
			case c == 'f':
				buf.WriteByte('\f')
			case c == 'n':
				buf.WriteByte('\n')
			case c == 'r':
				buf.WriteByte('\r')
			case c == 't':
				buf.WriteByte('\t')
			case c == 'v':
				buf.WriteByte('\v')
			case c == '\\':
				buf.WriteByte('\\')
			case c == '\'':
				buf.WriteByte('\'')
			case c == '"':
				buf.WriteByte('"')
			case c == '?':
				buf.WriteByte('?')
			default:
				reportErr("invalid escape sequence: %s", "\\"+string(c))
				continue
			}
		} else {
			buf.WriteRune(c)
		}
	}
	if escapeError != nil {
		return "", escapeError
	}
	return buf.String(), nil
}

func (l *protoLex) skipToEndOfLineComment(lval *protoSymType) (hasErr bool) {
	for {
		c, sz, err := l.input.readRune()
		if err != nil {
			// eof
			return false
		}
		switch c {
		case '\n':
			// don't include newline in the comment
			l.input.unreadRune(sz)
			return false
		case 0:
			l.setError(lval, errors.New("invalid control character"))
			return true
		}
	}
}

func (l *protoLex) skipToEndOfBlockComment(lval *protoSymType) (ok, hasErr bool) {
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			return false, false
		}
		if c == 0 {
			l.setError(lval, errors.New("invalid control character"))
			return false, true
		}
		l.maybeNewLine(c)
		if c == '*' {
			c, sz, err := l.input.readRune()
			if err != nil {
				return false, false
			}
			if c == '/' {
				return true, false
			}
			l.input.unreadRune(sz)
		}
	}
}

func (l *protoLex) skipToNextRune() (nextRune rune) {
LOOKAHEAD:
	for {
		cn, sz, err := l.input.readRune()
		if err != nil {
			break
		}
		switch cn {
		case '/':
			// hit a comment; skip to the end of it without lexing
			cn, _, err = l.input.readRune()
			if err != nil {
				break
			}
			if cn == '/' {
				// line comment
				readToEndOfLineComment(l.input)
			} else if cn == '*' {
				// block comment
				if !readToEndOfBlockComment(l.input) {
					// incomplete block comment
					break LOOKAHEAD
				}
			}
		case '\n', '\r', '\t', '\f', '\v', ' ':
			// skip whitespace
		default:
			nextRune = cn
			l.input.unreadRune(sz)
			break LOOKAHEAD
		}
	}
	return
}

func (l *protoLex) matchNextRune(targets ...rune) bool {
	var found bool
	l.input.save()
	defer l.input.restore()
	l.skipToNextRune()
	c, _, err := l.input.readRune()
	if err == nil {
		for _, t := range targets {
			if c == t {
				found = true
				break
			}
		}
	}
	return found
}

func (l *protoLex) peekNextIdent() (ident string, nextRune rune, ok bool) {
	l.input.save()
	defer l.input.restore()
	nextRune = l.skipToNextRune()
	mark := l.input.offset()
	l.readIdentifier()
	if l.input.offset() > mark {
		ok = true
		ident = string(l.input.data[mark:l.input.offset()])
	}
	return
}

func readToEndOfLineComment(input *runeReader) {
	for {
		c, sz, _ := input.readRune()
		switch c {
		case '\n':
			// don't include newline in the comment
			input.unreadRune(sz)
			return
		case 0:
			return
		}
	}
}

func readToEndOfBlockComment(input *runeReader) bool {
	for {
		c, _, err := input.readRune()
		if err != nil {
			return false
		}
		if c == '*' {
			c, sz, _ := input.readRune()
			if c == '/' {
				return true
			}
			input.unreadRune(sz)
		}
	}
}

func (l *protoLex) addSourceError(err error) (reporter.ErrorWithPos, bool) {
	ewp, ok := err.(reporter.ErrorWithPos)
	if !ok {
		// TODO: Store the previous span instead of just the position.
		ewp = reporter.Error(ast.NewSourceSpan(l.prev(), l.prev()), err)
	}
	handlerErr := l.handler.HandleError(ewp)
	return ewp, handlerErr == nil
}

func (l *protoLex) addSourceWarning(err error) {
	ewp, ok := err.(reporter.ErrorWithPos)
	if !ok {
		// TODO: Store the previous span instead of just the position.
		ewp = reporter.Error(ast.NewSourceSpan(l.prev(), l.prev()), err)
	}
	l.handler.HandleWarning(ewp)
}

func (l *protoLex) Error(s string) {
	_, _ = l.addSourceError(NewParseError(errors.New(s)))
}

func (l *protoLex) ErrExtendedSyntax(s string) {
	l.addSourceWarning(NewExtendedSyntaxError(fmt.Errorf("error: %s", s)))
}

// TODO: Accept both a start and end offset, and use that to create a span.
func (l *protoLex) errWithCurrentPos(err error, offset int) reporter.ErrorWithPos {
	if ewp, ok := err.(reporter.ErrorWithPos); ok {
		return ewp
	}
	pos := l.info.SourcePos(l.input.offset() + offset)
	return reporter.Error(ast.NewSourceSpan(pos, pos), err)
}

func (l *protoLex) maybeProcessPartialField(ident string) {
	// Determine if this ident terminates a partial field declaration, then
	// insert virtual semicolons as appropriate.
	// These are very difficult to parse correctly as the protobuf grammar makes
	// them ambiguous, so we must rely on heuristic rules.
	//
	// Form 1:
	//  message Foo {
	//    optional
	//    optional int32 bar = 1;
	//  }
	//
	// Form 2:
	//  message Foo {
	//    int32
	//    optional int32 bar = 1;
	//  }
	//
	// Form 3:
	//  message Foo {
	//    int32
	//    optional int32 bar = 1;
	//  }
	//
	// Form 3:
	//  message Foo {
	//    optional
	//    int32 bar = 1
	//  }

	// Forms 1 and 2 can be detected by analyzing the surrounding tokens to see
	// whether they would form a valid field declaration. Form 3 is impossible
	// to detect, as it is an actual valid field declaration.

	var canStartField bool
	if l.prevSym != nil {
		switch prev := l.prevSym.(type) {
		case *ast.RuneNode:
			if prev.Rune == '{' || prev.Rune == ';' {
				canStartField = true
			}
		}
	}

	if !canStartField {
		return
	}

	nextIdent, nextRune, ok := l.peekNextIdent()
	if !ok {
		if nextRune == '}' {
			// if next rune is '}', then we're at the end of a message
			// and can insert a semicolon
			l.insertSemi |= immediate | onlyIfMissing
		}
		return
	}

	var syntax string
	if l.res == nil || l.res.Syntax == nil {
		syntax = "proto2"
	} else {
		syntax = l.res.Syntax.Syntax.AsString()
	}

	matchKeyword := func(ident string) bool {
		switch syntax {
		case "proto2":
			switch ident {
			case "optional", "required", "repeated":
				return true
			}
		case "proto3":
			switch ident {
			case "optional", "repeated":
				return true
			}
		}
		return false
	}

	if matchKeyword(nextIdent) {
		l.insertSemi |= atNextNewline | onlyIfMissing
	}
}

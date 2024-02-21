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

// UnknownPos is a placeholder position when only the source file
// name is known.
func UnknownPos(filename string) SourcePos {
	return SourcePos{Filename: filename}
}

// unknownSpan is a placeholder span when only the source file
// name is known.
func UnknownSpan(filename string) SourceSpan {
	return unknownSpan{filename: filename}
}

type unknownSpan struct {
	filename string
}

func (n unknownSpan) Start() SourcePos {
	return UnknownPos(n.filename)
}

func (n unknownSpan) End() SourcePos {
	return UnknownPos(n.filename)
}

func (n unknownSpan) String() string {
	return n.filename
}

func (n *NoSourceNode) Start() Token {
	return TokenError
}

func (n *NoSourceNode) End() Token {
	return TokenError
}

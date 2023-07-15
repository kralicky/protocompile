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

package reporter

import (
	"errors"
	"fmt"

	"github.com/bufbuild/protocompile/ast"
)

// ErrInvalidSource is a sentinel error that is returned by compilation and
// stand-alone compilation steps (such as parsing, linking) when one or more
// errors is reported but the configured ErrorReporter always returns nil.
var ErrInvalidSource = errors.New("parse failed: invalid proto source")

// ErrorWithPos is an error about a proto source file that adds information
// about the location in the file that caused the error.
type ErrorWithPos interface {
	error
	// GetPosition returns the source position that caused the underlying error.
	GetPosition() ast.SourcePosInfo
	// Unwrap returns the underlying error.
	Unwrap() error
}

// Error creates a new ErrorWithPos from the given error and source position.
func Error(pos ast.SourcePosInfo, err error) ErrorWithPos {
	return errorWithSourcePos{pos: pos, underlying: err}
}

// Errorf creates a new ErrorWithPos whose underlying error is created using the
// given message format and arguments (via fmt.Errorf).
func Errorf(pos ast.SourcePosInfo, format string, args ...interface{}) ErrorWithPos {
	return errorWithSourcePos{pos: pos, underlying: fmt.Errorf(format, args...)}
}

type errorWithSourcePos struct {
	underlying error
	pos        ast.SourcePosInfo
}

func (e errorWithSourcePos) Error() string {
	sourcePos := e.GetPosition()
	return fmt.Sprintf("%s: %v", sourcePos, e.underlying)
}

func (e errorWithSourcePos) GetPosition() ast.SourcePosInfo {
	return e.pos
}

func (e errorWithSourcePos) Unwrap() error {
	return e.underlying
}

var _ ErrorWithPos = errorWithSourcePos{}

// Custom error types that contain additional information for each error.

type AlreadyDefinedError struct {
	isPkg              bool
	PreviousDefinition ast.SourcePosInfo
}

func AlreadyDefined(previousDefinition ast.SourcePosInfo) AlreadyDefinedError {
	return AlreadyDefinedError{
		PreviousDefinition: previousDefinition,
	}
}

func AlreadyDefinedAsPkg(previousDefinition ast.SourcePosInfo) AlreadyDefinedError {
	return AlreadyDefinedError{
		isPkg:              true,
		PreviousDefinition: previousDefinition,
	}
}

func (e AlreadyDefinedError) Error() string {
	var asPkg string
	if e.isPkg {
		asPkg = " as a package"
	}
	return fmt.Sprintf("already defined%s at %s", asPkg, e.PreviousDefinition)
}

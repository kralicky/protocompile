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
	"errors"
)

// ErrNoSyntax is a sentinel error that may be passed to a warning reporter.
// The error the reporter receives will be wrapped with source position that
// indicates the file that had no syntax statement.
var ErrNoSyntax = errors.New("no syntax specified; defaulting to proto2 syntax")

func NewParseError(base error) ParseError {
	return &parseError{
		base: base,
	}
}

type ParseError interface {
	error

	isParseError()
}

type parseError struct {
	base error
}

func (*parseError) isParseError() {}

func (e *parseError) Error() string {
	return e.base.Error()
}

func (e *parseError) Unwrap() error {
	return e.base
}

const (
	CategoryEmptyDecl      = "empty_decl"
	CategoryIncompleteDecl = "incomplete_decl"
	CategoryExtraTokens    = "extra_tokens"
	CategoryIncorrectToken = "wrong_token"
	CategoryMissingToken   = "missing_token"
	CategoryDeclNotAllowed = "decl_not_allowed"
)

func NewExtendedSyntaxError(base error, category string) ExtendedSyntaxError {
	return &extendedSyntaxError{
		base:     base,
		category: category,
	}
}

type ExtendedSyntaxError interface {
	error

	Category() string
	CanFormat() bool

	isExtendedSyntaxError()
}

type extendedSyntaxError struct {
	base     error
	category string
}

func (*extendedSyntaxError) isExtendedSyntaxError() {}

func (e *extendedSyntaxError) Error() string {
	return e.base.Error()
}

func (e *extendedSyntaxError) Unwrap() error {
	return e.base
}

func (e *extendedSyntaxError) Category() string {
	return e.category
}

func (e *extendedSyntaxError) CanFormat() bool {
	switch e.category {
	case CategoryEmptyDecl, CategoryIncorrectToken, CategoryMissingToken, CategoryExtraTokens:
		return true
	case CategoryIncompleteDecl:
		return false
	}
	panic("bug: CanFormat called with unknown category " + e.category)
}

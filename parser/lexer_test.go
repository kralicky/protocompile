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
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/reporter"
)

func TestLexer(t *testing.T) {
	t.Parallel()
	handler := reporter.NewHandler(nil)
	l := newTestLexer(t, strings.NewReader(`
	syntax = "proto2";
	// comment

	/*
	 * block comment
	 */ /* inline comment */

	message Foo {
		int32 name = 1 [opt= "\032\x16\n\rfoobar\"zap"		'another\tstring\'s\t']
  }

	// another comment
	// more and more...

	service Foo {
		rpc Foo(.type) returns (.f.q.n) {}
		rpc Foo(name) returns (f.q.n);
	}

	message Foo {
		.type a = 1 [option=foo]
		.f.q.n b = 2
		name c = 2
		f.q.n d = 4
		option a.(b.c).d = .01
		option a.(b.c). = .01e12
		option a.(b.c) = .01e+5
		option (a.b) = .033e-1
		option (a). = 12345
		option (a).(b) = -12345
		option (a.) = 123.1234
		option (a.b.) = 0.123
		option (a.b.c) = 012345
		option a.(b.).(c) = 0x2134abcdef30
		option a = -0543
		option b = -0xff76
		option a.(b).c.(d) = 101.0102
		option a.(b).c.(d). = 202.0203e1
		option a.(b).c. = 304.0304e-10
		option a.(b).c = 3.1234e+12
	}

	service Foo {
		rpc Foo (.) returns (.) { option (.) = . }
	}

	extend .google.protobuf.FieldOptions {
		optional uint64 foo = 1 [
			(syntax) = true
		]
	}

	message Foo {
		optional bool rpc = 2
		optional uint64 id = 1 [
			(syntax) = true
		];
	}

	message Simple {
		.
		optional .
		optional .g
		optional string name = 1;
	}

	// some strange cases
	1.543 g12 /* trailing inline comment */
	000.000
	0.1234 .5678 . // trailing line comment
	12e12 1.2345e123412341234

	Random_identifier_with_numbers_0123456789_and_letters...
	// this is a trailing comment
	// that spans multiple lines
	// over two in fact!
	/*
	 * this is a detached comment
	 * with lots of extra words and stuff...
	 */

	// this is an attached leading comment
	foo

	'abc 😊' /* this is not a trailing
	            comment because it ends on
	            same line as next token */ 'def 🙁'

	1.23e+20+20 // a trailing comment for last element

	// comment attached to no tokens (upcoming token is EOF!)
	/* another comment followed by some final whitespace*/


	`), handler)

	var prev ast.Node
	var sym protoSymType
	expected := []struct {
		t          int
		line, col  int
		span       int
		v          interface{}
		virtual    bool
		comments   []string
		trailCount int
	}{
		0: {t: _SYNTAX, v: "syntax"},
		1: {t: '=', v: '='},
		2: {t: _STRING_LIT, v: "proto2"},
		3: {t: ';', v: ';', virtual: false},

		4:  {t: _MESSAGE, v: "message"},
		5:  {t: _SINGULAR_IDENT, v: "Foo"},
		6:  {t: '{', v: '{'},
		7:  {t: _INT32, v: "int32"},
		8:  {t: _SINGULAR_IDENT, v: "name"},
		9:  {t: '=', v: '='},
		10: {t: _INT_LIT, v: uint64(1)},
		11: {t: '[', v: '['},
		12: {t: _SINGULAR_IDENT, v: "opt"},
		13: {t: '=', v: '='},
		14: {t: _STRING_LIT, v: "\032\x16\n\rfoobar\"zapanother\tstring's\t"},
		15: {t: ']', v: ']'},
		16: {t: ';', v: ';', virtual: true},
		17: {t: '}', v: '}'},
		18: {t: ';', v: ';', virtual: true},

		19: {t: _SERVICE, v: "service"},
		20: {t: _SINGULAR_IDENT, v: "Foo"},
		21: {t: '{', v: '{'},
		22: {t: _RPC, v: "rpc"},
		23: {t: _SINGULAR_IDENT, v: "Foo"},
		24: {t: '(', v: '('},
		25: {t: _FULLY_QUALIFIED_IDENT, v: ".type"},
		26: {t: ')', v: ')'},
		27: {t: _RETURNS, v: "returns"},
		28: {t: '(', v: '('},
		29: {t: _FULLY_QUALIFIED_IDENT, v: ".f.q.n"},
		30: {t: ')', v: ')'},
		31: {t: '{', v: '{'},
		32: {t: '}', v: '}'},
		33: {t: ';', v: ';', virtual: true},
		34: {t: _RPC, v: "rpc"},
		35: {t: _SINGULAR_IDENT, v: "Foo"},
		36: {t: '(', v: '('},
		37: {t: _SINGULAR_IDENT, v: "name"},
		38: {t: ')', v: ')'},
		39: {t: _RETURNS, v: "returns"},
		40: {t: '(', v: '('},
		41: {t: _QUALIFIED_IDENT, v: "f.q.n"},
		42: {t: ')', v: ')'},
		43: {t: ';', v: ';', virtual: false},
		44: {t: '}', v: '}'},
		45: {t: ';', v: ';', virtual: true},

		// message Foo {
		46: {t: _MESSAGE, v: "message"},
		47: {t: _SINGULAR_IDENT, v: "Foo"},
		48: {t: '{', v: '{'},
		// .type a = 1 [option=foo]
		49: {t: _FULLY_QUALIFIED_IDENT, v: ".type"},
		50: {t: _SINGULAR_IDENT, v: "a"},
		51: {t: '=', v: '='},
		52: {t: _INT_LIT, v: uint64(1)},
		53: {t: '[', v: '['},
		54: {t: _OPTION, v: "option"}, // this is lexed as a keyword token, but parsed as an identifier
		55: {t: '=', v: '='},
		56: {t: _SINGULAR_IDENT, v: "foo"},
		57: {t: ']', v: ']'},
		58: {t: ';', v: ';', virtual: true},
		// .f.q.n b = 2
		59: {t: _FULLY_QUALIFIED_IDENT, v: ".f.q.n"},
		60: {t: _SINGULAR_IDENT, v: "b"},
		61: {t: '=', v: '='},
		62: {t: _INT_LIT, v: uint64(2)},
		63: {t: ';', v: ';', virtual: true},
		// name c = 2
		64: {t: _SINGULAR_IDENT, v: "name"},
		65: {t: _SINGULAR_IDENT, v: "c"},
		66: {t: '=', v: '='},
		67: {t: _INT_LIT, v: uint64(2)},
		68: {t: ';', v: ';', virtual: true},
		// f.q.n d = 4
		69: {t: _QUALIFIED_IDENT, v: "f.q.n"},
		70: {t: _SINGULAR_IDENT, v: "d"},
		71: {t: '=', v: '='},
		72: {t: _INT_LIT, v: uint64(4)},
		73: {t: ';', v: ';', virtual: true},
		// option a.(b.c).d = .01
		74: {t: _OPTION, v: "option"},
		75: {t: _EXTENSION_IDENT, v: "a.(b.c).d"},
		76: {t: '=', v: '='},
		77: {t: _FLOAT_LIT, v: 0.01},
		78: {t: ';', v: ';', virtual: true},
		// option a.(b.c). = .01e12
		79: {t: _OPTION, v: "option"},
		80: {t: _EXTENSION_IDENT, v: "a.(b.c)."},
		81: {t: '=', v: '='},
		82: {t: _FLOAT_LIT, v: 0.01e12},
		83: {t: ';', v: ';', virtual: true},
		// option a.(b.c) = .01e+5
		84: {t: _OPTION, v: "option"},
		85: {t: _EXTENSION_IDENT, v: "a.(b.c)"},
		86: {t: '=', v: '='},
		87: {t: _FLOAT_LIT, v: 0.01e+5},
		88: {t: ';', v: ';', virtual: true},
		// option (a.b) = .033e-1
		89: {t: _OPTION, v: "option"},
		90: {t: _EXTENSION_IDENT, v: "(a.b)"},
		91: {t: '=', v: '='},
		92: {t: _FLOAT_LIT, v: 0.033e-1},
		93: {t: ';', v: ';', virtual: true},
		// option (a). = 12345
		94: {t: _OPTION, v: "option"},
		95: {t: _EXTENSION_IDENT, v: "(a)."},
		96: {t: '=', v: '='},
		97: {t: _INT_LIT, v: uint64(12345)},
		98: {t: ';', v: ';', virtual: true},
		// option (a).(b) = -12345
		99:  {t: _OPTION, v: "option"},
		100: {t: _EXTENSION_IDENT, v: "(a).(b)"},
		101: {t: '=', v: '='},
		102: {t: '-', v: '-'},
		103: {t: _INT_LIT, v: uint64(12345)},
		104: {t: ';', v: ';', virtual: true},
		// option (a.) = 123.1234
		105: {t: _OPTION, v: "option"},
		106: {t: _EXTENSION_IDENT, v: "(a.)"},
		107: {t: '=', v: '='},
		108: {t: _FLOAT_LIT, v: 123.1234},
		109: {t: ';', v: ';', virtual: true},
		// option (a.b.) = 0.123
		110: {t: _OPTION, v: "option"},
		111: {t: _EXTENSION_IDENT, v: "(a.b.)"},
		112: {t: '=', v: '='},
		113: {t: _FLOAT_LIT, v: 0.123},
		114: {t: ';', v: ';', virtual: true},
		// option (a.b.c) = 012345
		115: {t: _OPTION, v: "option"},
		116: {t: _EXTENSION_IDENT, v: "(a.b.c)"},
		117: {t: '=', v: '='},
		118: {t: _INT_LIT, v: uint64(0o12345)},
		119: {t: ';', v: ';', virtual: true},
		// option a.(b.).(c) = 0x2134abcdef30
		120: {t: _OPTION, v: "option"},
		121: {t: _EXTENSION_IDENT, v: "a.(b.).(c)"},
		122: {t: '=', v: '='},
		123: {t: _INT_LIT, v: uint64(0x2134abcdef30)},
		124: {t: ';', v: ';', virtual: true},
		// option a = -0543
		125: {t: _OPTION, v: "option"},
		126: {t: _SINGULAR_IDENT, v: "a"},
		127: {t: '=', v: '='},
		128: {t: '-', v: '-'},
		129: {t: _INT_LIT, v: uint64(0o543)},
		130: {t: ';', v: ';', virtual: true},
		// option b = -0xff76
		131: {t: _OPTION, v: "option"},
		132: {t: _SINGULAR_IDENT, v: "b"},
		133: {t: '=', v: '='},
		134: {t: '-', v: '-'},
		135: {t: _INT_LIT, v: uint64(0xff76)},
		136: {t: ';', v: ';', virtual: true},
		// option a.(b).c.(d) = 101.0102
		137: {t: _OPTION, v: "option"},
		138: {t: _EXTENSION_IDENT, v: "a.(b).c.(d)"},
		139: {t: '=', v: '='},
		140: {t: _FLOAT_LIT, v: 101.0102},
		141: {t: ';', v: ';', virtual: true},
		// option a.(b).c.(d). = 202.0203e1
		142: {t: _OPTION, v: "option"},
		143: {t: _EXTENSION_IDENT, v: "a.(b).c.(d)."},
		144: {t: '=', v: '='},
		145: {t: _FLOAT_LIT, v: 202.0203e1},
		146: {t: ';', v: ';', virtual: true},
		// option a.(b).c. = 304.0304e-10
		147: {t: _OPTION, v: "option"},
		148: {t: _EXTENSION_IDENT, v: "a.(b).c."},
		149: {t: '=', v: '='},
		150: {t: _FLOAT_LIT, v: 304.0304e-10},
		151: {t: ';', v: ';', virtual: true},
		// option a.(b).c = 3.1234e+12
		152: {t: _OPTION, v: "option"},
		153: {t: _EXTENSION_IDENT, v: "a.(b).c"},
		154: {t: '=', v: '='},
		155: {t: _FLOAT_LIT, v: 3.1234e+12},
		156: {t: ';', v: ';', virtual: true},
		// }
		157: {t: '}', v: '}'},
		158: {t: ';', v: ';', virtual: true},

		// service Foo {
		//   rpc Foo (.) returns (.) { option (.) = . }
		// }
		159: {t: _SERVICE, v: "service"},
		160: {t: _SINGULAR_IDENT, v: "Foo"},
		161: {t: '{', v: '{'},
		162: {t: _RPC, v: "rpc"},
		163: {t: _SINGULAR_IDENT, v: "Foo"},
		164: {t: '(', v: '('},
		165: {t: _FULLY_QUALIFIED_IDENT, v: "."},
		166: {t: ')', v: ')'},
		167: {t: _RETURNS, v: "returns"},
		168: {t: '(', v: '('},
		169: {t: _FULLY_QUALIFIED_IDENT, v: "."},
		170: {t: ')', v: ')'},
		171: {t: '{', v: '{'},
		172: {t: _OPTION, v: "option"},
		173: {t: _EXTENSION_IDENT, v: "(.)"},
		174: {t: '=', v: '='},
		175: {t: _FULLY_QUALIFIED_IDENT, v: "."},
		176: {t: ';', v: ';', virtual: true},
		177: {t: '}', v: '}'},
		178: {t: ';', v: ';', virtual: true},
		179: {t: '}', v: '}'},
		180: {t: ';', v: ';', virtual: true},

		// extend .google.protobuf.FieldOptions {
		//   optional uint64 foo = 1 [(syntax) = true]
		// }
		181: {t: _EXTEND, v: "extend"},
		182: {t: _FULLY_QUALIFIED_IDENT, v: ".google.protobuf.FieldOptions"},
		183: {t: '{', v: '{'},
		184: {t: _OPTIONAL, v: "optional"},
		185: {t: _UINT64, v: "uint64"},
		186: {t: _SINGULAR_IDENT, v: "foo"},
		187: {t: '=', v: '='},
		188: {t: _INT_LIT, v: uint64(1)},
		189: {t: '[', v: '['},
		190: {t: _EXTENSION_IDENT, v: "(syntax)"},
		191: {t: '=', v: '='},
		192: {t: _TRUE, v: "true"},
		193: {t: ']', v: ']'},
		194: {t: ';', v: ';', virtual: true},
		195: {t: '}', v: '}'},
		196: {t: ';', v: ';', virtual: true},

		// message Foo {
		//   optional bool rpc = 2
		//   optional uint64 id = 1 [(syntax) = true];
		// }
		197: {t: _MESSAGE, v: "message"},
		198: {t: _SINGULAR_IDENT, v: "Foo"},
		199: {t: '{', v: '{'},
		200: {t: _OPTIONAL, v: "optional"},
		201: {t: _BOOL, v: "bool"},
		202: {t: _RPC, v: "rpc"}, // this is lexed as a keyword token, but parsed as an identifier
		203: {t: '=', v: '='},
		204: {t: _INT_LIT, v: uint64(2)},
		205: {t: ';', v: ';', virtual: true},
		206: {t: _OPTIONAL, v: "optional"},
		207: {t: _UINT64, v: "uint64"},
		208: {t: _SINGULAR_IDENT, v: "id"},
		209: {t: '=', v: '='},
		210: {t: _INT_LIT, v: uint64(1)},
		211: {t: '[', v: '['},
		212: {t: _EXTENSION_IDENT, v: "(syntax)"},
		213: {t: '=', v: '='},
		214: {t: _TRUE, v: "true"},
		215: {t: ']', v: ']'},
		216: {t: ';', v: ';', virtual: false},
		217: {t: '}', v: '}'},
		218: {t: ';', v: ';', virtual: true},

		// message Simple {
		//   .
		//   optional .
		//   optional .g
		//   optional string name = 1;
		// }
		219: {t: _MESSAGE, v: "message"},
		220: {t: _SINGULAR_IDENT, v: "Simple"},
		221: {t: '{', v: '{'},
		222: {t: _FULLY_QUALIFIED_IDENT, v: "."},
		223: {t: ';', v: ';', virtual: true},
		{t: _OPTIONAL, v: "optional"},
		{t: _FULLY_QUALIFIED_IDENT, v: "."},
		{t: ';', v: ';', virtual: true},
		{t: _OPTIONAL, v: "optional"},
		{t: _FULLY_QUALIFIED_IDENT, v: ".g"},
		{t: ';', v: ';', virtual: true},
		{t: _OPTIONAL, v: "optional"},
		{t: _STRING, v: "string"},
		{t: _SINGULAR_IDENT, v: "name"},
		{t: '=', v: '='},
		{t: _INT_LIT, v: uint64(1)},
		{t: ';', v: ';', virtual: false},
		{t: '}', v: '}'},
		{t: ';', v: ';', virtual: true},

		// 0:  {t: _INT32, line: 8, col: 9, span: 5, v: "int32", comments: []string{"// comment", "/*\n\t * block comment\n\t */", "/* inline comment */"}},
		// 1:  {t: _STRING_LIT, line: 8, col: 16, span: 25, v: "\032\x16\n\rfoobar\"zap"},
		// 2:  {t: _STRING_LIT, line: 8, col: 57, span: 22, v: "another\tstring's\t"},
		// 3:  {t: _NAME, line: 9, col: 1, span: 3, v: "foo"},
		// 4:  {t: _SERVICE, line: 14, col: 9, span: 7, v: "service", comments: []string{"// another comment", "// more and more..."}},
		// 5:  {t: _RPC, line: 14, col: 17, span: 3, v: "rpc"},
		// 6:  {t: _MESSAGE, line: 14, col: 21, span: 7, v: "message"},
		// 7:  {t: '.', line: 15, col: 9, span: 1},
		// 8:  {t: _NAME, line: 15, col: 10, span: 4, v: "type"},
		// 9:  {t: '.', line: 16, col: 9, span: 1},
		// 10: {t: _NAME, line: 16, col: 10, span: 1, v: "f"},
		// 11: {t: '.', line: 16, col: 11, span: 1},
		// 12: {t: _NAME, line: 16, col: 12, span: 1, v: "q"},
		// 13: {t: '.', line: 16, col: 13, span: 1},
		// 14: {t: _NAME, line: 16, col: 14, span: 1, v: "n"},
		// 15: {t: _NAME, line: 17, col: 9, span: 4, v: "name"},
		// 16: {t: _NAME, line: 18, col: 9, span: 1, v: "f"},
		// 17: {t: '.', line: 18, col: 10, span: 1},
		// 18: {t: _NAME, line: 18, col: 11, span: 1, v: "q"},
		// 19: {t: '.', line: 18, col: 12, span: 1},
		// 20: {t: _NAME, line: 18, col: 13, span: 1, v: "n"},
		// 21: {t: _FLOAT_LIT, line: 20, col: 9, span: 3, v: 0.01},
		// 22: {t: _FLOAT_LIT, line: 21, col: 9, span: 6, v: 0.01e12},
		// 23: {t: _FLOAT_LIT, line: 22, col: 9, span: 6, v: 0.01e5},
		// 24: {t: _FLOAT_LIT, line: 23, col: 9, span: 7, v: 0.033e-1},
		// 25: {t: _INT_LIT, line: 25, col: 9, span: 5, v: uint64(12345)},
		// 26: {t: '-', line: 26, col: 9, span: 1, v: nil},
		// 27: {t: _INT_LIT, line: 26, col: 10, span: 5, v: uint64(12345)},
		// 28: {t: _FLOAT_LIT, line: 27, col: 9, span: 8, v: 123.1234},
		// 29: {t: _FLOAT_LIT, line: 28, col: 9, span: 5, v: 0.123},
		// 30: {t: _INT_LIT, line: 29, col: 9, span: 6, v: uint64(0o12345)},
		// 31: {t: _INT_LIT, line: 30, col: 9, span: 14, v: uint64(0x2134abcdef30)},
		// 32: {t: '-', line: 31, col: 9, span: 1, v: nil},
		// 33: {t: _INT_LIT, line: 31, col: 10, span: 4, v: uint64(0o543)},
		// 34: {t: '-', line: 32, col: 9, span: 1, v: nil},
		// 35: {t: _INT_LIT, line: 32, col: 10, span: 6, v: uint64(0xff76)},
		// 36: {t: _FLOAT_LIT, line: 33, col: 9, span: 8, v: 101.0102},
		// 37: {t: _FLOAT_LIT, line: 34, col: 9, span: 10, v: 202.0203e1},
		// 38: {t: _FLOAT_LIT, line: 35, col: 9, span: 12, v: 304.0304e-10},
		// 39: {t: _FLOAT_LIT, line: 36, col: 9, span: 10, v: 3.1234e+12},
		// 40: {t: '{', line: 38, col: 9, span: 1, v: nil},
		// 41: {t: '}', line: 38, col: 11, span: 1, v: nil},
		// 42: {t: '+', line: 38, col: 13, span: 1, v: nil},
		// 43: {t: '-', line: 38, col: 15, span: 1, v: nil},
		// 44: {t: ',', line: 38, col: 17, span: 1, v: nil},
		// 45: {t: ';', line: 38, col: 19, span: 1, v: nil},
		// 46: {t: '[', line: 40, col: 9, span: 1, v: nil},
		// 47: {t: _OPTION, line: 40, col: 10, span: 6, v: "option"},
		// 48: {t: '=', line: 40, col: 16, span: 1, v: nil},
		// 49: {t: _NAME, line: 40, col: 17, span: 3, v: "foo"},
		// 50: {t: ']', line: 40, col: 20, span: 1, v: nil},
		// 51: {t: _SYNTAX, line: 41, col: 9, span: 6, v: "syntax"},
		// 52: {t: '=', line: 41, col: 16, span: 1, v: nil},
		// 53: {t: _STRING_LIT, line: 41, col: 18, span: 8, v: "proto2"},
		// 54: {t: ';', line: 41, col: 26, span: 1, v: nil},
		// 55: {t: _FLOAT_LIT, line: 44, col: 9, span: 5, v: 1.543, comments: []string{"// some strange cases"}},
		// 56: {t: _NAME, line: 44, col: 15, span: 3, v: "g12"},
		// 57: {t: _FLOAT_LIT, line: 45, col: 9, span: 7, v: 0.0, comments: []string{"/* trailing inline comment */"}, trailCount: 1},
		// 58: {t: _FLOAT_LIT, line: 46, col: 9, span: 6, v: 0.1234},
		// 59: {t: _FLOAT_LIT, line: 46, col: 16, span: 5, v: 0.5678},
		// 60: {t: '.', line: 46, col: 22, span: 1, v: nil},
		// 61: {t: _FLOAT_LIT, line: 47, col: 9, span: 5, v: 12e12, comments: []string{"// trailing line comment"}, trailCount: 1},
		// 62: {t: _FLOAT_LIT, line: 47, col: 15, span: 19, v: math.Inf(1)},
		// 63: {t: _NAME, line: 49, col: 9, span: 53, v: "Random_identifier_with_numbers_0123456789_and_letters"},
		// 64: {t: '.', line: 49, col: 62, span: 1, v: nil},
		// 65: {t: '.', line: 49, col: 63, span: 1, v: nil},
		// 66: {t: '.', line: 49, col: 64, span: 1, v: nil},
		// 67: {t: _NAME, line: 59, col: 9, span: 3, v: "foo", comments: []string{"// this is a trailing comment", "// that spans multiple lines", "// over two in fact!", "/*\n\t * this is a detached comment\n\t * with lots of extra words and stuff...\n\t */", "// this is an attached leading comment"}},
		// 68: {t: _STRING_LIT, line: 61, col: 9, span: 7, v: "abc 😊"},
		// 69: {t: _STRING_LIT, line: 63, col: 48, span: 7, v: "def 🙁", comments: []string{"/* this is not a trailing\n\t            comment because it ends on\n\t            same line as next token */"}},
		// 70: {t: _FLOAT_LIT, line: 65, col: 9, span: 8, v: 1.23e+20},
		// 71: {t: '+', line: 65, col: 17, span: 1, v: nil},
		// 72: {t: _INT_LIT, line: 65, col: 18, span: 2, v: uint64(20)},

	}

	for i, exp := range expected {
		tok := l.Lex(&sym)
		if tok == 0 {
			t.Fatalf("lexer reported EOF but should have returned %v", exp)
		}
		var val any
		switch tok {
		case _SYNTAX, _OPTION, _INT32, _SERVICE, _RPC, _MESSAGE, _RETURNS, _EXTEND, _OPTIONAL, _UINT64, _TRUE, _FALSE, _BOOL, _STRING:
			val = sym.id.Val
		case _SINGULAR_IDENT:
			val = sym.id.Val
		case _QUALIFIED_IDENT, _FULLY_QUALIFIED_IDENT:
			val = string(sym.idv.AsIdentifier())
		case _EXTENSION_IDENT:
			val = stringForOptionName(sym.optName)
		case _INT_LIT:
			val = sym.i.Val
		case _FLOAT_LIT:
			val = sym.f.Val
		case _STRING_LIT:
			val = sym.sv.AsString()
		case ';':
			if exp.t != ';' {
				break // handle this error below
			}
			if !assert.Equal(t, exp.virtual, sym.b.Virtual, "case %d: wrong virtual flag", i) {
				return
			}
			fallthrough
		case '=', ':', '{', '}', '\\', '/', '?', ',', '>', '<', '+', '-', '(', ')', '[', ']', '*', '&', '^', '%', '$', '#', '@', '!', '~', '`':
			if rn, ok := exp.v.(rune); ok {
				exp.v = string(rn)
			}
			val = string(sym.b.Rune)
		case _ERROR:
			assert.FailNow(t, fmt.Sprintf("case %d: error: %v", i, sym.err))
		default:
			assert.FailNow(t, fmt.Sprintf("case %d: unexpected token type: %d", i, tok))
		}
		if !assert.Equal(t, exp.t, tok, "case %d: wrong token type (expecting %#v, got %#v)", i, exp.v, val) {
			return
		}
		if !assert.Equal(t, exp.v, val, "case %d: wrong token value", i) {
			return
		}
	}

	return

	for i, exp := range expected {
		tok := l.Lex(&sym)
		if tok == 0 {
			t.Fatalf("lexer reported EOF but should have returned %v", exp)
		}
		var n ast.Node
		var val interface{}
		switch tok {
		case _SYNTAX, _OPTION, _INT32, _SERVICE, _RPC, _MESSAGE, _SINGULAR_IDENT:
			n = sym.id
			val = sym.id.Val
		case _QUALIFIED_IDENT, _FULLY_QUALIFIED_IDENT:
			n = sym.idv
			val = string(sym.idv.AsIdentifier())
		case _EXTENSION_IDENT:
			n = sym.optName
			val = sym.optName
		case _STRING_LIT:
			n = sym.sv
			val = sym.sv.AsString()
		case _INT_LIT:
			n = sym.i
			val = sym.i.Val
		case _FLOAT_LIT:
			n = sym.f
			val = sym.f.Val
		case _ERROR:
			val = sym.err
		default:
			n = sym.b
			val = nil
		}
		if !assert.Equal(t, exp.t, tok, "case %d: wrong token type (expecting %+v, got %+v)", i, exp.v, val) {
			break
		}
		if !assert.Equal(t, exp.v, val, "case %d: wrong token value", i) {
			break
		}
		nodeInfo := l.info.NodeInfo(n)
		var prevNodeInfo ast.NodeInfo
		if prev != nil {
			prevNodeInfo = l.info.NodeInfo(prev)
		}
		assert.Equal(t, exp.line, nodeInfo.Start().Line, "case %d: wrong line number", i)
		assert.Equal(t, exp.col, nodeInfo.Start().Col, "case %d: wrong column number (on line %d)", i, exp.line)
		assert.Equal(t, exp.line, nodeInfo.End().Line, "case %d: wrong end line number", i)
		assert.Equal(t, exp.col+exp.span, nodeInfo.End().Col, "case %d: wrong end column number", i)
		actualTrailCount := 0
		if prev != nil {
			actualTrailCount = prevNodeInfo.TrailingComments().Len()
		}
		assert.Equal(t, exp.trailCount, actualTrailCount, "case %d: wrong number of trailing comments", i)
		assert.Equal(t, len(exp.comments)-exp.trailCount, nodeInfo.LeadingComments().Len(), "case %d: wrong number of comments", i)
		for ci := range exp.comments {
			var c ast.Comment
			if ci < exp.trailCount {
				if assert.Less(t, ci, prevNodeInfo.TrailingComments().Len(), "missing comment") {
					c = prevNodeInfo.TrailingComments().Index(ci)
				} else {
					continue
				}
			} else {
				if assert.Less(t, ci-exp.trailCount, nodeInfo.LeadingComments().Len(), "missing comment") {
					c = nodeInfo.LeadingComments().Index(ci - exp.trailCount)
				} else {
					continue
				}
			}
			assert.Equal(t, exp.comments[ci], c.RawText(), "case %d, comment #%d: unexpected text", i, ci+1)
		}
		prev = n
	}
	if tok := l.Lex(&sym); tok != 0 {
		t.Fatalf("lexer reported symbol after what should have been EOF: %d", tok)
	}
	require.NoError(t, handler.Error())
	// Now we check final state of lexer for unattached comments and final whitespace
	// One of the final comments get associated as trailing comment for final token
	prevNodeInfo := l.info.NodeInfo(prev)
	assert.Equal(t, 1, prevNodeInfo.TrailingComments().Len(), "last token: wrong number of trailing comments")
	eofNodeInfo := l.info.TokenInfo(l.eof)
	finalComments := eofNodeInfo.LeadingComments()
	if assert.Equal(t, 2, finalComments.Len(), "wrong number of final remaining comments") {
		assert.Equal(t, "// comment attached to no tokens (upcoming token is EOF!)", finalComments.Index(0).RawText(), "incorrect final comment text")
		assert.Equal(t, "/* another comment followed by some final whitespace*/", finalComments.Index(1).RawText(), "incorrect final comment text")
	}
	assert.Equal(t, "\n\n\t\n\t", eofNodeInfo.LeadingWhitespace(), "incorrect final whitespace")
}

func TestLexerErrors(t *testing.T) {
	t.Parallel()
	testCases := map[string]struct {
		input       string
		expectedErr string
	}{
		"int_hex_out_of_range": {
			input:       `0x10000000000000000`,
			expectedErr: "value out of range for hexadecimal integer",
		},
		"int_octal_out_of_range": {
			input:       `02000000000000000000000`,
			expectedErr: "value out of range for octal integer",
		},
		"str_incomplete": {
			input:       `"foobar`,
			expectedErr: "unexpected EOF",
		},
		"str_invalid_escape": {
			input:       `"foobar\J"`,
			expectedErr: "invalid escape sequence",
		},
		"str_invalid_hex_escape": {
			input:       `"foobar\xgfoo"`,
			expectedErr: "invalid hex escape",
		},
		"str_invalid_short_unicode_escape": {
			input:       `"foobar\u09gafoo"`,
			expectedErr: "invalid unicode escape",
		},
		"str_invalid_long_unicode_escape": {
			input:       `"foobar\U0010005zfoo"`,
			expectedErr: "invalid unicode escape",
		},
		"str_unicode_out_of_range": {
			input:       `"foobar\U00110000foo"`,
			expectedErr: "unicode escape is out of range",
		},
		"str_w_newline": {
			input:       "'foobar\nbaz'",
			expectedErr: "encountered end-of-line",
		},
		"str_w_null": {
			input:       "'foobar\000baz'",
			expectedErr: "null character ('\\0') not allowed",
		},
		"float_w_wrong_exp": {
			input:       `1.543g12`,
			expectedErr: "invalid syntax",
		},
		"float_w_multiple_points": {
			input:       `0.1234.5678.`,
			expectedErr: "invalid syntax",
		},
		"int_hex_w_point": {
			input:       `0x987.345aaf`,
			expectedErr: "invalid syntax",
		},
		"float_w_multiple_points2": {
			input:       `0.987.345`,
			expectedErr: "invalid syntax",
		},
		"float_w_two_exp": {
			input:       `0.987e34e-20`,
			expectedErr: "invalid syntax",
		},
		"float_w_two_exp2": {
			input:       `0.987e-345e20`,
			expectedErr: "invalid syntax",
		},
		"range_no_spaces": {
			input:       `.987to123`,
			expectedErr: "invalid syntax",
		},
		"int_binary": {
			input:       `0b0111`,
			expectedErr: "invalid syntax",
		},
		"int_octal_incorrect": {
			input:       `0o765432`,
			expectedErr: "invalid syntax",
		},
		"int_w_separators": {
			input:       `1_000_000`,
			expectedErr: "invalid syntax",
		},
		"float_w_separators": {
			input:       `1_000.000_001e6`,
			expectedErr: "invalid syntax",
		},
		"int_hex_invalid": {
			input:       `0X1F_FFP-16`,
			expectedErr: "invalid syntax",
		},
		"int_octal_invalid": {
			input:       "09",
			expectedErr: "invalid syntax in octal integer value: 09",
		},
		"float_f_suffix": {
			input:       "0f",
			expectedErr: "invalid syntax in octal integer value: 0f",
		},
		"block_comment_incomplete": {
			input:       `/* foobar`,
			expectedErr: "unexpected EOF",
		},
		"invalid_char_null": {
			input:       "\x00",
			expectedErr: "invalid control character",
		},
		"invalid_char": {
			input:       "\x03",
			expectedErr: "invalid control character",
		},
		"invalid_char2": {
			input:       "\x1B",
			expectedErr: "invalid control character",
		},
		"invalid_char3": {
			input:       "\x7F",
			expectedErr: "invalid control character",
		},
		"invalid_char4": {
			input:       "#",
			expectedErr: "invalid character",
		},
		"invalid_char5": {
			input:       "?",
			expectedErr: "invalid character",
		},
		"invalid_char6": {
			input:       "^",
			expectedErr: "invalid character",
		},
		"invalid_char7": {
			input:       "\uAAAA",
			expectedErr: "invalid character",
		},
		"invalid_char8": {
			input:       "\U0010FFFF",
			expectedErr: "invalid character",
		},
		"block_comment_w_null": {
			input:       "// foo \x00",
			expectedErr: "invalid control character",
		},
		"line_comment_w_null": {
			input:       "/* foo \x00",
			expectedErr: "invalid control character",
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			handler := reporter.NewHandler(nil)
			l := newTestLexer(t, strings.NewReader(tc.input), handler)
			var sym protoSymType
			tok := l.Lex(&sym)
			if assert.Equal(t, _ERROR, tok) {
				require.ErrorContains(t, sym.err, tc.expectedErr, "expected message to contain %q but does not: %q", tc.expectedErr, sym.err.Error())
			}
		})
	}
}

func TestStringLiteralMultipleErrors(t *testing.T) {
	t.Parallel()
	testCases := map[string]struct {
		input        string
		expectedErrs []string
	}{
		"null_char_and_bad_escapes": {
			input: "'foo \x00 \\L bar \\. baz \\+ \\[ buzz'",
			expectedErrs: []string{
				`test.proto:1:6: null character ('\0') not allowed in string literal`,
				`test.proto:1:8: invalid escape sequence: \L`,
				`test.proto:1:15: invalid escape sequence: \.`,
				`test.proto:1:22: invalid escape sequence: \+`,
				`test.proto:1:25: invalid escape sequence: \[`,
			},
		},
		"bad_hex_and_octal_escapes": {
			input: `" \xg \xg5 \X \X_ \477 \080 "`,
			expectedErrs: []string{
				`test.proto:1:3: invalid hex escape: \xg`,
				`test.proto:1:7: invalid hex escape: \xg5`,
				`test.proto:1:12: invalid hex escape: \X `,
				`test.proto:1:15: invalid hex escape: \X_`,
				`test.proto:1:19: octal escape is out range, must be between 0 and 377: \477`,
			},
		},
		"bad_unicode_escapes": {
			input: `" \u12 \ughij \ufgfg \U0010ffff \U10101010 \U0000FFAX "`,
			expectedErrs: []string{
				`test.proto:1:3: invalid unicode escape: \u12 `,
				`test.proto:1:8: invalid unicode escape: \ughij`,
				`test.proto:1:15: invalid unicode escape: \ufgfg`,
				`test.proto:1:33: unicode escape is out of range, must be between 0 and 0x10ffff: \U10101010`,
				`test.proto:1:44: invalid unicode escape: \U0000FFAX`,
			},
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var errors []reporter.ErrorWithPos
			handler := reporter.NewHandler(reporter.NewReporter(
				func(err reporter.ErrorWithPos) error {
					errors = append(errors, err)
					return nil
				},
				nil,
			))
			// add an extra integer literal *after* the bad string literal, to make sure we can tokenize it
			l := newTestLexer(t, strings.NewReader(tc.input+" 0"), handler)
			var sym protoSymType
			tok := l.Lex(&sym)
			require.Equal(t, _ERROR, tok)
			require.Equal(t, len(tc.expectedErrs), len(errors))
			require.Equal(t, errors[len(errors)-1], sym.err) // returned err in symbol should be last error
			for i := range tc.expectedErrs {
				assert.Equal(t, tc.expectedErrs[i], errors[i].Error(), "error#%d", i+1)
			}
			// make sure we can successfully tokenize the subsequent token
			tok = l.Lex(&sym)
			require.Equal(t, _INT_LIT, tok)
			require.Equal(t, uint64(0), sym.i.Val)
		})
	}
}

func newTestLexer(t *testing.T, in io.Reader, h *reporter.Handler) *protoLex {
	lexer, err := newLexer(in, "test.proto", h, 0)
	require.NoError(t, err)
	return lexer
}

func TestUTF8(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		data      string
		expectVal string
		succeeds  bool
	}{
		{
			data:      "'😊'",
			expectVal: "😊",
			succeeds:  true,
		},
		{
			data:      "'\xff\x80'",
			expectVal: "��", // replaces bad encoding bytes w/ replacement char
			succeeds:  true, // TODO: should be false if enforcing valid UTF8
		},
	}
	for _, tc := range testCases {
		handler := reporter.NewHandler(nil)
		l := newTestLexer(t, strings.NewReader(tc.data), handler)
		var sym protoSymType
		tok := l.Lex(&sym)
		if !tc.succeeds {
			assert.Equal(t, _ERROR, tok, "lexer should return error for %v", tc.data)
		} else if assert.Equal(t, _STRING_LIT, tok, "lexer should return string literal token for %v", tc.data) {
			assert.Equal(t, tc.expectVal, sym.sv)
		}
	}
}

func TestCompactOptionsLeadingComments(t *testing.T) {
	t.Parallel()
	contents := `
syntax = "proto2";

package testprotos;

import "google/protobuf/descriptor.proto";

extend google.protobuf.FieldOptions {
  // Leading comment on custom.
  optional int32 custom = 20000;
}

message Foo {
  // Leading comment on one.
  optional string one = 1 [
    // Leading comment on deprecated.
    deprecated = true,
    // Leading comment on (custom).
    (custom) = 2
  ];

  // Leading comment on two.
  optional string two = 2;
  // Leading comment on three.
  optional string three = 3;
}`

	fileNode, err := Parse("test.proto", strings.NewReader(contents), reporter.NewHandler(nil), 0)
	require.NoError(t, err)
	require.NoError(
		t,
		ast.Walk(
			fileNode,
			&ast.SimpleVisitor{
				DoVisitFieldReferenceNode: func(fieldReferenceNode *ast.FieldReferenceNode) error {
					// We're only testing compact options, so we can confidently
					// retrieve the leading comments from the FieldReference's name
					// since it will always be a terminal *IdentNode unless the
					// field reference has a '('.
					info := fileNode.NodeInfo(fieldReferenceNode.Name)
					if fieldReferenceNode.Open != nil {
						// The leading comments will be attached to the '(', if one exists.
						info = fileNode.NodeInfo(fieldReferenceNode.Open)
					}
					name := stringForFieldReference(fieldReferenceNode)
					if assert.Equal(t, 1, info.LeadingComments().Len(), "%s should have a leading comment", name) {
						assert.Equal(
							t,
							fmt.Sprintf("// Leading comment on %s.", name),
							info.LeadingComments().Index(0).RawText(),
						)
					}
					return nil
				},
				DoVisitFieldNode: func(fieldNode *ast.FieldNode) error {
					// The fields in these tests always define a label,
					// so we can confidently use it to retrieve the comments.
					info := fileNode.NodeInfo(fieldNode.Label)
					name := fieldNode.Name.Val
					if assert.Equal(t, 1, info.LeadingComments().Len(), "%s should have a leading comment", name) {
						assert.Equal(
							t,
							fmt.Sprintf("// Leading comment on %s.", name),
							info.LeadingComments().Index(0).RawText(),
						)
					}
					return nil
				},
			},
		),
	)
}

// stringForFieldReference returns the string representation of the
// given field reference.
func stringForFieldReference(fieldReference *ast.FieldReferenceNode) string {
	var result string
	if fieldReference.Open != nil {
		result += string(fieldReference.Open.Rune)
	}
	result += string(fieldReference.Name.AsIdentifier())
	if fieldReference.Close != nil {
		result += string(fieldReference.Close.Rune)
	}
	return result
}

// stringForOptionName returns the string representation of the given option name node.
func stringForOptionName(optionNameNode *ast.OptionNameNode) string {
	var result string
	var nodes []ast.Node
	for _, p := range optionNameNode.Parts {
		nodes = append(nodes, p)
	}
	for _, p := range optionNameNode.Dots {
		nodes = append(nodes, p)
	}
	slices.SortFunc(nodes, func(i, j ast.Node) int {
		return cmp.Compare(i.Start(), j.Start())
	})
	for _, node := range nodes {
		switch node := node.(type) {
		case *ast.FieldReferenceNode:
			result += stringForFieldReference(node)
		case *ast.RuneNode:
			result += string(node.Rune)
		}
	}
	return result
}

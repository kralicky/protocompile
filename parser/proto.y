%{
package parser

//lint:file-ignore SA4006 generated parser has unused values

import (
	"math"

	"github.com/kralicky/protocompile/ast"
)

%}

// fields inside this union end up as the fields in a structure known
// as ${PREFIX}SymType, of which a reference is passed to the lexer.
%union{
	file         *ast.FileNode
	syn          *ast.SyntaxNode
	ed           *ast.EditionNode
	fileElement  ast.FileElement
	fileElements []ast.FileElement
	pkg          *ast.PackageNode
	imprt        *ast.ImportNode
	msg          *ast.MessageNode
	msgElement   ast.MessageElement
	msgElements  []ast.MessageElement
	fld          *ast.FieldNode
	mapFld       *ast.MapFieldNode
	mapType      *ast.MapTypeNode
	grp          *ast.GroupNode
	oo           *ast.OneofNode
	ooElement    ast.OneofElement
	ooElements   []ast.OneofElement
	ext          *ast.ExtensionRangeNode
	resvd        *ast.ReservedNode
	en           *ast.EnumNode
	enElement    ast.EnumElement
	enElements   []ast.EnumElement
	env          *ast.EnumValueNode
	extend       *ast.ExtendNode
	extElement   ast.ExtendElement
	extElements  []ast.ExtendElement
	svc          *ast.ServiceNode
	svcElement   ast.ServiceElement
	svcElements  []ast.ServiceElement
	mtd          *ast.RPCNode
	mtdMsgType   *ast.RPCTypeNode
	mtdElement   ast.RPCElement
	mtdElements  []ast.RPCElement
	opt          *ast.OptionNode
	opts         *compactOptionSlices
	ref          *ast.FieldReferenceNode
	refp         *fieldRefParens
	optName      *ast.OptionNameNode
	cmpctOpts    *ast.CompactOptionsNode
	rng          *ast.RangeNode
	rngs         *rangeSlices
	names        *nameSlices
	cid          *identSlices
	xid          *identSlices
	idv          ast.IdentValueNode
	sl           *valueSlices
	msgLitFlds   *messageFieldList
	msgLitFld    *ast.MessageFieldNode
	v            ast.ValueNode
	il           ast.IntValueNode
	sv           ast.StringValueNode
	i            *ast.UintLiteralNode
	f            *ast.FloatLiteralNode
	id           *ast.IdentNode
	b            *ast.RuneNode
	err          error
}

// any non-terminal which returns a value needs a type, which is
// really a field name in the above union struct
%type <file>         file
%type <syn>          syntaxDecl
%type <ed>           editionDecl
%type <fileElement>  fileElement
%type <fileElements> fileElements
%type <imprt>        importDecl
%type <pkg>          packageDecl
%type <opt>          optionDecl compactOption
%type <opts>         compactOptionDecls
%type <ref>          messageLiteralFieldName
%type <optName>      optionName
%type <cmpctOpts>    compactOptions
%type <v>            value optionValue compactOptionValue scalarValue messageLiteralWithBraces messageLiteral numLit listLiteral listElement listOfMessagesLiteral messageValue
%type <il>           enumValueNumber
%type <id>           singularIdent identKeywordName mapKeyType fieldCardinality msgElementKeywordName extElementKeywordName oneofElementKeywordName notGroupElementKeywordName mtdElementKeywordName enumValueName enumValueKeywordName
%type <idv>          anyIdentifier msgElementTypeIdent extElementTypeIdent oneofElementTypeIdent notGroupElementTypeIdent mtdElementTypeIdent
%type <sl>           listElements messageLiterals
%type <msgLitFlds>   messageLiteralFieldEntry messageLiteralFields messageTextFormat
%type <msgLitFld>    messageLiteralField
%type <fld>          messageFieldDecl oneofFieldDecl extensionFieldDecl
%type <oo>           oneofDecl
%type <grp>          groupDecl oneofGroupDecl
%type <mapFld>       mapFieldDecl
%type <mapType>      mapType
%type <msg>          messageDecl
%type <msgElement>   messageElement
%type <msgElements>  messageElements messageBody
%type <ooElement>    oneofElement
%type <ooElements>   oneofElements oneofBody
%type <names>        fieldNameStrings fieldNameIdents
%type <resvd>        msgReserved enumReserved reservedNames
%type <rng>          tagRange enumValueRange
%type <rngs>         tagRanges enumValueRanges
%type <ext>          extensionRangeDecl
%type <en>           enumDecl
%type <enElement>    enumElement
%type <enElements>   enumElements enumBody
%type <env>          enumValueDecl
%type <extend>       extensionDecl
%type <extElement>   extensionElement
%type <extElements>  extensionElements extensionBody
%type <svc>          serviceDecl
%type <svcElement>   serviceElement
%type <svcElements>  serviceElements serviceBody
%type <mtd>          methodDecl methodWithBodyDecl
%type <mtdElement>   methodElement
%type <mtdElements>  methodElements methodBody
%type <mtdMsgType>   methodMessageType
%type <b>            nonVirtualSemicolonOrInvalidComma optionalTrailingComma nonVirtualSemicolon

// same for terminals
%token <sv>      _STRING_LIT
%token <i>       _INT_LIT
%token <f>       _FLOAT_LIT
%token <optName> _EXTENSION_IDENT
%token <id>      _SYNTAX _EDITION _IMPORT _WEAK _PUBLIC _PACKAGE _OPTION _TRUE _FALSE _INF _NAN _REPEATED _OPTIONAL _REQUIRED
%token <id>      _DOUBLE _FLOAT _INT32 _INT64 _UINT32 _UINT64 _SINT32 _SINT64 _FIXED32 _FIXED64 _SFIXED32 _SFIXED64
%token <id>      _BOOL _STRING _BYTES _GROUP _ONEOF _MAP _EXTENSIONS _TO _MAX _RESERVED _ENUM _MESSAGE _EXTEND
%token <id>      _SERVICE _RPC _STREAM _RETURNS
%token <id>      _SINGULAR_IDENT
%token <idv>     _QUALIFIED_IDENT _FULLY_QUALIFIED_IDENT
%token <err>     _ERROR
// we define all of these, even ones that aren't used, to improve error messages
// so it shows the unexpected symbol instead of showing "$unk"
%token <b>   '=' ';' ':' '{' '}' '\\' '/' '?' ',' '>' '<' '+' '-' '(' ')' '[' ']' '*' '&' '^' '%' '$' '#' '@' '!' '~' '`'

%%

file
	: syntaxDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, $1, nil, lex.eof)
		lex.res = $$
	}
	| editionDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNodeWithEdition(lex.info, $1, nil, lex.eof)
		lex.res = $$
	}
	| fileElements  {
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, nil, $1, lex.eof)
		lex.res = $$
	}
	| syntaxDecl nonVirtualSemicolon fileElements {
		$1.AddSemicolon($2)
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, $1, $3, lex.eof)
		lex.res = $$
	}
	| editionDecl nonVirtualSemicolon fileElements {
		$1.AddSemicolon($2)
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNodeWithEdition(lex.info, $1, $3, lex.eof)
		lex.res = $$
	}
	| {
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, nil, nil, lex.eof)
		lex.res = $$
	}

fileElements
	: fileElements fileElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| fileElement {
		if $1 != nil {
			$$ = []ast.FileElement{$1}
		} else {
			$$ = nil
		}
	}

fileElement
	: importDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| packageDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| optionDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| messageDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| enumDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| extensionDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| serviceDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| error {
		$$ = nil
	}

syntaxDecl
	: _SYNTAX '=' _STRING_LIT {
		$$ = ast.NewSyntaxNode($1.ToKeyword(), $2, $3)
	}

editionDecl
	: _EDITION '=' _STRING_LIT {
		$$ = ast.NewEditionNode($1.ToKeyword(), $2, $3)
	}

importDecl
	: _IMPORT _STRING_LIT {
		$$ = ast.NewImportNode($1.ToKeyword(), nil, nil, $2)
	}
	| _IMPORT _WEAK _STRING_LIT {
		$$ = ast.NewImportNode($1.ToKeyword(), nil, $2.ToKeyword(), $3)
	}
	| _IMPORT _PUBLIC _STRING_LIT {
		$$ = ast.NewImportNode($1.ToKeyword(), $2.ToKeyword(), nil, $3)
	}

packageDecl
	: _PACKAGE anyIdentifier {
		$$ = ast.NewPackageNode($1.ToKeyword(), $2)
	}

optionDecl
	: _OPTION optionName '=' optionValue {
		$$ = ast.NewOptionNode($1.ToKeyword(), $2, $3, $4)
	}
	| _OPTION optionName {
		protolex.(*protoLex).ErrExtendedSyntax("expected '='")
		$$ = ast.NewIncompleteOptionNode($1.ToKeyword(), $2, nil, nil)
	}

optionName
	: anyIdentifier {
		$$ = ast.OptionNameNodeFromIdentValue($1)
	}
	| _EXTENSION_IDENT {
		$$ = $1
	}

optionValue
	: scalarValue
	| messageLiteralWithBraces

compactOptionValue
	: scalarValue
	| messageLiteralWithBraces ';' {
		ast.AddVirtualSemicolon($1.(*ast.MessageLiteralNode), $2)
		$$ = $1
	}

scalarValue
	: _STRING_LIT {
		$$ = $1
	}
	| numLit
	| singularIdent {
		$$ = $1
	}

numLit
	: _FLOAT_LIT {
		$$ = $1
	}
	| '-' _FLOAT_LIT {
		$$ = ast.NewSignedFloatLiteralNode($1, $2)
	}
	| '+' _FLOAT_LIT {
		$$ = ast.NewSignedFloatLiteralNode($1, $2)
	}
	| '+' _INF {
		f := ast.NewSpecialFloatLiteralNode($2.ToKeyword())
		$$ = ast.NewSignedFloatLiteralNode($1, f)
	}
	| '-' _INF {
		f := ast.NewSpecialFloatLiteralNode($2.ToKeyword())
		$$ = ast.NewSignedFloatLiteralNode($1, f)
	}
	| _INT_LIT {
		$$ = $1
	}
	| '+' _INT_LIT {
		$$ = ast.NewPositiveUintLiteralNode($1, $2)
	}
	| '-' _INT_LIT {
		if $2.Val > math.MaxInt64 + 1 {
			// can't represent as int so treat as float literal
			$$ = ast.NewSignedFloatLiteralNode($1, $2)
		} else {
			$$ = ast.NewNegativeIntLiteralNode($1, $2)
		}
	}

messageLiteralWithBraces
	: '{' messageTextFormat '}' {
		if $2 == nil {
			$$ = ast.NewMessageLiteralNode($1, nil, nil, $3)
		} else {
			fields, delimiters := $2.toNodes()
			$$ = ast.NewMessageLiteralNode($1, fields, delimiters, $3)
		}
	}
	| '{' '}'{
		$$ = ast.NewMessageLiteralNode($1, nil, nil, $2)
	}

messageTextFormat
	: messageLiteralFields

messageLiteralFields
	: messageLiteralFieldEntry
	| messageLiteralFieldEntry messageLiteralFields {
		if $1 != nil {
			$1.next = $2
			$$ = $1
		} else {
			$$ = $2
		}
	}

messageLiteralFieldEntry
	: messageLiteralField {
		if $1 != nil {
			$$ = &messageFieldList{field: $1}
		} else {
			$$ = nil
		}
	}
	| messageLiteralField ',' {
		if $1 != nil {
			$$ = &messageFieldList{field: $1, delimiter: $2}
		} else {
			$$ = nil
		}
	}
	| messageLiteralField ';' {
		if $1 != nil {
			$$ = &messageFieldList{field: $1, delimiter: $2}
		} else {
			$$ = nil
		}
	}
	| error ',' {
		$$ = nil
	}
	| error ';' {
		$$ = nil
	}
	| error {
		$$ = nil
	}

messageLiteralField
	: messageLiteralFieldName ':' value {
		if $1 != nil && $2 != nil {
			$$ = ast.NewMessageFieldNode($1, $2, $3)
		} else {
			$$ = nil
		}
	}
	| messageLiteralFieldName messageValue {
		if $1 != nil && $2 != nil {
			$$ = ast.NewMessageFieldNode($1, nil, $2)
		} else {
			$$ = nil
		}
	}
	| error ':' value {
		$$ = nil
	}

singularIdent
	: identKeywordName {
		$$ = $1
	}
	| _SINGULAR_IDENT

anyIdentifier
	: identKeywordName {
		$$ = $1
	}
	| _SINGULAR_IDENT {
		$$ = $1
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

messageLiteralFieldName
	: singularIdent {
		$$ = ast.NewFieldReferenceNode($1)
	}
	| '[' _QUALIFIED_IDENT ']' ';'  {
		$$ = ast.NewExtensionFieldReferenceNode($1, $2, $3)
	}
	| '[' _QUALIFIED_IDENT '/' anyIdentifier ']' ';' {
		$$ = ast.NewAnyTypeReferenceNode($1, $2, $3, $4, $5)
	}
	| '[' error ']' ';' {
		$$ = nil
	}

value
	: scalarValue
	| messageLiteral ';' {
		ast.AddVirtualSemicolon($1.(*ast.MessageLiteralNode), $2)
		$$ = $1
	}
	| listLiteral ';' {
		ast.AddVirtualSemicolon($1.(*ast.ArrayLiteralNode), $2)
		$$ = $1
	}

messageValue
	: messageLiteral ';' {
		ast.AddVirtualSemicolon($1.(*ast.MessageLiteralNode), $2)
		$$ = $1
	}
	| listOfMessagesLiteral ';' {
		ast.AddVirtualSemicolon($1.(*ast.ArrayLiteralNode), $2)
		$$ = $1
	}

messageLiteral
	: messageLiteralWithBraces
	| '<' messageTextFormat '>' {
		if $2 == nil {
			$$ = ast.NewMessageLiteralNode($1, nil, nil, $3)
		} else {
			fields, delimiters := $2.toNodes()
			$$ = ast.NewMessageLiteralNode($1, fields, delimiters, $3)
		}
	}
	| '<' '>' {
		$$ = ast.NewMessageLiteralNode($1, nil, nil, $2)
	}

listLiteral
	: '[' listElements optionalTrailingComma ']' {
		if $2 == nil {
			$$ = ast.NewArrayLiteralNode($1, nil, nil, $4)
		} else {
			if $3 != nil {
				$2.commas = append($2.commas, $3)
			}
			$$ = ast.NewArrayLiteralNode($1, $2.vals, $2.commas, $4)
		}
	}
	| '[' listElements optionalTrailingComma ';' ']' {
		if $2 == nil {
			$$ = ast.NewArrayLiteralNode($1, nil, nil, $5)
		} else {
			if $3 != nil {
				$2.commas = append($2.commas, $3)
			}
			$$ = ast.NewArrayLiteralNode($1, $2.vals, $2.commas, $5)
		}
	}
	| '[' ']' {
		$$ = ast.NewArrayLiteralNode($1, nil, nil, $2)
	}
	| '[' error ']' {
		$$ = ast.NewArrayLiteralNode($1, nil, nil, $3)
	}

listElements
	: listElement {
		$$ = &valueSlices{vals: []ast.ValueNode{$1}}
	}
	| listElements ',' listElement {
		$1.vals = append($1.vals, $3)
		$1.commas = append($1.commas, $2)
		$$ = $1
	}

listElement
	: scalarValue
	| messageLiteral ';' {
		ast.AddVirtualSemicolon($1.(*ast.MessageLiteralNode), $2)
		$$ = $1
	}

listOfMessagesLiteral
	: '[' messageLiterals optionalTrailingComma ']' {
		if $2 == nil {
			$$ = ast.NewArrayLiteralNode($1, nil, nil, $4)
		} else {
			if $3 != nil {
				$2.commas = append($2.commas, $3)
			}
			$$ = ast.NewArrayLiteralNode($1, $2.vals, $2.commas, $4)
		}
	}
	| '[' messageLiterals optionalTrailingComma ';' ']' {
		if $2 == nil {
			$$ = ast.NewArrayLiteralNode($1, nil, nil, $5)
		} else {
			if $3 != nil {
				$2.commas = append($2.commas, $3)
			}
			$$ = ast.NewArrayLiteralNode($1, $2.vals, $2.commas, $5)
		}
	}
	| '[' ']' {
		$$ = ast.NewArrayLiteralNode($1, nil, nil, $2)
	}
	| '[' error ']' {
		$$ = ast.NewArrayLiteralNode($1, nil, nil, $3)
	}

messageLiterals
	: messageLiteral ';' {
		ast.AddVirtualSemicolon($1.(*ast.MessageLiteralNode), $2)
		$$ = &valueSlices{vals: []ast.ValueNode{$1}}
	}
	| messageLiterals ',' messageLiteral ';' {
		ast.AddVirtualSemicolon($3.(*ast.MessageLiteralNode), $4)
		$1.vals = append($1.vals, $3)
		$1.commas = append($1.commas, $2)
		$$ = $1
	}


msgElementTypeIdent
	: msgElementKeywordName {
		$$ = $1
	}
	| _SINGULAR_IDENT {
		$$ = $1
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

extElementTypeIdent
	: extElementKeywordName {
		$$ = $1
	}
	| _SINGULAR_IDENT {
		$$ = $1
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

oneofElementTypeIdent
	: oneofElementKeywordName {
		$$ = $1
	}
	| _SINGULAR_IDENT {
		$$ = $1
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

notGroupElementTypeIdent
	: notGroupElementKeywordName {
		$$ = $1
	}
	| _SINGULAR_IDENT {
		$$ = $1
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

mtdElementTypeIdent
	: mtdElementKeywordName {
		$$ = $1
	}
	| _SINGULAR_IDENT {
		$$ = $1
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

enumValueName
	: enumValueKeywordName
	| _SINGULAR_IDENT {
		$$ = $1
	}

fieldCardinality
	: _REQUIRED
	| _OPTIONAL
	| _REPEATED

compactOptions
	: '[' compactOptionDecls optionalTrailingComma ']' {
		if $3 != nil {
			$2.commas = append($2.commas, $3)
		}
		if len($2.options) == 0 {
			protolex.(*protoLex).ErrExtendedSyntax("compact options list cannot be empty")
		}
		$$ = ast.NewCompactOptionsNode($1, $2.options, $2.commas, $4)
	}

compactOptionDecls :
	compactOption {
		$$ = &compactOptionSlices{options: []*ast.OptionNode{$1}}
	}
	| compactOptionDecls ',' compactOption {
		if len($1.options) == 0 {
			protolex.(*protoLex).ErrExtendedSyntaxAt("expected option before ','", $2)
		}
		$1.options = append($1.options, $3)
		$1.commas = append($1.commas, $2)
		$$ = $1
	}
	| {
		$$ = &compactOptionSlices{options: []*ast.OptionNode{}}
	}

compactOption
	: optionName '=' compactOptionValue {
		$$ = ast.NewCompactOptionNode($1, $2, $3)
	}
	| optionName {
		protolex.(*protoLex).ErrExtendedSyntax("expected '='")
		$$ = ast.NewIncompleteCompactOptionNode($1, nil, nil)
	}

groupDecl
	: fieldCardinality _GROUP singularIdent '=' _INT_LIT '{' messageBody '}' {
		$$ = ast.NewGroupNode($1.ToKeyword(), $2.ToKeyword(), $3, $4, $5, nil, $6, $7, $8)
	}
	| fieldCardinality _GROUP singularIdent '=' _INT_LIT compactOptions ';' '{' messageBody '}' {
		$$ = ast.NewGroupNode($1.ToKeyword(), $2.ToKeyword(), $3, $4, $5, $6, $8, $9, $10)
	}

oneofDecl
	: _ONEOF singularIdent '{' oneofBody '}' {
		$$ = ast.NewOneofNode($1.ToKeyword(), $2, $3, $4, $5)
	}

oneofBody
	: {
		$$ = nil
	}
	| oneofElements

oneofElements
	: oneofElements oneofElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| oneofElement {
		if $1 != nil {
			$$ = []ast.OneofElement{$1}
		} else {
			$$ = nil
		}
	}

oneofElement
	: optionDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| oneofFieldDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| oneofGroupDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| error {
		$$ = nil
	}

oneofFieldDecl
	: oneofElementTypeIdent singularIdent '=' _INT_LIT {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, nil)
	}
	| oneofElementTypeIdent singularIdent '=' _INT_LIT compactOptions {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, $5)
	}

oneofGroupDecl
	: _GROUP singularIdent '=' _INT_LIT '{' messageBody '}' {
		$$ = ast.NewGroupNode(nil, $1.ToKeyword(), $2, $3, $4, nil, $5, $6, $7)
	}
	| _GROUP singularIdent '=' _INT_LIT compactOptions ';' '{' messageBody '}' {
		$$ = ast.NewGroupNode(nil, $1.ToKeyword(), $2, $3, $4, $5, $7, $8, $9)
	}

mapFieldDecl
	: mapType singularIdent '=' _INT_LIT {
		$$ = ast.NewMapFieldNode($1, $2, $3, $4, nil)
	}
	| mapType singularIdent '=' _INT_LIT compactOptions {
		$$ = ast.NewMapFieldNode($1, $2, $3, $4, $5)
	}

mapType
	: _MAP '<' mapKeyType ',' anyIdentifier '>' ';' {
		$$ = ast.NewMapTypeNode($1.ToKeyword(), $2, $3, $4, $5, $6)
		ast.AddVirtualSemicolon($$, $7)
	}

mapKeyType
	: _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING

extensionRangeDecl
	: _EXTENSIONS tagRanges optionalTrailingComma {
		if $3 != nil {
			$2.commas = append($2.commas, $3)
		}
		$$ = ast.NewExtensionRangeNode($1.ToKeyword(), $2.ranges, $2.commas, nil)
	}
	| _EXTENSIONS tagRanges optionalTrailingComma compactOptions {
		if $3 != nil {
			$2.commas = append($2.commas, $3)
		}
		$$ = ast.NewExtensionRangeNode($1.ToKeyword(), $2.ranges, $2.commas, $4)
	}

tagRanges
	: tagRange {
		$$ = &rangeSlices{ranges: []*ast.RangeNode{$1}}
	}
	| tagRanges ',' tagRange {
		$1.ranges = append($1.ranges, $3)
		$1.commas = append($1.commas, $2)
		$$ = $1
	}

tagRange
	: _INT_LIT {
		$$ = ast.NewRangeNode($1, nil, nil, nil)
	}
	| _INT_LIT _TO _INT_LIT {
		$$ = ast.NewRangeNode($1, $2.ToKeyword(), $3, nil)
	}
	| _INT_LIT _TO _MAX {
		$$ = ast.NewRangeNode($1, $2.ToKeyword(), nil, $3.ToKeyword())
	}

enumValueRanges
	: enumValueRange {
		$$ = &rangeSlices{ranges: []*ast.RangeNode{$1}}
	}
	| enumValueRanges ',' enumValueRange {
		$1.ranges = append($1.ranges, $3)
		$1.commas = append($1.commas, $2)
		$$ = $1
	}

enumValueRange
	: enumValueNumber {
		$$ = ast.NewRangeNode($1, nil, nil, nil)
	}
	| enumValueNumber _TO enumValueNumber {
		$$ = ast.NewRangeNode($1, $2.ToKeyword(), $3, nil)
	}
	| enumValueNumber _TO _MAX {
		$$ = ast.NewRangeNode($1, $2.ToKeyword(), nil, $3.ToKeyword())
	}

enumValueNumber
	: _INT_LIT {
		$$ = $1
	}
	| '-' _INT_LIT {
		$$ = ast.NewNegativeIntLiteralNode($1, $2)
	}

msgReserved
	: _RESERVED tagRanges optionalTrailingComma {
		if $3 != nil {
			$2.commas = append($2.commas, $3)
		}
		$$ = ast.NewReservedRangesNode($1.ToKeyword(), $2.ranges, $2.commas)
	}
	| reservedNames

enumReserved
	: _RESERVED enumValueRanges optionalTrailingComma {
		if $3 != nil {
			$2.commas = append($2.commas, $3)
		}
		$$ = ast.NewReservedRangesNode($1.ToKeyword(), $2.ranges, $2.commas)
	}
	| reservedNames

reservedNames
	: _RESERVED fieldNameStrings optionalTrailingComma {
		if $3 != nil {
			$2.commas = append($2.commas, $3)
		}
		$$ = ast.NewReservedNamesNode($1.ToKeyword(), $2.names, $2.commas)
	}
	| _RESERVED fieldNameIdents {
		$$ = ast.NewReservedIdentifiersNode($1.ToKeyword(), $2.idents, $2.commas)
	}

fieldNameStrings
	: _STRING_LIT {
		$$ = &nameSlices{names: []ast.StringValueNode{$1}}
	}
	| fieldNameStrings ',' _STRING_LIT {
		$1.names = append($1.names, $3)
		$1.commas = append($1.commas, $2)
		$$ = $1
	}

fieldNameIdents
	: singularIdent {
		$$ = &nameSlices{idents: []*ast.IdentNode{$1}}
	}
	| fieldNameIdents ',' singularIdent {
		$1.idents = append($1.idents, $3)
		$1.commas = append($1.commas, $2)
		$$ = $1
	}

enumDecl
	: _ENUM singularIdent '{' enumBody '}' {
		$$ = ast.NewEnumNode($1.ToKeyword(), $2, $3, $4, $5)
	}

enumBody
	: {
		$$ = nil
	}
	| enumElements

enumElements
	: enumElements enumElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| enumElement {
		if $1 != nil {
			$$ = []ast.EnumElement{$1}
		} else {
			$$ = nil
		}
	}

enumElement
	: optionDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| enumValueDecl nonVirtualSemicolonOrInvalidComma {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| enumReserved nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| error {
		$$ = nil
	}

enumValueDecl
	: enumValueName '=' enumValueNumber  {
		$$ = ast.NewEnumValueNode($1, $2, $3, nil)
	}
	|  enumValueName '=' enumValueNumber compactOptions {
		$$ = ast.NewEnumValueNode($1, $2, $3, $4)
	}

messageDecl
	: _MESSAGE singularIdent '{' messageBody '}' {
		$$ = ast.NewMessageNode($1.ToKeyword(), $2, $3, $4, $5)
	}

messageBody
	: {
		$$ = nil
	}
	| messageElements

messageElements
	: messageElements messageElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| messageElement {
		if $1 != nil {
			$$ = []ast.MessageElement{$1}
		} else {
			$$ = nil
		}
	}

messageElement
	: messageFieldDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| enumDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| messageDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| extensionDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| extensionRangeDecl nonVirtualSemicolon{
		$1.AddSemicolon($2)
		$$ = $1
	}
	| groupDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| optionDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| oneofDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| mapFieldDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| msgReserved nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| nonVirtualSemicolon {
		$$ = ast.NewEmptyDeclNode($1)
	}

messageFieldDecl
	: fieldCardinality notGroupElementTypeIdent singularIdent '=' _INT_LIT {
		$$ = ast.NewFieldNode($1.ToKeyword(), $2, $3, $4, $5, nil)
	}
	| fieldCardinality notGroupElementTypeIdent singularIdent '=' _INT_LIT compactOptions {
		$$ = ast.NewFieldNode($1.ToKeyword(), $2, $3, $4, $5, $6)
	}
	| msgElementTypeIdent singularIdent '=' _INT_LIT {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, nil)
	}
	| msgElementTypeIdent singularIdent '=' _INT_LIT compactOptions {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, $5)
	}
	| fieldCardinality notGroupElementTypeIdent singularIdent '=' {
		protolex.(*protoLex).ErrExtendedSyntax("missing field number after '='")
		$$ = ast.NewIncompleteFieldNode($1.ToKeyword(), $2, $3, $4, nil, nil)
	}
	| fieldCardinality notGroupElementTypeIdent singularIdent {
		protolex.(*protoLex).ErrExtendedSyntax("missing '=' after field name")
		$$ = ast.NewIncompleteFieldNode($1.ToKeyword(), $2, $3, nil, nil, nil)
	}
	| fieldCardinality notGroupElementTypeIdent {
		protolex.(*protoLex).ErrExtendedSyntax("missing field name")
		$$ = ast.NewIncompleteFieldNode($1.ToKeyword(), $2, nil, nil, nil, nil)
	}
	| msgElementTypeIdent singularIdent '=' {
		protolex.(*protoLex).ErrExtendedSyntax("missing field number after '='")
		$$ = ast.NewIncompleteFieldNode(nil, $1, $2, $3, nil, nil)
	}
	| msgElementTypeIdent singularIdent {
		protolex.(*protoLex).ErrExtendedSyntax("missing '=' after field name")
		$$ = ast.NewIncompleteFieldNode(nil, $1, $2, nil, nil, nil)
	}
	| msgElementTypeIdent {
		protolex.(*protoLex).ErrExtendedSyntax("missing field name")
		$$ = ast.NewIncompleteFieldNode(nil, $1, nil, nil, nil, nil)
	}
	| fieldCardinality {
		protolex.(*protoLex).ErrExtendedSyntax("missing field type")
		$$ = ast.NewIncompleteFieldNode($1.ToKeyword(), nil, nil, nil, nil, nil)
	}

extensionDecl
	: _EXTEND anyIdentifier '{' extensionBody '}' {
		$$ = ast.NewExtendNode($1.ToKeyword(), $2, $3, $4, $5)
	}

extensionBody
	: {
		$$ = nil
	}
	| extensionElements

extensionElements
	: extensionElements extensionElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| extensionElement {
		if $1 != nil {
			$$ = []ast.ExtendElement{$1}
		} else {
			$$ = nil
		}
	}

extensionElement
	: extensionFieldDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| groupDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| error {
		$$ = nil
	}

extensionFieldDecl
	: fieldCardinality notGroupElementTypeIdent singularIdent '=' _INT_LIT {
		$$ = ast.NewFieldNode($1.ToKeyword(), $2, $3, $4, $5, nil)
	}
	| fieldCardinality notGroupElementTypeIdent singularIdent '=' _INT_LIT compactOptions {
		$$ = ast.NewFieldNode($1.ToKeyword(), $2, $3, $4, $5, $6)
	}
	| extElementTypeIdent singularIdent '=' _INT_LIT {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, nil)
	}
	| extElementTypeIdent singularIdent '=' _INT_LIT compactOptions {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, $5)
	}

serviceDecl
	: _SERVICE singularIdent '{' serviceBody '}' {
		$$ = ast.NewServiceNode($1.ToKeyword(), $2, $3, $4, $5)
	}

serviceBody
	: {
		$$ = nil
	}
	| serviceElements

serviceElements
	: serviceElements serviceElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| serviceElement {
		if $1 != nil {
			$$ = []ast.ServiceElement{$1}
		} else {
			$$ = nil
		}
	}

// NB: doc suggests support for "stream" declaration, separate from "rpc", but
// it does not appear to be supported in protoc (doc is likely from grammar for
// Google-internal version of protoc, with support for streaming stubby)
serviceElement
	: optionDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| methodDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| methodWithBodyDecl ';' {
		ast.AddVirtualSemicolon($1, $2)
		$$ = $1
	}
	| error {
		$$ = nil
	}

methodDecl
	: _RPC singularIdent methodMessageType _RETURNS methodMessageType {
		$$ = ast.NewRPCNode($1.ToKeyword(), $2, $3, $4.ToKeyword(), $5)
	}

methodWithBodyDecl
  : _RPC singularIdent methodMessageType _RETURNS methodMessageType '{' methodBody '}' {
		$$ = ast.NewRPCNodeWithBody($1.ToKeyword(), $2, $3, $4.ToKeyword(), $5, $6, $7, $8)
	}

methodMessageType
	: '(' _STREAM mtdElementTypeIdent ')' {
		$$ = ast.NewRPCTypeNode($1, $2.ToKeyword(), $3, $4)
	}
	| '(' mtdElementTypeIdent ')' {
		$$ = ast.NewRPCTypeNode($1, nil, $2, $3)
	}

methodBody
	: {
		$$ = nil
	}
	| methodElements

methodElements
	: methodElements methodElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| methodElement {
		if $1 != nil {
			$$ = []ast.RPCElement{$1}
		} else {
			$$ = nil
		}
	}

methodElement
	: optionDecl nonVirtualSemicolon {
		$1.AddSemicolon($2)
		$$ = $1
	}
	| error {
		$$ = nil
	}

// excludes message, enum, oneof, extensions, reserved, extend,
//   option, group, optional, required, and repeated
msgElementKeywordName
	: _SYNTAX
	| _EDITION
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _MAP
	| _TO
	| _MAX
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

// excludes group, optional, required, and repeated
extElementKeywordName
	: _SYNTAX
	| _EDITION
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _OPTION
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _RESERVED
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

// excludes reserved, option
enumValueKeywordName
	: _SYNTAX
	| _EDITION
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _REPEATED
	| _OPTIONAL
	| _REQUIRED
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _GROUP
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

// excludes group, option, optional, required, and repeated
oneofElementKeywordName
	: _SYNTAX
	| _EDITION
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _RESERVED
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

// excludes group
notGroupElementKeywordName
	: _SYNTAX
	| _EDITION
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _OPTION
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _REPEATED
	| _OPTIONAL
	| _REQUIRED
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _RESERVED
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

// excludes stream
mtdElementKeywordName
	: _SYNTAX
	| _EDITION
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _OPTION
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _REPEATED
	| _OPTIONAL
	| _REQUIRED
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _GROUP
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _RESERVED
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _RETURNS

identKeywordName
	: _SYNTAX
	| _EDITION
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _OPTION
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _REPEATED
	| _OPTIONAL
	| _REQUIRED
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _GROUP
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _RESERVED
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS


// ======== extended syntax rules ========

optionalTrailingComma
	: ',' {
		protolex.(*protoLex).ErrExtendedSyntaxAt("unexpected trailing comma", $1)
		$$ = $1
	}
	| {
		$$ = nil
	}

nonVirtualSemicolonOrInvalidComma
	:	nonVirtualSemicolon {
		$$ = $1
	}
	| ',' {
		protolex.(*protoLex).ErrExtendedSyntaxAt("expected ';', found ','", $1)
		$$ = $1
	}

nonVirtualSemicolon
	: ';' {
		if $1.Virtual {
			protolex.(*protoLex).ErrExtendedSyntaxAt("expected ';'", $1)
		}
		$$ = $1
	}


// =======================================

%%

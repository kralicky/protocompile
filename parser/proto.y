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
	fileElement  *ast.FileElement
	fileElements []*ast.FileElement
	pkg          *ast.PackageNode
	imprt        *ast.ImportNode
	msg          *ast.MessageNode
	msgElement   *ast.MessageElement
	msgElements  []*ast.MessageElement
	fld          *ast.FieldNode
	mapFld       *ast.MapFieldNode
	mapType      *ast.MapTypeNode
	grp          *ast.GroupNode
	oo           *ast.OneofNode
	ooElement    *ast.OneofElement
	ooElements   []*ast.OneofElement
	ext          *ast.ExtensionRangeNode
	resvd        *ast.ReservedNode
	en           *ast.EnumNode
	enElement    *ast.EnumElement
	enElements   []*ast.EnumElement
	env          *ast.EnumValueNode
	extend       *ast.ExtendNode
	extElement   *ast.ExtendElement
	extElements  []*ast.ExtendElement
	svc          *ast.ServiceNode
	svcElement   *ast.ServiceElement
	svcElements  []*ast.ServiceElement
	mtd          *ast.RPCNode
	mtdMsgType   *ast.RPCTypeNode
	mtdElement   *ast.RPCElement
	mtdElements  []*ast.RPCElement
	opt          *ast.OptionNode
	opts         []*ast.OptionNode
	ref          *ast.FieldReferenceNode
	refp         *fieldRefParens
	optName      *ast.OptionNameNode
	cmpctOpts    *ast.CompactOptionsNode
	rng          *ast.RangeNode
	rngs         []*ast.RangeElement
	names        []*ast.ReservedElement
	cid          []*ast.ComplexIdentComponent
	xid          []*ast.ComplexIdentComponent
	idv          *ast.IdentValueNode
	sl           []*ast.ArrayLiteralElement
	msgLitFlds   []*ast.MessageFieldNode
	msgLitFld    *ast.MessageFieldNode
	v            *ast.ValueNode
	il           *ast.IntValueNode
	sv           *ast.StringValueNode
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
%type <v>            fieldValue optionValue compactOptionValue scalarValue fieldScalarValue messageLiteral numLit specialFloatLit listLiteral listElement listOfMessagesLiteral messageValue
%type <il>           enumValueNumber
%type <id>           singularIdent identKeywordName mapKeyType fieldCardinality msgElementKeywordName oneofElementKeywordName notGroupElementKeywordName mtdElementKeywordName enumValueName enumValueKeywordName
%type <idv>          anyIdentifier msgElementTypeIdent oneofElementTypeIdent notGroupElementTypeIdent mtdElementTypeIdent
%type <sl>           listElements messageLiterals
%type <msgLitFlds>   messageLiteralFieldEntry messageLiteralFields messageTextFormat
%type <msgLitFld>    messageLiteralField
%type <fld>          messageFieldDecl oneofFieldDecl
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
%type <b>            nonVirtualSemicolonOrInvalidComma optionalTrailingComma virtualComma nonVirtualSemicolon virtualSemicolon commaOrInvalidSemicolon messageLiteralOpen messageLiteralClose

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
		$1.Semicolon = $2
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, $1, nil, lex.eof)
		lex.res = $$
	}
	| editionDecl nonVirtualSemicolon {
		$1.Semicolon = $2
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
		$1.Semicolon = $2
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, $1, $3, lex.eof)
		lex.res = $$
	}
	| editionDecl nonVirtualSemicolon fileElements {
		$1.Semicolon = $2
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
			$$ = []*ast.FileElement{$1}
		} else {
			$$ = nil
		}
	}

fileElement
	: importDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsFileElement()
	}
	| packageDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsFileElement()
	}
	| optionDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsFileElement()
	}
	| messageDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsFileElement()
	}
	| enumDecl virtualSemicolon{
		$1.Semicolon = $2
		$$ = $1.AsFileElement()
	}
	| extensionDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsFileElement()
	}
	| serviceDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsFileElement()
	}
	| _SINGULAR_IDENT {
		protolex.(*protoLex).ErrExtendedSyntaxAt("unexpected identifier", $1, CategoryIncompleteDecl)
		$$ = (&ast.ErrorNode{Err: $1}).AsFileElement()
	}
	| error {
		$$ = nil
	}

syntaxDecl
	: _SYNTAX '=' _STRING_LIT {
		$$ = &ast.SyntaxNode{Keyword: $1.ToKeyword(), Equals: $2, Syntax: $3}
	}

editionDecl
	: _EDITION '=' _STRING_LIT {
		$$ = &ast.EditionNode{Keyword: $1.ToKeyword(), Equals: $2, Edition: $3}
	}

importDecl
	: _IMPORT _STRING_LIT {
		$$ = &ast.ImportNode{Keyword: $1.ToKeyword(), Name: $2}
	}
	| _IMPORT _WEAK _STRING_LIT {
		$$ = &ast.ImportNode{Keyword: $1.ToKeyword(), Weak: $2.ToKeyword(), Name: $3}
	}
	| _IMPORT _PUBLIC _STRING_LIT {
		$$ = &ast.ImportNode{Keyword: $1.ToKeyword(), Public: $2.ToKeyword(), Name: $3}
	}
	| _IMPORT {
		protolex.(*protoLex).ErrExtendedSyntax("expecting string literal or \"weak\" or \"public\"", CategoryIncompleteDecl)
		$$ = &ast.ImportNode{Keyword: $1.ToKeyword()}
	}
	| _IMPORT _WEAK {
		protolex.(*protoLex).ErrExtendedSyntax("expecting string literal", CategoryIncompleteDecl)
		$$ = &ast.ImportNode{Keyword: $1.ToKeyword(), Weak: $2.ToKeyword()}
	}
	| _IMPORT _PUBLIC {
		protolex.(*protoLex).ErrExtendedSyntax("expecting string literal", CategoryIncompleteDecl)
		$$ = &ast.ImportNode{Keyword: $1.ToKeyword(), Public: $2.ToKeyword()}
	}

packageDecl
	: _PACKAGE anyIdentifier {
		$$ = &ast.PackageNode{Keyword: $1.ToKeyword(), Name: $2}
	}
	| _PACKAGE {
		protolex.(*protoLex).ErrExtendedSyntax("expected package name", CategoryIncompleteDecl)
		$$ = &ast.PackageNode{Keyword: $1.ToKeyword()}
	}

optionDecl
	: _OPTION optionName '=' optionValue {
		$$ = &ast.OptionNode{Keyword: $1.ToKeyword(), Name: $2, Equals: $3, Val: $4}
	}
	| _OPTION optionName '=' {
		protolex.(*protoLex).ErrExtendedSyntax("expected value", CategoryIncompleteDecl)
		$$ = &ast.OptionNode{Keyword: $1.ToKeyword(), Name: $2, Equals: $3}
	}
	| _OPTION optionName {
		protolex.(*protoLex).ErrExtendedSyntax("expected '='", CategoryIncompleteDecl)
		$$ = &ast.OptionNode{Keyword: $1.ToKeyword(), Name: $2}
	}
	| _OPTION {
		protolex.(*protoLex).ErrExtendedSyntax("expected option name", CategoryIncompleteDecl)
		$$ = &ast.OptionNode{Keyword: $1.ToKeyword()}
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
	| messageLiteral

compactOptionValue
	: scalarValue
	| messageLiteral virtualSemicolon {
		$1.GetMessageLiteral().Semicolon = $2
		$$ = $1
	}

scalarValue
	: _STRING_LIT {
		$$ = $1.AsValueNode()
	}
	| numLit
	| specialFloatLit
	| singularIdent {
		$$ = $1.AsValueNode()
	}

numLit
	: _FLOAT_LIT {
		$$ = $1.AsValueNode()
	}
	| '-' _FLOAT_LIT {
		$$ = (&ast.SignedFloatLiteralNode{Sign: $1, Float: $2.AsFloatValueNode()}).AsValueNode()
	}
	| _INT_LIT {
		$$ = $1.AsValueNode()
	}
	| '-' _INT_LIT {
		if $2.Val > math.MaxInt64 + 1 {
			// can't represent as int so treat as float literal
			$$ = (&ast.SignedFloatLiteralNode{Sign: $1, Float: $2.AsFloatValueNode()}).AsValueNode()
		} else {
			$$ = (&ast.NegativeIntLiteralNode{Minus: $1, Uint: $2}).AsValueNode()
		}
	}

specialFloatLit
	: '-' _INF {
		f := ast.NewSpecialFloatLiteralNode($2.ToKeyword())
		$$ = (&ast.SignedFloatLiteralNode{Sign: $1, Float: f.AsFloatValueNode()}).AsValueNode()
	}
	| '-' _NAN {
		f := ast.NewSpecialFloatLiteralNode($2.ToKeyword())
		$$ = (&ast.SignedFloatLiteralNode{Sign: $1, Float: f.AsFloatValueNode()}).AsValueNode()
	}

messageLiteral
	: messageLiteralOpen messageTextFormat messageLiteralClose {
		$$ = (&ast.MessageLiteralNode{Open: $1, Elements: $2, Close: $3}).AsValueNode()
	}
	| messageLiteralOpen messageLiteralClose {
		$$ = (&ast.MessageLiteralNode{Open: $1, Close: $2}).AsValueNode()
	}

messageTextFormat
	: messageLiteralFields

messageLiteralFields
	: messageLiteralFieldEntry {
		$$ = $1
	}
	| messageLiteralFields messageLiteralFieldEntry {
		$$ = append($1, $2...)
	}

messageLiteralFieldEntry
	: messageLiteralField {
		$$ = []*ast.MessageFieldNode{$1}
	}
	| messageLiteralField ',' {
		$1.Semicolon = $2
		$$ = []*ast.MessageFieldNode{$1}
	}
	| messageLiteralField ';' {
		$1.Semicolon = $2
		$$ = []*ast.MessageFieldNode{$1}
	}

messageLiteralField
	: messageLiteralFieldName ':' fieldValue {
		if $1 != nil && $2 != nil {
			$$ = &ast.MessageFieldNode{Name: $1, Sep: $2, Val: $3}
		} else {
			$$ = nil
		}
	}
	| messageLiteralFieldName ':' virtualSemicolon {
		protolex.(*protoLex).ErrExtendedSyntax("expected value", CategoryIncompleteDecl)
		n := &ast.MessageFieldNode{Name: $1, Sep: $2, Semicolon: $3}
		$$ = n
	}
	| messageLiteralFieldName messageValue {
		if $1 != nil && $2 != nil {
			$$ = &ast.MessageFieldNode{Name: $1, Val: $2}
		} else {
			$$ = nil
		}
	}

singularIdent
	: identKeywordName {
		$$ = $1
	}
	| _SINGULAR_IDENT

anyIdentifier
	: identKeywordName {
		$$ = $1.AsIdentValueNode()
	}
	| _SINGULAR_IDENT {
		$$ = $1.AsIdentValueNode()
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

messageLiteralFieldName
	: singularIdent {
		$$ = &ast.FieldReferenceNode{Name: $1.AsIdentValueNode()}
	}
	| '[' _QUALIFIED_IDENT virtualComma ']' virtualSemicolon {
		$$ = &ast.FieldReferenceNode{Open: $1, Name: $2, Comma: $3, Close: $4, Semicolon: $5}
	}
	| '[' _QUALIFIED_IDENT '/' anyIdentifier virtualComma ']' virtualSemicolon {
		$$ = &ast.FieldReferenceNode{Open: $1, UrlPrefix: $2, Slash: $3, Name: $4, Comma: $5, Close: $6, Semicolon: $7}
	}
	| '[' error ']' ';' {
		$$ = nil
	}

fieldValue
	: fieldScalarValue
	| messageLiteral virtualSemicolon {
		$1.GetMessageLiteral().Semicolon = $2
		$$ = $1
	}
	| listLiteral virtualSemicolon {
		$1.GetArrayLiteral().Semicolon = $2
		$$ = $1
	}

fieldScalarValue
	: _STRING_LIT {
		$$ = $1.AsValueNode()
	}
	| numLit
	| '-' _INF {
		kw := $2.ToKeyword()
		f := ast.NewSpecialFloatLiteralNode(kw)
		$$ = (&ast.SignedFloatLiteralNode{Sign: $1, Float: f.AsFloatValueNode()}).AsValueNode()
	}
	| '-' _NAN {
		kw := $2.ToKeyword()
		f := ast.NewSpecialFloatLiteralNode(kw)
		$$ = (&ast.SignedFloatLiteralNode{Sign: $1, Float: f.AsFloatValueNode()}).AsValueNode()
	}
	| singularIdent {
		$$ = $1.AsValueNode()
	}

messageValue
	: messageLiteral virtualSemicolon {
		$1.GetMessageLiteral().Semicolon = $2
		$$ = $1
	}
	| listOfMessagesLiteral virtualSemicolon {
		$1.GetArrayLiteral().Semicolon = $2
		$$ = $1
	}

listLiteral
	: '[' listElements virtualComma ']' {
		if $3 != nil {
			$2 = append($2, $3.AsArrayLiteralElement())
		}
		$$ = (&ast.ArrayLiteralNode{OpenBracket: $1, Elements: $2, CloseBracket: $4}).AsValueNode()
	}
	| '[' virtualComma ']' {
		$$ = (&ast.ArrayLiteralNode{OpenBracket: $1, Elements: []*ast.ArrayLiteralElement{$2.AsArrayLiteralElement()}, CloseBracket: $3}).AsValueNode()
	}
	| '[' ']' {
		$$ = (&ast.ArrayLiteralNode{OpenBracket: $1, CloseBracket: $2}).AsValueNode()
	}
	| '[' error ']' {
		$$ = (&ast.ArrayLiteralNode{OpenBracket: $1, CloseBracket: $3}).AsValueNode()
	}

listElements
	: listElement {
		$$ = []*ast.ArrayLiteralElement{$1.AsArrayLiteralElement()}
	}
	| listElements ',' listElement {
		$$ = append($1, $2.AsArrayLiteralElement(), $3.AsArrayLiteralElement())
	}

listElement
	: fieldScalarValue
	| messageLiteral virtualSemicolon {
		$1.GetMessageLiteral().Semicolon = $2
		$$ = $1
	}

listOfMessagesLiteral
	: '[' messageLiterals virtualComma ']' {
		if $3 != nil {
			$2 = append($2, $3.AsArrayLiteralElement())
		}
		$$ = (&ast.ArrayLiteralNode{OpenBracket: $1, Elements: $2, CloseBracket: $4}).AsValueNode()
	}
	| '[' virtualComma ']' {
		$$ = (&ast.ArrayLiteralNode{OpenBracket: $1, Elements: []*ast.ArrayLiteralElement{$2.AsArrayLiteralElement()}, CloseBracket: $3}).AsValueNode()
	}
	| '[' ']' {
		$$ = (&ast.ArrayLiteralNode{OpenBracket: $1, CloseBracket: $2}).AsValueNode()
	}
	| '[' error ']' {
		$$ = (&ast.ArrayLiteralNode{OpenBracket: $1, CloseBracket: $3}).AsValueNode()
	}

messageLiterals
	: messageLiteral virtualSemicolon {
		$1.GetMessageLiteral().Semicolon = $2
		$$ = []*ast.ArrayLiteralElement{$1.AsArrayLiteralElement()}
	}
	| messageLiterals ',' messageLiteral virtualSemicolon {
		$3.GetMessageLiteral().Semicolon = $4
		$$ = append($1, $2.AsArrayLiteralElement(), $3.AsArrayLiteralElement())
	}


msgElementTypeIdent
	: msgElementKeywordName {
		$$ = $1.AsIdentValueNode()
	}
	| _SINGULAR_IDENT {
		$$ = $1.AsIdentValueNode()
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

oneofElementTypeIdent
	: oneofElementKeywordName {
		$$ = $1.AsIdentValueNode()
	}
	| _SINGULAR_IDENT {
		$$ = $1.AsIdentValueNode()
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

notGroupElementTypeIdent
	: notGroupElementKeywordName {
		$$ = $1.AsIdentValueNode()
	}
	| _SINGULAR_IDENT {
		$$ = $1.AsIdentValueNode()
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT

mtdElementTypeIdent
	: mtdElementKeywordName {
		$$ = $1.AsIdentValueNode()
	}
	| _SINGULAR_IDENT {
		$$ = $1.AsIdentValueNode()
	}
	| _QUALIFIED_IDENT
	| _FULLY_QUALIFIED_IDENT
	| {
		protolex.(*protoLex).ErrExtendedSyntax("expected message type", CategoryIncompleteDecl)
		$$ = nil
	}

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
	: '[' compactOptionDecls ']' {
		if r := $2[len($2)-1].Semicolon; r != nil && !r.Virtual {
			protolex.(*protoLex).ErrExtendedSyntax("unexpected trailing '"+string(r.Rune)+"'", CategoryExtraTokens)
		}
		$$ = &ast.CompactOptionsNode{OpenBracket: $1, Options: $2, CloseBracket: $3}
	}
	| '[' ']' {
		protolex.(*protoLex).ErrExtendedSyntax("compact options list cannot be empty", CategoryEmptyDecl)
		$$ = &ast.CompactOptionsNode{OpenBracket: $1, CloseBracket: $2}
	}

compactOptionDecls
	:	compactOption commaOrInvalidSemicolon {
		$1.Semicolon = $2
		$$ = []*ast.OptionNode{$1}
	}
	| compactOptionDecls compactOption commaOrInvalidSemicolon {
		$2.Semicolon = $3
		$$ = append($1, $2)
	}


compactOption
	: optionName '=' compactOptionValue {
		$$ = &ast.OptionNode{Name: $1, Equals: $2, Val: $3}
	}
	| optionName '=' {
		protolex.(*protoLex).ErrExtendedSyntax("expected value", CategoryIncompleteDecl)
		$$ = &ast.OptionNode{Name: $1, Equals: $2}
	}
	| optionName {
		protolex.(*protoLex).ErrExtendedSyntax("expected '='", CategoryIncompleteDecl)
		$$ = &ast.OptionNode{Name: $1}
	}
	| {
		protolex.(*protoLex).ErrExtendedSyntax("expected option name", CategoryIncompleteDecl)
		$$ = &ast.OptionNode{}
	}

groupDecl
	: fieldCardinality _GROUP singularIdent '=' _INT_LIT '{' messageBody '}' {
		$$ = &ast.GroupNode{Label: $1.ToKeyword(), Keyword: $2.ToKeyword(), Name: $3, Equals: $4, Tag: $5, OpenBrace: $6, Decls: $7, CloseBrace: $8}
	}
	| fieldCardinality _GROUP singularIdent '=' _INT_LIT compactOptions virtualSemicolon '{' messageBody '}' {
		$6.Semicolon = $7
		$$ = &ast.GroupNode{Label: $1.ToKeyword(), Keyword: $2.ToKeyword(), Name: $3, Equals: $4, Tag: $5, Options: $6, OpenBrace: $8, Decls: $9, CloseBrace: $10}
	}

oneofDecl
	: _ONEOF singularIdent '{' oneofBody '}' {
		$$ = &ast.OneofNode{Keyword: $1.ToKeyword(), Name: $2, OpenBrace: $3, Decls: $4, CloseBrace: $5}
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
			$$ = []*ast.OneofElement{$1}
		} else {
			$$ = nil
		}
	}

oneofElement
	: optionDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsOneofElement()
	}
	| oneofFieldDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsOneofElement()
	}
	| oneofGroupDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsOneofElement()
	}
	| error {
		$$ = nil
	}

oneofFieldDecl
	: oneofElementTypeIdent singularIdent '=' _INT_LIT {
		$$ = &ast.FieldNode{FieldType: $1, Name: $2, Equals: $3, Tag: $4}
	}
	| oneofElementTypeIdent singularIdent '=' _INT_LIT compactOptions {
		$$ = &ast.FieldNode{FieldType: $1, Name: $2, Equals: $3, Tag: $4, Options: $5}
	}

oneofGroupDecl
	: _GROUP singularIdent '=' _INT_LIT '{' messageBody '}' {
		$$ = &ast.GroupNode{Keyword: $1.ToKeyword(), Name: $2, Equals: $3, Tag: $4, OpenBrace: $5, Decls: $6, CloseBrace: $7}
	}
	| _GROUP singularIdent '=' _INT_LIT compactOptions virtualSemicolon '{' messageBody '}' {
		$5.Semicolon = $6
		$$ = &ast.GroupNode{Keyword: $1.ToKeyword(), Name: $2, Equals: $3, Tag: $4, Options: $5, OpenBrace: $7, Decls: $8, CloseBrace: $9}
	}

mapFieldDecl
	: mapType virtualSemicolon singularIdent '=' _INT_LIT {
		$1.Semicolon = $2
		$$ = &ast.MapFieldNode{MapType: $1, Name: $3, Equals: $4, Tag: $5}
	}
	| mapType virtualSemicolon singularIdent '=' _INT_LIT compactOptions {
		$1.Semicolon = $2
		$$ = &ast.MapFieldNode{MapType: $1, Name: $3, Equals: $4, Tag: $5, Options: $6}
	}

mapType
	: _MAP '<' mapKeyType ',' anyIdentifier '>' {
		$$ = &ast.MapTypeNode{Keyword: $1.ToKeyword(), OpenAngle: $2, KeyType: $3, Comma: $4, ValueType: $5, CloseAngle: $6}
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
			$2 = append($2, $3.AsRangeElement())
		}
		$$ = &ast.ExtensionRangeNode{Keyword: $1.ToKeyword(), Elements: $2}
	}
	| _EXTENSIONS tagRanges optionalTrailingComma compactOptions {
		if $3 != nil {
			$2 = append($2, $3.AsRangeElement())
		}
		$$ = &ast.ExtensionRangeNode{Keyword: $1.ToKeyword(), Elements: $2, Options: $4}
	}

tagRanges
	: tagRange {
		$$ = []*ast.RangeElement{$1.AsRangeElement()}
	}
	| tagRanges ',' tagRange {
		$$ = append($1, $2.AsRangeElement(), $3.AsRangeElement())
	}

tagRange
	: _INT_LIT {
		$$ = &ast.RangeNode{StartVal: $1.AsIntValueNode()}
	}
	| _INT_LIT _TO _INT_LIT {
		$$ = &ast.RangeNode{StartVal: $1.AsIntValueNode(), To: $2.ToKeyword(), EndVal: $3.AsIntValueNode()}
	}
	| _INT_LIT _TO _MAX {
		$$ = &ast.RangeNode{StartVal: $1.AsIntValueNode(), To: $2.ToKeyword(), Max: $3.ToKeyword()}
	}

enumValueRanges
	: enumValueRange {
		$$ = []*ast.RangeElement{$1.AsRangeElement()}
	}
	| enumValueRanges ',' enumValueRange {
		$$ = append($1, $2.AsRangeElement(), $3.AsRangeElement())
	}

enumValueRange
	: enumValueNumber {
		$$ = &ast.RangeNode{StartVal: $1}
	}
	| enumValueNumber _TO enumValueNumber {
		$$ = &ast.RangeNode{StartVal: $1, To: $2.ToKeyword(), EndVal: $3}
	}
	| enumValueNumber _TO _MAX {
		$$ = &ast.RangeNode{StartVal: $1, To: $2.ToKeyword(), Max: $3.ToKeyword()}
	}

enumValueNumber
	: _INT_LIT {
		$$ = $1.AsIntValueNode()
	}
	| '-' _INT_LIT {
		$$ = (&ast.NegativeIntLiteralNode{Minus: $1, Uint: $2}).AsIntValueNode()
	}

msgReserved
	: _RESERVED tagRanges optionalTrailingComma {
		if $3 != nil {
			$2 = append($2, $3.AsRangeElement())
		}
		$$ = &ast.ReservedNode{Keyword: $1.ToKeyword(), Elements: ast.RangeElementsToReservedElements($2)}
	}
	| reservedNames

enumReserved
	: _RESERVED enumValueRanges optionalTrailingComma {
		if $3 != nil {
			$2 = append($2, $3.AsRangeElement())
		}
		$$ = &ast.ReservedNode{Keyword: $1.ToKeyword(), Elements: ast.RangeElementsToReservedElements($2)}
	}
	| reservedNames

reservedNames
	: _RESERVED fieldNameStrings optionalTrailingComma {
		if $3 != nil {
			$2 = append($2, $3.AsReservedElement())
		}
		$$ = &ast.ReservedNode{Keyword: $1.ToKeyword(), Elements: $2}
	}
	| _RESERVED fieldNameIdents {
		$$ = &ast.ReservedNode{Keyword: $1.ToKeyword(), Elements: $2}
	}

fieldNameStrings
	: _STRING_LIT {
		$$ = []*ast.ReservedElement{$1.AsReservedElement()}
	}
	| fieldNameStrings ',' _STRING_LIT {
		$$ = append($1, $2.AsReservedElement(), $3.AsReservedElement())
	}

fieldNameIdents
	: singularIdent {
		$$ = []*ast.ReservedElement{$1.AsReservedElement()}
	}
	| fieldNameIdents ',' singularIdent {
		$$ = append($1, $2.AsReservedElement(), $3.AsReservedElement())
	}

enumDecl
	: _ENUM singularIdent '{' enumBody '}' {
		$$ = &ast.EnumNode{Keyword: $1.ToKeyword(), Name: $2, OpenBrace: $3, Decls: $4, CloseBrace: $5}
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
			$$ = []*ast.EnumElement{$1}
		} else {
			$$ = nil
		}
	}

enumElement
	: optionDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsEnumElement()
	}
	| enumValueDecl nonVirtualSemicolonOrInvalidComma {
		$1.Semicolon = $2
		$$ = $1.AsEnumElement()
	}
	| enumReserved nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsEnumElement()
	}
	| error {
		$$ = nil
	}

enumValueDecl
	: enumValueName '=' enumValueNumber  {
		$$ = &ast.EnumValueNode{Name: $1, Equals: $2, Number: $3}
	}
	| enumValueName '=' enumValueNumber compactOptions {
		$$ = &ast.EnumValueNode{Name: $1, Equals: $2, Number: $3, Options: $4}
	}

messageDecl
	: _MESSAGE singularIdent '{' messageBody '}' {
		$$ = &ast.MessageNode{Keyword: $1.ToKeyword(), Name: $2, OpenBrace: $3, Decls: $4, CloseBrace: $5}
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
			$$ = []*ast.MessageElement{$1}
		} else {
			$$ = nil
		}
	}

messageElement
	: messageFieldDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| enumDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| messageDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| extensionDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| extensionRangeDecl nonVirtualSemicolon{
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| groupDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| optionDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| oneofDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| mapFieldDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| msgReserved nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsMessageElement()
	}
	| nonVirtualSemicolon {
		$$ = (&ast.EmptyDeclNode{Semicolon: $1}).AsMessageElement()
	}

messageFieldDecl
	: fieldCardinality notGroupElementTypeIdent singularIdent '=' _INT_LIT {
		$$ = &ast.FieldNode{Label: $1.ToKeyword(), FieldType: $2, Name: $3, Equals: $4, Tag: $5}
	}
	| fieldCardinality notGroupElementTypeIdent singularIdent '=' _INT_LIT compactOptions {
		$$ = &ast.FieldNode{Label: $1.ToKeyword(), FieldType: $2, Name: $3, Equals: $4, Tag: $5, Options: $6}
	}
	| msgElementTypeIdent singularIdent '=' _INT_LIT {
		$$ = &ast.FieldNode{FieldType: $1, Name: $2, Equals: $3, Tag: $4}
	}
	| msgElementTypeIdent singularIdent '=' _INT_LIT compactOptions {
		$$ = &ast.FieldNode{FieldType: $1, Name: $2, Equals: $3, Tag: $4, Options: $5}
	}
	| fieldCardinality notGroupElementTypeIdent singularIdent '=' {
		protolex.(*protoLex).ErrExtendedSyntax("missing field number after '='", CategoryIncompleteDecl)
		$$ = &ast.FieldNode{Label: $1.ToKeyword(), FieldType: $2, Name: $3}
	}
	| fieldCardinality notGroupElementTypeIdent singularIdent {
		protolex.(*protoLex).ErrExtendedSyntax("missing '=' after field name", CategoryIncompleteDecl)
		$$ = &ast.FieldNode{Label: $1.ToKeyword(), FieldType: $2, Name: $3}
	}
	| fieldCardinality notGroupElementTypeIdent {
		protolex.(*protoLex).ErrExtendedSyntax("missing field name", CategoryIncompleteDecl)
		$$ = &ast.FieldNode{Label: $1.ToKeyword(), FieldType: $2}
	}
	| msgElementTypeIdent singularIdent '=' {
		protolex.(*protoLex).ErrExtendedSyntax("missing field number after '='", CategoryIncompleteDecl)
		$$ = &ast.FieldNode{FieldType: $1, Name: $2}
	}
	| msgElementTypeIdent singularIdent {
		protolex.(*protoLex).ErrExtendedSyntax("missing '=' after field name", CategoryIncompleteDecl)
		$$ = &ast.FieldNode{FieldType: $1, Name: $2}
	}
	| msgElementTypeIdent {
		protolex.(*protoLex).ErrExtendedSyntax("missing field name", CategoryIncompleteDecl)
		$$ = &ast.FieldNode{FieldType: $1}
	}
	| fieldCardinality {
		protolex.(*protoLex).ErrExtendedSyntax("missing field type", CategoryIncompleteDecl)
		$$ = &ast.FieldNode{Label: $1.ToKeyword()}
	}

extensionDecl
	: _EXTEND anyIdentifier '{' extensionBody '}' {
		$$ = &ast.ExtendNode{Keyword: $1.ToKeyword(), Extendee: $2, OpenBrace: $3, Decls: $4, CloseBrace: $5}
	}
	| _EXTEND anyIdentifier {
		protolex.(*protoLex).ErrExtendedSyntax("expected '{'", CategoryIncompleteDecl)
		$$ = &ast.ExtendNode{Keyword: $1.ToKeyword(), Extendee: $2}
	}
	| _EXTEND {
		protolex.(*protoLex).ErrExtendedSyntax("expected message name", CategoryIncompleteDecl)
		$$ = &ast.ExtendNode{Keyword: $1.ToKeyword()}
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
			$$ = []*ast.ExtendElement{$1}
		} else {
			$$ = nil
		}
	}

extensionElement
	: messageFieldDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsExtendElement()
	}
	| groupDecl virtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsExtendElement()
	}
	| mapFieldDecl ';' {
		$1.Semicolon = $2
		protolex.(*protoLex).ErrExtendedSyntaxAt("map fields not allowed in extend declarations", $1, CategoryDeclNotAllowed)
		$$ = nil
	}
	| oneofDecl ';' {
		$1.Semicolon = $2
		protolex.(*protoLex).ErrExtendedSyntaxAt("\"oneof\" not allowed in extend declarations", $1, CategoryDeclNotAllowed)
		$$ = nil
	}
	| msgReserved ';' {
		$1.Semicolon = $2
		protolex.(*protoLex).ErrExtendedSyntaxAt("\"reserved\" not allowed in extend declarations", $1, CategoryDeclNotAllowed)
		$$ = nil
	}
	| extensionRangeDecl ';' {
		$1.Semicolon = $2
		protolex.(*protoLex).ErrExtendedSyntaxAt("extension ranges not allowed in extend declarations", $1, CategoryDeclNotAllowed)
		$$ = nil
	}
	| messageDecl ';' {
		$1.Semicolon = $2
		protolex.(*protoLex).ErrExtendedSyntaxAt("nested messages not allowed in extend declarations", $1, CategoryDeclNotAllowed)
		$$ = nil
	}
	| enumDecl ';' {
		$1.Semicolon = $2
		protolex.(*protoLex).ErrExtendedSyntaxAt("nested enums not allowed in extend declarations", $1, CategoryDeclNotAllowed)
		$$ = nil
	}
	| error {
		$$ = nil
	}

serviceDecl
	: _SERVICE singularIdent '{' serviceBody '}' {
		$$ = &ast.ServiceNode{Keyword: $1.ToKeyword(), Name: $2, OpenBrace: $3, Decls: $4, CloseBrace: $5}
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
			$$ = []*ast.ServiceElement{$1}
		} else {
			$$ = nil
		}
	}

// NB: doc suggests support for "stream" declaration, separate from "rpc", but
// it does not appear to be supported in protoc (doc is likely from grammar for
// Google-internal version of protoc, with support for streaming stubby)
serviceElement
	: optionDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsServiceElement()
	}
	| methodDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsServiceElement()
	}
	| methodWithBodyDecl ';' {
		$1.Semicolon = $2
		$$ = $1.AsServiceElement()
	}
	| error {
		$$ = nil
	}

methodDecl
	: _RPC singularIdent methodMessageType _RETURNS methodMessageType {
		$$ = &ast.RPCNode{Keyword: $1.ToKeyword(), Name: $2, Input: $3, Returns: $4.ToKeyword(), Output: $5}
	}

methodWithBodyDecl
  : _RPC singularIdent methodMessageType _RETURNS methodMessageType '{' methodBody '}' {
		$$ = &ast.RPCNode{Keyword: $1.ToKeyword(), Name: $2, Input: $3, Returns: $4.ToKeyword(), Output: $5, OpenBrace: $6, Decls: $7, CloseBrace: $8}
	}

methodMessageType
	: '(' _STREAM mtdElementTypeIdent ')' {
		$$ = &ast.RPCTypeNode{OpenParen: $1, Stream: $2.ToKeyword(), MessageType: $3, CloseParen: $4}
	}
	| '(' mtdElementTypeIdent ')' {
		$$ = &ast.RPCTypeNode{OpenParen: $1, MessageType: $2, CloseParen: $3}
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
			$$ = []*ast.RPCElement{$1}
		} else {
			$$ = nil
		}
	}

methodElement
	: optionDecl nonVirtualSemicolon {
		$1.Semicolon = $2
		$$ = $1.AsRPCElement()
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


optionalTrailingComma
	: ',' {
		protolex.(*protoLex).ErrExtendedSyntaxAt("unexpected trailing comma", $1, CategoryExtraTokens)
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
		protolex.(*protoLex).ErrExtendedSyntaxAt("expected ';', found ','", $1, CategoryIncorrectToken)
		$$ = $1
	}

nonVirtualSemicolon
	: ';' {
		if $1.Virtual {
			protolex.(*protoLex).ErrExtendedSyntaxAt("expected ';'", $1, CategoryMissingToken)
		}
		$$ = $1
	}

virtualSemicolon
	: ';' {
		if !$1.Virtual {
			protolex.(*protoLex).ErrExtendedSyntaxAt("unexpected ';'", $1, CategoryExtraTokens)
		}
		$$ = $1
	}

virtualComma
	: ',' {
		if !$1.Virtual {
			protolex.(*protoLex).ErrExtendedSyntaxAt("unexpected ','", $1, CategoryExtraTokens)
		}
		$$ = $1
	}

commaOrInvalidSemicolon
	: ','
	| ';' {
		if $1.Virtual {
			protolex.(*protoLex).ErrExtendedSyntaxAt("expected ','", $1, CategoryMissingToken)
		} else {
			protolex.(*protoLex).ErrExtendedSyntaxAt("expected ',', found ';'", $1, CategoryIncorrectToken)
		}
	}

messageLiteralOpen
	: '{' {
		$$ = $1
	}
	| '<' {
		$$ = $1
	}

messageLiteralClose
	: '}' {
		$$ = $1
	}
	| '>' {
		$$ = $1
	}

%%

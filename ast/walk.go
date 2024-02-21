package ast

import "fmt"

type Visitor interface {
	Visit(node Node) Visitor
	Before(node Node) bool
	After(node Node)
}

func Walk(v Visitor, node Node) {
	node = Unwrap(node)

	v.Before(node)
	defer v.After(node)

	if v = v.Visit(node); v == nil {
		return
	}

	switch n := node.(type) {
	case *FileNode:
		if n.Syntax != nil {
			Walk(v, n.Syntax)
		}
		if n.Edition != nil {
			Walk(v, n.Edition)
		}
		for _, decl := range n.Decls {
			if !IsNil(decl) {
				Walk(v, decl)
			}
		}
		if n.EOF != nil {
			Walk(v, n.EOF)
		}
	case *SyntaxNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Equals != nil {
			Walk(v, n.Equals)
		}
		if !IsNil(n.Syntax) {
			Walk(v, n.Syntax)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *EditionNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Equals != nil {
			Walk(v, n.Equals)
		}
		if !IsNil(n.Edition) {
			Walk(v, n.Edition)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *PackageNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if !IsNil(n.Name) {
			Walk(v, n.Name)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *ImportNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Public != nil {
			Walk(v, n.Public)
		}
		if n.Weak != nil {
			Walk(v, n.Weak)
		}
		if !IsNil(n.Name) {
			Walk(v, n.Name)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *OptionNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.Equals != nil {
			Walk(v, n.Equals)
		}
		if !IsNil(n.Val) {
			Walk(v, n.Val)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *OptionNameNode:
		zipWalk(v, n.Parts, n.Dots)
	case *FieldReferenceNode:
		if n.Open != nil {
			Walk(v, n.Open)
		}
		if !IsNil(n.UrlPrefix) {
			Walk(v, n.UrlPrefix)
		}
		if n.Slash != nil {
			Walk(v, n.Slash)
		}
		if !IsNil(n.Name) {
			Walk(v, n.Name)
		}
		if n.Comma != nil {
			Walk(v, n.Comma)
		}
		if n.Close != nil {
			Walk(v, n.Close)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *CompactOptionsNode:
		if n.OpenBracket != nil {
			Walk(v, n.OpenBracket)
		}
		for _, opt := range n.Options {
			Walk(v, opt)
		}
		if n.CloseBracket != nil {
			Walk(v, n.CloseBracket)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *MessageNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.OpenBrace != nil {
			Walk(v, n.OpenBrace)
		}
		for _, decl := range n.Decls {
			if !IsNil(decl) {
				Walk(v, decl)
			}
		}
		if n.CloseBrace != nil {
			Walk(v, n.CloseBrace)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *ExtendNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if !IsNil(n.Extendee) {
			Walk(v, n.Extendee)
		}
		if n.OpenBrace != nil {
			Walk(v, n.OpenBrace)
		}
		for _, decl := range n.Decls {
			if !IsNil(decl) {
				Walk(v, decl)
			}
		}
		if n.CloseBrace != nil {
			Walk(v, n.CloseBrace)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *ExtensionRangeNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		zipWalk(v, n.Ranges, n.Commas)
		if n.Options != nil {
			Walk(v, n.Options)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *ReservedNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		switch {
		case len(n.Ranges) > 0:
			zipWalk(v, n.Ranges, n.Commas)
		case len(n.Names) > 0:
			zipWalk(v, n.Names, n.Commas)
		case len(n.Identifiers) > 0:
			zipWalk(v, n.Identifiers, n.Commas)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *RangeNode:
		if !IsNil(n.StartVal) {
			Walk(v, n.StartVal)
		}
		if n.To != nil {
			Walk(v, n.To)
		}
		if !IsNil(n.EndVal) {
			Walk(v, n.EndVal)
		}
		if n.Max != nil {
			Walk(v, n.Max)
		}
	case *FieldNode:
		if n.Label != nil {
			Walk(v, n.Label)
		}
		if !IsNil(n.FieldType) {
			Walk(v, n.FieldType)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.Equals != nil {
			Walk(v, n.Equals)
		}
		if n.Tag != nil {
			Walk(v, n.Tag)
		}
		if n.Options != nil {
			Walk(v, n.Options)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *GroupNode:
		if n.Label != nil {
			Walk(v, n.Label)
		}
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.Equals != nil {
			Walk(v, n.Equals)
		}
		if n.Tag != nil {
			Walk(v, n.Tag)
		}
		if n.Options != nil {
			Walk(v, n.Options)
		}
		if n.OpenBrace != nil {
			Walk(v, n.OpenBrace)
		}
		for _, decl := range n.Decls {
			if !IsNil(decl) {
				Walk(v, decl)
			}
		}
		if n.CloseBrace != nil {
			Walk(v, n.CloseBrace)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *MapFieldNode:
		if n.MapType != nil {
			Walk(v, n.MapType)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.Equals != nil {
			Walk(v, n.Equals)
		}
		if n.Tag != nil {
			Walk(v, n.Tag)
		}
		if n.Options != nil {
			Walk(v, n.Options)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *MapTypeNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.OpenAngle != nil {
			Walk(v, n.OpenAngle)
		}
		if n.KeyType != nil {
			Walk(v, n.KeyType)
		}
		if n.Comma != nil {
			Walk(v, n.Comma)
		}
		if !IsNil(n.ValueType) {
			Walk(v, n.ValueType)
		}
		if n.CloseAngle != nil {
			Walk(v, n.CloseAngle)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *OneofNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.OpenBrace != nil {
			Walk(v, n.OpenBrace)
		}
		for _, decl := range n.Decls {
			if !IsNil(decl) {
				Walk(v, decl)
			}
		}
		if n.CloseBrace != nil {
			Walk(v, n.CloseBrace)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *EnumNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.OpenBrace != nil {
			Walk(v, n.OpenBrace)
		}
		for _, decl := range n.Decls {
			if !IsNil(decl) {
				Walk(v, decl)
			}
		}
		if n.CloseBrace != nil {
			Walk(v, n.CloseBrace)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *EnumValueNode:
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.Equals != nil {
			Walk(v, n.Equals)
		}
		if !IsNil(n.Number) {
			Walk(v, n.Number)
		}
		if n.Options != nil {
			Walk(v, n.Options)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *ServiceNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.OpenBrace != nil {
			Walk(v, n.OpenBrace)
		}
		for _, decl := range n.Decls {
			if !IsNil(decl) {
				Walk(v, decl)
			}
		}
		if n.CloseBrace != nil {
			Walk(v, n.CloseBrace)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *RPCNode:
		if n.Keyword != nil {
			Walk(v, n.Keyword)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.Input != nil {
			Walk(v, n.Input)
		}
		if n.Returns != nil {
			Walk(v, n.Returns)
		}
		if n.Output != nil {
			Walk(v, n.Output)
		}
		if n.OpenBrace != nil {
			Walk(v, n.OpenBrace)
		}
		for _, decl := range n.Decls {
			if !IsNil(decl) {
				Walk(v, decl)
			}
		}
		if n.CloseBrace != nil {
			Walk(v, n.CloseBrace)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *RPCTypeNode:
		if n.OpenParen != nil {
			Walk(v, n.OpenParen)
		}
		if n.Stream != nil {
			Walk(v, n.Stream)
		}
		if !IsNil(n.MessageType) {
			Walk(v, n.MessageType)
		}
		if n.CloseParen != nil {
			Walk(v, n.CloseParen)
		}
	case *CompoundIdentNode:
		zipWalk(v, n.Components, n.Dots)
	case *CompoundStringLiteralNode:
		for _, elem := range n.Elements {
			if !IsNil(elem) {
				Walk(v, elem)
			}
		}
	case *NegativeIntLiteralNode:
		if n.Minus != nil {
			Walk(v, n.Minus)
		}
		if n.Uint != nil {
			Walk(v, n.Uint)
		}
	case *SignedFloatLiteralNode:
		if n.Sign != nil {
			Walk(v, n.Sign)
		}
		if !IsNil(n.Float) {
			Walk(v, n.Float)
		}
	case *ArrayLiteralNode:
		if n.OpenBracket != nil {
			Walk(v, n.OpenBracket)
		}
		zipWalk(v, n.Elements, n.Commas)
		if n.CloseBracket != nil {
			Walk(v, n.CloseBracket)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *MessageLiteralNode:
		if n.Open != nil {
			Walk(v, n.Open)
		}
		for i, field := range n.Elements {
			Walk(v, field)
			if sep := n.Seps[i]; sep != nil {
				Walk(v, sep)
			}
		}
		if n.Close != nil {
			Walk(v, n.Close)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *MessageFieldNode:
		if n.Name != nil {
			Walk(v, n.Name)
		}
		if n.Sep != nil {
			Walk(v, n.Sep)
		}
		if !IsNil(n.Val) {
			Walk(v, n.Val)
		}
		if n.Semicolon != nil {
			Walk(v, n.Semicolon)
		}
	case *SpecialFloatLiteralNode:
		Walk(v, n.Keyword)
	case *IdentNode,
		*StringLiteralNode,
		*UintLiteralNode,
		*FloatLiteralNode,
		*RuneNode,
		*EmptyDeclNode:
		// terminal node
	case *ErrorNode:
		if !IsNil(n.Err) {
			Walk(v, n.Err)
		}
	default:
		panic(fmt.Sprintf("unexpected type of node: %T", n))
	}
}

// walks two slices of nodes at once, ordered by token position.
func zipWalk[A, B Node](v Visitor, a []A, b []B) {
	ai := 0
	bi := 0
	for range len(a) + len(b) {
		var an, bn Node
		if ai < len(a) {
			an = a[ai]
		}
		if bi < len(b) {
			bn = b[bi]
		}
		if !IsNil(an) && (IsNil(bn) || an.Start() < bn.Start()) {
			Walk(v, an)
			ai++
		} else {
			Walk(v, bn)
			bi++
		}
	}
}

// WalkOption represents an option used with the Walk function. These
// allow optional before and after hooks to be invoked as each node in
// the tree is visited.
type WalkOption func(*walkOptions)

type walkOptions struct {
	before func(Node) bool
	after  func(Node)

	hasRangeRequirement bool
	start, end          Token
	depthLimit          int

	hasIntersectionRequirement bool
	intersects                 Token
}

// WithBefore returns a WalkOption that will cause the given function to be
// invoked before a node is visited during a walk operation. If this hook
// returns an error, the node is not visited and the walk operation is aborted.
func WithBefore(fn func(Node) bool) WalkOption {
	return func(options *walkOptions) {
		options.before = fn
	}
}

// WithAfter returns a WalkOption that will cause the given function to be
// invoked after a node (as well as any descendants) is visited during a walk
// operation. If this hook returns an error, the node is not visited and the
// walk operation is aborted.
//
// If the walk is aborted due to some other visitor or before hook returning an
// error, the after hook is still called for all nodes that have been visited.
// However, the walk operation fails with the first error it encountered, so any
// error returned from an after hook is effectively ignored.
func WithAfter(fn func(Node)) WalkOption {
	return func(options *walkOptions) {
		options.after = fn
	}
}

func WithRange(start, end Token) WalkOption {
	return func(options *walkOptions) {
		options.hasRangeRequirement = true
		options.start = start
		options.end = end
	}
}

func WithIntersection(intersects Token) WalkOption {
	return func(options *walkOptions) {
		options.hasIntersectionRequirement = true
		options.intersects = intersects
	}
}

func WithDepthLimit(limit int) WalkOption {
	return func(options *walkOptions) {
		options.depthLimit = limit
	}
}

type inspector struct {
	walkOptions
	fn func(Node) bool
}

func (i *inspector) Before(node Node) bool {
	if i.depthLimit == 0 {
		return false
	}
	i.depthLimit--

	if i.before != nil {
		return i.before(node)
	}
	return true
}

func (i *inspector) After(node Node) {
	i.depthLimit++
	if i.after != nil {
		i.after(node)
	}
}

func (i *inspector) Visit(node Node) Visitor {
	canVisit := true
	if i.hasRangeRequirement {
		if node.Start() > i.end || node.End() < i.start {
			canVisit = false
		}
	}
	if canVisit && i.hasIntersectionRequirement {
		if node.Start() > i.intersects || node.End() < i.intersects {
			canVisit = false
		}
	}

	if canVisit && i.fn(node) {
		return i
	}
	return nil
}

// Inspect traverses an AST in depth-first order: It starts by calling
// f(node); node must not be nil. If f returns true, Inspect invokes f
// recursively for each of the non-nil children of node.
func Inspect(node Node, f func(Node) bool, opts ...WalkOption) {
	wOpts := walkOptions{
		depthLimit: 32,
	}
	for _, opt := range opts {
		opt(&wOpts)
	}
	Walk(&inspector{
		walkOptions: wOpts,
		fn:          f,
	}, node)
}

func AncestorTrackerFromPath(path []Node) *AncestorTracker {
	return &AncestorTracker{
		ancestors: path,
	}
}

// AncestorTracker is used to track the path of nodes during a walk operation.
// By passing AsWalkOptions to a call to Walk, a visitor can inspect the path to
// the node being visited using this tracker.
type AncestorTracker struct {
	ancestors []Node
}

// AsWalkOptions returns WalkOption values that will cause this ancestor tracker
// to track the path through the AST during the walk operation.
func (t *AncestorTracker) AsWalkOptions() []WalkOption {
	return []WalkOption{
		WithBefore(func(n Node) bool {
			t.ancestors = append(t.ancestors, n)
			return true
		}),
		WithAfter(func(n Node) {
			t.ancestors = t.ancestors[:len(t.ancestors)-1]
		}),
	}
}

// Path returns a slice of nodes that represents the path from the root of the
// walk operaiton to the currently visited node. The first element in the path
// is the root supplied to Walk. The last element in the path is the currently
// visited node.
//
// The returned slice is not a defensive copy; so callers should NOT mutate it.
func (t *AncestorTracker) Path() []Node {
	return t.ancestors
}

// Parent returns the parent node of the currently visited node. If the node
// currently being visited is the root supplied to Walk then nil is returned.
func (t *AncestorTracker) Parent() Node {
	if len(t.ancestors) <= 1 {
		return nil
	}
	return t.ancestors[len(t.ancestors)-2]
}

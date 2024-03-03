package ast

import (
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

// WalkOption represents an option used with the Walk function. These
// allow optional before and after hooks to be invoked as each node in
// the tree is visited.
type WalkOption func(*walkOptions)

type walkOptions struct {
	before func(protopath.Values) error
	after  func(protopath.Values) error

	hasRangeRequirement bool
	start, end          Token
	depthLimit          int

	hasIntersectionRequirement bool
	intersects                 Token
}

// WithBefore returns a WalkOption that will cause the given function to be
// invoked before a node is visited during a walk operation. If this hook
// returns an error, the node is not visited and the walk operation is aborted.
func WithBefore(fn func(protopath.Values) error) WalkOption {
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
func WithAfter(fn func(protopath.Values) error) WalkOption {
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

// Inspect traverses an AST in depth-first order: It starts by calling
// f(node); node must not be nil. If f returns true, Inspect invokes f
// recursively for each of the non-nil children of node.
func Inspect(node Node, visit func(Node) bool, opts ...WalkOption) {
	wOpts := walkOptions{
		depthLimit: 32,
	}
	for _, opt := range opts {
		opt(&wOpts)
	}

	check := func(v protopath.Values) (isMessage, isList bool) {
		top := v.Index(-1)
		switch top.Step.Kind() {
		case protopath.RootStep:
			isMessage = true
		case protopath.FieldAccessStep:
			fd := top.Step.FieldDescriptor()
			if fd.IsExtension() {
				isMessage = false
				break
			}
			switch fd.Kind() {
			case protoreflect.MessageKind:
				isMessage = true
				isList = fd.IsList()
			}
		case protopath.ListIndexStep:
			// for list indexes, visit only if the list type is concrete
			prev := v.Index(-2)
			switch prev.Step.Kind() {
			case protopath.FieldAccessStep:
				switch prev.Step.FieldDescriptor().Kind() {
				case protoreflect.MessageKind:
					isMessage = true
				}
			}
		}
		return
	}

	shouldBreak := false
	protorange.Options{
		Stable: true,
	}.Range(
		node.ProtoReflect(),
		func(v protopath.Values) error {
			if len(v.Path) > wOpts.depthLimit {
				return protorange.Break
			}
			if shouldBreak {
				return protorange.Break
			}
			if isMessage, isList := check(v); isMessage {
				if wOpts.before != nil {
					if err := wOpts.before(v); err != nil {
						if err == protorange.Break {
							return nil
						}
						return err
					}
				}
				if !isList {
					node := v.Index(-1).Value.Message().Interface().(Node)
					canVisit := true
					if wOpts.hasRangeRequirement {
						if node.Start() > wOpts.end || node.End() < wOpts.start {
							canVisit = false
						}
					}
					if canVisit && wOpts.hasIntersectionRequirement {
						if node.Start() > wOpts.intersects || node.End() < wOpts.intersects {
							canVisit = false
						}
					}
					if canVisit {
						if !visit(node) {
							shouldBreak = true
						}
					}
				}
			}
			return nil
		},
		func(v protopath.Values) error {
			if shouldBreak {
				shouldBreak = false
				return nil // intentional - don't override error returned from push()
			}
			if isMessage, _ := check(v); isMessage {
				if wOpts.after != nil {
					wOpts.after(v)
				}
			}
			return nil
		},
	)
}

// AncestorTracker is used to track the path of nodes during a walk operation.
// By passing AsWalkOptions to a call to Walk, a visitor can inspect the path to
// the node being visited using this tracker.
type AncestorTracker struct {
	ancestors protopath.Values
}

func nodeIsConcrete(
	values protopath.Values,
	index int,
) bool {
	v := values.Index(index)
	switch v.Step.Kind() {
	case protopath.RootStep:
		return true
	case protopath.FieldAccessStep:
		fld := v.Step.FieldDescriptor()
		switch fld.Kind() {
		case protoreflect.MessageKind:
			if fld.IsList() || fld.IsMap() {
				return false
			}
			switch v.Value.Message().Interface().(type) {
			case WrapperNode:
				return false
			case Node:
				return true
			}
		}
	case protopath.ListIndexStep:
		prev := values.Index(index - 1)
		if prev.Step.Kind() == protopath.FieldAccessStep {
			prevFld := prev.Step.FieldDescriptor()
			if prevFld.Kind() != protoreflect.MessageKind {
				return false
			}
			switch v.Value.Message().Interface().(type) {
			case WrapperNode:
				return false
			case Node:
				return true
			}
		}
	}
	return false
}

// AsWalkOptions returns WalkOption values that will cause this ancestor tracker
// to track the path through the AST during the walk operation.
func (t *AncestorTracker) AsWalkOptions() []WalkOption {
	return []WalkOption{
		WithBefore(func(v protopath.Values) error {
			t.ancestors = v
			if nodeIsConcrete(v, -1) {
				return nil
			}
			return protorange.Break
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
	nodes := make([]Node, 0, len(t.ancestors.Values))
	for i := range len(t.ancestors.Values) {
		if nodeIsConcrete(t.ancestors, i) {
			nodes = append(nodes, t.ancestors.Index(i).Value.Message().Interface().(Node))
		}
	}
	return nodes
}

func (t *AncestorTracker) ProtoPath() protopath.Path {
	return t.ancestors.Path
}

func NodeView(in func(v Node)) func(v protopath.Values) error {
	return func(v protopath.Values) error {
		if nodeIsConcrete(v, -1) {
			in(v.Index(-1).Value.Message().Interface().(Node))
		}
		return nil
	}
}

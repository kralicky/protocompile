package ast

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	"google.golang.org/protobuf/reflect/protoreflect"
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

	check := func(v protopath.Values) (kind protoreflect.Kind, isList bool, err error) {
		top := v.Index(-1)
		switch top.Step.Kind() {
		case protopath.RootStep:
			return protoreflect.MessageKind, false, nil
		case protopath.FieldAccessStep:
			fd := top.Step.FieldDescriptor()
			if fd.IsExtension() {
				return 0, false, protorange.Break
			}
			return fd.Kind(), fd.IsList(), nil
		case protopath.ListIndexStep:
			// for list indexes, visit only if the list type is concrete and
			// not an extension
			prev := v.Index(-2)
			switch prev.Step.Kind() {
			case protopath.FieldAccessStep:
				fd := prev.Step.FieldDescriptor()
				return fd.Kind(), false, nil
			}
		default:
			panic(fmt.Sprintf("ast.Inspect: invalid step kind %q in path: %s"+top.Step.Kind().String(), v.Path))
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
			kind, isList, err := check(v)
			if err != nil {
				return err
			}
			if kind != protoreflect.MessageKind {
				return nil
			}

			if wOpts.before != nil {
				if err := wOpts.before(v); err != nil {
					if err == protorange.Break {
						return nil
					}
					return err
				}
			}

			if isList {
				return nil
			}

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
			return nil
		},
		func(v protopath.Values) error {
			if shouldBreak {
				shouldBreak = false
				return nil // intentional - don't override error returned from push()
			}
			if wOpts.after != nil {
				kind, _, err := check(v)
				if err == nil && kind == protoreflect.MessageKind {
					wOpts.after(v)
				}
			}
			return nil
		},
	)
}

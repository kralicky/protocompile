package paths

import (
	"slices"

	"github.com/kralicky/protocompile/ast"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type PathIndex = struct {
	Step  protopath.Step
	Value protoreflect.Value
}

// NodeIsConcrete returns true if the value at the given index in 'values' is
// a concrete node value, and false otherwise. A concrete node is a message that
// is not a wrapper node (or an array access to a wrapper node), and is not a
// list or map.
func NodeIsConcrete(values protopath.Values, index int) bool {
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
			case ast.WrapperNode:
				return false
			case ast.Node:
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
			case ast.WrapperNode:
				return false
			case ast.Node:
				return true
			}
		}
	}
	return false
}

// Join returns a new path constructed by appending the steps of 'b' to 'a'.
// The first step in 'b' is skipped; it is assumed (but not checked for)
// that 'b' starts with a Root step matching the message type of the last
// step in path 'a'.
func Join[T protopath.Step, S ~[]T](a protopath.Path, b S) protopath.Path {
	p := slices.Clone(a)
	for _, step := range b[1:] {
		p = append(p, protopath.Step(step))
	}
	return p
}

// NodeAt returns the node at the given index in a path.
func NodeAt[T ast.Node](idx PathIndex) (t T) {
	t, _ = idx.Value.Message().Interface().(T)
	return
}

// Slice returns a new path with both its path and values resliced by the given
// range. It is assumed that both slices are of the same length.
func Slice(from protopath.Values, start, end int) protopath.Values {
	// if start is > 0, transform the first step into a Root step with the
	// message type of the first value
	if start > 0 {
		rootStep := protopath.Root(from.Values[start].Message().Descriptor())
		return protopath.Values{
			Path:   append(protopath.Path{rootStep}, from.Path[start+1:end]...),
			Values: from.Values[start:end],
		}
	}
	return protopath.Values{
		Path:   from.Path[start:end],
		Values: from.Values[start:end],
	}
}

// Dereference walks the given path from the root node and returns the node at
// the end of the path.
func Dereference(root ast.Node, path protopath.Path) ast.Node {
	node := protoreflect.ValueOfMessage(root.ProtoReflect())
	for _, step := range path {
		switch step.Kind() {
		case protopath.FieldAccessStep:
			node = node.Message().Get(step.FieldDescriptor())
		case protopath.ListIndexStep:
			node = node.List().Get(step.ListIndex())
		case protopath.RootStep:
			// skip
		}
	}
	return node.Message().Interface().(ast.Node)
}

// DereferenceAll walks the given path from the root node and returns all nodes
// visited. The first node in the returned slice is the root node. Not all
// path steps reference an actual node in the AST, so the returned slice may
// contain fewer than len(path) nodes.
func DereferenceAll(root ast.Node, path protopath.Path) []ast.Node {
	list := []ast.Node{root}
	node := protoreflect.ValueOfMessage(root.ProtoReflect())
	for _, step := range path {
		switch step.Kind() {
		case protopath.FieldAccessStep:
			fd := step.FieldDescriptor()
			switch fd.Kind() {
			case protoreflect.MessageKind:
				node = node.Message().Get(step.FieldDescriptor())
				if !fd.IsList() {
					list = append(list, node.Message().Interface().(ast.Node))
				}
			}
		case protopath.ListIndexStep:
			node = node.List().Get(int(step.ListIndex()))
			list = append(list, node.Message().Interface().(ast.Node))
		case protopath.RootStep:
			// skip; root is already in the list
		}
	}
	return list
}

// ValuesToNodes returns a slice of nodes from the given values, filtering out
// wrapper nodes and other non-node values from the path.
func ValuesToNodes(values protopath.Values) (nodes []ast.Node) {
	for i, v := range values.Values {
		if NodeIsConcrete(values, i) {
			nodes = append(nodes, v.Message().Interface().(ast.Node))
		}
	}
	return
}

// NodeView returns a visitor suitable for use with [ast.WithBefore] and
// [ast.WithAfter] which calls the given function for each concrete node value
// encountered, ignoring wrapper nodes and other non-node values.
func NodeView(in func(v ast.Node)) func(v protopath.Values) error {
	return func(v protopath.Values) error {
		if NodeIsConcrete(v, -1) {
			in(v.Index(-1).Value.Message().Interface().(ast.Node))
		}
		return nil
	}
}

// AncestorTracker is used to track the path of nodes during a walk operation.
// By passing AsWalkOptions to a call to Walk, a visitor can inspect the path to
// the node being visited using this tracker.
type AncestorTracker struct {
	ancestors protopath.Values
}

// AsWalkOptions returns WalkOption values that will cause this ancestor tracker
// to track the path through the AST during the walk operation.
func (t *AncestorTracker) AsWalkOptions() []ast.WalkOption {
	return []ast.WalkOption{
		ast.WithBefore(func(v protopath.Values) error {
			t.ancestors = v
			if NodeIsConcrete(v, -1) {
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
func (t *AncestorTracker) Path() protopath.Path {
	return slices.Clone(t.ancestors.Path)
}

// Values returns a copy of the tracked path and values at the currently visited
// node.
func (t *AncestorTracker) Values() protopath.Values {
	return protopath.Values{
		Path:   slices.Clone(t.ancestors.Path),
		Values: slices.Clone(t.ancestors.Values),
	}
}

// Suffix2 inspects the given values and, if the path logically ends with the
// sequence of nodes 'T, U' (such that U is a message field of T), returns
// the two nodes and true. Otherwise, returns false. This method correctly
// handles non-node path steps such as list accesses. The returned nodes may
// not necessarily be the final 2 nodes in the path.
func Suffix2[T, U ast.Node](values protopath.Values) (
	out struct {
		T      T
		TIndex int
		U      U
		UIndex int
	},
	ok bool,
) {
	if len(values.Path) < 2 {
		return
	}

	var tailIdx int
	// find U
	out.U, tailIdx, ok = initSuffixMatch[U](values)
	if !ok {
		return
	}
	out.UIndex = tailIdx
	tailIdx--

	// walk up to find T
	out.T, ok = suffixMatchRev[T](values, &tailIdx)
	if !ok {
		return
	}
	out.TIndex = tailIdx
	return
}

func Suffix3[T, U, V ast.Node](values protopath.Values) (
	out struct {
		T      T
		TIndex int
		U      U
		UIndex int
		V      V
		VIndex int
	},
	ok bool,
) {
	if len(values.Path) < 3 {
		return
	}

	var tailIdx int
	// find V
	out.V, tailIdx, ok = initSuffixMatch[V](values)
	if !ok {
		return
	}
	out.VIndex = tailIdx
	tailIdx--

	// walk up to find U
	out.U, ok = suffixMatchRev[U](values, &tailIdx)
	if !ok {
		return
	}
	out.UIndex = tailIdx
	tailIdx--

	// walk up to find T
	out.T, ok = suffixMatchRev[T](values, &tailIdx)
	if !ok {
		return
	}
	out.TIndex = tailIdx

	return
}

func Suffix4[T, U, V, W ast.Node](values protopath.Values) (
	out struct {
		T      T
		TIndex int
		U      U
		UIndex int
		V      V
		VIndex int
		W      W
		WIndex int
	},
	ok bool,
) {
	if len(values.Path) < 4 {
		return
	}

	var tailIdx int
	// find W
	out.W, tailIdx, ok = initSuffixMatch[W](values)
	if !ok {
		return
	}
	out.WIndex = tailIdx
	tailIdx--

	// walk up to find V
	out.V, ok = suffixMatchRev[V](values, &tailIdx)
	if !ok {
		return
	}
	out.VIndex = tailIdx
	tailIdx--

	// walk up to find U
	out.U, ok = suffixMatchRev[U](values, &tailIdx)
	if !ok {
		return
	}
	out.UIndex = tailIdx
	tailIdx--

	// walk up to find T
	out.T, ok = suffixMatchRev[T](values, &tailIdx)
	if !ok {
		return
	}
	out.TIndex = tailIdx

	return
}

func Suffix5[T, U, V, W, X ast.Node](values protopath.Values) (
	out struct {
		T      T
		TIndex int
		U      U
		UIndex int
		V      V
		VIndex int
		W      W
		WIndex int
		X      X
		XIndex int
	},
	ok bool,
) {
	if len(values.Path) < 5 {
		return
	}

	var tailIdx int
	// find X
	out.X, tailIdx, ok = initSuffixMatch[X](values)
	if !ok {
		return
	}
	out.XIndex = tailIdx
	tailIdx--

	// walk up to find W
	out.W, ok = suffixMatchRev[W](values, &tailIdx)
	if !ok {
		return
	}
	out.WIndex = tailIdx
	tailIdx--

	// walk up to find V
	out.V, ok = suffixMatchRev[V](values, &tailIdx)
	if !ok {
		return
	}
	out.VIndex = tailIdx
	tailIdx--

	// walk up to find U
	out.U, ok = suffixMatchRev[U](values, &tailIdx)
	if !ok {
		return
	}
	out.UIndex = tailIdx
	tailIdx--

	// walk up to find T
	out.T, ok = suffixMatchRev[T](values, &tailIdx)
	if !ok {
		return
	}
	out.TIndex = tailIdx

	return
}

func initSuffixMatch[T ast.Node](values protopath.Values) (t T, tailIdx int, ok bool) {
	last := values.Index(-1)
	tailIdx = len(values.Path) - 1
	switch last.Step.Kind() {
	case protopath.FieldAccessStep:
		// last.Value MUST be a message type, otherwise the given path is invalid
		t, ok = last.Value.Message().Interface().(T)
	case protopath.ListIndexStep:
		t, ok = last.Value.Message().Interface().(T)
		tailIdx-- // skip over the list index step
	}
	return
}

func suffixMatchRev[T ast.Node](values protopath.Values, tailIdx *int) (_ T, _ bool) {
	for *tailIdx >= 0 {
		prev := values.Index(*tailIdx)
		var t protoreflect.ProtoMessage
		switch prev.Step.Kind() {
		case protopath.RootStep:
			t = prev.Value.Message().Interface()
		case protopath.FieldAccessStep:
			fd := prev.Step.FieldDescriptor()
			if fd.IsList() {
				t = prev.Value.List().Get(int(prev.Step.ListIndex())).Message().Interface()
			} else {
				t = prev.Value.Message().Interface()
			}
		case protopath.ListIndexStep:
			*tailIdx--
			continue
		default:
			panic("unsupported AST step kind: " + prev.Step.Kind().String())
		}

		switch t := t.(type) {
		case T:
			return t, true
		case ast.WrapperNode:
			// if T is actually a wrapper type, the previous case will be chosen
			*tailIdx--
		default:
			return // passed through a different node type before T
		}
	}

	panic("malformed path: " + values.String())
}

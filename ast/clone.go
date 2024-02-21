package ast

import "google.golang.org/protobuf/proto"

// Adapted from golang.org/x/tools/internal/astutil/clone.go

// Clone returns a deep copy of the AST rooted at n.
func Clone[T Node](n T) T {
	return proto.Clone(n).(T)
}

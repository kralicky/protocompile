package ast

import (
	"maps"

	"google.golang.org/protobuf/proto"
)

// Clone returns a deep copy of the AST rooted at n.
// As a special case, when n is a *FileNode, its FileInfo is shared with
// the original node.
func Clone[T Node](n T) T {
	if fileNode, ok := Node(n).(*FileNode); ok {
		fn := &FileNode{
			Pragmas: maps.Clone(fileNode.Pragmas),
			Syntax:  Clone(fileNode.Syntax),
			Edition: Clone(fileNode.Edition),
			Decls:   make([]*FileElement, len(fileNode.Decls)),
			EOF:     Clone(fileNode.EOF),
		}
		for i, decl := range fileNode.Decls {
			fn.Decls[i] = Clone(decl)
		}
		// don't need to clone FileInfo, it's effectively immutable
		proto.SetExtension(fn, E_FileInfo, proto.GetExtension(fileNode, E_FileInfo))
		return Node(fn).(T)
	}
	return proto.Clone(n).(T)
}

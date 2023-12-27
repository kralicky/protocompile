package ast

import (
	"fmt"
)

type virtualSemiContainerNode interface {
	CompositeNode

	interface {
		*EnumNode |
			*OneofNode |
			*MessageNode |
			*ExtendNode |
			*ServiceNode |
			*RPCNode |
			*GroupNode |
			*ArrayLiteralNode |
			*MessageLiteralNode |
			*CompactOptionsNode |
			*MapTypeNode
	}
}

func AddVirtualSemicolon[T virtualSemiContainerNode](node T, semi *RuneNode) {
	if !semi.Virtual {
		panic("expected virtual semicolon node")
	}
	switch node := Node(node).(type) {
	case *EnumNode:
		node.children = append(node.children, semi)
	case *OneofNode:
		node.children = append(node.children, semi)
	case *MessageNode:
		node.children = append(node.children, semi)
	case *ExtendNode:
		node.children = append(node.children, semi)
	case *ServiceNode:
		node.children = append(node.children, semi)
	case *RPCNode:
		node.children = append(node.children, semi)
	case *GroupNode:
		node.children = append(node.children, semi)
	case *ArrayLiteralNode:
		node.children = append(node.children, semi)
	case *MessageLiteralNode:
		node.children = append(node.children, semi)
	case *CompactOptionsNode:
		node.children = append(node.children, semi)
	case *MapTypeNode:
		node.children = append(node.children, semi)
	default:
		panic(fmt.Sprintf("invalid node type: %T", node))
	}
}

func VirtualSemicolon[T virtualSemiContainerNode](node T) *RuneNode {
	var closeRune *RuneNode
	switch node := Node(node).(type) {
	case *EnumNode:
		closeRune = node.CloseBrace
	case *OneofNode:
		closeRune = node.CloseBrace
	case *MessageNode:
		closeRune = node.CloseBrace
	case *ExtendNode:
		closeRune = node.CloseBrace
	case *ServiceNode:
		closeRune = node.CloseBrace
	case *RPCNode:
		closeRune = node.CloseBrace
	case *GroupNode:
		closeRune = node.CloseBrace
	case *ArrayLiteralNode:
		closeRune = node.CloseBracket
	case *MessageLiteralNode:
		closeRune = node.Close
	case *CompactOptionsNode:
		closeRune = node.CloseBracket
	case *MapTypeNode:
		closeRune = node.CloseAngle
	default:
		panic(fmt.Sprintf("invalid node type: %T", node))
	}
	if closeRune != nil && closeRune.Token() < node.End() {
		cs := node.Children()
		if vs, ok := cs[len(cs)-1].(*RuneNode); ok {
			return vs
		}
	}
	return nil
}

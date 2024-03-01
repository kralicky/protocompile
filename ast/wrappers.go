package ast

import reflect "reflect"

func Unwrap(node Node) Node {
	if node == nil {
		return nil
	}
	rn := node.ProtoReflect()
	oneofs := rn.Descriptor().Oneofs()
	for i := range oneofs.Len() {
		o := oneofs.Get(i)
		if o.IsSynthetic() {
			continue
		}
		if fd := rn.WhichOneof(o); fd != nil {
			return rn.Get(fd).Message().Interface().(Node)
		}
	}
	return node
}

type AnyValueNode interface {
	Node
	AsValueNode() *ValueNode
	Value() any
}

func (n *ValueNode) Unwrap() AnyValueNode {
	switch n := n.GetVal().(type) {
	case *ValueNode_Ident:
		return n.Ident
	case *ValueNode_CompoundIdent:
		return n.CompoundIdent
	case *ValueNode_StringLiteral:
		return n.StringLiteral
	case *ValueNode_CompoundStringLiteral:
		return n.CompoundStringLiteral
	case *ValueNode_UintLiteral:
		return n.UintLiteral
	case *ValueNode_NegativeIntLiteral:
		return n.NegativeIntLiteral
	case *ValueNode_FloatLiteral:
		return n.FloatLiteral
	case *ValueNode_SpecialFloatLiteral:
		return n.SpecialFloatLiteral
	case *ValueNode_SignedFloatLiteral:
		return n.SignedFloatLiteral
	case *ValueNode_ArrayLiteral:
		return n.ArrayLiteral
	case *ValueNode_MessageLiteral:
		return n.MessageLiteral
	}
	return nil
}

func (n *ValueNode) HasValue() bool {
	return n.GetVal() == nil || reflect.ValueOf(n.Val).IsNil()
}

type AnyStringValueNode interface {
	Node
	AsStringValueNode() *StringValueNode
	AsString() string
}

func (n *StringValueNode) Unwrap() AnyStringValueNode {
	switch n := n.GetVal().(type) {
	case *StringValueNode_StringLiteral:
		return n.StringLiteral
	case *StringValueNode_CompoundStringLiteral:
		return n.CompoundStringLiteral
	}
	return nil
}

func (s *StringValueNode) AsValueNode() *ValueNode {
	switch u := s.Unwrap().(type) {
	case *StringLiteralNode:
		return u.AsValueNode()
	case *CompoundStringLiteralNode:
		return u.AsValueNode()
	}
	return nil
}

func (n *StringLiteralNode) AsStringValueNode() *StringValueNode {
	return &StringValueNode{
		Val: &StringValueNode_StringLiteral{
			StringLiteral: n,
		},
	}
}

func (n *StringLiteralNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_StringLiteral{
			StringLiteral: n,
		},
	}
}

func (n *CompoundStringLiteralNode) AsStringValueNode() *StringValueNode {
	return &StringValueNode{
		Val: &StringValueNode_CompoundStringLiteral{
			CompoundStringLiteral: n,
		},
	}
}

func (n *CompoundStringLiteralNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_CompoundStringLiteral{
			CompoundStringLiteral: n,
		},
	}
}

type AnyIntValueNode interface {
	Node
	AsIntValueNode() *IntValueNode
	AsInt64() (int64, bool)
	AsUint64() (uint64, bool)
	Value() any
}

func (n *IntValueNode) Unwrap() AnyIntValueNode {
	switch n := n.GetVal().(type) {
	case *IntValueNode_UintLiteral:
		return n.UintLiteral
	case *IntValueNode_NegativeIntLiteral:
		return n.NegativeIntLiteral
	}
	return nil
}

func (n *UintLiteralNode) AsIntValueNode() *IntValueNode {
	return &IntValueNode{
		Val: &IntValueNode_UintLiteral{
			UintLiteral: n,
		},
	}
}

func (n *NegativeIntLiteralNode) AsIntValueNode() *IntValueNode {
	return &IntValueNode{
		Val: &IntValueNode_NegativeIntLiteral{
			NegativeIntLiteral: n,
		},
	}
}

type AnyFloatValueNode interface {
	Node
	AsFloatValueNode() *FloatValueNode
	AsFloat() float64
}

func (n *FloatValueNode) Unwrap() AnyFloatValueNode {
	switch n := n.GetVal().(type) {
	case *FloatValueNode_FloatLiteral:
		return n.FloatLiteral
	case *FloatValueNode_SpecialFloatLiteral:
		return n.SpecialFloatLiteral
	case *FloatValueNode_UintLiteral:
		return n.UintLiteral
	}
	return nil
}

func (n *ArrayLiteralNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_ArrayLiteral{
			ArrayLiteral: n,
		},
	}
}

func (n *MessageLiteralNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_MessageLiteral{
			MessageLiteral: n,
		},
	}
}

type AnyFileElement interface {
	Node
	AsFileElement() *FileElement
}

func (n *FileElement) Unwrap() Node {
	switch n := n.GetVal().(type) {
	case *FileElement_Import:
		return n.Import
	case *FileElement_Package:
		return n.Package
	case *FileElement_Option:
		return n.Option
	case *FileElement_Message:
		return n.Message
	case *FileElement_Enum:
		return n.Enum
	case *FileElement_Extend:
		return n.Extend
	case *FileElement_Service:
		return n.Service
	case *FileElement_Err:
		return n.Err
	}
	return nil
}

func (n *ImportNode) AsFileElement() *FileElement {
	return &FileElement{Val: &FileElement_Import{Import: n}}
}

func (n *PackageNode) AsFileElement() *FileElement {
	return &FileElement{Val: &FileElement_Package{Package: n}}
}

func (n *OptionNode) AsFileElement() *FileElement {
	return &FileElement{Val: &FileElement_Option{Option: n}}
}

func (n *MessageNode) AsFileElement() *FileElement {
	return &FileElement{Val: &FileElement_Message{Message: n}}
}

func (n *EnumNode) AsFileElement() *FileElement {
	return &FileElement{Val: &FileElement_Enum{Enum: n}}
}

func (n *ExtendNode) AsFileElement() *FileElement {
	return &FileElement{Val: &FileElement_Extend{Extend: n}}
}

func (n *ServiceNode) AsFileElement() *FileElement {
	return &FileElement{Val: &FileElement_Service{Service: n}}
}

func (n *ErrorNode) AsFileElement() *FileElement {
	return &FileElement{Val: &FileElement_Err{Err: n}}
}

func (n *FloatLiteralNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_FloatLiteral{
			FloatLiteral: n,
		},
	}
}

func (n *SignedFloatLiteralNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_SignedFloatLiteral{
			SignedFloatLiteral: n,
		},
	}
}

func (n *UintLiteralNode) AsFloatValueNode() *FloatValueNode {
	return &FloatValueNode{
		Val: &FloatValueNode_UintLiteral{
			UintLiteral: n,
		},
	}
}

func (n *UintLiteralNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_UintLiteral{
			UintLiteral: n,
		},
	}
}

func (n *IdentNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_Ident{
			Ident: n,
		},
	}
}

func (n *IdentNode) AsIdentValueNode() *IdentValueNode {
	return &IdentValueNode{
		Val: &IdentValueNode_Ident{
			Ident: n,
		},
	}
}

func (n *CompoundIdentNode) AsIdentValueNode() *IdentValueNode {
	return &IdentValueNode{
		Val: &IdentValueNode_CompoundIdent{
			CompoundIdent: n,
		},
	}
}

func (n *CompoundIdentNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_CompoundIdent{
			CompoundIdent: n,
		},
	}
}

type AnyIdentValueNode interface {
	Node
	AsIdentValueNode() *IdentValueNode
	AsIdentifier() Identifier
}

func (n *IdentValueNode) Unwrap() AnyIdentValueNode {
	switch val := n.GetVal().(type) {
	case *IdentValueNode_Ident:
		return val.Ident
	case *IdentValueNode_CompoundIdent:
		return val.CompoundIdent
	}
	return nil
}

func (n *NegativeIntLiteralNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_NegativeIntLiteral{
			NegativeIntLiteral: n,
		},
	}
}

func (n *SpecialFloatLiteralNode) AsFloatValueNode() *FloatValueNode {
	return &FloatValueNode{
		Val: &FloatValueNode_SpecialFloatLiteral{
			SpecialFloatLiteral: n,
		},
	}
}

func (n *SpecialFloatLiteralNode) AsValueNode() *ValueNode {
	return &ValueNode{
		Val: &ValueNode_SpecialFloatLiteral{
			SpecialFloatLiteral: n,
		},
	}
}

func (n *FloatLiteralNode) AsFloatValueNode() *FloatValueNode {
	return &FloatValueNode{
		Val: &FloatValueNode_FloatLiteral{
			FloatLiteral: n,
		},
	}
}

func (n *OptionNode) AsOneofElement() *OneofElement {
	return &OneofElement{Val: &OneofElement_Option{Option: n}}
}

func (n *FieldNode) AsOneofElement() *OneofElement {
	return &OneofElement{Val: &OneofElement_Field{Field: n}}
}

func (n *GroupNode) AsOneofElement() *OneofElement {
	return &OneofElement{Val: &OneofElement_Group{Group: n}}
}

type AnyEnumElement interface {
	Node
	AsEnumElement() *EnumElement
}

func (n *EnumElement) Unwrap() AnyEnumElement {
	switch n := n.GetVal().(type) {
	case *EnumElement_Option:
		return n.Option
	case *EnumElement_EnumValue:
		return n.EnumValue
	case *EnumElement_Reserved:
		return n.Reserved
	}
	return nil
}

func (n *EnumValueNode) AsEnumElement() *EnumElement {
	return &EnumElement{Val: &EnumElement_EnumValue{EnumValue: n}}
}

func (n *OptionNode) AsEnumElement() *EnumElement {
	return &EnumElement{Val: &EnumElement_Option{Option: n}}
}

func (n *ReservedNode) AsEnumElement() *EnumElement {
	return &EnumElement{Val: &EnumElement_Reserved{Reserved: n}}
}

type AnyMessageDeclNode interface {
	Node
	AsMessageDeclNode() *MessageDeclNode
	GetName() *IdentNode
}

func (n *MessageDeclNode) Unwrap() AnyMessageDeclNode {
	switch n := n.GetVal().(type) {
	case *MessageDeclNode_Message:
		return n.Message
	case *MessageDeclNode_Group:
		return n.Group
	case *MessageDeclNode_MapField:
		return n.MapField
	}
	return nil
}

func (n *MessageNode) AsMessageDeclNode() *MessageDeclNode {
	return &MessageDeclNode{Val: &MessageDeclNode_Message{Message: n}}
}

func (n *GroupNode) AsMessageDeclNode() *MessageDeclNode {
	return &MessageDeclNode{Val: &MessageDeclNode_Group{Group: n}}
}

func (n *MapFieldNode) AsMessageDeclNode() *MessageDeclNode {
	return &MessageDeclNode{Val: &MessageDeclNode_MapField{MapField: n}}
}

type AnyMessageElement interface {
	Node
	AsMessageElement() *MessageElement
}

func (n *MessageElement) Unwrap() AnyMessageElement {
	switch n := n.GetVal().(type) {
	case *MessageElement_Option:
		return n.Option
	case *MessageElement_Field:
		return n.Field
	case *MessageElement_MapField:
		return n.MapField
	case *MessageElement_Oneof:
		return n.Oneof
	case *MessageElement_Group:
		return n.Group
	case *MessageElement_Message:
		return n.Message
	case *MessageElement_Enum:
		return n.Enum
	case *MessageElement_Extend:
		return n.Extend
	case *MessageElement_ExtensionRange:
		return n.ExtensionRange
	case *MessageElement_Reserved:
		return n.Reserved
	case *MessageElement_Empty:
		return n.Empty
	}
	return nil
}

func (n *OptionNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_Option{Option: n}}
}

func (n *FieldNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_Field{Field: n}}
}

func (n *MapFieldNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_MapField{MapField: n}}
}

func (n *OneofNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_Oneof{Oneof: n}}
}

func (n *GroupNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_Group{Group: n}}
}

func (n *MessageNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_Message{Message: n}}
}

func (n *EnumNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_Enum{Enum: n}}
}

func (n *ExtendNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_Extend{Extend: n}}
}

func (n *ExtensionRangeNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_ExtensionRange{ExtensionRange: n}}
}

func (n *ReservedNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_Reserved{Reserved: n}}
}

func (n *EmptyDeclNode) AsMessageElement() *MessageElement {
	return &MessageElement{Val: &MessageElement_Empty{Empty: n}}
}

type AnyExtendElement interface {
	Node
	AsExtendElement() *ExtendElement
}

func (n *ExtendElement) Unwrap() AnyExtendElement {
	switch n := n.GetVal().(type) {
	case *ExtendElement_Field:
		return n.Field
	case *ExtendElement_Group:
		return n.Group
	case *ExtendElement_Empty:
		return n.Empty
	}
	return nil
}

func (n *FieldNode) AsExtendElement() *ExtendElement {
	return &ExtendElement{Val: &ExtendElement_Field{Field: n}}
}

func (n *GroupNode) AsExtendElement() *ExtendElement {
	return &ExtendElement{Val: &ExtendElement_Group{Group: n}}
}

func (n *EmptyDeclNode) AsExtendElement() *ExtendElement {
	return &ExtendElement{Val: &ExtendElement_Empty{Empty: n}}
}

type AnyServiceElement interface {
	Node
	AsServiceElement() *ServiceElement
}

func (n *ServiceElement) Unwrap() AnyServiceElement {
	switch n := n.GetVal().(type) {
	case *ServiceElement_Rpc:
		return n.Rpc
	case *ServiceElement_Option:
		return n.Option
	}
	return nil
}

func (n *RPCNode) AsServiceElement() *ServiceElement {
	return &ServiceElement{Val: &ServiceElement_Rpc{Rpc: n}}
}

func (n *OptionNode) AsServiceElement() *ServiceElement {
	return &ServiceElement{Val: &ServiceElement_Option{Option: n}}
}

type AnyRPCElement interface {
	Node
	AsRPCElement() *RPCElement
}

func (n *RPCElement) Unwrap() AnyRPCElement {
	switch n := n.GetVal().(type) {
	case *RPCElement_Option:
		return n.Option
	}
	return nil
}

func (n *OptionNode) AsRPCElement() *RPCElement {
	return &RPCElement{Val: &RPCElement_Option{Option: n}}
}

type AnyFieldDeclNode interface {
	Node
	AsFieldDeclNode() *FieldDeclNode
	GetLabel() *IdentNode
	GetName() *IdentNode
	GetFieldTypeNode() Node
	GetTag() *UintLiteralNode
	GetOptions() *CompactOptionsNode
}

func (f *FieldDeclNode) Unwrap() AnyFieldDeclNode {
	switch f := f.GetVal().(type) {
	case *FieldDeclNode_Field:
		return f.Field
	case *FieldDeclNode_Group:
		return f.Group
	case *FieldDeclNode_MapField:
		return f.MapField
	case *FieldDeclNode_SyntheticMapField:
		return f.SyntheticMapField
	default:
		return nil
	}
}

func (n *FieldNode) AsFieldDeclNode() *FieldDeclNode {
	return &FieldDeclNode{Val: &FieldDeclNode_Field{Field: n}}
}

func (n *GroupNode) AsFieldDeclNode() *FieldDeclNode {
	return &FieldDeclNode{Val: &FieldDeclNode_Group{Group: n}}
}

func (n *MapFieldNode) AsFieldDeclNode() *FieldDeclNode {
	return &FieldDeclNode{Val: &FieldDeclNode_MapField{MapField: n}}
}

func (n *SyntheticMapField) AsFieldDeclNode() *FieldDeclNode {
	return &FieldDeclNode{Val: &FieldDeclNode_SyntheticMapField{SyntheticMapField: n}}
}

type AnyOneofElement interface {
	Node
	AsOneofElement() *OneofElement
}

func (n *OneofElement) Unwrap() AnyOneofElement {
	switch n := n.GetVal().(type) {
	case *OneofElement_Option:
		return n.Option
	case *OneofElement_Field:
		return n.Field
	case *OneofElement_Group:
		return n.Group
	}
	return nil
}

func (n *OneofNode) GetElements() []*OneofElement {
	return n.GetDecls()
}

func (n *EnumNode) GetElements() []*EnumElement {
	return n.GetDecls()
}

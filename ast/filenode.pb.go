// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.32.0
// 	protoc        (unknown)
// source: github.com/kralicky/protocompile/ast/filenode.proto

package ast

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// FileNode is the root of the AST hierarchy. It represents an entire
// protobuf source file.
type FileNode struct {
	state           protoimpl.MessageState
	sizeCache       protoimpl.SizeCache
	unknownFields   protoimpl.UnknownFields
	extensionFields protoimpl.ExtensionFields

	// A map of implementation-specific key-value pairs parsed from comments on
	// the syntax or edition declaration. These work like the //go: comments in
	// Go source files.
	Syntax *SyntaxNode `protobuf:"bytes,1,opt,name=syntax" json:"syntax,omitempty"`
	// A file has either a Syntax or Edition node, never both.
	// If both are nil, neither declaration is present and the
	// file is assumed to use "proto2" syntax.
	Edition *EditionNode   `protobuf:"bytes,2,opt,name=edition" json:"edition,omitempty"`
	Decls   []*FileElement `protobuf:"bytes,3,rep,name=decls" json:"decls,omitempty"`
	// This synthetic node allows access to final comments and whitespace
	EOF *RuneNode `protobuf:"bytes,4,opt,name=EOF" json:"EOF,omitempty"`
}

func (x *FileNode) Reset() {
	*x = FileNode{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_kralicky_protocompile_ast_filenode_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileNode) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileNode) ProtoMessage() {}

func (x *FileNode) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_kralicky_protocompile_ast_filenode_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileNode.ProtoReflect.Descriptor instead.
func (*FileNode) Descriptor() ([]byte, []int) {
	return file_github_com_kralicky_protocompile_ast_filenode_proto_rawDescGZIP(), []int{0}
}

func (x *FileNode) GetSyntax() *SyntaxNode {
	if x != nil {
		return x.Syntax
	}
	return nil
}

func (x *FileNode) GetEdition() *EditionNode {
	if x != nil {
		return x.Edition
	}
	return nil
}

func (x *FileNode) GetDecls() []*FileElement {
	if x != nil {
		return x.Decls
	}
	return nil
}

func (x *FileNode) GetEOF() *RuneNode {
	if x != nil {
		return x.EOF
	}
	return nil
}

type ExtendedAttributes struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Pragmas map[string]string `protobuf:"bytes,1,rep,name=pragmas" json:"pragmas,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
}

func (x *ExtendedAttributes) Reset() {
	*x = ExtendedAttributes{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_kralicky_protocompile_ast_filenode_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ExtendedAttributes) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ExtendedAttributes) ProtoMessage() {}

func (x *ExtendedAttributes) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_kralicky_protocompile_ast_filenode_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ExtendedAttributes.ProtoReflect.Descriptor instead.
func (*ExtendedAttributes) Descriptor() ([]byte, []int) {
	return file_github_com_kralicky_protocompile_ast_filenode_proto_rawDescGZIP(), []int{1}
}

func (x *ExtendedAttributes) GetPragmas() map[string]string {
	if x != nil {
		return x.Pragmas
	}
	return nil
}

var file_github_com_kralicky_protocompile_ast_filenode_proto_extTypes = []protoimpl.ExtensionInfo{
	{
		ExtendedType:  (*FileNode)(nil),
		ExtensionType: (*FileInfo)(nil),
		Field:         7,
		Name:          "ast.fileInfo",
		Tag:           "bytes,7,opt,name=fileInfo",
		Filename:      "github.com/kralicky/protocompile/ast/filenode.proto",
	},
	{
		ExtendedType:  (*FileNode)(nil),
		ExtensionType: (*ExtendedAttributes)(nil),
		Field:         8,
		Name:          "ast.extendedAttributes",
		Tag:           "bytes,8,opt,name=extendedAttributes",
		Filename:      "github.com/kralicky/protocompile/ast/filenode.proto",
	},
}

// Extension fields to FileNode.
var (
	// optional ast.FileInfo fileInfo = 7;
	E_FileInfo = &file_github_com_kralicky_protocompile_ast_filenode_proto_extTypes[0]
	// optional ast.ExtendedAttributes extendedAttributes = 8;
	E_ExtendedAttributes = &file_github_com_kralicky_protocompile_ast_filenode_proto_extTypes[1]
)

var File_github_com_kralicky_protocompile_ast_filenode_proto protoreflect.FileDescriptor

var file_github_com_kralicky_protocompile_ast_filenode_proto_rawDesc = []byte{
	0x0a, 0x33, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6b, 0x72, 0x61,
	0x6c, 0x69, 0x63, 0x6b, 0x79, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6d, 0x70, 0x69,
	0x6c, 0x65, 0x2f, 0x61, 0x73, 0x74, 0x2f, 0x66, 0x69, 0x6c, 0x65, 0x6e, 0x6f, 0x64, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x03, 0x61, 0x73, 0x74, 0x1a, 0x2e, 0x67, 0x69, 0x74, 0x68,
	0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6b, 0x72, 0x61, 0x6c, 0x69, 0x63, 0x6b, 0x79, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65, 0x2f, 0x61, 0x73, 0x74,
	0x2f, 0x61, 0x73, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xb4, 0x01, 0x0a, 0x08, 0x46,
	0x69, 0x6c, 0x65, 0x4e, 0x6f, 0x64, 0x65, 0x12, 0x27, 0x0a, 0x06, 0x73, 0x79, 0x6e, 0x74, 0x61,
	0x78, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x61, 0x73, 0x74, 0x2e, 0x53, 0x79,
	0x6e, 0x74, 0x61, 0x78, 0x4e, 0x6f, 0x64, 0x65, 0x52, 0x06, 0x73, 0x79, 0x6e, 0x74, 0x61, 0x78,
	0x12, 0x2a, 0x0a, 0x07, 0x65, 0x64, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x10, 0x2e, 0x61, 0x73, 0x74, 0x2e, 0x45, 0x64, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x4e,
	0x6f, 0x64, 0x65, 0x52, 0x07, 0x65, 0x64, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x26, 0x0a, 0x05,
	0x64, 0x65, 0x63, 0x6c, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x10, 0x2e, 0x61, 0x73,
	0x74, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x45, 0x6c, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x52, 0x05, 0x64,
	0x65, 0x63, 0x6c, 0x73, 0x12, 0x1f, 0x0a, 0x03, 0x45, 0x4f, 0x46, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x0d, 0x2e, 0x61, 0x73, 0x74, 0x2e, 0x52, 0x75, 0x6e, 0x65, 0x4e, 0x6f, 0x64, 0x65,
	0x52, 0x03, 0x45, 0x4f, 0x46, 0x2a, 0x04, 0x08, 0x07, 0x10, 0x08, 0x2a, 0x04, 0x08, 0x08, 0x10,
	0x09, 0x22, 0x90, 0x01, 0x0a, 0x12, 0x45, 0x78, 0x74, 0x65, 0x6e, 0x64, 0x65, 0x64, 0x41, 0x74,
	0x74, 0x72, 0x69, 0x62, 0x75, 0x74, 0x65, 0x73, 0x12, 0x3e, 0x0a, 0x07, 0x70, 0x72, 0x61, 0x67,
	0x6d, 0x61, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x24, 0x2e, 0x61, 0x73, 0x74, 0x2e,
	0x45, 0x78, 0x74, 0x65, 0x6e, 0x64, 0x65, 0x64, 0x41, 0x74, 0x74, 0x72, 0x69, 0x62, 0x75, 0x74,
	0x65, 0x73, 0x2e, 0x50, 0x72, 0x61, 0x67, 0x6d, 0x61, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52,
	0x07, 0x70, 0x72, 0x61, 0x67, 0x6d, 0x61, 0x73, 0x1a, 0x3a, 0x0a, 0x0c, 0x50, 0x72, 0x61, 0x67,
	0x6d, 0x61, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61,
	0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65,
	0x3a, 0x02, 0x38, 0x01, 0x3a, 0x38, 0x0a, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x49, 0x6e, 0x66, 0x6f,
	0x12, 0x0d, 0x2e, 0x61, 0x73, 0x74, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4e, 0x6f, 0x64, 0x65, 0x18,
	0x07, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0d, 0x2e, 0x61, 0x73, 0x74, 0x2e, 0x46, 0x69, 0x6c, 0x65,
	0x49, 0x6e, 0x66, 0x6f, 0x52, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x3a, 0x56,
	0x0a, 0x12, 0x65, 0x78, 0x74, 0x65, 0x6e, 0x64, 0x65, 0x64, 0x41, 0x74, 0x74, 0x72, 0x69, 0x62,
	0x75, 0x74, 0x65, 0x73, 0x12, 0x0d, 0x2e, 0x61, 0x73, 0x74, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4e,
	0x6f, 0x64, 0x65, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x61, 0x73, 0x74, 0x2e,
	0x45, 0x78, 0x74, 0x65, 0x6e, 0x64, 0x65, 0x64, 0x41, 0x74, 0x74, 0x72, 0x69, 0x62, 0x75, 0x74,
	0x65, 0x73, 0x52, 0x12, 0x65, 0x78, 0x74, 0x65, 0x6e, 0x64, 0x65, 0x64, 0x41, 0x74, 0x74, 0x72,
	0x69, 0x62, 0x75, 0x74, 0x65, 0x73, 0x42, 0x26, 0x5a, 0x24, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6b, 0x72, 0x61, 0x6c, 0x69, 0x63, 0x6b, 0x79, 0x2f, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65, 0x2f, 0x61, 0x73, 0x74,
}

var (
	file_github_com_kralicky_protocompile_ast_filenode_proto_rawDescOnce sync.Once
	file_github_com_kralicky_protocompile_ast_filenode_proto_rawDescData = file_github_com_kralicky_protocompile_ast_filenode_proto_rawDesc
)

func file_github_com_kralicky_protocompile_ast_filenode_proto_rawDescGZIP() []byte {
	file_github_com_kralicky_protocompile_ast_filenode_proto_rawDescOnce.Do(func() {
		file_github_com_kralicky_protocompile_ast_filenode_proto_rawDescData = protoimpl.X.CompressGZIP(file_github_com_kralicky_protocompile_ast_filenode_proto_rawDescData)
	})
	return file_github_com_kralicky_protocompile_ast_filenode_proto_rawDescData
}

var file_github_com_kralicky_protocompile_ast_filenode_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_github_com_kralicky_protocompile_ast_filenode_proto_goTypes = []interface{}{
	(*FileNode)(nil),           // 0: ast.FileNode
	(*ExtendedAttributes)(nil), // 1: ast.ExtendedAttributes
	nil,                        // 2: ast.ExtendedAttributes.PragmasEntry
	(*SyntaxNode)(nil),         // 3: ast.SyntaxNode
	(*EditionNode)(nil),        // 4: ast.EditionNode
	(*FileElement)(nil),        // 5: ast.FileElement
	(*RuneNode)(nil),           // 6: ast.RuneNode
	(*FileInfo)(nil),           // 7: ast.FileInfo
}
var file_github_com_kralicky_protocompile_ast_filenode_proto_depIdxs = []int32{
	3, // 0: ast.FileNode.syntax:type_name -> ast.SyntaxNode
	4, // 1: ast.FileNode.edition:type_name -> ast.EditionNode
	5, // 2: ast.FileNode.decls:type_name -> ast.FileElement
	6, // 3: ast.FileNode.EOF:type_name -> ast.RuneNode
	2, // 4: ast.ExtendedAttributes.pragmas:type_name -> ast.ExtendedAttributes.PragmasEntry
	0, // 5: ast.fileInfo:extendee -> ast.FileNode
	0, // 6: ast.extendedAttributes:extendee -> ast.FileNode
	7, // 7: ast.fileInfo:type_name -> ast.FileInfo
	1, // 8: ast.extendedAttributes:type_name -> ast.ExtendedAttributes
	9, // [9:9] is the sub-list for method output_type
	9, // [9:9] is the sub-list for method input_type
	7, // [7:9] is the sub-list for extension type_name
	5, // [5:7] is the sub-list for extension extendee
	0, // [0:5] is the sub-list for field type_name
}

func init() { file_github_com_kralicky_protocompile_ast_filenode_proto_init() }
func file_github_com_kralicky_protocompile_ast_filenode_proto_init() {
	if File_github_com_kralicky_protocompile_ast_filenode_proto != nil {
		return
	}
	file_github_com_kralicky_protocompile_ast_ast_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_github_com_kralicky_protocompile_ast_filenode_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FileNode); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			case 3:
				return &v.extensionFields
			default:
				return nil
			}
		}
		file_github_com_kralicky_protocompile_ast_filenode_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ExtendedAttributes); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_github_com_kralicky_protocompile_ast_filenode_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 2,
			NumServices:   0,
		},
		GoTypes:           file_github_com_kralicky_protocompile_ast_filenode_proto_goTypes,
		DependencyIndexes: file_github_com_kralicky_protocompile_ast_filenode_proto_depIdxs,
		MessageInfos:      file_github_com_kralicky_protocompile_ast_filenode_proto_msgTypes,
		ExtensionInfos:    file_github_com_kralicky_protocompile_ast_filenode_proto_extTypes,
	}.Build()
	File_github_com_kralicky_protocompile_ast_filenode_proto = out.File
	file_github_com_kralicky_protocompile_ast_filenode_proto_rawDesc = nil
	file_github_com_kralicky_protocompile_ast_filenode_proto_goTypes = nil
	file_github_com_kralicky_protocompile_ast_filenode_proto_depIdxs = nil
}

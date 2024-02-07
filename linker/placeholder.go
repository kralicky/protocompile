package linker

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type placeholderFile struct {
	protoreflect.FileDescriptor
}

// Dependencies implements File.
func (placeholderFile) Dependencies() Files {
	return nil
}

// FindDescriptorByName implements File.
func (placeholderFile) FindDescriptorByName(name protoreflect.FullName) protoreflect.Descriptor {
	return nil
}

// FindExtensionByNumber implements File.
func (placeholderFile) FindExtensionByNumber(message protoreflect.FullName, tag protowire.Number) protoreflect.ExtensionTypeDescriptor {
	return nil
}

// FindImportByPath implements File.
func (placeholderFile) FindImportByPath(path string) File {
	return nil
}

// NewPlaceholderFile returns a new placeholder File. Its FileDescriptor is a
// valid instance of the internal filedesc.PlaceholderFile with the given path.
func NewPlaceholderFile(path string) File {
	fdp := descriptorpb.FileDescriptorProto{
		Name:       proto.String("placeholder"),
		Dependency: []string{path},
	}
	f, err := protodesc.FileOptions{
		AllowUnresolvable: true,
	}.New(&fdp, nil)
	if err != nil {
		panic(err)
	}
	return placeholderFile{
		FileDescriptor: f.Imports().Get(0),
	}
}

func NewPlaceholderMessage(name protoreflect.FullName) protoreflect.MessageDescriptor {
	fdp := descriptorpb.FileDescriptorProto{
		Name: proto.String("placeholder"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Placeholder"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("placeholder"),
						Number:   proto.Int32(1),
						TypeName: proto.String("." + string(name)),
					},
				},
			},
		},
	}
	f, err := protodesc.FileOptions{
		AllowUnresolvable: true,
	}.New(&fdp, nil)
	if err != nil {
		panic(err)
	}
	return f.Messages().Get(0).Fields().Get(0).Message()
}

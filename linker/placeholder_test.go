package linker_test

import (
	"testing"

	"github.com/kralicky/protocompile/linker"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestNewPlaceholderFile(t *testing.T) {
	path := "path/to/dependency.proto"
	f := linker.NewPlaceholderFile(path)

	if !f.IsPlaceholder() {
		t.Errorf("Expected IsPlaceholder() to be true, got false")
	}

	if got := f.Path(); got != path {
		t.Errorf("Expected path to be %q, got %q", path, got)
	}
}

func TestNewPlaceholderMessage(t *testing.T) {
	for _, msg := range []protoreflect.FullName{
		"Foo",
		"foo.Bar",
		"foo.bar.Baz",
	} {
		m := linker.NewPlaceholderMessage(msg)

		if !m.IsPlaceholder() {
			t.Errorf("Expected IsPlaceholder() to be true, got false")
		}

		if got := m.FullName(); got != protoreflect.FullName(msg) {
			t.Errorf("Expected FullName().String() to be %q, got %q", msg, got)
		}
	}
}

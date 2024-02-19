package ast

import "reflect"

// Adapted from golang.org/x/tools/internal/astutil/clone.go

// Clone returns a deep copy of the AST rooted at n.
func Clone[T Node](n T) T {
	var clone func(x reflect.Value) reflect.Value
	set := func(dst, src reflect.Value) {
		src = clone(src)
		if src.IsValid() {
			dst.Set(src)
		}
	}
	clone = func(orig reflect.Value) reflect.Value {
		switch orig.Kind() {
		case reflect.Ptr:
			if orig.IsNil() {
				return orig
			}
			cloned := reflect.New(orig.Type().Elem())
			set(cloned.Elem(), orig.Elem())
			return cloned
		case reflect.Struct:
			cloned := reflect.New(orig.Type()).Elem()
			for i := 0; i < orig.Type().NumField(); i++ {
				set(cloned.Field(i), orig.Field(i))
			}
			return cloned
		case reflect.Slice:
			if orig.IsNil() {
				return orig
			}
			cloned := reflect.MakeSlice(orig.Type(), orig.Len(), orig.Cap())
			for i := 0; i < orig.Len(); i++ {
				set(cloned.Index(i), orig.Index(i))
			}
			return cloned
		case reflect.Interface:
			cloned := reflect.New(orig.Type()).Elem()
			set(cloned, orig.Elem())
			return cloned

		default:
			return orig
		}
	}
	return clone(reflect.ValueOf(n)).Interface().(T)
}

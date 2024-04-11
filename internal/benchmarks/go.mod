module github.com/kralicky/protocompile/internal/benchmarks

go 1.22.0

require (
	github.com/igrmk/treemap/v2 v2.0.1
	github.com/jhump/protoreflect v1.14.1 // MUST NOT be updated to v1.15 or higher
	github.com/kralicky/protocompile v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.9.0
	golang.org/x/sync v0.6.0
	google.golang.org/protobuf v1.33.1-0.20240408130810-98873a205002
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/kr/pretty v0.2.1 // indirect
	github.com/kralicky/codegen v0.0.0-20240307225947-51de80fcb2f3 // indirect
	github.com/plar/go-adaptive-radix-tree v1.0.5 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/exp v0.0.0-20220317015231-48e79f11773a // indirect
	golang.org/x/net v0.17.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/kralicky/protocompile => ../../

module github.com/kralicky/protocompile

go 1.22.0

require (
	github.com/google/go-cmp v0.6.0
	github.com/kralicky/codegen v0.0.0-20240522012557-5193d3fdbeca
	github.com/kralicky/go-adaptive-radix-tree v0.0.0-20240619012453-a8f80032ba31
	github.com/stretchr/testify v1.9.0
	golang.org/x/sync v0.7.0
	google.golang.org/protobuf v1.34.2
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

retract v0.5.0 // Contains deadlock error

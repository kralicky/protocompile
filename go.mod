module github.com/bufbuild/protocompile

go 1.21

require (
	github.com/google/go-cmp v0.6.0
	github.com/plar/go-adaptive-radix-tree v1.0.5
	github.com/stretchr/testify v1.8.4
	golang.org/x/sync v0.4.0
	google.golang.org/protobuf v1.31.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

retract v0.5.0 // Contains deadlock error

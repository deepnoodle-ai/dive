module github.com/deepnoodle-ai/dive/a2a

go 1.25.0

toolchain go1.26.5

require (
	github.com/a2aproject/a2a-go/v2 v2.4.0
	github.com/deepnoodle-ai/dive v1.23.0
	github.com/deepnoodle-ai/wonton v0.0.37
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace github.com/deepnoodle-ai/dive => ..

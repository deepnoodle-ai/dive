module github.com/deepnoodle-ai/dive/experimental/a2alib

go 1.25.0

require (
	github.com/a2aproject/a2a-go/v2 v2.2.0
	github.com/deepnoodle-ai/dive v1.4.0
	github.com/deepnoodle-ai/wonton v0.0.29
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

// Local development: point at the parent module.
replace github.com/deepnoodle-ai/dive => ../..

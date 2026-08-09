module github.com/deepnoodle-ai/dive/providers/grok

go 1.25.0

toolchain go1.26.5

require (
	github.com/deepnoodle-ai/dive v1.18.0
	github.com/deepnoodle-ai/dive/providers/openai v1.18.0
	github.com/deepnoodle-ai/wonton v0.0.37
	github.com/openai/openai-go/v3 v3.50.0
	golang.org/x/image v0.44.0
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace (
	github.com/deepnoodle-ai/dive => ../..
	github.com/deepnoodle-ai/dive/providers/openai => ../openai
)

module github.com/deepnoodle-ai/dive/providers/openai

go 1.26.0

toolchain go1.26.8

require (
	github.com/deepnoodle-ai/dive v1.46.0
	github.com/deepnoodle-ai/wonton v0.14.2
	github.com/openai/openai-go/v3 v3.101.0
	golang.org/x/image v0.48.0
)

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/tidwall/gjson v1.20.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	golang.org/x/sys v0.50.0 // indirect
)

replace github.com/deepnoodle-ai/dive => ../..

module github.com/deepnoodle-ai/dive/experimental/cmd/dive

go 1.26.0

toolchain go1.26.8

require (
	github.com/deepnoodle-ai/dive v1.27.0
	github.com/deepnoodle-ai/dive/providers/google v1.27.0
	github.com/deepnoodle-ai/dive/providers/grok v1.27.0
	github.com/deepnoodle-ai/dive/providers/meta v1.27.0
	github.com/deepnoodle-ai/dive/providers/openai v1.27.0
	github.com/deepnoodle-ai/wonton v0.0.39
	github.com/mattn/go-runewidth v0.0.29
	google.golang.org/genai v1.71.0
)

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.23.2 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/alecthomas/chroma/v2 v2.27.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/dlclark/regexp2/v2 v2.7.1 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gobwas/glob v1.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.21 // indirect
	github.com/googleapis/gax-go/v2 v2.24.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/openai/openai-go/v3 v3.55.0 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/yuin/goldmark v1.8.5 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.71.0 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	golang.org/x/crypto v0.56.0 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/api v0.297.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260831171406-18b4a7587f8a // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/deepnoodle-ai/dive => ../../..
	github.com/deepnoodle-ai/dive/providers/google => ../../../providers/google
	github.com/deepnoodle-ai/dive/providers/grok => ../../../providers/grok
	github.com/deepnoodle-ai/dive/providers/meta => ../../../providers/meta
	github.com/deepnoodle-ai/dive/providers/openai => ../../../providers/openai
)

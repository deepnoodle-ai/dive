module github.com/deepnoodle-ai/dive/examples

go 1.25.5

toolchain go1.26.5

require (
	github.com/deepnoodle-ai/dive v1.20.0
	github.com/deepnoodle-ai/dive/a2a v1.20.0
	github.com/deepnoodle-ai/dive/experimental/mcp v1.20.0
	github.com/deepnoodle-ai/dive/otel v1.20.0
	github.com/deepnoodle-ai/dive/providers/google v1.20.0
	github.com/deepnoodle-ai/dive/providers/grok v1.20.0
	github.com/deepnoodle-ai/dive/providers/openai v1.20.0
	github.com/deepnoodle-ai/wonton v0.0.37
	github.com/fatih/color v1.19.0
	github.com/mark3labs/mcp-go v0.57.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.70.0
	go.opentelemetry.io/otel v1.45.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.45.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.45.0
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.45.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.45.0
	go.opentelemetry.io/otel/sdk v1.45.0
	go.opentelemetry.io/otel/sdk/metric v1.45.0
)

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.23.0 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/a2aproject/a2a-go/v2 v2.4.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.20 // indirect
	github.com/googleapis/gax-go/v2 v2.23.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/openai/openai-go/v3 v3.50.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.45.0 // indirect
	go.opentelemetry.io/otel/metric v1.45.0 // indirect
	go.opentelemetry.io/otel/trace v1.45.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/image v0.44.0 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/api v0.292.0 // indirect
	google.golang.org/genai v1.67.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260807164820-c8921c73eeea // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260807164820-c8921c73eeea // indirect
	google.golang.org/grpc v1.83.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/deepnoodle-ai/dive => ..
	github.com/deepnoodle-ai/dive/a2a => ../a2a
	github.com/deepnoodle-ai/dive/experimental/mcp => ../experimental/mcp
	github.com/deepnoodle-ai/dive/otel => ../otel
	github.com/deepnoodle-ai/dive/providers/google => ../providers/google
	github.com/deepnoodle-ai/dive/providers/grok => ../providers/grok
	github.com/deepnoodle-ai/dive/providers/openai => ../providers/openai
)

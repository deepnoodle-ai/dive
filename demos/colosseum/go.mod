module github.com/deepnoodle-ai/dive/demos/colosseum

go 1.25.0

require (
	github.com/deepnoodle-ai/dive v1.7.0
	github.com/deepnoodle-ai/dive/a2a v1.6.0
	github.com/deepnoodle-ai/dive/providers/google v1.6.0
	github.com/deepnoodle-ai/dive/providers/grok v1.6.0
	github.com/deepnoodle-ai/dive/providers/openai v1.7.0
	github.com/deepnoodle-ai/wonton v0.0.34
)

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.18.1 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/a2aproject/a2a-go/v2 v2.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.11 // indirect
	github.com/googleapis/gax-go/v2 v2.17.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/openai/openai-go/v3 v3.29.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.65.0 // indirect
	go.opentelemetry.io/otel v1.41.0 // indirect
	go.opentelemetry.io/otel/metric v1.41.0 // indirect
	go.opentelemetry.io/otel/trace v1.41.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/image v0.38.0 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genai v1.51.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209193700-7e5cd0f99864 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

// The Colosseum lives inside the Dive repo and builds against the working tree
// via local replace directives (matching the examples/ multi-module pattern, no
// go.work). The anthropic provider ships inside the core module, so Claude needs
// no extra replace; openai/google/grok are separate modules and each gets one.
// To graduate this demo into a standalone forkable repo, drop these replaces and
// pin a versioned `require` for the core + provider modules.
replace (
	github.com/deepnoodle-ai/dive => ../..
	github.com/deepnoodle-ai/dive/a2a => ../../a2a
	github.com/deepnoodle-ai/dive/providers/google => ../../providers/google
	github.com/deepnoodle-ai/dive/providers/grok => ../../providers/grok
	github.com/deepnoodle-ai/dive/providers/openai => ../../providers/openai
)

//go:build interop

// Package a2a interop tests cross-validate the wire format against the
// official a2a-python SDK. They are gated behind the `interop` build tag
// so default `go test ./...` runs do not require Python or `pip install
// a2a-sdk`.
//
// Run with:
//
//	go test -tags interop ./experimental/a2a/...
//
// Environment:
//
//	A2A_PYTHON — path to a Python 3 interpreter that can `import a2a`.
//	             Defaults to "python3".
//
// The test prints a clear skip reason if the interpreter or the SDK is
// missing, so it is safe to leave the tag on locally.
package a2a_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/experimental/a2a"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// pythonInteropDriver is a small Python script that exercises our server
// via the official a2a-python SDK. The Go test passes the server URL as
// argv[1] and reads JSON from the script's stdout: {"failures": int,
// "log": [str, ...]}. The script never raises — it captures errors and
// reports them as failures so the Go test gets a clean payload.
const pythonInteropDriver = `
import asyncio, json, sys, traceback
import httpx
from a2a.client.card_resolver import A2ACardResolver
from a2a.client.transports.jsonrpc import JsonRpcTransport
from a2a.types import (
    Message, MessageSendParams, Part, Role, TaskIdParams, TaskQueryParams,
    TaskStatusUpdateEvent, TextPart,
)

URL = sys.argv[1]

failures = 0
log = []

def ok(msg): log.append("PASS " + msg)
def bad(msg):
    global failures
    failures += 1
    log.append("FAIL " + msg)

async def run():
    global failures
    async with httpx.AsyncClient(timeout=10.0) as http:
        # 1. Card discovery via canonical path.
        try:
            resolver = A2ACardResolver(http, base_url=URL,
                agent_card_path="/.well-known/agent-card.json")
            card = await resolver.get_agent_card()
            ok("card decoded name=%r streaming=%s" % (card.name, card.capabilities.streaming))
        except Exception as e:
            bad("card decode/validation failed: %s" % e)
            return

        transport = JsonRpcTransport(http, agent_card=card, url=URL + "/")

        # 2. message/send returns a Task with an artifact.
        try:
            msg = Message(role=Role.user,
                parts=[Part(root=TextPart(text="What is the capital of France?"))],
                message_id="m-1")
            result = await transport.send_message(MessageSendParams(message=msg))
            if not hasattr(result, "status"):
                bad("message/send returned non-Task result: %r" % (result,))
                return
            ok("send task id=%s state=%s" % (result.id, result.status.state.value))
            if not result.artifacts:
                bad("send result has no artifacts")
            else:
                txt = "".join(p.root.text for p in result.artifacts[0].parts
                              if hasattr(p.root, "text"))
                ok("artifact text=%r" % txt)
            task_id = result.id
            ctx_id = result.context_id
        except Exception as e:
            bad("message/send raised: %s" % e)
            traceback.print_exc()
            return

        # 3. tasks/get round-trips.
        try:
            fetched = await transport.get_task(TaskQueryParams(id=task_id))
            if fetched.id == task_id:
                ok("tasks/get id=%s state=%s" % (fetched.id, fetched.status.state.value))
            else:
                bad("tasks/get id mismatch: %s != %s" % (fetched.id, task_id))
        except Exception as e:
            bad("tasks/get raised: %s" % e)

        # 4. tasks/cancel on a terminal task is rejected (TaskNotCancelable).
        try:
            await transport.cancel_task(TaskIdParams(id=task_id))
            bad("cancel of terminal task unexpectedly succeeded")
        except Exception as e:
            ok("cancel of terminal task rejected: %s" % e)

        # 5. message/stream emits at least one final status update.
        try:
            msg = Message(role=Role.user,
                parts=[Part(root=TextPart(text="Stream please"))],
                message_id="m-stream", context_id=ctx_id)
            events = []
            async for ev in transport.send_message_streaming(MessageSendParams(message=msg)):
                events.append(ev)
            ok("stream events count=%d" % len(events))
            final = next((e for e in reversed(events)
                          if isinstance(e, TaskStatusUpdateEvent)), None)
            if final is None:
                bad("no TaskStatusUpdateEvent in stream")
            elif not final.final:
                bad("last status update has final=False")
            else:
                ok("final status state=%s final=True" % final.status.state.value)
        except Exception as e:
            bad("message/stream raised: %s" % e)
            traceback.print_exc()

        # 6. tasks/get for unknown id returns an RPC error.
        try:
            await transport.get_task(TaskQueryParams(id="does-not-exist"))
            bad("tasks/get unknown id unexpectedly succeeded")
        except Exception as e:
            ok("tasks/get unknown id rejected: %s" % e)

asyncio.run(run())
print(json.dumps({"failures": failures, "log": log}))
`

// TestPythonInteropAgainstServer boots our server via httptest and drives
// it from the official a2a-python SDK. The test is skipped (not failed)
// if the local Python environment cannot import a2a — that keeps the
// integration friction-free for contributors who do not have the SDK
// installed.
func TestPythonInteropAgainstServer(t *testing.T) {
	pyBin := os.Getenv("A2A_PYTHON")
	if pyBin == "" {
		pyBin = "python3"
	}
	if _, err := exec.LookPath(pyBin); err != nil {
		t.Skipf("python interpreter %q not found in PATH", pyBin)
	}
	if err := exec.Command(pyBin, "-c", "import a2a, httpx").Run(); err != nil {
		t.Skipf("python a2a-sdk / httpx not importable via %s: %v "+
			"(install with `pip install a2a-sdk httpx`)", pyBin, err)
	}

	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("Paris is the capital of France."), nil
	}}
	agent := buildAgent(t, model)

	server, err := a2a.NewServer(a2a.ServerOptions{
		Agent: agent,
		Card: a2a.AgentCard{
			Description:        "Interop test agent.",
			DefaultInputModes:  []string{"text/plain"},
			DefaultOutputModes: []string{"text/plain"},
		},
	})
	assert.NoError(t, err)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	cmd := exec.Command(pyBin, "-c", pythonInteropDriver, ts.URL)
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("python interop driver failed: %v\nstderr:\n%s\nstdout:\n%s",
			err, stderr, string(out))
	}

	// The driver may print non-JSON tracebacks before the final JSON line
	// when something blows up. Take the last line for the report.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	last := lines[len(lines)-1]
	var report struct {
		Failures int      `json:"failures"`
		Log      []string `json:"log"`
	}
	if err := json.Unmarshal([]byte(last), &report); err != nil {
		t.Fatalf("driver did not return JSON: %v\nfull stdout:\n%s",
			err, string(out))
	}
	for _, line := range report.Log {
		t.Log(line)
	}
	if report.Failures > 0 {
		t.Fatalf("python interop driver reported %d wire-format failures", report.Failures)
	}
}


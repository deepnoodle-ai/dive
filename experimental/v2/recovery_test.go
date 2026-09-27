package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestToolExecutionCertainty(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result ToolResult
		err    error
		reason string
	}{
		{"success", ToolResult{State: ToolSucceeded, Text: "receipt"}, nil, "completed"},
		{"definite_failure", ToolResult{State: ToolFailed, Error: "declined"}, nil, "failed"},
		{"not_executed", ToolResult{State: ToolNotExecuted, Error: "validation failed before dispatch"}, nil, "not_executed"},
		{"unknown", ToolResult{State: ToolUnknown, Error: "status unavailable"}, nil, "reconcile"},
		{"transport_error", ToolResult{}, errors.New("response lost after effect"), "reconcile"},
		{"cancellation", ToolResult{}, context.Canceled, "reconcile"},
		{"conflicting_error", ToolResult{State: ToolSucceeded, Text: "unverified receipt"}, context.DeadlineExceeded, "reconcile"},
		{"invalid_result", ToolResult{}, nil, "reconcile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := hostWithInput()
			models, tools := 0, 0
			e := scripted(false, &models, &tools)
			e.Tools["echo"] = func(_ context.Context, invocation ToolInvocation) (ToolResult, error) {
				tools++
				assert.Equal(t, invocation.EffectID, "turn/model/1/tool")
				return tc.result, tc.err
			}
			out, err := e.Execute(ctx, nil, startCommand(2), h)
			assert.NoError(t, err)
			assert.Equal(t, out.Reason, tc.reason)
			assert.Equal(t, tools, 1)
			if tc.reason != "reconcile" {
				return
			}
			assert.Equal(t, out.Records[4].Kind, ToolUncertain)
			assert.Equal(t, out.Records[4].Tool.State, ToolUnknown)
			loaded, err := h.Load(ctx, "turn")
			assert.NoError(t, err)
			out, err = e.Execute(ctx, loaded, Command{}, h)
			assert.NoError(t, err)
			assert.Equal(t, out.Reason, "reconcile")
			assert.Equal(t, models, 1)
			assert.Equal(t, tools, 1)
			command := Command{Resolution: &Resolution{CommandID: "verified-receipt", EffectID: "turn/model/1/tool", Tool: &ToolResult{State: ToolSucceeded, Text: "verified"}}}
			out, err = e.Execute(ctx, out.Records, command, h)
			assert.NoError(t, err)
			assert.Equal(t, out.Reason, "completed")
			out, err = e.Execute(ctx, out.Records, command, h)
			assert.NoError(t, err)
			assert.True(t, out.Duplicate)
			assert.Equal(t, models, 2)
			assert.Equal(t, tools, 1)
			command.Resolution.Tool.Text = "different receipt"
			_, err = e.Execute(ctx, out.Records, command, h)
			assert.ErrorContains(t, err, "command identity reused")
		})
	}
}

func TestTakeoverWhileOldToolIsRunning(t *testing.T) {
	host := hostWithInput()
	old := host.Takeover("turn")
	entered, release := make(chan struct{}), make(chan struct{})
	oldModels, oldTools := 0, 0
	engine := scripted(false, &oldModels, &oldTools)
	engine.Tools["echo"] = func(_ context.Context, invocation ToolInvocation) (ToolResult, error) {
		close(entered)
		<-release
		return ToolResult{State: ToolSucceeded, Text: "late verified receipt"}, nil
	}
	type result struct {
		out Outcome
		err error
	}
	finished := make(chan result, 1)
	go func() {
		out, err := engine.Execute(ctx, nil, startCommand(2), old)
		finished <- result{out, err}
	}()
	<-entered
	current := host.Takeover("turn")
	records, err := current.Load(ctx, "turn")
	assert.NoError(t, err)
	assert.Equal(t, records[len(records)-1].Kind, ToolStarted)
	newModels, newTools := 0, 0
	engine = scripted(false, &newModels, &newTools)
	out, err := engine.Execute(ctx, records, Command{}, current)
	assert.NoError(t, err)
	assert.Equal(t, out.Reason, "reconcile")
	close(release)
	late := <-finished
	assert.NoError(t, late.err)
	assert.Equal(t, late.out.Reason, "recording_failed")
	assert.Equal(t, late.out.WriteError.Disposition, Rejected)
	assert.Equal(t, late.out.Unacknowledged.Tool.Text, "late verified receipt")
	assert.Error(t, old.Commit(ctx, *late.out.Unacknowledged))
	// The new owner explicitly verifies/accepts the old worker's observation;
	// it does not copy the stale worker's write authority.
	command := Command{Resolution: &Resolution{CommandID: "late-receipt", EffectID: late.out.Unacknowledged.EffectID, Tool: late.out.Unacknowledged.Tool}}
	out, err = engine.Execute(ctx, out.Records, command, current)
	assert.NoError(t, err)
	assert.Equal(t, out.Reason, "completed")
	assert.Equal(t, oldModels, 1)
	assert.Equal(t, newModels, 1)
	assert.Equal(t, newTools, 0)
}

// The parent process represents a surviving application database and remote
// effect service. Separate child processes run the disposable workers. HTTP
// connections really close after commits; no actual payment/provider is used.
type recoveryRPC struct {
	Epoch      uint64
	Step       *Step
	Invocation *ToolInvocation
}

type recoveryReply struct {
	Records     []Step
	Result      *ToolResult
	Error       string
	Disposition Disposition
}

type recoveryService struct {
	mu             sync.Mutex
	host           *TransactionalHost
	owners         map[uint64]*OwnerStore
	epoch          uint64
	charges        map[string]ToolInvocation
	resultAcksLost int
	balance        int
}

func (s *recoveryService) takeover() *OwnerStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.host.Takeover("turn")
	s.epoch = o.Epoch()
	s.owners[s.epoch] = o
	return o
}

func (s *recoveryService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req recoveryRPC
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reply := recoveryReply{}
	var err error
	dropReply := false
	owner := s.owners[req.Epoch]
	if owner == nil || req.Epoch != s.epoch {
		err = reject(errors.New("stale downstream owner"))
	} else {
		switch r.URL.Path {
		case "/load":
			reply.Records, err = owner.Load(r.Context(), "turn")
		case "/commit":
			if req.Step == nil {
				err = reject(errors.New("missing step"))
				break
			}
			err = owner.Commit(r.Context(), *req.Step)
			if err == nil && req.Step.CommandID == "verified-receipt" && s.resultAcksLost == 0 {
				s.resultAcksLost++
				dropReply = true
			}
		case "/charge":
			if req.Invocation == nil || req.Invocation.EffectID == "" {
				err = reject(errors.New("missing invocation identity"))
				break
			}
			invocation := *req.Invocation
			prior, exists := s.charges[invocation.EffectID]
			if exists && prior != invocation {
				err = reject(errors.New("effect identity reused with different input"))
				break
			}
			if !exists {
				s.charges[invocation.EffectID] = invocation
				s.balance += 7
				dropReply = true // Commit effect, then lose its transport response.
			}
			reply.Result = &ToolResult{State: ToolSucceeded, Text: "receipt-7"}
		case "/lookup":
			if req.Invocation == nil {
				err = errors.New("missing lookup identity")
				break
			}
			if _, exists := s.charges[req.Invocation.EffectID]; !exists {
				err = errors.New("no receipt") // Absence is not proof of nonexecution.
				break
			}
			reply.Result = &ToolResult{State: ToolSucceeded, Text: "receipt-7"}
		default:
			err = errors.New("unknown endpoint")
		}
	}
	if err != nil {
		reply.Error = err.Error()
		var commitErr *CommitError
		if errors.As(err, &commitErr) {
			reply.Disposition = commitErr.Disposition
		}
	}
	if dropReply {
		conn, _, hijackErr := w.(http.Hijacker).Hijack()
		if hijackErr == nil {
			_ = conn.Close()
		}
		return
	}
	_ = json.NewEncoder(w).Encode(reply)
}

type recoveryClient struct {
	url   string
	epoch uint64
}

func (c recoveryClient) request(ctx context.Context, endpoint string, req recoveryRPC) (recoveryReply, error) {
	req.Epoch = c.epoch
	data, err := json.Marshal(req)
	if err != nil {
		return recoveryReply{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+endpoint, bytes.NewReader(data))
	if err != nil {
		return recoveryReply{}, err
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(httpReq)
	if err != nil {
		return recoveryReply{}, err
	}
	defer response.Body.Close()
	var reply recoveryReply
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		return reply, err
	}
	if reply.Error != "" {
		if reply.Disposition != "" {
			return reply, &CommitError{Disposition: reply.Disposition, Cause: errors.New(reply.Error)}
		}
		return reply, errors.New(reply.Error)
	}
	return reply, nil
}

func (c recoveryClient) Commit(ctx context.Context, step Step) error {
	_, err := c.request(ctx, "/commit", recoveryRPC{Step: &step})
	return err
}

func (c recoveryClient) Load(ctx context.Context, turnID string) ([]Step, error) {
	if turnID != "turn" {
		return nil, errors.New("unexpected turn")
	}
	reply, err := c.request(ctx, "/load", recoveryRPC{})
	return reply.Records, err
}

func TestRecoveryAcrossWorkerProcesses(t *testing.T) {
	svc := &recoveryService{host: hostWithInput(), owners: make(map[uint64]*OwnerStore), charges: make(map[string]ToolInvocation)}
	server := httptest.NewServer(svc)
	defer server.Close()
	run := func(mode string, epoch uint64) {
		t.Helper()
		executable, err := os.Executable()
		assert.NoError(t, err)
		workerCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		command := exec.CommandContext(workerCtx, executable, "-test.run=^TestRecoveryWorkerProcess$", "-test.v")
		command.Env = append(os.Environ(), "DIVE_V2_RECOVERY_URL="+server.URL, "DIVE_V2_RECOVERY_EPOCH="+strconv.FormatUint(epoch, 10), "DIVE_V2_RECOVERY_MODE="+mode)
		output, err := command.CombinedOutput()
		assert.NoError(t, err, string(output))
		t.Log(string(output))
	}
	old := svc.takeover()
	run("start", old.Epoch())
	records, err := old.Load(ctx, "turn")
	assert.NoError(t, err)
	assert.Equal(t, records[len(records)-1].Kind, ToolUncertain)
	current := svc.takeover()
	assert.Error(t, old.Commit(ctx, records[len(records)-1]), "even duplicate writes from an old owner must be fenced")
	assert.Error(t, svc.host.Commit(ctx, records[len(records)-1]), "unfenced writes must not bypass claimed ownership")
	stale := recoveryClient{url: server.URL, epoch: old.Epoch()}
	_, err = stale.request(ctx, "/charge", recoveryRPC{Invocation: &ToolInvocation{EffectID: "turn/model/1/tool", Call: Call{Name: "echo", Arguments: `{"value":"original"}`}}})
	assert.ErrorContains(t, err, "stale downstream owner")
	run("recover", current.Epoch())
	// The downstream service also deduplicates an explicit same-key retry.
	client := recoveryClient{url: server.URL, epoch: current.Epoch()}
	invocation := ToolInvocation{EffectID: "turn/model/1/tool", Call: Call{Name: "echo", Arguments: `{"value":"original"}`}}
	reply, err := client.request(ctx, "/charge", recoveryRPC{Invocation: &invocation})
	assert.NoError(t, err)
	assert.Equal(t, reply.Result.Text, "receipt-7")
	invocation.Arguments = `{"value":"different"}`
	_, err = client.request(ctx, "/charge", recoveryRPC{Invocation: &invocation})
	assert.ErrorContains(t, err, "effect identity reused")
	records, err = current.Load(ctx, "turn")
	assert.NoError(t, err)
	assert.Equal(t, records[len(records)-1].Status, "completed")
	usage, unknown, inbox := svc.host.Snapshot()
	assert.Equal(t, usage, Usage{Input: 30, Output: 5})
	assert.Equal(t, unknown, 0)
	assert.Equal(t, len(inbox), 0)
	svc.mu.Lock()
	assert.Equal(t, svc.balance, 7)
	assert.Equal(t, len(svc.charges), 1)
	assert.Equal(t, svc.resultAcksLost, 1)
	svc.mu.Unlock()
}

// Invoked only in subprocesses by TestRecoveryAcrossWorkerProcesses.
func TestRecoveryWorkerProcess(t *testing.T) {
	url := os.Getenv("DIVE_V2_RECOVERY_URL")
	if url == "" {
		t.Skip("subprocess helper")
	}
	epoch, err := strconv.ParseUint(os.Getenv("DIVE_V2_RECOVERY_EPOCH"), 10, 64)
	assert.NoError(t, err)
	client := recoveryClient{url: url, epoch: epoch}
	models, tools := 0, 0
	e := scripted(false, &models, &tools)
	e.Tools["echo"] = func(ctx context.Context, invocation ToolInvocation) (ToolResult, error) {
		tools++
		reply, err := client.request(ctx, "/charge", recoveryRPC{Invocation: &invocation})
		if err != nil {
			return ToolResult{}, err
		}
		return *reply.Result, nil
	}
	runner := &SessionRunner{Store: client, Engine: e}
	if os.Getenv("DIVE_V2_RECOVERY_MODE") == "start" {
		out, err := runner.Run(ctx, "turn", startCommand(2))
		assert.NoError(t, err)
		assert.Equal(t, out.Reason, "reconcile")
		assert.Equal(t, models, 1)
		assert.Equal(t, tools, 1)
		t.Log("effect committed; HTTP response lost; uncertainty recorded; worker exits")
		return
	}
	out, err := runner.Run(ctx, "turn", Command{})
	assert.NoError(t, err)
	assert.Equal(t, out.Reason, "reconcile")
	assert.Equal(t, models, 0)
	assert.Equal(t, tools, 0)
	identity := ToolInvocation{EffectID: "turn/model/1/tool"}
	reply, err := client.request(ctx, "/lookup", recoveryRPC{Invocation: &identity})
	assert.NoError(t, err)
	command := Command{Resolution: &Resolution{CommandID: "verified-receipt", EffectID: identity.EffectID, Tool: reply.Result}}
	out, err = runner.Run(ctx, "turn", command)
	assert.NoError(t, err)
	assert.Equal(t, out.Reason, "recording_failed")
	assert.Equal(t, out.WriteError.Disposition, Unknown)
	out, err = runner.Run(ctx, "turn", command)
	assert.NoError(t, err)
	assert.True(t, out.Duplicate)
	assert.Equal(t, models, 0, "redelivery accepts the command without starting effects")
	out, err = runner.Run(ctx, "turn", Command{})
	assert.NoError(t, err)
	assert.Equal(t, out.Reason, "completed")
	out, err = runner.Run(ctx, "turn", command)
	assert.NoError(t, err)
	assert.True(t, out.Duplicate)
	assert.Equal(t, models, 1)
	assert.Equal(t, tools, 0)
	command.Resolution.Tool.Text = "conflicting receipt"
	_, err = runner.Run(ctx, "turn", command)
	assert.ErrorContains(t, err, "command identity reused")
	t.Log(fmt.Sprintf("owner %d reconciled; repeated result accepted; conflicting result rejected; no tool reexecution", epoch))
}

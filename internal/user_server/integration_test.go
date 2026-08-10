package userserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cpmores/lucinda/internal/task_management_layer/postman"
	"github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_board"
	"github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	"github.com/cpmores/lucinda/internal/testutil"
	"github.com/cpmores/lucinda/internal/task_wrapper"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_commander"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_executor"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_monitor"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_planner"
	"github.com/cpmores/lucinda/internal/user_server"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// setupStack wires the full workflow (eventbus + mock transport + mock
// provider + planner/commander/executor/monitor) and returns an httptest
// server fronting the HTTP handlers.
func setupStack(t *testing.T) (*testutil.MockProvider, *httptest.Server, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	log := logger.Discard()

	mm := modulemanager.NewModuleManager()
	eb := eventbus.NewInMemoryEventBus(log.Child("eb"))
	mockTp := testutil.NewMockTransport("node-A")
	prov := testutil.NewMockProvider("mock", nil)
	pc := testutil.NewMockProviderController(prov)

	eb.RegisterWithManager(mm)
	mockTp.RegisterWithManager(mm)
	pc.RegisterWithManager(mm)

	planner := taskplanner.NewTaskPlanner(log.Child("planner"))
	commander := taskcommander.NewTaskCommander(log.Child("commander"))
	executor := taskexecutor.NewTaskExecutor(log.Child("executor"))
	streamR := streamrouter.NewStreamRouter(log.Child("stream"))
	postman := taskpostman.NewTaskPostman(log.Child("postman"))
	board := taskboard.NewTaskBoard(log.Child("board"))
	tracer := tasktracer.NewTaskTracer(log.Child("tracer"))
	monitor := taskmonitor.NewTaskMonitor(log.Child("monitor"))

	planner.RegisterWithManager(mm)
	commander.RegisterWithManager(mm)
	executor.RegisterWithManager(mm)
	streamR.RegisterWithManager(mm)
	postman.RegisterWithManager(mm)
	board.RegisterWithManager(mm)
	tracer.RegisterWithManager(mm)
	monitor.RegisterWithManager(mm)

	if err := mm.VerifyInit(); err != nil {
		t.Fatalf("VerifyInit: %v", err)
	}
	if err := mm.EnableDeps(); err != nil {
		t.Fatalf("EnableDeps: %v", err)
	}

	startErr := func(name string, fn func(context.Context) error) {
		if err := fn(ctx); err != nil {
			t.Fatalf("%s start: %v", name, err)
		}
	}
	startErr("stream", streamR.Start)
	startErr("postman", postman.Start)
	startErr("board", board.Start)
	startErr("monitor", monitor.Start)
	startErr("planner", planner.Start)
	startErr("commander", commander.Start)
	startErr("executor", executor.Start)

	wrapper := taskwrapper.New(eb, monitor, string(mockTp.ID()))
	srv := userserver.NewHTTPServer(wrapper, monitor, log.Child("http"))
	ts := httptest.NewServer(srv.Routes())

	cleanup := func() { ts.Close(); cancel() }
	return prov, ts, cleanup
}

func TestChatToStreamSequence(t *testing.T) {
	prov, ts, cleanup := setupStack(t)
	defer cleanup()

	// Gate planning so the SSE stream is open before the plan executes.
	prov.Hold = make(chan struct{})

	planID := postChat(t, ts, "write a story about a dragon", "")

	// Open the stream; the workflow is parked inside planning Generate.
	resp, err := http.Get(ts.URL + "/stream?plan=" + planID)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	bodyCh := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodyCh <- string(b)
	}()

	close(prov.Hold) // release planning → workflow runs → frames then done

	body := <-bodyCh
	frames := parseSSE(t, body)

	if len(frames) == 0 {
		t.Fatal("no SSE frames received")
	}
	assertExactlyOneDone(t, frames, planID)
	text := assertDoneOK(t, frames)
	if !strings.Contains(text, "RESULT:step one") || !strings.Contains(text, "RESULT:step two") {
		t.Fatalf("synthesized text missing step outputs: %q", text)
	}
	assertEvent(t, frames, "status", "expected at least one status frame before done")
	assertEvent(t, frames, "step_result", "expected at least one step_result frame before done")
	assertOrdered(t, frames)
}

// TestChatToStreamReAct drives a ReAct plan end-to-end: the reasoning loop
// issues a subtask (step_result), then the final answer streams (stream) and
// closes with exactly one done frame.
func TestChatToStreamReAct(t *testing.T) {
	prov, ts, cleanup := setupStack(t)
	defer cleanup()

	prov.ReActOut = []string{
		`{"action":"continue","task":{"prompt":"subtask","model":"mock-model"}}`,
		`{"action":"done","answer":"one-shot fallback"}`,
	}
	prov.StreamOut = []string{"streamed ", "answer 42"}
	// A single transaction keeps the final answer exactly the streamed one.
	prov.PlanOut = `{"transactions":[{"id":"t1","goal":"answer a riddle","labels":["cpu"],"deps":[]}]}`

	planID := postChat(t, ts, "answer a riddle", "react")

	resp, err := http.Get(ts.URL + "/stream?plan=" + planID)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	frames := parseSSE(t, string(body))
	if len(frames) == 0 {
		t.Fatal("no SSE frames received")
	}
	assertEvent(t, frames, "status", "expected status frames")
	assertEvent(t, frames, "step_result", "expected a step_result frame (ReAct action)")
	assertEvent(t, frames, "stream", "expected stream frames (streamed final answer)")
	text := assertDoneOK(t, frames)
	if text != "streamed answer 42" {
		t.Fatalf("done text = %q, want the streamed answer", text)
	}
	assertOrdered(t, frames)
}

func TestStreamUnknownPlan(t *testing.T) {
	_, ts, cleanup := setupStack(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/stream?plan=plan-does-not-exist")
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (no frames for unknown plan)", resp.StatusCode)
	}
}

func postChat(t *testing.T, ts *httptest.Server, prompt, agent string) string {
	t.Helper()
	var body string
	if agent != "" {
		body = fmt.Sprintf(`{"agent":%q,"messages":[{"role":"user","content":[{"type":"text","text":%q}]}]}`, agent, prompt)
	} else {
		body = fmt.Sprintf(`{"messages":[{"role":"user","content":[{"type":"text","text":%q}]}]}`, prompt)
	}
	resp, err := http.Post(ts.URL+"/chat", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post /chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("post /chat status %d: %s", resp.StatusCode, b)
	}
	var out struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode /chat response: %v", err)
	}
	if out.PlanID == "" {
		t.Fatal("empty plan_id")
	}
	return out.PlanID
}

func parseSSE(t *testing.T, body string) []map[string]any {
	t.Helper()
	var frames []map[string]any
	for _, block := range strings.Split(body, "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		data := ""
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			}
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			t.Fatalf("bad frame %q: %v", block, err)
		}
		frames = append(frames, frame)
	}
	return frames
}

func assertExactlyOneDone(t *testing.T, frames []map[string]any, planID string) {
	t.Helper()
	count := 0
	for _, f := range frames {
		if f["event"] == "done" {
			count++
			if f["plan_id"] != planID {
				t.Fatalf("done frame for wrong plan: %v", f["plan_id"])
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one done frame, got %d", count)
	}
}

func assertDoneOK(t *testing.T, frames []map[string]any) string {
	t.Helper()
	for _, f := range frames {
		if f["event"] != "done" {
			continue
		}
		d, ok := f["data"].(map[string]any)
		if !ok {
			t.Fatal("done data not an object")
		}
		if d["status"] != "ok" {
			t.Fatalf("done status = %v, want ok", d["status"])
		}
		text, _ := d["text"].(string)
		if text == "" {
			t.Fatal("done text empty")
		}
		return text
	}
	t.Fatal("no done frame")
	return ""
}

func assertEvent(t *testing.T, frames []map[string]any, event, msg string) {
	t.Helper()
	for _, f := range frames {
		if f["event"] == event {
			return
		}
	}
	t.Fatal(msg)
}

// assertOrdered ensures status/step_result frames precede the done frame.
func assertOrdered(t *testing.T, frames []map[string]any) {
	t.Helper()
	seenDone := false
	for _, f := range frames {
		if seenDone && f["event"] != "done" {
			t.Fatalf("frame %v appeared after done", f["event"])
		}
		if f["event"] == "done" {
			seenDone = true
		}
	}
}

// Package userserver provides the HTTP ingress/egress for Lucinda: POST /chat
// turns a ChatRequest into a plan, and GET /stream emits the ordered SSE
// frames (status, step_result, stream, done) back to the user.
package userserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	APIChat "github.com/cpmores/lucinda/api/v1/domain/chat"
	APISteam "github.com/cpmores/lucinda/api/v1/domain/stream"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	taskwrapper "github.com/cpmores/lucinda/internal/task_wrapper"
	"github.com/cpmores/lucinda/internal/task_workflow_layer/task_monitor"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
)

// Server is the HTTP ingress.
type Server interface {
	Routes() http.Handler
	Start(addr string) error
	Shutdown(ctx context.Context) error
}

type HTTPServer struct {
	wrapper *taskwrapper.TaskWrapper
	monitor taskmonitor.TaskMonitor
	log     *logger.Logger
	httpSrv *http.Server
}

// NewHTTPServer creates the HTTP server.
func NewHTTPServer(wrapper *taskwrapper.TaskWrapper, monitor taskmonitor.TaskMonitor, log *logger.Logger) Server {
	if log == nil {
		log = logger.Discard()
	}
	return &HTTPServer{wrapper: wrapper, monitor: monitor, log: log}
}

func (s *HTTPServer) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat", s.handleChat)
	mux.HandleFunc("/stream", s.handleStream)
	mux.HandleFunc("/healthz", s.handleHealth)
	return mux
}

func (s *HTTPServer) Start(addr string) error {
	s.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.log.Info("http server listening", "addr", addr)
	return s.httpSrv.ListenAndServe()
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

// ── Handlers ──────────────────────────────────────────────────────────

func (s *HTTPServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	var req APIChat.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("invalid body: %v", err), http.StatusBadRequest)
		return
	}
	prompt, ok := lastUserText(req.Messages)
	if !ok {
		http.Error(w, "messages must contain at least one user text message", http.StatusBadRequest)
		return
	}
	arch := taskwrapper.NormalizeAgent(APITask.AgentArch(req.Agent))

	planID, _, err := s.wrapper.Wrap(prompt, arch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"plan_id": string(planID)})
}

// handleStream opens the plan's SSE stream: monitor frames flow until the
// wrapper's Notify delivers the terminal PlanResult, at which point a single
// done frame is written and the connection closes.
func (s *HTTPServer) handleStream(w http.ResponseWriter, r *http.Request) {
	planID := r.URL.Query().Get("plan")
	if planID == "" {
		http.Error(w, "plan parameter required", http.StatusBadRequest)
		return
	}

	// Unknown plan → error, no frames.
	frames, err := s.monitor.Open(planID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	notify, ok := s.wrapper.Notify(APITask.TaskID(planID))
	if !ok {
		http.Error(w, "no session for plan", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	// Flush headers immediately so the client sees the stream is open even
	// before the first frame arrives.
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return // client disconnected
		case frame, ok := <-frames:
			if !ok {
				return
			}
			if err := writeFrame(w, flusher, frame); err != nil {
				return
			}
		case result, ok := <-notify:
			if !ok {
				return
			}
			// Give in-flight progress frames a bounded moment to land before
			// the terminal done frame. With a real LLM the gap between the
			// last step_result and completion is large; a fast/empty plan can
			// otherwise race the last frames out.
			deadline := time.After(100 * time.Millisecond)
			for {
				select {
				case frame, ok := <-frames:
					if !ok {
						goto done
					}
					if err := writeFrame(w, flusher, frame); err != nil {
						return
					}
				case <-deadline:
					goto done
				}
			}
		done:
			_ = writeFrame(w, flusher, APISteam.SSEFrame{
				Event:  APISteam.SSETypeDone,
				PlanID: planID,
				Data:   APISteam.DoneData{Status: string(result.Status), Text: result.Text},
			})
			return
		}
	}
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func writeFrame(w http.ResponseWriter, flusher http.Flusher, frame APISteam.SSEFrame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", frame.Event, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// lastUserText extracts the text of the last user message in the chat.
func lastUserText(messages []APIChat.ChatMessage) (string, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		for _, part := range messages[i].Content {
			if part.Type == APIChat.ContentText && part.Text != "" {
				return part.Text, true
			}
		}
	}
	return "", false
}

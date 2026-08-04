// Package userserver provides the HTTP ingress for Lucinda.
package userserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	taskwrapper "github.com/cpmores/lucinda/internal/task_wrapper"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
)

type Server interface {
	Routes() http.Handler
	Start(addr string) error
	Shutdown(ctx context.Context) error
}

type HTTPServer struct {
	wrapper *taskwrapper.TaskWrapper
	streams map[APITask.TaskID]chan APITask.PlanResult
	mu      sync.Mutex
	httpSrv *http.Server
}

func NewHTTPServer(eb eventbus.EventBus) Server {
	return &HTTPServer{
		wrapper: taskwrapper.New(eb),
		streams: make(map[APITask.TaskID]chan APITask.PlanResult),
	}
}

func (s *HTTPServer) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat", s.handleChat)
	mux.HandleFunc("/stream", s.handleStream)
	mux.HandleFunc("/healthz", s.handleHealth)
	return mux
}

func (s *HTTPServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	var req struct {
		Prompt string `json:"prompt"`
		Owner  string `json:"owner,omitempty"`
	}
	json.Unmarshal(body, &req)
	if req.Prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}
	if req.Owner == "" {
		req.Owner = "anonymous"
	}

	ch := make(chan APITask.PlanResult, 1)
	id, err := s.wrapper.Wrap(req.Prompt, req.Owner, ch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.streams[id] = ch
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"tracking_id": string(id)})
}

func (s *HTTPServer) handleStream(w http.ResponseWriter, r *http.Request) {
	planID := APITask.TaskID(r.URL.Query().Get("plan"))
	if planID == "" {
		http.Error(w, "plan parameter required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	ch, ok := s.streams[planID]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "plan not found", http.StatusNotFound)
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

	// Wait for the plan to terminate, or the client to disconnect.
	var result APITask.PlanResult
	select {
	case result = <-ch:
	case <-r.Context().Done():
		// Client disconnected — clean up and return.
		s.mu.Lock()
		delete(s.streams, planID)
		s.mu.Unlock()
		return
	}

	switch result.Status {
	case APITask.PlanOK:
		fmt.Fprintf(w, "data: {\"type\":\"result\",\"text\":%s}\n\n", jsonEscape(result.Text))
	case APITask.PlanError, APITask.PlanTimeout, APITask.PlanCancelled:
		fmt.Fprintf(w, "data: {\"type\":\"error\",\"status\":\"%s\",\"text\":%s}\n\n",
			result.Status, jsonEscape(result.Text))
	}
	flusher.Flush()
	fmt.Fprintf(w, "data: {\"type\":\"done\"}\n\n")
	flusher.Flush()

	s.mu.Lock()
	delete(s.streams, planID)
	s.mu.Unlock()
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (s *HTTPServer) Start(addr string) error {
	s.mu.Lock()
	s.httpSrv = &http.Server{Addr: addr, Handler: s.Routes()}
	srv := s.httpSrv
	s.mu.Unlock()

	log.Printf("server: listening on %s", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the HTTP server without interrupting active connections.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpSrv
	s.mu.Unlock()

	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

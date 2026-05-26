// internel/server/httpServer.go
package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cpmores/lucinda/api/v1"
	eventbus "github.com/cpmores/lucinda/internel/eventBus"
	"github.com/cpmores/lucinda/internel/task"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
)

type HttpServer struct {
	config   *viper.Viper
	server   *http.Server
	eventBus eventbus.EventBus
	router   *mux.Router
}

// ========================== Only for TEST =========================
func newHttpServer(config *viper.Viper, eventBus eventbus.EventBus) (*HttpServer, error) {
	httpServer := &HttpServer{
		config:   config,
		eventBus: eventBus,
		router:   mux.NewRouter(),
	}

	httpServer.setupRouters()

	port := config.GetInt("http.port")
	if port == 0 {
		port = 8080
	}

	httpServer.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: httpServer.router,
	}

	return httpServer, nil
}

func (hs *HttpServer) setupRouters() {
	// hs.router.HandleFunc("/ollama", ollamaChatHandler)
	hs.router.HandleFunc("/healthz", healthzHandler)
	hs.router.HandleFunc("/chat", hs.chatHandlerSynchronous)
}

func (hs *HttpServer) chatHandlerSynchronous(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// 1. 解析并提交
	reqContent, _ := io.ReadAll(r.Body)
	defer r.Body.Close()
	chatReq := api.ChatRequest{
		Model:    "gemma3",
		Messages: []api.Message{{Role: "user", Content: string(reqContent)}},
	}

	// upload submit
	taskID, err := hs.Submit(chatReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. 向 EventBus 订阅该任务的最终结果主题
	// TODO: CREATE UNIQUE TASK TOPIC
	resultTopic := api.EventTopic(fmt.Sprintf("%s.%s", api.TASK_RESULT, taskID))
	resultChan, err := hs.eventBus.Subscribe(resultTopic)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer hs.eventBus.Unsubscribe(resultTopic, resultChan)

	// 3. 同步阻塞等待对应的单条结果
	select {
	case <-ctx.Done():
		http.Error(w, "Task Timeout or Client Cancelled", http.StatusRequestTimeout)
	case event := <-resultChan:
		if chatResp, ok := event.Data.(api.ChatResponse); ok {
			w.Write([]byte(chatResp.Message.Content))
		} else if errPayload, ok := event.Data.(error); ok {
			http.Error(w, errPayload.Error(), http.StatusInternalServerError)
		}
	}
}

// func ollamaChatHandler(w http.ResponseWriter, r *http.Request) {
// 	ctx := context.Background()
// 	ollamaProvider, err := provider.ProviderController.GetProvider("ollama")
// 	if err != nil {
// 		http.Error(w,
// 			err.Error(),
// 			http.StatusInternalServerError)

// 	}
// 	reqContent, err := io.ReadAll(r.Body)
// 	defer r.Body.Close()
// 	if err != nil {
// 		http.Error(w,
// 			err.Error(),
// 			http.StatusInternalServerError)
// 		return
// 	}
// 	reqMessages := []api.Message{
// 		{
// 			Role:    "user",
// 			Content: string(reqContent),
// 		},
// 	}

// 	chatReq := api.ChatRequest{
// 		Model:    "gemma3",
// 		Messages: reqMessages,
// 	}
// 	chatResp, err := ollamaProvider.Generate(ctx, &chatReq)
// 	if err != nil {
// 		log.Printf("Ollama Provider Generation Failed: %s", err.Error())
// 		http.Error(w,
// 			err.Error(),
// 			http.StatusInternalServerError)
// 		return
// 	}

// 	respContent := chatResp.Message.Content
// 	w.Write([]byte(respContent))
// }

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Good Health"))
}

// ============================ TEST End =============================

func (s *HttpServer) Start() error {
	return s.server.ListenAndServe()
}

func (s *HttpServer) Stop() error {
	return s.server.Close()
}

func (s *HttpServer) GetType() ServerType {
	return HTTP
}

func (s *HttpServer) Submit(chat api.ChatRequest) (api.TaskID, error) {
	taskID := api.TaskID("test")
	event := task.GenerateTaskPreSumbitEvent(taskID, chat)
	return taskID, s.eventBus.Publish(api.TASK_SUBMITTED, event)
}

type HttpServerFactory struct{}

func (sf *HttpServerFactory) Create(config *viper.Viper, eventBus eventbus.EventBus) (Server, error) {
	newHttp, err := newHttpServer(config, eventBus)
	if err != nil {
		return nil, err
	}
	return newHttp, nil
}

func init() {
	RegisterServerFactory(HTTP, &HttpServerFactory{})
}

// internel/server/httpServer.go
package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"log"

	"github.com/cpmores/lucinda/api/v1"
	"github.com/cpmores/lucinda/internel/provider"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
)

type HttpServer struct {
	config *viper.Viper
	server *http.Server
	router *mux.Router
}

// ========================== Only for TEST =========================
func newHttpServer(config *viper.Viper) (*HttpServer, error) {
	httpServer := &HttpServer{
		config: config,
		router: mux.NewRouter(),
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
	hs.router.HandleFunc("/ollama", ollamaChatHandler)
	hs.router.HandleFunc("/healthz", healthzHandler)
}

func ollamaChatHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	ollamaProvider := provider.Providers["ollama"]
	reqContent, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		http.Error(w,
			err.Error(),
			http.StatusInternalServerError)
		return
	}
	reqMessages := []api.Message{
		{
			Role:    "user",
			Content: string(reqContent),
		},
	}

	chatReq := api.ChatRequest{
		Model:    "gemma3",
		Messages: reqMessages,
	}
	chatResp, err := ollamaProvider.Generate(ctx, &chatReq)
	if err != nil {
		log.Printf("Ollama Provider Generation Failed: %s", err.Error())
		http.Error(w,
			err.Error(),
			http.StatusInternalServerError)
		return
	}

	respContent := chatResp.Message.Content
	w.Write([]byte(respContent))
}

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

type HttpServerFactory struct{}

func (sf *HttpServerFactory) Create(config *viper.Viper) (Server, error) {
	newHttp, err := newHttpServer(config)
	if err != nil {
		return nil, err
	}
	return newHttp, nil
}

func init() {
	RegisterServerFactory(HTTP, &HttpServerFactory{})
}

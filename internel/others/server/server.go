// internel/server/server.go
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/cpmores/lucinda/api/v1"
	eventbus "github.com/cpmores/lucinda/internel/eventBus"
	"github.com/spf13/viper"
)

type ServerType string

const (
	HTTP      ServerType = "http"
	GRPC      ServerType = "grpc"
	WebSocket ServerType = "websocket"
	Metrics   ServerType = "metrics"
	Admin     ServerType = "admin"
)

// Server Interface
type Server interface {
	Start() error
	Stop() error
	Submit(chat api.ChatRequest) (api.TaskID, error)
	GetType() ServerType
}

// ServerFactory interface
type ServerFactory interface {
	Create(config *viper.Viper, eventBus eventbus.EventBus) (Server, error)
}

// ServerFactory Registery
var serverFactories = make(map[ServerType]ServerFactory)

func RegisterServerFactory(serverType ServerType, factory ServerFactory) {
	serverFactories[serverType] = factory
}

// CreateServer according to ServerType
func StartServer(ctx context.Context, serverType ServerType, eventBus eventbus.EventBus, config *viper.Viper) error {
	factory, ok := serverFactories[serverType]
	if !ok {
		return fmt.Errorf("unsupported server type: %s", serverType)
	}

	server, err := factory.Create(config, eventBus)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := server.Stop(); err != nil {
			log.Printf("Error stopping server: %s", err.Error())
		} else {
			log.Printf("%s server stopped successfully\n", serverType)
		}
	}()

	log.Printf("Starting %s server...\n", serverType)
	if err := server.Start(); err != nil && err != http.ErrServerClosed {
		log.Printf("%s server error: %s", serverType, err.Error())
	}

	return err
}

func GetServerFactory(t ServerType) (ServerFactory, error) {
	factory, ok := serverFactories[t]
	if !ok {
		return nil, fmt.Errorf("Type %s server not Found", t)
	}

	return factory, nil
}

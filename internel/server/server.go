// internel/server/server.go
package server

import (
	"fmt"

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
	GetType() ServerType
}

// ServerFactory interface
type ServerFactory interface {
	Create(config *viper.Viper) (Server, error)
}

// ServerFactory Registery
var serverFactories = make(map[ServerType]ServerFactory)

func RegisterServerFactory(serverType ServerType, factory ServerFactory) {
	serverFactories[serverType] = factory
}

// CreateServer according to ServerType
func CreateServer(serverType ServerType, config *viper.Viper) (Server, error) {
	factory, ok := serverFactories[serverType]
	if !ok {
		return nil, fmt.Errorf("unsupported server type: %s", serverType)
	}

	return factory.Create(config)
}

func GetServerFactory(t ServerType) (ServerFactory, error) {
	factory, ok := serverFactories[t]
	if !ok {
		return nil, fmt.Errorf("Type %s server not Found", t)
	}

	return factory, nil
}
package transport

import (
	"context"
	"fmt"

	api "github.com/cpmores/lucinda/api/v1"
	"github.com/spf13/viper"
)

var TransporterFactories = make(map[string]TransporterFactory)

type Transporter interface {
	Start(ctx context.Context) error
	Stop() error
	ID() api.NodeID // get the only node ID

	// node connection
	Dial(ctx context.Context, targetAddr string) error // connect to a node
	Peers() []api.NodeID                               // get the connected nodes

	Send(ctx context.Context, nodeID api.NodeID, msg *api.NodeMessage) error // send a message to a node
}

func RegisterTransporterFactory(name string, factory TransporterFactory) {
	TransporterFactories[name] = factory
}

type TransporterFactory interface {
	Create(config *viper.Viper) (Transporter, error)
}

func CreateTransporter(name string) (Transporter, error) {
	config := viper.GetViper()
	factory, exists := TransporterFactories[name]
	if !exists {
		return nil, fmt.Errorf("transporter factory %s not found", name)
	}
	return factory.Create(config)
}

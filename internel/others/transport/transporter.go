package transport

import (
	"context"
	"fmt"
	"log"

	api "github.com/cpmores/lucinda/api/v1"
	eventbus "github.com/cpmores/lucinda/internel/eventBus"
	"github.com/spf13/viper"
)

var TransporterFactories = make(map[string]TransporterFactory)

// transport layer for inter-node connection
type Transporter interface {
	Start(ctx context.Context) error
	Stop() error
	ID() api.NodeID // get the only node ID

	// node connection
	Dial(ctx context.Context, targetAddr string) error // connect to a node
	Peers() []api.NodeID                               // get the connected nodes

	Send(ctx context.Context, nodeID api.NodeID, msg *api.NodeMessage) error // send a message to a node
	Publish(ctx context.Context, msg *api.NodeMessage) error                 // publish a message to all the neighbours
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

// transport message writer and reader
type NodePostman interface {
	Start(ctx context.Context) error // output inter-node operations
	ListenAndServe(ctx context.Context) error
	SendMsg(ctx context.Context, nodeID api.NodeID, msg *api.NodeMessage) error // send a message to a node
	PublishMsg(ctx context.Context, msg *api.NodeMessage) error                 // publish a message to all the neighbours
}

type Postman struct {
	Transporter Transporter
	EventBus    eventbus.EventBus
}

func (p *Postman) Start(ctx context.Context) error {
	if err := p.Transporter.Start(ctx); err != nil {
		return fmt.Errorf("failed to start transporter: %w", err)
	}

	log.Printf("Node Postman created with transporter: %s", p.Transporter.ID())
	return p.ListenAndServe(ctx)
}

// ListenAndServe listen for incoming messages and handle
func (p *Postman) ListenAndServe(ctx context.Context) error {
	// TODO
	return nil
}

func (p *Postman) SendMsg(ctx context.Context, nodeID api.NodeID, msg *api.NodeMessage) error {
	return p.Transporter.Send(ctx, nodeID, msg)
}

func (p *Postman) PublishMsg(ctx context.Context, msg *api.NodeMessage) error {
	return p.Transporter.Publish(ctx, msg)
}

func StartNodePostman(ctx context.Context, eventBus eventbus.EventBus, config *viper.Viper) error {
	// create transporter
	// TOOD: support multiple transporters
	transporter, err := CreateTransporter("libp2p")
	if err != nil {
		return fmt.Errorf("failed to create transporter: %w", err)
	}

	postman := &Postman{
		Transporter: transporter,
		EventBus:    eventBus,
	}

	return postman.Start(ctx)
}

// package transport provides the interface for the transport layer of this application
// the transport layer is responsible for receiving NodeMessage from network
// and sending NodeMessage to network
package transport

import (
	"context"
	APINode "github.com/cpmores/lucinda/api/v1/node"
)

// Transport is the interface dealing with messages between nodes
type Transport interface {
	// Start: begin managing in and out messages
	Start(ctx context.Context) error
	// Open: open transport protocal , allowing it to receive messages from the network
	Open(ctx context.Context, protocol APINode.Protocol) error
	// Close: close transport protocal, stop receiving messages from the network on specific protocol
	Close(ctx context.Context, protocol APINode.Protocol) error
	// Stop: stop managing in and out messages, close transport protocal
	Stop() error

	// message transfer methods
	// Send: send a message to a specific node
	Send(ctx context.Context, to APINode.NodeID, message APINode.NodeMessage) error
	// Publish: send a message to all nodes in the network
	Publish(ctx context.Context, message APINode.NodeMessage) error
}

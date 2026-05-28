// TransportLibp2p is a transport implementation that
// uses libp2p to send and receive messages between nodes in the network
package transport_libp2p

import (
	"context"
	APINode "github.com/cpmores/lucinda/api/v1/node"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-msgio"
	"github.com/multiformats/go-multiaddr"
	"sync"
)

// Libp2pTransport is a transport implementation that
// uses libp2p to send and receive messages between nodes in the network
type Libp2pTransport struct {
	// Lock
	sync.RWMutex
	// Libp2p components
	// NodeID is the unique identifier for this node in the network
	NodeID APINode.NodeID
	// Host is the libp2p host that manages the network connections
	Host host.Host
	// Options is the list of options used to create the libp2p host
	Options []libp2p.Option
	// IsStarted indicates whether the transport is currently running
	IsStarted bool

	// In and Out channels for managing incoming and outgoing messages
	// Outs is a cache of outgoing message
	outs map[APINode.NodeID]map[APINode.Protocol]chan APINode.NodeMessage
	// Ins is a cache of incoming message
	ins map[APINode.Protocol]chan APINode.NodeMessage
}

type Libp2pTransportOptions struct {
	Addrs      []string
	OutsLength int64
	InsLength  int64
}

func NewLibp2pTransport(options Libp2pTransportOptions) (*Libp2pTransport, error) {
	var opts []libp2p.Option
	for _, opt := range options.Addrs {
		opts = append(opts, libp2p.ListenAddrStrings(opt))
	}

	return &Libp2pTransport{
		Options:   opts,
		IsStarted: false,
		outs:      make(map[APINode.NodeID]map[APINode.Protocol]chan APINode.NodeMessage),
		ins:       make(map[APINode.Protocol]chan APINode.NodeMessage),
	}, nil
}

// TODO: implement the methods of the Transport interface for Libp2pTransport
func (lt *Libp2pTransport) Start(ctx context.Context) error {
	return nil
}

func (lt *Libp2pTransport) Open(ctx context.Context, protocol APINode.Protocol) error {
	return nil
}

func (lt *Libp2pTransport) Close(ctx context.Context, protocol APINode.Protocol) error {
	return nil
}

func (lt *Libp2pTransport) Stop() error {
	return nil
}

func (lt *Libp2pTransport) Send(ctx context.Context, to APINode.NodeID, message APINode.NodeMessage) error {
	return nil
}

func (lt *Libp2pTransport) Publish(ctx context.Context, message APINode.NodeMessage) error {
	return nil
}

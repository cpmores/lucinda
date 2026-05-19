package libp2p

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cpmores/lucinda/api/v1"
	"github.com/cpmores/lucinda/internel/transport"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-msgio"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/viper"
)

const (
	INCOMING_CHANNEL_SIZE  = 100
	OUTCOMING_CHANNEL_SIZE = 20
	SENDWORKER_RETRY_LIMIT = 3
)

// protocol ID for the stream handler
const (
	STREAM_PROTOCOL_ID = "/lucinda/1.0.0"
)

var DefaultAddrs = []string{"/ip4/0.0.0.0/tcp/0"}

type Libp2pTransporter struct {
	Host      host.Host
	Options   []libp2p.Option
	IsStarted bool

	// channels for incoming and outcoming nodes
	Incoming   map[string]chan *api.NodeMessage
	Outcomings map[string]map[peer.ID]chan *api.NodeMessage

	sync.RWMutex
}

// build a libp2p host with the given addresses
func NewLibp2pTransporter(addrs []string) (*Libp2pTransporter, error) {
	var opts []libp2p.Option
	for _, opt := range addrs {
		opts = append(opts, libp2p.ListenAddrStrings(opt))
	}

	return &Libp2pTransporter{
		Options:    opts,
		IsStarted:  false,
		Incoming:   make(map[string]chan *api.NodeMessage),
		Outcomings: make(map[string]map[peer.ID]chan *api.NodeMessage),
	}, nil
}

func (p2p *Libp2pTransporter) Start(ctx context.Context) error {
	p2p.Lock()
	defer p2p.Unlock()
	if p2p.IsStarted {
		return fmt.Errorf("libp2p %s is already started", p2p.Host.ID())
	}

	// create a new libp2p host
	host, err := libp2p.New(p2p.Options...)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %v", err)
	}

	p2p.Host = host
	p2p.IsStarted = true

	go func() {
		<-ctx.Done()
		log.Printf("context is done, stopping libp2p transporter %s", p2p.Host.ID())
		p2p.Stop()
	}()

	// log the host ID and listening addresses
	log.Printf("libp2p host started with ID: %s", p2p.Host.ID())
	for _, addr := range p2p.Host.Addrs() {
		log.Printf("libp2p host listening on: %s", addr)
	}

	// register a stream handler for incoming messages
	p2p.SubscribeProtocol(ctx, STREAM_PROTOCOL_ID)

	// mDNS discovery
	err = p2p.setupDiscovery()
	if err != nil {
		return fmt.Errorf("failed to setup mDNS discovery: %s", err.Error())
	}
	log.Printf("libp2p mDNS setup successfully")

	return nil
}

func (p2p *Libp2pTransporter) Stop() error {
	p2p.Lock()
	defer p2p.Unlock()
	if !p2p.IsStarted {
		return fmt.Errorf("libp2p %s is not started", p2p.Host.ID())
	}

	// close the host
	if err := p2p.Host.Close(); err != nil {
		return fmt.Errorf("failed to close libp2p host: %v", err)
	}

	p2p.IsStarted = false
	return nil
}

func (p2p *Libp2pTransporter) ID() api.NodeID {
	return api.NodeID(p2p.Host.ID())
}

func (p2p *Libp2pTransporter) Dial(ctx context.Context, targetAddr string) error {
	peerNA, err := multiaddr.NewMultiaddr(targetAddr)
	if err != nil {
		return fmt.Errorf("failed to parse target address: %s", err.Error())
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(peerNA)
	if err != nil {
		return fmt.Errorf("failed to get peer info from target address: %s", err.Error())
	}

	if err := p2p.Host.Connect(ctx, *peerInfo); err != nil {
		return fmt.Errorf("failed to connect to peer: %s", err.Error())
	}

	return nil
}

func (p2p *Libp2pTransporter) Peers() []api.NodeID {
	p2p.RLock()
	defer p2p.RUnlock()
	peerIDs := p2p.Host.Peerstore().Peers()
	nodeIDs := make([]api.NodeID, len(peerIDs))
	for i, peerID := range peerIDs {
		nodeIDs[i] = api.NodeID(peerID)
	}

	return nodeIDs
}

func (p2p *Libp2pTransporter) Send(ctx context.Context, nodeID api.NodeID, msg *api.NodeMessage) error {
	// get channel
	targetPeerID, err := peer.Decode(string(nodeID))
	if err != nil {
		return fmt.Errorf("failed to decode node ID: %s", err.Error())
	}

	protocolID := msg.ProtocolID
	ch := p2p.GetOutcomingChannel(ctx, protocolID, targetPeerID)
	select {
	case ch <- msg:
	case <-ctx.Done():
		return fmt.Errorf("failed to send message to node %s: context is done", nodeID)
	default:
		return fmt.Errorf("failed to send message to node %s: channel is full", nodeID)
	}
	return nil
}

func (p2p *Libp2pTransporter) Publish(ctx context.Context, msg *api.NodeMessage) error {
	// TODO
	return nil
}

// ====================== Additional Functions For Libp2pTransporter ======================
// using json as the message format, send a message to the target node
// accept specific protocol from other nodes
func (p2p *Libp2pTransporter) SubscribeProtocol(ctx context.Context, protocolID string) {
	// setup a channel for incomming message
	ch := make(chan *api.NodeMessage, INCOMING_CHANNEL_SIZE)
	p2p.Incoming[protocolID] = ch

	// register a stream handler for the protocol
	p2p.Host.SetStreamHandler(protocol.ID(protocolID), func(net network.Stream) {
		// read message from the stream
		defer net.Close()
		reader := msgio.NewVarintReader(net)

		done := make(chan struct{})
		go func() {
			<-ctx.Done()
			reader.Close()
			close(done)
		}()

		for {
			select {
			case <-done:
				return
			default:
				msgBytes, err := reader.ReadMsg()
				if err != nil {
					log.Printf("failed to read message from stream: %s", err.Error())
				}

				var msg api.NodeMessage
				if err := json.Unmarshal(msgBytes, &msg); err != nil {
					log.Printf("failed to unmarshal message: %s", err.Error())
				} else {
					select {
					case ch <- &msg:
					default:
						log.Printf("incoming message channel for protocol %s is full, dropping message from %s", protocolID, msg.From)
					}
				}
			}
		}
	})
}

// get outcoming channel for the target peer and protocol
func (p2p *Libp2pTransporter) GetOutcomingChannel(ctx context.Context, protocolID string, target peer.ID) chan *api.NodeMessage {
	p2p.Lock()
	defer p2p.Unlock()

	if _, ok := p2p.Outcomings[protocolID]; !ok {
		p2p.Outcomings[protocolID] = make(map[peer.ID]chan *api.NodeMessage)
	}

	ch, ok := p2p.Outcomings[protocolID][target]
	if ok {
		return ch
	}

	// if not created, create a new channel for the target peer
	ch = make(chan *api.NodeMessage, OUTCOMING_CHANNEL_SIZE)
	p2p.Outcomings[protocolID][target] = ch

	// start a worker focused on the channel
	go p2p.sendWorker(ctx, protocolID, target, ch)
	return ch
}

// send message to targets
func (p2p *Libp2pTransporter) sendWorker(ctx context.Context, protocolID string, target peer.ID, ch chan *api.NodeMessage) {
	var stream network.Stream
	var writer msgio.WriteCloser

	defer func() {
		if writer != nil {
			writer.Close()
		}
		if stream != nil {
			stream.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}

			if stream == nil {
				var err error
				stream, err = p2p.Host.NewStream(ctx, target, protocol.ID(protocolID))
				if err != nil {
					log.Printf("failed to dial: %v", err)
					continue
				}
				writer = msgio.NewVarintWriter(stream)
			}

			data, _ := json.Marshal(msg)
			if err := writer.WriteMsg(data); err != nil {
				log.Printf("write error, reset stream: %v", err)
				stream.Close()
				stream = nil
			}
		}
	}
}

func (p2p *Libp2pTransporter) cleanOutcomingChannel(protocolID string, target peer.ID) {
	time.Sleep(2 * time.Second)

	p2p.Lock()
	defer p2p.Unlock()

	if p2p.Host.Network().Connectedness(target) == network.Connected {
		log.Printf("peer %s is still connected, not closing channel for protocol %s", target, protocolID)
		return
	}

	if peerMap, ok := p2p.Outcomings[protocolID]; ok {
		if ch, exists := peerMap[target]; exists {
			close(ch)
			delete(peerMap, target)
			log.Printf("protocol %s under peer %s channel has been closed and removed", protocolID, target)
		}

		if len(peerMap) == 0 {
			delete(p2p.Outcomings, protocolID)
		}
	}
}

type Libp2pTransporterFactory struct{}

func (f *Libp2pTransporterFactory) Create(config *viper.Viper) (transport.Transporter, error) {
	addrs := config.GetStringSlice("transport.libp2p.addrs")
	if len(addrs) == 0 {
		addrs = DefaultAddrs
	}

	return NewLibp2pTransporter(addrs)
}

func init() {
	transport.RegisterTransporterFactory("libp2p", &Libp2pTransporterFactory{})
	log.Printf("libp2p transporter Registered")
}

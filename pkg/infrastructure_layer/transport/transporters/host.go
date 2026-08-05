// Package libp2p is a transport implementation that
// uses libp2p to send and receive messages between nodes in the network
package libp2p

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	APINode "github.com/cpmores/lucinda/api/v1/domain/node"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-msgio"
	"github.com/multiformats/go-multiaddr"

	logger "github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

const (
	defaultInsLength  = 100
	defaultOutsLength = 20
)

// Libp2pTransport is a transport implementation that
// uses libp2p to send and receive messages between nodes in the network
type Libp2pTransport struct {
	sync.RWMutex
	// NodeID is the unique identifier for this node in the network
	NodeID APINode.NodeID
	// Host is the libp2p host that manages the network connections
	Host host.Host
	// Options is the list of options used to create the libp2p host
	Options []libp2p.Option
	// IsStarted indicates whether the transport is currently running
	IsStarted bool

	// outsLength is the buffer size for outgoing message channels
	outsLength int64
	// insLength is the buffer size for incoming message channels
	insLength int64

	// outs caches outgoing message channels: target NodeID -> Protocol -> chan
	outs map[APINode.NodeID]map[APINode.Protocol]chan APINode.NodeMessage
	// ins caches incoming message channels: Protocol -> chan
	ins map[APINode.Protocol]chan APINode.NodeMessage

	// mdnsService tracks the active mDNS discovery service for shutdown.
	mdnsService mdns.Service

	// log is the component logger; defaults to Discard when not provided.
	log *logger.Logger
}

type Libp2pTransportOptions struct {
	Addrs      []string
	OutsLength int64
	InsLength  int64
	Logger     *logger.Logger
}

func NewLibp2pTransport(options Libp2pTransportOptions) (*Libp2pTransport, error) {
	var opts []libp2p.Option
	for _, addr := range options.Addrs {
		opts = append(opts, libp2p.ListenAddrStrings(addr))
	}

	outsLen := options.OutsLength
	if outsLen <= 0 {
		outsLen = defaultOutsLength
	}
	insLen := options.InsLength
	if insLen <= 0 {
		insLen = defaultInsLength
	}

	// Default to a silent logger when none is provided, so a nil logger
	// can never panic a log call inside the transport.
	log := options.Logger
	if log == nil {
		log = logger.Discard()
	}

	log.Info("created", "addrs", options.Addrs)
	return &Libp2pTransport{
		Options:    opts,
		IsStarted:  false,
		outs:       make(map[APINode.NodeID]map[APINode.Protocol]chan APINode.NodeMessage),
		ins:        make(map[APINode.Protocol]chan APINode.NodeMessage),
		outsLength: outsLen,
		insLength:  insLen,
		log:        log,
	}, nil
}

func (lt *Libp2pTransport) ID() APINode.NodeID {
	return lt.NodeID
}

// Start creates the libp2p host and begins listening on configured addresses.
func (lt *Libp2pTransport) Start(ctx context.Context) error {
	lt.Lock()
	defer lt.Unlock()

	if lt.IsStarted {
		return fmt.Errorf("libp2p transport is already started")
	}

	host, err := libp2p.New(lt.Options...)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}

	lt.Host = host
	lt.NodeID = APINode.NodeID(host.ID().String())
	lt.IsStarted = true

	// register network notifee: clean up outbound channels when peers disconnect
	host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			lt.log.Debug("peer connected", "peer", conn.RemotePeer())
		},
		DisconnectedF: func(n network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()
			lt.log.Debug("peer disconnected, cleaning outbound channels", "peer", peerID)
			// Spawn a goroutine to avoid deadlocking with the swarm's
			// internal notification lock during host.Close().
			go lt.cleanOutboundChannelsForPeer(APINode.NodeID(peerID.String()))
		},
	})

	// auto-shutdown when context is cancelled
	go func() {
		<-ctx.Done()
		lt.log.Info("context done, stopping")
		if err := lt.Stop(); err != nil {
			lt.log.Error("stop error", "err", err)
		}
	}()

	lt.log.Info("started", "node_id", lt.NodeID, "addrs", host.Addrs())
	return nil
}

// Open registers a stream handler for the given protocol so this node
// can receive messages on that protocol from the network.
func (lt *Libp2pTransport) Open(ctx context.Context, proto APINode.Protocol) error {
	lt.Lock()
	defer lt.Unlock()

	if !lt.IsStarted {
		return fmt.Errorf("libp2p transport is not started")
	}

	if _, exists := lt.ins[proto]; exists {
		return fmt.Errorf("protocol %s is already open", proto)
	}

	ch := make(chan APINode.NodeMessage, lt.insLength)
	lt.ins[proto] = ch

	// open an outbound channel for self-connection
	lt.selfOutConnectLocked(ctx, proto)

	protocolID := protocol.ID(proto)
	lt.Host.SetStreamHandler(protocolID, func(stream network.Stream) {
		defer stream.Close()
		reader := msgio.NewVarintReader(stream)

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
					return
				}
				var msg APINode.NodeMessage
				if err := json.Unmarshal(msgBytes, &msg); err != nil {
					lt.log.Error("failed to unmarshal message", "protocol", proto, "err", err)
					continue
				}
				select {
				case ch <- msg:
				case <-done:
					return
				default:
					lt.log.Warn("incoming channel full, dropping message", "protocol", proto)
				}
			}
		}
	})

	lt.log.Debug("opened protocol", "protocol", proto)
	return nil
}

// Close removes the stream handler for the given protocol and stops
// receiving messages on that protocol.
func (lt *Libp2pTransport) Close(ctx context.Context, proto APINode.Protocol) error {
	lt.Lock()
	defer lt.Unlock()

	if !lt.IsStarted {
		return fmt.Errorf("libp2p transport is not started")
	}

	ch, exists := lt.ins[proto]
	if !exists {
		return fmt.Errorf("protocol %s is not open", proto)
	}

	lt.Host.RemoveStreamHandler(protocol.ID(proto))
	close(ch)
	delete(lt.ins, proto)

	// self-disconnect to clean up any self-connection for this protocol
	lt.selfDisconnectLocked(proto)

	lt.log.Debug("closed protocol", "protocol", proto)
	return nil
}

// Stop shuts down the libp2p host and closes all channels.
func (lt *Libp2pTransport) Stop() error {
	lt.Lock()
	if !lt.IsStarted {
		lt.Unlock()
		return fmt.Errorf("libp2p transport is not started")
	}

	// close all incoming channels
	for proto, ch := range lt.ins {
		close(ch)
		delete(lt.ins, proto)
		lt.log.Debug("closed incoming channel", "protocol", proto)
	}

	// close all outgoing channels
	for nodeID, protoMap := range lt.outs {
		for proto, ch := range protoMap {
			close(ch)
			lt.log.Debug("closed outgoing channel", "node", nodeID, "protocol", proto)
		}
		delete(lt.outs, nodeID)
	}

	mdnsSvc := lt.mdnsService
	lt.mdnsService = nil
	h := lt.Host
	lt.IsStarted = false
	lt.Unlock()

	// Release the lock BEFORE closing mDNS and host. The host close
	// triggers DisconnectedF callbacks, which spawn goroutines that
	// call cleanOutboundChannelsForPeer — those need the lock too.

	if mdnsSvc != nil {
		if err := mdnsSvc.Close(); err != nil {
			lt.log.Error("failed to close mDNS service", "err", err)
		}
	}

	if h != nil {
		if err := h.Close(); err != nil {
			return fmt.Errorf("failed to close libp2p host: %w", err)
		}
	}

	lt.log.Info("stopped")
	return nil
}

// Send delivers a message to a specific node. The message's Protocol field
// determines which stream is used. If no outbound channel exists yet for
// (to, protocol), one is created and a sendWorker goroutine is started.
func (lt *Libp2pTransport) Send(ctx context.Context, to APINode.NodeID, message APINode.NodeMessage) error {
	ch := lt.getOrCreateOutChannel(ctx, to, message.Protocol)

	select {
	case ch <- message:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context cancelled while sending to %s", to)
	default:
		return fmt.Errorf("outbound channel full for node %s protocol %s", to, message.Protocol)
	}
}

// Publish sends a message to all currently connected peers.
func (lt *Libp2pTransport) Publish(ctx context.Context, message APINode.NodeMessage) error {
	lt.RLock()
	peers := lt.Host.Peerstore().Peers()
	lt.RUnlock()

	var lastErr error
	for _, p := range peers {
		if p == lt.Host.ID() {
			continue // don't publish to self
		}
		nodeID := APINode.NodeID(p.String())
		if err := lt.Send(ctx, nodeID, message); err != nil {
			lt.log.Warn("publish failed", "node", nodeID, "err", err)
			lastErr = err
		}
	}
	return lastErr
}

// getOrCreateOutChannel returns the buffered channel for (to, protocol),
// creating it and launching a sendWorker if it doesn't exist yet.
func (lt *Libp2pTransport) getOrCreateOutChannel(ctx context.Context, to APINode.NodeID, proto APINode.Protocol) chan APINode.NodeMessage {
	lt.Lock()
	defer lt.Unlock()

	if _, ok := lt.outs[to]; !ok {
		lt.outs[to] = make(map[APINode.Protocol]chan APINode.NodeMessage)
	}

	ch, ok := lt.outs[to][proto]
	if ok {
		return ch
	}

	ch = make(chan APINode.NodeMessage, lt.outsLength)
	lt.outs[to][proto] = ch

	go lt.sendWorker(ctx, to, proto, ch)
	return ch
}

// sendWorker drains the outbound channel and writes messages to the libp2p
// stream for (peer, protocol). If the stream breaks, it is recreated lazily
// on the next message.
func (lt *Libp2pTransport) sendWorker(ctx context.Context, to APINode.NodeID, protocolID APINode.Protocol, ch chan APINode.NodeMessage) {
	var stream network.Stream
	var writer msgio.WriteCloser

	defer func() {
		if writer != nil {
			writer.Close()
		}
		if stream != nil {
			stream.Close()
		}
		lt.removeOutChannel(to, protocolID)
	}()

	targetPeer, err := peer.Decode(string(to))
	if err != nil {
		lt.log.Error("failed to decode peer ID", "to", to, "err", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}

			// lazily open stream on first message or after a break
			if stream == nil {
				stream, err = lt.Host.NewStream(ctx, targetPeer, protocol.ID(protocolID))
				if err != nil {
					lt.log.Error("failed to open stream", "to", to, "err", err)
					continue
				}
				writer = msgio.NewVarintWriter(stream)
			}

			msg.Timestamp = time.Now().Unix()
			msg.From = lt.NodeID
			msg.To = to
			data, _ := json.Marshal(msg)
			if err := writer.WriteMsg(data); err != nil {
				lt.log.Error("write error, resetting stream", "to", to, "err", err)
				stream.Close()
				stream = nil
				writer = nil
			}
		}
	}
}

// removeOutChannel cleans up the outbound channel for (to, proto).
func (lt *Libp2pTransport) removeOutChannel(to APINode.NodeID, proto APINode.Protocol) {
	lt.Lock()
	defer lt.Unlock()

	if protoMap, ok := lt.outs[to]; ok {
		delete(protoMap, proto)
		if len(protoMap) == 0 {
			delete(lt.outs, to)
		}
	}
}

// Dial connects to a peer at the given multiaddr.
func (lt *Libp2pTransport) Dial(ctx context.Context, targetAddr string) error {
	peerMA, err := multiaddr.NewMultiaddr(targetAddr)
	if err != nil {
		return fmt.Errorf("failed to parse multiaddr %s: %w", targetAddr, err)
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(peerMA)
	if err != nil {
		return fmt.Errorf("failed to extract peer info from %s: %w", targetAddr, err)
	}

	if err := lt.Host.Connect(ctx, *peerInfo); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", targetAddr, err)
	}

	lt.log.Info("connected to", "addr", targetAddr)
	return nil
}

// Peers returns the NodeIDs of all connected peers.
func (lt *Libp2pTransport) Peers() []APINode.NodeID {
	lt.RLock()
	defer lt.RUnlock()

	peerIDs := lt.Host.Peerstore().Peers()
	nodeIDs := make([]APINode.NodeID, 0, len(peerIDs))
	for _, p := range peerIDs {
		if p == lt.Host.ID() {
			continue
		}
		nodeIDs = append(nodeIDs, APINode.NodeID(p.String()))
	}
	return nodeIDs
}

// Incoming returns the receive channel for a protocol, or error if not open.
func (lt *Libp2pTransport) Incoming(proto APINode.Protocol) (<-chan APINode.NodeMessage, error) {
	lt.RLock()
	defer lt.RUnlock()
	ch, ok := lt.ins[proto]
	if !ok {
		return nil, fmt.Errorf("protocol %s not open", proto)
	}
	return ch, nil
}

// cleanOutboundChannelsForPeer removes all outbound channels for a
// disconnected peer. Called automatically by the network notifee.
// Runs in its own goroutine to avoid deadlocking with swarm internals.
func (lt *Libp2pTransport) cleanOutboundChannelsForPeer(nodeID APINode.NodeID) {
	// Recover from panic if Stop() already closed the channels first.
	defer func() {
		if r := recover(); r != nil {
			lt.log.Warn("recovered panic cleaning channels", "node", nodeID, "recover", r)
		}
	}()

	lt.Lock()
	defer lt.Unlock()

	if protoMap, ok := lt.outs[nodeID]; ok {
		for proto, ch := range protoMap {
			close(ch)
			lt.log.Debug("cleaned outbound channel", "node", nodeID, "protocol", proto)
		}
		delete(lt.outs, nodeID)
	}
}

// =============================================================================
// mDNS Discovery
// =============================================================================

const defaultMDNSNamespace = "lucinda"

// mdnsNotifee implements mdns.Notifee. When a new peer is discovered via mDNS
// it automatically connects and logs the result.
type mdnsNotifee struct {
	host host.Host
	log  *logger.Logger
}

// HandlePeerFound is called by the mDNS service when a peer is discovered
// on the local network.
func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == n.host.ID() {
		return // don't connect to self
	}

	if len(n.host.Network().ConnsToPeer(pi.ID)) > 0 {
		return // already connected
	}

	ctx := context.Background()
	if err := n.host.Connect(ctx, pi); err != nil {
		n.log.Error("failed to connect to discovered peer", "peer", pi.ID, "err", err)
		return
	}
	n.log.Debug("discovered and connected to peer", "peer", pi.ID)
}

// DiscoverMDNS starts mDNS-based peer discovery on the local network.
//
// Nodes on the same LAN with mDNS enabled on the same namespace will
// automatically discover and connect to each other.
//
// Call this after Start. Only one mDNS service is allowed at a time;
// calling DiscoverMDNS again while one is running is a no-op.
func (lt *Libp2pTransport) DiscoverMDNS(namespace string) error {
	lt.Lock()
	defer lt.Unlock()

	if !lt.IsStarted {
		return fmt.Errorf("libp2p transport is not started")
	}

	if lt.mdnsService != nil {
		lt.log.Warn("discovery already active, skipping")
		return nil
	}

	if namespace == "" {
		namespace = defaultMDNSNamespace
	}

	notifee := &mdnsNotifee{host: lt.Host, log: lt.log}
	lt.mdnsService = mdns.NewMdnsService(lt.Host, namespace, notifee)

	if err := lt.mdnsService.Start(); err != nil {
		lt.mdnsService = nil
		return fmt.Errorf("failed to start mDNS discovery: %w", err)
	}

	lt.log.Info("discovery started", "namespace", namespace)
	return nil
}

// selfOutConnectLocked creates an outbound self-channel so the node can
// send messages to its own incoming handlers. Caller must hold lt.Lock().
func (lt *Libp2pTransport) selfOutConnectLocked(ctx context.Context, proto APINode.Protocol) {
	selfNodeID := APINode.NodeID(lt.Host.ID().String())
	if _, ok := lt.outs[selfNodeID]; !ok {
		lt.outs[selfNodeID] = make(map[APINode.Protocol]chan APINode.NodeMessage)
	}

	_, ok := lt.outs[selfNodeID][proto]
	if ok {
		return // already connected
	}

	ch := make(chan APINode.NodeMessage, lt.outsLength)
	lt.outs[selfNodeID][proto] = ch

	// capture references so the goroutine doesn't touch the map.
	insCh := lt.ins[proto]
	go lt.selfSendWorker(ctx, ch, insCh)
}

// selfDisconnectLocked closes the self-connection outbound channel for
// the given protocol. Caller must hold lt.Lock().
func (lt *Libp2pTransport) selfDisconnectLocked(proto APINode.Protocol) {
	selfNodeID := APINode.NodeID(lt.Host.ID().String())

	protoMap, ok := lt.outs[selfNodeID]
	if !ok {
		return
	}

	ch, ok := protoMap[proto]
	if !ok {
		return
	}

	close(ch)
	delete(protoMap, proto)
}

// selfSendWorker drains the self-outbound channel and pushes each message
// into the matching incoming channel (bypassing the network for local delivery).
// It receives channel references directly so it never touches the outs/ins maps.
func (lt *Libp2pTransport) selfSendWorker(ctx context.Context, outCh <-chan APINode.NodeMessage, insCh chan<- APINode.NodeMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-outCh:
			if !ok {
				return
			}
			select {
			case insCh <- msg:
			case <-ctx.Done():
				return
			default:
				lt.log.Warn("self-send dropped, incoming channel full", "from", msg.From)
			}
		}
	}
}

// ── AvailableModule Interface ──────────────────────────────────────────────────────────

func (lt *Libp2pTransport) GetModuleType() APIModule.ModuleType {
	return APIModule.Transport
}

func (lt *Libp2pTransport) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(lt.GetModuleType(), "libp2p")
}

func (lt *Libp2pTransport) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(lt.GetModuleID(), lt.GetModuleType(), APIModule.Running)
}

func (lt *Libp2pTransport) RegisterWithManager(manager modulemanager.ModuleManager) error {
	return manager.Register(lt)
}

func (lt *Libp2pTransport) DependsOn() map[APIModule.ModuleType]string {
	return nil
}

func (lt *Libp2pTransport) DependsEnable() error {
	return nil
}

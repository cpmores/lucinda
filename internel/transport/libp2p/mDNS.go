package libp2p

import (
	"context"
	"fmt"
	"log"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

const DISCOVERY_NAMESPACE = "_lucinda._tcp"

// node discovery service using libp2p's mDNS
type Libp2pNodeDiscovery struct {
	Host host.Host
}

func (d *Libp2pNodeDiscovery) HandlePeerFound(pi peer.AddrInfo) {
	log.Printf("discovered new peer: %s", pi.ID)
	if len(d.Host.Network().ConnsToPeer(pi.ID)) > 0 {
		log.Printf("already connected to peer: %s", pi.ID)
		return
	}
	if err := d.Host.Connect(context.Background(), pi); err != nil {
		log.Printf("failed to connect to discovered peer %s: %s", pi.ID, err.Error())
	}
}

func (p2p *Libp2pTransporter) setupDiscovery() error {
	d := &Libp2pNodeDiscovery{Host: p2p.Host}
	discoveryService := mdns.NewMdnsService(p2p.Host, DISCOVERY_NAMESPACE, d)
	if discoveryService == nil {
		return fmt.Errorf("failed to build mDNS service")
	}
	if err := discoveryService.Start(); err != nil {
		return fmt.Errorf("failed to start mDNS discovery: %s", err.Error())
	}
	return nil
}

package main

import (
	"log"

	eventbus "github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	hardwaremonitor "github.com/cpmores/lucinda/pkg/infrastructure_layer/hardware_monitor"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	transport "github.com/cpmores/lucinda/pkg/infrastructure_layer/transport/transporters"
)

func main() {
	// ── Infrastructure ──────────────────────────────────────────────────────────
	eb := eventbus.NewInMemoryEventBus()
	mm := modulemanager.NewModuleManager()

	tp, err := transport.NewLibp2pTransport(transport.Libp2pTransportOptions{
		Addrs:      []string{"/ip4/127.0.0.1/tcp/0"},
		OutsLength: 20,
		InsLength:  100,
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}

	hm := hardwaremonitor.NewHardwareMonitor(eb, 5)

	// ── Infrastructure-Register ──────────────────────────────────────────────────────────

	tp.RegisterWithManager(mm)
	hm.RegisterWithManager(mm)
}

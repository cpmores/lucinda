package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/viper"

	eventbus "github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	hardwaremonitor "github.com/cpmores/lucinda/pkg/infrastructure_layer/hardware_monitor"
	logger "github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	provider "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider"
	transport "github.com/cpmores/lucinda/pkg/infrastructure_layer/transport/transporters"
)

func loadConfig() (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// Look in ./configs/server/ relative to working dir or binary.
	v.AddConfigPath(".")
	v.AddConfigPath("./configs/server")
	if execPath, err := os.Executable(); err == nil {
		v.AddConfigPath(filepath.Join(filepath.Dir(execPath), "configs", "server"))
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Defaults
	v.SetDefault("http.port", 9090)
	v.SetDefault("hardware_monitor.interval_sec", 5)
	v.SetDefault("transport.libp2p.outs_length", 20)
	v.SetDefault("transport.libp2p.ins_length", 100)

	return v, nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ── Config ────────────────────────────────────────────────────────────
	v, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// ── Phase 1: Infrastructure ─────────────────────────────────────────
	rootLogger, err := logger.New(logger.Options{
		Level:  "info",
		Format: "colored",
		Output: "stdout",
	})
	if err != nil {
		log.Fatalf("logger: %v", err)
	}

	mm := modulemanager.NewModuleManager()
	eb := eventbus.NewInMemoryEventBus(rootLogger.Child("Level-1-Eventbus"))

	tp, err := transport.NewLibp2pTransport(transport.Libp2pTransportOptions{
		Addrs:      v.GetStringSlice("transport.libp2p.addrs"),
		OutsLength: int64(v.GetInt("transport.libp2p.outs_length")),
		InsLength:  int64(v.GetInt("transport.libp2p.ins_length")),
		Logger:     rootLogger.Child("Level-1-Transport"),
	})
	if err != nil {
		log.Fatalf("transport: %v", err)
	}

	hm := hardwaremonitor.NewHardwareMonitor(1, rootLogger.Child("Level-1-HardwareMonitor"))
	pc := provider.NewProviderController(rootLogger.Child("Level-1-ProviderController"))

	if err := pc.LoadProviders(v); err != nil {
		log.Fatalf("load providers: %v", err)
	}

	rootLogger.RegisterWithManager(mm)
	eb.RegisterWithManager(mm)
	tp.RegisterWithManager(mm)
	hm.RegisterWithManager(mm)
	pc.RegisterWithManager(mm)

	// ── Phase 2: Task Management ────────────────────────────────────────

	// ── Start services ───────────────────────────────────────────────────

	if err := mm.VerifyInit(); err != nil {
		log.Fatalf("module manager: %v", err)
	}
	if err := mm.EnableDeps(); err != nil {
		log.Fatalf("module manager: %v", err)
	}

	if err := tp.Start(ctx); err != nil {
		log.Fatalf("transport: %v", err)
	}
	if err := hm.Start(ctx); err != nil {
		log.Fatalf("monitor: %v", err)
	}
	// ── HTTP Server ─────────────────────────────────────────────────────
	<-ctx.Done()
	hm.Stop()
	tp.Stop()
}

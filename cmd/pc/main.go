package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/viper"

	taskpostman "github.com/cpmores/lucinda/internal/task_management_layer/task_postman"
	taskstatemanager "github.com/cpmores/lucinda/internal/task_management_layer/task_state_manager"
	tasktracer "github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	taskboard "github.com/cpmores/lucinda/internal/task_workflow_layer/task_board"
	taskexecutor "github.com/cpmores/lucinda/internal/task_workflow_layer/task_executor"
	taskplanner "github.com/cpmores/lucinda/internal/task_workflow_layer/task_planner"
	taskreducer "github.com/cpmores/lucinda/internal/task_workflow_layer/task_reducer"
	userserver "github.com/cpmores/lucinda/internal/user_server"
	eventbus "github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	hardwaremonitor "github.com/cpmores/lucinda/pkg/infrastructure_layer/hardware_monitor"
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
	eb := eventbus.NewInMemoryEventBus()
	mm := modulemanager.NewModuleManager()

	tp, err := transport.NewLibp2pTransport(transport.Libp2pTransportOptions{
		Addrs:      v.GetStringSlice("transport.libp2p.addrs"),
		OutsLength: int64(v.GetInt("transport.libp2p.outs_length")),
		InsLength:  int64(v.GetInt("transport.libp2p.ins_length")),
	})
	if err != nil {
		log.Fatalf("transport: %v", err)
	}

	hm := hardwaremonitor.NewHardwareMonitor(eb, int64(v.GetInt("hardware_monitor.interval_sec")))
	pc := provider.NewProviderController()

	if err := pc.LoadProviders(v); err != nil {
		log.Fatalf("load providers: %v", err)
	}

	// ── Phase 2: Task Management ────────────────────────────────────────
	pm := taskpostman.NewTaskPostman(eb)
	tt := tasktracer.NewTaskTracer()
	sm := taskstatemanager.NewTaskStateManager(eb)

	tp.RegisterWithManager(mm)
	hm.RegisterWithManager(mm)
	pc.RegisterWithManager(mm)
	pm.RegisterWithManager(mm)
	tt.RegisterWithManager(mm)
	sm.RegisterWithManager(mm)

	tb := taskboard.NewTaskBoard(mm, sm)
	te := taskexecutor.NewTaskExecutor(mm, sm)
	planner := taskplanner.NewTaskPlanner(mm)
	reducer := taskreducer.NewTaskReducer(mm)

	tb.RegisterWithManager(mm)
	te.RegisterWithManager(mm)
	planner.RegisterWithManager(mm)
	reducer.RegisterWithManager(mm)

	// ── Start services ───────────────────────────────────────────────────
	if err := tp.Start(ctx); err != nil {
		log.Fatalf("transport: %v", err)
	}
	if err := hm.Start(ctx); err != nil {
		log.Fatalf("monitor: %v", err)
	}
	if err := tb.Start(ctx); err != nil {
		log.Fatalf("taskboard: %v", err)
	}
	if err := te.Start(ctx); err != nil {
		log.Fatalf("executor: %v", err)
	}
	if err := planner.Start(ctx); err != nil {
		log.Fatalf("planner: %v", err)
	}
	if err := reducer.Start(ctx); err != nil {
		log.Fatalf("reducer: %v", err)
	}

	// ── HTTP Server ─────────────────────────────────────────────────────
	httpPort := fmt.Sprintf(":%d", v.GetInt("http.port"))
	srv := userserver.NewHTTPServer(eb)
	go func() {
		if err := srv.Start(httpPort); err != nil {
			log.Printf("server: %v", err)
		}
	}()

	log.Printf("lucinda: all services started on %s", httpPort)

	<-ctx.Done()
	log.Println("lucinda: shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}

	te.Stop()
	reducer.Stop()
	planner.Stop()
	tb.Stop()
	hm.Stop()
	tp.Stop()
	pm.Stop()
}

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

	taskcommander "github.com/cpmores/lucinda/internal/task_workflow_layer/task_commander"
	taskexecutor "github.com/cpmores/lucinda/internal/task_workflow_layer/task_executor"
	taskmonitor "github.com/cpmores/lucinda/internal/task_workflow_layer/task_monitor"
	taskplanner "github.com/cpmores/lucinda/internal/task_workflow_layer/task_planner"
	telemetrybridge "github.com/cpmores/lucinda/internal/task_management_layer/telemetry_bridge"
	streamrouter "github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	taskpostman "github.com/cpmores/lucinda/internal/task_management_layer/postman"
	taskboard "github.com/cpmores/lucinda/internal/task_management_layer/task_board"
	tasktracer "github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	taskwrapper "github.com/cpmores/lucinda/internal/task_wrapper"
	userserver "github.com/cpmores/lucinda/internal/user_server"
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

	// ── Phase 2: Task Workflow ──────────────────────────────────────────
	planner := taskplanner.NewTaskPlanner(rootLogger.Child("Level-3-TaskPlanner"))
	commander := taskcommander.NewTaskCommander(rootLogger.Child("Level-3-TaskCommander"))
	executor := taskexecutor.NewTaskExecutor(rootLogger.Child("Level-3-TaskExecutor"))
	telemetryB := telemetrybridge.NewTelemetryBridge(rootLogger.Child("Level-2-TelemetryBridge"))
	streamR := streamrouter.NewStreamRouter(rootLogger.Child("Level-2-StreamRouter"))
	postman := taskpostman.NewTaskPostman(rootLogger.Child("Level-2-TaskPostman"))
	board := taskboard.NewTaskBoard(rootLogger.Child("Level-2-TaskBoard"))
	tracer := tasktracer.NewTaskTracer(rootLogger.Child("Level-2-TaskTracer"))
	monitor := taskmonitor.NewTaskMonitor(rootLogger.Child("Level-2-TaskMonitor"))

	planner.RegisterWithManager(mm)
	commander.RegisterWithManager(mm)
	executor.RegisterWithManager(mm)
	telemetryB.RegisterWithManager(mm)
	streamR.RegisterWithManager(mm)
	postman.RegisterWithManager(mm)
	board.RegisterWithManager(mm)
	tracer.RegisterWithManager(mm)
	monitor.RegisterWithManager(mm)

	// ── Init verification ────────────────────────────────────────────────
	if err := mm.VerifyInit(); err != nil {
		log.Fatalf("module manager: %v", err)
	}
	if err := mm.EnableDeps(); err != nil {
		log.Fatalf("module manager: %v", err)
	}

	// ── Start services ───────────────────────────────────────────────────
	if err := tp.Start(ctx); err != nil {
		log.Fatalf("transport: %v", err)
	}
	if err := hm.Start(ctx); err != nil {
		log.Fatalf("monitor: %v", err)
	}
	if err := streamR.Start(ctx); err != nil {
		log.Fatalf("stream router: %v", err)
	}
	if err := telemetryB.Start(ctx); err != nil {
		log.Fatalf("telemetry bridge: %v", err)
	}
	if err := postman.Start(ctx); err != nil {
		log.Fatalf("task postman: %v", err)
	}
	if err := board.Start(ctx); err != nil {
		log.Fatalf("task board: %v", err)
	}
	if err := monitor.Start(ctx); err != nil {
		log.Fatalf("task monitor: %v", err)
	}
	if err := planner.Start(ctx); err != nil {
		log.Fatalf("planner: %v", err)
	}
	if err := commander.Start(ctx); err != nil {
		log.Fatalf("commander: %v", err)
	}
	if err := executor.Start(ctx); err != nil {
		log.Fatalf("executor: %v", err)
	}

	// ── HTTP Server (wrapper needs the transport's NodeID, which is only
	// ── set once the transport is started). ───────────────────────────────
	wrapper := taskwrapper.New(eb, monitor, string(tp.ID()))
	httpPort := v.GetInt("http.port")
	srv := userserver.NewHTTPServer(wrapper, monitor, rootLogger.Child("Level-4-HTTPServer"))

	// ── HTTP Server ──────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%d", httpPort)
	go func() {
		if err := srv.Start(addr); err != nil && err != context.Canceled {
			log.Printf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	_ = srv.Shutdown(context.Background())
	hm.Stop()
	tp.Stop()
}

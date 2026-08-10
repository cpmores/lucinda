package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	eventbus "github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	hardwaremonitor "github.com/cpmores/lucinda/pkg/infrastructure_layer/hardware_monitor"
	logger "github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	provider "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider"
	transport "github.com/cpmores/lucinda/pkg/infrastructure_layer/transport/transporters"

	taskpostman "github.com/cpmores/lucinda/internal/task_management_layer/postman"
	streamrouter "github.com/cpmores/lucinda/internal/task_management_layer/stream_router"
	taskboard "github.com/cpmores/lucinda/internal/task_management_layer/task_board"
	tasktracer "github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	telemetrybridge "github.com/cpmores/lucinda/internal/task_management_layer/telemetry_bridge"
	taskcommander "github.com/cpmores/lucinda/internal/task_workflow_layer/task_commander"
	taskexecutor "github.com/cpmores/lucinda/internal/task_workflow_layer/task_executor"
	taskmonitor "github.com/cpmores/lucinda/internal/task_workflow_layer/task_monitor"
	taskplanner "github.com/cpmores/lucinda/internal/task_workflow_layer/task_planner"
	taskwrapper "github.com/cpmores/lucinda/internal/task_wrapper"
	userserver "github.com/cpmores/lucinda/internal/user_server"

	"github.com/cpmores/lucinda/internal/config"
)

func main() {
	cfgPath := flag.String("config", "", "path to the NodeConfig manifest (env: LUCINDA_CONFIG)")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ── Config (typed, K8s-style manifest) ───────────────────────────────
	cfg, err := config.Load(config.Path(*cfgPath))
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
		Addrs:      cfg.Spec.Transport.Libp2p.Addrs,
		OutsLength: cfg.Spec.Transport.Libp2p.OutsLength,
		InsLength:  cfg.Spec.Transport.Libp2p.InsLength,
		Logger:     rootLogger.Child("Level-1-Transport"),
	})
	if err != nil {
		log.Fatalf("transport: %v", err)
	}

	hm := hardwaremonitor.NewHardwareMonitor(cfg.Spec.HardwareMonitor.IntervalSec, rootLogger.Child("Level-1-HardwareMonitor"))
	pc := provider.NewProviderController(rootLogger.Child("Level-1-ProviderController"))

	if err := pc.LoadProviders(ctx, cfg.Spec.Providers); err != nil {
		log.Fatalf("load providers: %v", err)
	}

	_ = rootLogger.RegisterWithManager(mm)
	_ = eb.RegisterWithManager(mm)
	_ = tp.RegisterWithManager(mm)
	_ = hm.RegisterWithManager(mm)
	_ = pc.RegisterWithManager(mm)

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

	_ = planner.RegisterWithManager(mm)
	_ = commander.RegisterWithManager(mm)
	_ = executor.RegisterWithManager(mm)
	_ = telemetryB.RegisterWithManager(mm)
	_ = streamR.RegisterWithManager(mm)
	_ = postman.RegisterWithManager(mm)
	_ = board.RegisterWithManager(mm)
	_ = tracer.RegisterWithManager(mm)
	_ = monitor.RegisterWithManager(mm)

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
	srv := userserver.NewHTTPServer(wrapper, monitor, rootLogger.Child("Level-4-HTTPServer"))

	addr := fmt.Sprintf(":%d", cfg.Spec.HTTP.Port)
	go func() {
		if err := srv.Start(addr); err != nil && err != context.Canceled {
			log.Printf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	_ = srv.Shutdown(context.Background())
	_ = hm.Stop()
	_ = tp.Stop()
}

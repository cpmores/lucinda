package main

//
// import (
// 	"context"
// 	"log"
// 	"os"
// 	"os/signal"
// 	"syscall"
//
// 	eventbus "github.com/cpmores/lucinda/internal/eventBus"
// 	"github.com/cpmores/lucinda/internal/provider"
// 	server "github.com/cpmores/lucinda/internal/server"
// 	taskcontroller "github.com/cpmores/lucinda/internal/taskController"
// 	"github.com/cpmores/lucinda/internal/transport"
// 	"github.com/spf13/viper"
// 	"golang.org/x/sync/errgroup"
//
// 	// libp2p transport init
// 	_ "github.com/cpmores/lucinda/internal/transport/libp2p"
// )
//
// func setupEnvironment() error {
// 	viper.SetConfigName("config")
// 	viper.SetConfigType("yaml")
// 	viper.AddConfigPath("../../configs/server")
//
// 	log.Print("Loaded Config Successfully")
// 	return viper.ReadInConfig()
// }
//
// func main() {
// 	log.SetFlags(log.LstdFlags | log.Lshortfile)
// 	err := setupEnvironment()
// 	if err != nil {
// 		log.Printf("Environment setting up failed: %s", err.Error())
// 		return
// 	}
//
// 	// context
// 	baseCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
// 	defer cancel()
// 	g, gCtx := errgroup.WithContext(baseCtx)
//
// 	// setup eventbus
// 	eventBus := eventbus.NewDefaultEventBus(gCtx)
// 	// setup providerController and load providers
// 	_, err = provider.NewProviderController(gCtx, eventBus, viper.GetViper())
// 	if err != nil {
// 		log.Printf("Failed to create provider controller: %s", err.Error())
// 		return
// 	}
//
// 	// start postman
// 	g.Go(func() error {
// 		return transport.StartNodePostman(gCtx, eventBus, viper.GetViper())
// 	})
// 	// start task controller
// 	g.Go(func() error {
// 		return taskcontroller.StartTaskController(gCtx, eventBus, viper.GetViper())
// 	})
// 	// start server
// 	g.Go(func() error {
// 		return server.StartServer(gCtx, server.HTTP, eventBus, viper.GetViper())
// 	})
//
// 	log.Println("Lucinda node started successfully. Press Ctrl+C to stop.")
// 	if err := g.Wait(); err != nil {
// 		log.Printf("Lucinda node stopped with error: %s", err.Error())
// 		return
// 	}
//
// 	log.Println("Lucinda node shutdown cleanly.")
// }

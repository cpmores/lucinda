package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cpmores/lucinda/internel/provider"
	server "github.com/cpmores/lucinda/internel/server"
	"github.com/cpmores/lucinda/internel/transport"
	"github.com/spf13/viper"

	// libp2p transport init
	_ "github.com/cpmores/lucinda/internel/transport/libp2p"
)

func setupEnvironment() error {

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("../../configs/server")

	log.Print("Loaded Config Successfully")
	return viper.ReadInConfig()
}

func setupProviders() error {
	providerName, err := provider.CreateProvider("ollama")
	if err != nil {
		return err
	}

	log.Printf("Created provider %s successfully", providerName)
	return nil
}

func setupNodes() error {
	transport, err := transport.CreateTransporter("libp2p")
	if err != nil {
		return err
	}

	if err := transport.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start libp2p transporter: %s", err.Error())
	}

	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := setupEnvironment()
	if err != nil {
		log.Printf("Environment setting up failed: %s", err.Error())
		return
	}

	err = setupProviders()
	if err != nil {
		log.Printf("Providers setting up failed: %s", err.Error())
		return
	}

	err = setupNodes()
	if err != nil {
		log.Printf("Nodes setting up failed: %s", err.Error())
	}

	httpServer, err := server.CreateServer(server.HTTP, viper.GetViper())
	if err != nil {
		log.Fatal("Something wrong with HTTP server create")
	}
	log.Print("HTTPServer Created Successfully")

	log.Fatal(httpServer.Start())
}

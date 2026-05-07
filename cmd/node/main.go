package main

import (
	"log"

	"github.com/cpmores/lucinda/internel/provider"
	server "github.com/cpmores/lucinda/internel/server"
	"github.com/spf13/viper"
)

func setupEnvironment() error {

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("../../configs/server")

	log.Print("Loaded Config Successfully")
	return viper.ReadInConfig()
}

func setupProviders() error {
	providerName, err :=	provider.CreateProvider("ollama")
	if err != nil {
		return err
	}

	log.Printf("Created provider %s successfully", providerName)
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

	httpServer, err := server.CreateServer(server.HTTP, viper.GetViper())
	if err != nil {
		log.Fatal("Something wrong with HTTP server create")
	}
	log.Print("HTTPServer Created Successfully")

	log.Fatal(httpServer.Start())
}

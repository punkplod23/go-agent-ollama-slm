package main

import (
	"log"
	"punkplod23/go-agent-ollama-slm/config"
	"punkplod23/go-agent-ollama-slm/pkg/api"
)

func main() {

	cfg, err := config.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	api.StartServer(cfg)
}

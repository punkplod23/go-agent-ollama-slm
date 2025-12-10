package main

import (
	"fmt"
	"log"
	"os"
	"punkplod23/go-agent-ollama-slm/config"
	"punkplod23/go-agent-ollama-slm/internal/webui"
)

func main() {

	cfg, err := config.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	err, _ = webui.CreateMainChat(cfg, "What is the capital of France?")
	if err != nil {
		fmt.Println("Error in main chat flow:", err)
		os.Exit(1)
	}

}

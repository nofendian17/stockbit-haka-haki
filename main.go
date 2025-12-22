package main

import (
	"log"

	"stockbit-haka-haki/app"
	"stockbit-haka-haki/config"
)

func main() {
	// Load config from .env file
	cfg := config.LoadFromEnv()

	// Create and start app
	application := app.New(cfg)
	if err := application.Start(); err != nil {
		log.Fatal(err)
	}
}

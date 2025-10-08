package main

import (
	"log"

	"github.com/foomo/contentfulcommander/commanderclient"
)

var VERSION = "v0.2.0"

func main() {
	client, logger, err := commanderclient.Init(commanderclient.LoadConfigFromEnv())
	if err != nil {
		log.Fatalf("Failed to initialize migration client: %v", err)
	}
	logger.Info("Client initialized", "stats", client.GetStats())
}

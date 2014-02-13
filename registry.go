package main

import (
	"flag"
	"log"
	"registry/api"
	"registry/config"
)

func main() {
	var cfgFile string
	flag.StringVar(&cfgFile, "config", "/etc/go-docker-registry/config.json", "config file")
	flag.Parse()

	cfg, err := config.New(cfgFile)
	if err != nil {
		log.Fatalln(err)
	}

	registryAPI := api.New(cfg)
	log.Fatalln(registryAPI.Serve())
}

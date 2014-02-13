package main

import (
	"flag"
	"log"
	"registry/api"
	"registry/config"
	"registry/storage"
)

func main() {
	var cfgFile string
	flag.StringVar(&cfgFile, "config", "/etc/go-docker-registry/config.json", "config file")
	flag.Parse()

	cfg, err := config.New(cfgFile)
	if err != nil {
		log.Fatalln(err)
	}

	storage, err := storage.New(cfg.Storage)
	if err != nil {
		log.Fatalln(err)
	}

	registryAPI := api.New(cfg.API, storage)
	log.Fatalln(registryAPI.ListenAndServe())
}

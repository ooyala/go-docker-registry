package main

import (
	"flag"
	"registry/api"
	"registry/config"
	"registry/logger"
	"registry/storage"
)

func main() {
	var cfgFile string
	flag.StringVar(&cfgFile, "config", "/etc/go-docker-registry/config.json", "config file")
	flag.Parse()

	cfg, err := config.New(cfgFile)
	if err != nil {
		logger.Fatal(err.Error())
	}

	storage, err := storage.New(cfg.Storage)
	if err != nil {
		logger.Fatal(err.Error())
	}

	registryAPI := api.New(cfg.API, storage)
	logger.Fatal(registryAPI.ListenAndServe().Error())
}

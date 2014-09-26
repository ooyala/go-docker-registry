package config

import (
	"encoding/json"
	"registry/api"
	"registry/storage"
	"os"
)

type Config struct {
	API     *api.Config     `json:"api"`
	Storage *storage.Config `json:"storage"`
}

func New(filename string) (*Config, error) {
	// read in config
	var cfg Config
	if cfgFile, err := os.Open(filename); err != nil {
		return nil, err
	} else {
		dec := json.NewDecoder(cfgFile)
		if err := dec.Decode(&cfg); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

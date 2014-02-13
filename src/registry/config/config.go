package config

import (
	"encoding/json"
	"log"
	"os"
)

type Config struct {
	Addr           string `json:"addr"`
	DefaultHeaders map[string][]string `json:"default_headers"`
}

func New(filename string) (*Config, error) {
	// read in config
	var cfg Config
	if cfgFile, err := os.Open(filename); err == nil {
		dec := json.NewDecoder(cfgFile)
		if err := dec.Decode(&cfg); err != nil {
			return nil, err
		}
	} else {
		cfg = Config{Addr: ":5000", DefaultHeaders: map[string][]string{}}
		log.Println("Could not find config file. Using defaults:")
		log.Printf("  %#v", cfg)
	}
	return &cfg, nil
}

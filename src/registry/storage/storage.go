package storage

import (
	"errors"
	"io"
)

type Storage interface{
	init() error

	Get(string) ([]byte, error)
	Put(string, []byte) error
	GetReader(string) (io.ReadCloser, error)
	PutReader(string, io.Reader) error
	List(string) ([]string, error)
	Exists(string) (bool, error)
	Remove(string) error
	RemoveAll(string) error
}

type Config struct{
	Type  string
	Local *Local
	S3    *S3
}

func New(cfg *Config) (Storage, error) {
	switch cfg.Type {
	case "local":
		if cfg.Local != nil {
			return cfg.Local, cfg.Local.init()
		}
		return nil, errors.New("No config for storage type 'local' found")
	case "s3":
		if cfg.S3 != nil {
			return cfg.S3, cfg.S3.init()
		}
		return nil, errors.New("No config for storage type 's3' found")
	default:
		return nil, errors.New("Invalid storage type: " + cfg.Type)
	}
}

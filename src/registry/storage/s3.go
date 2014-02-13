package storage

import (
	"errors"
	"io"
)

type S3 struct {
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Root      string `json:"root"`
	BufferDir string `json:"buffer_dir"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

func (s *S3) init() error {
	return errors.New("Not Implemented")
}

func (s *S3) Get(path string) ([]byte, error) {
	return nil, errors.New("Not Implemented")
}

func (s *S3) Put(path string, data []byte) error {
	return errors.New("Not Implemented")
}

func (s *S3) GetReader(path string) (io.ReadCloser, error) {
	return nil, errors.New("Not Implemented")
}

func (s *S3) PutReader(path string, r io.Reader) error {
	return errors.New("Not Implemented")
}

func (s *S3) List(path string) ([]string, error) {
	return nil, errors.New("Not Implemented")
}

func (s *S3) Exists(path string) (bool, error) {
	return false, errors.New("Not Implemented")
}

func (s *S3) Remove(path string) error {
	return errors.New("Not Implemented")
}

func (s *S3) RemoveAll(path string) error {
	return errors.New("Not Implemented")
}

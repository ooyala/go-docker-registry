package storage

import (
	"errors"
	"io"
)

type Local struct {
	Root string `json:"root"`
}

func (s *Local) init() error {
	return errors.New("Not Implemented")
}

func (s *Local) Get(path string) ([]byte, error) {
	return nil, errors.New("Not Implemented")
}

func (s *Local) Put(path string, data []byte) error {
	return errors.New("Not Implemented")
}

func (s *Local) GetReader(path string) (io.ReadCloser, error) {
	return nil, errors.New("Not Implemented")
}

func (s *Local) PutReader(path string, r io.Reader) error {
	return errors.New("Not Implemented")
}

func (s *Local) List(path string) ([]string, error) {
	return nil, errors.New("Not Implemented")
}

func (s *Local) Exists(path string) (bool, error) {
	return false, errors.New("Not Implemented")
}

func (s *Local) Remove(path string) error {
	return errors.New("Not Implemented")
}

func (s *Local) RemoveAll(path string) error {
	return errors.New("Not Implemented")
}

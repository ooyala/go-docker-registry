package storage

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

type Local struct {
	Root string `json:"root"`
}

func (s *Local) init() error {
	return os.MkdirAll(s.Root, 0755)
}

func (s *Local) createFile(relpath string) (*os.File, error) {
	abspath := path.Join(s.Root, relpath)
	if err := os.MkdirAll(path.Dir(abspath), 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(abspath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
}

func (s *Local) Get(relpath string) ([]byte, error) {
	return ioutil.ReadFile(path.Join(s.Root, relpath))
}

func (s *Local) Put(relpath string, data []byte) error {
	file, err := s.createFile(relpath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func (s *Local) GetReader(relpath string) (io.ReadCloser, error) {
	return os.Open(path.Join(s.Root, relpath))
}

func (s *Local) PutReader(relpath string, r io.Reader, afterWrite func(io.Reader)) error {
	file, err := s.createFile(relpath)
	if err != nil {
		return err
	}
	defer func() {
		file.Seek(0, 0)
		afterWrite(file)
		file.Close()
	}()
	_, err = io.Copy(file, r)
	return err
}

func (s *Local) List(relpath string) ([]string, error) {
	abspath := path.Join(s.Root, relpath)
	infos, err := ioutil.ReadDir(abspath)
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		// to be consistent with S3, return no such file or directory here. from docker-registry 0.6.5
		return nil, errors.New("open " + abspath + ": no such file or directory")
	}
	list := make([]string, len(infos))
	for i, info := range infos {
		list[i] = path.Join(relpath, info.Name())
		if !strings.HasPrefix(list[i], "/") {
			list[i] = "/" + list[i]
		}
	}
	return list, nil
}

func (s *Local) Exists(relpath string) (bool, error) {
	info, err := os.Stat(path.Join(s.Root, relpath))
	if os.IsNotExist(err) {
		return false, nil
	}
	return info != nil, err
}

func (s *Local) Size(relpath string) (int64, error) {
	info, err := os.Stat(path.Join(s.Root, relpath))
	if info == nil || err != nil {
		// dunno size
		return -1, err
	}
	return info.Size(), nil
}

func (s *Local) Remove(relpath string) error {
	if ok, err := s.Exists(relpath); !ok || err != nil {
		return errors.New("no such file or directory: " + relpath)
	}
	abspath := path.Join(s.Root, relpath)
	err := os.Remove(abspath)
	if err != nil {
		return err
	}
	for absdir := path.Dir(abspath); s.removeIfEmpty(absdir); absdir = path.Dir(absdir) {
		// loop over parent directires and remove them if empty
		// we do this because that is how s3 looks since it is purely a key-value store
	}
	return nil
}

func (s *Local) RemoveAll(relpath string) error {
	if ok, err := s.Exists(relpath); !ok || err != nil {
		return errors.New("no such file or directory: " + relpath)
	}
	abspath := path.Join(s.Root, relpath)
	err := os.RemoveAll(abspath)
	if err != nil {
		return err
	}
	for absdir := path.Dir(abspath); s.removeIfEmpty(absdir); absdir = path.Dir(absdir) {
		// loop over parent directires and remove them if empty
		// we do this because that is how s3 looks since it is purely a key-value store
	}
	return nil
}

func (s *Local) removeIfEmpty(dir string) bool {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		// eh. something weird happened, don't do anything
		return false
	}
	if len(infos) != 0 {
		// not empty, don't delete
		return false
	}
	return os.Remove(dir) == nil
}

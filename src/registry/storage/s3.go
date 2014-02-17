package storage

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/crowdmob/goamz/aws"
	"github.com/crowdmob/goamz/s3"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

const S3_CONTENT_TYPE = "application/binary"

var S3_OPTIONS = s3.Options{}
var EMPTY_HEADERS = map[string][]string{}

type S3 struct {
	auth      aws.Auth
	authLock  sync.RWMutex // lock for the auth so we can update it when we need to
	region    aws.Region
	s3        *s3.S3
	bucket    *s3.Bucket
	bufferDir *BufferDir // used to buffer content if length is unknown
	root      string     // sanitized root (no leading slash)

	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Root      string `json:"root"`
	BufferDir string `json:"buffer_dir"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

func (s *S3) getAuth() (err error) {
	s.auth, err = aws.GetAuth(s.AccessKey, s.SecretKey, "", time.Time{})
	if s.s3 != nil {
		s.s3.Auth = s.auth
	}
	return
}

func (s *S3) updateAuth() {
	s.authLock.Lock()
	defer s.authLock.Unlock()
	err := s.getAuth()
	for ; err != nil; err = s.getAuth() {
		time.Sleep(5 * time.Second)
	}
}

func (s *S3) updateAuthLoop() {
	// this function just updates the auth. s.auth should be set before this is called
	// this is primarily used for role tagged ec2 instances who get an expiry with their auth.
	// if you set the access key and secret in environment variables, this will exit immediately.
	for {
		if s.auth.Expiration().IsZero() {
			// no reason to update, expiration is zero.
			return
		}
		if diff := s.auth.Expiration().Sub(time.Now()); diff < 0 {
			// if we're past the expiration time, update the auth
			s.updateAuth()
		} else {
			// if we're not past the expiration time, sleep until the expiration time is up
			time.Sleep(diff)
		}
	}
}

func (s *S3) init() error {
	if s.Bucket == "" {
		return errors.New("Please Specify an S3 Bucket")
	}
	if s.Region == "" {
		return errors.New("Please Specify an S3 Region")
	}
	if s.Root == "" {
		return errors.New("Please Specify an S3 Root Path")
	}
	if s.BufferDir == "" {
		return errors.New("Please Specify a Buffer Directory to use for Uploads")
	}

	var ok bool
	if s.region, ok = aws.Regions[s.Region]; !ok {
		return errors.New("Invalid Region: " + s.Region)
	}
	err := s.getAuth()
	if err != nil {
		return err
	}
	s.s3 = s3.New(s.auth, s.region)
	s.bucket = s.s3.Bucket(s.Bucket)
	if err := os.MkdirAll(s.BufferDir, 0755); err != nil && !os.IsExist(err) {
		// there was an error and it wasn't that the directory already exists
		return err
	}
	s.bufferDir = &BufferDir{Mutex: sync.Mutex{}, root: s.BufferDir}
	s.root = strings.TrimPrefix(s.Root, "/")
	go s.updateAuthLoop()
	return nil
}

func (s *S3) key(relpath string) string {
	return path.Join(s.root, relpath) // s3 expects no leading slash in some operations
}

func (s *S3) Get(relpath string) ([]byte, error) {
	s.authLock.RLock()
	defer s.authLock.RUnlock()
	return s.bucket.Get(s.key(relpath))
}

func (s *S3) Put(relpath string, data []byte) error {
	s.authLock.RLock()
	defer s.authLock.RUnlock()
	return s.bucket.Put(s.key(relpath), data, S3_CONTENT_TYPE, s3.Private, S3_OPTIONS)
}

func (s *S3) GetReader(relpath string) (io.ReadCloser, error) {
	s.authLock.RLock()
	defer s.authLock.RUnlock()
	return s.bucket.GetReader(s.key(relpath))
}

func (s *S3) PutReader(relpath string, r io.Reader, afterWrite func(*os.File)) error {
	key := s.key(relpath)
	buffer, err := s.bufferDir.reserve(key)
	if err != nil {
		return err
	}
	defer buffer.release(afterWrite)
	// don't know the length, buffer to file first
	length, err := io.Copy(buffer, r)
	if err != nil {
		return err
	}
	buffer.Seek(0, 0) // seek to the beginning of the file
	// we know the length, write to s3 from file now
	return s.bucket.PutReader(s.key(relpath), buffer, length, S3_CONTENT_TYPE, s3.Private, S3_OPTIONS)
}

func (s *S3) List(relpath string) ([]string, error) {
	s.authLock.RLock()
	defer s.authLock.RUnlock()
	result, err := s.bucket.List(s.key(relpath)+"/", "/", "", 0)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(result.Contents)+len(result.CommonPrefixes))
	for i, key := range result.Contents {
		names[i] = strings.TrimPrefix(key.Key, s.root)
		if !strings.HasPrefix(names[i], "/") {
			names[i] = "/" + names[i]
		}
	}
	for i, prefix := range result.CommonPrefixes {
		// trim trailing "/"
		names[i+len(result.Contents)] = strings.TrimPrefix(strings.TrimSuffix(prefix, "/"), s.root)
		if !strings.HasPrefix(names[i+len(result.Contents)], "/") {
			names[i+len(result.Contents)] = "/" + names[i+len(result.Contents)]
		}
	}
	if len(names) == 0 {
		// nothing there. return an error.
		return nil, errors.New("No keys exist in " + s.key(relpath))
	}
	return names, nil
}

func (s *S3) Exists(relpath string) (bool, error) {
	s.authLock.RLock()
	defer s.authLock.RUnlock()
	return s.bucket.Exists(s.key(relpath))
}

func (s *S3) Size(relpath string) (int64, error) {
	s.authLock.RLock()
	defer s.authLock.RUnlock()
	resp, err := s.bucket.Head(s.key(relpath), EMPTY_HEADERS)
	if err != nil {
		return -1, err
	}
	return resp.ContentLength, nil
}

func (s *S3) Remove(relpath string) error {
	s.authLock.RLock()
	defer s.authLock.RUnlock()
	if exists, err := s.bucket.Exists(s.key(relpath)); !exists || err != nil {
		return errors.New("no such file or directory: " + relpath)
	}
	return s.bucket.Del(s.key(relpath))
}

func (s *S3) RemoveAll(relpath string) error {
	// find and remove everything "under" it
	s.authLock.RLock()
	defer s.authLock.RUnlock()
	result, err := s.bucket.List(s.key(relpath)+"/", "", "", 0)
	if err != nil {
		return err
	}
	if len(result.Contents) == 0 {
		// nothing under it, return error
		return errors.New("no such file or directory " + relpath)
	}
	for _, key := range result.Contents {
		s.bucket.Del(key.Key)
	}
	// finally, remove it if needed
	return s.bucket.Del(s.key(relpath))
}

// This will ensure that we don't try to upload the same thing from two different requests at the same time
type BufferDir struct {
	sync.Mutex
	root string
}

func (b *BufferDir) reserve(key string) (*Buffer, error) {
	b.Lock()
	defer b.Unlock()
	// sha key path and create temporary file
	filepath := path.Join(b.root, fmt.Sprintf("%x", sha256.Sum256([]byte(key))))
	if _, err := os.Stat(filepath); !os.IsNotExist(err) {
		// buffer file already exists
		return nil, errors.New("Upload already in progress for key " + key)
	}
	// if not exist, create buffer file
	file, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}
	return &Buffer{File: *file, dir: b}, nil
}

type Buffer struct {
	os.File
	dir *BufferDir
}

func (b *Buffer) release(beforeRelease func(*os.File)) {
	b.dir.Lock()
	defer b.dir.Unlock()
	b.Seek(0, 0)
	beforeRelease(&b.File)
	b.Close()
	os.Remove(b.Name())
}

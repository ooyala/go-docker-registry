package storage

import (
	"errors"
	"fmt"
	"io"
	"path"
)

const TAG_PREFIX = "tag_"

type Storage interface {
	init() error

	Get(string) ([]byte, error)
	Put(string, []byte) error
	GetReader(string) (io.ReadCloser, error)
	PutReader(string, io.Reader, func(io.ReadSeeker)) error
	List(string) ([]string, error)
	Exists(string) (bool, error)
	Size(string) (int64, error)
	Remove(string) error
	RemoveAll(string) error
}

type Config struct {
	Type  string `json:"type"`
	Local *Local `json:"local"`
	S3    *S3    `json:"s3"`
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

func ImageJsonPath(id string) string {
	return fmt.Sprintf("images/%s/json", id)
}

func ImageMarkPath(id string) string {
	return fmt.Sprintf("images/%s/_inprogress", id)
}

func ImageChecksumPath(id string) string {
	return fmt.Sprintf("images/%s/_checksum", id)
}

func ImageLayerPath(id string) string {
	return fmt.Sprintf("images/%s/layer", id)
}

func ImageAncestryPath(id string) string {
	return fmt.Sprintf("images/%s/ancestry", id)
}

func ImageFilesPath(id string) string {
	return fmt.Sprintf("images/%s/_files", id)
}

func ImageDiffPath(id string) string {
	return fmt.Sprintf("images/%s/_diff", id)
}

func RepoImagesListPath(namespace, repo string) string {
	return fmt.Sprintf("repositories/%s/_images_list", path.Join(namespace, repo))
}

func RepoTagPath(namespace, repo, tag string) string {
	if tag == "" {
		return fmt.Sprintf("repositories/%s", path.Join(namespace, repo))
	}
	return fmt.Sprintf("repositories/%s/%s", path.Join(namespace, repo), TAG_PREFIX+tag)
}

func RepoJsonPath(namespace, repo string) string {
	return fmt.Sprintf("repositories/%s/json", path.Join(namespace, repo))
}

func RepoIndexImagesPath(namespace, repo string) string {
	return fmt.Sprintf("repositories/%s/_index_images", path.Join(namespace, repo))
}

func RepoPrivatePath(namespace, repo string) string {
	return fmt.Sprintf("repositories/%s/_private", path.Join(namespace, repo))
}

func RepoTagJsonPath(namespace, repo, tag string) string {
	tag = "tag" + tag + "_json"
	return fmt.Sprintf("repositories/%s", path.Join(namespace, repo, tag))
}

func RepoPath(namespace, repo string) string {
	return fmt.Sprintf("repositories/%s", path.Join(namespace, repo))
}
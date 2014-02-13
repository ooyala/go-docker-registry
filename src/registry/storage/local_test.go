package storage

import (
	"testing"
)

func TestLocal(t *testing.T) {
	testStorage(t, &Local{
		Root: "/tmp/go-docker-registry-test",
	})
}

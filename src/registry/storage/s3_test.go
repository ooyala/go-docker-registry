package storage

import (
	"os"
	"testing"
)

func TestS3(t *testing.T) {
	// read test config. has sensitive data so pass filename in as env variable
	var s3 S3
	err := storageFromFile(os.Getenv("TEST_S3_CONFIG"), &s3)
	if err != nil {
		t.Fatal(err)
	}
	testStorage(t, &s3)
}

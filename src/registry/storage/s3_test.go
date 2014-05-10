package storage

import (
	"os"
	"testing"
)

func TestS3(t *testing.T) {
	// read test config. has sensitive data so pass filename in as env variable
	if testing.Short() {
		t.Skip("skipping s3 tests in short mode.")
	}

	s3_cfg_file := os.Getenv("TEST_S3_CONFIG")
	if _, err := os.Stat(s3_cfg_file); os.IsNotExist(err) {
		skip_msg := "skipping s3 tests because the config file `%s` does not exist."
		t.Skip(skip_msg, s3_cfg_file)
	}

	var s3 S3
	err := storageFromFile(os.Getenv("TEST_S3_CONFIG"), &s3)
	if err != nil {
		t.Fatal(err)
	}
	testStorage(t, &s3)
}

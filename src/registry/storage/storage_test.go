package storage

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
)

func storageFromFile(filename string, storage Storage) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	if err := dec.Decode(storage); err != nil {
		return err
	}
	return storage.init()
}

func testStorage(t *testing.T, storage Storage) {
	// remove all
	if err := storage.RemoveAll("/"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.List("/"); err == nil {
		t.Fatal("According to docker 0.6.5, listing an empty directory should return an error")
	}

	testGetPutExistsRemove(t, storage)
	testGetPutReaders(t, storage)
	testListRemoveAll(t, storage)

	// cleanup
	if err := storage.RemoveAll("/"); err != nil {
		t.Fatal(err)
	}
}

func testGetPutExistsRemove(t *testing.T, storage Storage) {
	if exists, _ := storage.Exists("/1"); exists == true {
		t.Fatal("Key should not exist yet")
	}
	if _, err := storage.Get("/1"); err == nil {
		t.Fatal("Getting something that doesn't exist should cause an error")
	}
	if err := storage.Remove("/1"); err == nil {
		t.Fatal("Removing something that doesn't exist should cause an error")
	}
	if err := storage.Put("/1", []byte("lolwtf")); err != nil {
		t.Fatal(err)
	}
	if exists, _ := storage.Exists("/1"); exists == false {
		t.Fatal("Key should exist now")
	}
	if content, err := storage.Get("/1"); err != nil {
		t.Fatal(err)
	} else if string(content) != "lolwtf" {
		t.Log("the content should be 'lolwtf' was '"+string(content)+"'")
		t.FailNow()
	}
	if err := storage.Remove("/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.List("/"); err == nil {
		t.Fatal("According to docker 0.6.5, listing an empty directory should return an error")
	}
}

func testGetPutReaders(t *testing.T, storage Storage) {
	if exists, _ := storage.Exists("/dir/1"); exists == true {
		t.Fatal("Key should not exist yet")
	}
	if _, err := storage.GetReader("/dir/1"); err == nil {
		t.Fatal("Getting something that doesn't exist should cause an error")
	}
	if err := storage.Remove("/dir/1"); err == nil {
		t.Fatal("Removing something that doesn't exist should cause an error")
	}
	if err := storage.PutReader("/dir/1", bytes.NewBufferString("lolwtfdir")); err != nil {
		t.Fatal(err)
	}
	if exists, _ := storage.Exists("/dir/1"); exists == false {
		t.Fatal("Key should exist now")
	}
	if reader, err := storage.GetReader("/dir/1"); err != nil {
		t.Fatal(err)
	} else {
		content, err := ioutil.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "lolwtfdir" {
			t.Log("the content should be 'lolwtfdir' was '"+string(content)+"'")
			t.FailNow()
		}
	}
	if err := storage.Remove("/dir/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.List("/dir"); err == nil {
		t.Fatal("According to docker 0.6.5, listing an empty directory should return an error")
	}
	if _, err := storage.List("/"); err == nil {
		// this tests to make sure empty directories are removed (s3 behavior exists on all storages)
		t.Fatal("According to docker 0.6.5, listing an empty directory should return an error")
	}
}

func testListRemoveAll(t *testing.T, storage Storage) {
	if err := storage.Put("/dir/1", []byte("lolwtfdir1")); err != nil {
		t.Fatal(err)
	}
	if err := storage.Put("/dir/2", []byte("lolwtfdir2")); err != nil {
		t.Fatal(err)
	}
	if err := storage.Put("/dir/3", []byte("lolwtfdir3")); err != nil {
		t.Fatal(err)
	}
	if err := storage.Put("/anotherdir/1", []byte("lolwtfanotherdir1")); err != nil {
		t.Fatal(err)
	}
	if names, err := storage.List("/"); err != nil {
		t.Fatal(err)
	} else if len(names) != 2 {
		t.Fatal("There should be two names in the directory list")
	}
	if names, err := storage.List("/dir"); err != nil {
		t.Fatal(err)
	} else if len(names) != 3 {
		t.Fatal("There should be three names in the directory list")
	}
	if names, err := storage.List("/anotherdir/"); err != nil {
		t.Fatal(err)
	} else if len(names) != 1 {
		t.Fatal("There should be one name in the directory list")
	}
	if err := storage.RemoveAll("/dir"); err != nil {
		t.Fatal(err)
	}
	if names, err := storage.List("/"); err != nil {
		t.Fatal(err)
	} else if len(names) != 1 {
		t.Fatal("There should be one name in the directory list")
	}
	if _, err := storage.List("/dir"); err == nil {
		t.Fatal("According to docker 0.6.5, listing an empty directory should return an error")
	}
	if names, err := storage.List("/anotherdir"); err != nil {
		t.Fatal(err)
	} else if len(names) != 1 {
		t.Fatal("There should be one name in the directory list")
	}
	if err := storage.RemoveAll("/anotherdir"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.List("/"); err == nil {
		t.Fatal("According to docker 0.6.5, listing an empty directory should return an error")
	}
	if _, err := storage.List("/dir"); err == nil {
		t.Fatal("According to docker 0.6.5, listing an empty directory should return an error")
	}
	if _, err := storage.List("/anotherdir"); err == nil {
		t.Fatal("According to docker 0.6.5, listing an empty directory should return an error")
	}
}

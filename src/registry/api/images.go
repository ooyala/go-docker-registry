package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"io/ioutil"
	"net/http"
	"registry/layers"
	"registry/logger"
	"registry/storage"
	"strconv"
	"strings"
)

const COOKIE_SEPARATOR = "|"

func (a *RegistryAPI) RequireCompletion(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		imageID := vars["imageID"]
		if exists, _ := a.Storage.Exists(storage.ImageMarkPath(imageID)); exists {
			a.response(w, "Image is being uploaded, retry later", http.StatusBadRequest, EMPTY_HEADERS)
			return
		}
		handler(w, r)
	}
}

// Must be wrapped by: RequiresCompletion, CheckIfModifiedSince
// Sets: DefaultCacheHeaders
func (a *RegistryAPI) GetImageLayerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	reader, err := a.Storage.GetReader(storage.ImageLayerPath(imageID))
	if err != nil {
		// every "Image not found" response in this file.
		a.response(w, "Image not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, reader, http.StatusOK, headers)
}

func (a *RegistryAPI) PutImageLayerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	jsonContent, err := a.Storage.Get(storage.ImageJsonPath(imageID))
	if err != nil {
		a.response(w, "Image not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	layerPath := storage.ImageLayerPath(imageID)
	markPath := storage.ImageMarkPath(imageID)
	layerExists, _ := a.Storage.Exists(layerPath)
	markExists, _ := a.Storage.Exists(markPath)
	if layerExists && !markExists {
		a.response(w, "Image already exists", http.StatusConflict, EMPTY_HEADERS)
		return
	}
	// This next section reads the tarball from the body while computing various checksums. sha256Writer is used
	// to compute a checksum of the entire tarball using a TeeReader which will read from the body while
	// simultaneously writing what it read to sha256Writer. tarInfo will read the tar after it is put into the
	// storage and checksum each individual file within it (and checksum those checksums with the jsonContent)
	sha256Writer := sha256.New()
	sha256Writer.Write(jsonContent)
	teeReader := io.TeeReader(r.Body, sha256Writer)
	// this will create the checksums for a tar and the json for tar file info
	tarInfo := layers.NewTarInfo()
	// PutReader takes a function that will run after the write finishes:
	err = a.Storage.PutReader(layerPath, teeReader, tarInfo.Load)
	if err != nil {
		a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}

	checksums := []string{"sha256:" + hex.EncodeToString(sha256Writer.Sum(nil))}

	docker_version, err := layers.DockerVersion(r.Header["User-Agent"])
	if err != nil {
		a.response(w, err.Error(), http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	version_numbers := strings.Split(docker_version, ".")
	if version_numbers[0] < "1" {
		if minor, _ := strconv.Atoi(version_numbers[1]); minor < 10 {
			if tarInfo.Error == nil {
				filesJson, err := tarInfo.TarFilesInfo.Json()
				if err != nil {
					a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
					return
				}
				layers.SetImageFilesCache(a.Storage, imageID, filesJson)
			}
			// computing tarsum even if tarinfo.Error is nil as per python docker-registry
			tarsum := tarInfo.TarSum.Compute(jsonContent)
			checksums = append(checksums, tarsum)
		}
	}

	if err := layers.StoreChecksum(a.Storage, imageID, checksums); err != nil {
		a.response(w, "Error storing Checksum: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
}

// Must be wrapped by: RequiresCompletion, CheckIfModifiedSince
// Sets: DefaultCacheHeaders
func (a *RegistryAPI) GetImageJsonHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	data, err := a.Storage.Get(storage.ImageJsonPath(imageID))
	if err != nil {
		a.response(w, "Image not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	size, err := a.Storage.Size(storage.ImageLayerPath(imageID))
	if err != nil {
		a.response(w, "Unable to Compute Layer Size: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	headers["X-Docker-Size"] = []string{fmt.Sprintf("%d", size)}
	checksumPath := storage.ImageChecksumPath(imageID)
	if _, err := a.Storage.Exists(checksumPath); err != nil {
		a.response(w, "Checksum Not Found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}

	var parsed_checksum []string
	checksum, err := a.Storage.Get(checksumPath)
	if err != nil {
		a.response(w, "Error Reading Checksum: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	json.Unmarshal(checksum, &parsed_checksum)
	headers["X-Docker-Checksum-Payload"] = parsed_checksum
	// check and compute header checksum for docker < 0.10
	docker_version, err := layers.DockerVersion(r.Header["User-Agent"])
	if err != nil {
		a.response(w, err.Error(), http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	version_numbers := strings.Split(docker_version, ".")
	if version_numbers[0] < "1" {
		if minor, _ := strconv.Atoi(version_numbers[1]); minor < 10 {
			headers["X-Docker-Checksum"] = parsed_checksum
			delete(headers, "X-Docker-Checksum-Payload")
		}
	}
	a.response(w, data, http.StatusOK, headers)
}

func (a *RegistryAPI) PutImageJsonHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	// decode json from request body
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		a.response(w, "Error Reading Body: "+err.Error(), http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	var data map[string]interface{}
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		a.response(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	logger.Debug("[PutImageJson] body:\n%s", bodyBytes)
	if _, exists := data["id"]; !exists {
		a.response(w, "Missing key 'id' in JSON", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	dataID, ok := data["id"].(string)
	if !ok {
		a.response(w, "Invalid JSON: 'id' is not a string", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	if imageID != dataID {
		a.response(w, "JSON image id != image id specified in path", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	var parentID string
	if _, exists := data["parent"]; exists {
		parentID, ok = data["parent"].(string)
		if !ok {
			a.response(w, "Invalid JSON: 'parent' is not a string", http.StatusBadRequest, EMPTY_HEADERS)
			return
		}
		if exists, _ := a.Storage.Exists(storage.ImageJsonPath(parentID)); !exists {
			a.response(w, "Image depends on non-existant parent", http.StatusBadRequest, EMPTY_HEADERS)
			return
		}
	}
	jsonPath := storage.ImageJsonPath(imageID)
	markPath := storage.ImageMarkPath(imageID)
	if exists, _ := a.Storage.Exists(jsonPath); exists {
		if markExists, _ := a.Storage.Exists(markPath); !markExists {
			a.response(w, "Image already exists", http.StatusConflict, EMPTY_HEADERS)
			return
		}
	}
	err = a.Storage.Put(markPath, []byte("true"))
	if err != nil {
		a.response(w, "Put Mark Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	// We cleanup any old checksum in case it's a retry after a fail
	a.Storage.Remove(storage.ImageChecksumPath(imageID))
	err = a.Storage.Put(jsonPath, bodyBytes)
	if err != nil {
		a.response(w, "Put Json Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	if err := layers.GenerateAncestry(a.Storage, imageID, parentID); err != nil {
		a.response(w, "Generate Ancestry Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	a.response(w, "true", http.StatusOK, EMPTY_HEADERS)
}

// Must be wrapped by: RequiresCompletion, CheckIfModifiedSince
// Sets: DefaultCacheHeaders
func (a *RegistryAPI) GetImageAncestryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	data, err := a.Storage.Get(storage.ImageAncestryPath(imageID))
	if err != nil {
		a.response(w, "Image not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, data, http.StatusOK, headers)
}

func (a *RegistryAPI) PutImageChecksumHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]

	checksum := r.Header.Get("X-Docker-Checksum-Payload")
	// compute checksum for docker < 0.10
	docker_version, err := layers.DockerVersion(r.Header["User-Agent"])
	if err != nil {
		a.response(w, err.Error(), http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	version_numbers := strings.Split(docker_version, ".")
	if version_numbers[0] < "1" {
		if minor, _ := strconv.Atoi(version_numbers[1]); minor < 10 {
			checksum = r.Header.Get("X-Docker-Checksum")
		}
	}

	if checksum == "" {
		a.response(w, "Missing Image's checksum", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	// check if image json exists
	if exists, _ := a.Storage.Exists(storage.ImageJsonPath(imageID)); !exists {
		a.response(w, "Image not found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}

	markPath := storage.ImageMarkPath(imageID)
	if exists, _ := a.Storage.Exists(markPath); !exists {
		a.response(w, "Cannot set this image checksum (mark path does not exist)", http.StatusConflict, EMPTY_HEADERS)
		return
	}

	checksums := loadChecksums(a, imageID)
	if !stringInSlice(checksum, checksums) {
		logger.Debug("[PutImageLayer]["+imageID+"] Wrong checksum:"+string(checksum)+" not in %#v", checksums)
		a.response(w, "Checksum mismatch, ignoring the layer", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}

	if err := a.Storage.Remove(markPath); err != nil {
		a.response(w, "Error removing Mark Path: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
}

// Must be wrapped by: RequiresCompletion, CheckIfModifiedSince
// Sets: DefaultCacheHeaders
func (a *RegistryAPI) GetImageFilesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	data, err := layers.GetImageFilesJson(a.Storage, imageID)
	if err != nil {
		switch err.(type) {
		case layers.TarError:
			a.response(w, "Layer format not supported", http.StatusBadRequest, EMPTY_HEADERS)
			return
		default:
			a.response(w, "Image not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
			return
		}
	}
	a.response(w, data, http.StatusOK, headers)
}

// Must be wrapped by: RequiresCompletion, CheckIfModifiedSince
// Sets: DefaultCacheHeaders
func (a *RegistryAPI) GetImageDiffHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	diffJson, err := layers.GetImageDiffCache(a.Storage, imageID)
	if err != nil {
		// not cache miss. actual error
		a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	if diffJson == nil {
		// cache miss spawn goroutine to generate the diff and push it to S3
		go layers.GenDiff(a.Storage, imageID)
		diffJson = []byte{}
	}
	// copied from docker-registry. not sure why we would return StatusOK when the cache missed...
	a.response(w, diffJson, http.StatusOK, headers)
}

func loadChecksums(a *RegistryAPI, imageID string) []string {
	var data []string
	checksumPath := storage.ImageChecksumPath(imageID)
	if exists, _ := a.Storage.Exists(checksumPath); exists {
		content, _ := a.Storage.Get(checksumPath)
		json.Unmarshal(content, &data)
	}
	return data
}

func stringInSlice(element string, list []string) bool {
	for _, item := range list {
		if item == element {
			return true
		}
	}
	return false
}

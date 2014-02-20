package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"net/http"
	"registry/layers"
	"registry/logger"
	"registry/storage"
)

func (a *RegistryAPI) RequireCompletion(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		imageID := vars["imageID"]
		if exists, _ := a.Storage.Exists(storage.ImageMarkPath(imageID)); exists {
			a.response(w, "Image is being uploaded, retry later", 400, EMPTY_HEADERS)
			return
		}
		handler(w, r)
	}
}

// RequiresCompletion
// CheckIfModifiedSince
// Sets DefaultCacheHeaders
func (a *RegistryAPI) GetImageLayerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	reader, err := a.Storage.GetReader(storage.ImageLayerPath(imageID))
	if err != nil {
		a.response(w, "Image not found", 404, EMPTY_HEADERS)
		return
	}
	a.response(w, reader, 200, headers)
}

func (a *RegistryAPI) PutImageLayerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	jsonContent, err := a.Storage.Get(storage.ImageJsonPath(imageID))
	if err != nil {
		a.response(w, "Image not found", 404, EMPTY_HEADERS)
		return
	}
	layerPath := storage.ImageLayerPath(imageID)
	markPath := storage.ImageMarkPath(imageID)
	layerExists, _ := a.Storage.Exists(layerPath)
	markExists, _ := a.Storage.Exists(markPath)
	if layerExists && !markExists {
		a.response(w, "Image already exists", 409, EMPTY_HEADERS)
		return
	}
	// compute checksum while reading. create a TeeReader
	sha256Writer := sha256.New()
	sha256Writer.Write(append(jsonContent, byte('\n'))) // write initial json
	teeReader := io.TeeReader(r.Body, sha256Writer)
	// this will create the checksums for a tar and the json for tar file info
	tarInfo := layers.NewTarInfo(append(jsonContent, byte('\n')))
	// PutReader takes a function that will run after the write finishes:
	err = a.Storage.PutReader(layerPath, teeReader, tarInfo.Load)
	if err != nil {
		a.response(w, "Internal Error: "+err.Error(), 500, EMPTY_HEADERS)
		return
	}
	checksums := map[string]bool{fmt.Sprintf("sha256:%x", sha256Writer.Sum(nil)): true}
	if tarInfo.Error == nil {
		filesJson, err := tarInfo.TarFilesInfo.Json()
		if err != nil {
			a.response(w, "Internal Error: "+err.Error(), 500, EMPTY_HEADERS)
			return
		}
		layers.SetImageFilesCache(a.Storage, imageID, filesJson)
		checksums[tarInfo.TarSum.Compute()] = true
	}

	storedSum, err := a.Storage.Get(storage.ImageChecksumPath(imageID))
	if err != nil {
		csumBytes, err := json.Marshal(&checksums)
		if err != nil {
			a.response(w, "Internal Error: "+err.Error(), 500, EMPTY_HEADERS)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "checksum", Value: string(csumBytes)})
		a.response(w, true, 200, EMPTY_HEADERS)
		return
	}
	if !checksums[string(storedSum)] {
		logger.Debug("put_image_layer: Wrong checksum")
		a.response(w, "Checksum mismatch, ignoring the layer", 400, EMPTY_HEADERS)
		return
	}
	if err := a.Storage.Remove(markPath); err != nil {
		logger.Debug("put_image_layer: Error removing mark path: %s", err.Error())
		a.response(w, "Internal Error", 500, EMPTY_HEADERS)
		return
	}
	a.response(w, true, 200, EMPTY_HEADERS)
}

// RequiresCompletion
// CheckIfModifiedSince
// Sets DefaultCacheHeaders
func (a *RegistryAPI) GetImageJsonHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	data, err := a.Storage.Get(storage.ImageJsonPath(imageID))
	if err != nil {
		a.response(w, "Image not found", 404, EMPTY_HEADERS)
		return
	}
	size, err := a.Storage.Size(storage.ImageLayerPath(imageID))
	if err == nil {
		headers["X-Docker-Size"] = []string{fmt.Sprintf("%d", size)}
	}
	checksumPath := storage.ImageChecksumPath(imageID)
	if exists, _ := a.Storage.Exists(checksumPath); exists {
		checksum, err := a.Storage.Get(checksumPath)
		if err != nil {
			headers["X-Docker-Checksum"] = []string{string(checksum)}
		}
	}
	a.response(w, data, 200, headers)
}

func (a *RegistryAPI) PutImageJsonHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	// decode json from request body
	dec := json.NewDecoder(r.Body)
	var data map[string]interface{}
	err := dec.Decode(&data)
	if err != nil {
		a.response(w, "Invalid JSON", 400, EMPTY_HEADERS)
		return
	}
	if _, exists := data["id"]; !exists {
		a.response(w, "Missing key 'id' in JSON", 400, EMPTY_HEADERS)
		return
	}
	checksum := r.Header.Get("X-Docker-Checksum")
	if checksum == "" {
		// remove the old checksum in case it's a retry after a fail
		a.Storage.Remove(storage.ImageChecksumPath(imageID))
	} else if err := layers.StoreChecksum(a.Storage, imageID, checksum); err != nil {
		a.response(w, err.Error(), 400, EMPTY_HEADERS)
		return
	}
	dataID, ok := data["id"].(string)
	if !ok {
		a.response(w, "Invalid JSON: 'id' is not a string", 400, EMPTY_HEADERS)
		return
	}
	if imageID != dataID {
		a.response(w, "JSON data contains invalid id", 400, EMPTY_HEADERS)
		return
	}
	var parentID string
	if _, exists := data["parent"]; exists {
		parentID, ok = data["parent"].(string)
		if !ok {
			a.response(w, "Invalid JSON: 'parent' is not a string", 400, EMPTY_HEADERS)
			return
		}
		if exists, _ := a.Storage.Exists(storage.ImageJsonPath(parentID)); !exists {
			a.response(w, "Image depends on non-existant parent", 400, EMPTY_HEADERS)
			return
		}
	}
	jsonPath := storage.ImageJsonPath(imageID)
	markPath := storage.ImageMarkPath(imageID)
	if exists, _ := a.Storage.Exists(jsonPath); exists {
		if markExists, _ := a.Storage.Exists(markPath); !markExists {
			a.response(w, "Image already exists", 409, EMPTY_HEADERS)
			return
		}
	}
	err = a.Storage.Put(markPath, []byte("true"))
	if err != nil {
		a.response(w, "Internal Error: "+err.Error(), 500, EMPTY_HEADERS)
		return
	}
	jsonBytes, err := json.Marshal(&data)
	if err != nil {
		a.response(w, "Internal Error: "+err.Error(), 500, EMPTY_HEADERS)
		return
	}
	err = a.Storage.Put(jsonPath, jsonBytes)
	if err != nil {
		a.response(w, "Internal Error: "+err.Error(), 500, EMPTY_HEADERS)
		return
	}
	if err := layers.GenerateAncestry(a.Storage, imageID, parentID); err != nil {
		a.response(w, "Internal Error: "+err.Error(), 500, EMPTY_HEADERS)
		return
	}
	a.response(w, "true", 200, EMPTY_HEADERS)
}

// RequiresCompletion
// CheckIfModifiedSince
// Sets DefaultCacheHeaders
func (a *RegistryAPI) GetImageAncestryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	data, err := a.Storage.Get(storage.ImageAncestryPath(imageID))
	if err != nil {
		a.response(w, "Image not found", 404, EMPTY_HEADERS)
		return
	}
	a.response(w, data, 200, headers)
}

func (a *RegistryAPI) PutImageChecksumHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	// read header
	checksum := r.Header.Get("X-Docker-Checksum")
	if checksum == "" {
		a.response(w, "Missing Image's checksum", 400, EMPTY_HEADERS)
		return
	}
	// read cookie
	checksumCookie, err := r.Cookie("checksum")
	if err != nil {
		a.response(w, "Checksum not found in Cookie", 400, EMPTY_HEADERS)
		return
	}
	// check if image json exists
	if exists, _ := a.Storage.Exists(storage.ImageJsonPath(imageID)); !exists {
		a.response(w, "Image not found", 404, EMPTY_HEADERS)
		return
	}
	markPath := storage.ImageMarkPath(imageID)
	if exists, _ := a.Storage.Exists(markPath); !exists {
		a.response(w, "Cannot set this image checksum (mark path does not exist)", 409, EMPTY_HEADERS)
		return
	}
	err = layers.StoreChecksum(a.Storage, imageID, checksum)
	// extract checksumCookie JSON
	var checksumMap map[string]bool
	err = json.Unmarshal([]byte(checksumCookie.Value), &checksumMap)
	if err != nil {
		a.response(w, "Can't read checksum Cookie", 400, EMPTY_HEADERS)
		return
	}
	if !checksumMap[checksum] {
		logger.Debug("put_image_layer: Wrong checksum")
		a.response(w, "Checksum mismatch", 400, EMPTY_HEADERS)
		return
	}
	a.Storage.Remove(markPath)
	a.response(w, true, 200, EMPTY_HEADERS)
}

// RequiresCompletion
// CheckIfModifiedSince
// Sets DefaultCacheHeaders
func (a *RegistryAPI) GetImageFilesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	data, err := layers.GetImageFilesJson(a.Storage, imageID)
	if err != nil {
		switch err.(type) {
		case layers.TarError:
			a.response(w, "Layer format not supported", 400, EMPTY_HEADERS)
			return
		default:
			a.response(w, "Image not found", 404, EMPTY_HEADERS)
			return
		}
	}
	a.response(w, data, 200, headers)
}

// RequiresCompletion
// CheckIfModifiedSince
// Sets DefaultCacheHeaders
func (a *RegistryAPI) GetImageDiffHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	diffJson, err := layers.GetImageDiffCache(a.Storage, imageID)
	if err != nil {
		// not cache miss. actual error
		a.response(w, "Internal Error: "+err.Error(), 500, EMPTY_HEADERS)
		return
	}
	if diffJson == nil {
		// cache miss spawn goroutine to generate the diff and push it to S3
		go layers.GenDiff(a.Storage, imageID)
		diffJson = []byte{}
	}
	a.response(w, diffJson, 200, headers)
}

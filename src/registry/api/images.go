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
	"strings"
)

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

// RequiresCompletion
// CheckIfModifiedSince
// Sets DefaultCacheHeaders
func (a *RegistryAPI) GetImageLayerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	reader, err := a.Storage.GetReader(storage.ImageLayerPath(imageID))
	if err != nil {
		a.response(w, "Image not found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, reader, http.StatusOK, headers)
}

func (a *RegistryAPI) PutImageLayerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	jsonContent, err := a.Storage.Get(storage.ImageJsonPath(imageID))
	if err != nil {
		a.response(w, "Image not found", http.StatusNotFound, EMPTY_HEADERS)
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
	// compute checksum while reading. create a TeeReader
	sha256Writer := sha256.New()
	sha256Writer.Write(append(jsonContent, byte('\n'))) // write initial json
	teeReader := io.TeeReader(r.Body, sha256Writer)
	// this will create the checksums for a tar and the json for tar file info
	tarInfo := layers.NewTarInfo()
	// PutReader takes a function that will run after the write finishes:
	err = a.Storage.PutReader(layerPath, teeReader, tarInfo.Load)
	if err != nil {
		a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	checksums := map[string]bool{"sha256:" + hex.EncodeToString(sha256Writer.Sum(nil)): true}
	if tarInfo.Error == nil {
		filesJson, err := tarInfo.TarFilesInfo.Json()
		if err != nil {
			a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
			return
		}
		layers.SetImageFilesCache(a.Storage, imageID, filesJson)
		checksums[tarInfo.TarSum.Compute(append(jsonContent, byte('\n')))] = true
	}

	storedSum, err := a.Storage.Get(storage.ImageChecksumPath(imageID))
	if err != nil {
		cookieString := ""
		for sum, _ := range checksums {
			cookieString += sum + ","
		}
		cookieString = strings.TrimSuffix(cookieString, ",")
		http.SetCookie(w, &http.Cookie{Name: "checksum", Value: cookieString})
		a.response(w, true, http.StatusOK, EMPTY_HEADERS)
		return
	}
	if !checksums[string(storedSum)] {
		logger.Debug("[PutImageLayer]["+imageID+"] Wrong checksum:"+string(storedSum)+" not in %#v", checksums)
		a.response(w, "Checksum mismatch, ignoring the layer", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	if err := a.Storage.Remove(markPath); err != nil {
		logger.Debug("[PutImageLayer]["+imageID+"] Error removing mark path: %s", err.Error())
		a.response(w, "Internal Error", http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
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
		a.response(w, "Image not found", http.StatusNotFound, EMPTY_HEADERS)
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
	a.response(w, data, http.StatusOK, headers)
}

func (a *RegistryAPI) PutImageJsonHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	// decode json from request body
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		a.response(w, "Invalid Body: "+err.Error(), http.StatusBadRequest, EMPTY_HEADERS)
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
	checksum := r.Header.Get("X-Docker-Checksum")
	if checksum == "" {
		// remove the old checksum in case it's a retry after a fail
		a.Storage.Remove(storage.ImageChecksumPath(imageID))
	} else if err := layers.StoreChecksum(a.Storage, imageID, checksum); err != nil {
		a.response(w, err.Error(), http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	dataID, ok := data["id"].(string)
	if !ok {
		a.response(w, "Invalid JSON: 'id' is not a string", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	if imageID != dataID {
		a.response(w, "JSON data contains invalid id", http.StatusBadRequest, EMPTY_HEADERS)
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
			a.response(w, "Image already exists", 409, EMPTY_HEADERS)
			return
		}
	}
	err = a.Storage.Put(markPath, []byte("true"))
	if err != nil {
		a.response(w, "Put Mark Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
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

// RequiresCompletion
// CheckIfModifiedSince
// Sets DefaultCacheHeaders
func (a *RegistryAPI) GetImageAncestryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	headers := DefaultCacheHeaders()
	data, err := a.Storage.Get(storage.ImageAncestryPath(imageID))
	if err != nil {
		a.response(w, "Image not found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, data, http.StatusOK, headers)
}

func (a *RegistryAPI) PutImageChecksumHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageID := vars["imageID"]
	// read header
	checksum := r.Header.Get("X-Docker-Checksum")
	if checksum == "" {
		a.response(w, "Missing Image's checksum", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	// read cookie
	checksumCookie, err := r.Cookie("checksum")
	if err != nil {
		a.response(w, "Checksum not found in Cookie", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	// check if image json exists
	if exists, _ := a.Storage.Exists(storage.ImageJsonPath(imageID)); !exists {
		a.response(w, "Image not found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	markPath := storage.ImageMarkPath(imageID)
	if exists, _ := a.Storage.Exists(markPath); !exists {
		a.response(w, "Cannot set this image checksum (mark path does not exist)", 409, EMPTY_HEADERS)
		return
	}
	err = layers.StoreChecksum(a.Storage, imageID, checksum)
	// extract checksumCookie JSON
	checksumMap := map[string]bool{}
	for _, checksum := range strings.Split(checksumCookie.Value, ",") {
		checksumMap[checksum] = true
	}
	if !checksumMap[checksum] {
		logger.Debug("[PutImageChecksum]["+imageID+"] Wrong checksum:"+checksum+" not in %#v", checksumMap)
		a.response(w, "Checksum mismatch", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	a.Storage.Remove(markPath)
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
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
			a.response(w, "Layer format not supported", http.StatusBadRequest, EMPTY_HEADERS)
			return
		default:
			a.response(w, "Image not found", http.StatusNotFound, EMPTY_HEADERS)
			return
		}
	}
	a.response(w, data, http.StatusOK, headers)
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
		a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	if diffJson == nil {
		// cache miss spawn goroutine to generate the diff and push it to S3
		go layers.GenDiff(a.Storage, imageID)
		diffJson = []byte{}
	}
	a.response(w, diffJson, http.StatusOK, headers)
}

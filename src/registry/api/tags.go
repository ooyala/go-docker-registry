package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path"
	"registry/logger"
	"registry/storage"
	"strings"
	"time"
)

var EMPTY_REPO_JSON = map[string]interface{}{
	"last_update":       nil,
	"docker_version":    nil,
	"docker_go_version": nil,
	"arch":              "amd64",
	"os":                "linux",
	"kernel":            nil,
}

func (a *RegistryAPI) GetRepoTagsHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, _ := parseRepo(r, "")
	logger.Debug("[GetRepoTags] namespace=%s; repository=%s", namespace, repo)
	names, err := a.Storage.List(storage.RepoTagPath(namespace, repo, ""))
	if err != nil {
		// CR(edanaher): Again, are other errors possible?  Also for all other "not found" errors
		a.response(w, "Repository not found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	data := map[string]string{}
	for _, name := range names {
		base := path.Base(name)
		if !strings.HasPrefix(base, storage.TAG_PREFIX) {
			continue
		}
		// this is a tag
		tagName := strings.TrimPrefix(base, storage.TAG_PREFIX)
		content, err := a.Storage.Get(name)
		if err != nil {
			a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
			return
		}
		data[tagName] = string(content)
	}
	a.response(w, data, http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) DeleteRepoTagsHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, _ := parseRepo(r, "")
	logger.Debug("[DeleteRepoTags] namespace=%s; repository=%s", namespace, repo)
	if err := a.Storage.RemoveAll(storage.RepoTagPath(namespace, repo, "")); err != nil {
		a.response(w, "Repository not found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) GetRepoTagHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, tag := parseRepo(r, "tag")
	logger.Debug("[GetRepoTag] namespace=%s; repository=%s; tag=%s", namespace, repo, tag)
	content, err := a.Storage.Get(storage.RepoTagPath(namespace, repo, tag))
	if err != nil {
		a.response(w, "Tag not found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, content, http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) PutRepoTagHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, tag := parseRepo(r, "tag")
	logger.Debug("[PutRepoTag] namespace=%s; repository=%s; tag=%s", namespace, repo, tag)
	data, err := ioutil.ReadAll(r.Body)
	// CR(edanaher): It might be nice to separate "empty data" from "invalid data"
	if err != nil || len(data) == 0 {
		a.response(w, "Invalid data", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	logger.Debug("[PutRepoTag] body:\n%s", data)
	imageID := strings.Trim(string(data), "\"") // trim quotes
	if exists, err := a.Storage.Exists(storage.ImageJsonPath(imageID)); err != nil || !exists {
		a.response(w, "Image not found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	err = a.Storage.Put(storage.RepoTagPath(namespace, repo, tag), []byte(imageID))
	if err != nil {
		a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	if tag == "latest" {
		// write some metadata about the repos
		uaStrings := r.Header["User-Agent"]
		uaString := ""
		if len(uaStrings) > 0 {
			// just use the first one. there *should* only be one to begin with.
			uaString = uaStrings[0]
		}
		dataMap := CreateRepoJson(uaString)
		jsonData, err := json.Marshal(&dataMap)
		if err != nil {
			// CR(edanaher): Would it be worth having an a.internal_error(w, err) method that adds the "Internal
			// Error:" test and sets http.StatusInternalServerError?  There's lots of repetition here.
			a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
			return
		}
		a.Storage.Put(storage.RepoJsonPath(namespace, repo), jsonData)
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) DeleteRepoTagHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, tag := parseRepo(r, "tag")
	logger.Debug("[DeleteRepoTag] namespace=%s; repository=%s; tag=%s", namespace, repo, tag)
	if err := a.Storage.Remove(storage.RepoTagPath(namespace, repo, tag)); err != nil {
		a.response(w, "Tag not found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) GetRepoJsonHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, _ := parseRepo(r, "")
	logger.Debug("[GetRepoJson] namespace=%s; repository=%s", namespace, repo)
	content, err := a.Storage.Get(storage.RepoJsonPath(namespace, repo))
	if err != nil {
		// CR(edanaher): So if *anything* goes wrong fetching the json, we return an success and an empty repo.
		// What if S3 flakes out?
		a.response(w, EMPTY_REPO_JSON, http.StatusOK, EMPTY_HEADERS)
		return
	}
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		// CR(edanaher): Is this expected to happen?  My instinct is that this should be a 500...
		a.response(w, EMPTY_REPO_JSON, http.StatusOK, EMPTY_HEADERS)
		return
	}
	a.response(w, data, http.StatusOK, EMPTY_HEADERS)
	return
}

func CreateRepoJson(userAgent string) map[string]interface{} {
	props := map[string]interface{}{
		"last_update": time.Now().Unix(),
	}
	matches := USER_AGENT_REGEXP.FindAllStringSubmatch(userAgent, -1)
	uaMap := map[string]string{}
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		uaMap[match[1]] = match[2]
	}
	// CR(edanaher): I'm hoping these values are only used for troubleshooting.  If so, nice use of UA.  If
	// they're used for actual logic, that really bothers me.
	if val, exists := uaMap["docker_version"]; exists {
		props["docker_version"] = val
	}
	if val, exists := uaMap["go"]; exists {
		props["docker_go_version"] = val
	}
	if val, exists := uaMap["arch"]; exists {
		props["arch"] = strings.ToLower(val)
	}
	if val, exists := uaMap["kernel"]; exists {
		props["kernel"] = strings.ToLower(val)
	}
	if val, exists := uaMap["os"]; exists {
		props["os"] = strings.ToLower(val)
	}
	return props
}

func (a *RegistryAPI) DeleteRepoHandler(w http.ResponseWriter, r *http.Request) {
	//namespace, repo, _ := parseRepo(r, "")
	// CR(edanaher): Do we intended to implement this?  It seems like it might be useful, if only as a way to
	// purge incomplete uploads.
	NotImplementedHandler(w, r)
}

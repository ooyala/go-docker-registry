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
		a.response(w, "Repository not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
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
			a.internalError(w, err.Error())
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
		a.response(w, "Repository not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) GetRepoTagHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, tag := parseRepo(r, "tag")
	logger.Debug("[GetRepoTag] namespace=%s; repository=%s; tag=%s", namespace, repo, tag)
	content, err := a.Storage.Get(storage.RepoTagPath(namespace, repo, tag))
	if err != nil {
		a.response(w, "Tag not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, content, http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) PutRepoTagHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, tag := parseRepo(r, "tag")
	logger.Debug("[PutRepoTag] namespace=%s; repository=%s; tag=%s", namespace, repo, tag)
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		a.response(w, "Error reading request body: "+err.Error(), http.StatusBadRequest, EMPTY_HEADERS)
		return
	} else if len(data) == 0 {
		a.response(w, "Empty data", http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	logger.Debug("[PutRepoTag] body:\n%s", data)
	imageID := strings.Trim(string(data), "\"") // trim quotes
	if exists, err := a.Storage.Exists(storage.ImageJsonPath(imageID)); err != nil || !exists {
		a.response(w, "Image not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	err = a.Storage.Put(storage.RepoTagPath(namespace, repo, tag), []byte(imageID))
	if err != nil {
		a.internalError(w, err.Error())
		return
	}
	uaStrings := r.Header["User-Agent"]
	uaString := ""
	if len(uaStrings) > 0 {
		// just use the first one. there *should* only be one to begin with.
		uaString = uaStrings[0]
	}
	dataMap := CreateRepoJson(uaString)
	jsonData, err := json.Marshal(&dataMap)
	if err != nil {
		a.internalError(w, err.Error())
		return
	}
	a.Storage.Put(storage.RepoTagJsonPath(namespace, repo, tag), jsonData)
	if tag == "latest" {
		a.Storage.Put(storage.RepoJsonPath(namespace, repo), jsonData)
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) DeleteRepoTagHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, tag := parseRepo(r, "tag")
	logger.Debug("[DeleteRepoTag] namespace=%s; repository=%s; tag=%s", namespace, repo, tag)
	if err := a.Storage.Remove(storage.RepoTagPath(namespace, repo, tag)); err != nil {
		a.response(w, "Tag not found: "+err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) GetRepoJsonHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, _ := parseRepo(r, "")
	logger.Debug("[GetRepoJson] namespace=%s; repository=%s", namespace, repo)
	content, err := a.Storage.Get(storage.RepoJsonPath(namespace, repo))
	if err != nil {
		// docker-registry has this error ignored. so i guess we will too...
		a.response(w, EMPTY_REPO_JSON, http.StatusOK, EMPTY_HEADERS)
		return
	}
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		// docker-registry has this error ignored. so i guess we will too...
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
	if val, exists := uaMap["docker"]; exists {
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
	namespace, repo, _ := parseRepo(r, "")
	err := a.Storage.RemoveAll(storage.RepoPath(namespace,repo))
	if err != nil{
		a.response(w, err.Error(), http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, true, http.StatusOK, EMPTY_HEADERS)
	return
}

func (a *RegistryAPI) GetRepoTagsJsonHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, tag := parseRepo(r, "tag")
	data := map[string]string{
		"last_update":       "",
		"docker_version":    "",
		"docker_go_version": "",
		"arch":              "amd64",
		"os":                "linux",
		"kernel":            "",
	}
	content, err := a.Storage.Get(storage.RepoTagJsonPath(namespace, repo, tag))
	if err != nil {
		a.response(w, data, http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, content, http.StatusOK, EMPTY_HEADERS)
	return
}

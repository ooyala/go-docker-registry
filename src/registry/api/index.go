package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"registry/layers"
	"registry/logger"
	"registry/storage"
)

func IndexHeaders(r *http.Request, namespace, repo, access string) map[string][]string {
	fakeToken := []string{"Token signature=FAKESIGNATURE123,repository=\"" + namespace + "/" + repo + "\",access=" + access}
	return map[string][]string{
		"X-Docker-Endpoints": []string{r.Host},
		"WWW-Authenticate":   fakeToken,
		"X-Docker-Token":     fakeToken,
	}
}

func (a *RegistryAPI) putRepoImageHandler(w http.ResponseWriter, r *http.Request, successStatus int) {
	namespace, repo, _ := parseRepo(r, "")
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	logger.Debug("[PutRepoImage] body:\n%s", bodyBytes)
	var body []map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		a.response(w, "Error Decoding JSON: "+err.Error(), http.StatusBadRequest, EMPTY_HEADERS)
		return
	}
	if err := layers.UpdateIndexImages(a.Storage, namespace, repo, bodyBytes, body); err != nil {
		a.response(w, "Internal Error: "+err.Error(), http.StatusInternalServerError, EMPTY_HEADERS)
		return
	}
	a.response(w, "", successStatus, IndexHeaders(r, namespace, repo, "write"))
}

func (a *RegistryAPI) LoginHandler(w http.ResponseWriter, r *http.Request) {
	// Empty Shell
	a.response(w, "OK", http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	// Empty Shell
	a.response(w, "User Created (lies)", http.StatusCreated, EMPTY_HEADERS)
}

func (a *RegistryAPI) UpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	// Empty Shell
	a.response(w, "", http.StatusNoContent, EMPTY_HEADERS)
}

func (a *RegistryAPI) PutRepoHandler(w http.ResponseWriter, r *http.Request) {
	a.putRepoImageHandler(w, r, http.StatusOK)
}

func (a *RegistryAPI) PutRepoAuthHandler(w http.ResponseWriter, r *http.Request) {
	// Empty Shell
	a.response(w, "OK", http.StatusOK, EMPTY_HEADERS)
}

func (a *RegistryAPI) GetRepoImagesHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, _ := parseRepo(r, "")
	data, err := a.Storage.Get(storage.RepoIndexImagesPath(namespace, repo))
	if err != nil {
		a.response(w, "Image Not Found", http.StatusNotFound, EMPTY_HEADERS)
		return
	}
	a.response(w, data, http.StatusOK, IndexHeaders(r, namespace, repo, "read"))
}

func (a *RegistryAPI) PutRepoImagesHandler(w http.ResponseWriter, r *http.Request) {
	a.putRepoImageHandler(w, r, http.StatusNoContent)
}

func (a *RegistryAPI) DeleteRepoImagesHandler(w http.ResponseWriter, r *http.Request) {
	namespace, repo, _ := parseRepo(r, "")
	// from docker-registry 0.6.5: Does nothing, this file will be removed when DELETE on repos
	a.response(w, "", http.StatusNoContent, IndexHeaders(r, namespace, repo, "delete"))
}

func (a *RegistryAPI) SearchHandler(w http.ResponseWriter, r *http.Request) {
	a.response(w, map[string]string{}, http.StatusOK, EMPTY_HEADERS)
}

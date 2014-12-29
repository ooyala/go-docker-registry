package api

import (
	"encoding/json"
	"fmt"
	"github.com/cespare/go-apachelog"
	"github.com/gorilla/mux"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"registry/storage"
)

var USER_AGENT_REGEXP = regexp.MustCompile("([^\\s/]+)/([^\\s/]+)")
var EMPTY_HEADERS = map[string][]string{}

type Config struct {
	Addr           string              `json:"addr"`
	DefaultHeaders map[string][]string `json:"default_headers"`
}

type RegistryAPI struct {
	*Config
	Storage storage.Storage
}

func New(cfg *Config, storage storage.Storage) *RegistryAPI {
	return &RegistryAPI{Config: cfg, Storage: storage}
}

func (a *RegistryAPI) ListenAndServe() error {
	r := mux.NewRouter()
	r.HandleFunc("/", a.HomeHandler)

	//
	// Registry APIs (http://docs.docker.io/en/latest/reference/api/registry_api/)
	//

	// http://docs.docker.io/en/latest/reference/api/registry_api/#status
	// Documented and implemented in docker-registry 0.6.5
	r.HandleFunc("/_ping", a.PingHandler)
	r.HandleFunc("/v1/_ping", a.PingHandler)
	// Undocumented but implemented in docker-registry 0.6.5
	r.HandleFunc("/_status", a.StatusHandler)
	r.HandleFunc("/v1/_status", a.StatusHandler)

	// http://docs.docker.io/en/latest/reference/api/registry_api/#images
	// Documented and implemented in docker-registry 0.6.5
	r.HandleFunc("/v1/images/{imageID}/layer", a.RequireCompletion(a.CheckIfModifiedSince(a.GetImageLayerHandler))).Methods("GET")
	r.HandleFunc("/v1/images/{imageID}/layer", a.PutImageLayerHandler).Methods("PUT")
	r.HandleFunc("/v1/images/{imageID}/json", a.RequireCompletion(a.CheckIfModifiedSince(a.GetImageJsonHandler))).Methods("GET")
	r.HandleFunc("/v1/images/{imageID}/json", a.PutImageJsonHandler).Methods("PUT")
	r.HandleFunc("/v1/images/{imageID}/ancestry", a.RequireCompletion(a.CheckIfModifiedSince(a.GetImageAncestryHandler))).Methods("GET")
	// Undocumented but implemented in docker-registry 0.6.5
	r.HandleFunc("/v1/images/{imageID}/checksum", a.PutImageChecksumHandler).Methods("PUT")
	r.HandleFunc("/v1/images/{imageID}/files", a.RequireCompletion(a.CheckIfModifiedSince(a.GetImageFilesHandler))).Methods("GET")
	r.HandleFunc("/v1/images/{imageID}/diff", a.RequireCompletion(a.CheckIfModifiedSince(a.GetImageDiffHandler))).Methods("GET")

	// http://docs.docker.io/en/latest/reference/api/registry_api/#tags
	// Documented and implemented in docker-registry 0.6.5
	r.HandleFunc("/v1/repositories/{repo}/tags", a.GetRepoTagsHandler).Methods("GET")
	r.HandleFunc("/v1/repositories/{repo}/tags/{tag}", a.GetRepoTagHandler).Methods("GET")
	r.HandleFunc("/v1/repositories/{repo}/tags/{tag}", a.PutRepoTagHandler).Methods("PUT")
	r.HandleFunc("/v1/repositories/{repo}/tags/{tag}", a.DeleteRepoTagHandler).Methods("DELETE")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/tags", a.GetRepoTagsHandler).Methods("GET")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/tags/{tag}", a.GetRepoTagHandler).Methods("GET")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/tags/{tag}/json", a.GetRepoTagJsonHandler).Methods("GET")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/tags/{tag}", a.PutRepoTagHandler).Methods("PUT")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/tags/{tag}", a.DeleteRepoTagHandler).Methods("DELETE")
	// Undocumented but implemented in docker-registry 0.6.5
	r.HandleFunc("/v1/repositories/{repo}/tags", a.DeleteRepoTagsHandler).Methods("DELETE")
	r.HandleFunc("/v1/repositories/{repo}/json", a.GetRepoJsonHandler).Methods("GET")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/tags", a.DeleteRepoTagsHandler).Methods("DELETE")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/json", a.GetRepoJsonHandler).Methods("GET")
	// Documented and unimplemented in docker-registry 0.6.5
	r.HandleFunc("/v1/repositories/{repo}/", a.DeleteRepoHandler).Methods("DELETE")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/", a.DeleteRepoHandler).Methods("DELETE")
	// Undocumented and unimplemented (additional)
	r.HandleFunc("/v1/repositories/{repo}", a.DeleteRepoHandler).Methods("DELETE")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}", a.DeleteRepoHandler).Methods("DELETE")

	// Unused (for private images)
	//r.HandleFunc("/v1/private_images/{imageID}/layer", a.GetPrivateImageLayerHandler).Methods("GET")
	//r.HandleFunc("/v1/private_images/{imageID}/json", a.GetPrivateImageJsonHandler).Methods("GET")
	//r.HandleFunc("/v1/private_images/{imageID}/files", a.GetPrivateImageFilesHandler).Methods("GET")
	//r.HandleFunc("/v1/repositories/{repo}/properties", a.GetRepoPropertiesHandler).Methods("GET")
	//r.HandleFunc("/v1/repositories/{repo}/properties", a.PutRepoPropertiesHandler).Methods("PUT")
	//r.HandleFunc("/v1/repositories/{namespace}/{repo}/properties", a.GetRepoPropertiesHandler).Methods("GET")
	//r.HandleFunc("/v1/repositories/{namespace}/{repo}/properties", a.PutRepoPropertiesHandler).Methods("PUT")

	//
	// Index APIs (http://docs.docker.io/en/latest/reference/api/index_api/)
	//

	// http://docs.docker.io/en/latest/reference/api/index_api/#users
	// Documented and implemented in docker-registry 0.6.5
	r.HandleFunc("/v1/users", a.LoginHandler).Methods("GET")
	r.HandleFunc("/v1/users", a.CreateUserHandler).Methods("POST")
	r.HandleFunc("/v1/users/", a.LoginHandler).Methods("GET")
	r.HandleFunc("/v1/users/", a.CreateUserHandler).Methods("POST")
	r.HandleFunc("/v1/users/{username}/", a.UpdateUserHandler).Methods("PUT")

	// http://docs.docker.io/en/latest/reference/api/index_api/#repository
	// Documented and implemented in docker-registry 0.6.5
	r.HandleFunc("/v1/repositories/{repo}/", a.PutRepoHandler).Methods("PUT")
	r.HandleFunc("/v1/repositories/{repo}/images", a.GetRepoImagesHandler).Methods("GET")
	r.HandleFunc("/v1/repositories/{repo}/images", a.PutRepoImagesHandler).Methods("PUT")
	r.HandleFunc("/v1/repositories/{repo}/auth", a.PutRepoAuthHandler).Methods("PUT")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/", a.PutRepoHandler).Methods("PUT")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/images", a.GetRepoImagesHandler).Methods("GET")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/images", a.PutRepoImagesHandler).Methods("PUT")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/auth", a.PutRepoAuthHandler).Methods("PUT")
	// Undocumented but implemented in docker-registry 0.6.5
	r.HandleFunc("/v1/repositories/{repo}", a.PutRepoHandler).Methods("PUT")
	r.HandleFunc("/v1/repositories/{repo}/images", a.DeleteRepoImagesHandler).Methods("DELETE")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}", a.PutRepoHandler).Methods("PUT")
	r.HandleFunc("/v1/repositories/{namespace}/{repo}/images", a.DeleteRepoImagesHandler).Methods("DELETE")

	// http://docs.docker.io/en/latest/reference/api/index_api/#search
	// Documented and implemented in docker-registry 0.6.5
	r.HandleFunc("/v1/search", a.SearchHandler).Methods("GET")

	log.Printf("Listening on %s", a.Config.Addr)
	return http.ListenAndServe(a.Config.Addr, apachelog.NewHandler(r, os.Stderr))
}

func (a *RegistryAPI) response(w http.ResponseWriter, data interface{}, code int, headers map[string][]string) {
	for name, values := range a.Config.DefaultHeaders {
		w.Header()[name] = append(w.Header()[name], values...)
	}
	for name, values := range headers {
		w.Header()[name] = append(w.Header()[name], values...)
	}
	switch typedData := data.(type) {
	case nil:
		w.WriteHeader(code)
		w.Write([]byte{})
	case bool:
		w.WriteHeader(code)
		fmt.Fprintf(w, "%t", typedData)
	case int:
		w.WriteHeader(code)
		fmt.Fprintf(w, "%d", typedData)
	case string:
		w.WriteHeader(code)
		if code >= 400 {
			// if error, jsonify
			w.Write([]byte("{\"error\":\"" + typedData + "\"}"))
		} else {
			w.Write([]byte(typedData))
		}
	case []byte:
		// no need to wrap error here because if data comes in as a []byte it is meant to be raw data
		w.WriteHeader(code)
		w.Write(typedData)
	case io.Reader:
		w.WriteHeader(code)
		io.Copy(w, typedData)
	default:
		// write json
		if encoded, err := json.Marshal(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			w.WriteHeader(code)
			w.Write(encoded)
		}
	}
}

func (a *RegistryAPI) internalError(w http.ResponseWriter, text string) {
	a.response(w, "Internal Error: "+text, http.StatusInternalServerError, EMPTY_HEADERS)
}

func NotImplementedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

func parseRepo(r *http.Request, extra string) (string, string, string) {
	vars := mux.Vars(r)
	namespace := vars["namespace"]
	if vars["namespace"] == "" {
		namespace = "library"
	}
	return namespace, vars["repo"], vars[extra]
}

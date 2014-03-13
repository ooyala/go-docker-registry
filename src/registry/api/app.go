package api

import (
	"fmt"
	"net/http"
)

func (a *RegistryAPI) HomeHandler(w http.ResponseWriter, r *http.Request) {
	// CR(edanaher): It might be nice to have a more informative message.
	fmt.Fprintln(w, "go-docker-registry server")
}

func (a *RegistryAPI) PingHandler(w http.ResponseWriter, r *http.Request) {
	a.response(w, true, http.StatusOK, map[string][]string{"X-Docker-Registry-Standalone": []string{"true"}})
}

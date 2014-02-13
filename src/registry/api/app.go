package api

import (
	"fmt"
	"net/http"
)

func (a *RegistryAPI) HomeHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "go-docker-registry server")
}

func (a *RegistryAPI) PingHandler(w http.ResponseWriter, r *http.Request) {
	a.response(w, true, http.StatusOK, map[string][]string{"X-Docker-Registry-Standalone": []string{"true"}})
}

package api

import (
	"fmt"
	"net/http"
	"time"
)

func DefaultCacheHeaders() map[string][]string {
	expires := time.Now().UTC().Add(365 * 24 * time.Hour)
	return map[string][]string{
		"Cache-Control": []string{fmt.Sprintf("public, max-age=%d", int64(365*24*time.Hour.Seconds()))},
		"Expires": []string{expires.Format("Thu, 01 Jan 1970 00:00:00 GMT")},
		"Last-Modified": []string{"Thu, 01 Jan 1970 00:00:00 GMT"},
	}
}

func (a *RegistryAPI) CheckIfModifiedSince(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// check If-Modified-Since (if it exists, just send back a 304 because it will never change)
		if len(r.Header["If-Modified-Since"]) > 0 {
			a.response(w, true, 304, DefaultCacheHeaders())
			return
		}
		handler(w, r)
	}
}

// Package mount puts the version-free API under a versioned prefix.
//
// The API's code (songs, profile, site) answers paths under /api/ with no version in them:
// /api/charts/{id}, /api/game/catalog. Each version of the API (package apiv1, apiv2) mounts
// that code at /api/<version>/: a request for /api/v2/charts/1 reaches it as /api/charts/1, with
// the version kept in its context, so a link the code gives to its own routes names the version
// the client asked through (Link). The code itself never names a version; a version is removed
// by removing its package.
package mount

import (
	"context"
	"net/http"
	"strings"
)

type versionKey struct{}

// Mount serves the requests under /api/<version>/ with api, as /api/…; anything else is not
// found.
func Mount(version string, api http.Handler) http.Handler {
	prefix := "/api/" + version
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, prefix)
		if !ok || (rest != "" && rest[0] != '/') {
			http.NotFound(w, r)
			return
		}
		inner := r.Clone(context.WithValue(r.Context(), versionKey{}, version))
		inner.URL.Path = "/api" + rest
		inner.URL.RawPath = ""
		inner.RequestURI = inner.URL.RequestURI()
		api.ServeHTTP(w, inner)
	})
}

// Version is the version r came through ("v1"), or "" when it was not mounted (tests of the
// version-free code).
func Version(r *http.Request) string {
	v, _ := r.Context().Value(versionKey{}).(string)
	return v
}

// Link is path, a version-free API path (/api/charts/1/audio), as the client that sent r reaches
// it: /api/v1/charts/1/audio through version v1.
func Link(r *http.Request, path string) string {
	rest, ok := strings.CutPrefix(path, "/api")
	if v := Version(r); ok && v != "" && (rest == "" || rest[0] == '/') {
		return "/api/" + v + rest
	}
	return path
}

// Serve sends a request under /api/<version>/ to that version's handler and any other /api/
// path to not found; the rest (health checks, /internal/, pages) goes to other.
func Serve(versions map[string]http.Handler, other http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, "/api/")
		if !ok {
			other.ServeHTTP(w, r)
			return
		}
		version, _, _ := strings.Cut(rest, "/")
		if h, ok := versions[version]; ok {
			h.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":"NOT_FOUND","message":"No such API version"}` + "\n"))
	})
}

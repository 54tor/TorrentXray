package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"mkvcheck/internal/abuse"
	"mkvcheck/internal/store"
)

const (
	contentSecurityPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'"
	permissionsPolicy     = "camera=(), microphone=(), geolocation=()"
	vendorCacheControl    = "public, max-age=31536000, immutable"
	staticCacheControl    = "public, max-age=300"
	vendorPrefix          = "vendor/"
	wasmExtension         = ".wasm"
	wasmContentType       = "application/wasm"
	indexFile             = "index.html"
)

type handlers struct {
	reports *store.Store
	guard   *abuse.Guard
	static  fs.FS
}

func New(_ Config, reports *store.Store, guard *abuse.Guard, static fs.FS) http.Handler {
	serving := handlers{reports: reports, guard: guard, static: static}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", serving.index)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.HandlerFunc(serving.staticFile)))
	mux.HandleFunc("GET /api/challenge", serving.challenge)
	mux.HandleFunc("POST /api/reports", serving.createReport)
	mux.HandleFunc("GET /r/{id}", serving.showReport)
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	return withSecurityHeaders(mux)
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := writer.Header()
		header.Set("Content-Security-Policy", contentSecurityPolicy)
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Permissions-Policy", permissionsPolicy)
		header.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(writer, request)
	})
}

func (h handlers) index(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", staticCacheControl)
	http.ServeFileFS(writer, request, h.static, indexFile)
}

func (h handlers) staticFile(writer http.ResponseWriter, request *http.Request) {
	name := path.Clean(request.URL.Path)
	if strings.HasPrefix(name+"/", vendorPrefix) {
		writer.Header().Set("Cache-Control", vendorCacheControl)
	} else {
		writer.Header().Set("Cache-Control", staticCacheControl)
	}
	if path.Ext(name) == wasmExtension {
		writer.Header().Set("Content-Type", wasmContentType)
	}
	http.FileServerFS(h.static).ServeHTTP(writer, request)
}

func (h handlers) challenge(writer http.ResponseWriter, request *http.Request) {
	if !h.guard.AllowRequest(request) {
		writeError(writer, http.StatusTooManyRequests, "rate_limited")
		return
	}
	h.guard.ChallengeHandler().ServeHTTP(writer, request)
}

func writeError(writer http.ResponseWriter, status int, code string) {
	writeJSON(writer, status, map[string]string{"error": code})
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

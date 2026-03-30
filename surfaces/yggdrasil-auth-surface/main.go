package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}
	coreBaseURL := os.Getenv("YGGDRASIL_CORE_HTTP_URL")
	if coreBaseURL == "" {
		coreBaseURL = "http://yggdrasil-core:9080"
	}
	targetURL, err := url.Parse(coreBaseURL)
	if err != nil {
		log.Fatalf("invalid YGGDRASIL_CORE_HTTP_URL: %v", err)
	}

	authProxy := httputil.NewSingleHostReverseProxy(targetURL)
	originalDirector := authProxy.Director
	authProxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Yggdrasil-Surface", "yggdrasil-auth-surface")
	}
	authProxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("auth proxy error: %v", err)
		http.Error(w, "upstream auth is unavailable", http.StatusBadGateway)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"surface":       "yggdrasil-auth-surface",
			"status":        "ok",
			"core_base_url": coreBaseURL,
			"auth_endpoints": []string{
				"/api/v1/auth/login",
				"/api/v1/auth/session",
				"/api/v1/auth/logout",
			},
		})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/api/v1/auth/login", authProxy)
	mux.Handle("/api/v1/auth/session", authProxy)
	mux.Handle("/api/v1/auth/logout", authProxy)

	addr := ":" + port
	log.Printf("yggdrasil-auth-surface listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

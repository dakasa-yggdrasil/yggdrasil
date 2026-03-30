package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "__SURFACE_PORT__"
	}
	coreBaseURL := os.Getenv("YGGDRASIL_CORE_HTTP_URL")
	if coreBaseURL == "" {
		coreBaseURL = "http://yggdrasil-core:9080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"surface":"__SURFACE_NAME__","status":"ok","core_base_url":"%s"}`, coreBaseURL)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := ":" + port
	log.Printf("__SURFACE_NAME__ listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

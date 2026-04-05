package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Hits: %d", cfg.fileserverHits.Load())
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// readinessHandler handles GET requests to the /healthz endpoint
func readinessHandler(w http.ResponseWriter, r *http.Request) {
	// Write the Content-Type header
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	// Write the status code
	w.WriteHeader(http.StatusOK)

	// Write the response body
	w.Write([]byte("OK"))
}

// makeHandler creates and configures the HTTP handler for the Chirpy server
func makeHandler() http.Handler {
	apiCfg := &apiConfig{}

	// Create a new http.ServeMux
	mux := http.NewServeMux()

	// Register the readiness endpoint
	mux.HandleFunc("/healthz", readinessHandler)

	// Register the metrics endpoint
	mux.HandleFunc("/metrics", apiCfg.metricsHandler)

	// Register the reset endpoint
	mux.HandleFunc("/reset", apiCfg.resetHandler)

	// Register a file server handler for the assets directory
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	// Register a file server handler for the /app path, wrapped with metrics middleware
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))

	return mux
}

func main() {
	// Create a new http.Server struct
	server := &http.Server{
		Handler: makeHandler(),
		Addr:    ":8080",
	}

	// Start the server using ListenAndServe
	server.ListenAndServe()
}

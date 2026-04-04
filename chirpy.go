package main

import (
	"net/http"
)

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
	// Create a new http.ServeMux
	mux := http.NewServeMux()

	// Register the readiness endpoint
	mux.HandleFunc("/healthz", readinessHandler)

	// Register a file server handler for the assets directory
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	// Register a file server handler for the /app path
	mux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("."))))

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

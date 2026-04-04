package main

import (
	"net/http"
)

// makeHandler creates and configures the HTTP handler for the Chirpy server
func makeHandler() http.Handler {
	// Create a new http.ServeMux
	mux := http.NewServeMux()

	// Register a file server handler for the root path
	mux.Handle("/", http.FileServer(http.Dir(".")))

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

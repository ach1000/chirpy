package main

import (
	"net/http"
)

func main() {
	// Create a new http.ServeMux
	mux := http.NewServeMux()

	// Create a new http.Server struct
	server := &http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	// Start the server using ListenAndServe
	server.ListenAndServe()
}

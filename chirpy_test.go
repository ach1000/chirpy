package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeIndexHTML(t *testing.T) {
	// Create a test server using the actual handler from chirpy.go
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	// Make a request to the root path
	resp, err := http.Get(server.URL + "/index.html")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check that the status code is 200 OK
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read the response body
	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("Failed to read response body: %v", err)
	}
	body := string(buf[:n])

	// Check that the response contains expected content from index.html
	if !strings.Contains(body, "Welcome to Chirpy") {
		t.Errorf("Response does not contain expected content. Got: %s", body)
	}
}

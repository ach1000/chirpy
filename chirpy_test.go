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

	// Make a request to the /app path
	resp, err := http.Get(server.URL + "/app/index.html")
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

func TestServeLogo(t *testing.T) {
	// Create a test server using the actual handler from chirpy.go
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	// Make a request to the logo asset
	resp, err := http.Get(server.URL + "/assets/logo.png")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check that the status code is 200 OK
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check that the Content-Type is correct for PNG
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "image") {
		t.Errorf("Expected image Content-Type, got %s", contentType)
	}

	// Read the response body to verify it has content
	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Check that the response body is not empty (image file has content)
	if n == 0 {
		t.Errorf("Response body is empty, expected image file content")
	}
}

// noRedirectClient returns an HTTP client that does not follow redirects,
// preventing the FileServer's index.html redirect from inflating hit counts.
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestMetricsEndpoint(t *testing.T) {
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	client := noRedirectClient()

	// Hit /app/ to increment the counter
	client.Get(server.URL + "/app/index.html")
	client.Get(server.URL + "/app/index.html")

	resp, err := client.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if body != "Hits: 2" {
		t.Errorf("Expected body 'Hits: 2', got '%s'", body)
	}
}

func TestResetEndpoint(t *testing.T) {
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	client := noRedirectClient()

	// Hit /app/ to increment the counter, then reset
	client.Get(server.URL + "/app/index.html")
	client.Get(server.URL + "/reset")

	resp, err := client.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if body != "Hits: 0" {
		t.Errorf("Expected body 'Hits: 0' after reset, got '%s'", body)
	}
}

func TestReadinessEndpoint(t *testing.T) {
	// Create a test server using the actual handler from chirpy.go
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	// Make a request to the readiness endpoint
	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check that the status code is 200 OK
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check that the Content-Type header is correct
	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Expected Content-Type 'text/plain; charset=utf-8', got '%s'", contentType)
	}

	// Read the response body
	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("Failed to read response body: %v", err)
	}
	body := string(buf[:n])

	// Check that the response body contains "OK"
	if body != "OK" {
		t.Errorf("Expected body 'OK', got '%s'", body)
	}
}

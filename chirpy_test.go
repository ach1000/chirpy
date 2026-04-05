package main

import (
	"io"
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

func TestMetricsEndpoint(t *testing.T) {
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	// Hit /app/ twice to increment the counter
	for i := 0; i < 2; i++ {
		resp, err := http.Get(server.URL + "/app/")
		if err != nil {
			t.Fatalf("Failed to make request to /app/: %v", err)
		}
		resp.Body.Close()
	}

	// Check /metrics reports the correct count
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("Failed to make request to /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Expected Content-Type 'text/plain; charset=utf-8', got '%s'", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != "Hits: 2" {
		t.Errorf("Expected body 'Hits: 2', got '%s'", string(body))
	}
}

func TestResetEndpoint(t *testing.T) {
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	// Increment counter by hitting /app/
	resp, err := http.Get(server.URL + "/app/")
	if err != nil {
		t.Fatalf("Failed to make request to /app/: %v", err)
	}
	resp.Body.Close()

	// Reset the counter
	resp, err = http.Get(server.URL + "/reset")
	if err != nil {
		t.Fatalf("Failed to make request to /reset: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 from /reset, got %d", resp.StatusCode)
	}

	// Verify counter is back to 0
	resp, err = http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("Failed to make request to /metrics: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != "Hits: 0" {
		t.Errorf("Expected body 'Hits: 0' after reset, got '%s'", string(body))
	}
}

func TestMiddlewareMetricsInc(t *testing.T) {
	cfg := &apiConfig{}

	// Create a simple handler that always returns 200
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := cfg.middlewareMetricsInc(next)

	// Fire three requests through the middleware
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/app/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}

	if got := cfg.fileserverHits.Load(); got != 3 {
		t.Errorf("Expected fileserverHits to be 3, got %d", got)
	}
}

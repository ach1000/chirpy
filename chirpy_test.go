package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeIndexHTML(t *testing.T) {
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/app/index.html")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if !strings.Contains(string(body), "Welcome to Chirpy") {
		t.Errorf("Response does not contain expected content. Got: %s", body)
	}
}

func TestReadinessEndpoint(t *testing.T) {
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/healthz")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
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

	if string(body) != "OK" {
		t.Errorf("Expected body 'OK', got '%s'", string(body))
	}
}

func TestMetricsEndpoint(t *testing.T) {
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	for i := 0; i < 2; i++ {
		resp, err := http.Get(server.URL + "/app/")
		if err != nil {
			t.Fatalf("Failed to make request to /app/: %v", err)
		}
		resp.Body.Close()
	}

	resp, err := http.Get(server.URL + "/api/metrics")
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

	resp, err := http.Get(server.URL + "/app/")
	if err != nil {
		t.Fatalf("Failed to make request to /app/: %v", err)
	}
	resp.Body.Close()

	resp, err = http.Post(server.URL+"/api/reset", "", nil)
	if err != nil {
		t.Fatalf("Failed to make request to /reset: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 from /reset, got %d", resp.StatusCode)
	}

	resp, err = http.Get(server.URL + "/api/metrics")
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

func TestMethodNotAllowed(t *testing.T) {
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/healthz"},
		{http.MethodPost, "/api/metrics"},
		{http.MethodGet, "/api/reset"},
	}

	for _, tc := range tests {
		req, err := http.NewRequest(tc.method, server.URL+tc.path, nil)
		if err != nil {
			t.Fatalf("%s %s: failed to create request: %v", tc.method, tc.path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: failed to make request: %v", tc.method, tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: expected 405, got %d", tc.method, tc.path, resp.StatusCode)
		}
	}
}

func TestMiddlewareMetricsInc(t *testing.T) {
	cfg := &apiConfig{}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := cfg.middlewareMetricsInc(next)

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

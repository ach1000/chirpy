package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/ach1000/chirpy/internal/database"
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

	resp, err := http.Get(server.URL + "/admin/metrics")
	if err != nil {
		t.Fatalf("Failed to make request to /admin/metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type 'text/html; charset=utf-8', got '%s'", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if !strings.Contains(string(body), "Chirpy has been visited 2 times!") {
		t.Errorf("Expected body to contain visit count 2, got: %s", body)
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

	resp, err = http.Post(server.URL+"/admin/reset", "", nil)
	if err != nil {
		t.Fatalf("Failed to make request to /admin/reset: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 from /admin/reset, got %d", resp.StatusCode)
	}

	resp, err = http.Get(server.URL + "/admin/metrics")
	if err != nil {
		t.Fatalf("Failed to make request to /admin/metrics: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if !strings.Contains(string(body), "Chirpy has been visited 0 times!") {
		t.Errorf("Expected body to contain visit count 0 after reset, got: %s", body)
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
		{http.MethodPost, "/admin/metrics"},
		{http.MethodGet, "/admin/reset"},
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

func TestValidateChirpEndpoint(t *testing.T) {
	server := httptest.NewServer(makeHandler())
	defer server.Close()

	tests := []struct {
		name         string
		body         string
		expectedCode int
		expectedKey  string
		expectedBody string
	}{
		{
			name:         "valid chirp",
			body:         `{"body":"This is an opinion I need to share with the world"}`,
			expectedCode: http.StatusOK,
			expectedKey:  "cleaned_body",
			expectedBody: "This is an opinion I need to share with the world",
		},
		{
			name:         "replaces profane words case insensitively",
			body:         `{"body":"This kerfuffle and SHARBERT and fornax should change"}`,
			expectedCode: http.StatusOK,
			expectedKey:  "cleaned_body",
			expectedBody: "This **** and **** and **** should change",
		},
		{
			name:         "does not replace punctuated words",
			body:         `{"body":"Sharbert! stays, but sharbert changes"}`,
			expectedCode: http.StatusOK,
			expectedKey:  "cleaned_body",
			expectedBody: "Sharbert! stays, but **** changes",
		},
		{
			name:         "chirp too long",
			body:         `{"body":"` + strings.Repeat("a", 141) + `"}`,
			expectedCode: http.StatusBadRequest,
			expectedKey:  "error",
		},
		{
			name:         "invalid JSON",
			body:         `{"body":`,
			expectedCode: http.StatusInternalServerError,
			expectedKey:  "error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(server.URL+"/api/validate_chirp", "application/json", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedCode {
				t.Errorf("Expected status %d, got %d", tc.expectedCode, resp.StatusCode)
			}

			contentType := resp.Header.Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
			}

			var result map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("Failed to decode response body: %v", err)
			}

			if _, ok := result[tc.expectedKey]; !ok {
				t.Errorf("Expected key '%s' in response, got: %v", tc.expectedKey, result)
			}

			if tc.expectedBody != "" {
				if got, ok := result[tc.expectedKey].(string); !ok || got != tc.expectedBody {
					t.Errorf("Expected %s to be %q, got %v", tc.expectedKey, tc.expectedBody, result[tc.expectedKey])
				}
			}
		})
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

func TestCreateUserEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	userID := "50746277-23c6-4d85-a890-564c0044c2fb"
	email := "user@example.com"

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO users (id, created_at, updated_at, email)")).
		WithArgs(email).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email"}).
				AddRow(userID, createdAt, createdAt, email),
		)

	cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
	server := httptest.NewServer(makeHandlerWithConfig(cfg))
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/users", "application/json", bytes.NewBufferString(`{"email":"user@example.com"}`))
	if err != nil {
		t.Fatalf("failed to create user request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", contentType)
	}

	var result struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Email     string `json:"email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.ID != userID {
		t.Errorf("expected id %q, got %q", userID, result.ID)
	}
	if result.Email != email {
		t.Errorf("expected email %q, got %q", email, result.Email)
	}
	createdAtParsed, err := time.Parse(time.RFC3339, result.CreatedAt)
	if err != nil {
		t.Fatalf("expected created_at to be RFC3339, parse error: %v", err)
	}
	updatedAtParsed, err := time.Parse(time.RFC3339, result.UpdatedAt)
	if err != nil {
		t.Fatalf("expected updated_at to be RFC3339, parse error: %v", err)
	}
	if !createdAtParsed.Equal(createdAt) {
		t.Errorf("expected created_at %v, got %v", createdAt, createdAtParsed)
	}
	if !updatedAtParsed.Equal(createdAt) {
		t.Errorf("expected updated_at %v, got %v", createdAt, updatedAtParsed)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestCreateUserEndpointInvalidJSON(t *testing.T) {
	cfg := &apiConfig{platform: "dev"}
	server := httptest.NewServer(makeHandlerWithConfig(cfg))
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/users", "application/json", bytes.NewBufferString(`{"email":`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", resp.StatusCode)
	}
}

func TestResetEndpointForbiddenInNonDev(t *testing.T) {
	cfg := &apiConfig{platform: "prod"}
	server := httptest.NewServer(makeHandlerWithConfig(cfg))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/reset", "", nil)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", resp.StatusCode)
	}
}

func TestResetEndpointDeletesUsersInDev(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM users")).WillReturnResult(sqlmock.NewResult(0, 2))

	cfg := &apiConfig{
		platform:  "dev",
		dbQueries: database.New(db),
	}
	cfg.fileserverHits.Store(9)

	req := httptest.NewRequest(http.MethodPost, "/admin/reset", nil)
	rr := httptest.NewRecorder()

	cfg.handlerReset(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if got := cfg.fileserverHits.Load(); got != 0 {
		t.Fatalf("expected fileserverHits to be reset to 0, got %d", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

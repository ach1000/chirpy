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
	"github.com/google/uuid"
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

func TestCreateChirpEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	chirpID := "94b7e44c-3604-42e3-bef7-ebfcc3efff8f"
	userIDStr := "50746277-23c6-4d85-a890-564c0044c2fb"
	userID := uuid.MustParse(userIDStr)

	tests := []struct {
		name         string
		body         string
		expectedCode int
		setupMock    func()
		checkBody    func(t *testing.T, result map[string]any)
	}{
		{
			name:         "valid chirp saved to db",
			body:         `{"body":"Hello, world!","user_id":"` + userIDStr + `"}`,
			expectedCode: http.StatusCreated,
			setupMock: func() {
				mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO chirps (id, created_at, updated_at, body, user_id)")).
					WithArgs("Hello, world!", userID).
					WillReturnRows(
						sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
							AddRow(chirpID, createdAt, createdAt, "Hello, world!", userIDStr),
					)
			},
			checkBody: func(t *testing.T, result map[string]any) {
				if result["id"] != chirpID {
					t.Errorf("expected id %q, got %v", chirpID, result["id"])
				}
				if result["body"] != "Hello, world!" {
					t.Errorf("expected body 'Hello, world!', got %v", result["body"])
				}
				if result["user_id"] != userIDStr {
					t.Errorf("expected user_id %q, got %v", userIDStr, result["user_id"])
				}
				if _, err := time.Parse(time.RFC3339, result["created_at"].(string)); err != nil {
					t.Errorf("created_at not RFC3339: %v", err)
				}
				if _, err := time.Parse(time.RFC3339, result["updated_at"].(string)); err != nil {
					t.Errorf("updated_at not RFC3339: %v", err)
				}
			},
		},
		{
			name:         "profanity is cleaned before saving",
			body:         `{"body":"This kerfuffle and SHARBERT!","user_id":"` + userIDStr + `"}`,
			expectedCode: http.StatusCreated,
			setupMock: func() {
				mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO chirps (id, created_at, updated_at, body, user_id)")).
					WithArgs("This **** and SHARBERT!", userID).
					WillReturnRows(
						sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
							AddRow(chirpID, createdAt, createdAt, "This **** and SHARBERT!", userIDStr),
					)
			},
			checkBody: func(t *testing.T, result map[string]any) {
				if result["body"] != "This **** and SHARBERT!" {
					t.Errorf("expected cleaned body, got %v", result["body"])
				}
			},
		},
		{
			name:         "chirp too long returns 400",
			body:         `{"body":"` + strings.Repeat("a", 141) + `","user_id":"` + userIDStr + `"}`,
			expectedCode: http.StatusBadRequest,
			setupMock:    func() {},
			checkBody: func(t *testing.T, result map[string]any) {
				if _, ok := result["error"]; !ok {
					t.Errorf("expected 'error' key in response, got: %v", result)
				}
			},
		},
		{
			name:         "invalid user_id returns 400",
			body:         `{"body":"Hello","user_id":"not-a-uuid"}`,
			expectedCode: http.StatusBadRequest,
			setupMock:    func() {},
			checkBody: func(t *testing.T, result map[string]any) {
				if _, ok := result["error"]; !ok {
					t.Errorf("expected 'error' key in response, got: %v", result)
				}
			},
		},
		{
			name:         "invalid JSON returns 500",
			body:         `{"body":`,
			expectedCode: http.StatusInternalServerError,
			setupMock:    func() {},
			checkBody: func(t *testing.T, result map[string]any) {
				if _, ok := result["error"]; !ok {
					t.Errorf("expected 'error' key in response, got: %v", result)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()
			cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
			server := httptest.NewServer(makeHandlerWithConfig(cfg))
			defer server.Close()

			resp, err := http.Post(server.URL+"/api/chirps", "application/json", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedCode {
				t.Errorf("expected status %d, got %d", tc.expectedCode, resp.StatusCode)
			}

			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			var result map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if tc.checkBody != nil {
				tc.checkBody(t, result)
			}
		})
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
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

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/ach1000/chirpy/internal/auth"
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

	jwtSecret := "test-secret"
	validToken, err := auth.MakeJWT(userID, jwtSecret, time.Hour)
	if err != nil {
		t.Fatalf("failed to create test JWT: %v", err)
	}

	tests := []struct {
		name         string
		body         string
		authHeader   string
		expectedCode int
		setupMock    func()
		checkBody    func(t *testing.T, result map[string]any)
	}{
		{
			name:         "valid chirp saved to db",
			body:         `{"body":"Hello, world!"}`,
			authHeader:   "Bearer " + validToken,
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
			body:         `{"body":"This kerfuffle and SHARBERT!"}`,
			authHeader:   "Bearer " + validToken,
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
			body:         `{"body":"` + strings.Repeat("a", 141) + `"}`,
			authHeader:   "Bearer " + validToken,
			expectedCode: http.StatusBadRequest,
			setupMock:    func() {},
			checkBody: func(t *testing.T, result map[string]any) {
				if _, ok := result["error"]; !ok {
					t.Errorf("expected 'error' key in response, got: %v", result)
				}
			},
		},
		{
			name:         "missing Authorization header returns 401",
			body:         `{"body":"Hello"}`,
			authHeader:   "",
			expectedCode: http.StatusUnauthorized,
			setupMock:    func() {},
			checkBody: func(t *testing.T, result map[string]any) {
				if _, ok := result["error"]; !ok {
					t.Errorf("expected 'error' key in response, got: %v", result)
				}
			},
		},
		{
			name:         "invalid JWT returns 401",
			body:         `{"body":"Hello"}`,
			authHeader:   "Bearer not-a-real-token",
			expectedCode: http.StatusUnauthorized,
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
			authHeader:   "Bearer " + validToken,
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
			cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
			server := httptest.NewServer(makeHandlerWithConfig(cfg))
			defer server.Close()

			req, err := http.NewRequest(http.MethodPost, server.URL+"/api/chirps", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("failed to build request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			resp, err := http.DefaultClient.Do(req)
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

func TestGetChirpsEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	t1 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	chirpID1 := "94b7e44c-3604-42e3-bef7-ebfcc3efff8f"
	chirpID2 := "f0f87ec2-a8b5-48cc-b66a-a85ce7c7b862"
	userIDStr := "50746277-23c6-4d85-a890-564c0044c2fb"

	t.Run("returns chirps ordered by created_at asc", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, body, user_id")).
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
					AddRow(chirpID1, t1, t1, "First chirp", userIDStr).
					AddRow(chirpID2, t2, t2, "Second chirp", userIDStr),
			)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/chirps")
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}

		var result []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 chirps, got %d", len(result))
		}
		if result[0]["id"] != chirpID1 {
			t.Errorf("expected first chirp id %q, got %v", chirpID1, result[0]["id"])
		}
		if result[1]["id"] != chirpID2 {
			t.Errorf("expected second chirp id %q, got %v", chirpID2, result[1]["id"])
		}
		if result[0]["body"] != "First chirp" {
			t.Errorf("expected body 'First chirp', got %v", result[0]["body"])
		}
		if result[0]["user_id"] != userIDStr {
			t.Errorf("expected user_id %q, got %v", userIDStr, result[0]["user_id"])
		}
		if _, err := time.Parse(time.RFC3339, result[0]["created_at"].(string)); err != nil {
			t.Errorf("created_at not RFC3339: %v", err)
		}
	})

	t.Run("returns chirps in descending order when sort=desc", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, body, user_id")).
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
					AddRow(chirpID1, t1, t1, "First chirp", userIDStr).
					AddRow(chirpID2, t2, t2, "Second chirp", userIDStr),
			)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/chirps?sort=desc")
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		var result []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 chirps, got %d", len(result))
		}
		if result[0]["id"] != chirpID2 {
			t.Errorf("expected first chirp id %q, got %v", chirpID2, result[0]["id"])
		}
		if result[1]["id"] != chirpID1 {
			t.Errorf("expected second chirp id %q, got %v", chirpID1, result[1]["id"])
		}
	})

	t.Run("filters by author_id at the database level", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, body, user_id\nFROM chirps\nWHERE user_id = $1")).
			WithArgs(uuid.MustParse(userIDStr)).
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
					AddRow(chirpID1, t1, t1, "First chirp", userIDStr),
			)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/chirps?author_id=" + userIDStr)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		var result []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(result) != 1 {
			t.Fatalf("expected 1 chirp, got %d", len(result))
		}
		if result[0]["id"] != chirpID1 {
			t.Errorf("expected chirp id %q, got %v", chirpID1, result[0]["id"])
		}
	})

	t.Run("returns 400 for an invalid author_id", func(t *testing.T) {
		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/chirps?author_id=not-a-uuid")
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", resp.StatusCode)
		}
	})

	t.Run("returns empty array when no chirps", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, body, user_id")).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}))

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/chirps")
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if string(body) != "[]" {
			t.Errorf("expected empty JSON array '[]', got %q", string(body))
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestGetChirpByIDEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	chirpID := "94b7e44c-3604-42e3-bef7-ebfcc3efff8f"
	userIDStr := "50746277-23c6-4d85-a890-564c0044c2fb"
	parsedChirpID := uuid.MustParse(chirpID)

	t.Run("returns chirp for existing id", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, body, user_id")).
			WithArgs(parsedChirpID).
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
					AddRow(chirpID, createdAt, createdAt, "fr? no clowning?", userIDStr),
			)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/chirps/" + chirpID)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if result["id"] != chirpID {
			t.Errorf("expected id %q, got %v", chirpID, result["id"])
		}
		if result["body"] != "fr? no clowning?" {
			t.Errorf("expected body %q, got %v", "fr? no clowning?", result["body"])
		}
		if result["user_id"] != userIDStr {
			t.Errorf("expected user_id %q, got %v", userIDStr, result["user_id"])
		}
	})

	t.Run("returns 404 for missing id", func(t *testing.T) {
		missingID := "f0f87ec2-a8b5-48cc-b66a-a85ce7c7b862"
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, body, user_id")).
			WithArgs(uuid.MustParse(missingID)).
			WillReturnError(sql.ErrNoRows)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/chirps/" + missingID)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", resp.StatusCode)
		}
	})

	t.Run("returns 404 for invalid id format", func(t *testing.T) {
		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/chirps/not-a-uuid")
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", resp.StatusCode)
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestDeleteChirpEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	chirpID := "94b7e44c-3604-42e3-bef7-ebfcc3efff8f"
	ownerIDStr := "50746277-23c6-4d85-a890-564c0044c2fb"
	otherIDStr := "f0f87ec2-a8b5-48cc-b66a-a85ce7c7b862"
	parsedChirpID := uuid.MustParse(chirpID)
	ownerID := uuid.MustParse(ownerIDStr)
	otherID := uuid.MustParse(otherIDStr)

	jwtSecret := "test-secret"
	ownerToken, err := auth.MakeJWT(ownerID, jwtSecret, time.Hour)
	if err != nil {
		t.Fatalf("failed to create owner JWT: %v", err)
	}
	otherToken, err := auth.MakeJWT(otherID, jwtSecret, time.Hour)
	if err != nil {
		t.Fatalf("failed to create other JWT: %v", err)
	}

	t.Run("owner can delete their chirp", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, body, user_id")).
			WithArgs(parsedChirpID).
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
					AddRow(chirpID, createdAt, createdAt, "mine", ownerIDStr),
			)
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM chirps")).
			WithArgs(parsedChirpID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		req, err := http.NewRequest(http.MethodDelete, server.URL+"/api/chirps/"+chirpID, nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+ownerToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected status 204, got %d", resp.StatusCode)
		}
	})

	t.Run("non-owner gets 403", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, body, user_id")).
			WithArgs(parsedChirpID).
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
					AddRow(chirpID, createdAt, createdAt, "mine", ownerIDStr),
			)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		req, err := http.NewRequest(http.MethodDelete, server.URL+"/api/chirps/"+chirpID, nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+otherToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", resp.StatusCode)
		}
	})

	t.Run("missing chirp returns 404", func(t *testing.T) {
		missingID := "11111111-1111-1111-1111-111111111111"
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, body, user_id")).
			WithArgs(uuid.MustParse(missingID)).
			WillReturnError(sql.ErrNoRows)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		req, err := http.NewRequest(http.MethodDelete, server.URL+"/api/chirps/"+missingID, nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+ownerToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", resp.StatusCode)
		}
	})

	t.Run("missing Authorization header returns 401", func(t *testing.T) {
		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		req, err := http.NewRequest(http.MethodDelete, server.URL+"/api/chirps/"+chirpID, nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})

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
	password := "04234"

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO users (id, created_at, updated_at, email, hashed_password)")).
		WithArgs(email, sqlmock.AnyArg()).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password", "is_chirpy_red"}).
				AddRow(userID, createdAt, createdAt, email, "$argon2id$v=19$m=65536,t=1,p=24$abc$def", false),
		)

	cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
	server := httptest.NewServer(makeHandlerWithConfig(cfg))
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/users", "application/json", bytes.NewBufferString(`{"email":"user@example.com","password":"`+password+`"}`))
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

func TestCreateUserEndpointMissingPassword(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	cfg := &apiConfig{platform: "dev", dbQueries: database.New(db)}
	server := httptest.NewServer(makeHandlerWithConfig(cfg))
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/users", "application/json", bytes.NewBufferString(`{"email":"user@example.com"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestUpdateUserEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	updatedAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	userIDStr := "50746277-23c6-4d85-a890-564c0044c2fb"
	userID := uuid.MustParse(userIDStr)
	newEmail := "newemail@example.com"

	jwtSecret := "test-secret"
	validToken, err := auth.MakeJWT(userID, jwtSecret, time.Hour)
	if err != nil {
		t.Fatalf("failed to create test JWT: %v", err)
	}

	tests := []struct {
		name         string
		body         string
		authHeader   string
		expectedCode int
		setupMock    func()
		checkBody    func(t *testing.T, result map[string]any)
	}{
		{
			name:         "valid update",
			body:         `{"email":"` + newEmail + `","password":"newpassword123"}`,
			authHeader:   "Bearer " + validToken,
			expectedCode: http.StatusOK,
			setupMock: func() {
				mock.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
					WithArgs(userID, newEmail, sqlmock.AnyArg()).
					WillReturnRows(
						sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password", "is_chirpy_red"}).
							AddRow(userIDStr, updatedAt, updatedAt, newEmail, "$argon2id$v=19$m=65536,t=1,p=24$abc$def", false),
					)
			},
			checkBody: func(t *testing.T, result map[string]any) {
				if result["id"] != userIDStr {
					t.Errorf("expected id %q, got %v", userIDStr, result["id"])
				}
				if result["email"] != newEmail {
					t.Errorf("expected email %q, got %v", newEmail, result["email"])
				}
				if _, ok := result["password"]; ok {
					t.Errorf("expected no password field in response, got: %v", result)
				}
			},
		},
		{
			name:         "missing Authorization header returns 401",
			body:         `{"email":"` + newEmail + `","password":"newpassword123"}`,
			authHeader:   "",
			expectedCode: http.StatusUnauthorized,
			setupMock:    func() {},
			checkBody: func(t *testing.T, result map[string]any) {
				if _, ok := result["error"]; !ok {
					t.Errorf("expected 'error' key in response, got: %v", result)
				}
			},
		},
		{
			name:         "malformed Authorization header returns 401",
			body:         `{"email":"` + newEmail + `","password":"newpassword123"}`,
			authHeader:   "Bearer not-a-real-token",
			expectedCode: http.StatusUnauthorized,
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
			cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
			server := httptest.NewServer(makeHandlerWithConfig(cfg))
			defer server.Close()

			req, err := http.NewRequest(http.MethodPut, server.URL+"/api/users", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("failed to build request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedCode {
				t.Errorf("expected status %d, got %d", tc.expectedCode, resp.StatusCode)
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

func TestLoginEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	userID := "50746277-23c6-4d85-a890-564c0044c2fb"
	email := "user@example.com"
	password := "04234"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	jwtSecret := "test-secret"

	makeUserRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password", "is_chirpy_red"}).
			AddRow(userID, createdAt, createdAt, email, hash, false)
	}

	t.Run("returns 200 with a valid access token and refresh token for correct credentials", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, email, hashed_password")).
			WithArgs(email).
			WillReturnRows(makeUserRows())
		mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO refresh_tokens")).
			WithArgs(sqlmock.AnyArg(), uuid.MustParse(userID), sqlmock.AnyArg()).
			WillReturnRows(
				sqlmock.NewRows([]string{"token", "created_at", "updated_at", "user_id", "expires_at", "revoked_at"}).
					AddRow("some-refresh-token", createdAt, createdAt, userID, createdAt.AddDate(0, 0, 60), nil),
			)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBufferString(`{"email":"`+email+`","password":"`+password+`"}`))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if result["id"] != userID {
			t.Errorf("expected id %q, got %v", userID, result["id"])
		}
		if result["email"] != email {
			t.Errorf("expected email %q, got %v", email, result["email"])
		}
		if _, hasHash := result["hashed_password"]; hasHash {
			t.Errorf("response should not include hashed_password")
		}
		if result["refresh_token"] != "some-refresh-token" {
			t.Errorf("expected refresh_token %q, got %v", "some-refresh-token", result["refresh_token"])
		}

		token, _ := result["token"].(string)
		gotUserID, err := auth.ValidateJWT(token, jwtSecret)
		if err != nil {
			t.Fatalf("expected a valid JWT in the response, got error: %v", err)
		}
		if gotUserID.String() != userID {
			t.Errorf("expected token subject %q, got %v", userID, gotUserID)
		}
	})

	t.Run("returns 401 for wrong password", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, email, hashed_password")).
			WithArgs(email).
			WillReturnRows(makeUserRows())

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBufferString(`{"email":"`+email+`","password":"bad"}`))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("returns 401 for missing user", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, email, hashed_password")).
			WithArgs("missing@example.com").
			WillReturnError(sql.ErrNoRows)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBufferString(`{"email":"missing@example.com","password":"whatever"}`))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRefreshEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	userID := "50746277-23c6-4d85-a890-564c0044c2fb"
	email := "user@example.com"
	jwtSecret := "test-secret"

	t.Run("returns 200 with a new access token for a valid refresh token", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT users.id, users.created_at, users.updated_at, users.email, users.hashed_password")).
			WithArgs("valid-refresh-token").
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password", "is_chirpy_red"}).
					AddRow(userID, createdAt, createdAt, email, "hash", false),
			)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/refresh", nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer valid-refresh-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		token, _ := result["token"].(string)
		gotUserID, err := auth.ValidateJWT(token, jwtSecret)
		if err != nil {
			t.Fatalf("expected a valid JWT in the response, got error: %v", err)
		}
		if gotUserID.String() != userID {
			t.Errorf("expected token subject %q, got %v", userID, gotUserID)
		}
	})

	t.Run("returns 401 for missing Authorization header", func(t *testing.T) {
		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Post(server.URL+"/api/refresh", "", nil)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("returns 401 for expired or revoked refresh token", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT users.id, users.created_at, users.updated_at, users.email, users.hashed_password")).
			WithArgs("expired-refresh-token").
			WillReturnError(sql.ErrNoRows)

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", jwtSecret: jwtSecret}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/refresh", nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer expired-refresh-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRevokeEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	t.Run("returns 204 and revokes the token", func(t *testing.T) {
		mock.ExpectExec(regexp.QuoteMeta("UPDATE refresh_tokens")).
			WithArgs("some-refresh-token").
			WillReturnResult(sqlmock.NewResult(0, 1))

		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/revoke", nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer some-refresh-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected status 204, got %d", resp.StatusCode)
		}
	})

	t.Run("returns 401 for missing Authorization header", func(t *testing.T) {
		cfg := &apiConfig{dbQueries: database.New(db), platform: "dev"}
		server := httptest.NewServer(makeHandlerWithConfig(cfg))
		defer server.Close()

		resp, err := http.Post(server.URL+"/api/revoke", "", nil)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestPolkaWebhooksEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	userID := "3311741c-680c-4546-99f3-fc9efac2036c"
	polkaKey := "f271c81ff7084ee5b99a5091b42d486e"

	cfg := &apiConfig{dbQueries: database.New(db), platform: "dev", polkaKey: polkaKey}
	server := httptest.NewServer(makeHandlerWithConfig(cfg))
	defer server.Close()

	postWebhook := func(t *testing.T, body, authHeader string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/polka/webhooks", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		return resp
	}

	t.Run("returns 204 and upgrades the user for a user.upgraded event", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
			WithArgs(uuid.MustParse(userID)).
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password", "is_chirpy_red"}).
					AddRow(userID, createdAt, createdAt, "user@example.com", "hash", true),
			)

		body := `{"event":"user.upgraded","data":{"user_id":"` + userID + `"}}`
		resp := postWebhook(t, body, "ApiKey "+polkaKey)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected status 204, got %d", resp.StatusCode)
		}
	})

	t.Run("returns 404 if the user is not found", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
			WithArgs(uuid.MustParse(userID)).
			WillReturnError(sql.ErrNoRows)

		body := `{"event":"user.upgraded","data":{"user_id":"` + userID + `"}}`
		resp := postWebhook(t, body, "ApiKey "+polkaKey)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", resp.StatusCode)
		}
	})

	t.Run("returns 204 immediately for other events", func(t *testing.T) {
		body := `{"event":"user.deleted","data":{"user_id":"` + userID + `"}}`
		resp := postWebhook(t, body, "ApiKey "+polkaKey)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected status 204, got %d", resp.StatusCode)
		}
	})

	t.Run("returns 401 for a missing Authorization header", func(t *testing.T) {
		body := `{"event":"user.upgraded","data":{"user_id":"` + userID + `"}}`
		resp := postWebhook(t, body, "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("returns 401 for the wrong API key", func(t *testing.T) {
		body := `{"event":"user.upgraded","data":{"user_id":"` + userID + `"}}`
		resp := postWebhook(t, body, "ApiKey wrong-key")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
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

# Chirpy Server - Project Documentation

## Overview
This is a simple Go HTTP fileserver that binds to port 8080 and serves static files from the current directory.

## Architecture

### Current Implementation
The server is implemented in `chirpy.go` with the following components:

1. **apiConfig struct**: Holds in-memory server state
   - `fileserverHits atomic.Int32`: count of requests served through the `/app/` handler, safe for concurrent access

2. **makeHandler()** function: Builds and returns the configured `http.Handler` (mux), independent of starting a real server
   - Creates an `apiConfig` instance (`apiCfg`)
   - Creates an http.ServeMux (request multiplexer/router)
   - Registers handlers in this order:
     - `GET /api/healthz`: Readiness endpoint that returns 200 OK with "OK" message
     - `/app/` path: Serves files from the current directory (`.`) via `http.StripPrefix` and `http.FileServer`, wrapped in `apiCfg.middlewareMetricsInc`
     - `GET /admin/metrics`: Returns the current hit count rendered as HTML
     - `POST /admin/reset`: Resets the hit count to 0
     - `POST /api/chirps` accepts `{ "body": "...", "user_id": "..." }`, validates body length (max 140 chars), cleans profanity, saves to DB via SQLC `CreateChirp`, and returns `201 Created` with the full chirp resource.
   - Returns the mux

3. **main()** function: Sets up and starts the server
   - Creates an http.Server struct configured with:
     - `Handler`: Set to `makeHandler()`
     - `Addr`: Set to ":8080"
   - Calls `ListenAndServe()` to start the HTTP server

4. **handlerReadiness()** function: Handles health check requests
   - Sets Content-Type header to "text/plain; charset=utf-8"
   - Writes HTTP 200 OK status code
   - Writes response body "OK"
   - Used for checking if the server is ready to receive traffic
   - Only registered for GET; other methods get an automatic 405

5. **middlewareMetricsInc()** method on `*apiConfig`: wraps an `http.Handler` and increments `fileserverHits` (via `.Add(1)`) on every request before calling the wrapped handler

6. **handlerMetrics()** method on `*apiConfig`: writes an HTML page (Content-Type `text/html; charset=utf-8`) showing "Chirpy has been visited x times!", where `x` is the current `fileserverHits` value (via `.Load()`), built with `fmt.Fprintf` against an HTML template

7. **handlerReset()** method on `*apiConfig`: resets `fileserverHits` to 0 (via `.Store(0)`) and returns 200 OK

8. **handlerChirpsCreate()** method on `*apiConfig`: decodes `{"body": "...", "user_id": "..."}`, returns 500 on decode failure, 400 if body exceeds 140 chars, 400 if `user_id` is not a valid UUID; otherwise cleans profanity, calls SQLC `CreateChirp`, returns 201 with `{"id", "created_at", "updated_at", "body", "user_id"}`

9. **respondWithJSON() / respondWithError()** helpers: `respondWithJSON` marshals any payload to JSON, sets `Content-Type: application/json`, and writes the status code + body; `respondWithError` wraps it to emit `{"error": msg}`

### FileServer Behavior
**App Handler** (`/app/`):
- Serves files from the current directory (`.`)
- Uses `http.StripPrefix` to remove `/app/` prefix from request path
- Automatically serves `index.html` when accessing `/app/`
- Example: Request to `/app/index.html` serves `index.html`

## Building and Running

```bash
# Build the executable
make build      # or: go build -o chirpy

# Run the server
make run        # builds then runs ./chirpy

# Run the test suite
make test       # or: go test ./...

# Clean the built binary
make clean
```

The server will start listening on `http://localhost:8080`.

## Database Migrations (Goose)
- Migration files live in `sql/schema`.
- Verified Linux Postgres connection string for this project: `postgres://postgres:postgres@localhost:5432/chirpy`.
- Goose is installed via:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

- If `goose` is not on PATH, use `$(go env GOPATH)/bin/goose`.
- Verified migration workflow from `sql/schema`:

```bash
$(go env GOPATH)/bin/goose postgres "postgres://postgres:postgres@localhost:5432/chirpy" up
$(go env GOPATH)/bin/goose postgres "postgres://postgres:postgres@localhost:5432/chirpy" down
$(go env GOPATH)/bin/goose postgres "postgres://postgres:postgres@localhost:5432/chirpy" up
```

- Verified with `psql` that the `users` table exists after the final `up`.

## SQLC and DB Wiring
- SQLC config file: `sqlc.yaml` (version 2).
- SQLC reads schema from `sql/schema` and queries from `sql/queries`.
- SQLC generated Go package output: `internal/database`.
- Current query file: `sql/queries/users.sql` with `CreateUser` insert query.
- `sql/queries/users.sql` also includes `DeleteUsers` for admin reset behavior.
- `sql/queries/chirps.sql` has `CreateChirp` query (INSERT with body and user_id params).
- SQLC CLI install/verify:

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc version
```

- SQLC generation from repo root:

```bash
sqlc generate
```

- Environment configuration:
   - `.env` is used locally and ignored by git.
   - `DB_URL` includes `?sslmode=disable` for local Postgres.
- Server DB setup in `chirpy.go`:
   - Loads `.env` with `godotenv.Load()`.
   - Reads `DB_URL` from environment.
   - Reads `PLATFORM` from environment.
   - Opens Postgres with `sql.Open("postgres", dbURL)`.
   - Creates SQLC queries via `database.New(db)`.
   - Stores `*database.Queries` on `apiConfig` for handler access.
- API additions:
   - `POST /api/users` creates a user from JSON body `{ "email": "..." }` using SQLC `CreateUser(r.Context(), email)` and returns `201 Created` with user fields.
   - `POST /admin/reset` is restricted to `PLATFORM=dev`; otherwise returns `403 Forbidden`.
   - In `dev`, `POST /admin/reset` deletes all users via SQLC `DeleteUsers` and also resets in-memory fileserver metrics.
- Required Go dependencies added:
   - `github.com/google/uuid`
   - `github.com/lib/pq`
   - `github.com/joho/godotenv`

## Testing
Automated tests live in `chirpy_test.go`, run via `go test ./...`. They use `httptest.NewServer(makeHandler())` to exercise the real mux without binding to the production port:
- **TestServeIndexHTML**: `GET /app/index.html` returns 200 and contains "Welcome to Chirpy"
- **TestReadinessEndpoint**: `GET /api/healthz` returns 200, correct Content-Type, and body "OK"
- **TestMetricsEndpoint**: two hits to `/app/` followed by `GET /admin/metrics` returns HTML containing "Chirpy has been visited 2 times!"
- **TestResetEndpoint**: a hit to `/app/` followed by `POST /admin/reset` brings `/admin/metrics` back to "Chirpy has been visited 0 times!"
- **TestMethodNotAllowed**: wrong-method requests to `/api/healthz`, `/admin/metrics`, `/admin/reset` all return 405
- **TestCreateChirpEndpoint**: table-driven test against `POST /api/chirps` using sqlmock; covers valid chirp creation (201 with all fields), profanity cleaning before DB insert, too-long chirp (400), invalid UUID user_id (400), malformed JSON (500)
- **TestMiddlewareMetricsInc**: calls `middlewareMetricsInc` directly against a stub handler and checks `fileserverHits` increments correctly
- **TestCreateUserEndpoint**: verifies `POST /api/users` returns 201 with `id`, `created_at`, `updated_at`, and `email` in the expected JSON shape
- **TestResetEndpointForbiddenInNonDev / TestResetEndpointDeletesUsersInDev**: verify `POST /admin/reset` returns 403 outside `dev`, and in `dev` it executes user deletion plus resets metrics

### Manual Testing
- **Health Check** (`GET /api/healthz`): Returns 200 OK with "OK" message; other methods get 405
- **App Path** (`/app/`): Serves index.html and other files from the current directory; each hit increments the metrics counter
- **Metrics** (`GET /admin/metrics`): Returns an HTML page showing the visit count; meant to be viewed in a browser; other methods get 405
- **Reset** (`POST /admin/reset`): Resets the hit counter to 0; other methods get 405
- **Chirps** (`POST /api/chirps`): Accepts `{"body": "...", "user_id": "..."}`, validates length and UUID, cleans profanity, saves to DB, returns 201 with full chirp resource

Example:
```bash
curl http://localhost:8080/api/healthz
curl http://localhost:8080/app/
curl http://localhost:8080/admin/metrics
curl -X POST http://localhost:8080/admin/reset
curl -X POST http://localhost:8080/api/chirps -d '{"body":"hello","user_id":"<uuid>"}'
```

## Key Design Decisions
- **Health Check Endpoint**: A dedicated `/api/healthz` readiness endpoint allows external systems (load balancers, orchestration systems) to monitor server health
- **Application Server Path**: The fileserver is under `/app/` instead of `/` to avoid conflicts with the health check endpoint and API endpoints
- **API Namespace**: Non-fileserver, externally-facing endpoints are served under the `/api` path prefix, keeping API routing decoupled from the website path even though the server is currently a monolith
- **Admin Namespace**: Endpoints intended for internal/administrative use (`/admin/metrics`, `/admin/reset`) are served under a separate `/admin` path prefix purely for organizational clarity — there is nothing inherently more secure about this namespace
- **HTML Admin Metrics**: `/admin/metrics` returns a small HTML page (not plain text) so it can be loaded and viewed directly in a browser, with the count updating on refresh as `/app/` is hit
- **http.StripPrefix**: Used to cleanly map the `/app/` URL path to the filesystem root (e.g., `/app/index.html` → `index.html`)
- **FileServer Handler**: Uses Go's standard `http.FileServer` to serve static files without custom route logic
- **Standard Library Only**: Uses only Go's `net/http` package (no external dependencies)
- **Method-Specific Routing**: `/api/healthz` and `/admin/metrics` are registered as `GET`, and `/admin/reset`/`/api/validate_chirp` as `POST`, using Go 1.22+'s `"METHOD /path"` mux pattern syntax — mismatched methods get an automatic 405 Method Not Allowed
- **makeHandler() Extraction**: Mux/handler setup lives in `makeHandler() http.Handler`, separate from `main()`, so it can be exercised in tests via `httptest.NewServer(makeHandler())` without starting a real listener
- **JSON Request/Response Helpers**: `respondWithJSON`/`respondWithError` centralize JSON marshalling and header/status-code handling so future JSON endpoints don't repeat that boilerplate

## Project Structure
```
chirpy/
├── sql/
│   └── schema/
│       ├── 001_users.sql   # Goose migration: create/drop users table
│       └── 002_chirps.sql  # Goose migration: create/drop chirps table (FK -> users ON DELETE CASCADE)
├── chirpy.go        # Server implementation (main, makeHandler, handlerReadiness, apiConfig handlers)
├── chirpy_test.go   # Unit tests for the handlers and middleware
├── go.mod           # Go module definition
├── index.html       # Static HTML file served at /app/
├── assets/          # Static assets (e.g. logo.png), served at /app/assets/ via the same fileserver handler
├── Makefile         # build/run/test/clean targets
├── PROJECT.md        # This documentation file
└── chirpy           # Compiled binary (not tracked in git)
```

## Future Changes
When adding more static files or modifying the server:
1. Add HTML/CSS/JS files to the project root directory (or `assets/` once added)
2. Rebuild with `make build`
3. Restart the server with `make run`

The FileServer will automatically serve any files placed in the served directory.

---
**Note**: Update this file whenever significant changes are made to the server implementation.

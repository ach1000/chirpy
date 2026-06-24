# Chirpy Server - Project Documentation

## Overview
This is a simple Go HTTP fileserver that binds to port 8080 and serves static files from the current directory.

## Architecture

`chirpy.go` builds an `http.ServeMux` in `makeHandlerWithConfig(apiCfg)` (wrapped by `makeHandler()` for production use) and registers:
- `GET /api/healthz`: readiness check, 200 "OK"
- `/app/`: static fileserver rooted at `.` via `http.StripPrefix`+`http.FileServer` (e.g. `/app/index.html` → `index.html`), wrapped in `middlewareMetricsInc`, which increments `apiConfig.fileserverHits` (`atomic.Int32`) on every hit
- `GET /admin/metrics`: HTML page showing the current hit count
- `POST /admin/reset`: resets the hit count; restricted to `PLATFORM=dev` (`403` otherwise), and in `dev` also deletes all users via SQLC `DeleteUsers`
- `POST /api/users`, `POST /api/login`, `POST /api/refresh`, `POST /api/revoke`: see API additions under SQLC and DB Wiring
- `POST /api/chirps`, `GET /api/chirps`, `GET /api/chirps/{chirpID}`: chirp CRUD; create requires a valid `Authorization: Bearer <JWT>` (the user id comes from the token, not the body), validates body length (140 chars max), cleans profanity, and saves via SQLC `CreateChirp`

`respondWithJSON`/`respondWithError` centralize JSON response handling. `userResponse`/`chirpResponse` (built via `newUserResponse`/`newChirpResponse`) are the shared resource shapes returned by the user and chirp handlers.

`main()` starts an `http.Server` on `:8080` using `makeHandler()`.

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
- Verified workflow from `sql/schema`: `goose postgres "<conn string>" up` (and `down` to roll back).

## SQLC and DB Wiring
- SQLC config file: `sqlc.yaml` (version 2).
- SQLC reads schema from `sql/schema` and queries from `sql/queries`.
- SQLC generated Go package output: `internal/database`.
- Current query file: `sql/queries/users.sql` with `CreateUser` insert query.
- `sql/queries/users.sql` also includes `DeleteUsers` for admin reset behavior, and `GetUserByEmail` for login lookups.
- `sql/queries/chirps.sql` has `CreateChirp` query (INSERT with body and user_id params).
- `users` table has a non-null `hashed_password TEXT` column (default `'unset'`, added in migration `003_users_hashed_password.sql`).
- `refresh_tokens` table (migration `004_refresh_tokens.sql`): `token` (PK, text), `created_at`, `updated_at`, `user_id` (FK → `users.id` ON DELETE CASCADE), `expires_at`, nullable `revoked_at`. Queries in `sql/queries/refresh_tokens.sql`: `CreateRefreshToken`, `GetUserFromRefreshToken` (join filtering out expired/revoked rows), `RevokeRefreshToken` (sets `revoked_at`/`updated_at` to now).
- All `created_at`/`updated_at`/`expires_at`/`revoked_at` columns across `users`, `chirps`, `refresh_tokens` are `TIMESTAMPTZ`, not the bare `TIMESTAMP` SQL default — needed so refresh-token expiry comparisons (`expires_at > NOW()`) are correct regardless of the Postgres session's timezone, since `TIMESTAMPTZ` stores an absolute instant rather than a naive wall-clock value.
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
   - `JWT_SECRET` signs/verifies JWTs (generate with `openssl rand -base64 64`); server fails fast at startup if unset.
- Server DB setup in `chirpy.go`:
   - Loads `.env` with `godotenv.Load()`.
   - Reads `DB_URL`, `PLATFORM`, `JWT_SECRET` from environment.
   - Opens Postgres with `sql.Open("postgres", dbURL)`.
   - Creates SQLC queries via `database.New(db)`.
   - Stores `*database.Queries`, `platform`, and `jwtSecret` on `apiConfig` for handler access.
- API additions:
   - `POST /api/users` creates a user from JSON body `{ "email": "...", "password": "..." }`; hashes the password with `internal/auth.HashPassword` before storing, returns `201 Created` with `{id, created_at, updated_at, email}` (never the hash).
   - `POST /api/login` checks `{ "email": "...", "password": "..." }` against the stored hash via `internal/auth.CheckPasswordHash`; any lookup/mismatch failure returns `401` with `"Incorrect email or password"`. On success returns `200` with the user fields plus an access `token` (JWT, 1 hour expiry) and a `refresh_token` (60-day expiry, persisted via SQLC `CreateRefreshToken`).
   - `POST /api/refresh` takes the refresh token from `Authorization: Bearer <token>`, looks it up via SQLC `GetUserFromRefreshToken` (fails if missing/expired/revoked), and returns `200` with a fresh `{"token": "..."}` access token.
   - `POST /api/revoke` takes the refresh token from `Authorization: Bearer <token>` and marks it revoked via SQLC `RevokeRefreshToken`; returns `204 No Content`.
- `internal/auth` package (tested in `internal/auth/auth_test.go`):
   - Password hashing via `github.com/alexedwards/argon2id`: `HashPassword`, `CheckPasswordHash`.
   - JWTs via `github.com/golang-jwt/jwt/v5`: `MakeJWT(userID, tokenSecret, expiresIn)` signs an HS256 token (`Issuer: "chirpy-access"`, `Subject` = user ID, UTC `IssuedAt`/`ExpiresAt`); `ValidateJWT(tokenString, tokenSecret)` verifies signature/expiry and returns the user ID from `Subject`.
   - `GetBearerToken(headers http.Header) (string, error)` extracts the token from `Authorization: Bearer <token>`, erroring if the header is missing or malformed.
   - `MakeRefreshToken() string` returns a 256-bit random value (`crypto/rand`) hex-encoded — opaque, not a JWT, validated only against the `refresh_tokens` table.
- Required Go dependencies added:
   - `github.com/google/uuid`
   - `github.com/lib/pq`
   - `github.com/joho/godotenv`
   - `github.com/alexedwards/argon2id`
   - `github.com/golang-jwt/jwt/v5`

## Testing
Automated tests live in `chirpy_test.go`, run via `go test ./...`. They use `httptest.NewServer(makeHandler())` to exercise the real mux without binding to the production port:
- **TestServeIndexHTML**: `GET /app/index.html` returns 200 and contains "Welcome to Chirpy"
- **TestReadinessEndpoint**: `GET /api/healthz` returns 200, correct Content-Type, and body "OK"
- **TestMetricsEndpoint**: two hits to `/app/` followed by `GET /admin/metrics` returns HTML containing "Chirpy has been visited 2 times!"
- **TestResetEndpoint**: a hit to `/app/` followed by `POST /admin/reset` brings `/admin/metrics` back to "Chirpy has been visited 0 times!"
- **TestMethodNotAllowed**: wrong-method requests to `/api/healthz`, `/admin/metrics`, `/admin/reset` all return 405
- **TestCreateChirpEndpoint**: table-driven test against `POST /api/chirps` using sqlmock and a real JWT in the `Authorization` header; covers valid chirp creation (201 with all fields), profanity cleaning before DB insert, too-long chirp (400), missing/invalid bearer token (401), malformed JSON (500)
- **TestMiddlewareMetricsInc**: calls `middlewareMetricsInc` directly against a stub handler and checks `fileserverHits` increments correctly
- **TestCreateUserEndpoint**: verifies `POST /api/users` hashes the password and returns 201 with `id`, `created_at`, `updated_at`, and `email` (no password/hash) in the expected JSON shape
- **TestCreateUserEndpointMissingPassword**: missing `password` field returns 400
- **TestLoginEndpoint**: table of subtests covering `POST /api/login` with correct credentials (200 + valid JWT and refresh token for the user), wrong password (401), and unknown email (401)
- **TestRefreshEndpoint**: `POST /api/refresh` returns a new valid access token for a valid refresh token, 401 for a missing header, and 401 for an expired/revoked/unknown one
- **TestRevokeEndpoint**: `POST /api/revoke` returns 204 and revokes the token, 401 for a missing header
- **TestResetEndpointForbiddenInNonDev / TestResetEndpointDeletesUsersInDev**: verify `POST /admin/reset` returns 403 outside `dev`, and in `dev` it executes user deletion plus resets metrics

`internal/auth/auth_test.go` also covers: **TestMakeJWTAndValidateJWT** (round-trip), **TestValidateJWTExpired** (negative `expiresIn` rejected), **TestValidateJWTWrongSecret** (token signed with a different secret rejected), **TestGetBearerToken** / **TestGetBearerTokenMissingHeader** / **TestGetBearerTokenMalformedHeader**, **TestMakeRefreshToken** (64-char hex, distinct per call).

### Manual Testing
Endpoint behavior matches the Architecture section above; `/admin/metrics` is meant to be viewed in a browser. Quick check: `curl http://localhost:8080/api/healthz`.

## Key Design Decisions
- **Health Check Endpoint**: A dedicated `/api/healthz` readiness endpoint allows external systems (load balancers, orchestration systems) to monitor server health
- **Application Server Path**: The fileserver is under `/app/` instead of `/` to avoid conflicts with the health check endpoint and API endpoints
- **API Namespace**: Non-fileserver, externally-facing endpoints are served under the `/api` path prefix, keeping API routing decoupled from the website path even though the server is currently a monolith
- **Admin Namespace**: Endpoints intended for internal/administrative use (`/admin/metrics`, `/admin/reset`) are served under a separate `/admin` path prefix purely for organizational clarity — there is nothing inherently more secure about this namespace
- **HTML Admin Metrics**: `/admin/metrics` returns a small HTML page (not plain text) so it can be loaded and viewed directly in a browser, with the count updating on refresh as `/app/` is hit
- **Method-Specific Routing**: routes use Go 1.22+'s `"METHOD /path"` mux pattern syntax — mismatched methods get an automatic 405 Method Not Allowed
- **makeHandler() Extraction**: Mux/handler setup lives in `makeHandler() http.Handler`, separate from `main()`, so it can be exercised in tests via `httptest.NewServer(makeHandler())` without starting a real listener
- **JSON Request/Response Helpers**: `respondWithJSON`/`respondWithError` centralize JSON marshalling and header/status-code handling so future JSON endpoints don't repeat that boilerplate
- **Response Type Helpers**: `userResponse`/`newUserResponse`, `chirpResponse`/`newChirpResponse`, and `accessTokenResponse` are shared response shapes so every handler returning a user, chirp, or token resource builds it the same way instead of redeclaring an anonymous struct
- **Auth Helpers**: `apiConfig.requireBearerToken` (extract `Authorization: Bearer <token>` or write 401) and `apiConfig.requireAccessToken` (also validates it as a JWT) centralize the bearer-token check used by `handlerChirpsCreate`, `handlerRefresh`, and `handlerRevoke` instead of repeating it per handler

## Project Structure
```
chirpy/
├── sql/
│   └── schema/
│       ├── 001_users.sql                    # Goose migration: create/drop users table
│       ├── 002_chirps.sql                   # Goose migration: create/drop chirps table (FK -> users ON DELETE CASCADE)
│       ├── 003_users_hashed_password.sql    # Goose migration: add non-null hashed_password column (default 'unset')
│       └── 004_refresh_tokens.sql           # Goose migration: create/drop refresh_tokens table (FK -> users ON DELETE CASCADE)
├── internal/
│   └── auth/        # Password hashing (argon2id) and JWT helpers, see SQLC and DB Wiring above
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
**Note**: Update this file whenever significant changes are made to the server implementation. Keep it under ~200 lines: fold updates into the existing section they belong to rather than appending a new dated section, and trim content that's directly derivable from the code.
